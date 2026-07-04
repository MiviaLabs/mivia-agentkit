// Package render renders embedded templates into target repo files.
// Plan: WS2. PRD: FR-1.1, FR-1.2, FR-10.6.
package render

import (
	"bytes"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"text/template"

	"github.com/MiviaLabs/mivia-agentkit/internal/templates"
	"github.com/MiviaLabs/mivia-agentkit/internal/version"
)

// Vars is the stable variable set available to templates.
type Vars struct {
	Project  ProjectVars
	Profile  string
	Adapters map[string]bool
	Binary   string
	Version  string
	Skills   []SkillEntry
}

// ProjectVars describes the target project.
type ProjectVars struct {
	Name string
}

// SkillEntry is one entry in .agents/skills.json.
type SkillEntry struct {
	Name   string
	Path   string
	Source string
}

// Renderer renders embedded templates.
type Renderer struct {
	fsys fs.FS
}

// RenderItem maps one template to one output path.
type RenderItem struct {
	Template string
	OutPath  string
}

// RenderPlan is an ordered list of render items.
type RenderPlan []RenderItem

// New constructs a renderer backed by embedded templates.
func New() Renderer {
	return Renderer{fsys: templates.FS()}
}

// Render renders one template.
func (r Renderer) Render(tplName string, vars Vars) ([]byte, error) {
	if vars.Binary == "" {
		vars.Binary = "mivia-agent"
	}
	if vars.Version == "" {
		vars.Version = version.Version
	}
	tpl, err := template.New(filepath.Base(tplName)).Option("missingkey=error").Funcs(funcs()).ParseFS(r.fsys, tplName)
	if err != nil {
		return nil, fmt.Errorf("parse template %s: %w", tplName, err)
	}
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, templateData(vars)); err != nil {
		return nil, fmt.Errorf("render template %s: %w", tplName, err)
	}
	return buf.Bytes(), nil
}

func templateData(vars Vars) map[string]any {
	return map[string]any{
		"Project":  map[string]any{"Name": vars.Project.Name},
		"Profile":  vars.Profile,
		"Adapters": vars.Adapters,
		"Binary":   vars.Binary,
		"Version":  vars.Version,
		"Skills":   vars.Skills,
	}
}

// RenderAll renders a full plan keyed by output path.
func (r Renderer) RenderAll(plan RenderPlan, vars Vars) (map[string][]byte, error) {
	out := map[string][]byte{}
	for _, item := range plan {
		data, err := r.Render(item.Template, vars)
		if err != nil {
			return nil, err
		}
		out[item.OutPath] = data
	}
	return out, nil
}

func funcs() template.FuncMap {
	return template.FuncMap{
		"title": strings.Title,
		"lower": strings.ToLower,
		"join":  strings.Join,
	}
}

func sortedSkills(skills map[string]string) []SkillEntry {
	names := make([]string, 0, len(skills))
	for name := range skills {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]SkillEntry, 0, len(names))
	for _, name := range names {
		out = append(out, SkillEntry{Name: name, Path: ".ai/skills/" + name + "/SKILL.md", Source: "project"})
	}
	return out
}

func skillEntries(globalSkills, projectSkills map[string]string) []SkillEntry {
	merged := map[string]SkillEntry{}
	for name := range globalSkills {
		merged[name] = SkillEntry{Name: name, Path: "~/.agents/skills/" + name + "/SKILL.md", Source: "global"}
	}
	for name := range projectSkills {
		merged[name] = SkillEntry{Name: name, Path: ".ai/skills/" + name + "/SKILL.md", Source: "project"}
	}
	names := make([]string, 0, len(merged))
	for name := range merged {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]SkillEntry, 0, len(names))
	for _, name := range names {
		out = append(out, merged[name])
	}
	return out
}
