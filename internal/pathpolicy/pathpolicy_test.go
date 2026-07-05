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
