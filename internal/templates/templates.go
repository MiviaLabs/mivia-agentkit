// Package templates embeds the mivia-agent target-repo template catalog.
// Plan: WS2. PRD: FR-1.1, FR-6.1, FR-6.2, FR-10.6.
package templates

import (
	"embed"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
)

//go:embed source/**
var embedded embed.FS

// FS returns the embedded template filesystem.
func FS() fs.FS {
	sub, err := fs.Sub(embedded, "source")
	if err != nil {
		return embedded
	}
	return sub
}

// List returns target repo paths for the selected profile and adapters.
func List(profile string, adapters []string) ([]string, error) {
	if profile == "" {
		profile = "standard"
	}
	switch profile {
	case "starter", "standard", "strict":
	default:
		return nil, fmt.Errorf("unknown profile %q", profile)
	}
	enabled := map[string]bool{}
	for _, adapter := range adapters {
		switch adapter {
		case "codex", "claude", "copilot", "antigravity", "crush":
			enabled[adapter] = true
		default:
			return nil, fmt.Errorf("unknown adapter %q", adapter)
		}
	}
	if len(enabled) == 0 {
		enabled["codex"] = true
		enabled["claude"] = true
		enabled["copilot"] = true
	}

	var out []string
	for _, path := range coreOutputs(profile, enabled) {
		out = append(out, path)
	}
	if enabled["codex"] {
		out = append(out, ".codex/hooks.json", ".codex/AGENTS.md")
	}
	if enabled["claude"] {
		out = append(out, "CLAUDE.md", ".claude/settings.json")
	}
	if enabled["copilot"] {
		out = append(out, ".github/copilot-instructions.md", ".github/instructions/agent-quality.instructions.md")
	}
	if enabled["antigravity"] {
		out = append(out, "GEMINI.md")
	}
	if enabled["crush"] {
		out = append(out, ".crush/README.md")
	}
	sort.Strings(out)
	return out, nil
}

// TemplateForOutput maps a target repo path to its embedded template path.
func TemplateForOutput(outPath string) (string, bool) {
	path := filepath.ToSlash(outPath)
	tpl, ok := outputTemplates[path]
	return tpl, ok
}

func coreOutputs(profile string, enabled map[string]bool) []string {
	out := []string{
		"AGENTS.md",
		"mivia-agent.yaml",
		".agents/skills.json",
		".ai/INDEX.md",
		".ai/rules/00-operating-doctrine.md",
		".ai/rules/01-output-budget.md",
		".ai/rules/10-security-privacy.md",
		".ai/rules/20-agent-quality.md",
		".ai/skills/airtight-feature-delivery/SKILL.md",
		".ai/skills/test-coverage-audit/SKILL.md",
		".ai/skills/deep-bug-audit/SKILL.md",
		".ai/skills/adversarial-test-review/SKILL.md",
		".ai/skills/mivia-agent-workflows/SKILL.md",
		".agents/skills/mivia-agent-workflows/SKILL.md",
		".ai/quality/contracts/project-runtime.yaml",
		".ai/quality/review-policies/default.yaml",
	}
	if enabled["claude"] {
		out = append(out, ".claude/skills/mivia-agent-workflows/SKILL.md")
	}
	if profile != "starter" && hasRuntimeAdapter(enabled) {
		out = append(out, ".ai/workflows/research-loop.yaml", ".ai/workflows/bug-audit-loop.yaml")
	}
	return out
}

func hasRuntimeAdapter(enabled map[string]bool) bool {
	for _, name := range []string{"codex", "claude", "antigravity", "crush"} {
		if enabled[name] {
			return true
		}
	}
	return false
}

var outputTemplates = map[string]string{
	"AGENTS.md":                                          "core/AGENTS.md.tmpl",
	"mivia-agent.yaml":                                   "core/mivia-agent.yaml.tmpl",
	".agents/skills.json":                                "core/agents-skills.json.tmpl",
	".ai/INDEX.md":                                       "core/INDEX.md.tmpl",
	".ai/rules/00-operating-doctrine.md":                 "core/rules/00-operating-doctrine.md.tmpl",
	".ai/rules/01-output-budget.md":                      "core/rules/01-output-budget.md.tmpl",
	".ai/rules/10-security-privacy.md":                   "core/rules/10-security-privacy.md.tmpl",
	".ai/rules/20-agent-quality.md":                      "core/rules/20-agent-quality.md.tmpl",
	".ai/skills/airtight-feature-delivery/SKILL.md":      "core/skills/airtight-feature-delivery/SKILL.md.tmpl",
	".ai/skills/test-coverage-audit/SKILL.md":            "core/skills/test-coverage-audit/SKILL.md.tmpl",
	".ai/skills/deep-bug-audit/SKILL.md":                 "core/skills/deep-bug-audit/SKILL.md.tmpl",
	".ai/skills/adversarial-test-review/SKILL.md":        "core/skills/adversarial-test-review/SKILL.md.tmpl",
	".ai/skills/mivia-agent-workflows/SKILL.md":          "core/skills/mivia-agent-workflows/SKILL.md.tmpl",
	".agents/skills/mivia-agent-workflows/SKILL.md":      "adapters/agents/skills/mivia-agent-workflows/SKILL.md.tmpl",
	".ai/quality/contracts/project-runtime.yaml":         "core/quality/contracts/project-runtime.yaml.tmpl",
	".ai/quality/review-policies/default.yaml":           "core/quality/review-policies/default.yaml.tmpl",
	".ai/workflows/research-loop.yaml":                   "workflows/research-loop.yaml.tmpl",
	".ai/workflows/bug-audit-loop.yaml":                  "workflows/bug-audit-loop.yaml.tmpl",
	".codex/hooks.json":                                  "adapters/codex/hooks.json.tmpl",
	".codex/AGENTS.md":                                   "adapters/codex/AGENTS.md.tmpl",
	"CLAUDE.md":                                          "adapters/claude/CLAUDE.md.tmpl",
	".claude/settings.json":                              "adapters/claude/settings.json.tmpl",
	".claude/skills/mivia-agent-workflows/SKILL.md":      "adapters/claude/skills/mivia-agent-workflows/SKILL.md.tmpl",
	".github/copilot-instructions.md":                    "adapters/copilot/copilot-instructions.md.tmpl",
	".github/instructions/agent-quality.instructions.md": "adapters/copilot/agent-quality.instructions.md.tmpl",
	"GEMINI.md":        "adapters/antigravity/GEMINI.md.tmpl",
	".crush/README.md": "adapters/crush/README.md.tmpl",
}
