// Package config tests campaign parsing and validation.
// Plan: WS15. PRD: FR-4.2 bounds; supervised audit-repair campaign config.
package config

import (
	"strings"
	"testing"
)

func enabledOrchestrable() map[string]AdapterRole {
	return map[string]AdapterRole{
		"codex":  AdapterRoleOrchestrable,
		"claude": AdapterRoleOrchestrable,
	}
}

func knownWorkflows() map[string]struct{} {
	return map[string]struct{}{
		"bug-audit-loop": {},
		"fix-loop":      {},
	}
}

func validEnabledCampaign() Campaign {
	c := CampaignDefaults()
	c.Enabled = true
	c.AuditWorkflow = "bug-audit-loop"
	c.FixWorkflow = "fix-loop"
	c.Auditor = "codex"
	c.Confirmer = "claude"
	return c
}

func TestCampaignDefaultsDisabled(t *testing.T) {
	c := CampaignDefaults()
	if c.Enabled {
		t.Fatalf("Enabled = true, want false by default")
	}
	if c.CleanPassThreshold < 2 {
		t.Fatalf("CleanPassThreshold = %d, want >= 2", c.CleanPassThreshold)
	}
	if err := c.Validate("deep-bug-audit-repair", enabledOrchestrable(), knownWorkflows()); err != nil {
		t.Fatalf("Validate() error = %v, want nil for disabled default", err)
	}
}

func TestCampaignRejectsMissingIndependentConfirmer(t *testing.T) {
	c := validEnabledCampaign()
	c.CommitEnabled = true
	c.VerifierProfile = "go-test"
	c.AllowedPaths = []string{"internal/config"}
	c.CommitMessageTemplate = "fix(quality): campaign repair"
	c.Confirmer = c.Auditor
	err := c.Validate("deep-bug-audit-repair", enabledOrchestrable(), knownWorkflows())
	if err == nil || !strings.Contains(err.Error(), "independent confirmer") {
		t.Fatalf("Validate() error = %v, want independent confirmer rejection", err)
	}
}

func TestCampaignRejectsNonPositiveLimits(t *testing.T) {
	cases := []struct {
		name string
		mut  func(*Campaign)
		want string
	}{
		{"max_cycles", func(c *Campaign) { c.MaxCycles = -1 }, "max_cycles"},
		{"max_duration", func(c *Campaign) { c.MaxDuration = "0s" }, "max_duration"},
		{"max_repair_attempts", func(c *Campaign) { c.MaxRepairAttempts = -5 }, "max_repair_attempts"},
		{"no_progress", func(c *Campaign) { c.NoProgressThreshold = -1 }, "no_progress_threshold"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := validEnabledCampaign()
			tc.mut(&c)
			err := c.Validate("c", enabledOrchestrable(), knownWorkflows())
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Validate() error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestCampaignRejectsCommitWithoutVerifierOrPathScope(t *testing.T) {
	c := validEnabledCampaign()
	c.CommitEnabled = true
	c.CommitMessageTemplate = "fix(quality): campaign repair"
	c.Confirmer = "claude"
	err := c.Validate("c", enabledOrchestrable(), knownWorkflows())
	if err == nil || !strings.Contains(err.Error(), "verifier_profile") {
		t.Fatalf("Validate() error = %v, want verifier_profile rejection", err)
	}
	c.VerifierProfile = "go-test"
	err = c.Validate("c", enabledOrchestrable(), knownWorkflows())
	if err == nil || !strings.Contains(err.Error(), "allowed_paths") {
		t.Fatalf("Validate() error = %v, want allowed_paths rejection", err)
	}
}

func TestCampaignRejectsCleanThresholdBelowTwo(t *testing.T) {
	c := validEnabledCampaign()
	c.CleanPassThreshold = 1
	err := c.Validate("c", enabledOrchestrable(), knownWorkflows())
	if err == nil || !strings.Contains(err.Error(), "clean_pass_threshold") {
		t.Fatalf("Validate() error = %v, want clean_pass_threshold rejection", err)
	}
}

func TestCampaignRejectsOnExhaustedProceed(t *testing.T) {
	c := validEnabledCampaign()
	c.OnExhausted = "proceed"
	err := c.Validate("c", enabledOrchestrable(), knownWorkflows())
	if err == nil || !strings.Contains(err.Error(), "proceed") {
		t.Fatalf("Validate() error = %v, want proceed rejection", err)
	}
}

func TestCampaignRejectsUnknownWorkflow(t *testing.T) {
	c := validEnabledCampaign()
	c.AuditWorkflow = "missing-workflow"
	err := c.Validate("c", enabledOrchestrable(), knownWorkflows())
	if err == nil || !strings.Contains(err.Error(), "unknown audit_workflow") {
		t.Fatalf("Validate() error = %v, want unknown audit_workflow", err)
	}
}

func TestCampaignRejectsUnsafeMessageTemplate(t *testing.T) {
	c := validEnabledCampaign()
	c.CommitMessageTemplate = "fix: $(rm -rf /)"
	err := c.Validate("c", enabledOrchestrable(), knownWorkflows())
	if err == nil || !strings.Contains(err.Error(), "unsafe token") {
		t.Fatalf("Validate() error = %v, want unsafe token rejection", err)
	}
}

func TestCampaignRejectsDeniedAllowedPaths(t *testing.T) {
	paths := []struct {
		path string
		want string
	}{
		{".ai/runs/foo", "denied"},
		{"./.ai/runs/foo", "denied"},
		{".ai//runs/foo", "denied"},
		{".ai/./runs/foo", "denied"},
		{".git", "denied"},
		{"./.git", "denied"},
		{"./.git/hooks", "denied"},
		{"foo/.git/config", "denied"},
		{"/abs/path", "repo-relative"},
		{"~/secrets", "repo-relative"},
		{"../escape", ".."},
	}
	for _, tc := range paths {
		t.Run(tc.path, func(t *testing.T) {
			c := validEnabledCampaign()
			c.CommitEnabled = true
			c.VerifierProfile = "go-test"
			c.CommitMessageTemplate = "fix(quality): campaign repair"
			c.AllowedPaths = []string{tc.path}
			err := c.Validate("c", enabledOrchestrable(), knownWorkflows())
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Validate() error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestCampaignRejectsNonInteractiveContinuous(t *testing.T) {
	c := validEnabledCampaign()
	f := false
	c.RequireInteractiveContinuous = &f
	err := c.Validate("c", enabledOrchestrable(), knownWorkflows())
	if err == nil || !strings.Contains(err.Error(), "non-interactive") {
		t.Fatalf("Validate() error = %v, want non-interactive rejection", err)
	}
}

func TestCampaignRejectsUnknownOrNonOrchestrableAdapters(t *testing.T) {
	c := validEnabledCampaign()
	c.Auditor = "missing"
	err := c.Validate("c", enabledOrchestrable(), knownWorkflows())
	if err == nil || !strings.Contains(err.Error(), "unknown or disabled adapter") {
		t.Fatalf("Validate() error = %v, want unknown auditor", err)
	}
	c = validEnabledCampaign()
	enabled := enabledOrchestrable()
	enabled["copilot"] = AdapterRoleGuidance
	c.Confirmer = "copilot"
	err = c.Validate("c", enabled, knownWorkflows())
	if err == nil || !strings.Contains(err.Error(), "not orchestrable") {
		t.Fatalf("Validate() error = %v, want non-orchestrable confirmer", err)
	}
}

func TestCampaignRejectsWhitespaceVerifierProfile(t *testing.T) {
	c := validEnabledCampaign()
	c.CommitEnabled = true
	c.VerifierProfile = "   "
	c.AllowedPaths = []string{"internal/config"}
	c.CommitMessageTemplate = "fix(quality): campaign repair"
	err := c.Validate("c", enabledOrchestrable(), knownWorkflows())
	if err == nil || !strings.Contains(err.Error(), "verifier_profile") {
		t.Fatalf("Validate() error = %v, want verifier_profile rejection", err)
	}
}

func TestCampaignValidatesCommitGuardsWhenDisabled(t *testing.T) {
	c := CampaignDefaults()
	c.Enabled = false
	c.CommitEnabled = true
	c.CommitMessageTemplate = "fix(quality): campaign repair"
	// Missing verifier/paths must still fail.
	err := c.Validate("c", enabledOrchestrable(), knownWorkflows())
	if err == nil || !strings.Contains(err.Error(), "verifier_profile") {
		t.Fatalf("Validate() error = %v, want verifier when commit_enabled", err)
	}
}

func TestCampaignRejectsSecretMessageTemplate(t *testing.T) {
	c := validEnabledCampaign()
	c.CommitMessageTemplate = "fix: rotate api_key material"
	err := c.Validate("c", enabledOrchestrable(), knownWorkflows())
	if err == nil || !strings.Contains(err.Error(), "forbidden term") {
		t.Fatalf("Validate() error = %v, want forbidden term rejection", err)
	}
}

func TestCampaignAcceptsCommitCapableIndependentSetup(t *testing.T) {
	c := validEnabledCampaign()
	c.CommitEnabled = true
	c.VerifierProfile = "go-test"
	c.AllowedPaths = []string{"internal/config"}
	c.CommitMessageTemplate = "fix(quality): campaign repair"
	if err := c.Validate("c", enabledOrchestrable(), knownWorkflows()); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
}
