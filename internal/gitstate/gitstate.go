// Package gitstate inspects local Git repository state.
// Plan: WS1. PRD: FR-2.4, FR-7.1.
package gitstate

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ErrNoCommits reports a repository with no HEAD commit.
var ErrNoCommits = errors.New("repository has no commits")

// DetectRoot walks upward from start until it finds a Git repository marker.
func DetectRoot(start string) (string, error) {
	current, err := filepath.Abs(start)
	if err != nil {
		return "", fmt.Errorf("absolute start: %w", err)
	}
	info, err := os.Stat(current)
	if err != nil {
		return "", fmt.Errorf("stat start: %w", err)
	}
	if !info.IsDir() {
		current = filepath.Dir(current)
	}
	for {
		if _, err := os.Stat(filepath.Join(current, ".git")); err == nil {
			return current, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", fmt.Errorf("git root not found from %q", start)
		}
		current = parent
	}
}

// Head returns the repository HEAD commit SHA.
func Head(repo string) (string, error) {
	cmd := exec.Command("git", "-C", repo, "rev-parse", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && strings.Contains(string(exitErr.Stderr), "ambiguous argument 'HEAD'") {
			return "", ErrNoCommits
		}
		return "", fmt.Errorf("git rev-parse HEAD: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// IndexTree returns the tree object currently staged in the Git index.
func IndexTree(repo string) (string, error) {
	return gitOutput(repo, "write-tree")
}

// CommitTree returns the tree object for commit.
func CommitTree(repo, commit string) (string, error) {
	return gitOutput(repo, "rev-parse", commit+"^{tree}")
}

// CommitParent returns commit's first parent. It rejects root commits because
// they cannot be promotions of a pre-existing HEAD subject.
func CommitParent(repo, commit string) (string, error) {
	return gitOutput(repo, "rev-parse", commit+"^")
}

// ResolveRef returns the commit ID named by a local ref or revision expression.
func ResolveRef(repo, ref string) (string, error) {
	return gitOutput(repo, "rev-parse", "--verify", ref+"^{commit}")
}

func gitOutput(repo string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(out)), nil
}
