// Package templates verifies the embedded-template source skeleton.
// Plan: WS0. PRD: §1, §4, §9.
package templates

import (
	"io/fs"
	"os"
	"path/filepath"
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
		"templates/adapters/gemini",
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
		".ai/skills/test-coverage-audit/SKILL.md",
		".ai/workflows/bug-audit-loop.yaml",
		".ai/workflows/research-loop.yaml",
		".claude/settings.json",
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

func TestListIncludesWorkflowTemplatesForStandard(t *testing.T) {
	got, err := List("standard", []string{"codex"})
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
