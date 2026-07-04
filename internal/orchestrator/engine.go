// Package orchestrator resolves and executes bounded agent loops.
// Plan: WS10. PRD: FR-4.1, FR-5.1, FR-7.1.
package orchestrator

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/MiviaLabs/mivia-agentkit/internal/adapter"
	"github.com/MiviaLabs/mivia-agentkit/internal/config"
	"github.com/MiviaLabs/mivia-agentkit/internal/policy"
	"github.com/MiviaLabs/mivia-agentkit/internal/runstore"
)

// PromptBuilder renders a prompt for a step and prior review notes.
type PromptBuilder func(step config.Step, iteration int, prior []adapter.Verdict) (string, error)

// StampChecker validates quality stamps before protected steps.
type StampChecker func(repo string) (string, error)

// Engine executes resolved loop nodes.
type Engine struct {
	Adapters      *adapter.Registry
	Stamp         StampChecker
	Policy        policy.Provider
	Store         runstore.Store
	Clock         func() time.Time
	PromptBuilder PromptBuilder
	Repo          string
	PriorVerdicts []adapter.Verdict
	MaxIterations int
}

// StepResult is the output of one executed step.
type StepResult struct {
	Artifact        string
	Result          adapter.Result
	Verdicts        []adapter.Verdict
	Consensus       bool
	DecisionRefs    []string
	PolicyDecisions []policy.Decision
}

// ExecuteStep executes one producer or review node.
func (e Engine) ExecuteStep(ctx context.Context, runID runstore.RunID, node Node, iteration int) (StepResult, error) {
	refs, decisions, err := e.decide(ctx, runID, node.Step, "")
	if err != nil {
		return StepResult{}, err
	}
	var res StepResult
	if node.Step.Producer != "" {
		res, err = e.executeProducer(ctx, runID, node, iteration)
	} else {
		res, err = e.executeReview(ctx, runID, node, iteration)
	}
	if err != nil {
		return StepResult{}, err
	}
	res.DecisionRefs = refs
	res.PolicyDecisions = decisions
	return res, nil
}

func (e Engine) executeProducer(ctx context.Context, runID runstore.RunID, node Node, iteration int) (StepResult, error) {
	a, ok := e.Adapters.Lookup(node.Step.Producer)
	if !ok {
		return StepResult{}, fmt.Errorf("adapter %q not found", node.Step.Producer)
	}
	prompt, err := e.prompt(node.Step, iteration)
	if err != nil {
		return StepResult{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, stepTimeout(node.Step))
	defer cancel()
	result, err := a.Run(ctx, adapter.Request{Prompt: prompt, Workdir: e.Repo, Approval: approval(node.Step), ArtifactOut: artifactName(node.Step), Timeout: stepTimeout(node.Step), MaxTurns: node.Step.MaxTurns})
	if err != nil {
		return StepResult{}, err
	}
	path, err := e.Store.WriteArtifact(runID, node.Step.ID, artifactName(node.Step), result.Stdout)
	if err != nil {
		return StepResult{}, err
	}
	if err := e.appendTrace(runID, "step.produced", node.Step.ID, iteration, map[string]any{"adapter": node.Step.Producer, "artifact": path}); err != nil {
		return StepResult{}, err
	}
	return StepResult{Artifact: path, Result: result}, nil
}

func (e Engine) executeReview(ctx context.Context, runID runstore.RunID, node Node, iteration int) (StepResult, error) {
	ctx, cancel := context.WithTimeout(ctx, stepTimeout(node.Step))
	defer cancel()
	type item struct {
		i        int
		reviewer string
		verdict  adapter.Verdict
		err      error
	}
	results := make(chan item, len(node.Step.Reviewers))
	var wg sync.WaitGroup
	for i, reviewer := range node.Step.Reviewers {
		i, reviewer := i, reviewer
		wg.Add(1)
		go func() {
			defer wg.Done()
			a, ok := e.Adapters.Lookup(reviewer)
			if !ok {
				results <- item{i: i, reviewer: reviewer, err: fmt.Errorf("adapter %q not found", reviewer)}
				return
			}
			prompt, err := e.prompt(node.Step, iteration)
			if err != nil {
				results <- item{i: i, reviewer: reviewer, err: err}
				return
			}
			v, err := a.Review(ctx, adapter.Request{Prompt: prompt, Workdir: e.Repo, Approval: approval(node.Step), Timeout: stepTimeout(node.Step), MaxTurns: node.Step.MaxTurns})
			results <- item{i: i, reviewer: reviewer, verdict: v, err: err}
		}()
	}
	wg.Wait()
	close(results)
	verdicts := make([]adapter.Verdict, len(node.Step.Reviewers))
	for item := range results {
		if item.err != nil {
			return StepResult{}, item.err
		}
		item.verdict.Adapter = item.reviewer
		verdicts[item.i] = item.verdict
		if err := e.appendTrace(runID, "step.reviewed", node.Step.ID, iteration, map[string]any{"reviewer": item.reviewer, "pass": item.verdict.Pass}); err != nil {
			return StepResult{}, err
		}
	}
	pass := consensusPass(verdicts)
	if err := e.appendTrace(runID, "step.consensus", node.Step.ID, iteration, map[string]any{"pass": pass}); err != nil {
		return StepResult{}, err
	}
	return StepResult{Verdicts: verdicts, Consensus: pass}, nil
}

func (e Engine) decide(ctx context.Context, runID runstore.RunID, step config.Step, stamp string) ([]string, []policy.Decision, error) {
	if e.Policy == nil {
		return nil, nil, nil
	}
	action := policy.Action{Kind: policy.ActionLoopStep, Step: step.ID, RunID: string(runID), Stamp: stamp}
	if kind, ok := protectedKind(step); ok {
		action.Kind = policy.ActionProtect
		action.ProtectedKind = kind
	}
	decision, err := e.Policy.Decide(ctx, action)
	if err != nil {
		return nil, nil, err
	}
	if !decision.Allowed {
		return nil, nil, fmt.Errorf("policy denied step %q: %s", step.ID, decision.Reason)
	}
	return []string{decision.Ref}, []policy.Decision{decision}, nil
}

func (e Engine) appendTrace(id runstore.RunID, kind, step string, iteration int, payload map[string]any) error {
	return e.Store.AppendTrace(id, runstore.TraceEvent{TS: e.now().UTC().Format(time.RFC3339), Kind: kind, Step: step, Iteration: iteration, Payload: payload})
}

func (e Engine) prompt(step config.Step, iteration int) (string, error) {
	if e.PromptBuilder == nil {
		return "", nil
	}
	return e.PromptBuilder(step, iteration, e.PriorVerdicts)
}

func (e Engine) now() time.Time {
	if e.Clock != nil {
		return e.Clock()
	}
	return time.Now()
}

func stepTimeout(step config.Step) time.Duration {
	if step.Timeout == "" {
		return 5 * time.Minute
	}
	d, err := time.ParseDuration(step.Timeout)
	if err != nil {
		return 5 * time.Minute
	}
	return d
}

func approval(step config.Step) string {
	if step.Approval == "" || strings.HasPrefix(step.Approval, "protect:") {
		return "never"
	}
	return step.Approval
}

func artifactName(step config.Step) string {
	if step.Artifact == "" {
		return "artifact.txt"
	}
	return step.Artifact
}

func consensusPass(verdicts []adapter.Verdict) bool {
	if len(verdicts) == 0 {
		return false
	}
	for _, v := range verdicts {
		if !v.Pass {
			return false
		}
	}
	return true
}

func protectedKind(step config.Step) (policy.ProtectedKind, bool) {
	raw, ok := strings.CutPrefix(step.Approval, "protect:")
	if !ok {
		return "", false
	}
	return policy.ProtectedKind(raw), true
}
