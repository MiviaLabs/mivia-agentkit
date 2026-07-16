// Package cli implements the mivia-agent command surface.
// Plan: WS13. PRD: FR-4.1, FR-4.4, FR-7.4.
package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MiviaLabs/mivia-agentkit/internal/adapter"
	"github.com/MiviaLabs/mivia-agentkit/internal/config"
	"github.com/MiviaLabs/mivia-agentkit/internal/orchestrator"
)

func TestRunDryRunPrintsPlanWithoutInvoking(t *testing.T) {
	calls := 0
	withRuntimeAdapters(t, fakeCLIAdapter{name: "codex", headless: true, calls: &calls}, fakeCLIAdapter{name: "claude", headless: true, calls: &calls})
	repo := repoWithResearchLoop(t)
	cmd := newRunCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--repo", repo, "--workflow", "research", "--dry-run", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("run dry-run error = %v", err)
	}
	if calls != 0 {
		t.Fatalf("calls = %d, want zero adapter invocations", calls)
	}
	if !containsAll(out.String(), "research", "review", "adapters") {
		t.Fatalf("dry-run output = %s, want execution plan", out.String())
	}
}

func TestRunDryRunPrintsModelAndEffort(t *testing.T) {
	repo := repoWithResearchLoop(t)
	mustWrite(t, filepath.Join(repo, "mivia-agent.yaml"), "version: \"1\"\nadapters:\n  codex:\n    enabled: true\n    role: orchestrable\n    model: gpt-5.5\n    effort: minimal\n  claude:\n    enabled: true\n    role: orchestrable\n    model: sonnet\n    effort: high\nloops:\n  research:\n    bound: iterations\n    max_iterations: 2\n    steps:\n      - id: research\n        producer: codex\n        artifact: research.md\n      - id: review\n        reviewers: [codex, claude]\n        artifact: research.md\n        model: claude-opus\n        effort: low\n")
	cmd := newRunCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--repo", repo, "--workflow", "research", "--dry-run", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("run dry-run error = %v", err)
	}
	var rows []struct {
		Step    string `json:"step"`
		Runtime []struct {
			Adapter string `json:"adapter"`
			Model   string `json:"model"`
			Effort  string `json:"effort"`
		} `json:"runtime"`
	}
	if err := json.Unmarshal(out.Bytes(), &rows); err != nil {
		t.Fatalf("Unmarshal() error = %v, output=%s", err, out.String())
	}
	if len(rows) != 2 {
		t.Fatalf("rows len = %d, want 2", len(rows))
	}
	if got := rows[0].Runtime; len(got) != 1 || got[0].Adapter != "codex" || got[0].Model != "gpt-5.5" || got[0].Effort != "minimal" {
		t.Fatalf("producer runtime = %#v, want codex defaults", got)
	}
	if got := rows[1].Runtime; len(got) != 2 || got[0].Model != "claude-opus" || got[0].Effort != "low" || got[1].Model != "claude-opus" || got[1].Effort != "low" {
		t.Fatalf("review runtime = %#v, want step overrides for both reviewers", got)
	}
}

func TestRunExecutesResearchLoopFixture(t *testing.T) {
	calls := 0
	repo := repoWithResearchLoop(t)
	withRuntimeAdapters(t,
		fakeCLIAdapter{name: "codex", headless: true, calls: &calls, run: adapter.Result{Stdout: []byte("artifact")}, verdict: adapter.Verdict{Pass: true, Severity: "low", Notes: "ok"}},
		fakeCLIAdapter{name: "claude", headless: true, calls: &calls, verdict: adapter.Verdict{Pass: true, Severity: "low", Notes: "ok"}},
	)
	cmd := newRunCommand()
	cmd.SetArgs([]string{"--repo", repo, "--workflow", "research", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("run error = %v", err)
	}
	if calls == 0 {
		t.Fatalf("calls = 0, want fake adapters invoked")
	}
	if matches, _ := filepath.Glob(filepath.Join(repo, ".ai", "runs", "*", "trace.jsonl")); len(matches) != 1 {
		t.Fatalf("trace files = %v, want one trace", matches)
	}
}

func TestRunIteratesOnReviewFail(t *testing.T) {
	var prompts []string
	repo := repoWithResearchLoop(t)
	codex := sequenceAdapter{name: "codex", run: adapter.Result{Stdout: []byte("artifact")}, verdicts: []adapter.Verdict{{Pass: false, Severity: "high", Notes: "fix it"}, {Pass: true, Severity: "low", Notes: "ok"}}, prompts: &prompts}
	claude := sequenceAdapter{name: "claude", verdicts: []adapter.Verdict{{Pass: false, Severity: "high", Notes: "fix it"}, {Pass: true, Severity: "low", Notes: "ok"}}, prompts: &prompts}
	withRuntimeAdapters(t, &codex, &claude)
	cmd := newRunCommand()
	cmd.SetArgs([]string{"--repo", repo, "--workflow", "research", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("run error = %v", err)
	}
	if !anyContains(prompts, "fix it") {
		t.Fatalf("prompts = %#v, want prior reviewer notes fed back", prompts)
	}
}

func TestRunReviewStepRequestsJSONVerdict(t *testing.T) {
	var prompts []string
	repo := repoWithResearchLoop(t)
	withRuntimeAdapters(t,
		fakeCLIAdapter{name: "codex", headless: true, run: adapter.Result{Stdout: []byte("artifact")}, verdict: adapter.Verdict{Pass: true, Severity: "low", Notes: "ok"}, prompts: &prompts},
		fakeCLIAdapter{name: "claude", headless: true, verdict: adapter.Verdict{Pass: true, Severity: "low", Notes: "ok"}, prompts: &prompts},
	)
	cmd := newRunCommand()
	cmd.SetArgs([]string{"--repo", repo, "--workflow", "research", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("run error = %v", err)
	}
	if !anyContains(prompts, "Return JSON only") {
		t.Fatalf("prompts = %#v, want reviewer prompt to request JSON verdict", prompts)
	}
}

func TestRunReviewStepUsesConcreteArtifactPath(t *testing.T) {
	var prompts []string
	repo := repoWithResearchLoop(t)
	withRuntimeAdapters(t,
		fakeCLIAdapter{name: "codex", headless: true, run: adapter.Result{Stdout: []byte("artifact")}, verdict: adapter.Verdict{Pass: true, Severity: "low", Notes: "ok"}, prompts: &prompts},
		fakeCLIAdapter{name: "claude", headless: true, verdict: adapter.Verdict{Pass: true, Severity: "low", Notes: "ok"}, prompts: &prompts},
	)
	cmd := newRunCommand()
	cmd.SetArgs([]string{"--repo", repo, "--workflow", "research", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("run error = %v", err)
	}
	if !anyContains(prompts, string(filepath.Separator)+".ai"+string(filepath.Separator)+"runs"+string(filepath.Separator)) ||
		!anyContains(prompts, string(filepath.Separator)+"iter-001"+string(filepath.Separator)+"research.md") {
		t.Fatalf("prompts = %#v, want concrete per-iteration artifact path", prompts)
	}
}

func TestRunPassesTemplateVariablesToPrompt(t *testing.T) {
	var prompts []string
	repo := repoWithResearchLoop(t)
	withRuntimeAdapters(t,
		fakeCLIAdapter{name: "codex", headless: true, run: adapter.Result{Stdout: []byte("artifact")}, verdict: adapter.Verdict{Pass: true, Severity: "low", Notes: "ok"}, prompts: &prompts},
		fakeCLIAdapter{name: "claude", headless: true, verdict: adapter.Verdict{Pass: true, Severity: "low", Notes: "ok"}, prompts: &prompts},
	)
	cmd := newRunCommand()
	cmd.SetArgs([]string{"--repo", repo, "--workflow", "research", "--var", "objective=Audit auth timeout behavior", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("run error = %v", err)
	}
	if len(prompts) == 0 || !strings.Contains(prompts[0], "Objective: Audit auth timeout behavior") ||
		!strings.Contains(prompts[0], "Produce artifact research.md") {
		t.Fatalf("prompts = %#v, want objective variable passed to producer prompt", prompts)
	}
}

func TestRunRejectsMalformedTemplateVariable(t *testing.T) {
	tests := []struct {
		name string
		arg  string
	}{
		{name: "missing_equals", arg: "objective"},
		{name: "invalid_template_key", arg: "bad key=value"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := repoWithResearchLoop(t)
			cmd := newRunCommand()
			cmd.SetArgs([]string{"--repo", repo, "--workflow", "research", "--var", tt.arg})
			if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "invalid --var") {
				t.Fatalf("run error = %v, want malformed --var rejection", err)
			}
		})
	}
}

func TestRunPassesManifestAdapterDefaultsToRuntime(t *testing.T) {
	var runReqs []adapter.Request
	var reviewReqs []adapter.Request
	repo := repoWithResearchLoop(t)
	mustWrite(t, filepath.Join(repo, "mivia-agent.yaml"), "version: \"1\"\nadapters:\n  codex:\n    enabled: true\n    role: orchestrable\n    model: gpt-5.5\n    effort: minimal\n  claude:\n    enabled: true\n    role: orchestrable\n    model: sonnet\n    effort: max\n")
	withRuntimeAdapters(t,
		fakeCLIAdapter{name: "codex", headless: true, run: adapter.Result{Stdout: []byte("artifact")}, verdict: adapter.Verdict{Pass: true, Severity: "low", Notes: "ok"}, runReqs: &runReqs, reviews: &reviewReqs},
		fakeCLIAdapter{name: "claude", headless: true, verdict: adapter.Verdict{Pass: true, Severity: "low", Notes: "ok"}, reviews: &reviewReqs},
	)
	cmd := newRunCommand()
	cmd.SetArgs([]string{"--repo", repo, "--workflow", "research", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("run error = %v", err)
	}
	if len(runReqs) == 0 || runReqs[0].Model != "gpt-5.5" || runReqs[0].Effort != "minimal" {
		t.Fatalf("producer requests = %#v, want manifest defaults", runReqs)
	}
	if len(reviewReqs) < 2 {
		t.Fatalf("review requests = %#v, want codex and claude reviews", reviewReqs)
	}
	var foundClaude bool
	for _, req := range reviewReqs {
		if req.Model == "sonnet" && req.Effort == "max" {
			foundClaude = true
			break
		}
	}
	if !foundClaude {
		t.Fatalf("review requests = %#v, want claude manifest defaults", reviewReqs)
	}
}

func TestRunWithCrushUsesRealSubprocessBoundary(t *testing.T) {
	repo := t.TempDir()
	stubDir := t.TempDir()
	writeCrushStub(t, stubDir)
	t.Setenv("PATH", stubDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("CRUSH_STUB_DIR", stubDir)
	withRuntimeAdapters(t, adapter.Crush{})
	mustWrite(t, filepath.Join(repo, "mivia-agent.yaml"), "version: \"1\"\nadapters:\n  crush:\n    enabled: true\n    role: orchestrable\n    model: ollama/qwen3:14b\nrouting:\n  consensus:\n    mode: unanimous\n    min_reviewers: 1\nloops:\n  build:\n    bound: iterations\n    max_iterations: 1\n    steps:\n      - id: build\n        producer: crush\n        artifact: build.md\n      - id: review\n        reviewers: [crush]\n        artifact: build.md\n    exit_when: review-pass\n    on_exhausted: fail\n")
	mustWrite(t, filepath.Join(repo, ".ai/workflows/build.yaml"), "bound: iterations\nmax_iterations: 1\nsteps:\n- id: build\n  producer: crush\n  artifact: build.md\n- id: review\n  reviewers: [crush]\n  artifact: build.md\nexit_when: review-pass\non_exhausted: fail\n")

	cmd := newRunCommand()
	cmd.SetArgs([]string{"--repo", repo, "--workflow", "build", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("run error = %v", err)
	}

	argsData, err := os.ReadFile(filepath.Join(stubDir, "run.args"))
	if err != nil {
		t.Fatalf("ReadFile(run.args) error = %v", err)
	}
	args := string(argsData)
	if !strings.Contains(args, "run\n--quiet\n--cwd\n"+repo+"\n--model\nollama/qwen3:14b\n") {
		t.Fatalf("run args = %q, want crush run with quiet cwd and model", args)
	}
	stdinData, err := os.ReadFile(filepath.Join(stubDir, "run.stdin"))
	if err != nil {
		t.Fatalf("ReadFile(run.stdin) error = %v", err)
	}
	if !strings.Contains(string(stdinData), "Produce artifact build.md") {
		t.Fatalf("stdin = %q, want generated producer prompt through real process stdin", stdinData)
	}
	if matches, _ := filepath.Glob(filepath.Join(repo, ".ai", "runs", "*", "build", "iter-001", "build.md")); len(matches) != 1 {
		t.Fatalf("artifact files = %v, want one build artifact from crush stdout", matches)
	}
}

func TestRunPropagatesCommandContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	repo := repoWithResearchLoop(t)
	withRuntimeAdapters(t, contextAwareAdapter{name: "codex"}, contextAwareAdapter{name: "claude"})
	cmd := newRunCommand()
	cmd.SetContext(ctx)
	cmd.SetArgs([]string{"--repo", repo, "--workflow", "research"})
	if err := cmd.Execute(); err == nil {
		t.Fatalf("run error = nil, want canceled context error")
	}
}

func TestRunFailsOnExhaustion(t *testing.T) {
	repo := repoWithResearchLoop(t)
	withRuntimeAdapters(t,
		fakeCLIAdapter{name: "codex", headless: true, run: adapter.Result{Stdout: []byte("artifact")}, verdict: adapter.Verdict{Pass: false, Severity: "high", Notes: "no"}},
		fakeCLIAdapter{name: "claude", headless: true, verdict: adapter.Verdict{Pass: false, Severity: "high", Notes: "no"}},
	)
	cmd := newRunCommand()
	cmd.SetArgs([]string{"--repo", repo, "--workflow", "research", "--max-iterations", "1"})
	if err := cmd.Execute(); err == nil {
		t.Fatalf("run error = nil, want exhaustion failure")
	}
}

func TestRunRejectsUnknownWorkflow(t *testing.T) {
	cmd := newRunCommand()
	cmd.SetArgs([]string{"--repo", t.TempDir(), "--workflow", "missing", "--dry-run"})
	if err := cmd.Execute(); err == nil {
		t.Fatalf("run error = nil, want unknown workflow rejection")
	}
}

func TestRunRejectsBudgetBoundLoopInMVP(t *testing.T) {
	repo := t.TempDir()
	mustWrite(t, filepath.Join(repo, ".ai/workflows/budget.yaml"), "bound: budget\nmax_iterations: 1\nsteps:\n- id: p\n  producer: codex\n")
	cmd := newRunCommand()
	cmd.SetArgs([]string{"--repo", repo, "--workflow", "budget", "--dry-run"})
	if err := cmd.Execute(); err == nil {
		t.Fatalf("run error = nil, want budget bound rejection")
	}
}

func TestRunArtifactContainsNoRawPromptsOrOutputs(t *testing.T) {
	repo := repoWithResearchLoop(t)
	withRuntimeAdapters(t,
		fakeCLIAdapter{name: "codex", headless: true, run: adapter.Result{Stdout: []byte("safe-artifact")}, verdict: adapter.Verdict{Pass: true, Severity: "low", Notes: "ok"}},
		fakeCLIAdapter{name: "claude", headless: true, verdict: adapter.Verdict{Pass: true, Severity: "low", Notes: "ok"}},
	)
	cmd := newRunCommand()
	cmd.SetArgs([]string{"--repo", repo, "--workflow", "research", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("run error = %v", err)
	}
	orchestrator.AssertNoLeaks(t, filepath.Join(repo, ".ai", "runs"))
}

func TestRunStrictFailsOnFirstPassConsensusForProtectBound(t *testing.T) {
	repo := t.TempDir()
	mustWrite(t, filepath.Join(repo, "mivia-agent.yaml"), "version: \"1\"\nprofile: strict\nadapters:\n  codex: {enabled: true, role: orchestrable}\nloops:\n  protected:\n    bound: iterations\n    max_iterations: 1\n    exit_when: protected_action\n    steps:\n      - id: protect\n        producer: codex\n        approval: protect:commit\n        consensus: {mode: first-pass}\n")
	cmd := newRunCommand()
	cmd.SetArgs([]string{"--repo", repo, "--workflow", "protected", "--dry-run", "--strict"})
	if err := cmd.Execute(); err == nil {
		t.Fatalf("run error = nil, want strict first-pass protect-bound rejection")
	}
}

func TestRunStrictFailsWarnOutcome(t *testing.T) {
	repo := t.TempDir()
	mustWrite(t, filepath.Join(repo, ".ai/workflows/research.yaml"), "bound: iterations\nmax_iterations: 1\nsteps:\n- id: research\n  producer: codex\n  artifact: research.md\n- id: review\n  reviewers: [codex, claude]\n  artifact: research.md\n  on_fail: iterate\nexit_when: review-pass\non_exhausted: warn\n")
	withRuntimeAdapters(t,
		fakeCLIAdapter{name: "codex", headless: true, run: adapter.Result{Stdout: []byte("artifact")}, verdict: adapter.Verdict{Pass: false, Severity: "high", Notes: "no"}},
		fakeCLIAdapter{name: "claude", headless: true, verdict: adapter.Verdict{Pass: false, Severity: "high", Notes: "no"}},
	)

	// Without --strict, warn outcome is success (exit 0).
	cmd := newRunCommand()
	cmd.SetArgs([]string{"--repo", repo, "--workflow", "research", "--max-iterations", "1"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("run without strict error = %v, want nil for warn outcome", err)
	}

	// With --strict, warn outcome must fail.
	strictCmd := newRunCommand()
	strictCmd.SetArgs([]string{"--repo", repo, "--workflow", "research", "--max-iterations", "1", "--strict"})
	if err := strictCmd.Execute(); err == nil {
		t.Fatalf("run --strict error = nil, want fail on warn outcome")
	}
}

func repoWithResearchLoop(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	mustWrite(t, filepath.Join(repo, ".ai/workflows/research.yaml"), "bound: iterations\nmax_iterations: 2\nsteps:\n- id: research\n  producer: codex\n  artifact: research.md\n- id: review\n  reviewers: [codex, claude]\n  artifact: research.md\n  on_fail: iterate\nexit_when: review-pass\non_exhausted: fail\n")
	return repo
}

type sequenceAdapter struct {
	name     string
	run      adapter.Result
	verdicts []adapter.Verdict
	reviews  int
	prompts  *[]string
}

func (s *sequenceAdapter) Name() string             { return s.name }
func (s *sequenceAdapter) Role() config.AdapterRole { return config.AdapterRoleOrchestrable }
func (s *sequenceAdapter) Detect(context.Context) (adapter.Detection, error) {
	return adapter.Detection{Name: s.name, HeadlessCapable: true}, nil
}
func (s *sequenceAdapter) Run(_ context.Context, req adapter.Request) (adapter.Result, error) {
	fakeAdapterMu.Lock()
	defer fakeAdapterMu.Unlock()
	if s.prompts != nil {
		*s.prompts = append(*s.prompts, req.Prompt)
	}
	return s.run, nil
}
func (s *sequenceAdapter) Review(_ context.Context, req adapter.Request) (adapter.Verdict, error) {
	fakeAdapterMu.Lock()
	defer fakeAdapterMu.Unlock()
	if s.prompts != nil {
		*s.prompts = append(*s.prompts, req.Prompt)
	}
	v := s.verdicts[min(s.reviews, len(s.verdicts)-1)]
	s.reviews++
	return v, nil
}

func mustWrite(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// writeCrushStub builds a real, native crush executable rather than a
// POSIX shell script: Go's exec package does not interpret shebangs on
// Windows, so a "#!/bin/sh" script fails to launch there. Compiling a
// tiny Go program (the same approach test/integration's buildStubCLI
// uses) produces a stub that behaves identically on every platform.
func writeCrushStub(t *testing.T, dir string) {
	t.Helper()
	srcDir := filepath.Join(dir, "crush-src")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", srcDir, err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "go.mod"), []byte("module crushstub\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	program := `package main

import (
	"fmt"
	"io"
	"os"
	"strings"
)

func main() {
	args := os.Args[1:]
	if len(args) == 1 && args[0] == "--version" {
		fmt.Print("crush version v0.79.1\n")
		return
	}
	if len(args) == 2 && args[0] == "run" && args[1] == "--help" {
		fmt.Print("Run a single prompt in non-interactive mode and exit.\n")
		fmt.Print("The prompt can be provided as arguments or piped from stdin.\n")
		fmt.Print("USAGE\n  crush run [prompt...] [--flags]\n")
		return
	}
	if len(args) > 0 && args[0] == "run" {
		stubDir := os.Getenv("CRUSH_STUB_DIR")
		if stubDir == "" {
			fmt.Fprintln(os.Stderr, "CRUSH_STUB_DIR is required")
			os.Exit(1)
		}
		argsFile, err := os.OpenFile(stubDir+string(os.PathSeparator)+"run.args", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Fprintln(argsFile, strings.Join(args, "\n"))
		argsFile.Close()

		stdinData, _ := io.ReadAll(os.Stdin)
		stdinFile, err := os.OpenFile(stubDir+string(os.PathSeparator)+"run.stdin", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Fprintf(stdinFile, "%s\n---CALL---\n", string(stdinData))
		stdinFile.Close()

		if strings.Contains(string(stdinData), "Return JSON only") {
			fmt.Print("{\"pass\":true,\"severity\":\"low\",\"notes\":\"ok\"}\n")
		} else {
			fmt.Print("crush artifact\n")
		}
		return
	}
	os.Exit(64)
}
`
	if err := os.WriteFile(filepath.Join(srcDir, "main.go"), []byte(program), 0o644); err != nil {
		t.Fatalf("write crush stub source: %v", err)
	}
	bin := filepath.Join(dir, "crush"+binarySuffix())
	cmd := exec.Command("go", "build", "-buildvcs=false", "-o", bin, ".")
	cmd.Dir = srcDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build crush stub: %v\n%s", err, out)
	}
}

type contextAwareAdapter struct {
	name string
}

func (c contextAwareAdapter) Name() string             { return c.name }
func (c contextAwareAdapter) Role() config.AdapterRole { return config.AdapterRoleOrchestrable }
func (c contextAwareAdapter) Detect(context.Context) (adapter.Detection, error) {
	return adapter.Detection{Name: c.name, HeadlessCapable: true}, nil
}
func (c contextAwareAdapter) Run(ctx context.Context, req adapter.Request) (adapter.Result, error) {
	if err := ctx.Err(); err != nil {
		return adapter.Result{}, err
	}
	return adapter.Result{Stdout: []byte("artifact")}, nil
}
func (c contextAwareAdapter) Review(ctx context.Context, req adapter.Request) (adapter.Verdict, error) {
	if err := ctx.Err(); err != nil {
		return adapter.Verdict{}, err
	}
	return adapter.Verdict{Pass: true, Severity: "low", Notes: "ok"}, nil
}
