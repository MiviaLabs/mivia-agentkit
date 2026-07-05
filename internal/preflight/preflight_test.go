// Package preflight writes and validates quality stamps.
// Plan: WS4. PRD: FR-2.4, FR-7.1.
package preflight

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPreflightWritesStampForLowRiskChange(t *testing.T) {
	repo := newRepo(t)
	writeFile(t, repo, "docs/readme.md", "hello\n")
	runGit(t, repo, "add", "docs/readme.md")
	stamp, err := Run(Context{Repo: repo, BroadVerifiers: []string{"go test ./..."}})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got, want := strings.Join(stamp.ChangedFiles, ","), "docs/readme.md"; got != want {
		t.Fatalf("ChangedFiles got %q want %q", got, want)
	}
	if _, err := os.Stat(filepath.Join(repo, stampRelPath)); err != nil {
		t.Fatalf("stamp not written under .git: %v", err)
	}
}

func TestPreflightRequiresContractRowsForHighRiskChange(t *testing.T) {
	repo := newRepo(t)
	writeFile(t, repo, ".github/workflows/ci.yml", "name: ci\n")
	runGit(t, repo, "add", ".github/workflows/ci.yml")
	_, err := Run(Context{Repo: repo, FocusedVerifiers: []string{"go test ./..."}, BroadVerifiers: []string{"go test ./..."}, MutationProofs: []string{"drop guard failed"}})
	if err == nil || !strings.Contains(err.Error(), "contract row") {
		t.Fatalf("Run() error = %v, want contract row requirement", err)
	}
}

func TestPreflightRequiresMutationProofForHighRiskChange(t *testing.T) {
	repo := newRepo(t)
	writeFile(t, repo, ".github/workflows/ci.yml", "name: ci\n")
	runGit(t, repo, "add", ".github/workflows/ci.yml")
	_, err := Run(Context{Repo: repo, ContractRows: []string{"ci"}, FocusedVerifiers: []string{"go test ./..."}, BroadVerifiers: []string{"go test ./..."}})
	if err == nil || !strings.Contains(err.Error(), "mutation proof") {
		t.Fatalf("Run() error = %v, want mutation proof requirement", err)
	}
}

func TestPreflightRequiresFocusedVerifierForHighRiskChange(t *testing.T) {
	repo := newRepo(t)
	writeFile(t, repo, ".github/workflows/ci.yml", "name: ci\n")
	runGit(t, repo, "add", ".github/workflows/ci.yml")
	_, err := Run(Context{Repo: repo, ContractRows: []string{"ci"}, BroadVerifiers: []string{"go test ./..."}, MutationProofs: []string{"drop guard failed"}})
	if err == nil || !strings.Contains(err.Error(), "focused verifier") {
		t.Fatalf("Run() error = %v, want focused verifier requirement", err)
	}
}

func TestPreflightAcceptsNotRunReasonForMissingBroad(t *testing.T) {
	repo := newRepo(t)
	writeFile(t, repo, "docs/readme.md", "hello\n")
	runGit(t, repo, "add", "docs/readme.md")
	if _, err := Run(Context{Repo: repo, NotRun: []string{"broad verifier runs in CI"}}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
}

func TestPreflightRejectsNotRunWhenBroadVerifierPresent(t *testing.T) {
	repo := newRepo(t)
	writeFile(t, repo, "docs/readme.md", "hello\n")
	runGit(t, repo, "add", "docs/readme.md")
	_, err := Run(Context{Repo: repo, BroadVerifiers: []string{"go test ./..."}, NotRun: []string{"skipped elsewhere"}})
	if err == nil || !strings.Contains(err.Error(), "only allowed when broad verifier is missing") {
		t.Fatalf("Run() error = %v, want broad verifier/not-run conflict", err)
	}
}

func TestPreflightRejectsNotRunWithoutReason(t *testing.T) {
	repo := newRepo(t)
	writeFile(t, repo, "docs/readme.md", "hello\n")
	runGit(t, repo, "add", "docs/readme.md")
	_, err := Run(Context{Repo: repo, NotRun: []string{""}})
	if err == nil || !strings.Contains(err.Error(), "not-run reason") {
		t.Fatalf("Run() error = %v, want not-run reason requirement", err)
	}
}

func TestPreflightHandlesUnstagedUntrackedFile(t *testing.T) {
	repo := newRepo(t)
	writeFile(t, repo, "docs/readme.md", "hello\n")
	stamp, err := Run(Context{Repo: repo, PipelinePreflight: true})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got, want := strings.Join(stamp.ChangedFiles, ","), "docs/readme.md"; got != want {
		t.Fatalf("ChangedFiles got %q want %q", got, want)
	}
}

func TestPreflightWritesPipelinePreflightMetadata(t *testing.T) {
	repo := newRepo(t)
	writeFile(t, repo, "docs/readme.md", "hello\n")
	runGit(t, repo, "add", "docs/readme.md")
	stamp, err := Run(Context{Repo: repo, PipelinePreflight: true})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got := stamp.PipelinePreflight["passed"]; got != true {
		t.Fatalf("PipelinePreflight[passed] got %v want true", got)
	}
	data, err := os.ReadFile(filepath.Join(repo, stampRelPath))
	if err != nil {
		t.Fatalf("ReadFile(stamp) error = %v", err)
	}
	if !strings.Contains(string(data), `"pipeline_preflight"`) {
		t.Fatalf("stamp file missing pipeline_preflight: %s", data)
	}
}

func TestPreflightStampWrittenUnderDotGit(t *testing.T) {
	repo := newRepo(t)
	writeFile(t, repo, "docs/readme.md", "hello\n")
	runGit(t, repo, "add", "docs/readme.md")
	if _, err := Run(Context{Repo: repo, PipelinePreflight: true}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(repo, stampRelPath)); err != nil {
		t.Fatalf("stamp path missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repo, "mivia-agent-quality-stamp.json")); !os.IsNotExist(err) {
		t.Fatalf("stamp written outside .git or stat failed err=%v", err)
	}
}

func TestNewRepoDisablesGlobalSigning(t *testing.T) {
	global := filepath.Join(t.TempDir(), "gitconfig")
	if err := os.WriteFile(global, []byte("[commit]\n\tgpgsign = true\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(global config) error = %v", err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", global)
	repo := newRepo(t)
	writeFile(t, repo, "docs/readme.md", "hello\n")
	runGit(t, repo, "add", "docs/readme.md")
	runGit(t, repo, "commit", "-q", "-m", "docs")
}

func newRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	runGit(t, repo, "init", "-q")
	runGit(t, repo, "config", "user.email", "test@example.invalid")
	runGit(t, repo, "config", "user.name", "Test User")
	runGit(t, repo, "config", "commit.gpgsign", "false")
	runGit(t, repo, "commit", "-q", "--allow-empty", "-m", "init")
	return repo
}

func writeFile(t *testing.T, repo, rel, content string) {
	t.Helper()
	path := filepath.Join(repo, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
}

func runGit(t *testing.T, repo string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v error = %v output = %s", args, err, out)
	}
}
