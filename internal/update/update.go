// Package update refreshes managed template blocks in place.
// Plan: WS7. PRD: FR-1.4.
package update

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/MiviaLabs/mivia-agentkit/internal/config"
	"github.com/MiviaLabs/mivia-agentkit/internal/render"
	"github.com/MiviaLabs/mivia-agentkit/internal/templates"
	"github.com/MiviaLabs/mivia-agentkit/internal/version"
)

// Change is one file that would change on update.
type Change struct {
	Path string `json:"path"`
	Kind string `json:"kind"`
}

// Conflict is one skipped update.
type Conflict struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

// Report is the update result.
type Report struct {
	Updated   []string   `json:"updated"`
	Conflicts []Conflict `json:"conflicts"`
}

// Diff lists managed files that differ from the embedded templates.
func Diff(repo string) ([]Change, error) {
	repo = absRepo(repo)
	manifest, err := loadManifest(repo)
	if err != nil {
		return nil, err
	}
	rendered, err := desiredFiles(repo, manifest)
	if err != nil {
		return nil, err
	}
	var changes []Change
	for rel, want := range rendered {
		path := filepath.Join(repo, filepath.FromSlash(rel))
		have, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			changes = append(changes, Change{Path: rel, Kind: "missing"})
			continue
		}
		if err != nil {
			return nil, err
		}
		if bytes.Equal(have, want) {
			continue
		}
		if rel == "mivia-agent.yaml" {
			changes = append(changes, Change{Path: rel, Kind: "manifest"})
			continue
		}
		if render.HasManaged(have) {
			changes = append(changes, Change{Path: rel, Kind: "managed-block"})
			continue
		}
		changes = append(changes, Change{Path: rel, Kind: "replace"})
	}
	sort.Slice(changes, func(i, j int) bool { return changes[i].Path < changes[j].Path })
	return changes, nil
}

// Apply refreshes managed blocks and preserves user content outside those blocks.
func Apply(repo string, force bool) (Report, error) {
	repo = absRepo(repo)
	manifest, err := loadManifest(repo)
	if err != nil {
		return Report{}, err
	}
	rendered, err := desiredFiles(repo, manifest)
	if err != nil {
		return Report{}, err
	}
	report := Report{}
	for rel, want := range rendered {
		path := filepath.Join(repo, filepath.FromSlash(rel))
		have, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return report, err
			}
			if err := os.WriteFile(path, want, 0o644); err != nil {
				return report, err
			}
			report.Updated = append(report.Updated, rel)
			continue
		}
		if err != nil {
			return report, err
		}
		if bytes.Equal(have, want) {
			continue
		}
		if rel == "mivia-agent.yaml" {
			if manifest.TemplateVersion == version.Version && !force {
				report.Conflicts = append(report.Conflicts, Conflict{Path: rel, Reason: "manifest was edited locally"})
				continue
			}
			if err := os.WriteFile(path, want, 0o644); err != nil {
				return report, err
			}
			report.Updated = append(report.Updated, rel)
			continue
		}
		if !render.HasManaged(have) {
			report.Conflicts = append(report.Conflicts, Conflict{Path: rel, Reason: "file is not managed"})
			continue
		}
		_, oldManaged, _, _ := render.ExtractManaged(have)
		_, newManaged, _, _ := render.ExtractManaged(want)
		if bytes.Equal(oldManaged, newManaged) {
			continue
		}
		if manifest.TemplateVersion == version.Version && !force {
			report.Conflicts = append(report.Conflicts, Conflict{Path: rel, Reason: "managed block was edited locally"})
			continue
		}
		next, err := render.ReplaceManaged(have, newManaged)
		if err != nil {
			return report, err
		}
		if err := os.WriteFile(path, next, 0o644); err != nil {
			return report, err
		}
		report.Updated = append(report.Updated, rel)
	}
	sort.Strings(report.Updated)
	sort.Slice(report.Conflicts, func(i, j int) bool { return report.Conflicts[i].Path < report.Conflicts[j].Path })
	if len(report.Conflicts) > 0 && !force {
		return report, fmt.Errorf("update conflicts require --force")
	}
	return report, nil
}

// JSON returns deterministic JSON.
func (r Report) JSON() ([]byte, error) {
	return json.MarshalIndent(r, "", "  ")
}

func desiredFiles(repo string, manifest config.Manifest) (map[string][]byte, error) {
	if manifest.Profile == "" {
		manifest.Profile = config.Defaults().Profile
	}
	outPaths, err := templates.List(manifest.Profile, enabledAdapters(manifest))
	if err != nil {
		return nil, err
	}
	plan := make(render.RenderPlan, 0, len(outPaths))
	for _, outPath := range outPaths {
		tpl, ok := templates.TemplateForOutput(outPath)
		if !ok {
			return nil, fmt.Errorf("no template for %s", outPath)
		}
		plan = append(plan, render.RenderItem{Template: tpl, OutPath: outPath})
	}
	return render.New().RenderAll(plan, render.Vars{
		Project:  render.ProjectVars{Name: filepath.Base(repo)},
		Profile:  manifest.Profile,
		Adapters: adapterMap(enabledAdapters(manifest)),
		Binary:   "mivia-agent",
		Version:  version.Version,
		Skills: []render.SkillEntry{
			{Name: "adversarial-test-review", Path: ".ai/skills/adversarial-test-review/SKILL.md", Source: "project"},
			{Name: "airtight-feature-delivery", Path: ".ai/skills/airtight-feature-delivery/SKILL.md", Source: "project"},
			{Name: "deep-bug-audit", Path: ".ai/skills/deep-bug-audit/SKILL.md", Source: "project"},
			{Name: "test-coverage-audit", Path: ".ai/skills/test-coverage-audit/SKILL.md", Source: "project"},
		},
	})
}

func loadManifest(repo string) (config.Manifest, error) {
	data, err := os.ReadFile(filepath.Join(repo, "mivia-agent.yaml"))
	if os.IsNotExist(err) {
		return config.Defaults(), nil
	}
	if err != nil {
		return config.Manifest{}, err
	}
	return config.Parse(data)
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

func adapterMap(adapters []string) map[string]bool {
	out := map[string]bool{}
	for _, adapter := range adapters {
		out[adapter] = true
	}
	return out
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
