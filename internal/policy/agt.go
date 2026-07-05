//go:build agt

// Package policy defines mivia-agent governance provider contracts.
// Plan: WS12. PRD: FR-2.2, FR-7.1, FR-7.2.
//
// AGT re-verification, 2026-07-05:
//   - Public quickstart documents the Go module path as
//     github.com/microsoft/agent-governance-toolkit/agent-governance-golang.
//   - The public overview documents tamper-evident decision records and policy
//     enforcement, but current public docs do not expose a stable Go evaluator
//     call shape suitable for this package to implement without guessing.
//   - The repository is MIT licensed. Keep the production AGT mapping behind this
//     build tag until the Go SDK API is pinned.
//
// Sources:
// - https://github.com/microsoft/agent-governance-toolkit/blob/main/docs/quickstart.md
// - https://microsoft.github.io/agent-governance-toolkit/
// - https://github.com/microsoft/agent-governance-toolkit
package policy

import (
	"context"
	"errors"
)

// AGT is the build-tagged governance provider placeholder.
type AGT struct {
	AuditPath string
}

// NewAGT returns the AGT provider once the SDK evaluator API is pinned.
func NewAGT(auditPath string) (Provider, error) {
	return nil, ErrAGTSDKUnavailable
}

// Name returns the provider name.
func (a AGT) Name() string {
	return "agt"
}

// Decide currently fails closed until the AGT Go SDK evaluator API is pinned.
func (a AGT) Decide(context.Context, Action) (Decision, error) {
	return Decision{}, ErrAGTSDKUnavailable
}

// Record currently fails closed until the AGT Go SDK audit API is pinned.
func (a AGT) Record(context.Context, Event) error {
	return ErrAGTSDKUnavailable
}

// ErrAGTSDKUnavailable means the AGT build tag is present but the SDK API is not wired.
var ErrAGTSDKUnavailable = errors.New("agt sdk evaluator api is not pinned")
