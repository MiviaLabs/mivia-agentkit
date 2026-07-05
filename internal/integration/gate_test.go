// Package integration gates real runtime coverage on explicit opt-in and local prerequisites.
// Plan: WS14. PRD: §3, §9, §14.
package integration

import (
	"strings"
	"testing"
)

func TestGateRequireBinaryReportsMissingTool(t *testing.T) {
	status := DefaultGate().RequireBinary("mivia-agentkit-missing-tool-for-test")
	if status.Available {
		t.Fatalf("RequireBinary() Available = true, want false")
	}
	if !strings.Contains(status.Reason, "missing binary") || !strings.Contains(status.Reason, "mivia-agentkit-missing-tool-for-test") {
		t.Fatalf("RequireBinary() Reason = %q, want missing binary detail", status.Reason)
	}
}

func TestGateRequireEnvReportsMissingVariable(t *testing.T) {
	status := DefaultGate().RequireEnv("MIVIA_AGENTKIT_MISSING_ENV_FOR_TEST")
	if status.Available {
		t.Fatalf("RequireEnv() Available = true, want false")
	}
	if !strings.Contains(status.Reason, "missing environment variable") || !strings.Contains(status.Reason, "MIVIA_AGENTKIT_MISSING_ENV_FOR_TEST") {
		t.Fatalf("RequireEnv() Reason = %q, want missing env detail", status.Reason)
	}
}

func TestGateAllowsExplicitlyEnabledRun(t *testing.T) {
	gate := Gate{Enabled: true, OptInEnv: RealCLITestsEnv}
	ok, reason := gate.Allow(ToolStatus{Name: "codex", Available: true})
	if !ok {
		t.Fatalf("Allow() ok = false, want true, reason = %q", reason)
	}
	if reason != "" {
		t.Fatalf("Allow() reason = %q, want empty", reason)
	}
}
