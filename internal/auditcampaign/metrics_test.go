// Package auditcampaign tests runtime metrics.
// Plan: WS15.
package auditcampaign

import (
	"strings"
	"testing"
	"time"
)

func TestPhaseMetricsOrderedNonNegativeElapsed(t *testing.T) {
	start := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	end := start.Add(50 * time.Millisecond)
	m, err := MeasurePhase("run", 1, "audit", "ok", start, end, nil)
	if err != nil {
		t.Fatalf("MeasurePhase() error = %v", err)
	}
	if err := m.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if m.ElapsedMS < 0 {
		t.Fatalf("ElapsedMS = %d, want >= 0", m.ElapsedMS)
	}
	_, err = MeasurePhase("run", 1, "audit", "ok", end, start, nil)
	if err == nil {
		t.Fatalf("want error for inverted timestamps")
	}
}

func TestTokenSourceUnavailableWhenMissing(t *testing.T) {
	start := time.Now().UTC()
	m, err := MeasurePhase("run", 1, "audit", "ok", start, start.Add(time.Millisecond), nil)
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if m.TokenSource != "unavailable" {
		t.Fatalf("TokenSource = %q, want unavailable", m.TokenSource)
	}
	if m.ProviderTokens != nil {
		t.Fatalf("ProviderTokens must be nil")
	}
	if RenderMeasuredElapsed(nil) != "NOT_MEASURED" {
		t.Fatalf("RenderMeasuredElapsed(nil) = %q", RenderMeasuredElapsed(nil))
	}
}

func TestParallelReviewAggregationDoesNotDoubleCount(t *testing.T) {
	start := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	// Two parallel 100ms reviews overlapping completely.
	a, _ := MeasurePhase("run", 1, "r1", "ok", start, start.Add(100*time.Millisecond), nil)
	b, _ := MeasurePhase("run", 1, "r2", "ok", start, start.Add(100*time.Millisecond), nil)
	wall, err := AggregateWallElapsed([]PhaseMetrics{a, b})
	if err != nil {
		t.Fatalf("AggregateWallElapsed() error = %v", err)
	}
	if wall != 100 {
		t.Fatalf("wall = %d, want 100 (not 200)", wall)
	}
	sum := a.ElapsedMS + b.ElapsedMS
	if wall >= sum && sum > 100 {
		t.Fatalf("wall must not equal summed overlapping elapsed")
	}
	if !strings.Contains(RenderMeasuredElapsed(&a), "ms") {
		t.Fatalf("render = %q", RenderMeasuredElapsed(&a))
	}
}
