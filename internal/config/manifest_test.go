// Package config tests manifest parsing and validation.
// Plan: WS-A. PRD: FR-1.1, FR-4.2, FR-10.2.
package config

import (
	"strings"
	"testing"
)

func TestManifestDefaultsIncludeRoutingAndLoopDefaults(t *testing.T) {
	got := Defaults()
	if got.Profile != "standard" {
		t.Fatalf("Profile = %q, want standard", got.Profile)
	}
	if !got.Adapters["codex"].Enabled || got.Adapters["codex"].Role != AdapterRoleOrchestrable {
		t.Fatalf("codex adapter = %+v, want enabled orchestrable", got.Adapters["codex"])
	}
	if got.Adapters["copilot"].Role != AdapterRoleGuidance {
		t.Fatalf("copilot role = %q, want guidance", got.Adapters["copilot"].Role)
	}
	if got.Adapters["crush"].Role != AdapterRoleGuidance {
		t.Fatalf("crush role = %q, want guidance", got.Adapters["crush"].Role)
	}
	if got.Routing.Consensus.Mode != "majority" || got.Routing.Consensus.MinReviewers != 2 {
		t.Fatalf("consensus = %+v, want majority min 2", got.Routing.Consensus)
	}
	if got.Routing.MaxIterations != 3 || got.Routing.OnReviewFail != "iterate" {
		t.Fatalf("routing = %+v, want max iterations 3 and iterate", got.Routing)
	}
	if got.Governance.Provider != "noop" {
		t.Fatalf("governance provider = %q, want noop", got.Governance.Provider)
	}
}

func TestManifestParseDoesNotEnableUnlistedDefaultAdapters(t *testing.T) {
	got, err := Parse([]byte(`
version: "1"
profile: standard
adapters:
  codex:
    enabled: true
    role: orchestrable
`))
	if err != nil {
		t.Fatalf("Parse() error = %v, want nil", err)
	}
	if _, ok := got.Adapters["copilot"]; ok {
		t.Fatalf("Adapters includes copilot = %+v, want only explicit adapters", got.Adapters)
	}
}

func TestManifestParsesAdapterModelDefaults(t *testing.T) {
	got, err := Parse([]byte(`
version: "1"
profile: standard
adapters:
  codex:
    enabled: true
    role: orchestrable
    model: gpt-5.5
    effort: high
`))
	if err != nil {
		t.Fatalf("Parse() error = %v, want nil", err)
	}
	if got.Adapters["codex"].Model != "gpt-5.5" {
		t.Fatalf("Model = %q, want gpt-5.5", got.Adapters["codex"].Model)
	}
	if got.Adapters["codex"].Effort != "high" {
		t.Fatalf("Effort = %q, want high", got.Adapters["codex"].Effort)
	}
}

func TestManifestAcceptsDocumentedEffortVariants(t *testing.T) {
	for _, effort := range []string{"none", "minimal", "low", "medium", "high", "xhigh", "max"} {
		t.Run(effort, func(t *testing.T) {
			m := Defaults()
			m.Adapters["codex"] = AdapterConfig{
				Enabled: true,
				Role:    AdapterRoleOrchestrable,
				Effort:  effort,
			}
			if err := m.Validate(); err != nil {
				t.Fatalf("Validate() error = %v, want nil", err)
			}
		})
	}
}

func TestManifestRejectsUnknownProfile(t *testing.T) {
	m := Defaults()
	m.Profile = "enterprise"
	if err := m.Validate(); err == nil || !strings.Contains(err.Error(), "unknown profile") {
		t.Fatalf("Validate() error = %v, want unknown profile", err)
	}
}

func TestManifestRejectsUnknownAdapterRole(t *testing.T) {
	m := Defaults()
	m.Adapters["codex"] = AdapterConfig{Enabled: true, Role: "chatty"}
	if err := m.Validate(); err == nil || !strings.Contains(err.Error(), "unknown role") {
		t.Fatalf("Validate() error = %v, want unknown role", err)
	}
}

func TestManifestRejectsUnknownEffort(t *testing.T) {
	tests := []struct {
		name     string
		manifest Manifest
	}{
		{
			name: "adapter default",
			manifest: func() Manifest {
				m := Defaults()
				m.Adapters["codex"] = AdapterConfig{
					Enabled: true,
					Role:    AdapterRoleOrchestrable,
					Effort:  "turbo",
				}
				return m
			}(),
		},
		{
			name: "step override",
			manifest: func() Manifest {
				m := Defaults()
				m.Loops = map[string]Loop{"research": validLoop()}
				loop := m.Loops["research"]
				loop.Steps[0].Effort = "turbo"
				m.Loops["research"] = loop
				return m
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.manifest.Validate(); err == nil || !strings.Contains(err.Error(), "unknown effort") {
				t.Fatalf("Validate() error = %v, want unknown effort", err)
			}
		})
	}
}

func TestManifestRejectsBudgetBoundInMVP(t *testing.T) {
	m := Defaults()
	m.Loops = map[string]Loop{"research": validLoop()}
	loop := m.Loops["research"]
	loop.Bound = "budget"
	m.Loops["research"] = loop
	if err := m.Validate(); err == nil || !strings.Contains(err.Error(), "budget") {
		t.Fatalf("Validate() error = %v, want budget rejection", err)
	}
}

func TestManifestRejectsExpertProfileInMVP(t *testing.T) {
	m := Defaults()
	m.Profile = "expert"
	if err := m.Validate(); err == nil || !strings.Contains(err.Error(), "expert") {
		t.Fatalf("Validate() error = %v, want expert rejection", err)
	}
}

func TestManifestRejectsUnknownConsensusMode(t *testing.T) {
	m := Defaults()
	m.Routing.Consensus.Mode = "coinflip"
	if err := m.Validate(); err == nil || !strings.Contains(err.Error(), "consensus") {
		t.Fatalf("Validate() error = %v, want consensus rejection", err)
	}
}

func TestManifestRejectsUnknownYAMLField(t *testing.T) {
	_, err := Parse([]byte("profile: standard\nsurprise: true\n"))
	if err == nil || !strings.Contains(err.Error(), "field surprise not found") {
		t.Fatalf("Parse() error = %v, want unknown field rejection", err)
	}
}

func TestManifestParsesCrushParams(t *testing.T) {
	got, err := Parse([]byte(`
version: "1"
profile: standard
adapters:
  crush:
    enabled: true
    role: guidance
    model: openai/gpt-5.5
    params:
      provider: openai
      base_url: https://api.openai.com/v1
`))
	if err != nil {
		t.Fatalf("Parse() error = %v, want nil", err)
	}
	if got.Adapters["crush"].Model != "openai/gpt-5.5" {
		t.Fatalf("Model = %q, want openai/gpt-5.5", got.Adapters["crush"].Model)
	}
	if got.Adapters["crush"].Params["provider"] != "openai" {
		t.Fatalf("provider = %q, want openai", got.Adapters["crush"].Params["provider"])
	}
	if got.Adapters["crush"].Params["base_url"] != "https://api.openai.com/v1" {
		t.Fatalf("base_url = %q, want https://api.openai.com/v1", got.Adapters["crush"].Params["base_url"])
	}
}

func TestManifestParseAppliesLoopDefaults(t *testing.T) {
	got, err := Parse([]byte(`
loops:
  release:
    bound: iterations
    max_iterations: 1
    exit_when: protected_action
    steps:
      - id: produce
        producer: codex
`))
	if err != nil {
		t.Fatalf("Parse() error = %v, want nil", err)
	}
	if got.Loops["release"].OnExhausted != "fail" {
		t.Fatalf("OnExhausted = %q, want fail", got.Loops["release"].OnExhausted)
	}
}

func validLoop() Loop {
	return Loop{
		Bound:         "iterations",
		MaxIterations: 2,
		Steps: []Step{{
			ID:       "produce",
			Producer: "codex",
		}},
	}
}
