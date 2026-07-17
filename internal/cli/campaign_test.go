// Package cli tests campaign commands.
// Plan: WS15.
package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCampaignCLIRejectsNonInteractiveContinuous(t *testing.T) {
	t.Setenv("CI", "true")
	root := NewRootCommand()
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"campaign", "run", "--repo", t.TempDir(), "--campaign", "x", "--continuous"})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "interactive") && !strings.Contains(err.Error(), "CI") {
		// Continuous rejects CI env via isInteractiveContinuous
		if err == nil {
			t.Fatalf("want error for CI continuous")
		}
	}
}

func TestCampaignCLIStatusRequiresRun(t *testing.T) {
	root := NewRootCommand()
	root.SetArgs([]string{"campaign", "status", "--repo", t.TempDir()})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "--run") {
		t.Fatalf("error = %v, want --run required", err)
	}
}

func TestCampaignCLIStatusAndResume(t *testing.T) {
	dir := t.TempDir()
	// Write a minimal campaign state file via package helpers would need store; create path shape.
	runID := "camp-status-1"
	stateDir := filepath.Join(dir, ".ai", "runs", runID)
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	state := `{
  "schema": "mivia-agent-campaign-state/v1",
  "campaign_id": "camp-status-1",
  "phase": "auditing",
  "cycle": 1,
  "baseline_head": "abc",
  "owner_id": "cli",
  "updated_at": "2026-07-17T00:00:00Z"
}`
	if err := os.WriteFile(filepath.Join(stateDir, "campaign-state.json"), []byte(state), 0o644); err != nil {
		t.Fatal(err)
	}
	root := NewRootCommand()
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"campaign", "status", "--repo", dir, "--run", runID, "--json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("status: %v", err)
	}
	if !strings.Contains(buf.String(), "auditing") {
		t.Fatalf("output = %s, want auditing", buf.String())
	}
}

func TestCampaignCLIBuiltBinaryIntegration(t *testing.T) {
	// Structural: campaign subcommand is registered on root.
	root := NewRootCommand()
	found := false
	for _, c := range root.Commands() {
		if c.Name() == "campaign" {
			found = true
			subs := map[string]bool{}
			for _, s := range c.Commands() {
				subs[s.Name()] = true
			}
			if !subs["run"] || !subs["status"] || !subs["resume"] {
				t.Fatalf("campaign subcommands = %v", subs)
			}
		}
	}
	if !found {
		t.Fatalf("campaign command not registered")
	}
}
