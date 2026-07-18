// Package gitstate inspects local Git repository state and performs scoped commits.
// Plan: WS15. PRD: FR-2.4, protected commit boundary.
package gitstate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ErrScopedCommit is returned when CommitScoped rejects unsafe state.
var ErrScopedCommit = errors.New("scoped commit rejected")

// CommitRequest is the coordinator input for a deterministic scoped commit.
type CommitRequest struct {
	// Repo is the owned worktree root (absolute or relative).
	Repo string
	// AllowedPaths are repo-relative paths to stage (never -A).
	AllowedPaths []string
	// Message is the validated commit message.
	Message string
	// Verifier is an argv array executed with no shell (empty skips).
	Verifier []string
	// VerifierTimeout bounds verifier execution.
	VerifierTimeout time.Duration
	// BaseHead is the expected HEAD before commit.
	BaseHead string
	// StampWriter writes a fresh stamp after staging; may be nil in tests that inject StampOK.
	StampCheck func(repo, head, indexHash string, paths []string) error
	// PolicyCheck decides protect:commit; may be nil when PolicyOK is forced.
	PolicyCheck func(repo, head, indexHash string) error
}

// CommitResult is safe commit metadata.
type CommitResult struct {
	SHA         string
	IndexHash   string
	StagedPaths []string
}

// CommitScoped stages only allowlisted paths, runs verifier, stamp, policy, and commits once.
func CommitScoped(ctx context.Context, req CommitRequest) (CommitResult, error) {
	if req.Repo == "" {
		return CommitResult{}, fmt.Errorf("%w: repo required", ErrScopedCommit)
	}
	if len(req.AllowedPaths) == 0 {
		return CommitResult{}, fmt.Errorf("%w: allowed paths required", ErrScopedCommit)
	}
	if strings.TrimSpace(req.Message) == "" || strings.ContainsAny(req.Message, "\n\r") {
		return CommitResult{}, fmt.Errorf("%w: invalid message", ErrScopedCommit)
	}
	// Fail closed: stamp and policy gates are required for production callers.
	// Tests must inject explicit no-op checks rather than omitting them.
	if req.StampCheck == nil {
		return CommitResult{}, fmt.Errorf("%w: stamp check required", ErrScopedCommit)
	}
	if req.PolicyCheck == nil {
		return CommitResult{}, fmt.Errorf("%w: policy check required", ErrScopedCommit)
	}
	repo, err := filepath.Abs(req.Repo)
	if err != nil {
		return CommitResult{}, err
	}

	// Reject merge/rebase/cherry-pick and dirty unrelated state.
	if err := assertCleanOwnedWorktree(repo, req.AllowedPaths); err != nil {
		return CommitResult{}, err
	}
	head, err := Head(repo)
	if err != nil {
		return CommitResult{}, err
	}
	if req.BaseHead != "" && head != req.BaseHead {
		return CommitResult{}, fmt.Errorf("%w: head mismatch got %s want %s", ErrScopedCommit, head, req.BaseHead)
	}

	// Normalize and deny dangerous paths.
	paths := make([]string, 0, len(req.AllowedPaths))
	for _, p := range req.AllowedPaths {
		np, err := normalizeScopedPath(p)
		if err != nil {
			return CommitResult{}, fmt.Errorf("%w: %v", ErrScopedCommit, err)
		}
		paths = append(paths, np)
	}

	// Stage only allowlisted paths (literal pathspecs; globs rejected in normalize).
	// Never use git add -A.
	args := append([]string{"-C", repo, "add", "--"}, paths...)
	if _, err := exec.CommandContext(ctx, "git", args...).CombinedOutput(); err != nil {
		return CommitResult{}, fmt.Errorf("%w: git add: %v", ErrScopedCommit, err)
	}

	// Post-stage: reject any staged path that is a secret or outside the literal allowlist.
	if err := assertStagedPathsSafe(ctx, repo, paths); err != nil {
		return CommitResult{}, err
	}

	indexHash, err := indexTreeHash(ctx, repo)
	if err != nil {
		return CommitResult{}, err
	}

	// Verifier as argv only; empty verifier fails closed.
	if len(req.Verifier) == 0 {
		return CommitResult{}, fmt.Errorf("%w: verifier required", ErrScopedCommit)
	}
	timeout := req.VerifierTimeout
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	vctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(vctx, req.Verifier[0], req.Verifier[1:]...)
	cmd.Dir = repo
	cmd.Env = safeEnv()
	if _, err := cmd.CombinedOutput(); err != nil {
		// Do not embed raw verifier stdout/stderr (may contain secrets).
		return CommitResult{}, fmt.Errorf("%w: verifier failed", ErrScopedCommit)
	}

	if err := req.StampCheck(repo, head, indexHash, paths); err != nil {
		return CommitResult{}, fmt.Errorf("%w: stamp: %v", ErrScopedCommit, err)
	}
	if err := req.PolicyCheck(repo, head, indexHash); err != nil {
		return CommitResult{}, fmt.Errorf("%w: policy: %v", ErrScopedCommit, err)
	}

	// Ensure index still matches accepted hash (detect mutation after stamp).
	after, err := indexTreeHash(ctx, repo)
	if err != nil {
		return CommitResult{}, err
	}
	if after != indexHash {
		return CommitResult{}, fmt.Errorf("%w: index mutated after stamp", ErrScopedCommit)
	}

	commitArgs := []string{"-C", repo, "commit", "--no-verify", "-m", req.Message, "--"}
	// commit only what is staged; do not embed raw git output (may leak paths/env).
	if _, err := exec.CommandContext(ctx, "git", commitArgs...).CombinedOutput(); err != nil {
		return CommitResult{}, fmt.Errorf("%w: commit: %v", ErrScopedCommit, err)
	}
	newHead, err := Head(repo)
	if err != nil {
		return CommitResult{}, err
	}
	if newHead == head {
		return CommitResult{}, fmt.Errorf("%w: head did not advance", ErrScopedCommit)
	}
	return CommitResult{SHA: newHead, IndexHash: indexHash, StagedPaths: paths}, nil
}

// assertStagedPathsSafe ensures every staged file is under a literal allowlisted
// path and is not a secret-like path (.env, secrets/**, private keys).
func assertStagedPathsSafe(ctx context.Context, repo string, allowed []string) error {
	out, err := exec.CommandContext(ctx, "git", "-C", repo, "diff", "--cached", "--name-only", "-z").Output()
	if err != nil {
		return fmt.Errorf("%w: list staged: %v", ErrScopedCommit, err)
	}
	allow := map[string]struct{}{}
	for _, a := range allowed {
		allow[a] = struct{}{}
	}
	for _, raw := range strings.Split(string(out), "\x00") {
		if raw == "" {
			continue
		}
		p := filepath.ToSlash(raw)
		covered := false
		for a := range allow {
			if p == a || strings.HasPrefix(p, a+"/") {
				covered = true
				break
			}
		}
		if !covered {
			return fmt.Errorf("%w: staged path %q outside allowlist", ErrScopedCommit, p)
		}
		if _, err := normalizeScopedPath(p); err != nil {
			return fmt.Errorf("%w: staged path %q: %v", ErrScopedCommit, p, err)
		}
	}
	return nil
}

func normalizeScopedPath(p string) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" {
		return "", errors.New("empty path")
	}
	if strings.HasPrefix(p, "/") || strings.HasPrefix(p, "~") || strings.Contains(p, "..") {
		return "", fmt.Errorf("unsafe path %q", p)
	}
	// Literal paths only — pathspec globs expand staging beyond intended scope.
	if strings.ContainsAny(p, "*?[]") || strings.Contains(p, ":(") {
		return "", fmt.Errorf("unsafe path %q: globs not allowed", p)
	}
	clean := filepath.ToSlash(filepath.Clean(p))
	if clean == ".git" || strings.HasPrefix(clean, ".git/") || clean == ".ai/runs" || strings.HasPrefix(clean, ".ai/runs/") {
		return "", fmt.Errorf("denied path %q", p)
	}
	for _, seg := range strings.Split(clean, "/") {
		if seg == ".git" {
			return "", fmt.Errorf("denied path %q", p)
		}
	}
	// Secret-aware deny list (aligned with pathpolicy defaults).
	base := strings.ToLower(filepath.Base(clean))
	lower := strings.ToLower(clean)
	if base == ".env" || strings.HasPrefix(base, ".env.") {
		return "", fmt.Errorf("denied path %q", p)
	}
	if lower == "secrets" || strings.HasPrefix(lower, "secrets/") ||
		strings.Contains(lower, "/secrets/") || strings.HasSuffix(lower, "/secrets") {
		return "", fmt.Errorf("denied path %q", p)
	}
	if strings.Contains(lower, "private") && strings.Contains(lower, "key") {
		return "", fmt.Errorf("denied path %q", p)
	}
	return clean, nil
}

func assertCleanOwnedWorktree(repo string, allowed []string) error {
	// Reject merge/rebase/cherry-pick markers.
	for _, p := range []string{
		".git/MERGE_HEAD", ".git/rebase-merge", ".git/rebase-apply", ".git/CHERRY_PICK_HEAD",
	} {
		if _, err := os.Stat(filepath.Join(repo, p)); err == nil {
			return fmt.Errorf("%w: merge/rebase/cherry-pick in progress", ErrScopedCommit)
		}
	}
	// Respect .gitignore so ignored coordinator state (.ai/runs/, fixtures) does not block.
	out, err := exec.Command("git", "-C", repo, "status", "--porcelain", "--untracked-files=all").Output()
	if err != nil {
		return fmt.Errorf("git status: %w", err)
	}
	allow := map[string]struct{}{}
	for _, p := range allowed {
		np, err := normalizeScopedPath(p)
		if err != nil {
			return err
		}
		allow[np] = struct{}{}
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		// porcelain: XY PATH or XY ORIG -> PATH
		if len(line) < 4 {
			continue
		}
		path := strings.TrimSpace(line[3:])
		if i := strings.Index(path, " -> "); i >= 0 {
			path = path[i+4:]
		}
		path = filepath.ToSlash(strings.Trim(path, "\""))
		// Directory entries from untracked-files=all end with '/'.
		path = strings.TrimSuffix(path, "/")
		if path == "" {
			continue
		}
		// Coordinator-owned run state is never commit scope and must not block
		// CommitScoped even when a target repo lacks .gitignore entries.
		if path == ".ai/runs" || strings.HasPrefix(path, ".ai/runs/") {
			continue
		}
		if path == ".ai/campaign-fixtures" || strings.HasPrefix(path, ".ai/campaign-fixtures/") {
			continue
		}
		if _, ok := allow[path]; ok {
			continue
		}
		// Prefix match: allowed "internal/foo" covers "internal/foo/bar.go".
		covered := false
		for a := range allow {
			if path == a || strings.HasPrefix(path, a+"/") {
				covered = true
				break
			}
		}
		if covered {
			continue
		}
		return fmt.Errorf("%w: unrelated dirty path %q", ErrScopedCommit, path)
	}
	return nil
}

func indexTreeHash(ctx context.Context, repo string) (string, error) {
	out, err := exec.CommandContext(ctx, "git", "-C", repo, "write-tree").Output()
	if err != nil {
		return "", fmt.Errorf("write-tree: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func safeEnv() []string {
	// Minimal env for verifiers; no secrets injected.
	return []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + os.Getenv("HOME"),
		"TMPDIR=" + os.TempDir(),
		"LANG=C",
	}
}

// WorktreeRef derives a deterministic branch name from campaign ID.
func WorktreeRef(campaignID string) string {
	sum := sha256.Sum256([]byte(campaignID))
	return "mivia-campaign/" + hex.EncodeToString(sum[:8])
}
