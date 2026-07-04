// Package importer inspects existing agent-control files for migration.
// Plan: WS7. PRD: FR-9.1, FR-9.2.
package importer

import (
	"path/filepath"
	"testing"

	"github.com/MiviaLabs/mivia-agentkit/internal/config"
)

func TestImportReadsExistingAgentFiles(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	repo := fixtureRepo(t, "basic")
	findings, err := Inspect(repo)
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	for _, want := range []string{"AGENTS.md", "CLAUDE.md", ".github/copilot-instructions.md"} {
		if !hasSource(findings, want) {
			t.Fatalf("Inspect() missing %q in %#v", want, findings)
		}
	}
}

func TestImportClassifiesKinds(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	repo := fixtureRepo(t, "basic")
	findings, err := Inspect(repo)
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	assertKind(t, findings, "AGENTS.md", "rules", true)
	assertKind(t, findings, ".codex/hooks.json", "hook", false)
	assertKind(t, findings, ".agents/skills/existing/SKILL.md", "skill", true)
}

func TestImportDetectsExistingWorkflowDefinitions(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	repo := fixtureRepo(t, "workflows")
	findings, err := Inspect(repo)
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	for _, want := range []string{".github/workflows/build-agent.yml", ".claude/agents/research.md", ".dagger", "legacy.loop.yaml"} {
		if !hasSource(findings, want) {
			t.Fatalf("Inspect() missing workflow %q in %#v", want, findings)
		}
	}
}

func TestImportFlagsConflicts(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	repo := fixtureRepo(t, "conflicts")
	plan, err := BuildPlan(repo, config.Defaults())
	if err != nil {
		t.Fatalf("BuildPlan() error = %v", err)
	}
	if len(plan.Conflicts) == 0 {
		t.Fatalf("BuildPlan() conflicts = %#v, want at least one", plan.Conflicts)
	}
}

func fixtureRepo(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join("testdata", "import", name)
}

func hasSource(findings []Finding, source string) bool {
	for _, finding := range findings {
		if finding.Source == source {
			return true
		}
	}
	return false
}

func assertKind(t *testing.T, findings []Finding, source, kind string, reusable bool) {
	t.Helper()
	for _, finding := range findings {
		if finding.Source == source {
			if finding.Kind != kind || finding.Reusable != reusable {
				t.Fatalf("finding %q = %#v, want kind=%q reusable=%t", source, finding, kind, reusable)
			}
			return
		}
	}
	t.Fatalf("finding %q missing from %#v", source, findings)
}
