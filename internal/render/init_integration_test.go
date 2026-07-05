// Package render renders embedded templates into target repo files.
// Plan: WS2. PRD: FR-1.1, FR-1.2, FR-1.3, FR-10.6.
package render

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MiviaLabs/mivia-agentkit/internal/config"
)

func TestInitDryRunWritesNothing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	repo := tempGitRepo(t)
	report, err := PreviewInit(InitConfig{Repo: repo, Profile: "standard", Adapters: []string{"codex"}})
	if err != nil {
		t.Fatalf("PreviewInit() error = %v, want nil", err)
	}
	if len(report.FilesCreated) == 0 {
		t.Fatal("PreviewInit() created list empty, want intended writes")
	}
	if files := repoFiles(t, repo); len(files) != 0 {
		t.Fatalf("PreviewInit() wrote files: %#v", files)
	}
}

func TestInitDryRunReportsGitignoreWithoutWriting(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	repo := tempGitRepo(t)
	report, err := PreviewInit(InitConfig{Repo: repo, Profile: "standard", Adapters: []string{"codex"}})
	if err != nil {
		t.Fatalf("PreviewInit() error = %v, want nil", err)
	}
	if !containsString(report.FilesCreated, ".gitignore") {
		t.Fatalf("FilesCreated = %#v, want .gitignore", report.FilesCreated)
	}
	if _, err := os.Stat(filepath.Join(repo, ".gitignore")); !os.IsNotExist(err) {
		t.Fatalf("Stat(.gitignore) error = %v, want not exist after dry-run", err)
	}
}

func TestInitWriteCreatesExpectedFiles(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	repo := tempGitRepo(t)
	if _, err := WriteInit(InitConfig{Repo: repo, Profile: "standard", Adapters: []string{"codex", "claude", "copilot"}}); err != nil {
		t.Fatalf("WriteInit() error = %v, want nil", err)
	}
	for _, rel := range []string{"AGENTS.md", ".ai/INDEX.md", ".codex/hooks.json", "CLAUDE.md", ".agents/skills.json"} {
		if _, err := os.Stat(filepath.Join(repo, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("Stat(%q) error = %v, want nil", rel, err)
		}
	}
}

func TestInitWriteCreatesMiviaAgentWorkflowSkills(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	repo := tempGitRepo(t)
	if _, err := WriteInit(InitConfig{Repo: repo, Profile: "standard", Adapters: []string{"codex", "claude"}}); err != nil {
		t.Fatalf("WriteInit() error = %v, want nil", err)
	}
	canonical := readFile(t, filepath.Join(repo, ".ai", "skills", "mivia-agent-workflows", "SKILL.md"))
	agents := readFile(t, filepath.Join(repo, ".agents", "skills", "mivia-agent-workflows", "SKILL.md"))
	claude := readFile(t, filepath.Join(repo, ".claude", "skills", "mivia-agent-workflows", "SKILL.md"))
	skillsJSON := readFile(t, filepath.Join(repo, ".agents", "skills.json"))

	for _, want := range []string{
		"name: mivia-agent-workflows",
		"triggers:",
		"./mivia-agent run --repo . --workflow <name> --dry-run --json",
		".ai/runs/<run-id>/<step-id>/iter-<nnn>/<artifact>",
		"crush-research-loop",
	} {
		if !strings.Contains(canonical, want) {
			t.Fatalf("canonical workflow skill = %q, want %q", canonical, want)
		}
	}
	if agents != canonical {
		t.Fatalf(".agents workflow skill differs from canonical .ai skill")
	}
	if !strings.Contains(claude, ".ai/skills/mivia-agent-workflows/SKILL.md") ||
		!strings.Contains(claude, "triggers:") ||
		!strings.Contains(claude, "discovery pointer") ||
		strings.Contains(claude, "./mivia-agent run --repo . --workflow <name>") {
		t.Fatalf("Claude workflow skill = %q, want concise canonical pointer", claude)
	}
	if !strings.Contains(skillsJSON, `"name": "mivia-agent-workflows"`) ||
		!strings.Contains(skillsJSON, `"path": ".ai/skills/mivia-agent-workflows/SKILL.md"`) ||
		!strings.Contains(skillsJSON, `"source": "project"`) {
		t.Fatalf("skills.json = %q, want mivia-agent-workflows project entry", skillsJSON)
	}
}

func TestInitWriteCreatesGitignoreWithAIRuns(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	repo := tempGitRepo(t)
	report, err := WriteInit(InitConfig{Repo: repo, Profile: "standard", Adapters: []string{"codex"}})
	if err != nil {
		t.Fatalf("WriteInit() error = %v, want nil", err)
	}
	got := readFile(t, filepath.Join(repo, ".gitignore"))
	if got != ".ai/runs/\n" {
		t.Fatalf(".gitignore = %q, want .ai/runs entry", got)
	}
	if !containsString(report.FilesCreated, ".gitignore") {
		t.Fatalf("FilesCreated = %#v, want .gitignore", report.FilesCreated)
	}
}

func TestInitWriteAppendsAIRunsToExistingGitignore(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	repo := tempGitRepo(t)
	writeFile(t, filepath.Join(repo, ".gitignore"), "dist/\n")
	report, err := WriteInit(InitConfig{Repo: repo, Profile: "standard", Adapters: []string{"codex"}})
	if err != nil {
		t.Fatalf("WriteInit() error = %v, want nil", err)
	}
	got := readFile(t, filepath.Join(repo, ".gitignore"))
	if got != "dist/\n.ai/runs/\n" {
		t.Fatalf(".gitignore = %q, want existing content plus .ai/runs entry", got)
	}
	if !containsString(report.FilesSkipped, ".gitignore") {
		t.Fatalf("FilesSkipped = %#v, want .gitignore update recorded", report.FilesSkipped)
	}
}

func TestInitWriteDoesNotDuplicateAIRunsGitignoreEntry(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	repo := tempGitRepo(t)
	writeFile(t, filepath.Join(repo, ".gitignore"), "dist/\n.ai/runs/\n")
	cfg := InitConfig{Repo: repo, Profile: "standard", Adapters: []string{"codex"}}
	if _, err := WriteInit(cfg); err != nil {
		t.Fatalf("first WriteInit() error = %v, want nil", err)
	}
	if _, err := WriteInit(cfg); err != nil {
		t.Fatalf("second WriteInit() error = %v, want nil", err)
	}
	got := readFile(t, filepath.Join(repo, ".gitignore"))
	if strings.Count(got, ".ai/runs/") != 1 {
		t.Fatalf(".gitignore = %q, want exactly one .ai/runs entry", got)
	}
}

func TestInitRendersWorkflowsForSingleRuntimeAdapter(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	repo := tempGitRepo(t)
	if _, err := WriteInit(InitConfig{Repo: repo, Profile: "standard", Adapters: []string{"codex", "crush"}}); err != nil {
		t.Fatalf("WriteInit() error = %v, want nil", err)
	}
	workflow := readFile(t, filepath.Join(repo, ".ai/workflows/research-loop.yaml"))
	if !strings.Contains(workflow, "producer: codex") || !strings.Contains(workflow, "min_reviewers: 1") {
		t.Fatalf("research workflow = %q, want codex routing with min reviewer 1", workflow)
	}
	if strings.Contains(workflow, "claude") || strings.Contains(workflow, "crush") {
		t.Fatalf("research workflow = %q, want no unavailable/guidance reviewers", workflow)
	}
}

func TestInitRendersRoutingDefaultsFromSelectedRuntimeAdapters(t *testing.T) {
	tests := []struct {
		name       string
		adapters   []string
		producer   string
		reviewers  []string
		minReviews int
	}{
		{name: "codex only", adapters: []string{"codex"}, producer: "codex", reviewers: []string{"codex"}, minReviews: 1},
		{name: "antigravity only", adapters: []string{"antigravity"}, producer: "antigravity", reviewers: []string{"antigravity"}, minReviews: 1},
		{name: "codex claude", adapters: []string{"codex", "claude"}, producer: "codex", reviewers: []string{"codex", "claude"}, minReviews: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			repo := tempGitRepo(t)
			if _, err := WriteInit(InitConfig{Repo: repo, Profile: "standard", Adapters: tt.adapters}); err != nil {
				t.Fatalf("WriteInit() error = %v, want nil", err)
			}
			manifest, err := config.Parse([]byte(readFile(t, filepath.Join(repo, "mivia-agent.yaml"))))
			if err != nil {
				t.Fatalf("Parse(rendered manifest) error = %v", err)
			}
			if manifest.Routing.DefaultProducer != tt.producer {
				t.Fatalf("default producer = %q, want %q", manifest.Routing.DefaultProducer, tt.producer)
			}
			if strings.Join(manifest.Routing.DefaultReviewers, ",") != strings.Join(tt.reviewers, ",") {
				t.Fatalf("default reviewers = %#v, want %#v", manifest.Routing.DefaultReviewers, tt.reviewers)
			}
			if manifest.Routing.Consensus.MinReviewers != tt.minReviews {
				t.Fatalf("min reviewers = %d, want %d", manifest.Routing.Consensus.MinReviewers, tt.minReviews)
			}
		})
	}
}

func TestInitWriteIsIdempotent(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	repo := tempGitRepo(t)
	cfg := InitConfig{Repo: repo, Profile: "standard", Adapters: []string{"codex", "claude", "copilot"}}
	if _, err := WriteInit(cfg); err != nil {
		t.Fatalf("first WriteInit() error = %v, want nil", err)
	}
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-m", "init")
	if _, err := WriteInit(cfg); err != nil {
		t.Fatalf("second WriteInit() error = %v, want nil", err)
	}
	cmd := exec.Command("git", "diff", "--exit-code")
	cmd.Dir = repo
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git diff --exit-code error = %v, output = %s", err, out)
	}
}

func TestInitDryRunAfterWriteSkipsGeneratedFiles(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	repo := tempGitRepo(t)
	cfg := InitConfig{Repo: repo, Profile: "standard", Adapters: []string{"codex", "claude", "copilot"}}
	if _, err := WriteInit(cfg); err != nil {
		t.Fatalf("WriteInit() error = %v, want nil", err)
	}
	report, err := PreviewInit(cfg)
	if err != nil {
		t.Fatalf("PreviewInit() error = %v, want nil", err)
	}
	if len(report.Conflicts) != 0 {
		t.Fatalf("PreviewInit() conflicts = %#v, want none for generated files", report.Conflicts)
	}
	if len(report.FilesSkipped) == 0 {
		t.Fatalf("PreviewInit() skipped = %#v, want existing generated files", report.FilesSkipped)
	}
}

func TestInitRefusesToOverwriteUserOwnedFile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	repo := tempGitRepo(t)
	path := filepath.Join(repo, "AGENTS.md")
	writeFile(t, path, "user-owned\n")
	_, err := WriteInit(InitConfig{Repo: repo, Profile: "standard", Adapters: []string{"codex"}})
	if err == nil {
		t.Fatal("WriteInit() error = nil, want conflict")
	}
	if got := readFile(t, path); got != "user-owned\n" {
		t.Fatalf("AGENTS.md = %q, want unchanged", got)
	}
}

func TestInitWritesSafeFilesWhenUserOwnedFilesConflict(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	repo := tempGitRepo(t)
	writeFile(t, filepath.Join(repo, "AGENTS.md"), "user-owned\n")
	report, err := WriteInit(InitConfig{Repo: repo, Profile: "standard", Adapters: []string{"codex", "crush"}})
	if err == nil || !strings.Contains(err.Error(), "init conflicts") {
		t.Fatalf("WriteInit() error = %v, want conflict", err)
	}
	if len(report.Conflicts) != 1 || report.Conflicts[0] != "AGENTS.md" {
		t.Fatalf("Conflicts = %#v, want AGENTS.md only", report.Conflicts)
	}
	if got := readFile(t, filepath.Join(repo, "AGENTS.md")); got != "user-owned\n" {
		t.Fatalf("AGENTS.md = %q, want unchanged", got)
	}
	for _, rel := range []string{"mivia-agent.yaml", ".codex/AGENTS.md", ".crush/README.md"} {
		if _, statErr := os.Stat(filepath.Join(repo, filepath.FromSlash(rel))); statErr != nil {
			t.Fatalf("Stat(%q) error = %v, want safe file written", rel, statErr)
		}
	}
}

func TestInitForceOverwritesUserOwnedFile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	repo := tempGitRepo(t)
	path := filepath.Join(repo, "AGENTS.md")
	writeFile(t, path, "user-owned\n")
	if _, err := WriteInit(InitConfig{Repo: repo, Profile: "standard", Adapters: []string{"codex"}, Force: true}); err != nil {
		t.Fatalf("WriteInit(force) error = %v, want nil", err)
	}
	if got := readFile(t, path); !strings.Contains(got, "mivia-agent:managed:start") {
		t.Fatalf("AGENTS.md = %q, want managed content", got)
	}
}

func TestInitRejectsUnknownAdapter(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	_, _, err := PlanInit(InitConfig{Repo: t.TempDir(), Adapters: []string{"unknown"}})
	if err == nil || !strings.Contains(err.Error(), "unknown adapter") {
		t.Fatalf("PlanInit() error = %v, want unknown adapter", err)
	}
}

func TestInitReportJSONShape(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	report, err := PreviewInit(InitConfig{Repo: tempGitRepo(t), Profile: "standard", Adapters: []string{"codex"}})
	if err != nil {
		t.Fatalf("PreviewInit() error = %v, want nil", err)
	}
	data, err := report.JSON()
	if err != nil {
		t.Fatalf("JSON() error = %v, want nil", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal(%s) error = %v", data, err)
	}
	for _, key := range []string{"files_created", "files_skipped", "conflicts"} {
		if _, ok := decoded[key]; !ok {
			t.Fatalf("JSON missing %q: %s", key, data)
		}
	}
}

func TestInitIncludesGlobalSkillsInSkillsJson(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeFile(t, filepath.Join(home, ".agents", "skills", "global-audit", "SKILL.md"), "global")
	repo := tempGitRepo(t)
	if _, err := WriteInit(InitConfig{Repo: repo, Profile: "standard", Adapters: []string{"codex"}}); err != nil {
		t.Fatalf("WriteInit() error = %v, want nil", err)
	}
	got := readFile(t, filepath.Join(repo, ".agents", "skills.json"))
	for _, want := range []string{"global-audit", `"source": "global"`, "~/.agents/skills/global-audit/SKILL.md"} {
		if !strings.Contains(got, want) {
			t.Fatalf("skills.json = %q, want %q", got, want)
		}
	}
}

func TestInitProjectSkillOverridesGlobalSkill(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeFile(t, filepath.Join(home, ".agents", "skills", "deep-bug-audit", "SKILL.md"), "global")
	repo := tempGitRepo(t)
	if _, err := WriteInit(InitConfig{Repo: repo, Profile: "standard", Adapters: []string{"codex"}}); err != nil {
		t.Fatalf("WriteInit() error = %v, want nil", err)
	}
	got := readFile(t, filepath.Join(repo, ".agents", "skills.json"))
	if strings.Count(got, "\"name\": \"deep-bug-audit\"") != 1 || strings.Contains(got, ".agents/skills/deep-bug-audit") {
		t.Fatalf("skills.json = %q, want project deep-bug-audit only", got)
	}
}

func TestInitGlobalConfigAbsentNoError(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if _, err := WriteInit(InitConfig{Repo: tempGitRepo(t), Profile: "standard", Adapters: []string{"codex"}}); err != nil {
		t.Fatalf("WriteInit() error = %v, want nil", err)
	}
}

func tempGitRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	runGit(t, repo, "init", "-q")
	runGit(t, repo, "config", "user.email", "test@example.invalid")
	runGit(t, repo, "config", "user.name", "Test User")
	runGit(t, repo, "commit", "-q", "--allow-empty", "-m", "init")
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

func repoFiles(t *testing.T, repo string) []string {
	t.Helper()
	var files []string
	if err := filepath.WalkDir(repo, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
		}
		if !d.IsDir() {
			rel, _ := filepath.Rel(repo, path)
			files = append(files, rel)
		}
		return nil
	}); err != nil {
		t.Fatalf("WalkDir() error = %v", err)
	}
	return files
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

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
