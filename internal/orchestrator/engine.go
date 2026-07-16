// Package orchestrator resolves and executes bounded agent loops.
// Plan: WS-B. PRD: FR-4.1, FR-5.1, FR-7.1.
package orchestrator

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/MiviaLabs/mivia-agentkit/internal/adapter"
	"github.com/MiviaLabs/mivia-agentkit/internal/config"
	"github.com/MiviaLabs/mivia-agentkit/internal/consensus"
	"github.com/MiviaLabs/mivia-agentkit/internal/policy"
	"github.com/MiviaLabs/mivia-agentkit/internal/runstore"
)

// PromptBuilder renders a prompt for a step, prior review notes, and any concrete artifact path.
type PromptBuilder func(step config.Step, iteration int, prior []adapter.Verdict, artifactPath string) (string, error)

// StampChecker validates quality stamps before protected steps.
type StampChecker func(repo string) (string, error)

// Engine executes resolved loop nodes.
type Engine struct {
	Adapters         *adapter.Registry
	Stamp            StampChecker
	Policy           policy.Provider
	Store            runstore.Store
	AdapterDefaults  map[string]config.AdapterConfig
	DefaultConsensus consensus.Policy
	Clock            func() time.Time
	PromptBuilder    PromptBuilder
	Repo             string
	PriorVerdicts    []adapter.Verdict
	CurrentArtifact  string
	// CurrentStamp is the fresh quality stamp head/hash bound into protect
	// policy decisions for the step currently under execution.
	CurrentStamp  string
	MaxIterations int
}

// StepResult is the output of one executed step.
type StepResult struct {
	Artifact         string
	Result           adapter.Result
	Verdicts         []adapter.Verdict
	Consensus        bool
	ConsensusOutcome consensus.Outcome
	DecisionRefs     []string
	PolicyDecisions  []policy.Decision
}

// ExecuteStep executes one producer or review node.
func (e Engine) ExecuteStep(ctx context.Context, runID runstore.RunID, node Node, iteration int) (StepResult, error) {
	refs, decisions, err := e.decide(ctx, runID, node.Step, e.CurrentStamp)
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
	prompt, err := e.prompt(node.Step, iteration, "")
	if err != nil {
		return StepResult{}, err
	}
	timeout, err := parseStepTimeout(node.Step)
	if err != nil {
		return StepResult{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req := e.requestFor(node.Step.Producer, node.Step, prompt, true)
	path, err := e.Store.WriteArtifact(runID, node.Step.ID, iteration, artifactName(node.Step), nil)
	if err != nil {
		return StepResult{}, err
	}
	req.ArtifactOut = path
	if err := validateAdapterRequest(a, req); err != nil {
		return StepResult{}, err
	}
	result, err := a.Run(ctx, req)
	if err != nil {
		return StepResult{}, err
	}
	content := result.Stdout
	if written, readErr := os.ReadFile(path); readErr == nil && len(written) > 0 {
		content = written
	}
	// Always scrub before persisting under .ai/runs — adapter file writes may
	// bypass stdout scrubbing (e.g. Codex --output-last-message).
	content = adapter.Scrub(content)
	path, err = e.Store.WriteArtifact(runID, node.Step.ID, iteration, artifactName(node.Step), content)
	if err != nil {
		return StepResult{}, err
	}
	if err := e.appendTrace(runID, "step.produced", node.Step.ID, iteration, map[string]any{"adapter": node.Step.Producer, "artifact": path}); err != nil {
		return StepResult{}, err
	}
	return StepResult{Artifact: path, Result: result}, nil
}

func (e Engine) executeReview(ctx context.Context, runID runstore.RunID, node Node, iteration int) (StepResult, error) {
	timeout, err := parseStepTimeout(node.Step)
	if err != nil {
		return StepResult{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	type item struct {
		i        int
		reviewer string
		verdict  adapter.Verdict
		err      error
	}
	results := make(chan item, len(node.Step.Reviewers))
	type reviewCall struct {
		i        int
		reviewer string
		adapter  adapter.Adapter
		request  adapter.Request
	}
	calls := make([]reviewCall, 0, len(node.Step.Reviewers))
	for i, reviewer := range node.Step.Reviewers {
		a, ok := e.Adapters.Lookup(reviewer)
		if !ok {
			return StepResult{}, fmt.Errorf("adapter %q not found", reviewer)
		}
		prompt, err := e.prompt(node.Step, iteration, e.CurrentArtifact)
		if err != nil {
			return StepResult{}, err
		}
		req := e.requestFor(reviewer, node.Step, prompt, false)
		if err := validateAdapterRequest(a, req); err != nil {
			return StepResult{}, err
		}
		calls = append(calls, reviewCall{i: i, reviewer: reviewer, adapter: a, request: req})
	}
	var wg sync.WaitGroup
	for _, call := range calls {
		call := call
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, err := call.adapter.Review(ctx, call.request)
			results <- item{i: call.i, reviewer: call.reviewer, verdict: v, err: err}
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
	policy := stepConsensusPolicy(node.Step.Consensus, e.DefaultConsensus)
	outcome, err := consensus.Evaluate(policy, verdicts)
	if err != nil {
		return StepResult{}, fmt.Errorf("consensus: %w", err)
	}
	if err := e.appendTrace(runID, "step.consensus", node.Step.ID, iteration, map[string]any{
		"pass":   outcome.Pass,
		"reason": outcome.Reason,
		"mode":   string(policy.Mode),
		"tied":   outcome.Tied,
	}); err != nil {
		return StepResult{}, err
	}
	return StepResult{Verdicts: verdicts, Consensus: outcome.Pass, ConsensusOutcome: outcome}, nil
}

func validateAdapterRequest(a adapter.Adapter, req adapter.Request) error {
	if validator, ok := a.(adapter.RequestValidator); ok {
		return validator.ValidateRequest(req)
	}
	return req.Validate()
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

func (e Engine) prompt(step config.Step, iteration int, artifactPath string) (string, error) {
	if e.PromptBuilder == nil {
		return "", nil
	}
	return e.PromptBuilder(step, iteration, e.PriorVerdicts, artifactPath)
}

func (e Engine) now() time.Time {
	if e.Clock != nil {
		return e.Clock()
	}
	return time.Now()
}

func stepTimeout(step config.Step) time.Duration {
	d, err := parseStepTimeout(step)
	if err != nil {
		// Callers that need hard rejection use parseStepTimeout; default only
		// when timeout is empty (already handled). Invalid values fall back to
		// a short cancel to avoid unbounded runs if validation was skipped.
		return time.Millisecond
	}
	return d
}

// parseStepTimeout rejects empty-safe, non-positive, or unparseable timeouts.
func parseStepTimeout(step config.Step) (time.Duration, error) {
	if step.Timeout == "" {
		return 5 * time.Minute, nil
	}
	d, err := time.ParseDuration(step.Timeout)
	if err != nil {
		return 0, fmt.Errorf("step %q timeout %q: %w", step.ID, step.Timeout, err)
	}
	if d <= 0 {
		return 0, fmt.Errorf("step %q timeout must be positive", step.ID)
	}
	return d, nil
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

func (e Engine) requestFor(adapterName string, step config.Step, prompt string, producer bool) adapter.Request {
	defaults := e.AdapterDefaults[adapterName]
	model := defaults.Model
	if step.Model != "" {
		model = step.Model
	}
	effort := defaults.Effort
	if step.Effort != "" {
		effort = step.Effort
	}
	req := adapter.Request{
		Prompt:   prompt,
		Workdir:  e.Repo,
		Approval: approval(step),
		Model:    model,
		Effort:   effort,
		Params:   copyParams(defaults.Params),
		Timeout:  stepTimeout(step),
		MaxTurns: step.MaxTurns,
	}
	return req
}

func copyParams(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]string, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

func stepConsensusPolicy(step config.Consensus, fallback consensus.Policy) consensus.Policy {
	if step.Mode != "" {
		p := consensus.Policy{
			Mode:         consensus.Mode(step.Mode),
			MinReviewers: step.MinReviewers,
			TieBreaker:   consensus.TieBreaker(step.TieBreaker),
			Weights:      config.WeightsToFloat(step.Weights),
		}
		// Inherit unset fields from the manifest/engine default so a step that
		// only sets mode cannot silently drop min_reviewers or tie-breakers.
		if p.MinReviewers == 0 {
			p.MinReviewers = fallback.MinReviewers
		}
		if p.TieBreaker == "" {
			p.TieBreaker = fallback.TieBreaker
		}
		if len(p.Weights) == 0 {
			p.Weights = fallback.Weights
		}
		if step.Mode == "weighted" {
			p.Threshold = fallback.Threshold
		}
		return p
	}
	return fallback
}

func protectedKind(step config.Step) (policy.ProtectedKind, bool) {
	raw, ok := strings.CutPrefix(step.Approval, "protect:")
	if !ok {
		return "", false
	}
	return policy.ProtectedKind(raw), true
}
