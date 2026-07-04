// Package adapter tests headless CLI adapter contracts.
// Plan: WS-B. PRD: FR-3.1, FR-3.2, FR-7.4.
package adapter

import (
	"context"
	"strings"
	"testing"
)

func TestRegistryLookupByName(t *testing.T) {
	fake := namedAdapter{name: "codex"}
	reg, err := NewRegistry(fake)
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	got, ok := reg.Lookup("codex")
	if !ok {
		t.Fatalf("Lookup(codex) ok = false")
	}
	if got.Name() != "codex" {
		t.Fatalf("Lookup(codex).Name() = %q, want codex", got.Name())
	}
}

func TestRequestRejectsEmptyApproval(t *testing.T) {
	err := (Request{Prompt: "x"}).Validate()
	if err == nil {
		t.Fatalf("Validate() error = nil, want rejection for empty approval")
	}
}

func TestRequestAcceptsModelAndEffort(t *testing.T) {
	err := (Request{
		Prompt:   "x",
		Approval: "never",
		Model:    "gpt-5.5",
		Effort:   "high",
		Params:   map[string]string{"provider": "openai"},
	}).Validate()
	if err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
}

func TestRequestAcceptsDocumentedEffortVariants(t *testing.T) {
	for _, effort := range []string{"none", "minimal", "low", "medium", "high", "xhigh", "max"} {
		t.Run(effort, func(t *testing.T) {
			err := (Request{Prompt: "x", Approval: "never", Effort: effort}).Validate()
			if err != nil {
				t.Fatalf("Validate() error = %v, want nil", err)
			}
		})
	}
}

func TestRequestRejectsUnknownEffort(t *testing.T) {
	err := (Request{Prompt: "x", Approval: "never", Effort: "turbo"}).Validate()
	if err == nil || !strings.Contains(err.Error(), "unknown effort") {
		t.Fatalf("Validate() error = %v, want unknown effort", err)
	}
}

type namedAdapter struct {
	name string
}

func (n namedAdapter) Name() string { return n.name }
func (n namedAdapter) Role() Role   { return RoleOrchestrable }
func (n namedAdapter) Detect(ctx context.Context) (Detection, error) {
	return Detection{Name: n.name, HeadlessCapable: true}, nil
}
func (n namedAdapter) Run(ctx context.Context, req Request) (Result, error) {
	return Result{}, nil
}
func (n namedAdapter) Review(ctx context.Context, req Request) (Verdict, error) {
	return Verdict{}, nil
}
