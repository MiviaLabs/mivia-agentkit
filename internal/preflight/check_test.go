// Package preflight writes and validates quality stamps.
// Plan: WS4. PRD: FR-2.4, FR-7.1.
package preflight

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/MiviaLabs/mivia-agentkit/internal/gitstate"
)

func TestCheckStampRejectsMissingStamp(t *testing.T) {
	repo := newRepo(t)
	_, err := CheckStamp(repo)
	if !errors.Is(err, ErrNoStamp) {
		t.Fatalf("CheckStamp() error = %v want ErrNoStamp", err)
	}
}

func TestStampPromotesFromValidatedIndexToMatchingCommit(t *testing.T) {
	repo := newRepo(t)
	writeFile(t, repo, "docs/readme.md", "hello\n")
	runGit(t, repo, "add", "docs/readme.md")
	if _, err := Run(Context{Repo: repo, BroadVerifiers: []string{"true"}, PipelinePreflight: true}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	runGit(t, repo, "add", "docs/readme.md")
	runGit(t, repo, "commit", "-q", "-m", "docs")
	stamp, err := CheckStamp(repo)
	if err != nil {
		t.Fatalf("CheckStamp() error = %v", err)
	}
	if stamp.Subject.Commit == "" {
		t.Fatal("CheckStamp() did not promote matching commit")
	}
}

func TestCheckStampKeepsIndexEvidenceValidWithUnstagedChanges(t *testing.T) {
	repo := newRepo(t)
	writeFile(t, repo, "docs/readme.md", "hello\n")
	runGit(t, repo, "add", "docs/readme.md")
	if _, err := Run(Context{Repo: repo, BroadVerifiers: []string{"true"}, PipelinePreflight: true}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	writeFile(t, repo, "docs/readme.md", "changed\n")
	if _, err := CheckStamp(repo); err != nil {
		t.Fatalf("CheckStamp() error = %v; unstaged content must not invalidate index evidence", err)
	}
}

func TestCheckStampRejectsForgedEvidence(t *testing.T) {
	repo := newRepo(t)
	writeFile(t, repo, "docs/readme.md", "hello\n")
	runGit(t, repo, "add", "docs/readme.md")
	stamp, err := Run(Context{Repo: repo, BroadVerifiers: []string{"true"}, PipelinePreflight: true})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	stamp.VerifierEvidence[0].SubjectHash = "forged"
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

func TestCheckStampRejectsFutureAndDuplicateEvidence(t *testing.T) {
	repo := newRepo(t)
	writeFile(t, repo, "docs/readme.md", "hello\n")
	runGit(t, repo, "add", "docs/readme.md")
	stamp, err := Run(Context{Repo: repo, BroadVerifiers: []string{"true"}})
	if err != nil {
		t.Fatal(err)
	}
	stamp.VerifierEvidence[0].StartedAt = time.Now().UTC().Add(time.Minute).Format(time.RFC3339)
	data, err := stamp.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, stampRelPath), data, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = CheckStamp(repo)
	assertStale(t, err)
}

func TestCheckStampRejectsDuplicateEvidence(t *testing.T) {
	repo := newRepo(t)
	writeFile(t, repo, "docs/readme.md", "hello\n")
	runGit(t, repo, "add", "docs/readme.md")
	stamp, err := Run(Context{Repo: repo, BroadVerifiers: []string{"true"}})
	if err != nil {
		t.Fatal(err)
	}
	stamp.VerifierEvidence = append(stamp.VerifierEvidence, stamp.VerifierEvidence[0])
	writeFile(t, repo, "mivia-agent.yaml", "quality:\n  required_verifiers: ['true', 'true']\n  verifiers:\n    'true':\n      command: ['true']\n")
	data, err := stamp.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, stampRelPath), data, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = CheckStamp(repo)
	assertStale(t, err)
}

func TestCheckStampAcceptsFreshStamp(t *testing.T) {
	repo := newRepo(t)
	writeFile(t, repo, "docs/readme.md", "hello\n")
	runGit(t, repo, "add", "docs/readme.md")
	if _, err := Run(Context{Repo: repo, BroadVerifiers: []string{"true"}, PipelinePreflight: true}); err != nil {
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

func TestPushRejectsUnvalidatedAdditionalCommit(t *testing.T) {
	repo := newRepo(t)
	writeFile(t, repo, "docs/readme.md", "first\n")
	runGit(t, repo, "add", "docs/readme.md")
	if _, err := Run(Context{Repo: repo, BroadVerifiers: []string{"true"}}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	runGit(t, repo, "commit", "-q", "-m", "first")
	if _, err := CheckStamp(repo); err != nil {
		t.Fatalf("CheckStamp() promotion error = %v", err)
	}
	writeFile(t, repo, "docs/second.md", "second\n")
	runGit(t, repo, "add", "docs/second.md")
	runGit(t, repo, "commit", "-q", "-m", "second")
	_, err := CheckStamp(repo)
	assertStale(t, err)
}

func TestValidateCommitCandidateRejectsWrongParentWithSameTree(t *testing.T) {
	repo := newRepo(t)
	writeFile(t, repo, "docs/a.md", "a\n")
	runGit(t, repo, "add", "docs/a.md")
	stamp, err := Run(Context{Repo: repo, BroadVerifiers: []string{"true"}})
	if err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "commit", "-q", "-m", "a")
	first, err := gitstate.Head(repo)
	if err != nil {
		t.Fatal(err)
	}
	stamp.Subject.BaseHead = first
	if err := validateCommitCandidate(repo, stamp, first); err == nil {
		t.Fatal("validateCommitCandidate() error = nil; want wrong parent")
	}
}

func TestValidateCommitCandidateRejectsSameParentWrongTree(t *testing.T) {
	repo := newRepo(t)
	writeFile(t, repo, "docs/a.md", "a\n")
	runGit(t, repo, "add", "docs/a.md")
	stamp, err := Run(Context{Repo: repo, BroadVerifiers: []string{"true"}})
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, repo, "docs/a.md", "different\n")
	runGit(t, repo, "add", "docs/a.md")
	runGit(t, repo, "commit", "-q", "-m", "different")
	head, err := gitstate.Head(repo)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateCommitCandidate(repo, stamp, head); err == nil {
		t.Fatal("validateCommitCandidate() error = nil; want wrong tree")
	}
}

func assertStale(t *testing.T, err error) {
	t.Helper()
	var stale ErrStaleStamp
	if !errors.As(err, &stale) {
		t.Fatalf("error = %v want ErrStaleStamp", err)
	}
}
