// Package auditcampaign implements the supervised audit-repair campaign runtime.
// Plan: WS15. PRD: measurement integrity.
package auditcampaign

import (
	"fmt"
	"time"
)

// PhaseMetrics is a runtime-owned measurement for one campaign phase.
type PhaseMetrics struct {
	// Source must be "runtime" for trusted measurements.
	Source      string    `json:"source"`
	Version     string    `json:"version"`
	RunID       string    `json:"run_id"`
	Cycle       int       `json:"cycle"`
	StepID      string    `json:"step_id"`
	StartedAt   time.Time `json:"started_at"`
	FinishedAt  time.Time `json:"finished_at"`
	ElapsedMS   int64     `json:"elapsed_ms"`
	Outcome     string    `json:"outcome"`
	TokenSource string    `json:"token_source"`
	// ProviderTokens is set only when TokenSource is provider.
	ProviderTokens *int64 `json:"provider_tokens,omitempty"`
}

// MetricsVersion is the metric contract version.
const MetricsVersion = "mivia-agent-metrics/v1"

// MeasurePhase records elapsed time around an operation using the process clock.
func MeasurePhase(runID string, cycle int, stepID string, outcome string, started time.Time, finished time.Time, providerTokens *int64) (PhaseMetrics, error) {
	if finished.Before(started) {
		return PhaseMetrics{}, fmt.Errorf("finished_at before started_at")
	}
	elapsed := finished.Sub(started).Milliseconds()
	if elapsed < 0 {
		return PhaseMetrics{}, fmt.Errorf("elapsed_ms must be non-negative")
	}
	m := PhaseMetrics{
		Source:     "runtime",
		Version:    MetricsVersion,
		RunID:      runID,
		Cycle:      cycle,
		StepID:     stepID,
		StartedAt:  started.UTC(),
		FinishedAt: finished.UTC(),
		ElapsedMS:  elapsed,
		Outcome:    outcome,
	}
	if providerTokens != nil {
		m.TokenSource = "provider"
		m.ProviderTokens = providerTokens
	} else {
		m.TokenSource = "unavailable"
	}
	return m, nil
}

// Validate checks metric ordering and provenance.
func (m PhaseMetrics) Validate() error {
	if m.Source != "runtime" {
		return fmt.Errorf("metrics source must be runtime, got %q", m.Source)
	}
	if m.Version != MetricsVersion {
		return fmt.Errorf("unknown metrics version %q", m.Version)
	}
	if m.FinishedAt.Before(m.StartedAt) {
		return fmt.Errorf("finished_at before started_at")
	}
	if m.ElapsedMS < 0 {
		return fmt.Errorf("elapsed_ms must be non-negative")
	}
	if m.TokenSource != "provider" && m.TokenSource != "unavailable" {
		return fmt.Errorf("unknown token_source %q", m.TokenSource)
	}
	if m.TokenSource == "provider" && m.ProviderTokens == nil {
		return fmt.Errorf("provider token_source requires provider_tokens")
	}
	if m.TokenSource == "unavailable" && m.ProviderTokens != nil {
		return fmt.Errorf("unavailable token_source must not set provider_tokens")
	}
	return nil
}

// AggregateWallElapsed returns wall elapsed from earliest start to latest finish.
// Parallel phases must not be summed as sequential wall time.
func AggregateWallElapsed(phases []PhaseMetrics) (int64, error) {
	if len(phases) == 0 {
		return 0, fmt.Errorf("no phases")
	}
	start := phases[0].StartedAt
	end := phases[0].FinishedAt
	for _, p := range phases {
		if err := p.Validate(); err != nil {
			return 0, err
		}
		if p.StartedAt.Before(start) {
			start = p.StartedAt
		}
		if p.FinishedAt.After(end) {
			end = p.FinishedAt
		}
	}
	return end.Sub(start).Milliseconds(), nil
}

// RenderMeasuredElapsed returns a display value or NOT_MEASURED.
func RenderMeasuredElapsed(m *PhaseMetrics) string {
	if m == nil || m.Source != "runtime" {
		return "NOT_MEASURED"
	}
	return fmt.Sprintf("%dms", m.ElapsedMS)
}
