// Package integration runs real built-binary and subprocess coverage for shipped command surfaces.
// Plan: WS14. PRD: §3, §4, §7, §14.
package integration

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	realint "github.com/MiviaLabs/mivia-agentkit/internal/integration"
)

type stubCLI struct {
	Name     string
	Version  string
	Stdout   string
	Stderr   string
	ExitCode int
	LogPath  string
}

func buildStubCLI(t *testing.T, dir string, cfg stubCLI) string {
	t.Helper()
	srcDir := filepath.Join(dir, cfg.Name+"-src")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", srcDir, err)
	}
	mustWriteFile(t, filepath.Join(srcDir, "go.mod"), "module stub\n\ngo 1.22\n")
	program := fmt.Sprintf(`package main

import (
	"fmt"
	"os"
	"strings"
)

func main() {
	if len(os.Args) > 1 {
		f, err := os.OpenFile(%q, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err == nil {
			_, _ = fmt.Fprintln(f, strings.Join(os.Args[1:], " "))
			_ = f.Close()
		}
	}
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Print(%q)
		return
	}
	if %q != "" {
		fmt.Fprint(os.Stdout, %q)
	}
	if %q != "" {
		fmt.Fprint(os.Stderr, %q)
	}
	os.Exit(%d)
}
`, cfg.LogPath, cfg.Version, cfg.Stdout, cfg.Stdout, cfg.Stderr, cfg.Stderr, cfg.ExitCode)
	mustWriteFile(t, filepath.Join(srcDir, "main.go"), program)
	bin := filepath.Join(dir, cfg.Name+exeSuffix())
	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Dir = srcDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build stub %s error = %v, output = %s", cfg.Name, err, out)
	}
	return bin
}

func requireOptInBinary(t *testing.T, binary string) {
	t.Helper()
	gate := realint.DefaultGate()
	ok, reason := gate.Allow(gate.RequireBinary(binary))
	if !ok {
		t.Skip(reason)
	}
}

func prependPath(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func readLogContains(t *testing.T, path string, parts ...string) {
	t.Helper()
	got := readFile(t, path)
	for _, part := range parts {
		if !strings.Contains(got, part) {
			t.Fatalf("log %s = %q, missing %q", path, got, part)
		}
	}
}
