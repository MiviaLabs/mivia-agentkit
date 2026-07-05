// Package doctor validates installed mivia-agent control surfaces.
// Plan: WS3. PRD: FR-2.1, FR-10.5.
package doctor

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/MiviaLabs/mivia-agentkit/internal/config"
	"github.com/MiviaLabs/mivia-agentkit/internal/pathpolicy"
	"github.com/MiviaLabs/mivia-agentkit/internal/render"
	"github.com/MiviaLabs/mivia-agentkit/internal/report"
	"gopkg.in/yaml.v3"
)

type manifestResult struct {
	manifest config.Manifest
	raw      []byte
	err      error
}

func readManifest(repo string) manifestResult {
	data, err := os.ReadFile(filepath.Join(repo, "mivia-agent.yaml"))
	if err != nil {
		return manifestResult{err: err}
	}
	manifest, err := config.Parse(data)
	return manifestResult{manifest: manifest, raw: data, err: err}
}

func checkManifest(ctx *Context) []report.Finding {
	if ctx.manifest.err == nil {
		return nil
	}
	code := "manifest.invalid"
	if os.IsNotExist(ctx.manifest.err) {
		code = "manifest.missing"
	}
	return []report.Finding{finding(report.SeverityError, code, "mivia-agent.yaml", ctx.manifest.err.Error())}
}

func checkAIIndex(ctx *Context) []report.Finding {
	if exists(ctx.Repo, ".ai/INDEX.md") {
		return nil
	}
	return []report.Finding{finding(report.SeverityError, "ai.index_missing", ".ai/INDEX.md", "missing canonical index")}
}

func checkAdaptersPointToIndex(ctx *Context) []report.Finding {
	var findings []report.Finding
	required := map[string][]byte{
		"AGENTS.md":        []byte(".ai/"),
		".codex/AGENTS.md": []byte(".ai/INDEX.md"),
		"CLAUDE.md":        []byte(".ai/INDEX.md"),
	}
	for _, rel := range sortedKeys(required) {
		if !exists(ctx.Repo, rel) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(ctx.Repo, filepath.FromSlash(rel)))
		if err != nil || !bytes.Contains(data, required[rel]) {
			findings = append(findings, finding(report.SeverityError, "adapter.index_missing", rel, "adapter must reference the canonical .ai surface"))
		}
	}
	return findings
}

func checkAdapterFiles(ctx *Context) []report.Finding {
	if !ctx.manifestOK {
		return nil
	}
	required := map[string][]string{
		"codex":       {"AGENTS.md", ".codex/AGENTS.md", ".codex/hooks.json"},
		"claude":      {"CLAUDE.md", ".claude/settings.json"},
		"copilot":     {".github/copilot-instructions.md"},
		"antigravity": {"GEMINI.md"},
		"crush":       {".crush/README.md"},
	}
	var findings []report.Finding
	for adapter, cfg := range ctx.manifest.manifest.Adapters {
		if !cfg.Enabled {
			continue
		}
		for _, rel := range required[adapter] {
			if !exists(ctx.Repo, rel) {
				findings = append(findings, finding(report.SeverityError, "adapter.file_missing", rel, "enabled adapter file is missing"))
			}
		}
	}
	return findings
}

func checkHooksCallMiviaAgent(ctx *Context) []report.Finding {
	var findings []report.Finding
	for _, rel := range []string{".codex/hooks.json", ".claude/settings.json"} {
		if !exists(ctx.Repo, rel) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(ctx.Repo, filepath.FromSlash(rel)))
		if err != nil || !hookConfigInvokesMiviaAgent(data) {
			findings = append(findings, finding(report.SeverityError, "hooks.missing_mivia_agent", rel, "hook config must invoke mivia-agent hook"))
		}
	}
	return findings
}

func hookConfigInvokesMiviaAgent(data []byte) bool {
	return bytes.Contains(data, []byte("mivia-agent hook")) || bytes.Contains(data, []byte("scripts/run_agent_hook_guard.sh"))
}

func checkSkillsFrontmatter(ctx *Context) []report.Finding {
	var findings []report.Finding
	for _, root := range []string{".ai/skills", ".claude/skills"} {
		base := filepath.Join(ctx.Repo, filepath.FromSlash(root))
		_ = filepath.WalkDir(base, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || d.Name() != "SKILL.md" {
				return nil
			}
			rel := relSlash(ctx.Repo, path)
			data, err := os.ReadFile(path)
			if err != nil || !validSkillFrontmatter(data) {
				findings = append(findings, finding(report.SeverityError, "skills.invalid_frontmatter", rel, "SKILL.md frontmatter must include name and description"))
			}
			return nil
		})
	}
	return findings
}

func checkManagedMarkers(ctx *Context) []report.Finding {
	var findings []report.Finding
	_ = filepath.WalkDir(ctx.Repo, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || strings.Contains(path, string(filepath.Separator)+".git"+string(filepath.Separator)) {
			return nil
		}
		rel := relSlash(ctx.Repo, path)
		if !isManagedMarkerPath(rel) {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		starts := bytes.Count(data, []byte("mivia-agent:managed:start"))
		ends := bytes.Count(data, []byte("mivia-agent:managed:end"))
		if starts != ends || (starts > 0 && !render.HasManaged(data)) {
			findings = append(findings, finding(report.SeverityError, "generated.markers_invalid", rel, "managed block markers must be balanced"))
		}
		return nil
	})
	return findings
}

func isManagedMarkerPath(rel string) bool {
	rel = filepath.ToSlash(rel)
	if isAgentControlPath(rel) {
		return true
	}
	return strings.HasPrefix(rel, ".github/workflows/")
}

func checkCICallsDoctor(ctx *Context) []report.Finding {
	rel := ".github/workflows/agent-control.yml"
	data, err := os.ReadFile(filepath.Join(ctx.Repo, filepath.FromSlash(rel)))
	if os.IsNotExist(err) {
		return []report.Finding{finding(report.SeverityWarn, "ci.missing_doctor_json", rel, "agent control workflow should run mivia-agent doctor --json")}
	}
	if err != nil || !ciCallsDoctorJSON(data) {
		return []report.Finding{finding(report.SeverityWarn, "ci.missing_doctor_json", rel, "agent control workflow should run mivia-agent doctor --json")}
	}
	return nil
}

func ciCallsDoctorJSON(data []byte) bool {
	return bytes.Contains(data, []byte("mivia-agent doctor")) && bytes.Contains(data, []byte("--json"))
}

func checkGeneratedArtifactsStaged(ctx *Context) []report.Finding {
	cmd := exec.Command("git", "-C", ctx.Repo, "status", "--porcelain=v1", "-z", "--", ".ai/runs")
	out, err := cmd.Output()
	if err != nil || len(out) == 0 {
		return nil
	}
	return []report.Finding{finding(report.SeverityError, "generated.artifacts_staged", ".ai/runs", "runtime artifacts must not be staged")}
}

func checkSecretPaths(ctx *Context) []report.Finding {
	policy := pathpolicy.NewDefault()
	ignored := gitIgnored(ctx.Repo)
	var findings []report.Finding
	_ = filepath.WalkDir(ctx.Repo, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel := relSlash(ctx.Repo, path)
		if strings.HasPrefix(rel, ".git/") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if isIgnoredPath(ignored, rel) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !isAgentControlPath(rel) {
			if d.IsDir() && isKnownLargeAppDir(rel) {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if err := policy.Check(ctx.Repo, rel); err != nil {
			findings = append(findings, finding(report.SeverityError, "paths.secret_generated", rel, err.Error()))
		}
		return nil
	})
	return findings
}

func isAgentControlPath(rel string) bool {
	rel = filepath.ToSlash(rel)
	switch rel {
	case "mivia-agent.yaml", "AGENTS.md", "CLAUDE.md", "GEMINI.md":
		return true
	}
	for _, prefix := range []string{".ai/", ".agents/", ".codex/", ".claude/", ".crush/", ".github/copilot-instructions.md", ".github/instructions/"} {
		if strings.HasPrefix(rel, prefix) {
			return true
		}
	}
	return false
}

func isKnownLargeAppDir(rel string) bool {
	switch filepath.Base(filepath.ToSlash(rel)) {
	case "node_modules", "vendor":
		return true
	default:
		return false
	}
}

func gitIgnored(repo string) map[string]struct{} {
	cmd := exec.Command("git", "-C", repo, "status", "--ignored", "--porcelain=v1", "-z")
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	ignored := map[string]struct{}{}
	for _, item := range bytes.Split(out, []byte{0}) {
		if len(item) < 4 || !bytes.HasPrefix(item, []byte("!! ")) {
			continue
		}
		rel := filepath.ToSlash(string(item[3:]))
		ignored[rel] = struct{}{}
	}
	return ignored
}

func isIgnoredPath(ignored map[string]struct{}, rel string) bool {
	if len(ignored) == 0 {
		return false
	}
	rel = filepath.ToSlash(rel)
	if _, ok := ignored[rel]; ok {
		return true
	}
	prefix := strings.TrimSuffix(rel, "/") + "/"
	for ignoredPath := range ignored {
		if strings.HasPrefix(ignoredPath, prefix) {
			return true
		}
	}
	return false
}

func checkGlobalReadable(ctx *Context) []report.Finding {
	if ctx.GlobalDir == "" {
		return nil
	}
	if _, err := os.Stat(ctx.GlobalDir); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return []report.Finding{finding(report.SeverityWarn, "global.unreadable", ctx.GlobalDir, err.Error())}
	}
	path := filepath.Join(ctx.GlobalDir, "mivia-agent.yaml")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		path = filepath.Join(ctx.GlobalDir, "mivia.yaml")
	}
	if data, err := os.ReadFile(path); err == nil {
		var parsed map[string]any
		if err := yaml.Unmarshal(data, &parsed); err != nil {
			return []report.Finding{finding(report.SeverityWarn, "global.parse_error", path, err.Error())}
		}
	}
	return nil
}

func checkGlobalRuleConflict(ctx *Context) []report.Finding {
	if !ctx.Strict {
		return nil
	}
	globalRules := filepath.Join(ctx.GlobalDir, "rules")
	projectRules := filepath.Join(ctx.Repo, ".ai", "rules")
	entries, err := os.ReadDir(globalRules)
	if err != nil {
		return nil
	}
	var findings []report.Finding
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			continue
		}
		globalData, err := os.ReadFile(filepath.Join(globalRules, entry.Name()))
		if err != nil {
			continue
		}
		projectPath := filepath.Join(projectRules, entry.Name())
		projectData, err := os.ReadFile(projectPath)
		if err == nil && !bytes.Equal(globalData, projectData) {
			findings = append(findings, finding(report.SeverityWarn, "global.rule_conflict", relSlash(ctx.Repo, projectPath), "project rule overrides divergent global rule"))
		}
	}
	return findings
}

func validSkillFrontmatter(data []byte) bool {
	if !bytes.HasPrefix(data, []byte("---\n")) {
		return false
	}
	end := bytes.Index(data[4:], []byte("\n---"))
	if end < 0 {
		return false
	}
	front := data[4 : 4+end]
	var parsed struct {
		Name        string `yaml:"name"`
		Description string `yaml:"description"`
	}
	if err := yaml.Unmarshal(front, &parsed); err != nil {
		return false
	}
	return parsed.Name != "" && parsed.Description != ""
}

func exists(repo, rel string) bool {
	_, err := os.Stat(filepath.Join(repo, filepath.FromSlash(rel)))
	return err == nil
}

func relSlash(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}

func sortedKeys[V any](in map[string]V) []string {
	keys := make([]string, 0, len(in))
	for key := range in {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
