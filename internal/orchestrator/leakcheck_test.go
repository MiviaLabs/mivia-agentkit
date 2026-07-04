// Package orchestrator resolves and executes bounded agent loops.
// Plan: WS10. PRD: FR-7.4.
package orchestrator

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAssertNoLeaksPassesOnCleanDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "trace.jsonl"), []byte(`{"kind":"ok"}`), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	AssertNoLeaks(t, dir)
}

func TestAssertNoLeaksFlagsSecret(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "trace.jsonl"), []byte("TOKEN=abcdefghijklmnop"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	assertNoLeaksFails(t, dir)
}

func TestAssertNoLeaksFlagsRawPromptField(t *testing.T) {
	dir := t.TempDir()
	fixture := []byte("{\"" + "pro" + "mpt" + "\":\"" + "task" + "\"}")
	if err := os.WriteFile(filepath.Join(dir, "trace.jsonl"), fixture, 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	assertNoLeaksFails(t, dir)
}

func assertNoLeaksFails(t *testing.T, dir string) {
	t.Helper()
	probe := &recordingTB{}
	AssertNoLeaks(probe, dir)
	if !probe.failed {
		t.Fatalf("AssertNoLeaks did not fail for %s", dir)
	}
}

type recordingTB struct{ failed bool }

func (r *recordingTB) Helper()               {}
func (r *recordingTB) Fatalf(string, ...any) { r.failed = true }
