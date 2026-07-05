// Package cli implements the mivia-agent command surface.
// Plan: WS13. PRD: FR-4.1, FR-4.4.
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/MiviaLabs/mivia-agentkit/internal/adapter"
	"github.com/MiviaLabs/mivia-agentkit/internal/config"
	"github.com/MiviaLabs/mivia-agentkit/internal/orchestrator"
	"github.com/MiviaLabs/mivia-agentkit/internal/policy"
	"github.com/MiviaLabs/mivia-agentkit/internal/preflight"
	"github.com/MiviaLabs/mivia-agentkit/internal/runstore"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func newRunCommand() *cobra.Command {
	var repo, workflow string
	var maxIterations int
	var dryRun, jsonOut, strict bool
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run a bounded agent workflow",
		RunE: func(cmd *cobra.Command, _ []string) error {
			manifest, err := loadManifest(repo)
			if err != nil {
				return err
			}
			loop, err := loadLoop(absRepoPath(repo), manifest, workflow)
			if err != nil {
				return err
			}
			if dryRun {
				return printRunPlan(cmd, manifest, loop, jsonOut)
			}
			reg, err := approvedRegistry(cmd.Context(), manifest, loopAdapterNames(loop)...)
			if err != nil {
				return err
			}
			prov, err := policy.New(manifest.Governance.Provider, filepath.Join(absRepoPath(repo), ".ai", "audit.jsonl"))
			if err != nil {
				return err
			}
			builder := PromptBuilder{Repo: absRepoPath(repo), Vars: map[string]string{"project": manifest.Project.Name}}
			engine := orchestrator.Engine{Adapters: reg, Policy: prov, Store: runstore.New(absRepoPath(repo)), AdapterDefaults: manifest.Adapters, Repo: absRepoPath(repo), MaxIterations: maxIterations, Stamp: func(repo string) (string, error) {
				stamp, err := preflight.CheckStamp(repo)
				return stamp.Head, err
			}}
			result, err := engine.RunLoop(cmd.Context(), loop, func(step config.Step, _ int, prior []adapter.Verdict, artifactPath string) (string, error) {
				if len(step.Reviewers) > 0 {
					if artifactPath == "" {
						artifactPath = step.Artifact
					}
					return builder.Reviewer(step, artifactPath)
				}
				return builder.Producer(step, prior)
			})
			if jsonOut {
				_ = json.NewEncoder(cmd.OutOrStdout()).Encode(result)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "outcome=%s iterations=%d trace=%s\n", result.Outcome, result.Iterations, result.Trace)
			}
			if err != nil {
				if result.Outcome == "warn" && !strict {
					return cobra.NoArgs(cmd, nil)
				}
				return err
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&repo, "repo", "", "target repository")
	cmd.Flags().StringVar(&workflow, "workflow", "", "workflow name")
	cmd.Flags().IntVar(&maxIterations, "max-iterations", 0, "iteration cap")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "preview plan")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit JSON")
	cmd.Flags().BoolVar(&strict, "strict", false, "fail warn-only outcomes")
	cmd.Flags().String("step", "", "specific step")
	cmd.Flags().String("input-artifact", "", "input artifact")
	cmd.Flags().StringArray("var", nil, "template variable")
	return cmd
}

func printRunPlan(cmd *cobra.Command, manifest config.Manifest, loop config.Loop, jsonOut bool) error {
	nodes, err := orchestrator.Resolve(loop)
	if err != nil {
		return err
	}
	type runtimeRow struct {
		Adapter string `json:"adapter"`
		Model   string `json:"model"`
		Effort  string `json:"effort"`
	}
	type row struct {
		Step     string       `json:"step"`
		Type     string       `json:"type"`
		Adapters []string     `json:"adapters"`
		Runtime  []runtimeRow `json:"runtime"`
		MaxTurns int          `json:"max_turns"`
		Timeout  string       `json:"timeout"`
		Artifact string       `json:"artifact"`
	}
	var rows []row
	for _, node := range nodes {
		r := row{Step: node.Step.ID, MaxTurns: node.Step.MaxTurns, Timeout: node.Step.Timeout, Artifact: node.Step.Artifact}
		if node.Step.Producer != "" {
			r.Type = "producer"
			r.Adapters = []string{node.Step.Producer}
		} else {
			r.Type = "review"
			r.Adapters = append([]string(nil), node.Step.Reviewers...)
		}
		for _, name := range r.Adapters {
			model := manifest.Adapters[name].Model
			if node.Step.Model != "" {
				model = node.Step.Model
			}
			effort := manifest.Adapters[name].Effort
			if node.Step.Effort != "" {
				effort = node.Step.Effort
			}
			r.Runtime = append(r.Runtime, runtimeRow{Adapter: name, Model: model, Effort: effort})
		}
		rows = append(rows, r)
	}
	if jsonOut {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(rows)
	}
	for _, row := range rows {
		fmt.Fprintf(cmd.OutOrStdout(), "%s %s %s\n", row.Step, row.Type, strings.Join(row.Adapters, ","))
	}
	return nil
}

func loadManifest(repo string) (config.Manifest, error) {
	path := filepath.Join(absRepoPath(repo), "mivia-agent.yaml")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return config.Defaults(), nil
	}
	if err != nil {
		return config.Manifest{}, err
	}
	return config.Parse(data)
}

func loadLoop(repo string, manifest config.Manifest, name string) (config.Loop, error) {
	if name == "" {
		return config.Loop{}, fmt.Errorf("workflow is required")
	}
	if loop, ok := manifest.Loops[name]; ok {
		if err := validateRunLoop(manifest, loop); err != nil {
			return config.Loop{}, err
		}
		return loop, nil
	}
	data, err := readWorkflow(repo, name)
	if err != nil {
		return config.Loop{}, fmt.Errorf("unknown workflow %q", name)
	}
	var doc workflowDoc
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true)
	if err := dec.Decode(&doc); err != nil {
		return config.Loop{}, err
	}
	loop := doc.Loop()
	if err := loop.Validate(enabledAdapters(manifest)); err != nil {
		return config.Loop{}, err
	}
	if err := validateRunLoop(manifest, loop); err != nil {
		return config.Loop{}, err
	}
	return loop, nil
}

type workflowDoc struct {
	Version       int           `yaml:"version"`
	Name          string        `yaml:"name"`
	Description   string        `yaml:"description"`
	Bound         string        `yaml:"bound"`
	MaxIterations int           `yaml:"max_iterations"`
	Steps         []config.Step `yaml:"steps"`
	ExitWhen      string        `yaml:"exit_when"`
	OnExhausted   string        `yaml:"on_exhausted"`
}

func (d workflowDoc) Loop() config.Loop {
	return config.Loop{Description: d.Description, Bound: d.Bound, MaxIterations: d.MaxIterations, Steps: d.Steps, ExitWhen: d.ExitWhen, OnExhausted: d.OnExhausted}
}

func readWorkflow(repo, name string) ([]byte, error) {
	candidates := []string{name + ".yaml"}
	if !strings.HasSuffix(name, "-loop") {
		candidates = append(candidates, name+"-loop.yaml")
	}
	for _, candidate := range candidates {
		data, err := os.ReadFile(filepath.Join(repo, ".ai", "workflows", candidate))
		if err == nil {
			return data, nil
		}
	}
	return nil, os.ErrNotExist
}

func validateRunLoop(manifest config.Manifest, loop config.Loop) error {
	if loop.Bound == "budget" {
		return orchestrator.ErrBudgetNotSupportedInMVP
	}
	if manifest.Profile != "strict" {
		return nil
	}
	protectBound := loop.ExitWhen == "protected_action"
	for _, step := range loop.Steps {
		if strings.HasPrefix(step.Approval, "protect:") {
			protectBound = true
		}
	}
	if !protectBound {
		return nil
	}
	for _, step := range loop.Steps {
		mode := step.Consensus.Mode
		if mode == "" {
			mode = manifest.Routing.Consensus.Mode
		}
		if mode == "first-pass" {
			return fmt.Errorf("strict protected loops cannot use first-pass consensus")
		}
	}
	return nil
}

func approvedRegistry(ctx context.Context, manifest config.Manifest, requiredNames ...string) (*adapter.Registry, error) {
	required := map[string]struct{}{}
	for _, name := range requiredNames {
		required[name] = struct{}{}
	}
	if len(required) == 0 {
		required = nil
	}
	statuses, err := detectAdapterStatuses(ctx, manifest, required)
	if err != nil {
		return nil, err
	}
	approved := map[string]bool{}
	for _, status := range statuses {
		approved[status.Name] = status.ApprovedForRun
	}
	var adapters []adapter.Adapter
	for _, a := range runtimeAdapters() {
		if approved[a.Name()] {
			adapters = append(adapters, a)
		}
	}
	return adapter.NewRegistry(adapters...)
}

func loopAdapterNames(loop config.Loop) []string {
	seen := map[string]struct{}{}
	for _, step := range loop.Steps {
		if step.Producer != "" {
			seen[step.Producer] = struct{}{}
		}
		for _, reviewer := range step.Reviewers {
			seen[reviewer] = struct{}{}
		}
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	return names
}

func enabledAdapters(manifest config.Manifest) map[string]config.AdapterRole {
	out := map[string]config.AdapterRole{}
	for name, cfg := range manifest.Adapters {
		if cfg.Enabled {
			out[name] = cfg.Role
		}
	}
	return out
}

func absRepoPath(repo string) string {
	if repo == "" {
		repo, _ = os.Getwd()
	}
	abs, err := filepath.Abs(repo)
	if err != nil {
		return repo
	}
	return abs
}
