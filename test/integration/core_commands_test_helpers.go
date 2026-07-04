// Package integration runs real built-binary and subprocess coverage for shipped command surfaces.
// Plan: WS14. PRD: §3, §4, §7, §9, §14.
package integration

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	cliint "github.com/MiviaLabs/mivia-agentkit/internal/cli"
)

type integrationEnv struct {
	binary   string
	repo     string
	home     string
	extraEnv []string
}

func newIntegrationEnv(t *testing.T) integrationEnv {
	t.Helper()
	bin, err := cliint.BuildBinary(cliint.BinaryBuild{ModuleRoot: filepath.Join("..", "..")})
	if err != nil {
		t.Fatalf("BuildBinary() error = %v", err)
	}
	return integrationEnv{
		binary: bin,
		repo:   tempGitRepo(t),
		home:   t.TempDir(),
	}
}

func (e integrationEnv) withEnv(kv ...string) integrationEnv {
	e.extraEnv = append(e.extraEnv, kv...)
	return e
}

func (e integrationEnv) run(t *testing.T, args ...string) cliint.BinaryResult {
	t.Helper()
	return e.runWithInput(t, nil, args...)
}

func (e integrationEnv) runWithInput(t *testing.T, input []byte, args ...string) cliint.BinaryResult {
	t.Helper()
	result, err := cliint.RunBinary(context.Background(), e.binary, cliint.BinaryRun{
		Args:  args,
		Dir:   e.repo,
		Env:   e.env(),
		Stdin: input,
		Scrub: map[string]string{
			e.repo: filepath.ToSlash("<repo>"),
			e.home: filepath.ToSlash("<home>"),
		},
	})
	if err != nil {
		t.Fatalf("RunBinary(%v) error = %v", args, err)
	}
	return result
}

func (e integrationEnv) env() []string {
	return append([]string{
		"HOME=" + e.home,
		"USERPROFILE=" + e.home,
	}, e.extraEnv...)
}

func tempGitRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	runGit(t, repo, "init", "-q")
	runGit(t, repo, "config", "user.email", "test@example.invalid")
	runGit(t, repo, "config", "user.name", "Test User")
	runGit(t, repo, "commit", "-q", "--allow-empty", "-m", "init")
	return repo
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v error = %v, output = %s", args, err, out)
	}
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", path, err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	return string(data)
}

func decodeJSON[T any](t *testing.T, raw string, dest *T) {
	t.Helper()
	if err := json.Unmarshal([]byte(raw), dest); err != nil {
		t.Fatalf("Unmarshal(%q) error = %v", raw, err)
	}
}

func exeSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}
