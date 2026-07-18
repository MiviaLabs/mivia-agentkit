// Package adapter defines headless CLI adapter contracts.
// Plan: WS-B. PRD: FR-3.1, FR-3.2, FR-7.4.
package adapter

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/MiviaLabs/mivia-agentkit/internal/config"
)

// Request is one headless adapter invocation.
type Request struct {
	Prompt      string
	Workdir     string
	Approval    string
	ArtifactOut string
	// OutputSchema is an optional path to a JSON Schema file for adapters that
	// support provider-enforced structured final responses (e.g. codex --output-schema).
	// Empty means the adapter falls back to free-form last-message / stdout extraction.
	OutputSchema string
	Model        string
	Effort       string
	Params       map[string]string
	Timeout      time.Duration
	MaxTurns     int
}

// Validate rejects unsafe or incomplete requests.
func (r Request) Validate() error {
	if r.Approval == "" {
		return errors.New("approval is required")
	}
	if r.MaxTurns < 0 {
		return errors.New("max turns cannot be negative")
	}
	if err := config.ValidateEffortValue(r.Effort); err != nil {
		return err
	}
	return nil
}

// Result is the scrubbed adapter result.
type Result struct {
	ExitCode     int
	Stdout       []byte
	Stderr       []byte
	ProviderMeta map[string]string
}

// Detection describes a local adapter binary.
type Detection struct {
	Name            string
	Version         string
	HeadlessCapable bool
}

// Verdict is a structured review result.
type Verdict struct {
	Adapter  string `json:"adapter,omitempty"`
	Pass     bool   `json:"pass"`
	Severity string `json:"severity"`
	Notes    string `json:"notes"`
}

// Adapter runs an agent CLI through a bounded runner.
type Adapter interface {
	Name() string
	Role() config.AdapterRole
	Detect(context.Context) (Detection, error)
	Run(context.Context, Request) (Result, error)
	Review(context.Context, Request) (Verdict, error)
}

// RequestValidator validates adapter-specific request fields before execution.
type RequestValidator interface {
	ValidateRequest(Request) error
}

// Registry stores adapters by name.
type Registry struct {
	adapters map[string]Adapter
}

// NewRegistry returns a registry containing adapters keyed by name.
func NewRegistry(adapters ...Adapter) (*Registry, error) {
	r := &Registry{adapters: map[string]Adapter{}}
	for _, a := range adapters {
		if a == nil {
			return nil, errors.New("adapter cannot be nil")
		}
		name := a.Name()
		if name == "" {
			return nil, errors.New("adapter name cannot be empty")
		}
		if _, exists := r.adapters[name]; exists {
			return nil, fmt.Errorf("duplicate adapter %q", name)
		}
		r.adapters[name] = a
	}
	return r, nil
}

// Lookup returns an adapter by name.
func (r *Registry) Lookup(name string) (Adapter, bool) {
	if r == nil {
		return nil, false
	}
	a, ok := r.adapters[name]
	return a, ok
}
