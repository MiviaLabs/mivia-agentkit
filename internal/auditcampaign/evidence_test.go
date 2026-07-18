// Package auditcampaign tests campaign evidence validation.
// Plan: WS15.
package auditcampaign

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func validEvidenceJSON() []byte {
	return []byte(`{
  "schema": "mivia-agent-campaign-evidence/v1",
  "campaign_run": "run-1",
  "cycle": 1,
  "baseline_head": "abc123",
  "disposition": "confirmed",
  "finding_fingerprint": "fpdeadbeef",
  "changed_path_ids": ["pathid1"],
  "verifier_ref": "go-test",
  "progress": 1,
  "token_source": "unavailable",
  "timing_source": "runtime",
  "measured_elapsed_ms": 12
}`)
}

func TestEvidenceAcceptsConfirmedFinding(t *testing.T) {
	e, err := DecodeEvidence(validEvidenceJSON())
	if err != nil {
		t.Fatalf("DecodeEvidence() error = %v", err)
	}
	if !e.CommitEligible() {
		t.Fatalf("CommitEligible() = false, want true")
	}
}

func TestEvidenceRejectsUnprovenTelemetry(t *testing.T) {
	raw := []byte(`{
  "schema": "mivia-agent-campaign-evidence/v1",
  "campaign_run": "run-1",
  "cycle": 0,
  "baseline_head": "abc",
  "measured_elapsed_ms": 99
}`)
	_, err := DecodeEvidence(raw)
	if err == nil || !strings.Contains(err.Error(), "timing_source") {
		t.Fatalf("error = %v, want timing_source rejection", err)
	}
}

func TestEvidenceRejectsSensitiveFields(t *testing.T) {
	raw := []byte(`{
  "schema": "mivia-agent-campaign-evidence/v1",
  "campaign_run": "run-1",
  "cycle": 0,
  "baseline_head": "abc",
  "changed_path_ids": ["/home/user/secret.go"]
}`)
	_, err := DecodeEvidence(raw)
	if err == nil || !strings.Contains(err.Error(), "opaque") {
		t.Fatalf("error = %v, want opaque path rejection", err)
	}
}

func TestEvidenceRejectsOversizeAndUnknownFields(t *testing.T) {
	raw := []byte(`{"schema":"mivia-agent-campaign-evidence/v1","campaign_run":"r","cycle":0,"baseline_head":"h","extra":1}`)
	_, err := DecodeEvidence(raw)
	if err == nil {
		t.Fatalf("error = nil, want unknown field rejection")
	}
	big := make([]byte, MaxEvidenceBytes+1)
	for i := range big {
		big[i] = 'a'
	}
	_, err = DecodeEvidence(big)
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("error = %v, want oversize rejection", err)
	}
}

func TestEvidenceCandidateNotCommitEligible(t *testing.T) {
	e := Evidence{
		Schema:             EvidenceSchema,
		CampaignRun:        "r",
		BaselineHead:       "h",
		Disposition:        DispositionCandidate,
		FindingFingerprint: "fp1",
		ChangedPathIDs:     []string{"p1"},
		VerifierRef:        "v",
	}
	if e.CommitEligible() {
		t.Fatalf("candidate must not be commit eligible")
	}
}

func TestEvidenceAcceptsReverifiableHandoff(t *testing.T) {
	raw := []byte(`{
  "schema": "mivia-agent-campaign-evidence/v1",
  "campaign_run": "run-1",
  "cycle": 1,
  "baseline_head": "abc123",
  "disposition": "candidate",
  "finding_fingerprint": "fpdeadbeef",
  "finding_claim": "nil deref on empty verifier argv under internal/cli",
  "path_hints": ["internal/cli/campaign_adapters.go"],
  "changed_path_ids": ["p1"],
  "progress": 0
}`)
	e, err := DecodeEvidence(raw)
	if err != nil {
		t.Fatalf("DecodeEvidence() error = %v", err)
	}
	if !e.HasReverifiableHandoff() {
		t.Fatalf("HasReverifiableHandoff() = false, want true")
	}
	if e.FindingClaim == "" || len(e.PathHints) != 1 {
		t.Fatalf("handoff fields = claim %q paths %v", e.FindingClaim, e.PathHints)
	}
	if e.CommitEligible() {
		t.Fatalf("candidate with handoff must not be commit eligible")
	}
}

func TestEvidenceRejectsSecretLikeFindingClaim(t *testing.T) {
	raw := []byte(`{
  "schema": "mivia-agent-campaign-evidence/v1",
  "campaign_run": "run-1",
  "cycle": 0,
  "baseline_head": "abc",
  "finding_claim": "leaked Bearer abcdefghijklmnop token"
}`)
	_, err := DecodeEvidence(raw)
	if err == nil || !strings.Contains(err.Error(), "secret-like") {
		t.Fatalf("error = %v, want secret-like claim rejection", err)
	}
}

func TestEvidenceRejectsOverlongFindingClaim(t *testing.T) {
	claim := strings.Repeat("a", MaxFindingClaimBytes+1)
	raw := []byte(fmt.Sprintf(`{
  "schema": "mivia-agent-campaign-evidence/v1",
  "campaign_run": "run-1",
  "cycle": 0,
  "baseline_head": "abc",
  "finding_claim": %q
}`, claim))
	_, err := DecodeEvidence(raw)
	if err == nil || !strings.Contains(err.Error(), "finding_claim exceeds") {
		t.Fatalf("error = %v, want overlong claim rejection", err)
	}
}

func TestEvidenceRejectsAbsolutePathHints(t *testing.T) {
	raw := []byte(`{
  "schema": "mivia-agent-campaign-evidence/v1",
  "campaign_run": "run-1",
  "cycle": 0,
  "baseline_head": "abc",
  "path_hints": ["/home/user/secret.go"]
}`)
	_, err := DecodeEvidence(raw)
	if err == nil || !strings.Contains(err.Error(), "repo-relative") {
		t.Fatalf("error = %v, want repo-relative path_hints rejection", err)
	}
}

func TestEvidenceRejectsSecretPathHints(t *testing.T) {
	raw := []byte(`{
  "schema": "mivia-agent-campaign-evidence/v1",
  "campaign_run": "run-1",
  "cycle": 0,
  "baseline_head": "abc",
  "path_hints": [".env.local"]
}`)
	_, err := DecodeEvidence(raw)
	if err == nil || !strings.Contains(err.Error(), "secret-like") {
		t.Fatalf("error = %v, want secret-like path_hints rejection", err)
	}
}

func TestEvidenceRejectsTooManyPathHints(t *testing.T) {
	hints := make([]string, MaxPathHints+1)
	for i := range hints {
		hints[i] = fmt.Sprintf("internal/file%d.go", i)
	}
	b, _ := json.Marshal(map[string]any{
		"schema": EvidenceSchema, "campaign_run": "r", "cycle": 0, "baseline_head": "h",
		"path_hints": hints,
	})
	_, err := DecodeEvidence(b)
	if err == nil || !strings.Contains(err.Error(), "path_hints exceeds") {
		t.Fatalf("error = %v, want path_hints exceeds rejection", err)
	}
}

func TestFilterPathHintsAllowlist(t *testing.T) {
	tests := []struct {
		name    string
		hints   []string
		allowed []string
		want    []string
	}{
		{
			name:    "empty allowlist passthrough",
			hints:   []string{"docs/a.md", "internal/cli/x.go"},
			allowed: nil,
			want:    []string{"docs/a.md", "internal/cli/x.go"},
		},
		{
			name:    "keep under prefix and exact allow",
			hints:   []string{"internal/cli/x.go", "internal", "cmd/mivia-agent/main.go", "docs/a.md"},
			allowed: []string{"internal", "cmd"},
			want:    []string{"internal/cli/x.go", "internal", "cmd/mivia-agent/main.go"},
		},
		{
			name:    "drop out of scope",
			hints:   []string{"scripts/x.sh", "internal/../secret"},
			allowed: []string{"internal"},
			want:    nil, // "internal/../secret" does not match prefix rule after literal check
		},
		{
			name:    "dedupe",
			hints:   []string{"internal/a.go", "internal/a.go"},
			allowed: []string{"internal"},
			want:    []string{"internal/a.go"},
		},
		{
			name:    "empty hints",
			hints:   nil,
			allowed: []string{"internal"},
			want:    nil,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := FilterPathHints(tc.hints, tc.allowed)
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("got %v, want %v", got, tc.want)
				}
			}
		})
	}
}

func TestCommitEligibleUnaffectedByMissingHandoff(t *testing.T) {
	// Evidence API: CommitEligible stays confirmed+fp+paths+verifier only.
	e := Evidence{
		Schema: EvidenceSchema, CampaignRun: "r", BaselineHead: "h",
		Disposition: DispositionConfirmed, FindingFingerprint: "fpok",
		ChangedPathIDs: []string{"p1"}, VerifierRef: "go-test",
	}
	if !e.CommitEligible() {
		t.Fatal("CommitEligible must not require handoff fields")
	}
	if e.HasReverifiableHandoff() {
		t.Fatal("expected no handoff")
	}
}
