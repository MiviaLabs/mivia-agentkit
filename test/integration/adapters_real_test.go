// Package integration runs real built-binary and subprocess coverage for shipped command surfaces.
// Plan: WS14. PRD: §3, §4, §7, §14.
package integration

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/MiviaLabs/mivia-agentkit/internal/adapter"
)

func TestCodexAdapterRealSubprocessContract(t *testing.T) {
	t.Run("installed-cli-detect", func(t *testing.T) {
		requireOptInBinary(t, "codex")
		d, err := (adapter.Codex{}).Detect(context.Background())
		if err != nil {
			t.Fatalf("Detect() error = %v", err)
		}
		if !d.HeadlessCapable {
			t.Fatalf("Detect() = %#v, want headless codex install", d)
		}
	})

	toolsDir := t.TempDir()
	logPath := filepath.Join(toolsDir, "codex.log")
	buildStubCLI(t, toolsDir, stubCLI{
		Name:    "codex",
		Version: "codex 1.2.3",
		Stdout:  `{"pass":true,"severity":"low","notes":"ok","model_id":"gpt-5","total_tokens":12}`,
		LogPath: logPath,
	})
	prependPath(t, toolsDir)

	a := adapter.Codex{}
	d, err := a.Detect(context.Background())
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if !d.HeadlessCapable || d.Version != "codex 1.2.3" {
		t.Fatalf("Detect() = %#v, want stubbed version", d)
	}

	result, err := a.Run(context.Background(), adapter.Request{Prompt: "run", Approval: "never", Workdir: toolsDir})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ProviderMeta["model_id"] != "gpt-5" {
		t.Fatalf("ProviderMeta = %#v, want model_id", result.ProviderMeta)
	}
	readLogContains(t, logPath, "exec", "--sandbox workspace-write", "--ask-for-approval never", "--json")

	verdict, err := a.Review(context.Background(), adapter.Request{Prompt: "review", Approval: "never", Workdir: toolsDir})
	if err != nil {
		t.Fatalf("Review() error = %v", err)
	}
	if !verdict.Pass || verdict.Severity != "low" {
		t.Fatalf("Review() verdict = %#v, want passing low severity", verdict)
	}
}

func TestClaudeAdapterRealSubprocessContract(t *testing.T) {
	t.Run("installed-cli-detect", func(t *testing.T) {
		requireOptInBinary(t, "claude")
		d, err := (adapter.Claude{}).Detect(context.Background())
		if err != nil {
			t.Fatalf("Detect() error = %v", err)
		}
		if !d.HeadlessCapable {
			t.Fatalf("Detect() = %#v, want headless claude install", d)
		}
	})

	toolsDir := t.TempDir()
	logPath := filepath.Join(toolsDir, "claude.log")
	buildStubCLI(t, toolsDir, stubCLI{
		Name:    "claude",
		Version: "claude 2.1.200",
		Stdout:  `{"pass":true,"severity":"medium","notes":"ok","model":"sonnet","total_tokens":9}`,
		LogPath: logPath,
	})
	prependPath(t, toolsDir)

	a := adapter.Claude{}
	d, err := a.Detect(context.Background())
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if !d.HeadlessCapable || d.Version != "claude 2.1.200" {
		t.Fatalf("Detect() = %#v, want stubbed version", d)
	}

	result, err := a.Run(context.Background(), adapter.Request{Prompt: "run", Approval: "plan", MaxTurns: 2, Workdir: toolsDir})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ProviderMeta["model"] != "sonnet" {
		t.Fatalf("ProviderMeta = %#v, want model", result.ProviderMeta)
	}
	readLogContains(t, logPath, "-p", "--output-format json", "--permission-mode plan", "--max-turns 2")

	verdict, err := a.Review(context.Background(), adapter.Request{Prompt: "review", Approval: "plan", Workdir: toolsDir})
	if err != nil {
		t.Fatalf("Review() error = %v", err)
	}
	if !verdict.Pass || verdict.Severity != "medium" {
		t.Fatalf("Review() verdict = %#v, want passing medium severity", verdict)
	}
}

func TestAntigravityAdapterRealSubprocessContract(t *testing.T) {
	t.Run("installed-cli-detect", func(t *testing.T) {
		requireOptInBinary(t, "agy")
		d, err := (adapter.Antigravity{}).Detect(context.Background())
		if err != nil {
			t.Fatalf("Detect() error = %v", err)
		}
		if !d.HeadlessCapable {
			t.Fatalf("Detect() = %#v, want headless antigravity install", d)
		}
	})

	toolsDir := t.TempDir()
	logPath := filepath.Join(toolsDir, "agy.log")
	buildStubCLI(t, toolsDir, stubCLI{
		Name:    "agy",
		Version: "agy 1.0.5",
		Stdout:  `{"pass":true,"severity":"low","notes":"ok","model":"agy","total_tokens":5}`,
		LogPath: logPath,
	})
	prependPath(t, toolsDir)

	a := adapter.Antigravity{}
	d, err := a.Detect(context.Background())
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if !d.HeadlessCapable || d.Version != "agy 1.0.5" {
		t.Fatalf("Detect() = %#v, want stubbed version", d)
	}

	result, err := a.Run(context.Background(), adapter.Request{Prompt: "run", Approval: "never", Workdir: toolsDir})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("Run() exit = %d, want 0", result.ExitCode)
	}
	readLogContains(t, logPath, "-p run")

	verdict, err := a.Review(context.Background(), adapter.Request{Prompt: "review", Approval: "never", Workdir: toolsDir})
	if err != nil {
		t.Fatalf("Review() error = %v", err)
	}
	if !verdict.Pass || verdict.Severity != "low" {
		t.Fatalf("Review() verdict = %#v, want passing low severity", verdict)
	}
}

func TestCrushAdapterRealSubprocessContract(t *testing.T) {
	t.Run("installed-cli-detect", func(t *testing.T) {
		requireOptInBinary(t, "crush")
		d, err := (adapter.Crush{}).Detect(context.Background())
		if err != nil {
			t.Fatalf("Detect() error = %v", err)
		}
		if d.HeadlessCapable {
			t.Fatalf("Detect() = %#v, want guidance-only crush install", d)
		}
	})

	toolsDir := t.TempDir()
	logPath := filepath.Join(toolsDir, "crush.log")
	buildStubCLI(t, toolsDir, stubCLI{
		Name:    "crush",
		Version: "crush 0.12.0",
		Stdout:  `guidance`,
		LogPath: logPath,
	})
	prependPath(t, toolsDir)

	a := adapter.Crush{}
	d, err := a.Detect(context.Background())
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if d.HeadlessCapable || d.Version != "crush 0.12.0" {
		t.Fatalf("Detect() = %#v, want non-headless crush version", d)
	}

	if _, err := a.Run(context.Background(), adapter.Request{Prompt: "run", Approval: "never", Workdir: toolsDir}); !errors.Is(err, adapter.ErrNotHeadlessCapable) {
		t.Fatalf("Run() error = %v, want ErrNotHeadlessCapable", err)
	}
	if _, err := a.Review(context.Background(), adapter.Request{Prompt: "review", Approval: "never", Workdir: toolsDir}); !errors.Is(err, adapter.ErrNotHeadlessCapable) {
		t.Fatalf("Review() error = %v, want ErrNotHeadlessCapable", err)
	}
	logData := readFile(t, logPath)
	if logData != "--version\n" {
		t.Fatalf("crush log = %q, want detect-only subprocess call", logData)
	}
}
