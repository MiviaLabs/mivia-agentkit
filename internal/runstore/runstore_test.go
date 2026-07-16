// Package runstore persists bounded workflow run artifacts.
// Plan: WS10. PRD: FR-4.5, FR-7.4.
package runstore

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mustNewRun(t *testing.T, s Store) RunID {
	t.Helper()
	id, err := s.NewRun()
	if err != nil {
		t.Fatalf("NewRun() error = %v", err)
	}
	return id
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

func TestAppendTraceFailsOnUnmarshalablePayload(t *testing.T) {
	s := New(t.TempDir())
	id := mustNewRun(t, s)
	// channels cannot be JSON-marshaled.
	err := s.AppendTrace(id, TraceEvent{
		TS:      "2026-07-05T00:00:00Z",
		Kind:    "bad",
		Payload: map[string]any{"ch": make(chan int)},
	})
	if err == nil {
		t.Fatalf("AppendTrace error = nil, want marshal failure")
	}
	// Ensure we did not write corrupt partial JSONL.
	path := filepath.Join(s.Dir(id), "trace.jsonl")
	if data, readErr := os.ReadFile(path); readErr == nil && bytes.Contains(data, []byte(`"ch":`)) {
		t.Fatalf("trace contains partial payload after marshal error: %s", data)
	}
}

func TestNewRunFailsWhenRootNotWritable(t *testing.T) {
	// Root parent is a file, so MkdirAll must fail.
	parent := filepath.Join(t.TempDir(), "file-not-dir")
	if err := os.WriteFile(parent, []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	s := Store{Root: filepath.Join(parent, "runs")}
	if _, err := s.NewRun(); err == nil {
		t.Fatalf("NewRun error = nil, want create run dir failure")
	}
}
