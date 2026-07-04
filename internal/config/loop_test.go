// Package config tests loop parsing and validation.
// Plan: WS-A. PRD: FR-4.2, FR-6.1.
package config

import (
	"strings"
	"testing"
)

func TestLoopParsesStepModelOverrides(t *testing.T) {
	got, err := Parse([]byte(`
adapters:
  codex:
    enabled: true
    role: orchestrable
loops:
  research:
    bound: iterations
    max_iterations: 2
    steps:
      - id: produce
        producer: codex
        model: gpt-5.5
        effort: xhigh
`))
	if err != nil {
		t.Fatalf("Parse() error = %v, want nil", err)
	}
	step := got.Loops["research"].Steps[0]
	if step.Model != "gpt-5.5" {
		t.Fatalf("Model = %q, want gpt-5.5", step.Model)
	}
	if step.Effort != "xhigh" {
		t.Fatalf("Effort = %q, want xhigh", step.Effort)
	}
}

func TestLoopValidateRejectsUnknownAdapter(t *testing.T) {
	loop := validLoop()
	loop.Steps[0].Producer = "missing"
	if err := loop.Validate(enabledAdapters()); err == nil || !strings.Contains(err.Error(), "unknown adapter") {
		t.Fatalf("Validate() error = %v, want unknown adapter", err)
	}
}

func TestLoopValidateRejectsGuidanceAdapterAsProducer(t *testing.T) {
	loop := validLoop()
	loop.Steps[0].Producer = "copilot"
	if err := loop.Validate(enabledAdapters()); err == nil || !strings.Contains(err.Error(), "not orchestrable") {
		t.Fatalf("Validate() error = %v, want guidance producer rejection", err)
	}
}

func TestLoopValidateRejectsNonPositiveMaxIterations(t *testing.T) {
	loop := validLoop()
	loop.MaxIterations = 0
	if err := loop.Validate(enabledAdapters()); err == nil || !strings.Contains(err.Error(), "positive") {
		t.Fatalf("Validate() error = %v, want positive max_iterations rejection", err)
	}
}

func TestLoopValidateRejectsDuplicateStepIDs(t *testing.T) {
	loop := validLoop()
	loop.Steps = append(loop.Steps, Step{ID: "produce", Producer: "claude"})
	if err := loop.Validate(enabledAdapters()); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("Validate() error = %v, want duplicate step rejection", err)
	}
}

func TestLoopValidateDefaultsOnExhaustedToFailForProtectBoundLoop(t *testing.T) {
	loop := validLoop()
	loop.ExitWhen = "protected_action"
	loop.OnExhausted = ""
	if err := loop.Validate(enabledAdapters()); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
	if loop.OnExhausted != "fail" {
		t.Fatalf("OnExhausted = %q, want fail", loop.OnExhausted)
	}
}

func TestLoopValidateDefaultsOnExhaustedToWarnForNonProtectLoop(t *testing.T) {
	loop := validLoop()
	loop.OnExhausted = ""
	if err := loop.Validate(enabledAdapters()); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
	if loop.OnExhausted != "warn" {
		t.Fatalf("OnExhausted = %q, want warn", loop.OnExhausted)
	}
}

func enabledAdapters() map[string]AdapterRole {
	return map[string]AdapterRole{
		"codex":   AdapterRoleOrchestrable,
		"claude":  AdapterRoleOrchestrable,
		"copilot": AdapterRoleGuidance,
	}
}
