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
	stamp, err := Run(Context{Repo: repo, BroadVerifiers: []string{"true"}})
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

func TestPreflightRunsConfiguredVerifierAndRejectsNonZero(t *testing.T) {
	repo := newRepo(t)
	writeFile(t, repo, "docs/readme.md", "hello\n")
	runGit(t, repo, "add", "docs/readme.md")
	if _, err := Run(Context{Repo: repo, BroadVerifiers: []string{"false"}}); err == nil || !strings.Contains(err.Error(), "exited 1") {
		t.Fatalf("Run() error = %v; want non-zero verifier rejection", err)
	}
	if _, err := os.Stat(filepath.Join(repo, stampRelPath)); !os.IsNotExist(err) {
		t.Fatalf("failed verifier wrote a stamp: %v", err)
	}
}

func TestPreflightRequiresContractRowsForHighRiskChange(t *testing.T) {
	repo := newRepo(t)
	writeFile(t, repo, ".github/workflows/ci.yml", "name: ci\n")
	runGit(t, repo, "add", ".github/workflows/ci.yml")
	_, err := Run(Context{Repo: repo, FocusedVerifiers: []string{"true"}, BroadVerifiers: []string{"true"}, MutationProofs: []string{"drop guard failed"}})
	if err == nil || !strings.Contains(err.Error(), "contract row") {
		t.Fatalf("Run() error = %v, want contract row requirement", err)
	}
}

func TestPreflightRequiresMutationProofForHighRiskChange(t *testing.T) {
	repo := newRepo(t)
	writeFile(t, repo, ".github/workflows/ci.yml", "name: ci\n")
	runGit(t, repo, "add", ".github/workflows/ci.yml")
	_, err := Run(Context{Repo: repo, ContractRows: []string{"ci"}, FocusedVerifiers: []string{"true"}, BroadVerifiers: []string{"true"}})
	if err == nil || !strings.Contains(err.Error(), "mutation proof") {
		t.Fatalf("Run() error = %v, want mutation proof requirement", err)
	}
}

func TestPreflightRequiresFocusedVerifierForHighRiskChange(t *testing.T) {
	repo := newRepo(t)
	writeFile(t, repo, ".github/workflows/ci.yml", "name: ci\n")
	runGit(t, repo, "add", ".github/workflows/ci.yml")
	_, err := Run(Context{Repo: repo, ContractRows: []string{"ci"}, BroadVerifiers: []string{"true"}, MutationProofs: []string{"drop guard failed"}})
	if err == nil || !strings.Contains(err.Error(), "focused verifier") {
		t.Fatalf("Run() error = %v, want focused verifier requirement", err)
	}
}

func TestPreflightRejectsProtectedStampWithNotRun(t *testing.T) {
	repo := newRepo(t)
	writeFile(t, repo, "docs/readme.md", "hello\n")
	runGit(t, repo, "add", "docs/readme.md")
	if _, err := Run(Context{Repo: repo, NotRun: []string{"broad verifier runs in CI"}}); err == nil {
		t.Fatal("Run() error = nil, want not-run rejection")
	}
}

func TestPreflightRejectsNotRunWhenBroadVerifierPresent(t *testing.T) {
	repo := newRepo(t)
	writeFile(t, repo, "docs/readme.md", "hello\n")
	runGit(t, repo, "add", "docs/readme.md")
	_, err := Run(Context{Repo: repo, BroadVerifiers: []string{"true"}, NotRun: []string{"skipped elsewhere"}})
	if err == nil || !strings.Contains(err.Error(), "cannot contain not-run") {
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
	stamp, err := Run(Context{Repo: repo, BroadVerifiers: []string{"true"}, PipelinePreflight: true})
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
	stamp, err := Run(Context{Repo: repo, BroadVerifiers: []string{"true"}, PipelinePreflight: true})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(stamp.VerifierEvidence) != 1 || stamp.VerifierEvidence[0].ExitCode != 0 {
		t.Fatalf("VerifierEvidence = %#v; want successful local evidence", stamp.VerifierEvidence)
	}
	data, err := os.ReadFile(filepath.Join(repo, stampRelPath))
	if err != nil {
		t.Fatalf("ReadFile(stamp) error = %v", err)
	}
	if !strings.Contains(string(data), `"verifier_evidence"`) {
		t.Fatalf("stamp file missing verifier evidence: %s", data)
	}
}

func TestPreflightStampWrittenUnderDotGit(t *testing.T) {
	repo := newRepo(t)
	writeFile(t, repo, "docs/readme.md", "hello\n")
	runGit(t, repo, "add", "docs/readme.md")
	if _, err := Run(Context{Repo: repo, BroadVerifiers: []string{"true"}, PipelinePreflight: true}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(repo, stampRelPath)); err != nil {
		t.Fatalf("stamp path missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repo, "mivia-agent-quality-stamp.json")); !os.IsNotExist(err) {
		t.Fatalf("stamp written outside .git or stat failed err=%v", err)
	}
}

func TestPreflightRejectsDotGitSymlinkEscape(t *testing.T) {
	repo := newRepo(t)
	writeFile(t, repo, "docs/readme.md", "hello\n")
	runGit(t, repo, "add", "docs/readme.md")
	dotGit := filepath.Join(repo, ".git")
	outside := t.TempDir()
	if err := os.Rename(dotGit, filepath.Join(outside, "git")); err != nil {
		t.Fatalf("Rename(.git) error = %v", err)
	}
	if err := os.Symlink(filepath.Join(outside, "git"), dotGit); err != nil {
		t.Fatalf("Symlink(.git) error = %v", err)
	}
	if _, err := Run(Context{Repo: repo, PipelinePreflight: true}); err == nil {
		t.Fatal("Run() error = nil, want symlink rejection")
	}
	if _, err := os.Stat(filepath.Join(outside, "git", "mivia-agent-quality-stamp.json")); !os.IsNotExist(err) {
		t.Fatalf("outside quality stamp exists or Stat failed: %v", err)
	}
}

func TestPreflightWritesStampForLinkedWorktree(t *testing.T) {
	main := newRepo(t)
	worktree := filepath.Join(t.TempDir(), "linked")
	runGit(t, main, "worktree", "add", "-b", "linked-stamp", worktree)
	writeFile(t, worktree, "docs/readme.md", "linked\n")
	runGit(t, worktree, "add", "docs/readme.md")
	if _, err := Run(Context{Repo: worktree, BroadVerifiers: []string{"true"}, PipelinePreflight: true}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	cmd := exec.Command("git", "-C", worktree, "rev-parse", "--git-path", stampName)
	path, err := cmd.Output()
	if err != nil {
		t.Fatalf("rev-parse stamp path error = %v", err)
	}
	stampPath := strings.TrimSpace(string(path))
	if !filepath.IsAbs(stampPath) {
		stampPath = filepath.Join(worktree, stampPath)
	}
	if _, err := os.Stat(stampPath); err != nil {
		t.Fatalf("Stat(linked-worktree stamp) error = %v", err)
	}
	if _, err := CheckStamp(worktree); err != nil {
		t.Fatalf("CheckStamp() error = %v", err)
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
	writeFile(t, repo, "mivia-agent.yaml", "quality:\n  required_verifiers: ['true']\n  verifiers:\n    'true':\n      command: ['true']\n    'false':\n      command: ['false']\n")
	runGit(t, repo, "add", "mivia-agent.yaml")
	runGit(t, repo, "commit", "-q", "-m", "quality config")
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
