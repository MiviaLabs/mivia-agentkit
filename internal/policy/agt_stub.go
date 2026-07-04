//go:build !agt

// Package policy defines mivia-agent governance provider contracts.
// Plan: WS12. PRD: FR-2.2, FR-7.2.
package policy

import (
	"context"
)

// NewAGT returns the AGT provider when the binary is built with AGT support.
func NewAGT(auditPath string) (Provider, error) {
	return nil, ErrAGTNotCompiled
}

type agtUnavailable struct{}

func (agtUnavailable) Name() string { return "agt" }

func (agtUnavailable) Decide(context.Context, Action) (Decision, error) {
	return Decision{}, ErrAGTNotCompiled
}

func (agtUnavailable) Record(context.Context, Event) error {
	return ErrAGTNotCompiled
}
