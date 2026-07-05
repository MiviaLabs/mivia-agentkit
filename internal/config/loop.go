// Package config implements mivia-agent.yaml parsing.
// Plan: WS-A. PRD: FR-4.2, FR-6.1.
package config

import "fmt"

// Loop is a bounded workflow definition.
type Loop struct {
	Description   string `yaml:"description"`
	Bound         string `yaml:"bound"`
	MaxIterations int    `yaml:"max_iterations"`
	Steps         []Step `yaml:"steps"`
	ExitWhen      string `yaml:"exit_when"`
	OnExhausted   string `yaml:"on_exhausted"`
}

// Step is one workflow routing step.
type Step struct {
	ID        string    `yaml:"id"`
	Producer  string    `yaml:"producer"`
	Reviewers []string  `yaml:"reviewers"`
	Model     string    `yaml:"model"`
	Effort    string    `yaml:"effort"`
	Artifact  string    `yaml:"artifact"`
	Approval  string    `yaml:"approval"`
	MaxTurns  int       `yaml:"max_turns"`
	Timeout   string    `yaml:"timeout"`
	Consensus Consensus `yaml:"consensus"`
	OnFail    string    `yaml:"on_fail"`
}

// Validate checks loop references and bounded execution fields.
func (l *Loop) Validate(enabledAdapters map[string]AdapterRole) error {
	if l == nil {
		return fmt.Errorf("loop is nil")
	}
	if l.MaxIterations <= 0 {
		return fmt.Errorf("max_iterations must be positive")
	}
	if l.OnExhausted == "" {
		if l.ExitWhen == "protected_action" {
			l.OnExhausted = "fail"
		} else {
			l.OnExhausted = "warn"
		}
	}
	switch l.OnExhausted {
	case "fail", "warn", "proceed":
	default:
		return fmt.Errorf("unknown on_exhausted %q", l.OnExhausted)
	}

	seen := map[string]struct{}{}
	for _, step := range l.Steps {
		if step.ID == "" {
			return fmt.Errorf("step id is required")
		}
		if _, ok := seen[step.ID]; ok {
			return fmt.Errorf("duplicate step id %q", step.ID)
		}
		seen[step.ID] = struct{}{}
		if step.Producer == "" && len(step.Reviewers) == 0 {
			return fmt.Errorf("step %q has neither producer nor reviewers", step.ID)
		}
		if err := validateEffort(fmt.Sprintf("step %q", step.ID), step.Effort); err != nil {
			return err
		}
		if step.Producer != "" {
			role, ok := enabledAdapters[step.Producer]
			if !ok {
				return fmt.Errorf("step %q references unknown adapter %q", step.ID, step.Producer)
			}
			if role != AdapterRoleOrchestrable {
				return fmt.Errorf("step %q producer %q is not orchestrable", step.ID, step.Producer)
			}
		}
		for _, reviewer := range step.Reviewers {
			role, ok := enabledAdapters[reviewer]
			if !ok {
				return fmt.Errorf("step %q references unknown adapter %q", step.ID, reviewer)
			}
			if role != AdapterRoleOrchestrable {
				return fmt.Errorf("step %q reviewer %q is not orchestrable", step.ID, reviewer)
			}
		}
	}
	return nil
}
