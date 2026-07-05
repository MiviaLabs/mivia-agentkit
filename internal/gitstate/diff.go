// Package gitstate inspects local Git repository state.
// Plan: WS1. PRD: FR-2.4.
package gitstate

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// ChangedFiles returns tracked modified, staged, and untracked files.
func ChangedFiles(repo string) ([]string, error) {
	out, err := exec.Command("git", "-C", repo, "status", "--porcelain=v1", "-z").Output()
	if err != nil {
		return nil, fmt.Errorf("git status: %w", err)
	}
	seen := map[string]struct{}{}
	entries := strings.Split(string(out), "\x00")
	for i := 0; i < len(entries); i++ {
		entry := entries[i]
		if entry == "" {
			continue
		}
		if len(entry) < 4 {
			return nil, fmt.Errorf("malformed git status entry %q", entry)
		}
		path := entry[3:]
		if entry[0] == 'R' || entry[1] == 'R' {
			i++
		}
		seen[filepath.ToSlash(path)] = struct{}{}
	}
	files := make([]string, 0, len(seen))
	for file := range seen {
		files = append(files, file)
	}
	sort.Strings(files)
	return files, nil
}

// DiffHash returns a stable hash for file path, status, and working-tree content.
func DiffHash(repo string, files []string) (string, error) {
	statuses, err := fileStatuses(repo)
	if err != nil {
		return "", err
	}
	sorted := append([]string(nil), files...)
	sort.Strings(sorted)
	h := sha256.New()
	for _, file := range sorted {
		status := statuses[filepath.ToSlash(file)]
		if status == "" {
			status = "??"
		}
		content, err := os.ReadFile(filepath.Join(repo, filepath.FromSlash(file)))
		if err != nil && !os.IsNotExist(err) {
			return "", fmt.Errorf("read %s: %w", file, err)
		}
		h.Write([]byte(file))
		h.Write([]byte{0})
		h.Write([]byte(status))
		h.Write([]byte{0})
		h.Write(content)
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func fileStatuses(repo string) (map[string]string, error) {
	out, err := exec.Command("git", "-C", repo, "status", "--porcelain=v1", "-z").Output()
	if err != nil {
		return nil, fmt.Errorf("git status: %w", err)
	}
	statuses := map[string]string{}
	entries := strings.Split(string(out), "\x00")
	for i := 0; i < len(entries); i++ {
		entry := entries[i]
		if entry == "" {
			continue
		}
		if len(entry) < 4 {
			return nil, fmt.Errorf("malformed git status entry %q", entry)
		}
		path := entry[3:]
		if entry[0] == 'R' || entry[1] == 'R' {
			i++
		}
		statuses[filepath.ToSlash(path)] = entry[:2]
	}
	return statuses, nil
}
