// Package pathpolicy enforces local path safety rules.
// Plan: WS1. PRD: FR-7.5.
package pathpolicy

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Policy checks repo-relative paths.
type Policy struct {
	Forbidden []string
}

// NewDefault returns the default secret-aware path policy.
func NewDefault() Policy {
	return Policy{Forbidden: []string{".env", ".env.*", "secrets/**", "**/*private*key*"}}
}

// Check validates rel under repoRoot.
func (p Policy) Check(repoRoot, rel string) error {
	if filepath.IsAbs(rel) {
		return p.checkAbs(repoRoot, rel)
	}
	for _, part := range strings.FieldsFunc(rel, func(r rune) bool {
		return r == '/' || r == '\\'
	}) {
		if part == ".." {
			return fmt.Errorf("path %q traverses outside repo", rel)
		}
	}
	clean := filepath.Clean(rel)
	if p.forbidden(clean) {
		return fmt.Errorf("path %q is forbidden", rel)
	}
	_, err := p.Abs(repoRoot, rel)
	return err
}

// Abs resolves rel under repoRoot and verifies it remains inside repoRoot.
func (p Policy) Abs(repoRoot, rel string) (string, error) {
	if filepath.IsAbs(rel) {
		return "", fmt.Errorf("absolute path %q is not repo-relative", rel)
	}
	root, err := filepath.Abs(repoRoot)
	if err != nil {
		return "", fmt.Errorf("absolute repo root: %w", err)
	}
	candidate := filepath.Join(root, filepath.Clean(rel))
	resolved := resolveExistingPrefix(candidate)
	if err := p.checkAbs(root, resolved); err != nil {
		return "", err
	}
	if p.forbidden(rel) {
		return "", fmt.Errorf("path %q is forbidden", rel)
	}
	return resolved, nil
}

// checkAbs verifies abs resolves to a location inside repoRoot. Both sides
// are canonicalized identically before comparing: a path that exists may
// reach repoRoot only through a symlink (macOS commonly aliases the temp
// directory root, e.g. /var -> /private/var) or an OS short-name form
// (Windows temp paths can surface as either the 8.3 alias, e.g. RUNNER~1,
// or the long form). Comparing an unresolved root against a resolved leaf
// (or vice versa) makes every path look like it escapes.
func (p Policy) checkAbs(repoRoot, abs string) error {
	root, err := filepath.Abs(repoRoot)
	if err != nil {
		return fmt.Errorf("absolute repo root: %w", err)
	}
	root = resolveExistingPrefix(root)
	path, err := filepath.Abs(abs)
	if err != nil {
		return fmt.Errorf("absolute path: %w", err)
	}
	path = resolveExistingPrefix(path)
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return fmt.Errorf("relative path: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("path %q escapes repo root %q", abs, repoRoot)
	}
	return nil
}

// resolveExistingPrefix returns the canonical form of path, resolving
// symlinks (and, on Windows, short-name aliases) through the longest
// existing ancestor. Segments that do not exist yet — e.g. a file about to
// be written — are preserved verbatim and rejoined onto the resolved
// ancestor, so a not-yet-created leaf still compares consistently against
// an already-resolved repo root.
func resolveExistingPrefix(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	dir := filepath.Dir(path)
	if dir == path {
		return path
	}
	return filepath.Join(resolveExistingPrefix(dir), filepath.Base(path))
}

func (p Policy) forbidden(rel string) bool {
	slash := filepath.ToSlash(filepath.Clean(rel))
	lower := strings.ToLower(slash)
	base := filepath.Base(lower)
	for _, pattern := range p.Forbidden {
		switch pattern {
		case ".env":
			// Match .env at any depth (config/.env, .ai/.env), not only repo root.
			if base == ".env" {
				return true
			}
		case ".env.*":
			// Match .env.* basenames at any depth (.env.local, app/.env.production).
			if strings.HasPrefix(base, ".env.") {
				return true
			}
		case "secrets/**":
			// Match a secrets directory segment at any depth.
			if lower == "secrets" || strings.HasPrefix(lower, "secrets/") ||
				strings.Contains(lower, "/secrets/") || strings.HasSuffix(lower, "/secrets") {
				return true
			}
		case "**/*private*key*":
			if strings.Contains(lower, "private") && strings.Contains(lower, "key") {
				return true
			}
		}
	}
	return false
}
