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
