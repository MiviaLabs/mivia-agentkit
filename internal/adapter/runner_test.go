// Package adapter defines headless CLI adapter contracts.
// Plan: WS9. PRD: FR-3.1, FR-3.2, FR-7.4.
package adapter

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"
)

func TestOSRunnerRespectsTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, err := (OSRunner{}).Run(ctx, []string{"sleep", "30"}, nil, "")
	if !errors.Is(err, ErrCommandTimeout) {
		t.Fatalf("Run() error = %v, want ErrCommandTimeout", err)
	}
}

func TestOSRunnerTruncatesLargeStdout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	res, err := (OSRunner{}).Run(ctx, []string{"sh", "-c", "yes x | head -c 1200000"}, nil, "")
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(res.Stdout) != maxCapturedBytes {
		t.Fatalf("stdout len = %d, want %d", len(res.Stdout), maxCapturedBytes)
	}
}

func TestOSRunnerMissingCommandReturnsError(t *testing.T) {
	_, err := (OSRunner{}).Run(context.Background(), []string{"mivia-agentkit-missing-command-for-test"}, nil, "")
	if err == nil {
		t.Fatalf("Run() error = nil, want missing command error")
	}
}

func TestFakeRunnerRecordsInvocation(t *testing.T) {
	r := &FakeRunner{Scripts: map[string]FakeResponse{"codex": {Result: RunResult{ExitCode: 0}}}}
	_, err := r.Run(context.Background(), []string{"codex", "--version"}, []string{"A=B"}, "/tmp/x")
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(r.Calls) != 1 || r.Calls[0].Workdir != "/tmp/x" || r.Calls[0].Env[0] != "A=B" {
		t.Fatalf("Calls = %#v, want recorded invocation", r.Calls)
	}
}

func TestFakeRunnerRecordsStdin(t *testing.T) {
	r := &FakeRunner{Scripts: map[string]FakeResponse{"crush": {Result: RunResult{ExitCode: 0}}}}
	_, err := r.RunWithInput(context.Background(), []string{"crush", "run"}, nil, "/tmp/x", []byte("prompt"))
	if err != nil {
		t.Fatalf("RunWithInput() error = %v", err)
	}
	if len(r.Calls) != 1 || string(r.Calls[0].Stdin) != "prompt" {
		t.Fatalf("Calls = %#v, want recorded stdin", r.Calls)
	}
}

func TestFakeRunnerScriptsByCommandName(t *testing.T) {
	r := &FakeRunner{Scripts: map[string]FakeResponse{"claude": {Result: RunResult{Stdout: []byte("ok")}}}}
	res, err := r.Run(context.Background(), []string{"claude", "-p", "x"}, nil, "")
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !bytes.Equal(res.Stdout, []byte("ok")) {
		t.Fatalf("stdout = %q, want ok", res.Stdout)
	}
}
