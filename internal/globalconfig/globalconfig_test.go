package globalconfig

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MiviaLabs/mivia-agentkit/internal/config"
)

func TestGlobalConfigReadAbsentReturnsZeroNoError(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	got, err := Read()
	if err != nil {
		t.Fatalf("Read() error = %v, want nil", err)
	}
	if got.Defaults.Profile != "" || len(got.Rules) != 0 || len(got.Skills) != 0 {
		t.Fatalf("Read() = %+v, want zero config", got)
	}
}

func TestGlobalConfigReadParsesMiviaYaml(t *testing.T) {
	home := setupHome(t)
	writeHomeFile(t, home, ".agents/mivia.yaml", "defaults:\n  profile: starter\n  governance:\n    provider: noop\n")
	got, err := Read()
	if err != nil {
		t.Fatalf("Read() error = %v, want nil", err)
	}
	if got.Defaults.Profile != "starter" {
		t.Fatalf("Defaults.Profile = %q, want starter", got.Defaults.Profile)
	}
}

func TestGlobalConfigReadParsesRulesDir(t *testing.T) {
	home := setupHome(t)
	writeHomeFile(t, home, ".agents/rules/quality.md", "quality\n")
	got, err := Read()
	if err != nil {
		t.Fatalf("Read() error = %v, want nil", err)
	}
	if got.Rules["quality.md"] != "quality\n" {
		t.Fatalf("Rules = %#v, want quality.md", got.Rules)
	}
}

func TestGlobalConfigReadParsesSkillsDir(t *testing.T) {
	home := setupHome(t)
	writeHomeFile(t, home, ".agents/skills/audit/SKILL.md", "skill\n")
	got, err := Read()
	if err != nil {
		t.Fatalf("Read() error = %v, want nil", err)
	}
	if got.Skills["audit"] != "skill\n" {
		t.Fatalf("Skills = %#v, want audit", got.Skills)
	}
}

func TestGlobalConfigLayerProjectWinsOnConflict(t *testing.T) {
	global := GlobalConfig{Defaults: config.Manifest{Profile: "starter", Governance: config.Governance{Provider: "agt"}}}
	project := config.Manifest{Profile: "standard", Governance: config.Governance{Provider: "noop"}}
	got := Layer(global, project)
	if got.Manifest.Profile != "standard" || got.Manifest.Governance.Provider != "noop" {
		t.Fatalf("Layer() manifest = %+v, want project values", got.Manifest)
	}
}

func TestGlobalConfigLayerFillsEmptyProjectFields(t *testing.T) {
	global := GlobalConfig{Defaults: config.Manifest{Profile: "starter", Adapters: map[string]config.AdapterConfig{"codex": {Enabled: true, Role: config.AdapterRoleOrchestrable}}}}
	got := Layer(global, config.Manifest{})
	if got.Manifest.Profile != "starter" {
		t.Fatalf("Profile = %q, want starter", got.Manifest.Profile)
	}
	if !got.Manifest.Adapters["codex"].Enabled {
		t.Fatalf("Adapters = %+v, want global codex default", got.Manifest.Adapters)
	}
}

func TestGlobalConfigRejectsSymlinkEscape(t *testing.T) {
	home := setupHome(t)
	mkdirPath(t, filepath.Join(home, ".agents", "rules"))
	outside := filepath.Join(t.TempDir(), "outside.md")
	writePath(t, outside, "outside\n")
	if err := os.Symlink(outside, filepath.Join(home, ".agents", "rules", "escape.md")); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}
	if _, err := Read(); err == nil || !strings.Contains(err.Error(), "escapes") {
		t.Fatalf("Read() error = %v, want symlink escape", err)
	}
}

func TestGlobalConfigRejectsUnknownYAMLField(t *testing.T) {
	home := setupHome(t)
	writeHomeFile(t, home, ".agents/mivia.yaml", "defaults:\n  profile: starter\nmystery: true\n")
	if _, err := Read(); err == nil || !strings.Contains(err.Error(), "field mystery not found") {
		t.Fatalf("Read() error = %v, want unknown field", err)
	}
}

func setupHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	return home
}

func writeHomeFile(t *testing.T, home, rel, content string) {
	t.Helper()
	path := filepath.Join(home, filepath.FromSlash(rel))
	mkdirPath(t, filepath.Dir(path))
	writePath(t, path, content)
}

func mkdirPath(t *testing.T, path string) {
	t.Helper()
	if err := os.Mkdir(path, 0o755); err == nil || os.IsExist(err) {
		return
	}
	parent := filepath.Dir(path)
	if parent != path {
		mkdirPath(t, parent)
	}
	if err := os.Mkdir(path, 0o755); err != nil && !os.IsExist(err) {
		t.Fatalf("Mkdir(%q) error = %v", path, err)
	}
}

func writePath(t *testing.T, path, content string) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("Create(%q) error = %v", path, err)
	}
	defer file.Close()
	if _, err := io.WriteString(file, content); err != nil {
		t.Fatalf("WriteString(%q) error = %v", path, err)
	}
}
