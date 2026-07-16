// Package orchestrator resolves and executes bounded agent loops.
// Plan: WS10. PRD: FR-4.2, FR-4.3, FR-6.3, FR-7.1.
package orchestrator

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/MiviaLabs/mivia-agentkit/internal/adapter"
	"github.com/MiviaLabs/mivia-agentkit/internal/config"
	"github.com/MiviaLabs/mivia-agentkit/internal/preflight"
	"github.com/MiviaLabs/mivia-agentkit/internal/runstore"
)

func TestLoopExitsWhenGatePasses(t *testing.T) {
	e := testEngine(t, scriptedAdapter{name: "codex", run: adapter.Result{Stdout: []byte("artifact")}}, sequenceReviewer("claude", true))
	res, err := e.RunLoop(context.Background(), testLoop(3, "iterate", "fail"), nil)
	if err != nil {
		t.Fatalf("RunLoop error = %v", err)
	}
	if res.Outcome != "pass" || res.Iterations != 1 {
		t.Fatalf("result = %#v, want pass in 1 iteration", res)
	}
}

func TestLoopIteratesOnReviewFail(t *testing.T) {
	reviewer := &sequenceAdapter{name: "claude", verdicts: []adapter.Verdict{{Pass: false}, {Pass: true}}}
	e := testEngine(t, scriptedAdapter{name: "codex", run: adapter.Result{Stdout: []byte("artifact")}}, reviewer)
	res, err := e.RunLoop(context.Background(), testLoop(3, "iterate", "fail"), nil)
	if err != nil {
		t.Fatalf("RunLoop error = %v", err)
	}
	if res.Iterations != 2 || reviewer.calls != 2 {
		t.Fatalf("iterations=%d reviewer calls=%d, want 2", res.Iterations, reviewer.calls)
	}
}

func TestLoopPreservesArtifactsAcrossIterations(t *testing.T) {
	producer := &sequenceProducerAdapter{name: "codex", outputs: []string{"artifact-one", "artifact-two"}}
	reviewer := &sequenceAdapter{name: "claude", verdicts: []adapter.Verdict{{Pass: false}, {Pass: true}}}
	e := testEngine(t, producer, reviewer)
	res, err := e.RunLoop(context.Background(), testLoop(3, "iterate", "fail"), nil)
	if err != nil {
		t.Fatalf("RunLoop error = %v", err)
	}
	first, err := os.ReadFile(filepath.Join(e.Store.Dir(res.Trace), "produce", "iter-001", "artifact.txt"))
	if err != nil {
		t.Fatalf("read first iteration artifact: %v", err)
	}
	second, err := os.ReadFile(filepath.Join(e.Store.Dir(res.Trace), "produce", "iter-002", "artifact.txt"))
	if err != nil {
		t.Fatalf("read second iteration artifact: %v", err)
	}
	if string(first) != "artifact-one" || string(second) != "artifact-two" {
		t.Fatalf("artifacts = %q / %q, want per-iteration preservation", first, second)
	}
}

func TestLoopFeedsReviewNotesIntoNextProducerPrompt(t *testing.T) {
	reviewer := &sequenceAdapter{name: "claude", verdicts: []adapter.Verdict{{Pass: false, Notes: "fix it"}, {Pass: true}}}
	e := testEngine(t, scriptedAdapter{name: "codex", run: adapter.Result{Stdout: []byte("artifact")}}, reviewer)
	var secondPrior []adapter.Verdict
	e.PromptBuilder = func(step config.Step, iteration int, prior []adapter.Verdict, artifactPath string) (string, error) {
		if step.ID == "produce" && iteration == 2 {
			secondPrior = append([]adapter.Verdict(nil), prior...)
		}
		return "prompt", nil
	}
	if _, err := e.RunLoop(context.Background(), testLoop(3, "iterate", "fail"), nil); err != nil {
		t.Fatalf("RunLoop error = %v", err)
	}
	if len(secondPrior) != 1 || secondPrior[0].Notes != "fix it" {
		t.Fatalf("second prior = %#v, want reviewer notes from failed iteration", secondPrior)
	}
}

func TestLoopFailsOnExhaustionWithOnExhaustedFail(t *testing.T) {
	e := testEngine(t, scriptedAdapter{name: "codex", run: adapter.Result{Stdout: []byte("artifact")}}, sequenceReviewer("claude", false))
	res, err := e.RunLoop(context.Background(), testLoop(1, "iterate", "fail"), nil)
	if err == nil || res.Outcome != "fail" {
		t.Fatalf("result=%#v err=%v, want fail", res, err)
	}
}

func TestLoopWarnsOnExhaustionWithOnExhaustedWarn(t *testing.T) {
	e := testEngine(t, scriptedAdapter{name: "codex", run: adapter.Result{Stdout: []byte("artifact")}}, sequenceReviewer("claude", false))
	res, err := e.RunLoop(context.Background(), testLoop(1, "iterate", "warn"), nil)
	if err != nil {
		t.Fatalf("RunLoop error = %v", err)
	}
	if res.Outcome != "warn" {
		t.Fatalf("outcome = %q, want warn", res.Outcome)
	}
	AssertNoLeaks(t, e.Store.Dir(res.Trace))
}

func TestLoopRejectsBudgetBoundInMVP(t *testing.T) {
	e := testEngine(t)
	_, err := e.RunLoop(context.Background(), config.Loop{Bound: "budget", MaxIterations: 1}, nil)
	if !errors.Is(err, ErrBudgetNotSupportedInMVP) {
		t.Fatalf("RunLoop error = %v, want ErrBudgetNotSupportedInMVP", err)
	}
}

func TestLoopRequiresFreshStampBeforeProtectedStep(t *testing.T) {
	e := testEngine(t, scriptedAdapter{name: "codex", run: adapter.Result{Stdout: []byte("artifact")}})
	e.Stamp = func(repo string) (string, error) { return "", errors.New("stale") }
	loop := config.Loop{Bound: "iterations", MaxIterations: 1, ExitWhen: "review-pass", OnExhausted: "fail", Steps: []config.Step{{ID: "protect", Producer: "codex", Approval: "protect:commit"}}}
	_, err := e.RunLoop(context.Background(), loop, nil)
	if !errors.Is(err, ErrStaleStamp) {
		t.Fatalf("RunLoop error = %v, want ErrStaleStamp", err)
	}
}

func TestLoopPassesProtectedActionProducer(t *testing.T) {
	e := testEngine(t, scriptedAdapter{name: "codex", run: adapter.Result{Stdout: []byte("artifact")}})
	e.Stamp = func(repo string) (string, error) { return "fresh-head", nil }
	loop := config.Loop{
		Bound: "iterations", MaxIterations: 1, ExitWhen: "protected_action", OnExhausted: "fail",
		Steps: []config.Step{{ID: "protect", Producer: "codex", Approval: "protect:commit"}},
	}
	res, err := e.RunLoop(context.Background(), loop, nil)
	if err != nil {
		t.Fatalf("RunLoop error = %v", err)
	}
	if res.Outcome != "pass" || res.Iterations != 1 {
		t.Fatalf("result = %#v, want pass in 1 iteration for protected_action producer", res)
	}
}

func TestProtectedStepPreservesErrNoStamp(t *testing.T) {
	e := testEngine(t, scriptedAdapter{name: "codex", run: adapter.Result{Stdout: []byte("artifact")}})
	e.Stamp = func(repo string) (string, error) { return "", preflight.ErrNoStamp }
	loop := config.Loop{
		Bound: "iterations", MaxIterations: 1, ExitWhen: "protected_action", OnExhausted: "fail",
		Steps: []config.Step{{ID: "protect", Producer: "codex", Approval: "protect:commit"}},
	}
	_, err := e.RunLoop(context.Background(), loop, nil)
	if !errors.Is(err, ErrStaleStamp) {
		t.Fatalf("RunLoop error = %v, want ErrStaleStamp", err)
	}
	if !errors.Is(err, preflight.ErrNoStamp) {
		t.Fatalf("RunLoop error = %v, want errors.Is preflight.ErrNoStamp", err)
	}
}

func TestRunLoopExhaustTraceWriteFailureSurfaces(t *testing.T) {
	// Store root under a file path so NewRun/AppendTrace cannot create dirs.
	badRoot := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(badRoot, []byte("x"), 0o644); err != nil {
		t.Fatalf("write bad root: %v", err)
	}
	e := testEngine(t, scriptedAdapter{name: "codex", run: adapter.Result{Stdout: []byte("artifact")}}, sequenceReviewer("claude", false))
	e.Store = runstore.Store{Root: filepath.Join(badRoot, "runs")}
	_, err := e.RunLoop(context.Background(), testLoop(1, "iterate", "warn"), nil)
	if err == nil {
		t.Fatalf("RunLoop error = nil, want create run / exhaust-trace failure")
	}
}

func TestLoopHonorsMaxIterationsOverrideWithinManifestBound(t *testing.T) {
	e := testEngine(t, scriptedAdapter{name: "codex", run: adapter.Result{Stdout: []byte("artifact")}}, sequenceReviewer("claude", false))
	e.MaxIterations = 2
	loop := testLoop(1, "iterate", "warn")
	if _, err := e.RunLoop(context.Background(), loop, nil); !errors.Is(err, ErrMaxIterationsExceeded) {
		t.Fatalf("RunLoop error = %v, want ErrMaxIterationsExceeded", err)
	}
	e.MaxIterations = 1
	res, err := e.RunLoop(context.Background(), loop, nil)
	if err != nil {
		t.Fatalf("RunLoop error = %v", err)
	}
	if res.Iterations != 1 {
		t.Fatalf("iterations = %d, want manifest bound 1", res.Iterations)
	}
}

func testLoop(max int, onFail, exhausted string) config.Loop {
	return config.Loop{Bound: "iterations", MaxIterations: max, ExitWhen: "review-pass", OnExhausted: exhausted, Steps: []config.Step{{ID: "produce", Producer: "codex"}, {ID: "review", Reviewers: []string{"claude"}, OnFail: onFail}}}
}

func sequenceReviewer(name string, pass bool) *sequenceAdapter {
	return &sequenceAdapter{name: name, verdicts: []adapter.Verdict{{Pass: pass}}}
}

type sequenceAdapter struct {
	name     string
	verdicts []adapter.Verdict
	calls    int
}

func (s *sequenceAdapter) Name() string             { return s.name }
func (s *sequenceAdapter) Role() config.AdapterRole { return config.AdapterRoleOrchestrable }
func (s *sequenceAdapter) Detect(context.Context) (adapter.Detection, error) {
	return adapter.Detection{Name: s.name, HeadlessCapable: true}, nil
}
func (s *sequenceAdapter) Run(context.Context, adapter.Request) (adapter.Result, error) {
	return adapter.Result{}, nil
}
func (s *sequenceAdapter) Review(context.Context, adapter.Request) (adapter.Verdict, error) {
	i := s.calls
	s.calls++
	if i >= len(s.verdicts) {
		i = len(s.verdicts) - 1
	}
	return s.verdicts[i], nil
}

type sequenceProducerAdapter struct {
	name    string
	outputs []string
	calls   int
}

func (s *sequenceProducerAdapter) Name() string             { return s.name }
func (s *sequenceProducerAdapter) Role() config.AdapterRole { return config.AdapterRoleOrchestrable }
func (s *sequenceProducerAdapter) Detect(context.Context) (adapter.Detection, error) {
	return adapter.Detection{Name: s.name, HeadlessCapable: true}, nil
}
func (s *sequenceProducerAdapter) Run(context.Context, adapter.Request) (adapter.Result, error) {
	i := s.calls
	s.calls++
	if i >= len(s.outputs) {
		i = len(s.outputs) - 1
	}
	return adapter.Result{Stdout: []byte(s.outputs[i])}, nil
}
func (s *sequenceProducerAdapter) Review(context.Context, adapter.Request) (adapter.Verdict, error) {
	return adapter.Verdict{}, nil
}
