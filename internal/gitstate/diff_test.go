package gitstate

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestChangedFilesDetectsModifiedStagedUntracked(t *testing.T) {
	repo := initRepo(t)
	writeFile(t, repo, "modified.txt", "old\n")
	writeFile(t, repo, "staged.txt", "old\n")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "initial")
	writeFile(t, repo, "modified.txt", "new\n")
	writeFile(t, repo, "staged.txt", "new\n")
	writeFile(t, repo, "untracked.txt", "new\n")
	git(t, repo, "add", "staged.txt")

	got, err := ChangedFiles(repo)
	if err != nil {
		t.Fatalf("ChangedFiles() error = %v, want nil", err)
	}
	want := []string{"modified.txt", "staged.txt", "untracked.txt"}
	if !equalStrings(got, want) {
		t.Fatalf("ChangedFiles() = %#v, want %#v", got, want)
	}
}

func TestChangedFilesHandlesRenames(t *testing.T) {
	repo := initRepo(t)
	writeFile(t, repo, "old.txt", "content\n")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "initial")
	git(t, repo, "mv", "old.txt", "new.txt")

	got, err := ChangedFiles(repo)
	if err != nil {
		t.Fatalf("ChangedFiles() error = %v, want nil", err)
	}
	want := []string{"new.txt"}
	if !equalStrings(got, want) {
		t.Fatalf("ChangedFiles() = %#v, want %#v", got, want)
	}
}

func TestDiffHashStableForIdenticalContent(t *testing.T) {
	repo := initRepo(t)
	git(t, repo, "commit", "--allow-empty", "-m", "initial")
	writeFile(t, repo, "file.txt", "hello\n")
	first, err := DiffHash(repo, []string{"file.txt"})
	if err != nil {
		t.Fatalf("DiffHash() error = %v, want nil", err)
	}
	second, err := DiffHash(repo, []string{"file.txt"})
	if err != nil {
		t.Fatalf("DiffHash() error = %v, want nil", err)
	}
	if first != second {
		t.Fatalf("DiffHash unstable: first %q second %q", first, second)
	}
}

func TestDiffHashChangesWhenFileChanges(t *testing.T) {
	repo := initRepo(t)
	git(t, repo, "commit", "--allow-empty", "-m", "initial")
	writeFile(t, repo, "file.txt", "one\n")
	first, err := DiffHash(repo, []string{"file.txt"})
	if err != nil {
		t.Fatalf("DiffHash() error = %v, want nil", err)
	}
	writeFile(t, repo, "file.txt", "two\n")
	second, err := DiffHash(repo, []string{"file.txt"})
	if err != nil {
		t.Fatalf("DiffHash() error = %v, want nil", err)
	}
	if first == second {
		t.Fatalf("DiffHash did not change for content change: %q", first)
	}
}

func TestDiffHashChangesWhenStatusChanges(t *testing.T) {
	repo := initRepo(t)
	writeFile(t, repo, "file.txt", "same\n")
	git(t, repo, "add", "file.txt")
	git(t, repo, "commit", "-m", "initial")
	writeFile(t, repo, "file.txt", "changed\n")
	modified, err := DiffHash(repo, []string{"file.txt"})
	if err != nil {
		t.Fatalf("DiffHash() error = %v, want nil", err)
	}
	git(t, repo, "add", "file.txt")
	staged, err := DiffHash(repo, []string{"file.txt"})
	if err != nil {
		t.Fatalf("DiffHash() error = %v, want nil", err)
	}
	if modified == staged {
		t.Fatalf("DiffHash did not change for status change: %q", modified)
	}
}

func TestDiffHashMatchesQualityGateContract(t *testing.T) {
	repo := initRepo(t)
	writeFile(t, repo, "file.txt", "old\n")
	git(t, repo, "add", "file.txt")
	git(t, repo, "commit", "-m", "initial")
	writeFile(t, repo, "file.txt", "changed\n")
	writeFile(t, repo, "untracked.txt", "new\n")
	head, err := Head(repo)
	if err != nil {
		t.Fatalf("Head() error = %v, want nil", err)
	}

	got, err := DiffHash(repo, []string{"untracked.txt", "file.txt"})
	if err != nil {
		t.Fatalf("DiffHash() error = %v, want nil", err)
	}
	digest := sha256.New()
	digest.Write([]byte("head:" + head + "\n"))
	for _, entry := range []struct {
		status  string
		path    string
		content string
	}{
		{status: " M", path: "file.txt", content: "changed\n"},
		{status: "??", path: "untracked.txt", content: "new\n"},
	} {
		digest.Write([]byte(entry.status))
		digest.Write([]byte{0})
		digest.Write([]byte(entry.path))
		digest.Write([]byte{0})
		digest.Write([]byte(entry.content))
		digest.Write([]byte{0})
	}
	want := hex.EncodeToString(digest.Sum(nil))
	if got != want {
		t.Fatalf("DiffHash() = %q, want quality-gate contract hash %q", got, want)
	}
}

func TestDiffHashEmpty(t *testing.T) {
	repo := initRepo(t)
	git(t, repo, "commit", "--allow-empty", "-m", "initial")
	head, err := Head(repo)
	if err != nil {
		t.Fatalf("Head() error = %v, want nil", err)
	}
	got, err := DiffHash(repo, nil)
	if err != nil {
		t.Fatalf("DiffHash() error = %v, want nil", err)
	}
	digest := sha256.New()
	digest.Write([]byte("head:" + head + "\n"))
	want := hex.EncodeToString(digest.Sum(nil))
	if got != want {
		t.Fatalf("DiffHash(nil) = %q, want %q", got, want)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
