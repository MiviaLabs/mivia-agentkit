// Package config implements mivia-agent.yaml parsing.
// Plan: WS-A. PRD: FR-1.1, FR-4.2, FR-10.2.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// AdapterRole is the role an adapter can play in workflows.
type AdapterRole string

const (
	// AdapterRoleOrchestrable can be invoked by mivia-agent workflows.
	AdapterRoleOrchestrable AdapterRole = "orchestrable"
	// AdapterRoleGuidance contributes instructions only.
	AdapterRoleGuidance AdapterRole = "guidance"
)

// Manifest is the project-level mivia-agent.yaml configuration.
type Manifest struct {
	Version          string                   `yaml:"version"`
	Profile          string                   `yaml:"profile"`
	TemplateVersion  string                   `yaml:"template_version"`
	Project          Project                  `yaml:"project"`
	Adapters         map[string]AdapterConfig `yaml:"adapters"`
	Routing          Routing                  `yaml:"routing"`
	Loops            map[string]Loop          `yaml:"loops"`
	Commands         map[string]string        `yaml:"commands"`
	ProtectedActions []string                 `yaml:"protected_actions"`
	Quality          Quality                  `yaml:"quality"`
	Paths            Paths                    `yaml:"paths"`
	Governance       Governance               `yaml:"governance"`
	Global           Global                   `yaml:"global"`
	MCP              MCP                      `yaml:"mcp"`
}

// Project describes the target project.
type Project struct {
	Name string `yaml:"name"`
}

// AdapterConfig configures one adapter.
type AdapterConfig struct {
	Enabled bool              `yaml:"enabled"`
	Role    AdapterRole       `yaml:"role"`
	Model   string            `yaml:"model"`
	Effort  string            `yaml:"effort"`
	Params  map[string]string `yaml:"params"`
}

// Routing is the default workflow routing policy.
type Routing struct {
	DefaultProducer  string    `yaml:"default_producer"`
	DefaultReviewers []string  `yaml:"default_reviewers"`
	Consensus        Consensus `yaml:"consensus"`
	OnReviewFail     string    `yaml:"on_review_fail"`
	MaxIterations    int       `yaml:"max_iterations"`
}

// Consensus defines review aggregation behavior.
type Consensus struct {
	Mode         string         `yaml:"mode"`
	Weights      map[string]int `yaml:"weights"`
	TieBreaker   string         `yaml:"tie_breaker"`
	MinReviewers int            `yaml:"min_reviewers"`
}

// Quality configures quality checks.
type Quality struct {
	RequiredVerifiers []string            `yaml:"required_verifiers"`
	Verifiers         map[string]Verifier `yaml:"verifiers"`
}

// Verifier declares one trusted, local, argv-only quality command.
type Verifier struct {
	Command []string `yaml:"command"`
}

// Paths configures path allow and deny lists.
type Paths struct {
	Allow []string `yaml:"allow"`
	Deny  []string `yaml:"deny"`
}

// Governance configures local governance.
type Governance struct {
	Provider        string `yaml:"provider"`
	AuditLog        string `yaml:"audit_log"`
	PolicyDecisions string `yaml:"policy_decisions"`
}

// Global configures global ~/.agents layering.
type Global struct {
	Layer string `yaml:"layer"`
	Merge string `yaml:"merge"`
}

// MCP configures MCP allow-list metadata.
type MCP struct {
	Servers []string `yaml:"servers"`
}

// Defaults returns the standard profile defaults.
func Defaults() Manifest {
	return Manifest{
		Version:         "1",
		Profile:         "standard",
		TemplateVersion: "dev",
		Adapters: map[string]AdapterConfig{
			"codex":       {Enabled: true, Role: AdapterRoleOrchestrable},
			"claude":      {Enabled: true, Role: AdapterRoleOrchestrable},
			"copilot":     {Enabled: true, Role: AdapterRoleGuidance},
			"antigravity": {Enabled: false, Role: AdapterRoleOrchestrable},
			"crush":       {Enabled: false, Role: AdapterRoleGuidance},
		},
		Routing: Routing{
			DefaultProducer:  "codex",
			DefaultReviewers: []string{"codex", "claude"},
			Consensus: Consensus{
				Mode:         "majority",
				Weights:      map[string]int{},
				TieBreaker:   "strict",
				MinReviewers: 2,
			},
			OnReviewFail:  "iterate",
			MaxIterations: 3,
		},
		Governance: Governance{Provider: "noop", AuditLog: ".ai/audit.jsonl"},
		Global:     Global{Layer: "~/.agents", Merge: "project_wins"},
	}
}

// Parse decodes a strict YAML manifest.
func Parse(data []byte) (Manifest, error) {
	m := Defaults()
	if declaresAdapters(data) {
		m.Adapters = map[string]AdapterConfig{}
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&m); err != nil {
		return Manifest{}, fmt.Errorf("parse manifest: %w", err)
	}
	if err := m.Validate(); err != nil {
		return Manifest{}, err
	}
	return m, nil
}

func declaresAdapters(data []byte) bool {
	for _, line := range bytes.Split(data, []byte("\n")) {
		if bytes.Equal(bytes.TrimSpace(line), []byte("adapters:")) {
			return true
		}
	}
	return false
}

// Validate checks the manifest contract.
func (m *Manifest) Validate() error {
	if m == nil {
		return errors.New("manifest is nil")
	}
	if m.Profile == "" {
		m.Profile = "standard"
	}
	switch m.Profile {
	case "starter", "standard", "strict":
	case "expert":
		return errors.New("expert profile is not supported in MVP")
	default:
		return fmt.Errorf("unknown profile %q", m.Profile)
	}

	for name, adapter := range m.Adapters {
		if adapter.Role != AdapterRoleOrchestrable && adapter.Role != AdapterRoleGuidance {
			return fmt.Errorf("adapter %q has unknown role %q", name, adapter.Role)
		}
		if err := validateEffort(fmt.Sprintf("adapter %q", name), adapter.Effort); err != nil {
			return err
		}
	}
	for id, verifier := range m.Quality.Verifiers {
		if !verifierID.MatchString(id) || len(verifier.Command) == 0 || verifier.Command[0] == "" {
			return fmt.Errorf("quality verifier %q must have a valid id and command", id)
		}
	}
	for _, id := range m.Quality.RequiredVerifiers {
		if _, ok := m.Quality.Verifiers[id]; !ok {
			return fmt.Errorf("required quality verifier %q is not declared", id)
		}
	}

	if err := validateConsensus("routing", m.Routing.Consensus); err != nil {
		return err
	}

	enabled := map[string]AdapterRole{}
	for name, adapter := range m.Adapters {
		if adapter.Enabled {
			enabled[name] = adapter.Role
		}
	}
	for name, loop := range m.Loops {
		if loop.Bound == "" {
			return fmt.Errorf("loop %q has no bound", name)
		}
		if loop.Bound == "budget" {
			return fmt.Errorf("loop %q uses budget bound, unsupported in MVP", name)
		}
		if err := loop.Validate(enabled); err != nil {
			return fmt.Errorf("loop %q: %w", name, err)
		}
		for _, step := range loop.Steps {
			if err := validateConsensus(fmt.Sprintf("loop %q step %q", name, step.ID), step.Consensus); err != nil {
				return err
			}
		}
		m.Loops[name] = loop
	}
	return nil
}

func validateConsensus(scope string, consensus Consensus) error {
	if consensus.Mode != "" && !validConsensusMode(consensus.Mode) {
		return fmt.Errorf("%s has unknown consensus mode %q", scope, consensus.Mode)
	}
	if consensus.MinReviewers < 0 {
		return fmt.Errorf("%s consensus min_reviewers must not be negative", scope)
	}
	for adapter, weight := range consensus.Weights {
		if strings.TrimSpace(adapter) == "" || weight <= 0 {
			return fmt.Errorf("%s consensus weight for %q must be positive", scope, adapter)
		}
	}
	return nil
}

var verifierID = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)

func validConsensusMode(mode string) bool {
	switch mode {
	case "majority", "unanimous", "weighted", "first-pass":
		return true
	default:
		return false
	}
}

// ValidateEffortValue rejects unknown cross-adapter effort values.
func ValidateEffortValue(effort string) error {
	if effort == "" {
		return nil
	}
	switch effort {
	case "none", "minimal", "low", "medium", "high", "xhigh", "max":
		return nil
	default:
		return fmt.Errorf("unknown effort %q", effort)
	}
}

func validateEffort(scope string, effort string) error {
	if err := ValidateEffortValue(effort); err != nil {
		return fmt.Errorf("%s has %s", scope, err)
	}
	return nil
}
