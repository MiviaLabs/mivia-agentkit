// Package cli implements the mivia-agent command surface.
// Plan: WS2. PRD: FR-1.1, FR-1.2, FR-10.6.
package cli

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitBinaryWritesMiviaAgentWorkflowSkills(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	repo := tempInitGitRepo(t)
	bin, err := BuildBinary(BinaryBuild{ModuleRoot: filepath.Join("..", "..")})
	if err != nil {
		t.Fatalf("BuildBinary() error = %v", err)
	}
	result, err := RunBinary(context.Background(), bin, BinaryRun{
		Args: []string{
			"init",
			"--repo", repo,
			"--profile", "standard",
			"--adapter", "codex",
			"--adapter", "claude",
			"--write",
			"--json",
		},
		Scrub: map[string]string{repo: "<repo>"},
	})
	if err != nil {
		t.Fatalf("RunBinary(init) error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("init exit = %d, want 0, stdout=%s stderr=%s", result.ExitCode, result.Stdout, result.Stderr)
	}

	canonical := readInitFile(t, repo, ".ai/skills/mivia-agent-workflows/SKILL.md")
	agents := readInitFile(t, repo, ".agents/skills/mivia-agent-workflows/SKILL.md")
	claude := readInitFile(t, repo, ".claude/skills/mivia-agent-workflows/SKILL.md")
	skillsJSON := readInitFile(t, repo, ".agents/skills.json")
	if agents != canonical {
		t.Fatalf(".agents workflow skill differs from canonical .ai skill")
	}
	for _, want := range []string{
		"name: mivia-agent-workflows",
		"triggers:",
		"./mivia-agent run --repo . --workflow <name> --dry-run --json",
		`--var objective="<free-text objective>"`,
		".ai/runs/<run-id>/<step-id>/iter-<nnn>/<artifact>",
	} {
		if !strings.Contains(canonical, want) {
			t.Fatalf("canonical workflow skill = %q, want %q", canonical, want)
		}
	}
	if !strings.Contains(claude, ".ai/skills/mivia-agent-workflows/SKILL.md") ||
		!strings.Contains(claude, "triggers:") ||
		!strings.Contains(claude, "discovery pointer") {
		t.Fatalf("Claude workflow skill = %q, want canonical pointer", claude)
	}
	if !strings.Contains(skillsJSON, `"name": "mivia-agent-workflows"`) ||
		!strings.Contains(skillsJSON, `"path": ".ai/skills/mivia-agent-workflows/SKILL.md"`) {
		t.Fatalf("skills.json = %q, want mivia-agent-workflows entry", skillsJSON)
	}
}

func tempInitGitRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	runInitGit(t, repo, "init", "-q")
	runInitGit(t, repo, "config", "user.email", "test@example.invalid")
	runInitGit(t, repo, "config", "user.name", "Test User")
	runInitGit(t, repo, "config", "commit.gpgsign", "false")
	runInitGit(t, repo, "commit", "-q", "--allow-empty", "-m", "init")
	return repo
}

func runInitGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v error = %v, output = %s", args, err, out)
	}
}

func readInitFile(t *testing.T, repo string, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(repo, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", rel, err)
	}
	return string(data)
}
