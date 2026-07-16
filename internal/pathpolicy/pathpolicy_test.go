package pathpolicy

import (
	"os"
	"path/filepath"
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

// TestPathPolicyAllowsSymlinkedRepoRootForExistingFile reproduces the
// macOS/Windows CI failure where the repo root is only reachable through an
// alias (a symlink such as macOS's /var -> /private/var, or an OS
// short-name form) while an already-written file resolves to its canonical
// path. Comparing an unresolved root against a resolved leaf made every
// generated-artifact validation look like an escape.
func TestPathPolicyAllowsSymlinkedRepoRootForExistingFile(t *testing.T) {
	p := NewDefault()
	real := t.TempDir()
	if err := os.MkdirAll(filepath.Join(real, "sub"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(real, "sub", "file.txt"), []byte("data"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(real, alias); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}
	if _, err := p.Abs(alias, filepath.Join("sub", "file.txt")); err != nil {
		t.Fatalf("Abs() error = %v, want existing file under aliased root accepted", err)
	}
}

// TestPathPolicyAllowsSymlinkedRepoRootForNotYetWrittenFile covers the
// companion case: the leaf being validated does not exist yet (e.g. output
// about to be written), so only the ancestor directories can be resolved.
func TestPathPolicyAllowsSymlinkedRepoRootForNotYetWrittenFile(t *testing.T) {
	p := NewDefault()
	real := t.TempDir()
	if err := os.MkdirAll(filepath.Join(real, "sub"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(real, alias); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}
	if _, err := p.Abs(alias, filepath.Join("sub", "not-yet-written.txt")); err != nil {
		t.Fatalf("Abs() error = %v, want not-yet-written path under aliased root accepted", err)
	}
}
