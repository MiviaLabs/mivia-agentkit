// Package preflight writes and validates quality stamps.
// Plan: WS4. PRD: FR-2.4, FR-7.1.
package preflight

import "testing"

func TestClassifyHighForCIChange(t *testing.T) {
	if got := Classify([]string{".github/workflows/ci.yml"}, ContractMatrix{}); got != High {
		t.Fatalf("Classify() got %v want High", got)
	}
}

func TestClassifyHighForHookConfigChange(t *testing.T) {
	if got := Classify([]string{".codex/hooks.json"}, ContractMatrix{}); got != High {
		t.Fatalf("Classify() got %v want High", got)
	}
}

func TestClassifyLowForDocsOnly(t *testing.T) {
	if got := Classify([]string{"docs/readme.md"}, ContractMatrix{}); got != Low {
		t.Fatalf("Classify() got %v want Low", got)
	}
}

func TestClassifyMediumForCodeWithVerifier(t *testing.T) {
	if got := Classify([]string{"internal/preflight/risk.go"}, ContractMatrix{}); got != Medium {
		t.Fatalf("Classify() got %v want Medium", got)
	}
}
