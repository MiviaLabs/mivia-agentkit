// Package adapter tests the Codex CLI adapter.
// Plan: WS-C. PRD: FR-3.1, FR-3.2, FR-7.4.
package adapter

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

func TestCodexDetectHeadlessCapability(t *testing.T) {
	r := &FakeRunner{Scripts: map[string]FakeResponse{"codex": {Result: RunResult{Stdout: []byte("codex 1.2.3")}}}}
	d, err := (Codex{Runner: r}).Detect(context.Background())
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if !d.HeadlessCapable || d.Version != "codex 1.2.3" {
		t.Fatalf("Detection = %#v, want headless version", d)
	}
}

func TestCodexRunEnforcesNonInteractiveApproval(t *testing.T) {
	r := codexRunner([]byte("{}"), nil)
	_, err := (Codex{Runner: r}).Run(context.Background(), Request{Prompt: "x", Approval: "never"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	args := strings.Join(r.Calls[0].Args, " ")
	if !strings.Contains(args, "exec") || !strings.Contains(args, `--config approval_policy="never"`) || !strings.Contains(args, "--sandbox workspace-write") {
		t.Fatalf("args = %q, want non-interactive approval and sandbox flags", args)
	}
}

func TestCodexRunMapsExitCode(t *testing.T) {
	r := codexRunner([]byte("{}"), nil)
	r.Scripts["codex"] = FakeResponse{Result: RunResult{ExitCode: 7}}
	got, _ := (Codex{Runner: r}).Run(context.Background(), Request{Prompt: "x", Approval: "never"})
	if got.ExitCode != 7 {
		t.Fatalf("ExitCode = %d, want 7", got.ExitCode)
	}
}

func TestCodexRunScrubsSecretsFromStdout(t *testing.T) {
	got, err := (Codex{Runner: codexRunner([]byte(fakeAWSKey()), nil)}).Run(context.Background(), Request{Prompt: "x", Approval: "never"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if bytes.Contains(got.Stdout, []byte("AKIA")) || !bytes.Contains(got.Stdout, []byte("<redacted:aws>")) {
		t.Fatalf("Stdout = %q, want redacted", got.Stdout)
	}
}

func TestCodexRunTruncatesLargeStdout(t *testing.T) {
	out := append([]byte(`{"safe":"`), bytes.Repeat([]byte("x"), maxCapturedBytes+20)...)
	out = append(out, []byte(`"}`)...)
	got, err := (Codex{Runner: codexRunner(out, nil)}).Run(context.Background(), Request{Prompt: "x", Approval: "never"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(got.Stdout) != maxCapturedBytes {
		t.Fatalf("stdout len = %d, want %d", len(got.Stdout), maxCapturedBytes)
	}
}

func TestCodexRunRespectsTimeout(t *testing.T) {
	errRunner := codexRunner(nil, ErrCommandTimeout)
	_, err := (Codex{Runner: errRunner}).Run(context.Background(), Request{Prompt: "x", Approval: "never", Timeout: time.Millisecond})
	if err == nil {
		t.Fatalf("Run() error = nil, want timeout")
	}
}

func TestCodexRunPassesModelFlag(t *testing.T) {
	r := codexRunner([]byte("{}"), nil)
	_, err := (Codex{Runner: r}).Run(context.Background(), Request{Prompt: "x", Approval: "never", Model: "gpt-5.5"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	args := strings.Join(r.Calls[0].Args, " ")
	if !strings.Contains(args, "--model gpt-5.5") {
		t.Fatalf("args = %q, want model flag", args)
	}
}

func TestCodexRunPassesArtifactOutFlag(t *testing.T) {
	r := codexRunner([]byte("{}"), nil)
	_, err := (Codex{Runner: r}).Run(context.Background(), Request{Prompt: "x", Approval: "never", ArtifactOut: "/tmp/out.md"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	args := strings.Join(r.Calls[0].Args, " ")
	if !strings.Contains(args, "--output-last-message /tmp/out.md") {
		t.Fatalf("args = %q, want output-last-message artifact path", args)
	}
}

func TestCodexRunPassesReasoningEffortOverride(t *testing.T) {
	r := codexRunner([]byte("{}"), nil)
	_, err := (Codex{Runner: r}).Run(context.Background(), Request{Prompt: "x", Approval: "never", Effort: "xhigh"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	args := strings.Join(r.Calls[0].Args, " ")
	if !strings.Contains(args, `--config model_reasoning_effort="xhigh"`) {
		t.Fatalf("args = %q, want reasoning effort override", args)
	}
}

func TestCodexRunRejectsUnsupportedEffort(t *testing.T) {
	r := codexRunner([]byte("{}"), nil)
	_, err := (Codex{Runner: r}).Run(context.Background(), Request{Prompt: "x", Approval: "never", Effort: "max"})
	if err == nil || !strings.Contains(err.Error(), "codex unsupported effort") {
		t.Fatalf("Run() error = %v, want codex unsupported effort", err)
	}
	if len(r.Calls) != 0 {
		t.Fatalf("runner calls = %d, want 0 before unsupported effort reaches CLI", len(r.Calls))
	}
}

func TestCodexRunRejectsUnsupportedParams(t *testing.T) {
	r := codexRunner([]byte("{}"), nil)
	_, err := (Codex{Runner: r}).Run(context.Background(), Request{Prompt: "x", Approval: "never", Params: map[string]string{"provider": "openai"}})
	if err == nil || !strings.Contains(err.Error(), "codex unsupported params") {
		t.Fatalf("Run() error = %v, want codex unsupported params", err)
	}
	if len(r.Calls) != 0 {
		t.Fatalf("runner calls = %d, want 0 before unsupported params reach CLI", len(r.Calls))
	}
}

func TestCodexRunDropsPromptAndCompletionFromMeta(t *testing.T) {
	out := []byte(`{"model_id":"m","total_tokens":12,"prompt":"raw","completion":"raw"}`)
	got, err := (Codex{Runner: codexRunner(out, nil)}).Run(context.Background(), Request{Prompt: "x", Approval: "never"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got.ProviderMeta["prompt"] != "" || got.ProviderMeta["completion"] != "" {
		t.Fatalf("ProviderMeta = %#v, want no raw prompt/completion", got.ProviderMeta)
	}
}

func TestCodexRunRemovesAgentMessageTextFromStdout(t *testing.T) {
	out := []byte(`{"type":"item.completed","item":{"type":"agent_message","text":"raw model output"}}
{"type":"turn.completed","usage":{"total_tokens":12}}`)
	got, err := (Codex{Runner: codexRunner(out, nil)}).Run(context.Background(), Request{Prompt: "x", Approval: "never"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if bytes.Contains(got.Stdout, []byte("raw model output")) || bytes.Contains(got.Stdout, []byte(`"text"`)) {
		t.Fatalf("Stdout = %q, want provider text removed", got.Stdout)
	}
}

func TestCodexRunRedactsPlainProviderStdout(t *testing.T) {
	got, err := (Codex{Runner: codexRunner([]byte("plain model output"), nil)}).Run(context.Background(), Request{Prompt: "x", Approval: "never"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if bytes.Contains(got.Stdout, []byte("plain model output")) || !bytes.Contains(got.Stdout, []byte("<redacted:provider-output>")) {
		t.Fatalf("Stdout = %q, want provider output redacted", got.Stdout)
	}
}

func TestCodexReviewParsesVerdict(t *testing.T) {
	v, err := (Codex{Runner: codexRunner([]byte(`{"pass":true,"severity":"low","notes":"ok"}`), nil)}).Review(context.Background(), Request{Prompt: "x", Approval: "never"})
	if err != nil {
		t.Fatalf("Review() error = %v", err)
	}
	if !v.Pass || v.Severity != "low" {
		t.Fatalf("Verdict = %#v, want pass low", v)
	}
}

func TestCodexReviewParsesVerdictFromJSONLMessageText(t *testing.T) {
	out := []byte(`{"type":"item.completed","item":{"type":"agent_message","text":"{\"pass\":true,\"severity\":\"low\",\"notes\":\"ok\"}"}}`)
	v, err := (Codex{Runner: codexRunner(out, nil)}).Review(context.Background(), Request{Prompt: "x", Approval: "never"})
	if err != nil {
		t.Fatalf("Review() error = %v", err)
	}
	if !v.Pass || v.Severity != "low" {
		t.Fatalf("Verdict = %#v, want parsed wrapper verdict", v)
	}
}

func TestCodexReviewRejectsUnsupportedEffort(t *testing.T) {
	r := codexRunner([]byte(`{"pass":true,"severity":"low","notes":"ok"}`), nil)
	_, err := (Codex{Runner: r}).Review(context.Background(), Request{Prompt: "x", Approval: "never", Effort: "max"})
	if err == nil || !strings.Contains(err.Error(), "codex unsupported effort") {
		t.Fatalf("Review() error = %v, want codex unsupported effort", err)
	}
	if len(r.Calls) != 0 {
		t.Fatalf("runner calls = %d, want 0 before unsupported effort reaches CLI", len(r.Calls))
	}
}

func TestCodexReviewRejectsUnsupportedParams(t *testing.T) {
	r := codexRunner([]byte(`{"pass":true,"severity":"low","notes":"ok"}`), nil)
	_, err := (Codex{Runner: r}).Review(context.Background(), Request{Prompt: "x", Approval: "never", Params: map[string]string{"provider": "openai"}})
	if err == nil || !strings.Contains(err.Error(), "codex unsupported params") {
		t.Fatalf("Review() error = %v, want codex unsupported params", err)
	}
	if len(r.Calls) != 0 {
		t.Fatalf("runner calls = %d, want 0 before unsupported params reach CLI", len(r.Calls))
	}
}

func TestCodexReviewFailsClosedOnUnparseable(t *testing.T) {
	v, err := (Codex{Runner: codexRunner([]byte("not json"), nil)}).Review(context.Background(), Request{Prompt: "x", Approval: "never"})
	if err != nil {
		t.Fatalf("Review() error = %v", err)
	}
	if v.Pass || v.Severity != "error" {
		t.Fatalf("Verdict = %#v, want fail-closed error", v)
	}
}

func codexRunner(stdout []byte, err error) *FakeRunner {
	return &FakeRunner{Scripts: map[string]FakeResponse{"codex": {Result: RunResult{Stdout: stdout}, Err: err}}}
}
