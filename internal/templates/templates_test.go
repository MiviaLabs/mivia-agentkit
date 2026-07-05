// Package templates verifies the embedded-template source skeleton.
// Plan: WS0. PRD: §1, §4, §9.
package templates

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTemplatesDirExists(t *testing.T) {
	path := repoPath(t, "templates", "README.md")
	if _, err := os.ReadFile(path); err != nil {
		t.Fatalf("ReadFile(%q) error = %v, want nil", path, err)
	}
}

func TestTemplatesSubdirsExist(t *testing.T) {
	for _, dir := range []string{
		"templates/core/rules",
		"templates/core/skills",
		"templates/core/quality/contracts",
		"templates/core/quality/review-policies",
		"templates/adapters/codex",
		"templates/adapters/claude",
		"templates/adapters/copilot",
		"templates/adapters/antigravity",
		"templates/adapters/crush",
		"templates/workflows",
		"templates/prompts",
		"templates/ci/github-actions",
	} {
		dir := repoPath(t, filepath.FromSlash(dir))
		info, err := os.Stat(dir)
		if err != nil {
			t.Fatalf("Stat(%q) error = %v, want nil", dir, err)
		}
		if !info.IsDir() {
			t.Fatalf("Stat(%q).IsDir() = false, want true", dir)
		}
	}
}

func TestEmbeddedFSContainsCoreFiles(t *testing.T) {
	for _, path := range []string{
		"core/INDEX.md.tmpl",
		"core/rules/00-operating-doctrine.md.tmpl",
		"core/skills/deep-bug-audit/SKILL.md.tmpl",
		"core/quality/contracts/project-runtime.yaml.tmpl",
		"adapters/codex/hooks.json.tmpl",
	} {
		if _, err := FS().Open(path); err != nil {
			t.Fatalf("FS().Open(%q) error = %v, want nil", path, err)
		}
	}
}

func TestListStandardProfileReturnsExpectedFiles(t *testing.T) {
	got, err := List("standard", []string{"codex", "claude", "copilot"})
	if err != nil {
		t.Fatalf("List() error = %v, want nil", err)
	}
	want := []string{
		".agents/skills.json",
		".agents/skills/mivia-agent-workflows/SKILL.md",
		".ai/INDEX.md",
		".ai/quality/contracts/project-runtime.yaml",
		".ai/quality/review-policies/default.yaml",
		".ai/rules/00-operating-doctrine.md",
		".ai/rules/01-output-budget.md",
		".ai/rules/10-security-privacy.md",
		".ai/rules/20-agent-quality.md",
		".ai/skills/adversarial-test-review/SKILL.md",
		".ai/skills/airtight-feature-delivery/SKILL.md",
		".ai/skills/deep-bug-audit/SKILL.md",
		".ai/skills/mivia-agent-workflows/SKILL.md",
		".ai/skills/test-coverage-audit/SKILL.md",
		".ai/workflows/bug-audit-loop.yaml",
		".ai/workflows/research-loop.yaml",
		".claude/settings.json",
		".claude/skills/mivia-agent-workflows/SKILL.md",
		".codex/AGENTS.md",
		".codex/hooks.json",
		".github/copilot-instructions.md",
		".github/instructions/agent-quality.instructions.md",
		"AGENTS.md",
		"CLAUDE.md",
		"mivia-agent.yaml",
	}
	if len(got) != len(want) {
		t.Fatalf("List() len = %d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("List()[%d] = %q, want %q\nall=%#v", i, got[i], want[i], got)
		}
	}
}

func TestListRespectsAdapterSelection(t *testing.T) {
	got, err := List("standard", []string{"claude"})
	if err != nil {
		t.Fatalf("List() error = %v, want nil", err)
	}
	for _, path := range got {
		if path == ".codex/hooks.json" {
			t.Fatalf("List() includes codex hooks with codex disabled: %#v", got)
		}
	}
}

func TestAntigravityAdapterOnlyRendersWhenSelected(t *testing.T) {
	got, err := List("standard", []string{"codex"})
	if err != nil {
		t.Fatalf("List() error = %v, want nil", err)
	}
	for _, path := range got {
		if path == "GEMINI.md" {
			t.Fatalf("List() includes GEMINI.md with antigravity disabled: %#v", got)
		}
	}

	got, err = List("standard", []string{"antigravity"})
	if err != nil {
		t.Fatalf("List() error = %v, want nil", err)
	}
	if !containsTemplatePath(got, "GEMINI.md") {
		t.Fatalf("List() missing GEMINI.md with antigravity enabled: %#v", got)
	}
}

func TestAntigravityAdapterIsThinPointer(t *testing.T) {
	data, err := fs.ReadFile(FS(), "adapters/antigravity/GEMINI.md.tmpl")
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	assertThinAdapterPointer(t, string(data))
}

func TestCrushAdapterShimRendersWhenSelected(t *testing.T) {
	got, err := List("standard", []string{"codex"})
	if err != nil {
		t.Fatalf("List() error = %v, want nil", err)
	}
	for _, path := range got {
		if path == ".crush/README.md" {
			t.Fatalf("List() includes Crush shim with crush disabled: %#v", got)
		}
	}

	got, err = List("standard", []string{"crush"})
	if err != nil {
		t.Fatalf("List() error = %v, want nil", err)
	}
	if !containsTemplatePath(got, ".crush/README.md") {
		t.Fatalf("List() missing Crush shim with crush enabled: %#v", got)
	}
}

func TestCrushShimDoesNotDuplicateLongPolicy(t *testing.T) {
	data, err := fs.ReadFile(FS(), "adapters/crush/README.md.tmpl")
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	assertThinAdapterPointer(t, string(data))
}

func TestCrushTemplateIncludesRuntimeConfigGuidance(t *testing.T) {
	readme, err := fs.ReadFile(FS(), "adapters/crush/README.md.tmpl")
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !strings.Contains(string(readme), "crush run --quiet --cwd <repo>") ||
		!strings.Contains(string(readme), "adapters.crush.model") ||
		!strings.Contains(string(readme), "ollama/qwen3:14b") ||
		!strings.Contains(string(readme), "http://localhost:11434/v1/") ||
		strings.Contains(string(readme), "guidance-only") {
		t.Fatalf("crush README = %q, want runtime and Ollama/Qwen guidance without guidance-only wording", readme)
	}

	manifest, err := fs.ReadFile(FS(), "core/mivia-agent.yaml.tmpl")
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !strings.Contains(string(manifest), "# model: ollama/qwen3:14b") || strings.Contains(string(manifest), "# params:") {
		t.Fatalf("mivia-agent.yaml template = %q, want crush model example without params", manifest)
	}
}

func TestCrushAdapterRendersOrchestrableRole(t *testing.T) {
	data, err := fs.ReadFile(FS(), "core/mivia-agent.yaml.tmpl")
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if strings.Contains(string(data), `(eq $name "crush")`) || !strings.Contains(string(data), "orchestrable") {
		t.Fatalf("mivia-agent.yaml template = %q, want crush to use orchestrable role path", data)
	}
}

func TestListIncludesWorkflowTemplatesForStandard(t *testing.T) {
	got, err := List("standard", []string{"codex", "claude"})
	if err != nil {
		t.Fatalf("List() error = %v, want nil", err)
	}
	found := map[string]bool{}
	for _, path := range got {
		found[path] = true
	}
	for _, want := range []string{".ai/workflows/research-loop.yaml", ".ai/workflows/bug-audit-loop.yaml"} {
		if !found[want] {
			t.Fatalf("List() missing %q: %#v", want, got)
		}
	}
}

func TestListIncludesWorkflowTemplatesWithAnyRuntimeAdapter(t *testing.T) {
	tests := []struct {
		name     string
		adapters []string
	}{
		{name: "codex only", adapters: []string{"codex"}},
		{name: "claude only", adapters: []string{"claude"}},
		{name: "other adapter", adapters: []string{"antigravity"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := List("standard", tt.adapters)
			if err != nil {
				t.Fatalf("List() error = %v, want nil", err)
			}
			for _, want := range []string{".ai/workflows/research-loop.yaml", ".ai/workflows/bug-audit-loop.yaml"} {
				if !containsTemplatePath(got, want) {
					t.Fatalf("List() missing %q with runtime adapter enabled: %#v", want, got)
				}
			}
		})
	}
}

func TestListIncludesWorkflowTemplatesWithCrushRuntimeAdapter(t *testing.T) {
	got, err := List("standard", []string{"crush"})
	if err != nil {
		t.Fatalf("List() error = %v, want nil", err)
	}
	for _, want := range []string{".ai/workflows/research-loop.yaml", ".ai/workflows/bug-audit-loop.yaml"} {
		if !containsTemplatePath(got, want) {
			t.Fatalf("List() missing %q with crush runtime adapter: %#v", want, got)
		}
	}
}

func TestWorkflowTemplatesUseRoutingVars(t *testing.T) {
	for _, tpl := range []string{"workflows/research-loop.yaml.tmpl", "workflows/bug-audit-loop.yaml.tmpl"} {
		t.Run(tpl, func(t *testing.T) {
			data, err := fs.ReadFile(FS(), tpl)
			if err != nil {
				t.Fatalf("ReadFile() error = %v", err)
			}
			for _, forbidden := range []string{"producer: codex", "- claude"} {
				if strings.Contains(string(data), forbidden) {
					t.Fatalf("%s = %q, want routing vars instead of hard-coded %q", tpl, data, forbidden)
				}
			}
		})
	}
}

func TestMiviaAgentWorkflowSkillTemplates(t *testing.T) {
	for _, path := range []string{
		"core/skills/mivia-agent-workflows/SKILL.md.tmpl",
		"adapters/agents/skills/mivia-agent-workflows/SKILL.md.tmpl",
		"adapters/claude/skills/mivia-agent-workflows/SKILL.md.tmpl",
	} {
		if _, err := FS().Open(path); err != nil {
			t.Fatalf("FS().Open(%q) error = %v, want nil", path, err)
		}
		data, err := fs.ReadFile(FS(), path)
		if err != nil {
			t.Fatalf("ReadFile(%q) error = %v", path, err)
		}
		for _, want := range []string{"triggers:", "mivia-agent workflow", "workflow artifacts"} {
			if !strings.Contains(string(data), want) {
				t.Fatalf("%s = %q, want frontmatter trigger %q", path, data, want)
			}
		}
	}
	canonical, err := fs.ReadFile(FS(), "core/skills/mivia-agent-workflows/SKILL.md.tmpl")
	if err != nil {
		t.Fatalf("ReadFile(canonical) error = %v", err)
	}
	agents, err := fs.ReadFile(FS(), "adapters/agents/skills/mivia-agent-workflows/SKILL.md.tmpl")
	if err != nil {
		t.Fatalf("ReadFile(.agents mirror) error = %v", err)
	}
	if string(agents) != string(canonical) {
		t.Fatalf(".agents mivia-agent-workflows skill differs from .ai canonical")
	}
	claude, err := fs.ReadFile(FS(), "adapters/claude/skills/mivia-agent-workflows/SKILL.md.tmpl")
	if err != nil {
		t.Fatalf("ReadFile(claude pointer) error = %v", err)
	}
	for _, want := range []string{
		"name: mivia-agent-workflows",
		".ai/skills/mivia-agent-workflows/SKILL.md",
		"discovery pointer",
	} {
		if !strings.Contains(string(claude), want) {
			t.Fatalf("Claude workflow skill = %q, want %q", claude, want)
		}
	}
	if len(strings.Split(strings.TrimSpace(string(claude)), "\n")) > 20 {
		t.Fatalf("Claude workflow skill has too many lines; want concise pointer")
	}
}

func containsTemplatePath(paths []string, want string) bool {
	for _, path := range paths {
		if path == want {
			return true
		}
	}
	return false
}

func assertThinAdapterPointer(t *testing.T, content string) {
	t.Helper()
	if !strings.Contains(content, ".ai/INDEX.md") {
		t.Fatalf("template = %q, want pointer to .ai/INDEX.md", content)
	}
	if strings.Contains(content, "deterministic local checks") || strings.Contains(content, "Every guard has a mutation proof") {
		t.Fatalf("template duplicates long policy: %q", content)
	}
}

func TestCommittedTemplatesMatchEmbeddedSource(t *testing.T) {
	for outPath, embeddedPath := range outputTemplates {
		committedPath := repoPath(t, "templates", filepath.FromSlash(embeddedPath))
		committed, err := os.ReadFile(committedPath)
		if err != nil {
			t.Fatalf("ReadFile(%q for output %s) error = %v, want nil", committedPath, outPath, err)
		}
		embedded, err := fs.ReadFile(FS(), embeddedPath)
		if err != nil {
			t.Fatalf("FS().ReadFile(%q for output %s) error = %v, want nil", embeddedPath, outPath, err)
		}
		if string(committed) != string(embedded) {
			t.Fatalf("template %q differs between templates/ and embedded source", embeddedPath)
		}
	}
}

func repoPath(t *testing.T, parts ...string) string {
	t.Helper()
	joined := filepath.Join(parts...)
	return filepath.Join("..", "..", joined)
}
