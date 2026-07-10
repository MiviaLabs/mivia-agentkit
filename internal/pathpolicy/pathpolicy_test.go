package pathpolicy

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPathPolicyRejectsTraversal(t *testing.T) {
	p := NewDefault()
	if err := p.Check(t.TempDir(), "nested/../allowed"); err == nil {
		t.Fatal("Check() error = nil, want traversal rejection")
	}
}

func TestPathPolicyRejectsSecretPaths(t *testing.T) {
	p := NewDefault()
	root := t.TempDir()
	for _, rel := range []string{".env", ".env.production", "secrets/db.pem", "id_rsa_private_key", "nested/private/api_key.txt"} {
		if err := p.Check(root, rel); err == nil {
			t.Fatalf("Check(%q) error = nil, want forbidden path", rel)
		}
	}
}

func TestPathPolicyRejectsSymlinkEscape(t *testing.T) {
	p := NewDefault()
	root := t.TempDir()
	outside := t.TempDir()
	link := filepath.Join(root, "link")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}
	if _, err := p.Abs(root, "link"); err == nil {
		t.Fatal("Abs() error = nil, want symlink escape rejection")
	}
}

func TestResolveWritePathRejectsSymlinkedParentWithMissingLeaf(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, ".ai")); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}
	if _, err := ResolveWritePath(root, ".ai/missing/INDEX.md"); err == nil {
		t.Fatal("ResolveWritePath() error = nil, want symlink rejection")
	}
	if _, err := os.Stat(filepath.Join(outside, "missing", "INDEX.md")); !os.IsNotExist(err) {
		t.Fatalf("outside file exists or Stat failed: %v", err)
	}
}

func TestWriteFileWritesInsideRepoIdempotently(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 2; i++ {
		if err := WriteFile(root, ".ai/INDEX.md", []byte("stable\n"), 0o644); err != nil {
			t.Fatalf("WriteFile() iteration %d error = %v", i, err)
		}
	}
	data, err := os.ReadFile(filepath.Join(root, ".ai", "INDEX.md"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if got, want := string(data), "stable\n"; got != want {
		t.Fatalf("file content = %q, want %q", got, want)
	}
}

func TestWriteFileRejectsFinalAndNestedSymlinks(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".ai"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.Symlink(filepath.Join(outside, "final.md"), filepath.Join(root, ".ai", "INDEX.md")); err != nil {
		t.Fatalf("Symlink(final) error = %v", err)
	}
	if err := WriteFile(root, ".ai/INDEX.md", []byte("bad"), 0o644); err == nil {
		t.Fatal("WriteFile(final symlink) error = nil, want rejection")
	}
	if err := os.Remove(filepath.Join(root, ".ai", "INDEX.md")); err != nil {
		t.Fatalf("Remove(final symlink) error = %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(root, ".ai", "nested")); err != nil {
		t.Fatalf("Symlink(nested) error = %v", err)
	}
	if err := WriteFile(root, ".ai/nested/INDEX.md", []byte("bad"), 0o644); err == nil {
		t.Fatal("WriteFile(nested symlink) error = nil, want rejection")
	}
	if _, err := os.Stat(filepath.Join(outside, "INDEX.md")); !os.IsNotExist(err) {
		t.Fatalf("outside file exists or Stat failed: %v", err)
	}
}

func TestWriteFilePreservesExistingMode(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "AGENTS.md")
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatalf("WriteFile fixture error = %v", err)
	}
	if err := WriteFile(root, "AGENTS.md", []byte("new"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if got, want := info.Mode().Perm(), os.FileMode(0o600); got != want {
		t.Fatalf("mode = %o, want %o", got, want)
	}
}

func TestAppendFilePreservesCrossProcessRecords(t *testing.T) {
	root := t.TempDir()
	commands := []*exec.Cmd{
		exec.Command(os.Args[0], "-test.run=TestAppendFileSubprocessHelper"),
		exec.Command(os.Args[0], "-test.run=TestAppendFileSubprocessHelper"),
	}
	for i, cmd := range commands {
		cmd.Env = append(os.Environ(), "GO_WANT_PATHPOLICY_APPEND_HELPER=1", "PATHPOLICY_ROOT="+root, "PATHPOLICY_RECORD=record-"+string(rune('a'+i))+"\n")
		if err := cmd.Start(); err != nil {
			t.Fatalf("Start helper %d error = %v", i, err)
		}
	}
	for i, cmd := range commands {
		if err := cmd.Wait(); err != nil {
			t.Fatalf("helper %d error = %v", i, err)
		}
	}
	data, err := os.ReadFile(filepath.Join(root, ".ai", "runs", "trace.jsonl"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	for _, record := range []string{"record-a", "record-b"} {
		if strings.Count(string(data), record) != 1 {
			t.Fatalf("trace = %q, want exactly one %q", data, record)
		}
	}
}

func TestAppendFileSubprocessHelper(t *testing.T) {
	if os.Getenv("GO_WANT_PATHPOLICY_APPEND_HELPER") != "1" {
		return
	}
	if err := AppendFile(os.Getenv("PATHPOLICY_ROOT"), ".ai/runs/trace.jsonl", []byte(os.Getenv("PATHPOLICY_RECORD")), 0o644); err != nil {
		t.Fatalf("AppendFile() error = %v", err)
	}
}

func TestPathPolicyAllowsRepoRelativeGeneratedPaths(t *testing.T) {
	p := NewDefault()
	root := t.TempDir()
	for _, rel := range []string{".ai/INDEX.md", ".git/mivia-agent-quality-stamp.json", "AGENTS.md", ".codex/hooks.json"} {
		if err := p.Check(root, rel); err != nil {
			t.Fatalf("Check(%q) error = %v, want nil", rel, err)
		}
	}
}

func TestPathPolicyAbsRejectsAbsOutsideRepo(t *testing.T) {
	p := NewDefault()
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "file")
	if _, err := p.Abs(root, outside); err == nil {
		t.Fatal("Abs() error = nil, want absolute outside rejection")
	}
}
