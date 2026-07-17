// Package consensus evaluates review verdicts against deterministic voting policy.
// Plan: WS11. PRD: FR-5.2, FR-6.4.
package consensus

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/MiviaLabs/mivia-agentkit/internal/adapter"
)

// ErrMinReviewersUnsatisfied is returned when too few unique reviewers voted.
var ErrMinReviewersUnsatisfied = errors.New("min_reviewers unsatisfied")

// Evaluate applies a consensus policy to adapter verdicts.
func Evaluate(p Policy, verdicts []adapter.Verdict) (Outcome, error) {
	if p.Mode == "" {
		p.Mode = Majority
	}
	if p.TieBreaker == "" {
		p.TieBreaker = Strict
	}
	unique, deduped := dedupeVerdicts(verdicts)
	if p.MinReviewers > 0 && len(unique) < p.MinReviewers {
		return Outcome{}, fmt.Errorf("%w: got %d want %d", ErrMinReviewersUnsatisfied, len(unique), p.MinReviewers)
	}

	out := classify(unique)
	if deduped {
		out.Reason = "duplicate adapter verdicts deduped by name"
	}

	switch p.Mode {
	case Majority:
		pass, tied := majority(out)
		out.Pass, out.Tied = pass, tied
	case Unanimous:
		out.Pass = len(unique) > 0 && len(out.Against) == 0
	case Weighted:
		out.Pass, out.Tied = weighted(p, unique)
	case FirstPass:
		out.Pass = len(out.For) > 0
	default:
		return Outcome{}, fmt.Errorf("unknown consensus mode %q", p.Mode)
	}
	if out.Tied {
		out = breakTie(p, unique, out)
	}
	if out.Reason == "" {
		out.Reason = reasonFor(out.Pass)
	}
	return out, nil
}

func dedupeVerdicts(verdicts []adapter.Verdict) ([]adapter.Verdict, bool) {
	positions := map[string]int{}
	var unique []adapter.Verdict
	deduped := false
	for _, verdict := range verdicts {
		name := verdict.Adapter
		if name == "" {
			name = "unknown"
		}
		verdict.Adapter = name
		if pos, ok := positions[name]; ok {
			unique[pos] = verdict
			deduped = true
			continue
		}
		positions[name] = len(unique)
		unique = append(unique, verdict)
	}
	return unique, deduped
}

func classify(verdicts []adapter.Verdict) Outcome {
	out := Outcome{}
	for _, verdict := range verdicts {
		if verdict.Pass {
			out.For = append(out.For, verdict.Adapter)
		} else {
			out.Against = append(out.Against, verdict.Adapter)
		}
	}
	sort.Strings(out.For)
	sort.Strings(out.Against)
	return out
}

func majority(out Outcome) (bool, bool) {
	passers := len(out.For)
	against := len(out.Against)
	total := passers + against
	pass := passers*2 > total
	return pass, !pass && total > 0 && passers == against
}

func weighted(p Policy, verdicts []adapter.Verdict) (bool, bool) {
	var total, passed float64
	for _, verdict := range verdicts {
		weight := weightFor(p.Weights, verdict.Adapter)
		total += weight
		if verdict.Pass {
			passed += weight
		}
	}
	// Fail closed: empty verdicts or all-zero weights must never pass (0 >= 0).
	if total <= 0 {
		return false, false
	}
	threshold := p.Threshold
	if threshold == 0 {
		threshold = 0.5 * total
	}
	return passed >= threshold, false
}

func breakTie(p Policy, verdicts []adapter.Verdict, out Outcome) Outcome {
	reason := func(tieReason string) string {
		if out.Reason == "" {
			return tieReason
		}
		return out.Reason + "; " + tieReason
	}
	switch {
	case p.TieBreaker == Manual:
		out.Pass = false
		out.Reason = reason("tie: requires manual resolution")
	case strings.HasPrefix(string(p.TieBreaker), string(PreferPrefix)):
		preferred, ok := preferredAdapter(p.TieBreaker)
		if ok {
			for _, verdict := range verdicts {
				if verdict.Adapter == preferred {
					out.Pass = verdict.Pass
					out.Reason = reason("tie: preferred adapter " + preferred)
					return out
				}
			}
		}
		out.Pass = false
		out.Reason = reason("tie: strict fails")
	default:
		out.Pass = false
		out.Reason = reason("tie: strict fails")
	}
	return out
}

func reasonFor(pass bool) string {
	if pass {
		return "consensus passed"
	}
	return "consensus failed"
}
