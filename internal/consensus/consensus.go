// Package consensus evaluates review verdicts against deterministic voting policy.
// Plan: WS11. PRD: FR-5.2, FR-6.4.
package consensus

import "strings"

// Mode selects how reviewer verdicts are counted.
type Mode string

const (
	// Majority passes when more than half of reviewers pass.
	Majority Mode = "majority"
	// Unanimous passes only when every reviewer passes.
	Unanimous Mode = "unanimous"
	// Weighted passes when passing reviewer weight reaches the threshold.
	Weighted Mode = "weighted"
	// FirstPass passes when any reviewer passes.
	FirstPass Mode = "first-pass"
)

// TieBreaker selects how tied outcomes are resolved.
type TieBreaker string

const (
	// Strict fails tied outcomes.
	Strict TieBreaker = "strict"
	// Manual fails tied outcomes and asks the caller to route manual resolution.
	Manual TieBreaker = "manual"
	// PreferPrefix is the prefix for prefer:<adapter> tie breakers.
	PreferPrefix TieBreaker = "prefer:"
)

// Policy configures deterministic consensus over adapter verdicts.
type Policy struct {
	Mode         Mode
	MinReviewers int
	Weights      map[string]float64
	TieBreaker   TieBreaker
	Threshold    float64
}

// Outcome is the consensus decision and its counted reviewers.
type Outcome struct {
	Pass    bool
	Reason  string
	For     []string
	Against []string
	Tied    bool
}

func weightFor(weights map[string]float64, adapter string) float64 {
	if weights == nil {
		return 1
	}
	if weight, ok := weights[adapter]; ok {
		return weight
	}
	return 1
}

func preferredAdapter(t TieBreaker) (string, bool) {
	raw := string(t)
	if !strings.HasPrefix(raw, string(PreferPrefix)) {
		return "", false
	}
	name := strings.TrimPrefix(raw, string(PreferPrefix))
	return name, name != ""
}
