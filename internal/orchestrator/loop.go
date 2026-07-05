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
	Outcome    string
	Iterations int
	Trace      runstore.RunID
	Err        error
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
	max := loop.MaxIterations
	if max <= 0 {
		max = 1
	}
	if e.MaxIterations > 0 {
		if e.MaxIterations > max {
			return LoopResult{Trace: runID, Err: ErrMaxIterationsExceeded}, ErrMaxIterationsExceeded
		}
		max = e.MaxIterations
	}
	var prior []adapter.Verdict
	var currentArtifact string
	for iteration := 1; iteration <= max; iteration++ {
		var last StepResult
		for _, node := range nodes {
			if _, ok := protectedKind(node.Step); ok {
				if e.Stamp == nil {
					return LoopResult{Iterations: iteration, Trace: runID, Err: ErrStaleStamp}, ErrStaleStamp
				}
				if _, err := e.Stamp(e.Repo); err != nil {
					return LoopResult{Iterations: iteration, Trace: runID, Err: ErrStaleStamp}, fmt.Errorf("%w: %v", ErrStaleStamp, err)
				}
			}
			e.PriorVerdicts = prior
			e.CurrentArtifact = currentArtifact
			last, err = e.ExecuteStep(ctx, runID, node, iteration)
			if err != nil {
				return LoopResult{Iterations: iteration, Trace: runID, Err: err}, err
			}
			if last.Artifact != "" {
				currentArtifact = last.Artifact
			}
			if len(last.Verdicts) > 0 {
				prior = last.Verdicts
			}
		}
		if last.Consensus && loop.ExitWhen == "review-pass" {
			return LoopResult{Outcome: "pass", Iterations: iteration, Trace: runID}, nil
		}
		if !last.Consensus && loopOnFail(loop) == "fail" {
			err := errors.New("review gate failed")
			return LoopResult{Outcome: "fail", Iterations: iteration, Trace: runID, Err: err}, err
		}
	}
	if loop.OnExhausted == "fail" {
		err := errors.New("loop exhausted")
		return LoopResult{Outcome: "fail", Iterations: max, Trace: runID, Err: err}, err
	}
	_ = e.Store.AppendTrace(runID, runstore.TraceEvent{Kind: "loop.exhausted", Payload: map[string]any{"on_exhausted": loop.OnExhausted}})
	return LoopResult{Outcome: "warn", Iterations: max, Trace: runID}, nil
}

func loopOnFail(loop config.Loop) string {
	for _, step := range loop.Steps {
		if step.OnFail != "" {
			return step.OnFail
		}
	}
	return "iterate"
}
