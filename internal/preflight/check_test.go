// Package preflight writes and validates quality stamps.
// Plan: WS4. PRD: FR-2.4, FR-7.1.
package preflight

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestCheckStampRejectsMissingStamp(t *testing.T) {
	repo := newRepo(t)
	_, err := CheckStamp(repo)
	if !errors.Is(err, ErrNoStamp) {
		t.Fatalf("CheckStamp() error = %v want ErrNoStamp", err)
	}
}

func TestCheckStampRejectsStaleHead(t *testing.T) {
	repo := newRepo(t)
	writeFile(t, repo, "docs/readme.md", "hello\n")
	runGit(t, repo, "add", "docs/readme.md")
	if _, err := Run(Context{Repo: repo, PipelinePreflight: true}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	runGit(t, repo, "add", "docs/readme.md")
	runGit(t, repo, "commit", "-q", "-m", "docs")
	_, err := CheckStamp(repo)
	assertStale(t, err)
}

func TestCheckStampRejectsStaleDiffHash(t *testing.T) {
	repo := newRepo(t)
	writeFile(t, repo, "docs/readme.md", "hello\n")
	runGit(t, repo, "add", "docs/readme.md")
	if _, err := Run(Context{Repo: repo, PipelinePreflight: true}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	writeFile(t, repo, "docs/readme.md", "changed\n")
	_, err := CheckStamp(repo)
	assertStale(t, err)
}

func TestCheckStampRejectsChangedFilesMismatch(t *testing.T) {
	repo := newRepo(t)
	writeFile(t, repo, "docs/readme.md", "hello\n")
	runGit(t, repo, "add", "docs/readme.md")
	stamp, err := Run(Context{Repo: repo, PipelinePreflight: true})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	stamp.ChangedFiles = []string{"docs/other.md"}
	data, err := stamp.Marshal()
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, stampRelPath), data, 0o600); err != nil {
		t.Fatalf("WriteFile(stamp) error = %v", err)
	}
	_, err = CheckStamp(repo)
	assertStale(t, err)
}

func TestCheckStampAcceptsFreshStamp(t *testing.T) {
	repo := newRepo(t)
	writeFile(t, repo, "docs/readme.md", "hello\n")
	runGit(t, repo, "add", "docs/readme.md")
	if _, err := Run(Context{Repo: repo, PipelinePreflight: true}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	stamp, err := CheckStamp(repo)
	if err != nil {
		t.Fatalf("CheckStamp() error = %v", err)
	}
	if stamp.Head == "" || stamp.DiffSHA256 == "" {
		t.Fatalf("CheckStamp() returned incomplete stamp: %+v", stamp)
	}
	if _, err := os.Stat(filepath.Join(repo, stampRelPath)); err != nil {
		t.Fatalf("stamp path missing: %v", err)
	}
}

func assertStale(t *testing.T, err error) {
	t.Helper()
	var stale ErrStaleStamp
	if !errors.As(err, &stale) {
		t.Fatalf("error = %v want ErrStaleStamp", err)
	}
}
