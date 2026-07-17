// Package auditcampaign tests campaign evidence validation.
// Plan: WS15.
package auditcampaign

import (
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
