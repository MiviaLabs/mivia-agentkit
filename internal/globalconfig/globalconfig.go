// Package globalconfig reads ~/.agents defaults without writing them.
// Plan: WS1. PRD: FR-10.1, FR-10.2, FR-10.3, FR-10.4.
package globalconfig

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/MiviaLabs/mivia-agentkit/internal/config"
	"github.com/MiviaLabs/mivia-agentkit/internal/pathpolicy"
	"gopkg.in/yaml.v3"
)

// GlobalConfig is the read-only ~/.agents layer.
type GlobalConfig struct {
	Defaults config.Manifest
	Rules    map[string]string
	Skills   map[string]string
}

// EffectiveConfig is a project manifest plus merged global/project content maps.
type EffectiveConfig struct {
	Manifest config.Manifest
	Rules    map[string]string
	Skills   map[string]string
}

// ProjectContent contains project-level rules and skills that override globals.
type ProjectContent struct {
	Rules  map[string]string
	Skills map[string]string
}

type globalYAML struct {
	Defaults config.Manifest `yaml:"defaults"`
}

type legacyGlobalYAML struct {
	Version  int                  `yaml:"version"`
	Defaults legacyGlobalDefaults `yaml:"defaults"`
	Hooks    legacyHooks          `yaml:"hooks"`
}

type legacyGlobalDefaults struct {
	Profile         string            `yaml:"profile"`
	TemplateVersion string            `yaml:"template_version"`
	Adapters        legacyAdapters    `yaml:"adapters"`
	Governance      config.Governance `yaml:"governance"`
	Verification    map[string]bool   `yaml:"verification"`
	Privacy         map[string]bool   `yaml:"privacy"`
}

type legacyAdapters struct {
	ByName       map[string]config.AdapterConfig
	Orchestrable []string `yaml:"orchestrable"`
	Guidance     []string `yaml:"guidance"`
}

type legacyHooks struct {
	ProtectedActions      []string `yaml:"protected_actions"`
	DefaultPolicyProvider string   `yaml:"default_policy_provider"`
}

// Read loads ~/.agents if present. Absence is not an error.
func Read() (GlobalConfig, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return GlobalConfig{}, fmt.Errorf("home dir: %w", err)
	}
	root := filepath.Join(home, ".agents")
	info, err := os.Stat(root)
	if os.IsNotExist(err) {
		return GlobalConfig{}, nil
	}
	if err != nil {
		return GlobalConfig{}, fmt.Errorf("stat .agents: %w", err)
	}
	if !info.IsDir() {
		return GlobalConfig{}, fmt.Errorf("%s is not a directory", root)
	}
	policy := pathpolicy.NewDefault()
	cfg := GlobalConfig{Rules: map[string]string{}, Skills: map[string]string{}}
	if err := readDefaults(root, policy, &cfg); err != nil {
		return GlobalConfig{}, err
	}
	if err := readRules(root, policy, cfg.Rules); err != nil {
		return GlobalConfig{}, err
	}
	if err := readSkills(root, policy, cfg.Skills); err != nil {
		return GlobalConfig{}, err
	}
	return cfg, nil
}

// Layer merges global defaults and content under the project configuration.
func Layer(global GlobalConfig, project config.Manifest, projectContent ...ProjectContent) EffectiveConfig {
	effective := project
	if effective.Profile == "" {
		effective.Profile = global.Defaults.Profile
	}
	if effective.TemplateVersion == "" {
		effective.TemplateVersion = global.Defaults.TemplateVersion
	}
	if len(effective.Adapters) == 0 && len(global.Defaults.Adapters) > 0 {
		effective.Adapters = cloneAdapters(global.Defaults.Adapters)
	}
	if effective.Governance.Provider == "" {
		effective.Governance = global.Defaults.Governance
	}
	content := ProjectContent{}
	if len(projectContent) > 0 {
		content = projectContent[0]
	}
	rules := mergeMap(global.Rules, content.Rules)
	skills := mergeMap(global.Skills, content.Skills)
	return EffectiveConfig{Manifest: effective, Rules: rules, Skills: skills}
}

func readDefaults(root string, policy pathpolicy.Policy, cfg *GlobalConfig) error {
	path, rel, ok, err := defaultConfigPath(root)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	if err := policy.Check(filepath.Dir(root), rel); err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", filepath.Base(path), err)
	}
	var parsed globalYAML
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&parsed); err != nil {
		legacy, legacyErr := parseLegacyDefaults(data)
		if legacyErr != nil {
			return fmt.Errorf("parse %s: %w", filepath.Base(path), err)
		}
		cfg.Defaults = legacy
		return nil
	}
	cfg.Defaults = parsed.Defaults
	return nil
}

func defaultConfigPath(root string) (string, string, bool, error) {
	for _, name := range []string{"mivia-agent.yaml", "mivia.yaml"} {
		path := filepath.Join(root, name)
		_, err := os.Stat(path)
		if err == nil {
			return path, filepath.Join(".agents", name), true, nil
		}
		if !os.IsNotExist(err) {
			return "", "", false, fmt.Errorf("stat %s: %w", path, err)
		}
	}
	return "", "", false, nil
}

func parseLegacyDefaults(data []byte) (config.Manifest, error) {
	var parsed legacyGlobalYAML
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&parsed); err != nil {
		return config.Manifest{}, err
	}
	out := config.Manifest{
		Version:         "1",
		Profile:         parsed.Defaults.Profile,
		TemplateVersion: parsed.Defaults.TemplateVersion,
		Adapters:        parsed.Defaults.Adapters.ByName,
		Governance:      parsed.Defaults.Governance,
	}
	if out.Governance.Provider == "" {
		out.Governance.Provider = parsed.Hooks.DefaultPolicyProvider
	}
	if out.Governance.AuditLog == "" {
		out.Governance.AuditLog = ".ai/audit.jsonl"
	}
	return out, nil
}

func (a *legacyAdapters) UnmarshalYAML(value *yaml.Node) error {
	var byName map[string]config.AdapterConfig
	if err := value.Decode(&byName); err == nil {
		a.ByName = byName
		return nil
	}
	var grouped struct {
		Orchestrable []string `yaml:"orchestrable"`
		Guidance     []string `yaml:"guidance"`
	}
	if err := value.Decode(&grouped); err != nil {
		return err
	}
	a.ByName = map[string]config.AdapterConfig{}
	for _, name := range grouped.Orchestrable {
		a.ByName[name] = config.AdapterConfig{Enabled: true, Role: config.AdapterRoleOrchestrable}
	}
	for _, name := range grouped.Guidance {
		a.ByName[name] = config.AdapterConfig{Enabled: true, Role: config.AdapterRoleGuidance}
	}
	a.Orchestrable = grouped.Orchestrable
	a.Guidance = grouped.Guidance
	return nil
}

func readRules(root string, policy pathpolicy.Policy, rules map[string]string) error {
	dir := filepath.Join(root, "rules")
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read rules dir: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			continue
		}
		rel := filepath.Join(".agents", "rules", entry.Name())
		if err := policy.Check(filepath.Dir(root), rel); err != nil {
			return err
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return fmt.Errorf("read rule %s: %w", entry.Name(), err)
		}
		rules[entry.Name()] = string(data)
	}
	return nil
}

func readSkills(root string, policy pathpolicy.Policy, skills map[string]string) error {
	dir := filepath.Join(root, "skills")
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read skills dir: %w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.Contains(name, string(filepath.Separator)) {
			return fmt.Errorf("invalid skill name %q", name)
		}
		rel := filepath.Join(".agents", "skills", name, "SKILL.md")
		if err := policy.Check(filepath.Dir(root), rel); err != nil {
			return err
		}
		data, err := os.ReadFile(filepath.Join(dir, name, "SKILL.md"))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("read skill %s: %w", name, err)
		}
		skills[name] = string(data)
	}
	return nil
}

func cloneMap(in map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range in {
		out[k] = v
	}
	return out
}

func mergeMap(base, override map[string]string) map[string]string {
	out := cloneMap(base)
	for k, v := range override {
		out[k] = v
	}
	return out
}

func cloneAdapters(in map[string]config.AdapterConfig) map[string]config.AdapterConfig {
	out := map[string]config.AdapterConfig{}
	for k, v := range in {
		out[k] = v
	}
	return out
}
