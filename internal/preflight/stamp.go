// Package preflight writes and validates quality stamps.
// Plan: WS4. PRD: FR-2.4, FR-7.1.
package preflight

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"time"
)

// Stamp records the repository state and proofs accepted by preflight.
type Stamp struct {
	Head               string   `json:"head"`
	DiffSHA256         string   `json:"diff_sha256"`
	ChangedFiles       []string `json:"changed_files"`
	ContractRows       []string `json:"contract_rows"`
	FocusedVerifiers   []string `json:"focused_verifiers"`
	BroadVerifiers     []string `json:"broad_verifiers"`
	MutationProofs     []string `json:"mutation_proofs"`
	NotRun             []string `json:"not_run"`
	PolicyDecisionRefs []string `json:"policy_decision_refs"`
	CreatedAt          string   `json:"created_at"`
}

// NewStamp returns a stamp with normalized slices and UTC creation time.
func NewStamp(head, diff string, changed []string) Stamp {
	return Stamp{
		Head:               head,
		DiffSHA256:         diff,
		ChangedFiles:       sortedCopy(changed),
		ContractRows:       []string{},
		FocusedVerifiers:   []string{},
		BroadVerifiers:     []string{},
		MutationProofs:     []string{},
		NotRun:             []string{},
		PolicyDecisionRefs: []string{},
		CreatedAt:          time.Now().UTC().Format(time.RFC3339),
	}
}

// Marshal returns deterministic JSON with sorted keys and a trailing newline.
func (s Stamp) Marshal() ([]byte, error) {
	if s.CreatedAt != "" {
		created, err := time.Parse(time.RFC3339, s.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("created_at must be RFC3339 UTC: %w", err)
		}
		if created.Location() != time.UTC {
			s.CreatedAt = created.UTC().Format(time.RFC3339)
		}
	}
	normalizeStamp(&s)
	type orderedStamp struct {
		BroadVerifiers     []string `json:"broad_verifiers"`
		ChangedFiles       []string `json:"changed_files"`
		ContractRows       []string `json:"contract_rows"`
		CreatedAt          string   `json:"created_at"`
		DiffSHA256         string   `json:"diff_sha256"`
		FocusedVerifiers   []string `json:"focused_verifiers"`
		Head               string   `json:"head"`
		MutationProofs     []string `json:"mutation_proofs"`
		NotRun             []string `json:"not_run"`
		PolicyDecisionRefs []string `json:"policy_decision_refs"`
	}
	data, err := json.Marshal(orderedStamp{
		BroadVerifiers:     s.BroadVerifiers,
		ChangedFiles:       s.ChangedFiles,
		ContractRows:       s.ContractRows,
		CreatedAt:          s.CreatedAt,
		DiffSHA256:         s.DiffSHA256,
		FocusedVerifiers:   s.FocusedVerifiers,
		Head:               s.Head,
		MutationProofs:     s.MutationProofs,
		NotRun:             s.NotRun,
		PolicyDecisionRefs: s.PolicyDecisionRefs,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal stamp: %w", err)
	}
	var out bytes.Buffer
	if err := json.Indent(&out, data, "", "  "); err != nil {
		return nil, fmt.Errorf("format stamp: %w", err)
	}
	out.WriteByte('\n')
	return out.Bytes(), nil
}

// ParseStamp decodes a stamp and rejects malformed or incomplete content.
func ParseStamp(b []byte) (Stamp, error) {
	var s Stamp
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&s); err != nil {
		return Stamp{}, fmt.Errorf("parse stamp: %w", err)
	}
	if s.Head == "" {
		return Stamp{}, fmt.Errorf("stamp missing head")
	}
	if s.DiffSHA256 == "" {
		return Stamp{}, fmt.Errorf("stamp missing diff_sha256")
	}
	if s.CreatedAt != "" {
		created, err := time.Parse(time.RFC3339, s.CreatedAt)
		if err != nil {
			return Stamp{}, fmt.Errorf("stamp created_at invalid: %w", err)
		}
		s.CreatedAt = created.UTC().Format(time.RFC3339)
	}
	normalizeStamp(&s)
	return s, nil
}

func normalizeStamp(s *Stamp) {
	s.ChangedFiles = sortedCopy(s.ChangedFiles)
	s.ContractRows = sortedCopy(s.ContractRows)
	s.FocusedVerifiers = sortedCopy(s.FocusedVerifiers)
	s.BroadVerifiers = sortedCopy(s.BroadVerifiers)
	s.MutationProofs = sortedCopy(s.MutationProofs)
	s.NotRun = sortedCopy(s.NotRun)
	s.PolicyDecisionRefs = sortedCopy(s.PolicyDecisionRefs)
}

func sortedCopy(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	if out == nil {
		return []string{}
	}
	return out
}
