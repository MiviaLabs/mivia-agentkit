// Package render renders embedded templates into target repo files.
// Plan: WS2. PRD: FR-1.1, FR-1.2, FR-1.3, FR-10.6.
package render

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/MiviaLabs/mivia-agentkit/internal/config"
	"github.com/MiviaLabs/mivia-agentkit/internal/globalconfig"
	"github.com/MiviaLabs/mivia-agentkit/internal/templates"
	"github.com/MiviaLabs/mivia-agentkit/internal/version"
)

// InitConfig configures init planning and writing.
type InitConfig struct {
	Repo     string
	Profile  string
	Adapters []string
	Force    bool
}

// FileAction is one planned or executed file operation.
type FileAction struct {
	Path   string `json:"path"`
	Action string `json:"action"`
}

// InitReport is the init command report.
type InitReport struct {
	FilesCreated []string     `json:"files_created"`
	FilesSkipped []string     `json:"files_skipped"`
	Conflicts    []string     `json:"conflicts"`
	Actions      []FileAction `json:"actions"`
}

// PlanInit returns the render plan for an init run.
func PlanInit(cfg InitConfig) (RenderPlan, Vars, error) {
	if cfg.Profile == "" {
		cfg.Profile = "standard"
	}
	manifest := config.Defaults()
	manifest.Profile = cfg.Profile
	for name, adapter := range manifest.Adapters {
		adapter.Enabled = false
		manifest.Adapters[name] = adapter
	}
	for _, name := range cfg.Adapters {
		adapter, ok := manifest.Adapters[name]
		if !ok {
			return nil, Vars{}, fmt.Errorf("unknown adapter %q", name)
		}
		adapter.Enabled = true
		manifest.Adapters[name] = adapter
	}
	if len(cfg.Adapters) == 0 {
		cfg.Adapters = []string{"codex", "claude", "copilot"}
		for _, name := range cfg.Adapters {
			adapter := manifest.Adapters[name]
			adapter.Enabled = true
			manifest.Adapters[name] = adapter
		}
	}
	global, err := globalconfig.Read()
	if err != nil {
		return nil, Vars{}, err
	}
	projectSkills := map[string]string{
		"airtight-feature-delivery": "project",
		"test-coverage-audit":       "project",
		"deep-bug-audit":            "project",
		"adversarial-test-review":   "project",
	}
	effective := globalconfig.Layer(global, manifest, globalconfig.ProjectContent{Skills: projectSkills})
	outPaths, err := templates.List(effective.Manifest.Profile, cfg.Adapters)
	if err != nil {
		return nil, Vars{}, err
	}
	plan := make(RenderPlan, 0, len(outPaths))
	for _, out := range outPaths {
		tpl, ok := templates.TemplateForOutput(out)
		if !ok {
			return nil, Vars{}, fmt.Errorf("no template for %s", out)
		}
		plan = append(plan, RenderItem{Template: tpl, OutPath: out})
	}
	vars := Vars{
		Project:  ProjectVars{Name: filepath.Base(absRepo(cfg.Repo))},
		Profile:  effective.Manifest.Profile,
		Adapters: adapterMap(cfg.Adapters),
		Binary:   "mivia-agent",
		Version:  version.Version,
		Skills:   skillEntries(global.Skills, projectSkills),
	}
	return plan, vars, nil
}

// PreviewInit computes actions without writing.
func PreviewInit(cfg InitConfig) (InitReport, error) {
	repo := absRepo(cfg.Repo)
	plan, vars, err := PlanInit(cfg)
	if err != nil {
		return InitReport{}, err
	}
	rendered, err := New().RenderAll(plan, vars)
	if err != nil {
		return InitReport{}, err
	}
	return classifyWrite(repo, plan, rendered, cfg.Force)
}

// WriteInit renders and writes init output.
func WriteInit(cfg InitConfig) (InitReport, error) {
	repo := absRepo(cfg.Repo)
	plan, vars, err := PlanInit(cfg)
	if err != nil {
		return InitReport{}, err
	}
	rendered, err := New().RenderAll(plan, vars)
	if err != nil {
		return InitReport{}, err
	}
	report, err := classifyWrite(repo, plan, rendered, cfg.Force)
	if err != nil {
		return InitReport{}, err
	}
	for _, item := range plan {
		if isConflict(report, item.OutPath) {
			continue
		}
		path := filepath.Join(repo, filepath.FromSlash(item.OutPath))
		data := rendered[item.OutPath]
		if old, err := os.ReadFile(path); err == nil && HasManaged(old) {
			_, managed, _, _ := ExtractManaged(data)
			next, err := ReplaceManaged(old, managed)
			if err != nil {
				return InitReport{}, err
			}
			data = next
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return InitReport{}, err
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return InitReport{}, err
		}
	}
	if len(report.Conflicts) > 0 {
		return report, fmt.Errorf("init conflicts: %v", report.Conflicts)
	}
	return report, nil
}

func classifyWrite(repo string, plan RenderPlan, rendered map[string][]byte, force bool) (InitReport, error) {
	report := InitReport{}
	for _, item := range plan {
		path := filepath.Join(repo, filepath.FromSlash(item.OutPath))
		action := "would-create"
		if data, err := os.ReadFile(path); err == nil {
			switch {
			case HasManaged(data), force:
				action = "would-update"
				report.FilesSkipped = append(report.FilesSkipped, item.OutPath)
			case string(data) == string(rendered[item.OutPath]):
				action = "would-skip"
				report.FilesSkipped = append(report.FilesSkipped, item.OutPath)
			default:
				action = "would-conflict"
				report.Conflicts = append(report.Conflicts, item.OutPath)
			}
		} else if !os.IsNotExist(err) {
			return InitReport{}, err
		} else {
			report.FilesCreated = append(report.FilesCreated, item.OutPath)
		}
		report.Actions = append(report.Actions, FileAction{Path: item.OutPath, Action: action})
	}
	sort.Strings(report.FilesCreated)
	sort.Strings(report.FilesSkipped)
	sort.Strings(report.Conflicts)
	return report, nil
}

func isConflict(report InitReport, outPath string) bool {
	for _, conflict := range report.Conflicts {
		if conflict == outPath {
			return true
		}
	}
	return false
}

// JSON returns the report encoded as deterministic JSON.
func (r InitReport) JSON() ([]byte, error) {
	return json.MarshalIndent(r, "", "  ")
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
