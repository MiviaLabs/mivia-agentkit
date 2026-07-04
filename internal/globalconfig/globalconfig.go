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

type globalYAML struct {
	Defaults config.Manifest `yaml:"defaults"`
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

// Layer merges global defaults under the project manifest.
func Layer(global GlobalConfig, project config.Manifest) EffectiveConfig {
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
	rules := cloneMap(global.Rules)
	skills := cloneMap(global.Skills)
	return EffectiveConfig{Manifest: effective, Rules: rules, Skills: skills}
}

func readDefaults(root string, policy pathpolicy.Policy, cfg *GlobalConfig) error {
	path := filepath.Join(root, "mivia.yaml")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}
	if err := policy.Check(filepath.Dir(root), filepath.Join(".agents", "mivia.yaml")); err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read mivia.yaml: %w", err)
	}
	var parsed globalYAML
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&parsed); err != nil {
		return fmt.Errorf("parse mivia.yaml: %w", err)
	}
	cfg.Defaults = parsed.Defaults
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

func cloneAdapters(in map[string]config.AdapterConfig) map[string]config.AdapterConfig {
	out := map[string]config.AdapterConfig{}
	for k, v := range in {
		out[k] = v
	}
	return out
}
