// Package doctor validates installed mivia-agent control surfaces.
// Plan: WS3. PRD: FR-2.1, FR-5.4, FR-10.5.
package doctor

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MiviaLabs/mivia-agentkit/internal/render"
	"github.com/MiviaLabs/mivia-agentkit/internal/report"
)

func TestDoctorPassesFreshInit(t *testing.T) {
	repo, home := freshRepo(t)
	got := Run(Context{Repo: repo, GlobalDir: filepath.Join(home, ".agents")})
	if hasSeverity(got.Findings, report.SeverityError) {
		t.Fatalf("Run() errors = %+v, want none", got.Findings)
	}
}

func TestDoctorFailsMissingAIIndex(t *testing.T) {
	repo, home := freshRepo(t)
	removePath(t, repo, ".ai/INDEX.md")
	assertCode(t, Run(Context{Repo: repo, GlobalDir: filepath.Join(home, ".agents")}), "ai.index_missing")
}

func TestDoctorFailsMissingAdapterFile(t *testing.T) {
	repo, home := freshRepo(t)
	removePath(t, repo, ".codex/hooks.json")
	assertCode(t, Run(Context{Repo: repo, GlobalDir: filepath.Join(home, ".agents")}), "adapter.file_missing")
}

func TestDoctorFailsMissingCodexAdapterInstructions(t *testing.T) {
	repo, home := freshRepo(t)
	removePath(t, repo, ".codex/AGENTS.md")
	assertCode(t, Run(Context{Repo: repo, GlobalDir: filepath.Join(home, ".agents")}), "adapter.file_missing")
}

func TestDoctorFailsHookNotCallingMiviaAgent(t *testing.T) {
	repo, home := freshRepo(t)
	writeFile(t, filepath.Join(repo, ".codex", "hooks.json"), `{"PreToolUse":[{"command":"echo bypass"}]}`)
	assertCode(t, Run(Context{Repo: repo, GlobalDir: filepath.Join(home, ".agents")}), "hooks.missing_mivia_agent")
}

func TestDoctorFailsLoopWithNoBound(t *testing.T) {
	repo, home := freshRepo(t)
	writeFile(t, filepath.Join(repo, "mivia-agent.yaml"), manifestWithLoop("bound: \"\"\n    max_iterations: 1"))
	assertCode(t, Run(Context{Repo: repo, GlobalDir: filepath.Join(home, ".agents")}), "loop.unbounded")
}

func TestDoctorFailsLoopReferencingUnknownAdapter(t *testing.T) {
	repo, home := freshRepo(t)
	writeFile(t, filepath.Join(repo, "mivia-agent.yaml"), manifestWithLoop("bound: iterations\n    max_iterations: 1\n    steps:\n      - id: produce\n        producer: missing"))
	assertCode(t, Run(Context{Repo: repo, GlobalDir: filepath.Join(home, ".agents")}), "loop.unknown_adapter")
}

func TestDoctorFailsConsensusMinReviewersUnsatisfiable(t *testing.T) {
	repo, home := freshRepo(t)
	writeFile(t, filepath.Join(repo, "mivia-agent.yaml"), manifestWithLoop("bound: iterations\n    max_iterations: 1\n    steps:\n      - id: review\n        reviewers: [codex]\n        consensus:\n          min_reviewers: 3"))
	assertCode(t, Run(Context{Repo: repo, GlobalDir: filepath.Join(home, ".agents")}), "consensus.unsatisfiable")
}

func TestDoctorFailsUnknownGovernanceProvider(t *testing.T) {
	repo, home := freshRepo(t)
	data := strings.Replace(readFile(t, filepath.Join(repo, "mivia-agent.yaml")), "provider: noop", "provider: unknown", 1)
	writeFile(t, filepath.Join(repo, "mivia-agent.yaml"), data)
	assertCode(t, Run(Context{Repo: repo, GlobalDir: filepath.Join(home, ".agents")}), "governance.provider_unknown")
}

func TestDoctorWarnsGlobalRuleConflict(t *testing.T) {
	repo, home := freshRepo(t)
	writeFile(t, filepath.Join(home, ".agents", "rules", "00-operating-doctrine.md"), "global\n")
	writeFile(t, filepath.Join(repo, ".ai", "rules", "00-operating-doctrine.md"), "project\n")
	got := Run(Context{Repo: repo, GlobalDir: filepath.Join(home, ".agents")})
	assertCode(t, got, "global.rule_conflict")
	if got.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0 for non-strict warning", got.ExitCode)
	}
}

func TestDoctorPassesWithNoGlobalConfig(t *testing.T) {
	repo, home := freshRepo(t)
	removePath(t, home, ".agents")
	got := Run(Context{Repo: repo, GlobalDir: filepath.Join(home, ".agents")})
	if hasCode(got.Findings, "global.rule_conflict") || hasCode(got.Findings, "global.unreadable") {
		t.Fatalf("Run() global findings = %+v, want none", got.Findings)
	}
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

func manifestWithLoop(loopBody string) string {
	return `version: "1"
profile: standard
adapters:
  codex:
    enabled: true
    role: orchestrable
  claude:
    enabled: true
    role: orchestrable
  copilot:
    enabled: true
    role: guidance
governance:
  provider: noop
loops:
  research:
    ` + loopBody + "\n"
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

func hasSeverity(findings []report.Finding, severity report.Severity) bool {
	for _, finding := range findings {
		if finding.Severity == severity {
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
