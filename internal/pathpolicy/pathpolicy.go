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
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		resolved = candidate
	}
	if err := p.checkAbs(root, resolved); err != nil {
		return "", err
	}
	if p.forbidden(rel) {
		return "", fmt.Errorf("path %q is forbidden", rel)
	}
	return resolved, nil
}

func (p Policy) checkAbs(repoRoot, abs string) error {
	root, err := filepath.Abs(repoRoot)
	if err != nil {
		return fmt.Errorf("absolute repo root: %w", err)
	}
	path, err := filepath.Abs(abs)
	if err != nil {
		return fmt.Errorf("absolute path: %w", err)
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return fmt.Errorf("relative path: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("path %q escapes repo root %q", abs, repoRoot)
	}
	return nil
}

func (p Policy) forbidden(rel string) bool {
	slash := filepath.ToSlash(filepath.Clean(rel))
	lower := strings.ToLower(slash)
	for _, pattern := range p.Forbidden {
		switch pattern {
		case ".env":
			if lower == ".env" {
				return true
			}
		case ".env.*":
			if strings.HasPrefix(lower, ".env.") {
				return true
			}
		case "secrets/**":
			if lower == "secrets" || strings.HasPrefix(lower, "secrets/") {
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
