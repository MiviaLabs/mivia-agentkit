// Package audit reports advisory mivia-agent quality gaps.
// Plan: WS3. PRD: FR-2.3, FR-5.4, FR-6.4.
package audit

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MiviaLabs/mivia-agentkit/internal/render"
	"github.com/MiviaLabs/mivia-agentkit/internal/report"
)

func TestAuditReportsDuplicatedAdapterPolicy(t *testing.T) {
	repo, home := freshRepo(t)
	block := "This canonical policy paragraph is intentionally long enough to cross the duplicate threshold and must remain verbatim only in the canonical control surface."
	writeFile(t, filepath.Join(repo, ".ai", "rules", "duplicate.md"), block+"\n")
	writeFile(t, filepath.Join(repo, "AGENTS.md"), block+"\n")
	assertCode(t, Run(Context{Repo: repo, GlobalDir: filepath.Join(home, ".agents")}), "policy.duplicated_in_adapters")

	writeFile(t, filepath.Join(repo, "AGENTS.md"), strings.Replace(block, "verbatim", "near-verbatim", 1)+"\n")
	got := Run(Context{Repo: repo, GlobalDir: filepath.Join(home, ".agents")})
	if hasCode(got.Findings, "policy.duplicated_in_adapters") {
		t.Fatalf("near duplicate findings = %+v, want no verbatim duplicate", got.Findings)
	}
}

func TestAuditReportsMissingCIForStrictProfile(t *testing.T) {
	repo, home := freshRepo(t)
	removePath(t, repo, ".github/workflows/agent-control.yml")
	assertCode(t, Run(Context{Repo: repo, GlobalDir: filepath.Join(home, ".agents"), Strict: true}), "ci.missing_control_check")
}

func TestAuditReportsNoReviewBeforeProtect(t *testing.T) {
	repo, home := freshRepo(t)
	writeFile(t, filepath.Join(repo, "mivia-agent.yaml"), strictProtectLoop("first-pass", false))
	assertCode(t, Run(Context{Repo: repo, GlobalDir: filepath.Join(home, ".agents")}), "loop.no_review_before_protect")
}

func TestAuditReportsWeakConsensusUnderStrict(t *testing.T) {
	repo, home := freshRepo(t)
	writeFile(t, filepath.Join(repo, "mivia-agent.yaml"), strictProtectLoop("first-pass", true))
	assertCode(t, Run(Context{Repo: repo, GlobalDir: filepath.Join(home, ".agents")}), "consensus.weaker_than_profile_requires")
}

func TestAuditReportsEditedManagedFileOutsideBlocks(t *testing.T) {
	repo, home := freshRepo(t)
	agents := filepath.Join(repo, "AGENTS.md")
	writeFile(t, agents, "user-added heading\n"+readFile(t, agents))
	assertCode(t, Run(Context{Repo: repo, GlobalDir: filepath.Join(home, ".agents")}), "generated.edited_outside_managed_blocks")
}

func TestAuditReportsGlobalRuleConflictWithProject(t *testing.T) {
	repo, home := freshRepo(t)
	writeFile(t, filepath.Join(home, ".agents", "rules", "20-agent-quality.md"), "global\n")
	writeFile(t, filepath.Join(repo, ".ai", "rules", "20-agent-quality.md"), "project\n")
	assertCode(t, Run(Context{Repo: repo, GlobalDir: filepath.Join(home, ".agents")}), "global.rule_conflict_with_project")
}

func strictProtectLoop(consensus string, withReview bool) string {
	review := ""
	if withReview {
		review = `
      - id: review
        reviewers: [codex]
        consensus:
          mode: ` + consensus + `
          min_reviewers: 1`
	}
	return `version: "1"
profile: strict
adapters:
  codex:
    enabled: true
    role: orchestrable
governance:
  provider: noop
loops:
  release:
    bound: iterations
    max_iterations: 1
    exit_when: protected_action
    steps:
      - id: produce
        producer: codex` + review + `
`
}

func freshRepo(t *testing.T) (string, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	repo := t.TempDir()
	runGit(t, repo, "init", "-q")
	runGit(t, repo, "config", "user.email", "test@example.invalid")
	runGit(t, repo, "config", "user.name", "Test User")
	runGit(t, repo, "commit", "-q", "--allow-empty", "-m", "init")
	if _, err := render.WriteInit(render.InitConfig{Repo: repo, Profile: "standard", Adapters: []string{"codex", "claude", "copilot"}}); err != nil {
		t.Fatalf("WriteInit() error = %v, want nil", err)
	}
	return repo, home
}

func assertCode(t *testing.T, got report.Report, code string) {
	t.Helper()
	if !hasCode(got.Findings, code) {
		t.Fatalf("Run() findings = %+v, want code %s", got.Findings, code)
	}
}

func hasCode(findings []report.Finding, code string) bool {
	for _, finding := range findings {
		if finding.Code == code {
			return true
		}
	}
	return false
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v error = %v, output = %s", args, err, out)
	}
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

func removePath(t *testing.T, root, rel string) {
	t.Helper()
	if err := os.RemoveAll(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
		t.Fatalf("RemoveAll(%s) error = %v", rel, err)
	}
}
