// Package adapter tests the Claude Code adapter.
// Plan: WS-C. PRD: FR-3.1, FR-3.2, FR-7.4.
package adapter

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestClaudeDetectHeadlessCapability(t *testing.T) {
	r := claudeRunner([]byte("claude 2.1.200"), nil)
	d, err := (Claude{Runner: r}).Detect(context.Background())
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if !d.HeadlessCapable || d.Version != "claude 2.1.200" {
		t.Fatalf("Detection = %#v, want headless version", d)
	}
}

func TestClaudeRunEnforcesNonInteractiveApproval(t *testing.T) {
	r := claudeRunner([]byte("{}"), nil)
	_, err := (Claude{Runner: r}).Run(context.Background(), Request{Prompt: "x", Approval: "plan", MaxTurns: 3})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	args := strings.Join(r.Calls[0].Args, " ")
	if !strings.Contains(args, "-p") || !strings.Contains(args, "--output-format json") || !strings.Contains(args, "--permission-mode plan") || !strings.Contains(args, "--max-turns 3") {
		t.Fatalf("args = %q, want print/json/permission/max-turn flags", args)
	}
}

func TestClaudeRunMapsExitCode(t *testing.T) {
	r := claudeRunner(nil, nil)
	r.Scripts["claude"] = FakeResponse{Result: RunResult{ExitCode: 5}}
	got, _ := (Claude{Runner: r}).Run(context.Background(), Request{Prompt: "x", Approval: "plan"})
	if got.ExitCode != 5 {
		t.Fatalf("ExitCode = %d, want 5", got.ExitCode)
	}
}

func TestClaudeRunPassesModelFlag(t *testing.T) {
	r := claudeRunner([]byte("{}"), nil)
	_, err := (Claude{Runner: r}).Run(context.Background(), Request{Prompt: "x", Approval: "plan", Model: "claude-sonnet-5"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	args := strings.Join(r.Calls[0].Args, " ")
	if !strings.Contains(args, "--model claude-sonnet-5") {
		t.Fatalf("args = %q, want model flag", args)
	}
}

func TestClaudeRunPassesEffortFlag(t *testing.T) {
	r := claudeRunner([]byte("{}"), nil)
	_, err := (Claude{Runner: r}).Run(context.Background(), Request{Prompt: "x", Approval: "plan", Effort: "xhigh"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	args := strings.Join(r.Calls[0].Args, " ")
	if !strings.Contains(args, "--effort xhigh") {
		t.Fatalf("args = %q, want effort flag", args)
	}
}

func TestClaudeRunScrubsSecretsFromStdout(t *testing.T) {
	token := "Bearer " + strings.Repeat("a", 16)
	got, err := (Claude{Runner: claudeRunner([]byte(token), nil)}).Run(context.Background(), Request{Prompt: "x", Approval: "plan"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if bytes.Contains(got.Stdout, []byte("abcdefghijklmnop")) || !bytes.Contains(got.Stdout, []byte("<redacted:bearer>")) {
		t.Fatalf("Stdout = %q, want redacted", got.Stdout)
	}
}

func TestClaudeRunDropsPromptAndCompletionFromMeta(t *testing.T) {
	out := []byte(`{"model":"sonnet","total_tokens":4,"result":"raw","content":"raw"}`)
	got, err := (Claude{Runner: claudeRunner(out, nil)}).Run(context.Background(), Request{Prompt: "x", Approval: "plan"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got.ProviderMeta["result"] != "" || got.ProviderMeta["content"] != "" {
		t.Fatalf("ProviderMeta = %#v, want no raw result/content", got.ProviderMeta)
	}
}

func TestClaudeRunRemovesRawResultFromStdout(t *testing.T) {
	out := []byte(`{"model":"sonnet","total_tokens":4,"result":"raw model output"}`)
	got, err := (Claude{Runner: claudeRunner(out, nil)}).Run(context.Background(), Request{Prompt: "x", Approval: "plan"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if bytes.Contains(got.Stdout, []byte("raw model output")) || bytes.Contains(got.Stdout, []byte(`"result"`)) {
		t.Fatalf("Stdout = %q, want provider result removed", got.Stdout)
	}
}

func TestClaudeRunRedactsPlainProviderStdout(t *testing.T) {
	got, err := (Claude{Runner: claudeRunner([]byte("plain model output"), nil)}).Run(context.Background(), Request{Prompt: "x", Approval: "plan"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if bytes.Contains(got.Stdout, []byte("plain model output")) || !bytes.Contains(got.Stdout, []byte("<redacted:provider-output>")) {
		t.Fatalf("Stdout = %q, want provider output redacted", got.Stdout)
	}
}

func TestClaudeReviewParsesVerdict(t *testing.T) {
	v, err := (Claude{Runner: claudeRunner([]byte(`{"pass":true,"severity":"medium","notes":"ok"}`), nil)}).Review(context.Background(), Request{Prompt: "x", Approval: "plan"})
	if err != nil {
		t.Fatalf("Review() error = %v", err)
	}
	if !v.Pass || v.Severity != "medium" {
		t.Fatalf("Verdict = %#v, want pass medium", v)
	}
}

func TestClaudeReviewParsesVerdictFromJSONResult(t *testing.T) {
	out := []byte(`{"result":"{\"pass\":true,\"severity\":\"medium\",\"notes\":\"ok\"}","model":"sonnet"}`)
	v, err := (Claude{Runner: claudeRunner(out, nil)}).Review(context.Background(), Request{Prompt: "x", Approval: "plan"})
	if err != nil {
		t.Fatalf("Review() error = %v", err)
	}
	if !v.Pass || v.Severity != "medium" {
		t.Fatalf("Verdict = %#v, want parsed wrapper verdict", v)
	}
}

func TestClaudeReviewFailsClosedOnUnparseable(t *testing.T) {
	v, err := (Claude{Runner: claudeRunner([]byte("not json"), nil)}).Review(context.Background(), Request{Prompt: "x", Approval: "plan"})
	if err != nil {
		t.Fatalf("Review() error = %v", err)
	}
	if v.Pass || v.Severity != "error" {
		t.Fatalf("Verdict = %#v, want fail-closed error", v)
	}
}

func claudeRunner(stdout []byte, err error) *FakeRunner {
	return &FakeRunner{Scripts: map[string]FakeResponse{"claude": {Result: RunResult{Stdout: stdout}, Err: err}}}
}
