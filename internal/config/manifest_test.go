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
