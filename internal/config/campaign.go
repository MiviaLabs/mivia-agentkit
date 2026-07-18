// Package config implements mivia-agent.yaml parsing.
// Plan: WS15. PRD: FR-4.2 bounds; supervised audit-repair campaign config.
package config

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/MiviaLabs/mivia-agentkit/internal/pathpolicy"
)

// Campaign is a named, disabled-by-default supervised audit-repair campaign.
// It is intentionally separate from config.Loop and must not overload Loop fields.
type Campaign struct {
	Enabled                      bool     `yaml:"enabled"`
	AuditWorkflow                string   `yaml:"audit_workflow"`
	FixWorkflow                  string   `yaml:"fix_workflow"`
	Auditor                      string   `yaml:"auditor"`
	Confirmer                    string   `yaml:"confirmer"`
	CleanPassThreshold           int      `yaml:"clean_pass_threshold"`
	MaxCycles                    int      `yaml:"max_cycles"`
	MaxDuration                  string   `yaml:"max_duration"`
	MaxRepairAttempts            int      `yaml:"max_repair_attempts"`
	NoProgressThreshold          int      `yaml:"no_progress_threshold"`
	CommitEnabled                bool     `yaml:"commit_enabled"`
	CommitMessageTemplate        string   `yaml:"commit_message_template"`
	VerifierProfile              string   `yaml:"verifier_profile"`
	AllowedPaths                 []string `yaml:"allowed_paths"`
	OnExhausted                  string   `yaml:"on_exhausted"`
	RequireInteractiveContinuous *bool    `yaml:"require_interactive_continuous"`
}

// CampaignDefaults returns a disabled campaign with safe finite limits.
func CampaignDefaults() Campaign {
	req := true
	return Campaign{
		Enabled:                      false,
		CleanPassThreshold:           2,
		MaxCycles:                    10,
		MaxDuration:                  "2h",
		MaxRepairAttempts:            3,
		NoProgressThreshold:          2,
		CommitEnabled:                false,
		OnExhausted:                  "fail",
		RequireInteractiveContinuous: &req,
	}
}

// Validate checks campaign enablement, independence, and finite bounds.
func (c *Campaign) Validate(name string, enabledAdapters map[string]AdapterRole, knownWorkflows map[string]struct{}) error {
	if c == nil {
		return fmt.Errorf("campaign %q is nil", name)
	}
	if c.CleanPassThreshold == 0 {
		c.CleanPassThreshold = 2
	}
	if c.MaxCycles == 0 {
		c.MaxCycles = 10
	}
	if c.MaxDuration == "" {
		c.MaxDuration = "2h"
	}
	if c.MaxRepairAttempts == 0 {
		c.MaxRepairAttempts = 3
	}
	if c.NoProgressThreshold == 0 {
		c.NoProgressThreshold = 2
	}
	if c.OnExhausted == "" {
		c.OnExhausted = "fail"
	}
	if c.RequireInteractiveContinuous == nil {
		req := true
		c.RequireInteractiveContinuous = &req
	}
	if c.Enabled && c.RequireInteractiveContinuous != nil && !*c.RequireInteractiveContinuous {
		return fmt.Errorf("campaign %q rejects non-interactive continuous (require_interactive_continuous must be true)", name)
	}
	if c.CleanPassThreshold < 2 {
		return fmt.Errorf("campaign %q clean_pass_threshold must be >= 2", name)
	}
	if c.MaxCycles <= 0 {
		return fmt.Errorf("campaign %q max_cycles must be positive", name)
	}
	if c.MaxRepairAttempts <= 0 {
		return fmt.Errorf("campaign %q max_repair_attempts must be positive", name)
	}
	if c.NoProgressThreshold <= 0 {
		return fmt.Errorf("campaign %q no_progress_threshold must be positive", name)
	}
	d, err := time.ParseDuration(c.MaxDuration)
	if err != nil {
		return fmt.Errorf("campaign %q max_duration %q is invalid: %w", name, c.MaxDuration, err)
	}
	if d <= 0 {
		return fmt.Errorf("campaign %q max_duration must be positive", name)
	}
	switch c.OnExhausted {
	case "fail", "warn":
	case "proceed":
		return fmt.Errorf("campaign %q on_exhausted=proceed is not allowed", name)
	default:
		return fmt.Errorf("campaign %q unknown on_exhausted %q", name, c.OnExhausted)
	}
	if err := validateCommitMessageTemplate(name, c.CommitMessageTemplate); err != nil {
		return err
	}
	if c.CommitEnabled {
		if strings.TrimSpace(c.VerifierProfile) == "" {
			return fmt.Errorf("campaign %q commit_enabled requires verifier_profile", name)
		}
		c.VerifierProfile = strings.TrimSpace(c.VerifierProfile)
		if err := validateVerifierProfile(name, c.VerifierProfile); err != nil {
			return err
		}
		if len(c.AllowedPaths) == 0 {
			return fmt.Errorf("campaign %q commit_enabled requires allowed_paths", name)
		}
		for i, p := range c.AllowedPaths {
			if err := validateCampaignPath(name, p); err != nil {
				return err
			}
			c.AllowedPaths[i] = filepath.ToSlash(filepath.Clean(strings.TrimSpace(p)))
		}
		if strings.TrimSpace(c.CommitMessageTemplate) == "" {
			return fmt.Errorf("campaign %q commit_enabled requires commit_message_template", name)
		}
		if c.Confirmer != "" && c.Auditor != "" && c.Confirmer == c.Auditor {
			return fmt.Errorf("campaign %q commit_enabled requires independent confirmer different from auditor", name)
		}
	}
	if !c.Enabled {
		return nil
	}
	if c.AuditWorkflow == "" {
		return fmt.Errorf("campaign %q audit_workflow is required when enabled", name)
	}
	if c.FixWorkflow == "" {
		return fmt.Errorf("campaign %q fix_workflow is required when enabled", name)
	}
	if _, ok := knownWorkflows[c.AuditWorkflow]; !ok {
		return fmt.Errorf("campaign %q references unknown audit_workflow %q", name, c.AuditWorkflow)
	}
	if _, ok := knownWorkflows[c.FixWorkflow]; !ok {
		return fmt.Errorf("campaign %q references unknown fix_workflow %q", name, c.FixWorkflow)
	}
	if c.Auditor == "" {
		return fmt.Errorf("campaign %q auditor is required when enabled", name)
	}
	if c.Confirmer == "" {
		return fmt.Errorf("campaign %q confirmer is required when enabled", name)
	}
	if err := requireOrchestrable(name, "auditor", c.Auditor, enabledAdapters); err != nil {
		return err
	}
	if err := requireOrchestrable(name, "confirmer", c.Confirmer, enabledAdapters); err != nil {
		return err
	}
	if c.CommitEnabled && c.Confirmer == c.Auditor {
		return fmt.Errorf("campaign %q commit_enabled requires independent confirmer different from auditor", name)
	}
	return nil
}

func requireOrchestrable(campaign, field, adapter string, enabled map[string]AdapterRole) error {
	role, ok := enabled[adapter]
	if !ok {
		return fmt.Errorf("campaign %q %s references unknown or disabled adapter %q", campaign, field, adapter)
	}
	if role != AdapterRoleOrchestrable {
		return fmt.Errorf("campaign %q %s adapter %q is not orchestrable", campaign, field, adapter)
	}
	return nil
}

func validateCommitMessageTemplate(name, tmpl string) error {
	if tmpl == "" {
		return nil
	}
	if strings.ContainsAny(tmpl, "\n\r") {
		return fmt.Errorf("campaign %q commit_message_template must be a single line", name)
	}
	if strings.Contains(tmpl, "$") {
		return fmt.Errorf("campaign %q commit_message_template contains unsafe token %q", name, "$")
	}
	lower := strings.ToLower(tmpl)
	for _, bad := range []string{"$(", "`", "${", "&&", "||", ";", "|", ">", "<"} {
		if strings.Contains(tmpl, bad) {
			return fmt.Errorf("campaign %q commit_message_template contains unsafe token %q", name, bad)
		}
	}
	for _, bad := range []string{"password", "secret", "token", "apikey", "api_key", "private_key"} {
		if strings.Contains(lower, bad) {
			return fmt.Errorf("campaign %q commit_message_template contains forbidden term %q", name, bad)
		}
	}
	return nil
}

func validateCampaignPath(name, p string) error {
	p = strings.TrimSpace(p)
	if p == "" {
		return fmt.Errorf("campaign %q allowed_paths contains empty path", name)
	}
	if strings.HasPrefix(p, "/") || strings.HasPrefix(p, "~") {
		return fmt.Errorf("campaign %q allowed_paths path %q must be repo-relative", name, p)
	}
	if strings.Contains(p, "..") {
		return fmt.Errorf("campaign %q allowed_paths path %q must not contain ..", name, p)
	}
	// Literal pathspecs only — git pathspec metacharacters would expand staging scope.
	if strings.ContainsAny(p, "*?[]") || strings.Contains(p, ":(") {
		return fmt.Errorf("campaign %q allowed_paths path %q must be a literal path (no globs)", name, p)
	}
	cleaned := filepath.ToSlash(filepath.Clean(p))
	if cleaned == "." || cleaned == "" {
		return fmt.Errorf("campaign %q allowed_paths path %q is invalid", name, p)
	}
	if strings.HasPrefix(cleaned, "../") || cleaned == ".." {
		return fmt.Errorf("campaign %q allowed_paths path %q must be repo-relative", name, p)
	}
	if cleaned == ".git" || strings.HasPrefix(cleaned, ".git/") {
		return fmt.Errorf("campaign %q allowed_paths path %q is denied", name, p)
	}
	if cleaned == ".ai/runs" || strings.HasPrefix(cleaned, ".ai/runs/") {
		return fmt.Errorf("campaign %q allowed_paths path %q is denied", name, p)
	}
	for _, seg := range strings.Split(cleaned, "/") {
		if seg == ".git" {
			return fmt.Errorf("campaign %q allowed_paths path %q is denied", name, p)
		}
	}
	// Apply default secret-aware path policy (.env, secrets/**, private keys).
	if err := pathpolicy.NewDefault().Check("", cleaned); err != nil {
		return fmt.Errorf("campaign %q allowed_paths path %q is denied: %w", name, p, err)
	}
	return nil
}

// validateVerifierProfile rejects multi-word free-form profiles that would
// otherwise be shell-expanded or silently no-op. Named profiles and single PATH tokens only.
func validateVerifierProfile(name, profile string) error {
	profile = strings.TrimSpace(profile)
	if profile == "" {
		return fmt.Errorf("campaign %q verifier_profile is empty", name)
	}
	switch profile {
	case "true", "noop", "true-cmd", "go-test":
		return nil
	}
	if strings.ContainsAny(profile, " \t\n\r") {
		return fmt.Errorf("campaign %q verifier_profile %q is multi-word; use a named profile (true, go-test) or a single PATH token", name, profile)
	}
	if strings.ContainsAny(profile, "*?[]{}") {
		return fmt.Errorf("campaign %q verifier_profile %q contains metacharacters", name, profile)
	}
	return nil
}
