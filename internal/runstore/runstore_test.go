// Package runstore persists bounded workflow run artifacts.
// Plan: WS10. PRD: FR-4.5, FR-7.4.
package runstore

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func mustNewRun(t *testing.T, s Store) RunID {
	t.Helper()
	id, err := s.NewRun()
	if err != nil {
		t.Fatalf("NewRun() error = %v", err)
	}
	return id
}

func TestNewRunPropagatesRandomFailure(t *testing.T) {
	previous := randomBytes
	randomBytes = func([]byte) (int, error) { return 0, errors.New("rng unavailable") }
	t.Cleanup(func() { randomBytes = previous })
	if _, err := New(t.TempDir()).NewRun(); err == nil {
		t.Fatal("NewRun() error = nil, want random failure")
	}
}

func TestNewRunRejectsCollision(t *testing.T) {
	previous := randomBytes
	randomBytes = func(b []byte) (int, error) { return len(b), nil }
	t.Cleanup(func() { randomBytes = previous })
	repo := t.TempDir()
	when := time.Date(2026, 7, 10, 1, 2, 3, 0, time.UTC)
	s := Store{Root: filepath.Join(repo, ".ai", "runs"), now: func() time.Time { return when }}
	stamp := when.Format("20060102T150405Z") + "-" + strings.Repeat("00", 16)
	if err := os.MkdirAll(filepath.Join(repo, ".ai", "runs", stamp), 0o755); err != nil {
		t.Fatalf("MkdirAll collision: %v", err)
	}
	if _, err := s.NewRun(); err == nil {
		t.Fatal("NewRun() error = nil, want collision rejection")
	}
}

func TestNewRunPropagatesMkdirFailure(t *testing.T) {
	s := Store{Root: filepath.Join(t.TempDir(), "missing", ".ai", "runs")}
	if _, err := s.NewRun(); err == nil {
		t.Fatal("NewRun() error = nil, want directory creation failure")
	}
}

func TestNewRunCreatesDir(t *testing.T) {
	s := New(t.TempDir())
	id := mustNewRun(t, s)
	if id == "" {
		t.Fatalf("NewRun id is empty")
	}
	if st, err := os.Stat(s.Dir(id)); err != nil || !st.IsDir() {
		t.Fatalf("NewRun dir got stat=%v err=%v, want dir", st, err)
	}
}

func TestWriteArtifactStaysUnderRuns(t *testing.T) {
	repo := t.TempDir()
	s := New(repo)
	path, err := s.WriteArtifact(mustNewRun(t, s), "produce", 1, "out.txt", []byte("ok"))
	if err != nil {
		t.Fatalf("WriteArtifact error = %v", err)
	}
	if !strings.HasPrefix(path, filepath.Join(repo, ".ai", "runs")) {
		t.Fatalf("artifact path = %q, want under .ai/runs", path)
	}
}

func TestWriteArtifactUsesIterationSubdirectory(t *testing.T) {
	s := New(t.TempDir())
	path, err := s.WriteArtifact(mustNewRun(t, s), "produce", 2, "artifact.md", []byte("ok"))
	if err != nil {
		t.Fatalf("WriteArtifact error = %v", err)
	}
	if !strings.Contains(path, filepath.Join("produce", "iter-002", "artifact.md")) {
		t.Fatalf("artifact path = %q, want iteration subdirectory", path)
	}
}

func TestAppendTraceAppendsJSONL(t *testing.T) {
	s := New(t.TempDir())
	id := mustNewRun(t, s)
	for _, kind := range []string{"one", "two"} {
		if err := s.AppendTrace(id, TraceEvent{TS: "2026-07-05T00:00:00Z", Kind: kind, Step: "s"}); err != nil {
			t.Fatalf("AppendTrace error = %v", err)
		}
	}
	data, err := os.ReadFile(filepath.Join(s.Dir(id), "trace.jsonl"))
	if err != nil {
		t.Fatalf("read trace: %v", err)
	}
	if got := bytes.Count(data, []byte{'\n'}); got != 2 {
		t.Fatalf("trace lines got %d, want 2: %s", got, data)
	}
}

func TestAppendTraceStableKeyOrder(t *testing.T) {
	s := New(t.TempDir())
	id := mustNewRun(t, s)
	err := s.AppendTrace(id, TraceEvent{TS: "2026-07-05T00:00:00Z", Kind: "k", Step: "s", Payload: map[string]any{"z": "last", "a": "first"}})
	if err != nil {
		t.Fatalf("AppendTrace error = %v", err)
	}
	data, err := os.ReadFile(filepath.Join(s.Dir(id), "trace.jsonl"))
	if err != nil {
		t.Fatalf("read trace: %v", err)
	}
	want := `{"iteration":0,"kind":"k","payload":{"a":"first","z":"last"},"step":"s","ts":"2026-07-05T00:00:00Z"}` + "\n"
	if string(data) != want {
		t.Fatalf("trace = %q, want %q", data, want)
	}
}

func TestAppendTraceRejectsRunIDTraversal(t *testing.T) {
	repo := t.TempDir()
	s := New(repo)
	if err := s.AppendTrace(RunID("../escape"), TraceEvent{TS: "2026-07-05T00:00:00Z", Kind: "k"}); err == nil {
		t.Fatalf("AppendTrace traversal error = nil, want error")
	}
	if _, err := os.Stat(filepath.Join(repo, ".ai", "escape", "trace.jsonl")); !os.IsNotExist(err) {
		t.Fatalf("trace escaped runs or stat failed err=%v", err)
	}
	absolute := filepath.Join(t.TempDir(), "escape")
	if err := s.AppendTrace(RunID(absolute), TraceEvent{TS: "2026-07-05T00:00:00Z", Kind: "k"}); err == nil {
		t.Fatalf("AppendTrace absolute run id error = nil, want error")
	}
	if _, err := os.Stat(filepath.Join(absolute, "trace.jsonl")); !os.IsNotExist(err) {
		t.Fatalf("trace escaped to absolute run id or stat failed err=%v", err)
	}
}

func TestAppendTraceStaysUnderRuns(t *testing.T) {
	repo := t.TempDir()
	s := New(repo)
	id := mustNewRun(t, s)
	if err := s.AppendTrace(id, TraceEvent{TS: "2026-07-05T00:00:00Z", Kind: "k"}); err != nil {
		t.Fatalf("AppendTrace error = %v", err)
	}
	path := filepath.Join(s.Dir(id), "trace.jsonl")
	if !strings.HasPrefix(path, filepath.Join(repo, ".ai", "runs")+string(filepath.Separator)) {
		t.Fatalf("trace path = %q, want under .ai/runs", path)
	}
}

func TestReadArtifactRoundTrip(t *testing.T) {
	s := New(t.TempDir())
	id := mustNewRun(t, s)
	if _, err := s.WriteArtifact(id, "produce", 1, "artifact.md", []byte("hello")); err != nil {
		t.Fatalf("WriteArtifact error = %v", err)
	}
	got, err := s.ReadArtifact(id, "produce", 1, "artifact.md")
	if err != nil {
		t.Fatalf("ReadArtifact error = %v", err)
	}
	if string(got) != "hello" {
		t.Fatalf("ReadArtifact got %q, want hello", got)
	}
}

func TestWriteArtifactRejectsTraversal(t *testing.T) {
	s := New(t.TempDir())
	if _, err := s.WriteArtifact(mustNewRun(t, s), "produce", 1, "../../escape.txt", []byte("bad")); err == nil {
		t.Fatalf("WriteArtifact traversal error = nil, want error")
	}
}

func TestRunstoreRejectsAISymlinkEscape(t *testing.T) {
	repo := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(repo, ".ai")); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}
	s := New(repo)
	if _, err := s.WriteArtifact("run", "produce", 1, "artifact.md", []byte("bad")); err == nil {
		t.Fatal("WriteArtifact() error = nil, want symlink rejection")
	}
	if err := s.AppendTrace("run", TraceEvent{TS: "2026-07-10T00:00:00Z", Kind: "test"}); err == nil {
		t.Fatal("AppendTrace() error = nil, want symlink rejection")
	}
	if _, err := os.Stat(filepath.Join(outside, "runs", "run", "produce", "iter-001", "artifact.md")); !os.IsNotExist(err) {
		t.Fatalf("outside artifact exists or Stat failed: %v", err)
	}
}
