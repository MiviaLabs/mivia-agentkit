// Package cli implements the mivia-agent command surface.
// Plan: WS13. PRD: FR-4.1, FR-5.3.
package cli

import (
	"testing"

	"github.com/MiviaLabs/mivia-agentkit/internal/adapter"
	"github.com/MiviaLabs/mivia-agentkit/internal/config"
)

func TestProducerPromptIncludesPriorNotesOnIterate(t *testing.T) {
	builder := PromptBuilder{Vars: map[string]string{"project": "demo"}}
	got, err := builder.Producer(config.Step{Artifact: "out.md"}, []adapter.Verdict{{Adapter: "claude", Notes: "tighten tests"}})
	if err != nil {
		t.Fatalf("Producer() error = %v", err)
	}
	if !containsAll(got, "Prior reviewer notes", "claude", "tighten tests") {
		t.Fatalf("Producer() = %q, want prior review notes", got)
	}
}

func TestReviewerPromptRequestsJSONVerdict(t *testing.T) {
	builder := PromptBuilder{Vars: map[string]string{"project": "demo"}}
	got, err := builder.Reviewer(config.Step{Artifact: "out.md"}, "out.md")
	if err != nil {
		t.Fatalf("Reviewer() error = %v", err)
	}
	if !containsAll(got, "Return JSON only", "\"pass\":bool", "out.md") {
		t.Fatalf("Reviewer() = %q, want strict JSON verdict request", got)
	}
}

func TestProducerPromptErrorsOnUndefinedVar(t *testing.T) {
	repo := t.TempDir()
	mustWrite(t, repo+"/.ai/workflows/prompts/producer.tmpl", "hello {{.Vars.missing}}")
	builder := PromptBuilder{Repo: repo, Vars: map[string]string{"project": "demo"}}
	if _, err := builder.Producer(config.Step{}, nil); err == nil {
		t.Fatalf("Producer() error = nil, want undefined var error")
	}
}
