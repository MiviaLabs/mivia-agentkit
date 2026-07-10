// Package integration runs real built-binary and subprocess coverage for shipped command surfaces.
// Plan: WS14. PRD: §3, §4, §5, §7, §14.
package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHookRejectsProtectedActionWithoutFreshStamp(t *testing.T) {
	env := newIntegrationEnv(t)
	result := env.runWithInput(t, []byte(`{"tool":"bash","command":"git push"}`), "hook", "codex", "pre-tool-use", "--repo", env.repo)
	if result.ExitCode != 0 {
		t.Fatalf("hook exit = %d, stdout=%s stderr=%s", result.ExitCode, result.Stdout, result.Stderr)
	}
	if !strings.Contains(result.Stdout, `"permissionDecision": "deny"`) || !strings.Contains(result.Stdout, "quality stamp required") {
		t.Fatalf("hook stdout = %s, want protected-action denial", result.Stdout)
	}
}

func TestPushAcceptsValidatedTipWithoutRerun(t *testing.T) {
	env := newIntegrationEnv(t)
	writeTrustedVerifierManifest(t, env.repo)
	mustWriteFile(t, filepath.Join(env.repo, "docs", "validated.md"), "validated\n")
	runGit(t, env.repo, "add", "docs/validated.md")
	preflight := env.run(t, "preflight", "--repo", env.repo, "--broad-verifier", "true")
	if preflight.ExitCode != 0 {
		t.Fatalf("preflight exit = %d stderr=%s", preflight.ExitCode, preflight.Stderr)
	}
	runGit(t, env.repo, "commit", "-q", "-m", "validated")
	result := env.runWithInput(t, []byte(`{"tool":"bash","command":"git push origin HEAD"}`), "hook", "codex", "pre-tool-use", "--repo", env.repo)
	if result.ExitCode != 0 || result.Stdout != "" {
		t.Fatalf("push hook exit=%d stdout=%s stderr=%s; want accepted promoted evidence", result.ExitCode, result.Stdout, result.Stderr)
	}
}

func TestPushRejectsUnvalidatedAdditionalCommit(t *testing.T) {
	env := newIntegrationEnv(t)
	writeTrustedVerifierManifest(t, env.repo)
	mustWriteFile(t, filepath.Join(env.repo, "docs", "validated.md"), "validated\n")
	runGit(t, env.repo, "add", "docs/validated.md")
	if result := env.run(t, "preflight", "--repo", env.repo, "--broad-verifier", "true"); result.ExitCode != 0 {
		t.Fatalf("preflight exit=%d stderr=%s", result.ExitCode, result.Stderr)
	}
	runGit(t, env.repo, "commit", "-q", "-m", "validated")
	if result := env.runWithInput(t, []byte(`{"tool":"bash","command":"git push origin HEAD"}`), "hook", "codex", "pre-tool-use", "--repo", env.repo); result.ExitCode != 0 || result.Stdout != "" {
		t.Fatalf("first push hook = %+v; want promotion", result)
	}
	mustWriteFile(t, filepath.Join(env.repo, "docs", "unvalidated.md"), "unvalidated\n")
	runGit(t, env.repo, "add", "docs/unvalidated.md")
	runGit(t, env.repo, "commit", "-q", "-m", "unvalidated")
	result := env.runWithInput(t, []byte(`{"tool":"bash","command":"git push origin HEAD"}`), "hook", "codex", "pre-tool-use", "--repo", env.repo)
	if result.ExitCode != 0 || !strings.Contains(result.Stdout, `"permissionDecision": "deny"`) || !strings.Contains(result.Stdout, "validated commit is not HEAD") {
		t.Fatalf("second push hook exit=%d stdout=%s stderr=%s; want unvalidated commit denial", result.ExitCode, result.Stdout, result.Stderr)
	}
}

func TestRunDryRunProducesBoundedPlan(t *testing.T) {
	env := newIntegrationEnv(t)
	writeResearchWorkflow(t, env.repo)

	result := env.run(t, "run", "--repo", env.repo, "--workflow", "research", "--dry-run", "--json")
	if result.ExitCode != 0 {
		t.Fatalf("run dry-run exit = %d, stdout=%s stderr=%s", result.ExitCode, result.Stdout, result.Stderr)
	}
	var rows []struct {
		Step     string   `json:"step"`
		Type     string   `json:"type"`
		Adapters []string `json:"adapters"`
	}
	decodeJSON(t, result.Stdout, &rows)
	if len(rows) != 2 {
		t.Fatalf("rows = %#v, want two bounded workflow steps", rows)
	}
	if _, err := os.Stat(filepath.Join(env.repo, ".ai", "runs")); !os.IsNotExist(err) {
		t.Fatalf("run dry-run wrote .ai/runs: err = %v", err)
	}
}

func TestRunBinaryUsesManifestWeightedConsensus(t *testing.T) {
	env := newIntegrationEnv(t)
	toolsDir := t.TempDir()
	buildStubCLI(t, toolsDir, stubCLI{Name: "codex", Version: "codex 1.2.3", Stdout: `{"pass":false,"severity":"high","notes":"reject"}`})
	buildStubCLI(t, toolsDir, stubCLI{Name: "claude", Version: "claude 2.1.200", Stdout: `{"pass":true,"severity":"low","notes":"approve"}`})
	mustWriteFile(t, filepath.Join(env.repo, "mivia-agent.yaml"), "version: \"1\"\nadapters:\n  codex: {enabled: true, role: orchestrable}\n  claude: {enabled: true, role: orchestrable}\nrouting:\n  consensus:\n    mode: weighted\n    min_reviewers: 2\n    weights: {codex: 1, claude: 3}\nloops:\n  weighted:\n    bound: iterations\n    max_iterations: 1\n    steps:\n      - id: produce\n        producer: codex\n        artifact: out.md\n      - id: review\n        reviewers: [codex, claude]\n        artifact: out.md\n    exit_when: review-pass\n    on_exhausted: fail\n")
	env = env.withEnv("PATH=" + toolsDir + string(os.PathListSeparator) + os.Getenv("PATH"))
	result := env.run(t, "run", "--repo", env.repo, "--workflow", "weighted", "--json")
	if result.ExitCode != 0 || !strings.Contains(result.Stdout, `"Outcome":"pass"`) {
		t.Fatalf("weighted run exit=%d stdout=%s stderr=%s; want manifest-weighted pass despite one rejection", result.ExitCode, result.Stdout, result.Stderr)
	}
}

func TestReviewProducesConsensusReport(t *testing.T) {
	env := newIntegrationEnv(t)
	artifactPath := filepath.Join(env.repo, "artifact.md")
	mustWriteFile(t, artifactPath, "review me")

	toolsDir := t.TempDir()
	codexLog := filepath.Join(toolsDir, "codex.log")
	claudeLog := filepath.Join(toolsDir, "claude.log")
	buildStubCLI(t, toolsDir, stubCLI{
		Name:    "codex",
		Version: "codex 1.2.3",
		Stdout:  `{"pass":true,"severity":"low","notes":"ok","model_id":"gpt-5","total_tokens":12}`,
		LogPath: codexLog,
	})
	buildStubCLI(t, toolsDir, stubCLI{
		Name:    "claude",
		Version: "claude 2.1.200",
		Stdout:  `{"pass":true,"severity":"low","notes":"ok","model":"sonnet","total_tokens":8}`,
		LogPath: claudeLog,
	})

	env = env.withEnv("PATH=" + toolsDir + string(os.PathListSeparator) + os.Getenv("PATH"))
	result := env.run(t,
		"review",
		"--repo", env.repo,
		"--artifact", artifactPath,
		"--reviewers", "codex,claude",
		"--mode", "majority",
		"--min-reviewers", "2",
		"--json",
	)
	if result.ExitCode != 0 {
		t.Fatalf("review exit = %d, stdout=%s stderr=%s", result.ExitCode, result.Stdout, result.Stderr)
	}
	if !strings.Contains(result.Stdout, `"pass":true`) || !strings.Contains(result.Stdout, `"adapter":"codex"`) || !strings.Contains(result.Stdout, `"adapter":"claude"`) {
		t.Fatalf("review stdout = %s, want structured consensus report", result.Stdout)
	}
	readLogContains(t, codexLog, `--config approval_policy="never"`)
	readLogContains(t, claudeLog, "--permission-mode never")
}
