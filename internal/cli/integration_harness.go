// Package cli implements the mivia-agent command surface.
// Plan: WS14. PRD: §3, §9, §14.
package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
)

// BinaryBuild configures a real-binary build for integration coverage.
type BinaryBuild struct {
	ModuleRoot string
	LDFlags    string
}

// BinaryRun configures one subprocess execution of the built binary.
type BinaryRun struct {
	Args  []string
	Dir   string
	Env   []string
	Stdin []byte
	Scrub map[string]string
}

// BinaryResult captures subprocess streams and exit code.
type BinaryResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

type buildCacheEntry struct {
	once sync.Once
	path string
	err  error
}

var binaryBuildCache sync.Map

// BuildBinary builds the real mivia-agent executable for integration tests.
func BuildBinary(cfg BinaryBuild) (string, error) {
	root, err := filepath.Abs(defaultString(cfg.ModuleRoot, "."))
	if err != nil {
		return "", err
	}
	if cfg.LDFlags != "" {
		return buildBinary(root, cfg.LDFlags)
	}
	entryValue, _ := binaryBuildCache.LoadOrStore(root, &buildCacheEntry{})
	entry := entryValue.(*buildCacheEntry)
	entry.once.Do(func() {
		entry.path, entry.err = buildBinary(root, "")
	})
	return entry.path, entry.err
}

// RunBinary runs the built binary and captures scrubbed output and exit code.
func RunBinary(ctx context.Context, binary string, run BinaryRun) (BinaryResult, error) {
	if strings.TrimSpace(binary) == "" {
		return BinaryResult{}, errors.New("binary path is required")
	}
	if len(run.Args) == 0 {
		return BinaryResult{}, errors.New("at least one binary argument is required")
	}
	cmd := exec.CommandContext(ctx, binary, run.Args...)
	cmd.Dir = run.Dir
	cmd.Env = append(os.Environ(), run.Env...)
	if len(run.Stdin) > 0 {
		cmd.Stdin = bytes.NewReader(run.Stdin)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	result := BinaryResult{
		Stdout: scrubOutput(stdout.String(), run.Scrub),
		Stderr: scrubOutput(stderr.String(), run.Scrub),
	}
	if err == nil {
		return result, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		result.ExitCode = exitErr.ExitCode()
		return result, nil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return result, ctxErr
	}
	return result, err
}

func buildBinary(moduleRoot, ldflags string) (string, error) {
	tmpDir, err := os.MkdirTemp("", "mivia-agent-bin-*")
	if err != nil {
		return "", err
	}
	bin := filepath.Join(tmpDir, "mivia-agent"+binarySuffix())
	args := []string{"build", "-o", bin}
	if ldflags != "" {
		args = append(args, "-ldflags", ldflags)
	}
	args = append(args, "./cmd/mivia-agent")
	cmd := exec.Command("go", args...)
	cmd.Dir = moduleRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("go build %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return bin, nil
}

func scrubOutput(raw string, scrub map[string]string) string {
	if len(scrub) == 0 || raw == "" {
		return raw
	}
	type replacement struct {
		from string
		to   string
	}
	replacements := make([]replacement, 0, len(scrub))
	for from, to := range scrub {
		replacements = append(replacements, replacement{from: from, to: to})
	}
	sort.Slice(replacements, func(i, j int) bool {
		return len(replacements[i].from) > len(replacements[j].from)
	})
	out := raw
	for _, replacement := range replacements {
		if replacement.from == "" {
			continue
		}
		out = strings.ReplaceAll(out, replacement.from, replacement.to)
		out = strings.ReplaceAll(out, filepath.ToSlash(replacement.from), replacement.to)
	}
	return out
}

func binarySuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}
