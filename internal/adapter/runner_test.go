// Package adapter defines headless CLI adapter contracts.
// Plan: WS9. PRD: FR-3.1, FR-3.2, FR-7.4.
package adapter

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"runtime"
	"testing"
	"time"
)

func TestOSRunnerRespectsTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := (OSRunner{}).Run(ctx, longSleepArgs(), nil, "")
	if !errors.Is(err, ErrCommandTimeout) {
		t.Fatalf("Run() error = %v, want ErrCommandTimeout", err)
	}
}

func TestOSRunnerTruncatesLargeStdout(t *testing.T) {
	// Windows CI cannot rely on Unix `yes | head`; generate ~1.2MiB portably.
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	res, err := (OSRunner{}).Run(ctx, largeStdoutArgs(t), nil, "")
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(res.Stdout) != maxCapturedBytes {
		t.Fatalf("stdout len = %d, want %d", len(res.Stdout), maxCapturedBytes)
	}
}

// longSleepArgs returns a portable command that sleeps longer than the test timeout.
func longSleepArgs() []string {
	if runtime.GOOS == "windows" {
		// PowerShell sleep; available on GitHub windows-latest runners.
		return []string{"powershell", "-NoProfile", "-Command", "Start-Sleep -Seconds 30"}
	}
	return []string{"sleep", "30"}
}

// largeStdoutArgs returns a portable command that writes > maxCapturedBytes to stdout.
func largeStdoutArgs(t *testing.T) []string {
	t.Helper()
	// Prefer Python when available (common on GHA linux/mac/windows images).
	if path, err := exec.LookPath("python3"); err == nil {
		return []string{path, "-c", "import sys; sys.stdout.buffer.write(b'x'*1200000)"}
	}
	if path, err := exec.LookPath("python"); err == nil {
		return []string{path, "-c", "import sys; sys.stdout.buffer.write(b'x'*1200000)"}
	}
	if runtime.GOOS == "windows" {
		// Fall back to PowerShell without Unix pipeline tools (fast fill, no per-byte loop).
		return []string{
			"powershell", "-NoProfile", "-Command",
			"$b = [byte[]]::new(1200000); [Array]::Fill($b, [byte]120); " +
				"[Console]::OpenStandardOutput().Write($b, 0, $b.Length)",
		}
	}
	return []string{"sh", "-c", "yes x | head -c 1200000"}
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
