// Package adapter defines the Google Antigravity CLI adapter.
// Plan: WS6. PRD: FR-3.1, FR-3.4.
package adapter

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestAntigravityDetectHeadlessCapability(t *testing.T) {
	r := antigravityRunner([]byte("agy 1.0.5"), nil)
	d, err := (Antigravity{Runner: r}).Detect(context.Background())
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if d.Name != "antigravity" || !d.HeadlessCapable || d.Version != "agy 1.0.5" {
		t.Fatalf("Detection = %#v, want headless version", d)
	}
}

func TestAntigravityRunEnforcesNonInteractiveApproval(t *testing.T) {
	r := antigravityRunner([]byte("{}"), nil)
	_, err := (Antigravity{Runner: r}).Run(context.Background(), Request{Prompt: "x", Approval: "yolo"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	args := strings.Join(r.Calls[0].Args, " ")
	if !strings.Contains(args, "agy") || !strings.Contains(args, "-p x") {
		t.Fatalf("args = %q, want agy one-shot prompt flags", args)
	}
	if strings.Contains(args, "--output-format") || strings.Contains(args, "--yolo") {
		t.Fatalf("args = %q, want no legacy Gemini CLI-only flags", args)
	}
}

func TestAntigravityRunMapsExitCode(t *testing.T) {
	r := antigravityRunner(nil, nil)
	r.Scripts["agy"] = FakeResponse{Result: RunResult{ExitCode: 6}}
	got, _ := (Antigravity{Runner: r}).Run(context.Background(), Request{Prompt: "x", Approval: "yolo"})
	if got.ExitCode != 6 {
		t.Fatalf("ExitCode = %d, want 6", got.ExitCode)
	}
}

func TestAntigravityRunScrubsSecretsFromStdout(t *testing.T) {
	got, err := (Antigravity{Runner: antigravityRunner([]byte(fakeAWSKey()), nil)}).Run(context.Background(), Request{Prompt: "x", Approval: "yolo"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if bytes.Contains(got.Stdout, []byte("AKIA")) || !bytes.Contains(got.Stdout, []byte("<redacted:aws>")) {
		t.Fatalf("Stdout = %q, want redacted", got.Stdout)
	}
}

func TestAntigravityReviewParsesVerdict(t *testing.T) {
	out := []byte(`{"pass":true,"severity":"low","notes":"ok"}`)
	v, err := (Antigravity{Runner: antigravityRunner(out, nil)}).Review(context.Background(), Request{Prompt: "x", Approval: "yolo"})
	if err != nil {
		t.Fatalf("Review() error = %v", err)
	}
	if !v.Pass || v.Severity != "low" {
		t.Fatalf("Verdict = %#v, want parsed wrapper verdict", v)
	}
}

func antigravityRunner(stdout []byte, err error) *FakeRunner {
	return &FakeRunner{Scripts: map[string]FakeResponse{"agy": {Result: RunResult{Stdout: stdout}, Err: err}}}
}
