// Package integration runs real built-binary and subprocess coverage for shipped command surfaces.
// Plan: WS14. PRD: §3, §4, §7, §14.
package integration

import (
	"context"
	"os"
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
		if !d.HeadlessCapable {
			t.Fatalf("Detect() = %#v, want headless crush install with run support", d)
		}
	})

	toolsDir := t.TempDir()
	logPath := filepath.Join(toolsDir, "crush.log")
	buildCrushRunStub(t, toolsDir, logPath)
	prependPath(t, toolsDir)

	a := adapter.Crush{}
	d, err := a.Detect(context.Background())
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if !d.HeadlessCapable || d.Version != "crush version v0.79.1" {
		t.Fatalf("Detect() = %#v, want headless crush version", d)
	}

	result, err := a.Run(context.Background(), adapter.Request{Prompt: "run", Approval: "never", Workdir: toolsDir, Model: "ollama/qwen3:14b"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if string(result.Stdout) != "crush artifact\n" {
		t.Fatalf("Run() stdout = %q, want artifact", result.Stdout)
	}
	readLogContains(t, logPath, "run --quiet --cwd "+toolsDir+" --model ollama/qwen3:14b", "stdin:run")

	verdict, err := a.Review(context.Background(), adapter.Request{Prompt: "review", Approval: "never", Workdir: toolsDir})
	if err != nil {
		t.Fatalf("Review() error = %v", err)
	}
	if !verdict.Pass || verdict.Severity != "low" {
		t.Fatalf("Review() verdict = %#v, want passing low severity", verdict)
	}
	readLogContains(t, logPath, "stdin:review")
}

func buildCrushRunStub(t *testing.T, dir, logPath string) string {
	t.Helper()
	bin := filepath.Join(dir, "crush"+exeSuffix())
	script := `#!/bin/sh
set -eu
if [ "$#" -eq 1 ] && [ "$1" = "--version" ]; then
  printf 'crush version v0.79.1\n'
  exit 0
fi
if [ "$#" -eq 2 ] && [ "$1" = "run" ] && [ "$2" = "--help" ]; then
  printf 'Run a single prompt in non-interactive mode and exit.\n'
  printf 'The prompt can be provided as arguments or piped from stdin.\n'
  printf 'USAGE\n  crush run [prompt...] [--flags]\n'
  exit 0
fi
if [ "$#" -gt 0 ] && [ "$1" = "run" ]; then
  printf '%s\n' "$*" >> "$CRUSH_LOG"
  stdin=$(cat)
  printf 'stdin:%s\n' "$stdin" >> "$CRUSH_LOG"
  case "$stdin" in
    *"Return JSON only"*) printf '{"pass":true,"severity":"low","notes":"ok"}\n' ;;
    *) printf 'crush artifact\n' ;;
  esac
  exit 0
fi
exit 64
`
	mustWriteFile(t, bin, script)
	if err := os.Chmod(bin, 0o755); err != nil {
		t.Fatalf("Chmod(%s) error = %v", bin, err)
	}
	t.Setenv("CRUSH_LOG", logPath)
	return bin
}
