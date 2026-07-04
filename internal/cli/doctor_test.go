// Package cli implements the mivia-agent command surface.
// Plan: WS3. PRD: FR-2.1, FR-2.3.
package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MiviaLabs/mivia-agentkit/internal/render"
)

func TestDoctorCmdExitsOneOnFinding(t *testing.T) {
	repo := freshRepo(t)
	if err := os.Remove(filepath.Join(repo, ".ai", "INDEX.md")); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"doctor", "--repo", repo, "--json"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "doctor failed") {
		t.Fatalf("Execute() error = %v, want doctor failed", err)
	}
	if !strings.Contains(out.String(), "ai.index_missing") {
		t.Fatalf("doctor output = %q, want ai.index_missing", out.String())
	}
}

func TestAuditCmdExitsZeroUnlessStrict(t *testing.T) {
	repo := freshRepo(t)
	if err := os.RemoveAll(filepath.Join(repo, ".github", "workflows", "agent-control.yml")); err != nil {
		t.Fatalf("RemoveAll() error = %v", err)
	}
	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"audit", "--repo", repo})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v, want nil without strict", err)
	}

	strictCmd := NewRootCommand()
	out.Reset()
	strictCmd.SetOut(&out)
	strictCmd.SetErr(&out)
	strictCmd.SetArgs([]string{"audit", "--repo", repo, "--strict"})
	err := strictCmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "audit failed") {
		t.Fatalf("Execute(strict) error = %v, want audit failed", err)
	}
}

func freshRepo(t *testing.T) string {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	repo := t.TempDir()
	runGit(t, repo, "init", "-q")
	runGit(t, repo, "config", "user.email", "test@example.invalid")
	runGit(t, repo, "config", "user.name", "Test User")
	runGit(t, repo, "commit", "-q", "--allow-empty", "-m", "init")
	if _, err := render.WriteInit(render.InitConfig{Repo: repo, Profile: "standard", Adapters: []string{"codex", "claude", "copilot"}}); err != nil {
		t.Fatalf("WriteInit() error = %v, want nil", err)
	}
	return repo
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v error = %v, output = %s", args, err, out)
	}
}
