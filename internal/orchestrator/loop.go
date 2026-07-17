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
	runID, err := e.Store.NewRun()
	if err != nil {
		return LoopResult{Err: err}, err
	}
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
		// skipRemaining is set when a mid-loop review fails with on_fail:iterate
		// so later nodes (including protect:) do not run in this iteration.
		skipRemaining := false
		for _, node := range nodes {
			if skipRemaining {
				break
			}
			lastNode = node
			e.CurrentStamp = ""
			if _, ok := protectedKind(node.Step); ok {
				if e.Stamp == nil {
					return result("", iteration, ErrStaleStamp)
				}
				stampVal, stampErr := e.Stamp(e.Repo)
				if stampErr != nil {
					// Keep both the orchestrator sentinel and the underlying
					// preflight error (e.g. ErrNoStamp) for errors.Is checks.
					return result("", iteration, fmt.Errorf("%w: %w", ErrStaleStamp, stampErr))
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
			// Apply review gates immediately so later protect steps never run
			// after a failed review that demands fail or iterate.
			if len(node.Step.Reviewers) > 0 && !last.Consensus {
				failAction := stepOnFailValue(node.Step.OnFail, "fail")
				switch failAction {
				case "iterate":
					skipRemaining = true
				case "proceed":
					// Continue to subsequent nodes despite the failed review.
				default:
					// Unknown/empty-default "fail" and any typo: fail closed.
					return result("fail", iteration, errors.New("review gate failed"))
				}
			}
		}
		if skipRemaining {
			continue
		}
		// Successful review-pass exit after all nodes in the iteration.
		if len(lastNode.Step.Reviewers) > 0 && last.Consensus && loop.ExitWhen == "review-pass" {
			return result("pass", iteration, nil)
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
	outcome := "warn"
	if loop.OnExhausted == "proceed" {
		outcome = "proceed"
	}
	if err := e.Store.AppendTrace(runID, runstore.TraceEvent{Kind: "loop.exhausted", Payload: map[string]any{"on_exhausted": loop.OnExhausted}}); err != nil {
		return result(outcome, max, fmt.Errorf("append exhaust trace: %w", err))
	}
	return result(outcome, max, nil)
}

func stepOnFailValue(onFail, fallback string) string {
	if onFail != "" {
		return onFail
	}
	return fallback
}
