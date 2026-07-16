// Package integration runs real built-binary and subprocess coverage for shipped command surfaces.
// Plan: WS14. PRD: §3, §4, §7, §14.
package integration

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
	readLogContains(t, logPath, "exec", "--sandbox workspace-write", `--config approval_policy="never"`, "--json")

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
	if !strings.Contains(string(result.Stdout), `"type":"status"`) {
		t.Fatalf("Run() stdout = %q, want sanitized status event", result.Stdout)
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

// buildCrushRunStub builds a real, native crush executable rather than a
// POSIX shell script written directly to a .exe-suffixed file: Windows
// tries to load that file as a PE image regardless of its extension and
// refuses to run it ("This version of %1 is not compatible..."). Compiling
// a tiny Go program, as buildStubCLI already does for the other adapters,
// behaves identically on every platform.
func buildCrushRunStub(t *testing.T, dir, logPath string) string {
	t.Helper()
	srcDir := filepath.Join(dir, "crush-src")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", srcDir, err)
	}
	mustWriteFile(t, filepath.Join(srcDir, "go.mod"), "module crushstub\n\ngo 1.22\n")
	program := `package main

import (
	"fmt"
	"io"
	"os"
	"strings"
)

func main() {
	args := os.Args[1:]
	if len(args) == 1 && args[0] == "--version" {
		fmt.Print("crush version v0.79.1\n")
		return
	}
	if len(args) == 2 && args[0] == "run" && args[1] == "--help" {
		fmt.Print("Run a single prompt in non-interactive mode and exit.\n")
		fmt.Print("The prompt can be provided as arguments or piped from stdin.\n")
		fmt.Print("USAGE\n  crush run [prompt...] [--flags]\n")
		return
	}
	if len(args) > 0 && args[0] == "run" {
		logPath := os.Getenv("CRUSH_LOG")
		logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Fprintln(logFile, strings.Join(args, " "))

		stdinData, _ := io.ReadAll(os.Stdin)
		fmt.Fprintf(logFile, "stdin:%s\n", string(stdinData))
		logFile.Close()

		if strings.Contains(string(stdinData), "Return JSON only") {
			fmt.Print("{\"pass\":true,\"severity\":\"low\",\"notes\":\"ok\"}\n")
		} else {
			// Structured event survives sanitizeProviderOutput; plain prose is redacted.
			fmt.Print("{\"type\":\"status\",\"ok\":true}\n")
		}
		return
	}
	os.Exit(64)
}
`
	mustWriteFile(t, filepath.Join(srcDir, "main.go"), program)
	bin := filepath.Join(dir, "crush"+exeSuffix())
	cmd := exec.Command("go", "build", "-buildvcs=false", "-o", bin, ".")
	cmd.Dir = srcDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build crush stub: %v\n%s", err, out)
	}
	t.Setenv("CRUSH_LOG", logPath)
	return bin
}
