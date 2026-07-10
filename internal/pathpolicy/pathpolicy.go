// Package pathpolicy enforces local path safety rules.
// Plan: WS1. PRD: FR-7.5.
package pathpolicy

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
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
	if err := p.checkRelative(rel); err != nil {
		return err
	}
	_, err := p.Abs(repoRoot, rel)
	return err
}

// Abs resolves rel under repoRoot and verifies it remains inside repoRoot.
func (p Policy) Abs(repoRoot, rel string) (string, error) {
	return p.resolveWritePath(repoRoot, rel)
}

// ResolveWritePath returns a repo-contained path only when every existing
// component is a real directory or regular file, never a symlink.
func ResolveWritePath(repoRoot, rel string) (string, error) {
	p := NewDefault()
	return p.resolveWritePath(repoRoot, rel)
}

func (p Policy) resolveWritePath(repoRoot, rel string) (string, error) {
	if err := p.checkRelative(rel); err != nil {
		return "", err
	}
	root, rootPath, err := openRoot(repoRoot)
	if err != nil {
		return "", err
	}
	defer root.Close()
	if err := rejectExistingSymlinks(root, rel); err != nil {
		return "", err
	}
	return filepath.Join(rootPath, filepath.Clean(rel)), nil
}

// EnsureDir creates a repo-relative directory without traversing symlinks.
func EnsureDir(repoRoot, rel string) error {
	p := NewDefault()
	if err := p.checkRelative(rel); err != nil {
		return err
	}
	root, _, err := openRoot(repoRoot)
	if err != nil {
		return err
	}
	defer root.Close()
	if err := rejectExistingSymlinks(root, rel); err != nil {
		return err
	}
	if err := root.MkdirAll(filepath.Clean(rel), 0o755); err != nil {
		return fmt.Errorf("create directory %q: %w", rel, err)
	}
	if err := rejectExistingSymlinks(root, rel); err != nil {
		return err
	}
	return nil
}

// CreateDirExclusive creates a repo-relative directory without following
// symlinks and rejects an existing destination.
func CreateDirExclusive(repoRoot, rel string) error {
	p := NewDefault()
	if err := p.checkRelative(rel); err != nil {
		return err
	}
	root, _, err := openRoot(repoRoot)
	if err != nil {
		return err
	}
	defer root.Close()
	parent := filepath.Dir(rel)
	if err := rejectExistingSymlinks(root, parent); err != nil {
		return err
	}
	if err := root.MkdirAll(parent, 0o755); err != nil {
		return fmt.Errorf("create parent directory %q: %w", parent, err)
	}
	if err := rejectExistingSymlinks(root, parent); err != nil {
		return err
	}
	if err := root.Mkdir(filepath.Clean(rel), 0o755); err != nil {
		return fmt.Errorf("create exclusive directory %q: %w", rel, err)
	}
	return nil
}

// WriteFile atomically writes data to a repo-relative file without following
// symlinks in the destination path.
func WriteFile(repoRoot, rel string, data []byte, mode fs.FileMode) error {
	p := NewDefault()
	if err := p.checkRelative(rel); err != nil {
		return err
	}
	root, _, err := openRoot(repoRoot)
	if err != nil {
		return err
	}
	defer root.Close()
	parent := filepath.Dir(rel)
	if err := rejectExistingSymlinks(root, parent); err != nil {
		return err
	}
	if err := root.MkdirAll(parent, 0o755); err != nil {
		return fmt.Errorf("create destination directory: %w", err)
	}
	if err := rejectExistingSymlinks(root, parent); err != nil {
		return err
	}
	if info, err := root.Lstat(rel); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("path %q is a symlink", rel)
		}
		if info.IsDir() {
			return fmt.Errorf("path %q is a directory", rel)
		}
		mode = info.Mode().Perm()
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect destination %q: %w", rel, err)
	}
	tmpName, err := temporaryName(rel)
	if err != nil {
		return err
	}
	tmp, err := root.OpenFile(tmpName, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return fmt.Errorf("create temporary file: %w", err)
	}
	defer root.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write temporary file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("sync temporary file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temporary file: %w", err)
	}
	if err := root.Rename(tmpName, rel); err != nil {
		return fmt.Errorf("replace destination: %w", err)
	}
	return nil
}

// AppendFile appends one record through a root-anchored append descriptor.
func AppendFile(repoRoot, rel string, data []byte, mode fs.FileMode) error {
	p := NewDefault()
	if err := p.checkRelative(rel); err != nil {
		return err
	}
	root, _, err := openRoot(repoRoot)
	if err != nil {
		return err
	}
	defer root.Close()
	parent := filepath.Dir(rel)
	if err := rejectExistingSymlinks(root, parent); err != nil {
		return err
	}
	if err := root.MkdirAll(parent, 0o755); err != nil {
		return fmt.Errorf("create append directory: %w", err)
	}
	if err := rejectExistingSymlinks(root, parent); err != nil {
		return err
	}
	if info, err := root.Lstat(rel); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("path %q is a symlink", rel)
	} else if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("inspect append destination %q: %w", rel, err)
	}
	f, err := root.OpenFile(rel, os.O_CREATE|os.O_APPEND|os.O_WRONLY, mode)
	if err != nil {
		return fmt.Errorf("open append destination: %w", err)
	}
	defer f.Close()
	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("append file: %w", err)
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("sync appended file: %w", err)
	}
	return nil
}

// ReadFile reads a repo-relative file through a root-anchored descriptor.
func ReadFile(repoRoot, rel string) ([]byte, error) {
	p := NewDefault()
	if err := p.checkRelative(rel); err != nil {
		return nil, err
	}
	root, _, err := openRoot(repoRoot)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	if err := rejectExistingSymlinks(root, rel); err != nil {
		return nil, err
	}
	data, err := root.ReadFile(rel)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func (p Policy) checkRelative(rel string) error {
	if filepath.IsAbs(rel) {
		return fmt.Errorf("absolute path %q is not repo-relative", rel)
	}
	for _, part := range strings.FieldsFunc(rel, func(r rune) bool { return r == '/' || r == '\\' }) {
		if part == ".." {
			return fmt.Errorf("path %q traverses outside repo", rel)
		}
	}
	if p.forbidden(rel) {
		return fmt.Errorf("path %q is forbidden", rel)
	}
	return nil
}

func openRoot(repoRoot string) (*os.Root, string, error) {
	root, err := filepath.Abs(repoRoot)
	if err != nil {
		return nil, "", fmt.Errorf("absolute repo root: %w", err)
	}
	info, err := os.Stat(root)
	if err != nil {
		return nil, "", fmt.Errorf("inspect repo root: %w", err)
	}
	if !info.IsDir() {
		return nil, "", fmt.Errorf("repo root %q must be a directory", repoRoot)
	}
	opened, err := os.OpenRoot(root)
	if err != nil {
		return nil, "", fmt.Errorf("open repo root: %w", err)
	}
	return opened, root, nil
}

func rejectExistingSymlinks(root *os.Root, rel string) error {
	path := ""
	for _, part := range strings.Split(filepath.Clean(rel), string(filepath.Separator)) {
		if part == "." || part == "" {
			continue
		}
		path = filepath.Join(path, part)
		info, err := root.Lstat(path)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("inspect path component %q: %w", path, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("path %q escapes root through symlink component %q", rel, path)
		}
	}
	return nil
}

func temporaryName(rel string) (string, error) {
	var random [12]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", fmt.Errorf("generate temporary file name: %w", err)
	}
	return filepath.Join(filepath.Dir(rel), ".mivia-agent-"+hex.EncodeToString(random[:])), nil
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
