// Package importer inspects existing agent-control files for migration.
// Plan: WS7. PRD: FR-9.1, FR-9.2.
package importer

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MiviaLabs/mivia-agentkit/internal/config"
)

func TestImportPlanDoesNotWriteByDefault(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	repo := tempRepo(t)
	writeFile(t, filepath.Join(repo, "CLAUDE.md"), "legacy\n")
	cmd := exec.Command("go", "run", "./cmd/mivia-agent", "import", "--repo", repo)
	cmd.Dir = filepath.Join("..", "..")
	cmd.Env = append(os.Environ(), "HOME="+t.TempDir())
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go run import error = %v, output = %s", err, out)
	}
	if _, err := os.Stat(filepath.Join(repo, ".ai")); !os.IsNotExist(err) {
		t.Fatalf("Stat(.ai) err = %v, want not-exist", err)
	}
}

func TestImportWriteCreatesAIMappedFiles(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	repo := tempRepo(t)
	writeFile(t, filepath.Join(repo, "CLAUDE.md"), "legacy\n")
	plan, err := BuildPlan(repo, config.Defaults())
	if err != nil {
		t.Fatalf("BuildPlan() error = %v", err)
	}
	report, err := plan.Apply(repo, false)
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if len(report.Written) == 0 {
		t.Fatalf("Apply() wrote nothing: %#v", report)
	}
	got := readFile(t, filepath.Join(repo, ".ai", "imported", "instructions", "CLAUDE.md"))
	if !strings.Contains(got, "legacy") {
		t.Fatalf("mapped file = %q, want imported content", got)
	}
}

func TestImportWriteRejectsAISymlinkEscape(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	repo := tempRepo(t)
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(repo, ".ai")); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}
	plan, err := BuildPlan(repo, config.Defaults())
	if err != nil {
		t.Fatalf("BuildPlan() error = %v", err)
	}
	if _, err := plan.Apply(repo, true); err == nil {
		t.Fatal("Apply() error = nil, want symlink rejection")
	}
	if _, err := os.Stat(filepath.Join(outside, "INDEX.md")); !os.IsNotExist(err) {
		t.Fatalf("outside INDEX.md exists or Stat failed: %v", err)
	}
}

func TestImportWritePreservesExistingUserFiles(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	repo := tempRepo(t)
	path := filepath.Join(repo, "CLAUDE.md")
	writeFile(t, path, "legacy\n")
	plan, err := BuildPlan(repo, config.Defaults())
	if err != nil {
		t.Fatalf("BuildPlan() error = %v", err)
	}
	if _, err := plan.Apply(repo, false); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if got := readFile(t, path); !strings.Contains(got, "legacy") || !strings.Contains(got, ".ai/INDEX.md") {
		t.Fatalf("CLAUDE.md = %q, want legacy text plus canonical pointer", got)
	}
}

func TestImportReportsConflicts(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	repo := tempRepo(t)
	writeFile(t, filepath.Join(repo, "AGENTS.md"), "legacy\n")
	writeFile(t, filepath.Join(repo, ".ai", "imported", "rules", "AGENTS.md"), "different\n")
	plan, err := BuildPlan(repo, config.Defaults())
	if err != nil {
		t.Fatalf("BuildPlan() error = %v", err)
	}
	if len(plan.Conflicts) == 0 {
		t.Fatalf("BuildPlan() conflicts = %#v, want conflict", plan.Conflicts)
	}
}

func tempRepo(t *testing.T) string {
	t.Helper()
	return t.TempDir()
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
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
