// Package adapter defines the Crush adapter.
// Plan: WS6. PRD: FR-3.1, FR-3.4.
package adapter

import (
	"bytes"
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestCrushDetectHeadlessRunSupport(t *testing.T) {
	r := &crushDetectRunner{
		version: []byte("crush version v0.79.1\n"),
		help:    []byte("Run a single prompt in non-interactive mode and exit. The prompt can be provided as arguments or piped from stdin.\nUSAGE\n  crush run [prompt...] [--flags]\n"),
	}
	d, err := (Crush{Runner: r}).Detect(context.Background())
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if !d.HeadlessCapable {
		t.Fatalf("HeadlessCapable = false, want true when run --help confirms noninteractive run")
	}
	if d.Version != "crush version v0.79.1" {
		t.Fatalf("Version = %q, want crush version", d.Version)
	}
	wantCalls := [][]string{{"crush", "--version"}, {"crush", "run", "--help"}}
	if !reflect.DeepEqual(r.calls, wantCalls) {
		t.Fatalf("calls = %#v, want %#v", r.calls, wantCalls)
	}
}

func TestCrushDetectRequiresRunHelpNoninteractiveSupport(t *testing.T) {
	r := &crushDetectRunner{
		version: []byte("crush version v0.79.1\n"),
		help:    []byte("USAGE\n  crush [flags]\n"),
	}
	d, err := (Crush{Runner: r}).Detect(context.Background())
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if d.HeadlessCapable {
		t.Fatalf("HeadlessCapable = true, want false without run --help noninteractive support")
	}
}

func TestCrushRunInvokesCrushRunWithCWDModelAndPrompt(t *testing.T) {
	// Structured JSON event without raw provider fields survives sanitization.
	r := crushRunner([]byte(`{"type":"status","ok":true}`), nil)
	got, err := (Crush{Runner: r}).Run(context.Background(), Request{Prompt: "long prompt", Workdir: "/repo", Approval: "never", Model: "ollama/qwen3:14b"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !bytes.Contains(got.Stdout, []byte(`"type":"status"`)) {
		t.Fatalf("Stdout = %q, want sanitized status event", got.Stdout)
	}
	if len(r.Calls) != 1 {
		t.Fatalf("runner calls = %d, want 1", len(r.Calls))
	}
	call := r.Calls[0]
	wantArgs := []string{"crush", "run", "--quiet", "--cwd", "/repo", "--model", "ollama/qwen3:14b"}
	if !reflect.DeepEqual(call.Args, wantArgs) {
		t.Fatalf("args = %#v, want %#v", call.Args, wantArgs)
	}
	if call.Workdir != "" {
		t.Fatalf("workdir = %q, want empty because --cwd carries the repo", call.Workdir)
	}
	if string(call.Stdin) != "long prompt" {
		t.Fatalf("stdin = %q, want prompt through stdin", call.Stdin)
	}
}

func TestCrushRunRejectsUnsupportedEffort(t *testing.T) {
	r := crushRunner([]byte("artifact"), nil)
	_, err := (Crush{Runner: r}).Run(context.Background(), Request{Prompt: "x", Approval: "never", Effort: "low"})
	if err == nil || !strings.Contains(err.Error(), "crush unsupported effort") {
		t.Fatalf("Run() error = %v, want crush unsupported effort", err)
	}
	if len(r.Calls) != 0 {
		t.Fatalf("runner calls = %d, want 0 before unsupported effort reaches CLI", len(r.Calls))
	}
}

func TestCrushReviewParsesJSONVerdict(t *testing.T) {
	v, err := (Crush{Runner: crushRunner([]byte(`{"pass":true,"severity":"low","notes":"ok"}`), nil)}).Review(context.Background(), Request{Prompt: "review", Workdir: "/repo", Approval: "never", Model: "ollama/qwen3:14b"})
	if err != nil {
		t.Fatalf("Review() error = %v", err)
	}
	if !v.Pass || v.Severity != "low" || v.Notes != "ok" {
		t.Fatalf("Verdict = %#v, want parsed pass/low verdict", v)
	}
}

func TestCrushReviewFailsClosedOnUnparseable(t *testing.T) {
	v, err := (Crush{Runner: crushRunner([]byte("not json"), nil)}).Review(context.Background(), Request{Prompt: "review", Workdir: "/repo", Approval: "never"})
	if err != nil {
		t.Fatalf("Review() error = %v", err)
	}
	if v.Pass || v.Severity != "error" {
		t.Fatalf("Verdict = %#v, want fail-closed error verdict", v)
	}
}

func TestCrushRunScrubsSecretsFromStdout(t *testing.T) {
	got, err := (Crush{Runner: crushRunner([]byte(fakeAWSKey()), nil)}).Run(context.Background(), Request{Prompt: "x", Approval: "never"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if bytes.Contains(got.Stdout, []byte("AKIA")) || !bytes.Contains(got.Stdout, []byte("<redacted:aws>")) {
		t.Fatalf("Stdout = %q, want redacted", got.Stdout)
	}
}

func TestCrushRunRedactsProviderOutputAndPromptFields(t *testing.T) {
	// Plain model text must be redacted like other adapters.
	got, err := (Crush{Runner: crushRunner([]byte("raw assistant prose about the task"), nil)}).Run(context.Background(), Request{Prompt: "x", Approval: "never"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if bytes.Contains(got.Stdout, []byte("raw assistant prose")) {
		t.Fatalf("Stdout = %q, want plain provider output redacted", got.Stdout)
	}
	if !bytes.Contains(got.Stdout, []byte("<redacted:provider-output>")) {
		t.Fatalf("Stdout = %q, want provider-output redaction marker", got.Stdout)
	}

	// JSON payloads with prompt fields must drop raw prompt content.
	payload := []byte(`{"type":"result","prompt":"SECRET_PROMPT_VALUE","result":"done"}`)
	got, err = (Crush{Runner: crushRunner(payload, nil)}).Run(context.Background(), Request{Prompt: "x", Approval: "never"})
	if err != nil {
		t.Fatalf("Run() JSON error = %v", err)
	}
	if bytes.Contains(got.Stdout, []byte("SECRET_PROMPT_VALUE")) {
		t.Fatalf("Stdout = %q, want prompt field removed", got.Stdout)
	}
}

func crushRunner(stdout []byte, err error) *FakeRunner {
	return &FakeRunner{Scripts: map[string]FakeResponse{"crush": {Result: RunResult{Stdout: stdout}, Err: err}}}
}

type crushDetectRunner struct {
	version []byte
	help    []byte
	calls   [][]string
}

func (r *crushDetectRunner) Run(_ context.Context, args []string, _ []string, _ string) (RunResult, error) {
	r.calls = append(r.calls, append([]string(nil), args...))
	switch strings.Join(args, " ") {
	case "crush --version":
		return RunResult{Stdout: r.version}, nil
	case "crush run --help":
		return RunResult{Stdout: r.help}, nil
	default:
		return RunResult{ExitCode: 127}, fmt.Errorf("unexpected args %v", args)
	}
}
