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
	if _, err := DetectRoot(t.TempDir()); err == nil {
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

func initRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	git(t, repo, "init")
	git(t, repo, "config", "user.email", "test@example.invalid")
	git(t, repo, "config", "user.name", "Test User")
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
