package gitstate

import "testing"

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

func TestDiffHashEmpty(t *testing.T) {
	repo := initRepo(t)
	got, err := DiffHash(repo, nil)
	if err != nil {
		t.Fatalf("DiffHash() error = %v, want nil", err)
	}
	want := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
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
