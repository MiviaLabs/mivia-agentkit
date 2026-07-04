// Package importer inspects existing agent-control files for migration.
// Plan: WS7. PRD: FR-9.1, FR-9.2.
package importer

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Finding describes one detected source artifact.
type Finding struct {
	Source   string `json:"source"`
	Kind     string `json:"kind"`
	Path     string `json:"path"`
	Reusable bool   `json:"reusable"`
}

// Inspect scans repo for existing agent-control files without writing.
func Inspect(repo string) ([]Finding, error) {
	repo = absRepo(repo)
	var findings []Finding
	for _, rel := range []string{
		"AGENTS.md",
		"CLAUDE.md",
		"GEMINI.md",
		".github/copilot-instructions.md",
		".codex/AGENTS.md",
		".codex/hooks.json",
		".claude/settings.json",
	} {
		if exists(repo, rel) {
			findings = append(findings, classify(rel))
		}
	}
	if err := walkFindings(repo, &findings); err != nil {
		return nil, err
	}
	sort.Slice(findings, func(i, j int) bool {
		if findings[i].Source != findings[j].Source {
			return findings[i].Source < findings[j].Source
		}
		if findings[i].Kind != findings[j].Kind {
			return findings[i].Kind < findings[j].Kind
		}
		return findings[i].Path < findings[j].Path
	})
	return dedup(findings), nil
}

func walkFindings(repo string, findings *[]Finding) error {
	return filepath.WalkDir(repo, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(repo, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == "." || strings.HasPrefix(rel, ".git/") {
			if d.IsDir() && rel == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			if rel == ".dagger" {
				*findings = append(*findings, Finding{Source: rel, Kind: "loop", Path: rel, Reusable: false})
				return filepath.SkipDir
			}
			return nil
		}
		switch {
		case strings.HasPrefix(rel, ".agents/skills/") && strings.HasSuffix(rel, "/SKILL.md"):
			*findings = append(*findings, Finding{Source: rel, Kind: "skill", Path: rel, Reusable: true})
		case strings.HasPrefix(rel, ".github/instructions/"):
			*findings = append(*findings, Finding{Source: rel, Kind: "instruction", Path: rel, Reusable: true})
		case strings.HasPrefix(rel, ".github/agents/"):
			*findings = append(*findings, Finding{Source: rel, Kind: "skill", Path: rel, Reusable: true})
		case strings.HasPrefix(rel, ".claude/agents/"):
			*findings = append(*findings, Finding{Source: rel, Kind: "loop", Path: rel, Reusable: false})
		case strings.HasPrefix(rel, ".github/workflows/") && strings.Contains(strings.ToLower(filepath.Base(rel)), "agent") && hasYAMLExt(rel):
			*findings = append(*findings, Finding{Source: rel, Kind: "loop", Path: rel, Reusable: false})
		case strings.HasSuffix(rel, ".loop.yaml") || filepath.Base(rel) == "workflow.yaml":
			*findings = append(*findings, Finding{Source: rel, Kind: "loop", Path: rel, Reusable: true})
		}
		return nil
	})
}

func classify(rel string) Finding {
	switch rel {
	case "AGENTS.md":
		return Finding{Source: rel, Kind: "rules", Path: rel, Reusable: true}
	case ".codex/hooks.json", ".claude/settings.json":
		return Finding{Source: rel, Kind: "hook", Path: rel, Reusable: false}
	default:
		return Finding{Source: rel, Kind: "instruction", Path: rel, Reusable: true}
	}
}

func dedup(findings []Finding) []Finding {
	seen := map[string]struct{}{}
	out := make([]Finding, 0, len(findings))
	for _, finding := range findings {
		key := strings.Join([]string{finding.Source, finding.Kind, finding.Path}, "\x00")
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, finding)
	}
	return out
}

func hasYAMLExt(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".yml", ".yaml":
		return true
	default:
		return false
	}
}

func exists(repo, rel string) bool {
	_, err := os.Stat(filepath.Join(repo, filepath.FromSlash(rel)))
	return err == nil
}

func absRepo(repo string) string {
	if repo == "" {
		repo, _ = os.Getwd()
	}
	abs, err := filepath.Abs(repo)
	if err != nil {
		return repo
	}
	return abs
}
