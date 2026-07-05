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
	out, err := exec.Command("git", "-C", repo, "status", "--porcelain=v1", "--untracked-files=all", "-z").Output()
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
	head, err := Head(repo)
	if err != nil {
		return "", err
	}
	entries := make([]statusEntry, 0, len(files))
	for _, file := range files {
		path := filepath.ToSlash(file)
		status := statuses[path]
		if status == "" {
			status = "??"
		}
		entries = append(entries, statusEntry{Path: path, Status: status})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Path != entries[j].Path {
			return entries[i].Path < entries[j].Path
		}
		return entries[i].Status < entries[j].Status
	})
	h := sha256.New()
	h.Write([]byte("head:" + head + "\n"))
	for _, entry := range entries {
		content, err := os.ReadFile(filepath.Join(repo, filepath.FromSlash(entry.Path)))
		if err != nil && !os.IsNotExist(err) {
			return "", fmt.Errorf("read %s: %w", entry.Path, err)
		}
		h.Write([]byte(entry.Status))
		h.Write([]byte{0})
		h.Write([]byte(entry.Path))
		h.Write([]byte{0})
		if os.IsNotExist(err) {
			h.Write([]byte("<missing>"))
		} else {
			h.Write(content)
		}
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

type statusEntry struct {
	Path   string
	Status string
}

func fileStatuses(repo string) (map[string]string, error) {
	out, err := exec.Command("git", "-C", repo, "status", "--porcelain=v1", "--untracked-files=all", "-z").Output()
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
