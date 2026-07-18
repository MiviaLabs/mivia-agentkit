// Package adapter tests the Kimi Code CLI adapter.
// Plan: WS15. PRD: FR-3.1, FR-3.2.
package adapter

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestKimiDetectHeadlessCapability(t *testing.T) {
	r := kimiRunner([]byte("0.26.0"), nil)
	d, err := (Kimi{Runner: r}).Detect(context.Background())
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if !d.HeadlessCapable || d.Version != "0.26.0" {
		t.Fatalf("Detection = %#v", d)
	}
}

func TestKimiRunPassesPromptFlag(t *testing.T) {
	r := kimiRunner([]byte("ok"), nil)
	_, err := (Kimi{Runner: r}).Run(context.Background(), Request{Prompt: "hello", Approval: "never"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	args := strings.Join(r.Calls[0].Args, " ")
	if !strings.Contains(args, "-p hello") {
		t.Fatalf("args = %q, want -p hello", args)
	}
}

func TestKimiRunPassesModelFlag(t *testing.T) {
	r := kimiRunner([]byte("ok"), nil)
	_, err := (Kimi{Runner: r}).Run(context.Background(), Request{Prompt: "x", Approval: "never", Model: "kimi-k2"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	args := strings.Join(r.Calls[0].Args, " ")
	if !strings.Contains(args, "-m kimi-k2") {
		t.Fatalf("args = %q, want model flag", args)
	}
}

func TestKimiRejectsNonNeverApproval(t *testing.T) {
	err := (Kimi{}).ValidateRequest(Request{Prompt: "x", Approval: "plan"})
	if err == nil || !strings.Contains(err.Error(), "unsupported approval") {
		t.Fatalf("ValidateRequest error = %v", err)
	}
}

func TestKimiRejectsEffort(t *testing.T) {
	err := (Kimi{}).ValidateRequest(Request{Prompt: "x", Approval: "never", Effort: "high"})
	if err == nil || !strings.Contains(err.Error(), "unsupported effort") {
		t.Fatalf("ValidateRequest error = %v", err)
	}
}

func TestKimiRunFailsClosedOnAuthError(t *testing.T) {
	r := kimiRunner([]byte("Authentication Failed"), nil)
	_, err := (Kimi{Runner: r}).Run(context.Background(), Request{Prompt: "x", Approval: "never"})
	if err == nil || !strings.Contains(err.Error(), "provider failure") {
		t.Fatalf("Run() error = %v, want provider failure", err)
	}
}

func TestKimiRunIgnoresAuthStringInsideLongTranscript(t *testing.T) {
	// Agent tool output may quote docs/errors containing "Authentication Failed".
	var b strings.Builder
	b.WriteString("reading internal/adapter/zai.go mentions Authentication Failed in comments\n")
	for i := 0; i < 40; i++ {
		b.WriteString("tool result line about repo files and tests\n")
	}
	b.WriteString(`{"schema":"mivia-agent-campaign-evidence/v1","campaign_run":"r","cycle":1,"baseline_head":"h","finding_fingerprint":"fp1","disposition":"confirmed","changed_path_ids":["p1"],"verifier_ref":"go-test","progress":1}`)
	r := kimiRunner([]byte(b.String()), nil)
	_, err := (Kimi{Runner: r}).Run(context.Background(), Request{Prompt: "x", Approval: "never"})
	if err != nil {
		t.Fatalf("Run() error = %v, want nil for long transcript", err)
	}
}

// TestKimiDetectRealBinary exercises the production default-runner path
// (OSRunner{}) against the real kimi binary. It is the real-subprocess
// integration closure required by AGENTS.md Testing Standards for shipped
// adapters; the FakeRunner tests above are not sufficient on their own.
func TestKimiDetectRealBinary(t *testing.T) {
	if _, err := exec.LookPath("kimi"); err != nil {
		t.Skip("kimi binary not on PATH")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	d, err := (Kimi{}).Detect(ctx)
	if err != nil || d.Name != "kimi" || d.HeadlessCapable != true || d.Version == "" {
		t.Fatalf("Detect = %#v, want headless kimi with non-empty version", d)
	}
}

func kimiRunner(stdout []byte, err error) *FakeRunner {
	return &FakeRunner{Scripts: map[string]FakeResponse{"kimi": {Result: RunResult{Stdout: stdout}, Err: err}}}
}
