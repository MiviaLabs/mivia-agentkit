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
	stamp.PipelinePreflight = Metadata{
		"created_at": "2026-07-05T00:00:00Z",
		"passed":     true,
	}
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
	if got := parsed.PipelinePreflight["passed"]; got != true {
		t.Fatalf("PipelinePreflight[passed] got %v want true", got)
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

func TestParseStampRejectsMalformedJSON(t *testing.T) {
	_, err := ParseStamp([]byte(`{"head":`))
	if err == nil {
		t.Fatalf("ParseStamp() error = nil, want malformed JSON error")
	}
}

func TestParseStampAcceptsPipelinePreflightMetadata(t *testing.T) {
	data := []byte(`{
  "head": "abc",
  "diff_sha256": "def",
  "changed_files": ["b.go", "a.go"],
  "contract_rows": [],
  "focused_verifiers": [],
  "broad_verifiers": [],
  "mutation_proofs": [],
  "not_run": [],
  "policy_decision_refs": [],
  "pipeline_preflight": {
    "passed": true,
    "contract_sha256": "contract",
    "categories": ["pipeline"],
    "stages": ["preflight"],
    "verifiers": ["scripts/preflight-v2-pipeline"],
    "created_at": "2026-07-05T00:00:00Z",
    "future_metadata": {"accepted": true}
  },
  "created_at": "2026-07-05T00:00:00Z"
}`)
	stamp, err := ParseStamp(data)
	if err != nil {
		t.Fatalf("ParseStamp() error = %v", err)
	}
	if got := stamp.PipelinePreflight["passed"]; got != true {
		t.Fatalf("PipelinePreflight[passed] got %v want true", got)
	}
	if got, want := strings.Join(stamp.ChangedFiles, ","), "a.go,b.go"; got != want {
		t.Fatalf("ChangedFiles got %q want %q", got, want)
	}
}

func TestParseStampRejectsUnknownTopLevelField(t *testing.T) {
	_, err := ParseStamp([]byte(`{
  "head": "abc",
  "diff_sha256": "def",
  "unexpected": true
}`))
	if err == nil || !strings.Contains(err.Error(), `unknown field "unexpected"`) {
		t.Fatalf("ParseStamp() error = %v, want unknown top-level field rejection", err)
	}
}
