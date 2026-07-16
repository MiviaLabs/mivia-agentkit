// Package orchestrator tests bounded agent loop execution.
// Plan: WS-B. PRD: FR-4.1, FR-5.1, FR-7.1.
package orchestrator

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MiviaLabs/mivia-agentkit/internal/adapter"
	"github.com/MiviaLabs/mivia-agentkit/internal/config"
	"github.com/MiviaLabs/mivia-agentkit/internal/consensus"
	"github.com/MiviaLabs/mivia-agentkit/internal/policy"
	"github.com/MiviaLabs/mivia-agentkit/internal/runstore"
)

func TestExecuteProducerStepWritesArtifact(t *testing.T) {
	e := testEngine(t, scriptedAdapter{name: "codex", run: adapter.Result{Stdout: []byte("artifact")}})
	res, err := e.ExecuteStep(context.Background(), e.Store.NewRun(), Node{Step: config.Step{ID: "produce", Producer: "codex", Artifact: "out.md"}}, 1)
	if err != nil {
		t.Fatalf("ExecuteStep error = %v", err)
	}
	got, err := os.ReadFile(res.Artifact)
	if err != nil {
		t.Fatalf("read artifact: %v", err)
	}
	if string(got) != "artifact" {
		t.Fatalf("artifact = %q, want artifact", got)
	}
}

func TestExecuteProducerStepUsesIterationArtifactPath(t *testing.T) {
	e := testEngine(t, scriptedAdapter{name: "codex", run: adapter.Result{Stdout: []byte("artifact")}})
	res, err := e.ExecuteStep(context.Background(), e.Store.NewRun(), Node{Step: config.Step{ID: "produce", Producer: "codex", Artifact: "out.md"}}, 2)
	if err != nil {
		t.Fatalf("ExecuteStep error = %v", err)
	}
	if !strings.Contains(res.Artifact, filepath.Join("produce", "iter-002", "out.md")) {
		t.Fatalf("artifact path = %q, want iteration subdirectory", res.Artifact)
	}
}

func TestExecuteProducerStepPassesArtifactOutAndStoresAdapterFile(t *testing.T) {
	producer := &artifactOutAdapter{name: "codex", stdout: []byte(`{"type":"event"}`), fileContent: []byte("final artifact")}
	e := testEngine(t, producer)
	res, err := e.ExecuteStep(context.Background(), e.Store.NewRun(), Node{Step: config.Step{ID: "produce", Producer: "codex", Artifact: "out.md"}}, 1)
	if err != nil {
		t.Fatalf("ExecuteStep error = %v", err)
	}
	if !strings.Contains(producer.artifactOut, filepath.Join(".ai", "runs")) || !strings.Contains(producer.artifactOut, filepath.Join("produce", "iter-001", "out.md")) {
		t.Fatalf("ArtifactOut = %q, want concrete run artifact path", producer.artifactOut)
	}
	got, err := os.ReadFile(res.Artifact)
	if err != nil {
		t.Fatalf("read artifact: %v", err)
	}
	if string(got) != "final artifact" {
		t.Fatalf("artifact = %q, want adapter-written final artifact", got)
	}
}

func TestExecuteProducerScrubsSecretsFromArtifactOut(t *testing.T) {
	secret := "API_KEY=" + strings.Repeat("a", 16)
	producer := &artifactOutAdapter{
		name:        "codex",
		stdout:      []byte(`{"type":"event"}`),
		fileContent: []byte("leaked " + secret + " value"),
	}
	e := testEngine(t, producer)
	res, err := e.ExecuteStep(context.Background(), e.Store.NewRun(), Node{Step: config.Step{ID: "produce", Producer: "codex", Artifact: "out.md"}}, 1)
	if err != nil {
		t.Fatalf("ExecuteStep error = %v", err)
	}
	got, err := os.ReadFile(res.Artifact)
	if err != nil {
		t.Fatalf("read artifact: %v", err)
	}
	if strings.Contains(string(got), secret) {
		t.Fatalf("artifact still contains secret %q: %q", secret, got)
	}
	if !strings.Contains(string(got), "<redacted:env>") {
		t.Fatalf("artifact = %q, want redacted env secret", got)
	}
}

func TestExecuteReviewStepFansOutConcurrently(t *testing.T) {
	e := testEngine(t, scriptedAdapter{name: "a", delay: 200 * time.Millisecond, verdict: adapter.Verdict{Pass: true}}, scriptedAdapter{name: "b", delay: 200 * time.Millisecond, verdict: adapter.Verdict{Pass: true}})
	start := time.Now()
	_, err := e.ExecuteStep(context.Background(), e.Store.NewRun(), Node{Step: config.Step{ID: "review", Reviewers: []string{"a", "b"}}}, 1)
	if err != nil {
		t.Fatalf("ExecuteStep error = %v", err)
	}
	if elapsed := time.Since(start); elapsed >= 350*time.Millisecond {
		t.Fatalf("review elapsed = %s, want concurrent under 350ms", elapsed)
	}
}

func TestExecuteReviewStepCollectsAllVerdicts(t *testing.T) {
	e := testEngine(t, scriptedAdapter{name: "a", verdict: adapter.Verdict{Pass: true, Notes: "a"}}, scriptedAdapter{name: "b", verdict: adapter.Verdict{Pass: false, Notes: "b"}})
	res, err := e.ExecuteStep(context.Background(), e.Store.NewRun(), Node{Step: config.Step{ID: "review", Reviewers: []string{"a", "b"}}}, 1)
	if err != nil {
		t.Fatalf("ExecuteStep error = %v", err)
	}
	if len(res.Verdicts) != 2 || res.Verdicts[0].Notes != "a" || res.Verdicts[1].Notes != "b" {
		t.Fatalf("verdicts = %#v, want both in reviewer order", res.Verdicts)
	}
}

func TestExecuteReviewStepSetsVerdictAdapterNames(t *testing.T) {
	e := testEngine(t, scriptedAdapter{name: "a", verdict: adapter.Verdict{Pass: true}}, scriptedAdapter{name: "b", verdict: adapter.Verdict{Pass: false}})
	res, err := e.ExecuteStep(context.Background(), e.Store.NewRun(), Node{Step: config.Step{ID: "review", Reviewers: []string{"a", "b"}}}, 1)
	if err != nil {
		t.Fatalf("ExecuteStep error = %v", err)
	}
	if len(res.Verdicts) != 2 || res.Verdicts[0].Adapter != "a" || res.Verdicts[1].Adapter != "b" {
		t.Fatalf("verdict adapters = %#v, want reviewer names in order", res.Verdicts)
	}
}

func TestExecuteReviewStepSendsReviewPrompt(t *testing.T) {
	a := &promptRecorderAdapter{name: "a", verdict: adapter.Verdict{Pass: true}}
	e := testEngine(t, a)
	e.CurrentArtifact = "/tmp/run/produce/iter-001/artifact.md"
	e.PromptBuilder = func(step config.Step, iteration int, prior []adapter.Verdict, artifactPath string) (string, error) {
		return "review " + artifactPath, nil
	}
	if _, err := e.ExecuteStep(context.Background(), e.Store.NewRun(), Node{Step: config.Step{ID: "review", Reviewers: []string{"a"}, Artifact: "artifact.md"}}, 1); err != nil {
		t.Fatalf("ExecuteStep error = %v", err)
	}
	if a.prompt != "review /tmp/run/produce/iter-001/artifact.md" {
		t.Fatalf("review prompt = %q, want concrete artifact path", a.prompt)
	}
}

func TestExecuteReviewStepValidatesRequestsBeforeFanout(t *testing.T) {
	var reviewCalls int
	started := &validatingReviewAdapter{name: "started", verdict: adapter.Verdict{Pass: true}, reviewCalls: &reviewCalls}
	blocked := &validatingReviewAdapter{name: "blocked", validateErr: errors.New("blocked invalid request")}
	e := testEngine(t, started, blocked)

	_, err := e.ExecuteStep(context.Background(), e.Store.NewRun(), Node{Step: config.Step{ID: "review", Reviewers: []string{"started", "blocked"}}}, 1)
	if err == nil || !strings.Contains(err.Error(), "blocked invalid request") {
		t.Fatalf("ExecuteStep error = %v, want blocked invalid request", err)
	}
	if reviewCalls != 0 {
		t.Fatalf("review calls = %d, want 0 before all reviewer requests validate", reviewCalls)
	}
}

func TestExecuteProducerStepValidatesRequestBeforeRun(t *testing.T) {
	var runCalls int
	producer := &validatingReviewAdapter{name: "codex", validateErr: errors.New("producer invalid request"), runCalls: &runCalls}
	e := testEngine(t, producer)

	_, err := e.ExecuteStep(context.Background(), e.Store.NewRun(), Node{Step: config.Step{ID: "produce", Producer: "codex"}}, 1)
	if err == nil || !strings.Contains(err.Error(), "producer invalid request") {
		t.Fatalf("ExecuteStep error = %v, want producer invalid request", err)
	}
	if runCalls != 0 {
		t.Fatalf("run calls = %d, want 0 before producer request validates", runCalls)
	}
}

func TestExecuteProducerStepValidatesGenericRequestBeforeRun(t *testing.T) {
	var runCalls int
	producer := scriptedAdapter{name: "codex", runCalls: &runCalls}
	e := testEngine(t, producer)

	_, err := e.ExecuteStep(context.Background(), e.Store.NewRun(), Node{Step: config.Step{ID: "produce", Producer: "codex", MaxTurns: -1}}, 1)
	if err == nil || !strings.Contains(err.Error(), "max turns cannot be negative") {
		t.Fatalf("ExecuteStep error = %v, want max turns validation error", err)
	}
	if runCalls != 0 {
		t.Fatalf("run calls = %d, want 0 before generic request validates", runCalls)
	}
}

func TestExecuteReviewStepValidatesGenericRequestBeforeReview(t *testing.T) {
	var reviewCalls int
	reviewer := scriptedAdapter{name: "reviewer", reviewCalls: &reviewCalls}
	e := testEngine(t, reviewer)

	_, err := e.ExecuteStep(context.Background(), e.Store.NewRun(), Node{Step: config.Step{ID: "review", Reviewers: []string{"reviewer"}, MaxTurns: -1}}, 1)
	if err == nil || !strings.Contains(err.Error(), "max turns cannot be negative") {
		t.Fatalf("ExecuteStep error = %v, want max turns validation error", err)
	}
	if reviewCalls != 0 {
		t.Fatalf("review calls = %d, want 0 before generic request validates", reviewCalls)
	}
}

func TestExecuteProducerStepAppendsTrace(t *testing.T) {
	e := testEngine(t, scriptedAdapter{name: "codex", run: adapter.Result{Stdout: []byte("artifact")}})
	id := e.Store.NewRun()
	if _, err := e.ExecuteStep(context.Background(), id, Node{Step: config.Step{ID: "produce", Producer: "codex"}}, 1); err != nil {
		t.Fatalf("ExecuteStep error = %v", err)
	}
	data, err := os.ReadFile(filepath.Join(e.Store.Dir(id), "trace.jsonl"))
	if err != nil {
		t.Fatalf("read trace: %v", err)
	}
	if !strings.Contains(string(data), "step.produced") {
		t.Fatalf("trace = %s, want step.produced", data)
	}
}

func TestExecuteProducerStepPassesResolvedModelAndEffort(t *testing.T) {
	producer := &requestRecorderAdapter{name: "codex", run: adapter.Result{Stdout: []byte("artifact")}}
	e := testEngine(t, producer)
	e.AdapterDefaults = map[string]config.AdapterConfig{
		"codex": {Model: "gpt-5.5", Effort: "high"},
	}
	if _, err := e.ExecuteStep(context.Background(), e.Store.NewRun(), Node{Step: config.Step{ID: "produce", Producer: "codex"}}, 1); err != nil {
		t.Fatalf("ExecuteStep error = %v", err)
	}
	if producer.runReq.Model != "gpt-5.5" || producer.runReq.Effort != "high" {
		t.Fatalf("run request = %+v, want adapter defaults", producer.runReq)
	}
}

func TestExecuteReviewStepPassesResolvedModelAndEffort(t *testing.T) {
	reviewer := &requestRecorderAdapter{name: "claude", verdict: adapter.Verdict{Pass: true}}
	e := testEngine(t, reviewer)
	e.AdapterDefaults = map[string]config.AdapterConfig{
		"claude": {Model: "sonnet", Effort: "medium"},
	}
	if _, err := e.ExecuteStep(context.Background(), e.Store.NewRun(), Node{Step: config.Step{ID: "review", Reviewers: []string{"claude"}}}, 1); err != nil {
		t.Fatalf("ExecuteStep error = %v", err)
	}
	if reviewer.reviewReq.Model != "sonnet" || reviewer.reviewReq.Effort != "medium" {
		t.Fatalf("review request = %+v, want adapter defaults", reviewer.reviewReq)
	}
}

func TestStepOverrideWinsOverAdapterDefault(t *testing.T) {
	producer := &requestRecorderAdapter{name: "codex", run: adapter.Result{Stdout: []byte("artifact")}}
	e := testEngine(t, producer)
	e.AdapterDefaults = map[string]config.AdapterConfig{
		"codex": {Model: "gpt-5.5", Effort: "high"},
	}
	step := config.Step{ID: "produce", Producer: "codex", Model: "gpt-5.5-mini", Effort: "low"}
	if _, err := e.ExecuteStep(context.Background(), e.Store.NewRun(), Node{Step: step}, 1); err != nil {
		t.Fatalf("ExecuteStep error = %v", err)
	}
	if producer.runReq.Model != "gpt-5.5-mini" || producer.runReq.Effort != "low" {
		t.Fatalf("run request = %+v, want step override", producer.runReq)
	}
}

func TestExecuteStepRespectsTimeout(t *testing.T) {
	e := testEngine(t, scriptedAdapter{name: "codex", delay: 100 * time.Millisecond})
	if _, err := e.ExecuteStep(context.Background(), e.Store.NewRun(), Node{Step: config.Step{ID: "produce", Producer: "codex", Timeout: "10ms"}}, 1); err == nil {
		t.Fatalf("ExecuteStep timeout error = nil, want error")
	}
}

func TestExecuteStepRecordsPolicyDecisionRef(t *testing.T) {
	prov := &recordingPolicy{}
	e := testEngine(t, scriptedAdapter{name: "codex", run: adapter.Result{Stdout: []byte("artifact")}})
	e.Policy = prov
	res, err := e.ExecuteStep(context.Background(), e.Store.NewRun(), Node{Step: config.Step{ID: "produce", Producer: "codex"}}, 1)
	if err != nil {
		t.Fatalf("ExecuteStep error = %v", err)
	}
	if len(res.DecisionRefs) != 1 || res.DecisionRefs[0] == "" || prov.calls != 1 {
		t.Fatalf("decision refs = %v calls=%d, want one recorded ref", res.DecisionRefs, prov.calls)
	}
}

func TestDecideReceivesStampForProtectedStep(t *testing.T) {
	prov := &recordingPolicy{}
	e := testEngine(t, scriptedAdapter{name: "codex", run: adapter.Result{Stdout: []byte("artifact")}})
	e.Policy = prov
	e.CurrentStamp = "stamp-head-abc"
	_, err := e.ExecuteStep(context.Background(), e.Store.NewRun(), Node{Step: config.Step{ID: "protect", Producer: "codex", Approval: "protect:commit"}}, 1)
	if err != nil {
		t.Fatalf("ExecuteStep error = %v", err)
	}
	if prov.last.Stamp != "stamp-head-abc" {
		t.Fatalf("policy Action.Stamp = %q, want stamp-head-abc", prov.last.Stamp)
	}
	if prov.last.Kind != policy.ActionProtect || prov.last.ProtectedKind != policy.ProtectedCommit {
		t.Fatalf("policy action = %+v, want protect:commit", prov.last)
	}
}

func TestStepConsensusInheritsMinReviewers(t *testing.T) {
	// Step only sets mode; MinReviewers must inherit from DefaultConsensus (2).
	// A single reviewer must fail min_reviewers rather than silently pass.
	e := testEngine(t, scriptedAdapter{name: "a", verdict: adapter.Verdict{Pass: true}})
	e.DefaultConsensus = consensus.Policy{Mode: consensus.Majority, MinReviewers: 2, TieBreaker: consensus.Strict}
	step := config.Step{ID: "review", Reviewers: []string{"a"}, Consensus: config.Consensus{Mode: "majority"}}
	_, err := e.ExecuteStep(context.Background(), e.Store.NewRun(), Node{Step: step}, 1)
	if err == nil || !errors.Is(err, consensus.ErrMinReviewersUnsatisfied) {
		t.Fatalf("ExecuteStep error = %v, want ErrMinReviewersUnsatisfied", err)
	}
}

func TestExecuteReviewUsesConsensusPolicy(t *testing.T) {
	// 3 reviewers: 2 pass, 1 fail. Majority consensus passes; the old
	// unanimity implementation returned false here, so this is the key
	// mutation proof that step.Consensus is honored.
	e := testEngine(t,
		scriptedAdapter{name: "a", verdict: adapter.Verdict{Pass: true}},
		scriptedAdapter{name: "b", verdict: adapter.Verdict{Pass: true}},
		scriptedAdapter{name: "c", verdict: adapter.Verdict{Pass: false}},
	)
	step := config.Step{ID: "review", Reviewers: []string{"a", "b", "c"}, Consensus: config.Consensus{Mode: "majority"}}
	res, err := e.ExecuteStep(context.Background(), e.Store.NewRun(), Node{Step: step}, 1)
	if err != nil {
		t.Fatalf("ExecuteStep error = %v", err)
	}
	if !res.Consensus {
		t.Fatalf("Consensus = false, want true (majority of 2/3 pass)")
	}
	if !res.ConsensusOutcome.Pass {
		t.Fatalf("ConsensusOutcome.Pass = false, want true")
	}
}

func TestExecuteReviewUnanimousFailsOnDissent(t *testing.T) {
	e := testEngine(t,
		scriptedAdapter{name: "a", verdict: adapter.Verdict{Pass: true}},
		scriptedAdapter{name: "b", verdict: adapter.Verdict{Pass: true}},
		scriptedAdapter{name: "c", verdict: adapter.Verdict{Pass: false}},
	)
	step := config.Step{ID: "review", Reviewers: []string{"a", "b", "c"}, Consensus: config.Consensus{Mode: "unanimous"}}
	res, err := e.ExecuteStep(context.Background(), e.Store.NewRun(), Node{Step: step}, 1)
	if err != nil {
		t.Fatalf("ExecuteStep error = %v", err)
	}
	if res.Consensus {
		t.Fatalf("Consensus = true, want false (unanimous fails on dissent)")
	}
}

func TestExecuteReviewFirstPassPassesOnAny(t *testing.T) {
	e := testEngine(t,
		scriptedAdapter{name: "a", verdict: adapter.Verdict{Pass: true}},
		scriptedAdapter{name: "b", verdict: adapter.Verdict{Pass: false}},
	)
	step := config.Step{ID: "review", Reviewers: []string{"a", "b"}, Consensus: config.Consensus{Mode: "first-pass"}}
	res, err := e.ExecuteStep(context.Background(), e.Store.NewRun(), Node{Step: step}, 1)
	if err != nil {
		t.Fatalf("ExecuteStep error = %v", err)
	}
	if !res.Consensus {
		t.Fatalf("Consensus = false, want true (first-pass passes when any reviewer passes)")
	}
}

func TestExecuteReviewUsesDefaultConsensusWhenStepOmits(t *testing.T) {
	// Step omits Consensus entirely; engine DefaultConsensus (majority)
	// should apply. 2/3 pass -> majority passes.
	e := testEngine(t,
		scriptedAdapter{name: "a", verdict: adapter.Verdict{Pass: true}},
		scriptedAdapter{name: "b", verdict: adapter.Verdict{Pass: true}},
		scriptedAdapter{name: "c", verdict: adapter.Verdict{Pass: false}},
	)
	e.DefaultConsensus = consensus.Policy{Mode: consensus.Majority}
	step := config.Step{ID: "review", Reviewers: []string{"a", "b", "c"}}
	res, err := e.ExecuteStep(context.Background(), e.Store.NewRun(), Node{Step: step}, 1)
	if err != nil {
		t.Fatalf("ExecuteStep error = %v", err)
	}
	if !res.Consensus {
		t.Fatalf("Consensus = false, want true (default majority should apply)")
	}
	if res.ConsensusOutcome.Pass != res.Consensus {
		t.Fatalf("ConsensusOutcome.Pass = %v, want %v", res.ConsensusOutcome.Pass, res.Consensus)
	}
}

func testEngine(t *testing.T, adapters ...adapter.Adapter) Engine {
	t.Helper()
	reg, err := adapter.NewRegistry(adapters...)
	if err != nil {
		t.Fatalf("NewRegistry error = %v", err)
	}
	repo := t.TempDir()
	return Engine{Adapters: reg, Store: runstore.New(repo), Repo: repo, Clock: func() time.Time { return time.Date(2026, 7, 5, 0, 0, 0, 0, time.UTC) }, PromptBuilder: func(config.Step, int, []adapter.Verdict, string) (string, error) { return "prompt", nil }}
}

type scriptedAdapter struct {
	name        string
	run         adapter.Result
	verdict     adapter.Verdict
	delay       time.Duration
	runCalls    *int
	reviewCalls *int
}

func (s scriptedAdapter) Name() string             { return s.name }
func (s scriptedAdapter) Role() config.AdapterRole { return config.AdapterRoleOrchestrable }
func (s scriptedAdapter) Detect(context.Context) (adapter.Detection, error) {
	return adapter.Detection{Name: s.name, HeadlessCapable: true}, nil
}
func (s scriptedAdapter) Run(ctx context.Context, req adapter.Request) (adapter.Result, error) {
	if s.runCalls != nil {
		(*s.runCalls)++
	}
	if err := sleepContext(ctx, s.delay); err != nil {
		return adapter.Result{}, err
	}
	return s.run, nil
}
func (s scriptedAdapter) Review(ctx context.Context, req adapter.Request) (adapter.Verdict, error) {
	if s.reviewCalls != nil {
		(*s.reviewCalls)++
	}
	if err := sleepContext(ctx, s.delay); err != nil {
		return adapter.Verdict{}, err
	}
	return s.verdict, nil
}

type artifactOutAdapter struct {
	name        string
	stdout      []byte
	fileContent []byte
	artifactOut string
}

func (a *artifactOutAdapter) Name() string             { return a.name }
func (a *artifactOutAdapter) Role() config.AdapterRole { return config.AdapterRoleOrchestrable }
func (a *artifactOutAdapter) Detect(context.Context) (adapter.Detection, error) {
	return adapter.Detection{Name: a.name, HeadlessCapable: true}, nil
}
func (a *artifactOutAdapter) Run(_ context.Context, req adapter.Request) (adapter.Result, error) {
	a.artifactOut = req.ArtifactOut
	if err := os.WriteFile(req.ArtifactOut, a.fileContent, 0o644); err != nil {
		return adapter.Result{}, err
	}
	return adapter.Result{Stdout: a.stdout}, nil
}
func (a *artifactOutAdapter) Review(context.Context, adapter.Request) (adapter.Verdict, error) {
	return adapter.Verdict{}, nil
}

type promptRecorderAdapter struct {
	name    string
	verdict adapter.Verdict
	prompt  string
}

func (p *promptRecorderAdapter) Name() string             { return p.name }
func (p *promptRecorderAdapter) Role() config.AdapterRole { return config.AdapterRoleOrchestrable }
func (p *promptRecorderAdapter) Detect(context.Context) (adapter.Detection, error) {
	return adapter.Detection{Name: p.name, HeadlessCapable: true}, nil
}
func (p *promptRecorderAdapter) Run(context.Context, adapter.Request) (adapter.Result, error) {
	return adapter.Result{}, nil
}
func (p *promptRecorderAdapter) Review(_ context.Context, req adapter.Request) (adapter.Verdict, error) {
	p.prompt = req.Prompt
	return p.verdict, nil
}

type requestRecorderAdapter struct {
	name      string
	run       adapter.Result
	verdict   adapter.Verdict
	runReq    adapter.Request
	reviewReq adapter.Request
}

func (r *requestRecorderAdapter) Name() string             { return r.name }
func (r *requestRecorderAdapter) Role() config.AdapterRole { return config.AdapterRoleOrchestrable }
func (r *requestRecorderAdapter) Detect(context.Context) (adapter.Detection, error) {
	return adapter.Detection{Name: r.name, HeadlessCapable: true}, nil
}
func (r *requestRecorderAdapter) Run(_ context.Context, req adapter.Request) (adapter.Result, error) {
	r.runReq = req
	return r.run, nil
}
func (r *requestRecorderAdapter) Review(_ context.Context, req adapter.Request) (adapter.Verdict, error) {
	r.reviewReq = req
	return r.verdict, nil
}

type validatingReviewAdapter struct {
	name        string
	verdict     adapter.Verdict
	validateErr error
	runCalls    *int
	reviewCalls *int
}

func (v *validatingReviewAdapter) Name() string             { return v.name }
func (v *validatingReviewAdapter) Role() config.AdapterRole { return config.AdapterRoleOrchestrable }
func (v *validatingReviewAdapter) Detect(context.Context) (adapter.Detection, error) {
	return adapter.Detection{Name: v.name, HeadlessCapable: true}, nil
}
func (v *validatingReviewAdapter) ValidateRequest(adapter.Request) error {
	return v.validateErr
}
func (v *validatingReviewAdapter) Run(context.Context, adapter.Request) (adapter.Result, error) {
	if v.runCalls != nil {
		(*v.runCalls)++
	}
	return adapter.Result{}, nil
}
func (v *validatingReviewAdapter) Review(context.Context, adapter.Request) (adapter.Verdict, error) {
	if v.reviewCalls != nil {
		(*v.reviewCalls)++
	}
	return v.verdict, nil
}

type recordingPolicy struct {
	calls int
	last  policy.Action
}

func (r *recordingPolicy) Name() string { return "recording" }
func (r *recordingPolicy) Decide(ctx context.Context, action policy.Action) (policy.Decision, error) {
	r.calls++
	r.last = action
	return (policy.Decision{Allowed: true, Reason: "ok"}).EnsureRef(r.Name(), action), nil
}
func (r *recordingPolicy) Record(context.Context, policy.Event) error { return nil }

func sleepContext(ctx context.Context, d time.Duration) error {
	if d == 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
