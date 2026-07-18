// Package gitstate tests scoped commits with real git.
// Plan: WS15.
package gitstate

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func initTempRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	run("init")
	run("config", "user.email", "t@example.com")
	run("config", "user.name", "t")
	if err := os.WriteFile(filepath.Join(dir, "README"), []byte("hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "README")
	run("commit", "-m", "init")
	return dir
}

func TestCommitScopedSuccess(t *testing.T) {
	dir := initTempRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	head, err := Head(dir)
	if err != nil {
		t.Fatal(err)
	}
	res, err := CommitScoped(context.Background(), CommitRequest{
		Repo:         dir,
		AllowedPaths: []string{"a.go"},
		Message:      "fix(quality): scoped",
		BaseHead:     head,
		Verifier:     []string{"true"},
		StampCheck:   func(string, string, string, []string) error { return nil },
		PolicyCheck:  func(string, string, string) error { return nil },
	})
	if err != nil {
		t.Fatalf("CommitScoped: %v", err)
	}
	if res.SHA == head {
		t.Fatalf("SHA did not advance")
	}
}

func TestCommitScopedRejectsDirtyUnrelated(t *testing.T) {
	dir := initTempRepo(t)
	_ = os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a\n"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "noise.txt"), []byte("x\n"), 0o644)
	head, _ := Head(dir)
	_, err := CommitScoped(context.Background(), CommitRequest{
		Repo: dir, AllowedPaths: []string{"a.go"}, Message: "fix(quality): x", BaseHead: head,
		Verifier:    []string{"true"},
		StampCheck:  func(string, string, string, []string) error { return nil },
		PolicyCheck: func(string, string, string) error { return nil },
	})
	if err == nil || !strings.Contains(err.Error(), "unrelated dirty") {
		t.Fatalf("error = %v, want unrelated dirty", err)
	}
}

func TestCommitScopedRejectsDeniedPaths(t *testing.T) {
	dir := initTempRepo(t)
	_, err := CommitScoped(context.Background(), CommitRequest{
		Repo: dir, AllowedPaths: []string{".ai/runs/x"}, Message: "fix(quality): x",
		Verifier:    []string{"true"},
		StampCheck:  func(string, string, string, []string) error { return nil },
		PolicyCheck: func(string, string, string) error { return nil },
	})
	if err == nil || !strings.Contains(err.Error(), "denied") {
		t.Fatalf("error = %v, want denied", err)
	}
}

func TestCommitScopedRejectsBroadStagingBypass(t *testing.T) {
	// Ensure API never accepts empty allowlist (would encourage broad staging).
	_, err := CommitScoped(context.Background(), CommitRequest{
		Repo: t.TempDir(), AllowedPaths: nil, Message: "fix(quality): x",
		StampCheck:  func(string, string, string, []string) error { return nil },
		PolicyCheck: func(string, string, string) error { return nil },
	})
	if err == nil || !strings.Contains(err.Error(), "allowed paths") {
		t.Fatalf("error = %v, want allowed paths required", err)
	}
}

func TestCommitScopedRejectsStaleStampAndPolicyDenial(t *testing.T) {
	dir := initTempRepo(t)
	_ = os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a\n"), 0o644)
	head, _ := Head(dir)
	_, err := CommitScoped(context.Background(), CommitRequest{
		Repo: dir, AllowedPaths: []string{"a.go"}, Message: "fix(quality): x", BaseHead: head,
		Verifier:    []string{"true"},
		StampCheck:  func(string, string, string, []string) error { return errorsNew("stale stamp") },
		PolicyCheck: func(string, string, string) error { return nil },
	})
	if err == nil || !strings.Contains(err.Error(), "stamp") {
		t.Fatalf("error = %v, want stamp rejection", err)
	}
	_, err = CommitScoped(context.Background(), CommitRequest{
		Repo: dir, AllowedPaths: []string{"a.go"}, Message: "fix(quality): x", BaseHead: head,
		Verifier:    []string{"true"},
		StampCheck:  func(string, string, string, []string) error { return nil },
		PolicyCheck: func(string, string, string) error { return errorsNew("policy denied") },
	})
	if err == nil || !strings.Contains(err.Error(), "policy") {
		t.Fatalf("error = %v, want policy rejection", err)
	}
}

func TestCommitScopedRejectsMissingStampAndPolicyHooks(t *testing.T) {
	dir := initTempRepo(t)
	_ = os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a\n"), 0o644)
	head, _ := Head(dir)
	_, err := CommitScoped(context.Background(), CommitRequest{
		Repo: dir, AllowedPaths: []string{"a.go"}, Message: "fix(quality): x", BaseHead: head,
		Verifier:    []string{"true"},
		PolicyCheck: func(string, string, string) error { return nil },
	})
	if err == nil || !strings.Contains(err.Error(), "stamp check required") {
		t.Fatalf("error = %v, want stamp check required", err)
	}
	_, err = CommitScoped(context.Background(), CommitRequest{
		Repo: dir, AllowedPaths: []string{"a.go"}, Message: "fix(quality): x", BaseHead: head,
		Verifier:   []string{"true"},
		StampCheck: func(string, string, string, []string) error { return nil },
	})
	if err == nil || !strings.Contains(err.Error(), "policy check required") {
		t.Fatalf("error = %v, want policy check required", err)
	}
}

func TestCommitScopedRejectsEmptyVerifier(t *testing.T) {
	dir := initTempRepo(t)
	_ = os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a\n"), 0o644)
	head, _ := Head(dir)
	_, err := CommitScoped(context.Background(), CommitRequest{
		Repo: dir, AllowedPaths: []string{"a.go"}, Message: "fix(quality): x", BaseHead: head,
		StampCheck:  func(string, string, string, []string) error { return nil },
		PolicyCheck: func(string, string, string) error { return nil },
	})
	if err == nil || !strings.Contains(err.Error(), "verifier required") {
		t.Fatalf("error = %v, want verifier required", err)
	}
}

func TestCommitScopedRejectsFailedVerifier(t *testing.T) {
	dir := initTempRepo(t)
	_ = os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a\n"), 0o644)
	head, _ := Head(dir)
	_, err := CommitScoped(context.Background(), CommitRequest{
		Repo: dir, AllowedPaths: []string{"a.go"}, Message: "fix(quality): x", BaseHead: head,
		Verifier:    []string{"false"},
		StampCheck:  func(string, string, string, []string) error { return nil },
		PolicyCheck: func(string, string, string) error { return nil },
	})
	if err == nil || !strings.Contains(err.Error(), "verifier failed") {
		t.Fatalf("error = %v, want verifier failed", err)
	}
}

func TestCommitScopedRejectsGlobAndSecretPaths(t *testing.T) {
	dir := initTempRepo(t)
	head, _ := Head(dir)
	_, err := CommitScoped(context.Background(), CommitRequest{
		Repo: dir, AllowedPaths: []string{"*.go"}, Message: "fix(quality): x", BaseHead: head,
		Verifier:    []string{"true"},
		StampCheck:  func(string, string, string, []string) error { return nil },
		PolicyCheck: func(string, string, string) error { return nil },
	})
	if err == nil || !strings.Contains(err.Error(), "glob") {
		t.Fatalf("error = %v, want glob rejection", err)
	}
	_, err = CommitScoped(context.Background(), CommitRequest{
		Repo: dir, AllowedPaths: []string{".env"}, Message: "fix(quality): x", BaseHead: head,
		Verifier:    []string{"true"},
		StampCheck:  func(string, string, string, []string) error { return nil },
		PolicyCheck: func(string, string, string) error { return nil },
	})
	if err == nil || !strings.Contains(err.Error(), "denied") {
		t.Fatalf("error = %v, want denied secret path", err)
	}
}

func TestCommitScopedIgnoresUntrackedCampaignRunState(t *testing.T) {
	// Campaign store writes .ai/runs/<id>/; without .gitignore that would block
	// CommitScoped even though run state is never commit scope.
	dir := initTempRepo(t)
	if err := os.MkdirAll(filepath.Join(dir, ".ai", "runs", "camp-1"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".ai", "runs", "camp-1", "campaign-state.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	head, _ := Head(dir)
	res, err := CommitScoped(context.Background(), CommitRequest{
		Repo: dir, AllowedPaths: []string{"a.go"}, Message: "fix(quality): x", BaseHead: head,
		Verifier:    []string{"true"},
		StampCheck:  func(string, string, string, []string) error { return nil },
		PolicyCheck: func(string, string, string) error { return nil },
	})
	if err != nil {
		t.Fatalf("CommitScoped: %v", err)
	}
	if res.SHA == head {
		t.Fatal("HEAD did not advance")
	}
}

func TestCommitScopedRejectsStagedSecretUnderDirAllowlist(t *testing.T) {
	dir := initTempRepo(t)
	if err := os.MkdirAll(filepath.Join(dir, "internal"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "internal", "ok.go"), []byte("package internal\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "internal", ".env"), []byte("SECRET=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	head, _ := Head(dir)
	_, err := CommitScoped(context.Background(), CommitRequest{
		Repo: dir, AllowedPaths: []string{"internal"}, Message: "fix(quality): x", BaseHead: head,
		Verifier:    []string{"true"},
		StampCheck:  func(string, string, string, []string) error { return nil },
		PolicyCheck: func(string, string, string) error { return nil },
	})
	if err == nil || !strings.Contains(err.Error(), "denied") {
		t.Fatalf("error = %v, want denied staged secret under dir allowlist", err)
	}
}

func errorsNew(s string) error { return &simpleErr{s} }

type simpleErr struct{ s string }

func (e *simpleErr) Error() string { return e.s }

func TestWorktreeRefDeterministic(t *testing.T) {
	a := WorktreeRef("camp-1")
	b := WorktreeRef("camp-1")
	if a != b || !strings.HasPrefix(a, "mivia-campaign/") {
		t.Fatalf("WorktreeRef = %q / %q", a, b)
	}
}
