// Package update refreshes managed template blocks in place.
// Plan: WS7. PRD: FR-1.4.
package update

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MiviaLabs/mivia-agentkit/internal/render"
	"github.com/MiviaLabs/mivia-agentkit/internal/version"
)

func TestUpdateChangesManagedBlockOnly(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	repo := freshRepo(t)
	path := filepath.Join(repo, "AGENTS.md")
	data := readFile(t, path)
	data = strings.Replace(data, "Run configured verification before claiming completion.", "legacy verification line", 1)
	writeFile(t, path, data)
	bumpTemplateVersion(t, repo, "v0.0.1")
	report, err := Apply(repo, false)
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if len(report.Updated) == 0 {
		t.Fatalf("Apply() updated = %#v, want change", report.Updated)
	}
	if got := readFile(t, path); !strings.Contains(got, "Run configured verification before claiming completion.") {
		t.Fatalf("AGENTS.md = %q, want refreshed managed block", got)
	}
}

func TestUpdatePreservesUserTextOutsideManagedBlocks(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	repo := freshRepo(t)
	path := filepath.Join(repo, "AGENTS.md")
	original := readFile(t, path)
	withUserText := "user preface\n" + original + "\nuser tail\n"
	writeFile(t, path, withUserText)
	updated := strings.Replace(withUserText, "Run configured verification before claiming completion.", "legacy verification line", 1)
	writeFile(t, path, updated)
	bumpTemplateVersion(t, repo, "v0.0.1")
	if _, err := Apply(repo, false); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	got := readFile(t, path)
	for _, want := range []string{"user preface", "user tail", "Run configured verification before claiming completion."} {
		if !strings.Contains(got, want) {
			t.Fatalf("AGENTS.md = %q, missing %q", got, want)
		}
	}
}

func TestUpdateReportsConflictForUserEditedManagedBlock(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	repo := freshRepo(t)
	path := filepath.Join(repo, "AGENTS.md")
	data := readFile(t, path)
	writeFile(t, path, strings.Replace(data, "Run configured verification before claiming completion.", "edited verification line", 1))
	_, err := Apply(repo, false)
	if err == nil {
		t.Fatal("Apply() error = nil, want conflict")
	}
}

func TestUpdateForceOverwritesConflictedBlock(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	repo := freshRepo(t)
	path := filepath.Join(repo, "AGENTS.md")
	data := readFile(t, path)
	writeFile(t, path, strings.Replace(data, "Run configured verification before claiming completion.", "edited verification line", 1))
	report, err := Apply(repo, true)
	if err != nil {
		t.Fatalf("Apply(force) error = %v", err)
	}
	if len(report.Updated) == 0 {
		t.Fatalf("Apply(force) updated = %#v, want overwrite", report.Updated)
	}
}

func TestUpdateNoOpWhenAlreadyCurrent(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	repo := freshRepo(t)
	changes, err := Diff(repo)
	if err != nil {
		t.Fatalf("Diff() error = %v", err)
	}
	if len(changes) != 0 {
		t.Fatalf("Diff() = %#v, want no changes", changes)
	}
	report, err := Apply(repo, false)
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if len(report.Updated) != 0 || len(report.Conflicts) != 0 {
		t.Fatalf("Apply() = %#v, want no-op", report)
	}
}

func TestUpdateNoOpKeepsMiviaAgentWorkflowSkillRegistry(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	repo := freshRepo(t)
	changes, err := Diff(repo)
	if err != nil {
		t.Fatalf("Diff() error = %v", err)
	}
	if len(changes) != 0 {
		t.Fatalf("Diff() = %#v, want no changes after init", changes)
	}
	got := readFile(t, filepath.Join(repo, ".agents", "skills.json"))
	if !strings.Contains(got, `"name": "mivia-agent-workflows"`) ||
		!strings.Contains(got, `"path": ".ai/skills/mivia-agent-workflows/SKILL.md"`) {
		t.Fatalf("skills.json = %q, want mivia-agent-workflows project entry", got)
	}
}

func freshRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	if _, err := render.WriteInit(render.InitConfig{Repo: repo, Profile: "standard", Adapters: []string{"codex", "claude", "copilot"}}); err != nil {
		t.Fatalf("WriteInit() error = %v", err)
	}
	return repo
}

func bumpTemplateVersion(t *testing.T, repo, next string) {
	t.Helper()
	path := filepath.Join(repo, "mivia-agent.yaml")
	data := readFile(t, path)
	data = strings.Replace(data, "template_version: "+version.Version, "template_version: "+next, 1)
	writeFile(t, path, data)
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	return string(data)
}
