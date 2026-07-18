// Package auditcampaign implements the supervised audit-repair campaign runtime.
// Plan: WS15. PRD: FR-4.2, measurement integrity, scoped commit boundary.
package auditcampaign

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

// EvidenceSchema is the campaign evidence envelope version.
const EvidenceSchema = "mivia-agent-campaign-evidence/v1"

// MaxEvidenceBytes bounds a single evidence payload.
const MaxEvidenceBytes = 256 * 1024

// MaxFindingClaimBytes bounds the non-secret re-verifiable claim handoff.
const MaxFindingClaimBytes = 240

// MaxPathHints bounds path_hints entries on one evidence envelope.
const MaxPathHints = 8

// MaxPathHintBytes bounds a single repo-relative path hint.
const MaxPathHintBytes = 200

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
	Schema             string      `json:"schema"`
	CampaignRun        string      `json:"campaign_run"`
	Cycle              int         `json:"cycle"`
	BaselineHead       string      `json:"baseline_head"`
	FindingFingerprint string      `json:"finding_fingerprint,omitempty"`
	Disposition        Disposition `json:"disposition,omitempty"`
	// FindingClaim is a short non-secret description of the bug class/location
	// for independent re-verification (not free-form model prose / secrets).
	FindingClaim string `json:"finding_claim,omitempty"`
	// PathHints are repo-relative path hints the confirmer may open to re-check
	// the claim (not opaque ids; validated relative path shapes only).
	PathHints         []string      `json:"path_hints,omitempty"`
	ConfirmedFindings []FindingRef  `json:"confirmed_findings,omitempty"`
	Progress          int           `json:"progress"`
	ChangedPathIDs    []string      `json:"changed_path_ids,omitempty"`
	VerifierRef       string        `json:"verifier_ref,omitempty"`
	Commit            *CommitRef    `json:"commit,omitempty"`
	Resume            string        `json:"resume,omitempty"`
	Metrics           *PhaseMetrics `json:"metrics,omitempty"`
	TokenSource       string        `json:"token_source,omitempty"`
	TimingSource      string        `json:"timing_source,omitempty"`
	MeasuredElapsedMS *int64        `json:"measured_elapsed_ms,omitempty"`
	ProviderTokens    *int64        `json:"provider_tokens,omitempty"`
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
	if err := validateFindingClaim(e.FindingClaim); err != nil {
		return err
	}
	if err := validatePathHints(e.PathHints); err != nil {
		return err
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
	return nil
}

// CommitEligible reports whether evidence can drive a commit-capable fix.
// Requires confirmed disposition, opaque fingerprint, path IDs, and verifier ref.
func (e Evidence) CommitEligible() bool {
	if e.Disposition != DispositionConfirmed {
		return false
	}
	if e.FindingFingerprint == "" || !isOpaqueID(e.FindingFingerprint) {
		return false
	}
	if e.VerifierRef == "" || len(e.ChangedPathIDs) == 0 {
		return false
	}
	return true
}

// HasReverifiableHandoff reports whether evidence carries a non-empty claim
// and/or path hints suitable for independent confirm re-check.
func (e Evidence) HasReverifiableHandoff() bool {
	if strings.TrimSpace(e.FindingClaim) != "" {
		return true
	}
	for _, p := range e.PathHints {
		if strings.TrimSpace(p) != "" {
			return true
		}
	}
	return false
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

// secretLikeClaim matches common secret material that must never appear in claims.
var secretLikeClaim = regexp.MustCompile(`(?i)(bearer\s+[a-z0-9._~+/=-]{8,}|sk-[a-z0-9]{8,}|api[_-]?key\s*[:=]\s*\S+|-----begin[ a-z]*private key-----|xox[baprs]-[a-z0-9-]+)`)

func validateFindingClaim(claim string) error {
	if claim == "" {
		return nil
	}
	if len(claim) > MaxFindingClaimBytes {
		return fmt.Errorf("finding_claim exceeds %d bytes", MaxFindingClaimBytes)
	}
	if !utf8.ValidString(claim) {
		return fmt.Errorf("finding_claim must be valid utf-8")
	}
	if strings.ContainsAny(claim, "\x00\r\n") {
		return fmt.Errorf("finding_claim must be a single line without control chars")
	}
	if secretLikeClaim.MatchString(claim) {
		return fmt.Errorf("finding_claim contains secret-like content")
	}
	// Block absolute / home paths and credential file names in the claim text.
	lower := strings.ToLower(claim)
	if strings.Contains(lower, "/.env") || strings.Contains(lower, "\\secrets\\") ||
		strings.Contains(lower, "private_key") || strings.Contains(lower, "password=") {
		return fmt.Errorf("finding_claim contains disallowed sensitive path or credential text")
	}
	return nil
}

func validatePathHints(hints []string) error {
	if len(hints) > MaxPathHints {
		return fmt.Errorf("path_hints exceeds %d entries", MaxPathHints)
	}
	for _, h := range hints {
		if err := validatePathHint(h); err != nil {
			return err
		}
	}
	return nil
}

func validatePathHint(p string) error {
	if p == "" {
		return fmt.Errorf("path_hints entry must be non-empty")
	}
	if len(p) > MaxPathHintBytes {
		return fmt.Errorf("path_hints entry exceeds %d bytes", MaxPathHintBytes)
	}
	if strings.ContainsAny(p, "\x00\r\n\t") {
		return fmt.Errorf("path_hints entry contains control characters")
	}
	if strings.HasPrefix(p, "/") || strings.HasPrefix(p, "\\") || strings.Contains(p, ":") {
		return fmt.Errorf("path_hints must be repo-relative, got %q", p)
	}
	if strings.Contains(p, "..") || strings.Contains(p, "\\") {
		return fmt.Errorf("path_hints must not contain .. or backslashes, got %q", p)
	}
	// Disallow secret-like path shapes (aligned with pathpolicy defaults).
	base := p
	if i := strings.LastIndex(p, "/"); i >= 0 {
		base = p[i+1:]
	}
	lower := strings.ToLower(p)
	lowerBase := strings.ToLower(base)
	if lowerBase == ".env" || strings.HasPrefix(lowerBase, ".env.") ||
		strings.Contains(lower, "secrets/") || strings.Contains(lowerBase, "private") && strings.Contains(lowerBase, "key") {
		return fmt.Errorf("path_hints forbids secret-like path %q", p)
	}
	// Path segments: allow alnum, ., _, -, /
	for _, r := range p {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') ||
			r == '/' || r == '.' || r == '_' || r == '-' {
			continue
		}
		return fmt.Errorf("path_hints has disallowed characters in %q", p)
	}
	return nil
}
