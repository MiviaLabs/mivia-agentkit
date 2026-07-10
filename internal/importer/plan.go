// Package importer inspects existing agent-control files for migration.
// Plan: WS7. PRD: FR-9.1, FR-9.2.
package importer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/MiviaLabs/mivia-agentkit/internal/config"
	"github.com/MiviaLabs/mivia-agentkit/internal/pathpolicy"
	"github.com/MiviaLabs/mivia-agentkit/internal/render"
	"github.com/MiviaLabs/mivia-agentkit/internal/templates"
	"github.com/MiviaLabs/mivia-agentkit/internal/version"
)

// Action is one planned write.
type Action struct {
	Source string `json:"source"`
	Kind   string `json:"kind"`
	Path   string `json:"path"`
}

// Conflict is one manual migration or overwrite conflict.
type Conflict struct {
	Source string `json:"source"`
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

// Plan is the read-only migration plan.
type Plan struct {
	Actions   []Action   `json:"actions"`
	Conflicts []Conflict `json:"conflicts"`

	manifest config.Manifest
}

// Report is the apply result.
type Report struct {
	Written   []string   `json:"written"`
	Skipped   []string   `json:"skipped"`
	Conflicts []Conflict `json:"conflicts"`
}

// BuildPlan maps existing artifacts into a migration plan.
func BuildPlan(repo string, manifest config.Manifest) (Plan, error) {
	repo = absRepo(repo)
	findings, err := Inspect(repo)
	if err != nil {
		return Plan{}, err
	}
	if len(manifest.Adapters) == 0 {
		manifest = config.Defaults()
	}
	plan := Plan{manifest: manifest}
	initPlan, _, err := bootstrapFiles(repo, manifest)
	if err != nil {
		return Plan{}, err
	}
	for _, item := range initPlan {
		plan.Actions = append(plan.Actions, Action{Source: "template", Kind: "bootstrap", Path: item.OutPath})
	}
	policy := pathpolicy.NewDefault()
	for _, finding := range findings {
		if !finding.Reusable {
			plan.Conflicts = append(plan.Conflicts, Conflict{Source: finding.Source, Path: finding.Path, Reason: "requires manual migration"})
			continue
		}
		target := targetPath(finding)
		if err := policy.Check(repo, target); err != nil {
			plan.Conflicts = append(plan.Conflicts, Conflict{Source: finding.Source, Path: target, Reason: err.Error()})
			continue
		}
		content, err := mappedContent(repo, finding)
		if err != nil {
			return Plan{}, err
		}
		if existing, err := os.ReadFile(filepath.Join(repo, filepath.FromSlash(target))); err == nil && !bytes.Equal(existing, content) {
			plan.Conflicts = append(plan.Conflicts, Conflict{Source: finding.Source, Path: target, Reason: "existing mapped file differs"})
			continue
		}
		plan.Actions = append(plan.Actions, Action{Source: finding.Source, Kind: finding.Kind, Path: target})
	}
	sort.Slice(plan.Actions, func(i, j int) bool {
		if plan.Actions[i].Path != plan.Actions[j].Path {
			return plan.Actions[i].Path < plan.Actions[j].Path
		}
		return plan.Actions[i].Source < plan.Actions[j].Source
	})
	sort.Slice(plan.Conflicts, func(i, j int) bool {
		if plan.Conflicts[i].Path != plan.Conflicts[j].Path {
			return plan.Conflicts[i].Path < plan.Conflicts[j].Path
		}
		return plan.Conflicts[i].Source < plan.Conflicts[j].Source
	})
	return plan, nil
}

// Apply writes the planned .ai files without touching source files.
func (p Plan) Apply(repo string, force bool) (Report, error) {
	repo = absRepo(repo)
	report := Report{Conflicts: append([]Conflict(nil), p.Conflicts...)}
	if len(p.Conflicts) > 0 && !force {
		return report, fmt.Errorf("import conflicts require --force")
	}
	initPlan, rendered, err := bootstrapFiles(repo, p.manifest)
	if err != nil {
		return report, err
	}
	for _, item := range initPlan {
		target := filepath.Join(repo, filepath.FromSlash(item.OutPath))
		data := rendered[item.OutPath]
		if existing, err := os.ReadFile(target); err == nil {
			switch {
			case bytes.Equal(existing, data):
				report.Skipped = append(report.Skipped, item.OutPath)
				continue
			case render.HasManaged(existing) || force:
				if render.HasManaged(existing) {
					_, managed, _, _ := render.ExtractManaged(data)
					data, err = render.ReplaceManaged(existing, managed)
					if err != nil {
						return report, err
					}
				}
			case canAppendManagedPointer(item.OutPath):
				appended := appendManagedPointer(existing, data)
				if bytes.Equal(existing, appended) {
					report.Skipped = append(report.Skipped, item.OutPath)
					continue
				}
				data = appended
			default:
				report.Skipped = append(report.Skipped, item.OutPath)
				continue
			}
		} else if !os.IsNotExist(err) {
			return report, err
		}
		if err := pathpolicy.WriteFile(repo, item.OutPath, data, 0o644); err != nil {
			return report, err
		}
		report.Written = append(report.Written, item.OutPath)
	}
	for _, action := range p.Actions {
		if action.Kind == "bootstrap" {
			continue
		}
		finding := Finding{Source: action.Source, Kind: action.Kind, Path: action.Source, Reusable: true}
		data, err := mappedContent(repo, finding)
		if err != nil {
			return report, err
		}
		target := filepath.Join(repo, filepath.FromSlash(action.Path))
		if existing, err := os.ReadFile(target); err == nil {
			if bytes.Equal(existing, data) {
				report.Skipped = append(report.Skipped, action.Path)
				continue
			}
			if !force {
				report.Conflicts = append(report.Conflicts, Conflict{Source: action.Source, Path: action.Path, Reason: "existing mapped file differs"})
				continue
			}
		} else if !os.IsNotExist(err) {
			return report, err
		}
		if err := pathpolicy.WriteFile(repo, action.Path, data, 0o644); err != nil {
			return report, err
		}
		report.Written = append(report.Written, action.Path)
	}
	sort.Strings(report.Written)
	sort.Strings(report.Skipped)
	sort.Slice(report.Conflicts, func(i, j int) bool { return report.Conflicts[i].Path < report.Conflicts[j].Path })
	if len(report.Conflicts) > 0 && !force {
		return report, fmt.Errorf("import conflicts require --force")
	}
	return report, nil
}

// JSON returns deterministic JSON.
func (p Plan) JSON() ([]byte, error) {
	return json.MarshalIndent(p, "", "  ")
}

// JSON returns deterministic JSON.
func (r Report) JSON() ([]byte, error) {
	return json.MarshalIndent(r, "", "  ")
}

func enabledAdapters(manifest config.Manifest) []string {
	var adapters []string
	for name, cfg := range manifest.Adapters {
		if cfg.Enabled {
			adapters = append(adapters, name)
		}
	}
	sort.Strings(adapters)
	return adapters
}

func bootstrapFiles(repo string, manifest config.Manifest) (render.RenderPlan, map[string][]byte, error) {
	if manifest.Profile == "" {
		manifest.Profile = config.Defaults().Profile
	}
	adapters := enabledAdapters(manifest)
	outPaths, err := templates.List(manifest.Profile, adapters)
	if err != nil {
		return nil, nil, err
	}
	plan := make(render.RenderPlan, 0, len(outPaths))
	for _, outPath := range outPaths {
		tpl, ok := templates.TemplateForOutput(outPath)
		if !ok {
			return nil, nil, fmt.Errorf("no template for %s", outPath)
		}
		plan = append(plan, render.RenderItem{Template: tpl, OutPath: outPath})
	}
	rendered, err := render.New().RenderAll(plan, render.Vars{
		Project:  render.ProjectVars{Name: filepath.Base(repo)},
		Profile:  manifest.Profile,
		Adapters: adapterMap(adapters),
		Binary:   "mivia-agent",
		Version:  version.Version,
		Skills: []render.SkillEntry{
			{Name: "adversarial-test-review", Path: ".ai/skills/adversarial-test-review/SKILL.md", Source: "project"},
			{Name: "airtight-feature-delivery", Path: ".ai/skills/airtight-feature-delivery/SKILL.md", Source: "project"},
			{Name: "deep-bug-audit", Path: ".ai/skills/deep-bug-audit/SKILL.md", Source: "project"},
			{Name: "test-coverage-audit", Path: ".ai/skills/test-coverage-audit/SKILL.md", Source: "project"},
		},
	})
	if err != nil {
		return nil, nil, err
	}
	return plan, rendered, nil
}

func adapterMap(adapters []string) map[string]bool {
	out := map[string]bool{}
	for _, adapter := range adapters {
		out[adapter] = true
	}
	return out
}

func targetPath(f Finding) string {
	base := filepath.Base(f.Path)
	switch f.Kind {
	case "rules":
		return ".ai/imported/rules/" + base
	case "skill":
		parts := strings.Split(filepath.ToSlash(f.Path), "/")
		if len(parts) >= 2 {
			return ".ai/imported/skills/" + strings.Join(parts[len(parts)-2:], "/")
		}
	case "loop":
		return ".ai/workflows/imported-" + base
	}
	return ".ai/imported/instructions/" + strings.ReplaceAll(filepath.ToSlash(f.Path), "/", "_")
}

func mappedContent(repo string, f Finding) ([]byte, error) {
	data, err := os.ReadFile(filepath.Join(repo, filepath.FromSlash(f.Source)))
	if err != nil {
		return nil, err
	}
	if f.Kind == "loop" && hasYAMLExt(f.Path) {
		return wrapManaged("#", f.Source, data), nil
	}
	return wrapManaged("<!--", f.Source, data), nil
}

func wrapManaged(prefix, source string, body []byte) []byte {
	if prefix == "#" {
		return []byte("# mivia-agent:managed:start\n# imported from " + source + "\n" + strings.TrimLeft(string(body), "\n") + "\n# mivia-agent:managed:end\n")
	}
	return []byte("<!-- mivia-agent:managed:start -->\n<!-- imported from " + source + " -->\n" + strings.TrimLeft(string(body), "\n") + "\n<!-- mivia-agent:managed:end -->\n")
}

func canAppendManagedPointer(path string) bool {
	switch path {
	case "AGENTS.md", "CLAUDE.md", "GEMINI.md", ".codex/AGENTS.md", ".github/copilot-instructions.md":
		return true
	default:
		return false
	}
}

func appendManagedPointer(existing, desired []byte) []byte {
	if bytes.Contains(existing, []byte(".ai/INDEX.md")) || bytes.Contains(existing, []byte(".ai/")) {
		return existing
	}
	pre, managed, post, ok := render.ExtractManaged(desired)
	if ok {
		block := append([]byte{}, pre...)
		block = append(block, []byte("<!-- mivia-agent:managed:start -->")...)
		block = append(block, managed...)
		block = append(block, []byte("<!-- mivia-agent:managed:end -->")...)
		block = append(block, post...)
		desired = block
	}
	trimmed := bytes.TrimRight(existing, "\n")
	return append(append(trimmed, []byte("\n\n")...), desired...)
}
