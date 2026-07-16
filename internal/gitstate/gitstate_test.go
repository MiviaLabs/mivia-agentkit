package gitstate

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestDetectRootFindsGitDir(t *testing.T) {
	repo := initRepo(t)
	nested := filepath.Join(repo, "a", "b")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	got, err := DetectRoot(nested)
	if err != nil {
		t.Fatalf("DetectRoot() error = %v, want nil", err)
	}
	if got != repo {
		t.Fatalf("DetectRoot() = %q, want %q", got, repo)
	}
}

func TestDetectRootErrorsOutsideRepo(t *testing.T) {
	// t.TempDir() may sit under a directory with a stray .git (e.g.
	// /tmp/.git), causing DetectRoot to walk up and succeed spuriously.
	// Use /var/tmp as a known-clean base on typical systems.
	base := "/var/tmp"
	if _, err := os.Stat(base); err != nil {
		t.Skipf("clean temp base %q unavailable", base)
	}
	if _, err := os.Stat(filepath.Join(base, ".git")); err == nil {
		t.Skipf("clean temp base %q has a .git; skipping to avoid false negative", base)
	}
	clean, err := os.MkdirTemp(base, "gitstate-test-")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(clean) })
	if _, err := DetectRoot(clean); err == nil {
		t.Fatal("DetectRoot() error = nil, want error")
	}
}

func TestHeadReturnsCommitSha(t *testing.T) {
	repo := initRepo(t)
	writeFile(t, repo, "README.md", "hello\n")
	git(t, repo, "add", "README.md")
	git(t, repo, "commit", "-m", "initial")

	got, err := Head(repo)
	if err != nil {
		t.Fatalf("Head() error = %v, want nil", err)
	}
	if len(got) != 40 {
		t.Fatalf("Head() length = %d, want 40; sha %q", len(got), got)
	}
}

func TestHeadErrorsOnRepoWithNoCommits(t *testing.T) {
	repo := initRepo(t)
	_, err := Head(repo)
	if !errors.Is(err, ErrNoCommits) {
		t.Fatalf("Head() error = %v, want ErrNoCommits", err)
	}
}

func TestInitRepoDisablesGlobalSigning(t *testing.T) {
	global := filepath.Join(t.TempDir(), "gitconfig")
	if err := os.WriteFile(global, []byte("[commit]\n\tgpgsign = true\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(global config) error = %v", err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", global)
	repo := initRepo(t)
	writeFile(t, repo, "README.md", "hello\n")
	git(t, repo, "add", "README.md")
	git(t, repo, "commit", "-m", "initial")
}

// initRepo creates a temporary Git repository with commit signing disabled.
func initRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	git(t, repo, "init")
	git(t, repo, "config", "user.email", "test@example.invalid")
	git(t, repo, "config", "user.name", "Test User")
	git(t, repo, "config", "commit.gpgsign", "false")
	return repo
}

func git(t *testing.T, repo string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v error = %v output = %s", args, err, out)
	}
}

func writeFile(t *testing.T, repo, name, content string) {
	t.Helper()
	path := filepath.Join(repo, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}
