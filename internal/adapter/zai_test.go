// Package adapter tests the ZAI CLI adapter.
// Plan: WS-C. PRD: FR-3.1, FR-3.2, FR-7.4.
package adapter

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestZaiName(t *testing.T) {
	if got := (Zai{}).Name(); got != "zai" {
		t.Fatalf("Name() = %q, want %q", got, "zai")
	}
}

func TestZaiRole(t *testing.T) {
	if got := (Zai{}).Role(); string(got) != "orchestrable" {
		t.Fatalf("Role() = %q, want orchestrable", got)
	}
}

func TestZaiDetectHeadlessCapability(t *testing.T) {
	r := &FakeRunner{Scripts: map[string]FakeResponse{"zai": {Result: RunResult{Stdout: []byte("0.3.5")}}}}
	d, err := (Zai{Runner: r}).Detect(context.Background())
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if !d.HeadlessCapable || d.Version != "0.3.5" {
		t.Fatalf("Detection = %#v, want headless version 0.3.5", d)
	}
}

// TestZaiDetectRealBinary exercises the production default-runner path
// (OSRunner{}) against the real zai binary. It is the real-subprocess
// integration closure required by AGENTS.md Testing Standards for shipped
// adapters; the FakeRunner tests above are not sufficient on their own.
func TestZaiDetectRealBinary(t *testing.T) {
	if _, err := exec.LookPath("zai"); err != nil {
		t.Skip("zai binary not on PATH")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	d, err := (Zai{}).Detect(ctx)
	if err != nil || d.Name != "zai" || d.HeadlessCapable != true || d.Version == "" {
		t.Fatalf("Detect = %#v, want headless zai with non-empty version", d)
	}
}

func TestZaiValidateRequestAcceptsValid(t *testing.T) {
	if err := (Zai{}).ValidateRequest(Request{Prompt: "x", Approval: "never"}); err != nil {
		t.Fatalf("ValidateRequest() error = %v, want nil for valid request", err)
	}
}

func TestZaiValidateRequestRejectsEmptyApproval(t *testing.T) {
	err := (Zai{}).ValidateRequest(Request{Prompt: "x"})
	if err == nil || !strings.Contains(err.Error(), "approval is required") {
		t.Fatalf("ValidateRequest() error = %v, want approval is required", err)
	}
}

func TestZaiValidateRequestRejectsInvalidApproval(t *testing.T) {
	err := (Zai{}).ValidateRequest(Request{Prompt: "x", Approval: "plan"})
	if err == nil || !strings.Contains(err.Error(), "zai unsupported approval") {
		t.Fatalf("ValidateRequest() error = %v, want zai unsupported approval", err)
	}
}

func TestZaiValidateRequestRejectsEffort(t *testing.T) {
	err := (Zai{}).ValidateRequest(Request{Prompt: "x", Approval: "never", Effort: "high"})
	if err == nil || !strings.Contains(err.Error(), "zai unsupported effort") {
		t.Fatalf("ValidateRequest() error = %v, want zai unsupported effort", err)
	}
}

func TestZaiValidateRequestRejectsUnknownParams(t *testing.T) {
	err := (Zai{}).ValidateRequest(Request{
		Prompt:   "x",
		Approval: "never",
		Params:   map[string]string{"provider": "zai"},
	})
	if err == nil || !strings.Contains(err.Error(), "zai unsupported params") {
		t.Fatalf("ValidateRequest() error = %v, want zai unsupported params", err)
	}
}

func TestZaiValidateRequestAcceptsModelParam(t *testing.T) {
	if err := (Zai{}).ValidateRequest(Request{
		Prompt:   "x",
		Approval: "never",
		Params:   map[string]string{"model": "glm-5-turbo"},
	}); err != nil {
		t.Fatalf("ValidateRequest() error = %v, want nil for model param", err)
	}
}

func TestZaiRunEnforcesHeadlessMode(t *testing.T) {
	r := zaiRunner([]byte(`{"role":"assistant","content":"ok"}`), nil)
	_, err := (Zai{Runner: r}).Run(context.Background(), Request{Prompt: "x", Approval: "never"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	args := strings.Join(r.Calls[0].Args, " ")
	if !strings.Contains(args, "-p x") || !strings.Contains(args, "--no-color") || !strings.Contains(args, "-m glm-5.2") {
		t.Fatalf("args = %q, want headless -p/--no-color/-m glm-5.2 flags", args)
	}
}

func TestZaiRunMapsExitCode(t *testing.T) {
	r := zaiRunner(nil, nil)
	r.Scripts["zai"] = FakeResponse{Result: RunResult{ExitCode: 5}}
	got, _ := (Zai{Runner: r}).Run(context.Background(), Request{Prompt: "x", Approval: "never"})
	if got.ExitCode != 5 {
		t.Fatalf("ExitCode = %d, want 5", got.ExitCode)
	}
}

func TestZaiRunPassesModelFlag(t *testing.T) {
	r := zaiRunner([]byte(`{"role":"assistant","content":"ok"}`), nil)
	_, err := (Zai{Runner: r}).Run(context.Background(), Request{Prompt: "x", Approval: "never", Model: "glm-5-turbo"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	args := strings.Join(r.Calls[0].Args, " ")
	if !strings.Contains(args, "-m glm-5-turbo") {
		t.Fatalf("args = %q, want -m glm-5-turbo", args)
	}
}

func TestZaiRunFallsBackToDefaultModel(t *testing.T) {
	r := zaiRunner([]byte(`{"role":"assistant","content":"ok"}`), nil)
	_, err := (Zai{Runner: r}).Run(context.Background(), Request{Prompt: "x", Approval: "never"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	args := strings.Join(r.Calls[0].Args, " ")
	if !strings.Contains(args, "-m glm-5.2") {
		t.Fatalf("args = %q, want default -m glm-5.2", args)
	}
}

func TestZaiRunUsesModelParamWhenModelEmpty(t *testing.T) {
	r := zaiRunner([]byte(`{"role":"assistant","content":"ok"}`), nil)
	_, err := (Zai{Runner: r}).Run(context.Background(), Request{
		Prompt:   "x",
		Approval: "never",
		Params:   map[string]string{"model": "glm-5-turbo"},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	args := strings.Join(r.Calls[0].Args, " ")
	if !strings.Contains(args, "-m glm-5-turbo") {
		t.Fatalf("args = %q, want -m glm-5-turbo from model param", args)
	}
}

func TestZaiRunPassesWorkdir(t *testing.T) {
	r := zaiRunner([]byte(`{"role":"assistant","content":"ok"}`), nil)
	_, err := (Zai{Runner: r}).Run(context.Background(), Request{Prompt: "x", Approval: "never", Workdir: "/tmp/repo"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	args := strings.Join(r.Calls[0].Args, " ")
	if !strings.Contains(args, "-d /tmp/repo") {
		t.Fatalf("args = %q, want -d /tmp/repo", args)
	}
}

func TestZaiRunPassesMaxTurnsAsToolRounds(t *testing.T) {
	r := zaiRunner([]byte(`{"role":"assistant","content":"ok"}`), nil)
	_, err := (Zai{Runner: r}).Run(context.Background(), Request{Prompt: "x", Approval: "never", MaxTurns: 8})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	args := strings.Join(r.Calls[0].Args, " ")
	if !strings.Contains(args, "--max-tool-rounds 8") {
		t.Fatalf("args = %q, want --max-tool-rounds 8", args)
	}
}

// TestZaiRunPassesPromptIntact asserts the prompt is forwarded as the exact
// value immediately following -p (not dropped, truncated, or shell-escaped),
// guarding against the regression class where a joined-string Contains check
// would still pass even if the prompt value itself were mangled.
func TestZaiRunPassesPromptIntact(t *testing.T) {
	r := zaiRunner([]byte(`{"role":"assistant","content":"ok"}`), nil)
	prompt := "multi word prompt with -m and --flag-like substrings"
	_, err := (Zai{Runner: r}).Run(context.Background(), Request{Prompt: prompt, Approval: "never"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	args := r.Calls[0].Args
	idx := -1
	for i, a := range args {
		if a == "-p" {
			idx = i
			break
		}
	}
	if idx < 0 || idx+1 >= len(args) {
		t.Fatalf("args = %#v, want -p immediately followed by the prompt value", args)
	}
	if args[idx+1] != prompt {
		t.Fatalf("-p argument = %q, want exact prompt %q", args[idx+1], prompt)
	}
}

func TestZaiRunRejectsUnsupportedEffort(t *testing.T) {
	r := zaiRunner([]byte(`{"role":"assistant","content":"ok"}`), nil)
	_, err := (Zai{Runner: r}).Run(context.Background(), Request{Prompt: "x", Approval: "never", Effort: "high"})
	if err == nil || !strings.Contains(err.Error(), "zai unsupported effort") {
		t.Fatalf("Run() error = %v, want zai unsupported effort", err)
	}
	if len(r.Calls) != 0 {
		t.Fatalf("runner calls = %d, want 0 before unsupported effort reaches CLI", len(r.Calls))
	}
}

func TestZaiRunRejectsUnsupportedParams(t *testing.T) {
	r := zaiRunner([]byte(`{"role":"assistant","content":"ok"}`), nil)
	_, err := (Zai{Runner: r}).Run(context.Background(), Request{Prompt: "x", Approval: "never", Params: map[string]string{"provider": "zai"}})
	if err == nil || !strings.Contains(err.Error(), "zai unsupported params") {
		t.Fatalf("Run() error = %v, want zai unsupported params", err)
	}
	if len(r.Calls) != 0 {
		t.Fatalf("runner calls = %d, want 0 before unsupported params reach CLI", len(r.Calls))
	}
}

func TestZaiRunRespectsTimeout(t *testing.T) {
	r := zaiRunner(nil, ErrCommandTimeout)
	_, err := (Zai{Runner: r}).Run(context.Background(), Request{Prompt: "x", Approval: "never", Timeout: time.Millisecond})
	if err == nil {
		t.Fatalf("Run() error = nil, want timeout")
	}
}

func TestZaiRunScrubsSecretsFromStdout(t *testing.T) {
	token := "Bearer " + strings.Repeat("a", 16)
	got, err := (Zai{Runner: zaiRunner([]byte(token), nil)}).Run(context.Background(), Request{Prompt: "x", Approval: "never"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if bytes.Contains(got.Stdout, []byte("aaaaaaaaaaaaaaaa")) || !bytes.Contains(got.Stdout, []byte("<redacted:bearer>")) {
		t.Fatalf("Stdout = %q, want redacted bearer token", got.Stdout)
	}
}

func TestZaiReviewParsesVerdict(t *testing.T) {
	out := []byte(`{"role":"assistant","content":"{\"pass\":true,\"severity\":\"medium\",\"notes\":\"ok\"}"}`)
	v, err := (Zai{Runner: zaiRunner(out, nil)}).Review(context.Background(), Request{Prompt: "x", Approval: "never"})
	if err != nil {
		t.Fatalf("Review() error = %v", err)
	}
	if !v.Pass || v.Severity != "medium" {
		t.Fatalf("Verdict = %#v, want pass medium", v)
	}
}

func TestZaiReviewFailsClosedOnUnparseable(t *testing.T) {
	v, err := (Zai{Runner: zaiRunner([]byte("not json"), nil)}).Review(context.Background(), Request{Prompt: "x", Approval: "never"})
	if err != nil {
		t.Fatalf("Review() error = %v", err)
	}
	if v.Pass || v.Severity != "error" {
		t.Fatalf("Verdict = %#v, want fail-closed error", v)
	}
}

// zaiRunner returns a FakeRunner scripted to respond to the "zai" command with
// the given stdout and error, matching the helper shape used by codexRunner
// and claudeRunner.
func zaiRunner(stdout []byte, err error) *FakeRunner {
	return &FakeRunner{Scripts: map[string]FakeResponse{"zai": {Result: RunResult{Stdout: stdout}, Err: err}}}
}
