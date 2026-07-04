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

func TestNewRunCreatesDir(t *testing.T) {
	s := New(t.TempDir())
	id := s.NewRun()
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
	path, err := s.WriteArtifact(s.NewRun(), "produce", 1, "out.txt", []byte("ok"))
	if err != nil {
		t.Fatalf("WriteArtifact error = %v", err)
	}
	if !strings.HasPrefix(path, filepath.Join(repo, ".ai", "runs")) {
		t.Fatalf("artifact path = %q, want under .ai/runs", path)
	}
}

func TestWriteArtifactUsesIterationSubdirectory(t *testing.T) {
	s := New(t.TempDir())
	path, err := s.WriteArtifact(s.NewRun(), "produce", 2, "artifact.md", []byte("ok"))
	if err != nil {
		t.Fatalf("WriteArtifact error = %v", err)
	}
	if !strings.Contains(path, filepath.Join("produce", "iter-002", "artifact.md")) {
		t.Fatalf("artifact path = %q, want iteration subdirectory", path)
	}
}

func TestAppendTraceAppendsJSONL(t *testing.T) {
	s := New(t.TempDir())
	id := s.NewRun()
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
	id := s.NewRun()
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

func TestReadArtifactRoundTrip(t *testing.T) {
	s := New(t.TempDir())
	id := s.NewRun()
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
	if _, err := s.WriteArtifact(s.NewRun(), "produce", 1, "../../escape.txt", []byte("bad")); err == nil {
		t.Fatalf("WriteArtifact traversal error = nil, want error")
	}
}
