// Package preflight writes and validates quality stamps.
// Plan: WS4. PRD: FR-2.4, FR-7.1.
package preflight

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestStampMarshalRoundTrip(t *testing.T) {
	stamp := NewStamp("abc", "def", []string{"b.go", "a.go"})
	stamp.ContractRows = []string{"hooks"}
	data, err := stamp.Marshal()
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	parsed, err := ParseStamp(data)
	if err != nil {
		t.Fatalf("ParseStamp() error = %v", err)
	}
	if parsed.Head != stamp.Head || parsed.DiffSHA256 != stamp.DiffSHA256 {
		t.Fatalf("parsed head/diff mismatch got %q/%q want %q/%q", parsed.Head, parsed.DiffSHA256, stamp.Head, stamp.DiffSHA256)
	}
	if got, want := strings.Join(parsed.ChangedFiles, ","), "a.go,b.go"; got != want {
		t.Fatalf("ChangedFiles got %q want %q", got, want)
	}
	if _, err := time.Parse(time.RFC3339, parsed.CreatedAt); err != nil {
		t.Fatalf("CreatedAt is not RFC3339: %v", err)
	}
}

func TestStampMarshalStableOrder(t *testing.T) {
	stamp := NewStamp("abc", "def", []string{"z.go", "a.go"})
	stamp.MutationProofs = []string{"proof-b", "proof-a"}
	first, err := stamp.Marshal()
	if err != nil {
		t.Fatalf("first Marshal() error = %v", err)
	}
	second, err := stamp.Marshal()
	if err != nil {
		t.Fatalf("second Marshal() error = %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("Marshal() not stable\nfirst=%s\nsecond=%s", first, second)
	}
	if !bytes.HasSuffix(first, []byte("\n")) {
		t.Fatalf("Marshal() must end with newline, got %q", first)
	}
}

func TestParseStampRejectsMissingHead(t *testing.T) {
	_, err := ParseStamp([]byte(`{"diff_sha256":"abc"}`))
	if err == nil {
		t.Fatalf("ParseStamp() error = nil, want missing head error")
	}
}

func TestParseStampToleratesUnknownFields(t *testing.T) {
	// Repo-local quality gates (e.g. a Python preflight) may write extra
	// evidence fields; stamps must stay readable across schema drift.
	data := []byte(`{"head":"abc","diff_sha256":"def","changed_files":[],"execution_evidence":[{"command_id":"x"}],"pipeline_preflight":{"ok":true}}`)
	parsed, err := ParseStamp(data)
	if err != nil {
		t.Fatalf("ParseStamp() error = %v, want unknown fields tolerated", err)
	}
	if parsed.Head != "abc" || parsed.DiffSHA256 != "def" {
		t.Fatalf("parsed head/diff got %q/%q want abc/def", parsed.Head, parsed.DiffSHA256)
	}
}

func TestParseStampRejectsMalformedJSON(t *testing.T) {
	_, err := ParseStamp([]byte(`{"head":`))
	if err == nil {
		t.Fatalf("ParseStamp() error = nil, want malformed JSON error")
	}
}
