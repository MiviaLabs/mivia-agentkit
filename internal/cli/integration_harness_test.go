// Package cli implements the mivia-agent command surface.
// Plan: WS14. PRD: §3, §9, §14.
package cli

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildBinaryProducesRunnableExecutable(t *testing.T) {
	bin, err := BuildBinary(BinaryBuild{ModuleRoot: filepath.Join("..", "..")})
	if err != nil {
		t.Fatalf("BuildBinary() error = %v", err)
	}
	result, err := RunBinary(context.Background(), bin, BinaryRun{Args: []string{"version"}})
	if err != nil {
		t.Fatalf("RunBinary() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("RunBinary() exit = %d, want 0, stderr=%s", result.ExitCode, result.Stderr)
	}
	if !strings.Contains(result.Stdout, "mivia-agent ") {
		t.Fatalf("stdout = %q, want version output", result.Stdout)
	}
}

func TestRunBinaryCapturesExitCodeAndStreams(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "main.go")
	program := `package main
import (
	"fmt"
	"os"
)
func main() {
	fmt.Println(os.Getwd())
	fmt.Fprintln(os.Stderr, "stderr line")
	os.Exit(7)
}`
	if err := os.WriteFile(src, []byte(program), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", src, err)
	}
	bin := filepath.Join(dir, "fixture"+binarySuffix())
	build := exec.Command("go", "build", "-buildvcs=false", "-o", bin, src)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build error = %v, output = %s", err, out)
	}
	result, err := RunBinary(context.Background(), bin, BinaryRun{
		Args:  []string{"fixture"},
		Dir:   dir,
		Scrub: map[string]string{dir: "<tmp>"},
	})
	if err != nil {
		t.Fatalf("RunBinary() error = %v", err)
	}
	if result.ExitCode != 7 {
		t.Fatalf("ExitCode = %d, want 7", result.ExitCode)
	}
	if !strings.Contains(result.Stdout, "<tmp>") {
		t.Fatalf("stdout = %q, want scrubbed temp path", result.Stdout)
	}
	if !strings.Contains(result.Stderr, "stderr line") {
		t.Fatalf("stderr = %q, want captured stderr", result.Stderr)
	}
}
