// Package cli tests campaign commands.
// Plan: WS15.
package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MiviaLabs/mivia-agentkit/internal/auditcampaign"
	"github.com/MiviaLabs/mivia-agentkit/internal/gitstate"
)

func TestCampaignCLIRejectsNonInteractiveContinuous(t *testing.T) {
	t.Setenv("CI", "true")
	root := NewRootCommand()
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"campaign", "run", "--repo", t.TempDir(), "--campaign", "x", "--continuous"})
	err := root.Execute()
	if err == nil {
		t.Fatalf("want error for CI continuous")
	}
	if !strings.Contains(err.Error(), "interactive") && !strings.Contains(err.Error(), "CI") && !strings.Contains(err.Error(), "TTY") {
		t.Fatalf("error = %v, want interactive/CI rejection", err)
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
	t.Setenv("HOME", t.TempDir())
	for _, k := range []string{"CI", "GITHUB_ACTIONS", "GITLAB_CI", "BUILDKITE", "CIRCLECI", "TF_BUILD"} {
		t.Setenv(k, "")
	}
	repo := tempCampaignGitRepo(t)
	writeCampaignManifest(t, repo, campaignManifestOpts{
		enabled:       true,
		commitEnabled: false,
		maxCycles:     5,
	})

	bin, err := BuildBinary(BinaryBuild{ModuleRoot: filepath.Join("..", "..")})
	if err != nil {
		t.Fatalf("BuildBinary: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	result, err := RunBinary(ctx, bin, BinaryRun{
		Args: []string{"campaign", "run", "--repo", repo, "--campaign", "deep-bug-audit-repair", "--json"},
		Dir:  repo,
		Env:  []string{"CI=", "GITHUB_ACTIONS="},
		Scrub: map[string]string{repo: "<repo>"},
	})
	if err != nil {
		t.Fatalf("RunBinary: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("exit = %d stdout=%s stderr=%s", result.ExitCode, result.Stdout, result.Stderr)
	}
	var res auditcampaign.Result
	if err := json.Unmarshal([]byte(result.Stdout), &res); err != nil {
		t.Fatalf("decode result: %v stdout=%s", err, result.Stdout)
	}
	if res.Terminal != auditcampaign.TerminalClean {
		t.Fatalf("terminal = %s, want clean; full=%s", res.Terminal, result.Stdout)
	}
	if res.Commits != 0 {
		t.Fatalf("commits = %d, want 0", res.Commits)
	}
	if res.Cycles < 2 {
		t.Fatalf("cycles = %d, want >= 2", res.Cycles)
	}
}

func TestCampaignCLIBuiltBinaryScopedCommit(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	for _, k := range []string{"CI", "GITHUB_ACTIONS", "GITLAB_CI", "BUILDKITE", "CIRCLECI", "TF_BUILD"} {
		t.Setenv(k, "")
	}
	repo := tempCampaignGitRepo(t)
	writeCampaignManifest(t, repo, campaignManifestOpts{
		enabled:       true,
		commitEnabled: true,
		maxCycles:     6,
	})

	fixtureDir := filepath.Join(repo, ".ai", "campaign-fixtures", "deep-bug-audit-repair")
	if err := os.MkdirAll(fixtureDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Auditor emits candidate; independent confirmer supplies commit-eligible evidence.
	audit1 := `{
  "schema": "mivia-agent-campaign-evidence/v1",
  "campaign_run": "placeholder",
  "cycle": 1,
  "baseline_head": "placeholder",
  "disposition": "candidate",
  "finding_fingerprint": "fp-scoped-1",
  "finding_claim": "missing fixme content under allowlisted path",
  "path_hints": ["fixme.txt"]
}`
	confirm1 := `{
  "schema": "mivia-agent-campaign-evidence/v1",
  "campaign_run": "placeholder",
  "cycle": 1,
  "baseline_head": "placeholder",
  "disposition": "confirmed",
  "finding_fingerprint": "fp-scoped-1",
  "finding_claim": "missing fixme content under allowlisted path",
  "path_hints": ["fixme.txt"],
  "changed_path_ids": ["p1"],
  "verifier_ref": "true"
}`
	fix1 := `{
  "schema": "mivia-agent-campaign-evidence/v1",
  "campaign_run": "placeholder",
  "cycle": 1,
  "baseline_head": "placeholder",
  "disposition": "fixed",
  "finding_fingerprint": "fp-scoped-1",
  "finding_claim": "missing fixme content under allowlisted path",
  "path_hints": ["fixme.txt"],
  "changed_path_ids": ["p1"],
  "verifier_ref": "true"
}`
	if err := os.WriteFile(filepath.Join(fixtureDir, "audit-cycle-1.json"), []byte(audit1), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fixtureDir, "confirm-cycle-1.json"), []byte(confirm1), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fixtureDir, "fix-cycle-1.json"), []byte(fix1), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fixtureDir, "fix-writes-cycle-1.json"), []byte(`{"files":{"fixme.txt":"fixed\n"}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	before, err := gitstate.Head(repo)
	if err != nil {
		t.Fatal(err)
	}

	bin, err := BuildBinary(BinaryBuild{ModuleRoot: filepath.Join("..", "..")})
	if err != nil {
		t.Fatalf("BuildBinary: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	result, err := RunBinary(ctx, bin, BinaryRun{
		Args: []string{"campaign", "run", "--repo", repo, "--campaign", "deep-bug-audit-repair", "--json"},
		Dir:  repo,
		Env:  []string{"CI=", "GITHUB_ACTIONS="},
	})
	if err != nil {
		t.Fatalf("RunBinary: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("exit = %d stdout=%s stderr=%s", result.ExitCode, result.Stdout, result.Stderr)
	}
	var res auditcampaign.Result
	if err := json.Unmarshal([]byte(result.Stdout), &res); err != nil {
		t.Fatalf("decode: %v stdout=%s", err, result.Stdout)
	}
	if res.Commits != 1 {
		t.Fatalf("commits = %d, want 1; out=%s", res.Commits, result.Stdout)
	}
	after, err := gitstate.Head(repo)
	if err != nil {
		t.Fatal(err)
	}
	if after == before {
		t.Fatalf("HEAD did not advance after scoped commit")
	}
	body, err := os.ReadFile(filepath.Join(repo, "fixme.txt"))
	if err != nil {
		t.Fatalf("fixme.txt missing after commit: %v", err)
	}
	if string(body) != "fixed\n" {
		t.Fatalf("fixme.txt = %q", body)
	}
	if res.Terminal != auditcampaign.TerminalClean && res.Terminal != auditcampaign.TerminalCycleCap {
		t.Fatalf("terminal = %s, want clean or cycle_cap", res.Terminal)
	}
}

func TestCampaignCLIRejectsSelfConfirmCommit(t *testing.T) {
	repo := tempCampaignGitRepo(t)
	yaml := `
version: "1"
profile: standard
adapters:
  local:
    enabled: true
    role: orchestrable
loops:
  bug-audit-loop:
    bound: iterations
    max_iterations: 2
    steps:
      - id: audit
        producer: local
  fix-loop:
    bound: iterations
    max_iterations: 1
    steps:
      - id: fix
        producer: local
campaigns:
  bad:
    enabled: true
    audit_workflow: bug-audit-loop
    fix_workflow: fix-loop
    auditor: local
    confirmer: local
    commit_enabled: true
    verifier_profile: true
    allowed_paths: ["fixme.txt"]
    commit_message_template: "fix(quality): bad"
`
	if err := os.WriteFile(filepath.Join(repo, "mivia-agent.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := loadCampaignManifest(repo)
	if err == nil || !strings.Contains(err.Error(), "independent confirmer") {
		t.Fatalf("error = %v, want independent confirmer rejection", err)
	}
}

type campaignManifestOpts struct {
	enabled       bool
	commitEnabled bool
	maxCycles     int
}

func writeCampaignManifest(t *testing.T, repo string, opt campaignManifestOpts) {
	t.Helper()
	if opt.maxCycles <= 0 {
		opt.maxCycles = 5
	}
	yaml := fmt.Sprintf(`version: "1"
profile: standard
adapters:
  local:
    enabled: true
    role: orchestrable
  local-confirm:
    enabled: true
    role: orchestrable
loops:
  bug-audit-loop:
    bound: iterations
    max_iterations: 2
    steps:
      - id: audit
        producer: local
  fix-loop:
    bound: iterations
    max_iterations: 1
    steps:
      - id: fix
        producer: local
governance:
  provider: noop
  audit_log: .ai/audit.jsonl
campaigns:
  deep-bug-audit-repair:
    enabled: %t
    audit_workflow: bug-audit-loop
    fix_workflow: fix-loop
    auditor: local
    confirmer: local-confirm
    clean_pass_threshold: 2
    max_cycles: %d
    max_duration: 10m
    max_repair_attempts: 2
    no_progress_threshold: 2
    commit_enabled: %t
    verifier_profile: true
    allowed_paths:
      - fixme.txt
    commit_message_template: "fix(quality): campaign scoped repair"
    on_exhausted: fail
`, opt.enabled, opt.maxCycles, opt.commitEnabled)
	if err := os.WriteFile(filepath.Join(repo, "mivia-agent.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	// Commit baseline so scoped-commit dirty checks only see allowed repair paths.
	cmd := exec.Command("git", "-C", repo, "add", "mivia-agent.yaml")
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add manifest: %v: %s", err, out)
	}
	cmd = exec.Command("git", "-C", repo, "commit", "-m", "add campaign manifest")
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit manifest: %v: %s", err, out)
	}
}

func tempCampaignGitRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	run("init")
	run("config", "user.email", "t@example.com")
	run("config", "user.name", "t")
	run("config", "commit.gpgsign", "false")
	// Campaign runtime state and fixtures are gitignored (product contract for .ai/runs).
	ignore := strings.Join([]string{
		".ai/runs/",
		".ai/campaign-fixtures/",
		".ai/audit.jsonl",
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(repo, ".gitignore"), []byte(ignore), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "README"), []byte("hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "README", ".gitignore")
	run("commit", "-m", "init")
	return repo
}
