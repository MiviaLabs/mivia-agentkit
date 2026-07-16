// Package orchestrator resolves and executes bounded agent loops.
// Plan: WS10. PRD: FR-4.2, FR-4.3, FR-6.3, FR-7.1.
package orchestrator

import (
	"context"
	"errors"
	"fmt"

	"github.com/MiviaLabs/mivia-agentkit/internal/adapter"
	"github.com/MiviaLabs/mivia-agentkit/internal/config"
	"github.com/MiviaLabs/mivia-agentkit/internal/runstore"
)

// Loop errors returned by RunLoop.
var (
	ErrBudgetNotSupportedInMVP = errors.New("budget bound is not supported in MVP")
	ErrStaleStamp              = errors.New("fresh quality stamp required before protected step")
	ErrMaxIterationsExceeded   = errors.New("max iterations override exceeds manifest bound")
)

// LoopResult summarizes a loop run.
type LoopResult struct {
	Outcome    string         `json:"Outcome"`
	Iterations int            `json:"Iterations"`
	Trace      runstore.RunID `json:"Trace"`
	RunDir     string         `json:"RunDir"`
	Artifacts  []string       `json:"Artifacts,omitempty"`
	Err        error          `json:"Err,omitempty"`
}

// RunLoop executes a bounded workflow loop.
func (e Engine) RunLoop(ctx context.Context, loop config.Loop, pb PromptBuilder) (LoopResult, error) {
	if loop.Bound == "budget" {
		return LoopResult{Err: ErrBudgetNotSupportedInMVP}, ErrBudgetNotSupportedInMVP
	}
	if pb != nil {
		e.PromptBuilder = pb
	}
	nodes, err := Resolve(loop)
	if err != nil {
		return LoopResult{Err: err}, err
	}
	runID := e.Store.NewRun()
	runDir := e.Store.Dir(runID)
	max := loop.MaxIterations
	if max <= 0 {
		max = 1
	}
	if e.MaxIterations > 0 {
		if e.MaxIterations > max {
			return LoopResult{Trace: runID, RunDir: runDir, Err: ErrMaxIterationsExceeded}, ErrMaxIterationsExceeded
		}
		max = e.MaxIterations
	}
	var prior []adapter.Verdict
	var currentArtifact string
	seenArtifacts := map[string]struct{}{}
	var artifacts []string
	trackArtifact := func(path string) {
		if path == "" {
			return
		}
		if _, ok := seenArtifacts[path]; ok {
			return
		}
		seenArtifacts[path] = struct{}{}
		artifacts = append(artifacts, path)
	}
	result := func(outcome string, iteration int, err error) (LoopResult, error) {
		return LoopResult{Outcome: outcome, Iterations: iteration, Trace: runID, RunDir: runDir, Artifacts: artifacts, Err: err}, err
	}
	for iteration := 1; iteration <= max; iteration++ {
		var last StepResult
		var lastNode Node
		for _, node := range nodes {
			lastNode = node
			e.CurrentStamp = ""
			if _, ok := protectedKind(node.Step); ok {
				if e.Stamp == nil {
					return result("", iteration, ErrStaleStamp)
				}
				stampVal, err := e.Stamp(e.Repo)
				if err != nil {
					return result("", iteration, fmt.Errorf("%w: %v", ErrStaleStamp, err))
				}
				e.CurrentStamp = stampVal
			}
			e.PriorVerdicts = prior
			e.CurrentArtifact = currentArtifact
			last, err = e.ExecuteStep(ctx, runID, node, iteration)
			if err != nil {
				return result("", iteration, err)
			}
			if last.Artifact != "" {
				currentArtifact = last.Artifact
				trackArtifact(last.Artifact)
			}
			if len(last.Verdicts) > 0 {
				prior = last.Verdicts
			}
		}
		// Review gates only apply when the last node is a review step.
		if len(lastNode.Step.Reviewers) > 0 {
			if last.Consensus && loop.ExitWhen == "review-pass" {
				return result("pass", iteration, nil)
			}
			if !last.Consensus {
				failAction := stepOnFailValue(lastNode.Step.OnFail, "fail")
				if failAction == "fail" {
					return result("fail", iteration, errors.New("review gate failed"))
				}
			}
			continue
		}
		// Producer-final protected_action loops pass after a successful protect step.
		if loop.ExitWhen == "protected_action" {
			if _, ok := protectedKind(lastNode.Step); ok {
				return result("pass", iteration, nil)
			}
		}
	}
	if loop.OnExhausted == "fail" {
		return result("fail", max, errors.New("loop exhausted"))
	}
	_ = e.Store.AppendTrace(runID, runstore.TraceEvent{Kind: "loop.exhausted", Payload: map[string]any{"on_exhausted": loop.OnExhausted}})
	return result("warn", max, nil)
}

func stepOnFailValue(onFail, fallback string) string {
	if onFail != "" {
		return onFail
	}
	return fallback
}
