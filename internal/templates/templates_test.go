// Package templates verifies the embedded-template source skeleton.
// Plan: WS0. PRD: §1, §4, §9.
package templates

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTemplatesDirExists(t *testing.T) {
	path := repoPath(t, "templates", "README.md")
	if _, err := os.ReadFile(path); err != nil {
		t.Fatalf("ReadFile(%q) error = %v, want nil", path, err)
	}
}

func TestTemplatesSubdirsExist(t *testing.T) {
	for _, dir := range []string{
		"templates/core/rules",
		"templates/core/skills",
		"templates/core/quality/contracts",
		"templates/core/quality/review-policies",
		"templates/adapters/codex",
		"templates/adapters/claude",
		"templates/adapters/copilot",
		"templates/adapters/gemini",
		"templates/adapters/crush",
		"templates/workflows",
		"templates/prompts",
		"templates/ci/github-actions",
	} {
		dir := repoPath(t, filepath.FromSlash(dir))
		info, err := os.Stat(dir)
		if err != nil {
			t.Fatalf("Stat(%q) error = %v, want nil", dir, err)
		}
		if !info.IsDir() {
			t.Fatalf("Stat(%q).IsDir() = false, want true", dir)
		}
	}
}

func repoPath(t *testing.T, parts ...string) string {
	t.Helper()
	joined := filepath.Join(parts...)
	return filepath.Join("..", "..", joined)
}
