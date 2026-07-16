// Package integration runs real built-binary and subprocess coverage for shipped command surfaces.
// Plan: WS14. PRD: §3, §4, §7, §9, §14.
package integration

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	cliint "github.com/MiviaLabs/mivia-agentkit/internal/cli"
)

func TestInitDoctorAuditPreflightFlow(t *testing.T) {
	env := newIntegrationEnv(t)

	initResult := env.run(t,
		"init",
		"--repo", env.repo,
		"--profile", "standard",
		"--adapter", "codex",
		"--adapter", "claude",
		"--adapter", "copilot",
		"--write",
	)
	if initResult.ExitCode != 0 {
		t.Fatalf("init exit = %d, stdout=%s stderr=%s", initResult.ExitCode, initResult.Stdout, initResult.Stderr)
	}
	if _, err := os.Stat(filepath.Join(env.repo, ".ai", "INDEX.md")); err != nil {
		t.Fatalf("Stat(.ai/INDEX.md) error = %v", err)
	}

	doctorResult := env.run(t, "doctor", "--repo", env.repo, "--json")
	var doctorReport struct {
		ExitCode int `json:"exit_code"`
	}
	decodeJSON(t, doctorResult.Stdout, &doctorReport)
	if doctorReport.ExitCode != 0 {
		t.Fatalf("doctor exit_code = %d, stdout=%s stderr=%s", doctorReport.ExitCode, doctorResult.Stdout, doctorResult.Stderr)
	}

	auditResult := env.run(t, "audit", "--repo", env.repo, "--json")
	var auditReport struct {
		Findings []struct {
			Severity string `json:"severity"`
		} `json:"findings"`
	}
	decodeJSON(t, auditResult.Stdout, &auditReport)
	for _, finding := range auditReport.Findings {
		if finding.Severity == "error" {
			t.Fatalf("audit findings contain error severity: stdout=%s stderr=%s", auditResult.Stdout, auditResult.Stderr)
		}
	}

	preflightResult := env.run(t,
		"preflight",
		"--repo", env.repo,
		"--contract-row", "ws14",
		"--focused-verifier", "go test ./test/integration/... -count=1",
		"--mutation-proof", "ws14 integration flow coverage",
		"--json",
	)
	if preflightResult.ExitCode != 0 {
		t.Fatalf("preflight exit = %d, stdout=%s stderr=%s", preflightResult.ExitCode, preflightResult.Stdout, preflightResult.Stderr)
	}
	if _, err := os.Stat(filepath.Join(env.repo, ".git", "mivia-agent-quality-stamp.json")); err != nil {
		t.Fatalf("quality stamp missing: %v", err)
	}
}

func TestUpdatePreservesUserContentOutsideManagedRegions(t *testing.T) {
	env := newIntegrationEnv(t)
	env.run(t,
		"init",
		"--repo", env.repo,
		"--profile", "standard",
		"--adapter", "codex",
		"--adapter", "claude",
		"--adapter", "copilot",
		"--write",
	)

	agentsPath := filepath.Join(env.repo, "AGENTS.md")
	original := readFile(t, agentsPath)
	withUserText := "user preface\n" + original + "\nuser tail\n"
	withUserText = strings.Replace(withUserText, "Run configured verification before claiming completion.", "legacy verification line", 1)
	mustWriteFile(t, agentsPath, withUserText)

	manifestPath := filepath.Join(env.repo, "mivia-agent.yaml")
	manifest := strings.Replace(readFile(t, manifestPath), "template_version: dev", "template_version: v0.0.1", 1)
	mustWriteFile(t, manifestPath, manifest)

	updateResult := env.run(t, "update", "--repo", env.repo, "--write")
	if updateResult.ExitCode != 0 {
		t.Fatalf("update exit = %d, stdout=%s stderr=%s", updateResult.ExitCode, updateResult.Stdout, updateResult.Stderr)
	}

	got := readFile(t, agentsPath)
	for _, want := range []string{"user preface", "user tail", "Run configured verification before claiming completion."} {
		if !strings.Contains(got, want) {
			t.Fatalf("AGENTS.md = %q, missing %q", got, want)
		}
	}
}

func TestImportInspectsWithoutWriting(t *testing.T) {
	env := newIntegrationEnv(t)
	mustWriteFile(t, filepath.Join(env.repo, "CLAUDE.md"), "legacy\n")

	result := env.run(t, "import", "--repo", env.repo, "--json")
	if result.ExitCode != 0 {
		t.Fatalf("import exit = %d, stdout=%s stderr=%s", result.ExitCode, result.Stdout, result.Stderr)
	}
	if !strings.Contains(result.Stdout, ".ai/imported/instructions/CLAUDE.md") {
		t.Fatalf("import stdout = %s, want mapped Claude target", result.Stdout)
	}
	if _, err := os.Stat(filepath.Join(env.repo, ".ai")); !os.IsNotExist(err) {
		t.Fatalf("Stat(.ai) err = %v, want no writes in inspect mode", err)
	}
}

func TestVersionCommandOutputsStructuredBuildInfo(t *testing.T) {
	env := newIntegrationEnv(t)
	bin, err := cliint.BuildBinary(cliint.BinaryBuild{
		ModuleRoot: filepath.Join("..", ".."),
		LDFlags: "-X github.com/MiviaLabs/mivia-agentkit/internal/version.Version=v1.2.3-test " +
			"-X github.com/MiviaLabs/mivia-agentkit/internal/version.Commit=abc1234 " +
			"-X github.com/MiviaLabs/mivia-agentkit/internal/version.Date=2026-07-05T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("BuildBinary(ldflags) error = %v", err)
	}
	result, err := cliint.RunBinary(context.Background(), bin, cliint.BinaryRun{
		Args: []string{"version", "--json"},
		Dir:  env.repo,
		Env:  env.env(),
		Scrub: map[string]string{
			env.repo: filepath.ToSlash("<repo>"),
			env.home: filepath.ToSlash("<home>"),
		},
	})
	if err != nil {
		t.Fatalf("RunBinary(version --json) error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("version exit = %d, stdout=%s stderr=%s", result.ExitCode, result.Stdout, result.Stderr)
	}
	var buildInfo struct {
		Version string `json:"version"`
		Commit  string `json:"commit"`
		Date    string `json:"date"`
	}
	decodeJSON(t, result.Stdout, &buildInfo)
	if buildInfo.Version != "v1.2.3-test" || buildInfo.Commit != "abc1234" || buildInfo.Date != "2026-07-05T00:00:00Z" {
		t.Fatalf("build info = %#v, want exact ldflags-backed version, commit, and date", buildInfo)
	}
}

func TestTempGitRepoDisablesGlobalSigning(t *testing.T) {
	global := filepath.Join(t.TempDir(), "gitconfig")
	if err := os.WriteFile(global, []byte("[commit]\n\tgpgsign = true\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(global config) error = %v", err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", global)
	repo := tempGitRepo(t)
	mustWriteFile(t, filepath.Join(repo, "README.md"), "hello\n")
	runGit(t, repo, "add", "README.md")
	runGit(t, repo, "commit", "-q", "-m", "docs")
}
