// Package consensus evaluates review verdicts against deterministic voting policy.
// Plan: WS11. PRD: FR-5.2, FR-6.4.
package consensus

import (
	"errors"
	"strings"
	"testing"

	"github.com/MiviaLabs/mivia-agentkit/internal/adapter"
)

func TestMajorityPassesAboveThreshold(t *testing.T) {
	out := mustEvaluate(t, Policy{Mode: Majority}, verdict("a", true), verdict("b", true), verdict("c", false))
	if !out.Pass {
		t.Fatalf("Pass = false, want true")
	}
}

func TestMajorityFailsAtOrBelowHalf(t *testing.T) {
	out := mustEvaluate(t, Policy{Mode: Majority}, verdict("a", true), verdict("b", true), verdict("c", false), verdict("d", false))
	if out.Pass {
		t.Fatalf("Pass = true, want false for 2-of-4")
	}
}

func TestMajorityDedupsByAdapterName(t *testing.T) {
	out := mustEvaluate(t, Policy{Mode: Majority}, verdict("codex", true), verdict("claude", true), verdict("codex", false))
	if out.Pass {
		t.Fatalf("Pass = true, want false after last codex vote wins")
	}
	if !strings.Contains(out.Reason, "deduped") {
		t.Fatalf("Reason = %q, want dedupe warning", out.Reason)
	}
}

func TestUnanimousFailsOnOneReject(t *testing.T) {
	out := mustEvaluate(t, Policy{Mode: Unanimous}, verdict("a", true), verdict("b", false))
	if out.Pass {
		t.Fatalf("Pass = true, want false")
	}
}

func TestUnanimousPassesAllPass(t *testing.T) {
	out := mustEvaluate(t, Policy{Mode: Unanimous}, verdict("a", true), verdict("b", true))
	if !out.Pass {
		t.Fatalf("Pass = false, want true")
	}
}

func TestWeightedRespectsWeights(t *testing.T) {
	p := Policy{Mode: Weighted, Weights: map[string]float64{"codex": 3, "claude": 1}, Threshold: 3}
	out := mustEvaluate(t, p, verdict("codex", true), verdict("claude", false))
	if !out.Pass {
		t.Fatalf("Pass = false, want true when codex weight reaches threshold")
	}
}

func TestWeightedThresholdDefaultsToHalf(t *testing.T) {
	out := mustEvaluate(t, Policy{Mode: Weighted}, verdict("a", true), verdict("b", false))
	if !out.Pass {
		t.Fatalf("Pass = false, want true at default half threshold")
	}
}

func TestFirstPassPassesOnOne(t *testing.T) {
	out := mustEvaluate(t, Policy{Mode: FirstPass}, verdict("a", false), verdict("b", true))
	if !out.Pass {
		t.Fatalf("Pass = false, want true")
	}
}

func TestFirstPassFailsOnNone(t *testing.T) {
	out := mustEvaluate(t, Policy{Mode: FirstPass}, verdict("a", false), verdict("b", false))
	if out.Pass {
		t.Fatalf("Pass = true, want false")
	}
}

func TestMinReviewersUnsatisfiedErrors(t *testing.T) {
	_, err := Evaluate(Policy{Mode: Majority, MinReviewers: 2}, []adapter.Verdict{verdict("a", true)})
	if !errors.Is(err, ErrMinReviewersUnsatisfied) {
		t.Fatalf("Evaluate() error = %v, want ErrMinReviewersUnsatisfied", err)
	}
}

func TestTieBreakerStrictFailsOnTie(t *testing.T) {
	out := mustEvaluate(t, Policy{Mode: Majority, TieBreaker: Strict}, verdict("a", true), verdict("b", false))
	if out.Pass || out.Reason != "tie: strict fails" {
		t.Fatalf("Outcome = %+v, want strict tie failure", out)
	}
}

func TestTieBreakerManualFailsOnTie(t *testing.T) {
	out := mustEvaluate(t, Policy{Mode: Majority, TieBreaker: Manual}, verdict("a", true), verdict("b", false))
	if out.Pass || out.Reason != "tie: requires manual resolution" {
		t.Fatalf("Outcome = %+v, want manual tie failure", out)
	}
}

func TestTieBreakerPrefersNamedAdapter(t *testing.T) {
	out := mustEvaluate(t, Policy{Mode: Majority, TieBreaker: TieBreaker("prefer:codex")}, verdict("codex", true), verdict("claude", false))
	if !out.Pass || !strings.Contains(out.Reason, "preferred adapter codex") {
		t.Fatalf("Outcome = %+v, want preferred codex pass", out)
	}
}

func TestTieBreakerPreferFallsBackToStrictWhenAbsent(t *testing.T) {
	out := mustEvaluate(t, Policy{Mode: Majority, TieBreaker: TieBreaker("prefer:gemini")}, verdict("codex", true), verdict("claude", false))
	if out.Pass || out.Reason != "tie: strict fails" {
		t.Fatalf("Outcome = %+v, want strict fallback", out)
	}
}

func mustEvaluate(t *testing.T, p Policy, verdicts ...adapter.Verdict) Outcome {
	t.Helper()
	out, err := Evaluate(p, verdicts)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	return out
}

func verdict(name string, pass bool) adapter.Verdict {
	return adapter.Verdict{Adapter: name, Pass: pass}
}
