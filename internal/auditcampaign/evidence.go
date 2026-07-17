// Package auditcampaign implements the supervised audit-repair campaign runtime.
// Plan: WS15. PRD: FR-4.2, measurement integrity, scoped commit boundary.
package auditcampaign

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// EvidenceSchema is the campaign evidence envelope version.
const EvidenceSchema = "mivia-agent-campaign-evidence/v1"

// MaxEvidenceBytes bounds a single evidence payload.
const MaxEvidenceBytes = 256 * 1024

// Disposition is the lifecycle state of a finding.
type Disposition string

const (
	DispositionCandidate Disposition = "candidate"
	DispositionConfirmed Disposition = "confirmed"
	DispositionDuplicate Disposition = "duplicate"
	DispositionFixed     Disposition = "fixed"
	DispositionRejected  Disposition = "rejected"
)

// Evidence is the versioned campaign evidence envelope (strict JSON).
type Evidence struct {
	Schema             string             `json:"schema"`
	CampaignRun        string             `json:"campaign_run"`
	Cycle              int                `json:"cycle"`
	BaselineHead       string             `json:"baseline_head"`
	FindingFingerprint string             `json:"finding_fingerprint,omitempty"`
	Disposition        Disposition        `json:"disposition,omitempty"`
	ConfirmedFindings  []FindingRef       `json:"confirmed_findings,omitempty"`
	Progress           int                `json:"progress"`
	ChangedPathIDs     []string           `json:"changed_path_ids,omitempty"`
	VerifierRef        string             `json:"verifier_ref,omitempty"`
	Commit             *CommitRef         `json:"commit,omitempty"`
	Resume             string             `json:"resume,omitempty"`
	Metrics            *PhaseMetrics      `json:"metrics,omitempty"`
	TokenSource        string             `json:"token_source,omitempty"`
	TimingSource       string             `json:"timing_source,omitempty"`
	MeasuredElapsedMS  *int64             `json:"measured_elapsed_ms,omitempty"`
	ProviderTokens     *int64             `json:"provider_tokens,omitempty"`
}

// FindingRef is a redacted finding reference.
type FindingRef struct {
	Fingerprint string      `json:"fingerprint"`
	Disposition Disposition `json:"disposition"`
	PathIDs     []string    `json:"path_ids,omitempty"`
	VerifierID  string      `json:"verifier_id,omitempty"`
}

// CommitRef is safe commit metadata only.
type CommitRef struct {
	SHA         string `json:"sha"`
	MessageHash string `json:"message_hash"`
}

// DecodeEvidence parses a strict, bounded campaign evidence envelope.
func DecodeEvidence(b []byte) (Evidence, error) {
	if len(b) == 0 {
		return Evidence{}, fmt.Errorf("evidence is empty")
	}
	if len(b) > MaxEvidenceBytes {
		return Evidence{}, fmt.Errorf("evidence exceeds %d bytes", MaxEvidenceBytes)
	}
	if bytes.Contains(b, []byte("\x00")) {
		return Evidence{}, fmt.Errorf("evidence contains null bytes")
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	var e Evidence
	if err := dec.Decode(&e); err != nil {
		return Evidence{}, fmt.Errorf("decode evidence: %w", err)
	}
	if err := e.Validate(); err != nil {
		return Evidence{}, err
	}
	return e, nil
}

// Validate checks evidence invariants and rejects unproven telemetry.
func (e Evidence) Validate() error {
	if e.Schema != EvidenceSchema {
		return fmt.Errorf("unknown evidence schema %q", e.Schema)
	}
	if e.CampaignRun == "" {
		return fmt.Errorf("campaign_run is required")
	}
	if e.Cycle < 0 {
		return fmt.Errorf("cycle must be non-negative")
	}
	if e.BaselineHead == "" {
		return fmt.Errorf("baseline_head is required")
	}
	if e.Disposition != "" {
		switch e.Disposition {
		case DispositionCandidate, DispositionConfirmed, DispositionDuplicate, DispositionFixed, DispositionRejected:
		default:
			return fmt.Errorf("unknown disposition %q", e.Disposition)
		}
	}
	for _, id := range e.ChangedPathIDs {
		if !isOpaqueID(id) {
			return fmt.Errorf("changed_path_ids must be opaque ids, got %q", id)
		}
	}
	if e.FindingFingerprint != "" && !isOpaqueID(e.FindingFingerprint) {
		return fmt.Errorf("finding_fingerprint must be opaque")
	}
	// Unproven agent telemetry: numeric elapsed/tokens without source is rejected.
	if e.MeasuredElapsedMS != nil && e.TimingSource == "" {
		return fmt.Errorf("measured_elapsed_ms requires timing_source")
	}
	if e.ProviderTokens != nil {
		if e.TokenSource != "provider" {
			return fmt.Errorf("provider_tokens requires token_source=provider")
		}
	}
	if e.TokenSource != "" && e.TokenSource != "provider" && e.TokenSource != "unavailable" {
		return fmt.Errorf("unknown token_source %q", e.TokenSource)
	}
	if e.TimingSource != "" && e.TimingSource != "runtime" && e.TimingSource != "NOT_MEASURED" {
		return fmt.Errorf("unknown timing_source %q", e.TimingSource)
	}
	// Reject secret-like free text fields by construction: none present.
	return nil
}

// CommitEligible reports whether evidence can drive a commit-capable fix.
func (e Evidence) CommitEligible() bool {
	if e.Disposition != DispositionConfirmed {
		return false
	}
	if e.VerifierRef == "" || len(e.ChangedPathIDs) == 0 {
		return false
	}
	return true
}

func isOpaqueID(s string) bool {
	if s == "" || len(s) > 128 {
		return false
	}
	// Opaque: no path separators, no spaces, no absolute roots.
	if strings.ContainsAny(s, "/\\ \t\n\r") {
		return false
	}
	if strings.HasPrefix(s, ".") {
		return false
	}
	return true
}
