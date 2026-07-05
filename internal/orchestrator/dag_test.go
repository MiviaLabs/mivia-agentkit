// Package orchestrator resolves and executes bounded agent loops.
// Plan: WS10. PRD: FR-4.1, FR-6.3.
package orchestrator

import (
	"testing"

	"github.com/MiviaLabs/mivia-agentkit/internal/config"
)

func TestDAGResolvesSequentialHandoff(t *testing.T) {
	nodes, err := Resolve(config.Loop{Steps: []config.Step{{ID: "produce", Producer: "codex"}, {ID: "review", Reviewers: []string{"claude"}}}})
	if err != nil {
		t.Fatalf("Resolve error = %v", err)
	}
	if got := nodes[1].DependsOn; len(got) != 1 || got[0] != "produce" {
		t.Fatalf("review deps = %v, want [produce]", got)
	}
}

func TestDAGRejectsCycle(t *testing.T) {
	nodes := Nodes{{Step: config.Step{ID: "a", Producer: "codex"}, DependsOn: []string{"b"}}, {Step: config.Step{ID: "b", Producer: "claude"}, DependsOn: []string{"a"}}}
	if err := nodes.Validate(); err == nil {
		t.Fatalf("Validate cycle error = nil, want error")
	}
}

func TestDAGRejectsDanglingDep(t *testing.T) {
	if err := (Nodes{{Step: config.Step{ID: "a", Producer: "codex"}, DependsOn: []string{"missing"}}}).Validate(); err == nil {
		t.Fatalf("Validate dangling dep error = nil, want error")
	}
}

func TestDAGRejectsProducerAndReviewOnSameStep(t *testing.T) {
	if err := (Nodes{{Step: config.Step{ID: "mixed", Producer: "codex", Reviewers: []string{"claude"}}}}).Validate(); err == nil {
		t.Fatalf("Validate mixed step error = nil, want error")
	}
}

func TestDAGRejectsMissingStepID(t *testing.T) {
	if err := (Nodes{{Step: config.Step{Producer: "codex"}}}).Validate(); err == nil {
		t.Fatalf("Validate missing id error = nil, want error")
	}
}
