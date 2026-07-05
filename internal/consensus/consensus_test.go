// Package consensus evaluates review verdicts against deterministic voting policy.
// Plan: WS11. PRD: FR-5.2, FR-6.4.
package consensus

import "testing"

func TestWeightDefaultsToOne(t *testing.T) {
	if got := weightFor(nil, "codex"); got != 1 {
		t.Fatalf("weightFor(nil) = %v, want 1", got)
	}
	if got := weightFor(map[string]float64{"claude": 2}, "codex"); got != 1 {
		t.Fatalf("weightFor(missing) = %v, want 1", got)
	}
}

func TestPreferredAdapterParsesNamedTieBreaker(t *testing.T) {
	got, ok := preferredAdapter(TieBreaker("prefer:codex"))
	if !ok || got != "codex" {
		t.Fatalf("preferredAdapter() = %q, %v, want codex, true", got, ok)
	}
}
