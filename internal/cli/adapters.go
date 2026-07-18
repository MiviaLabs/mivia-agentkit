// Package cli implements the mivia-agent command surface.
// Plan: WS13. PRD: FR-3.2.
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/MiviaLabs/mivia-agentkit/internal/adapter"
	"github.com/MiviaLabs/mivia-agentkit/internal/config"
	"github.com/spf13/cobra"
)

type adapterStatus struct {
	Name           string `json:"name"`
	Installed      bool   `json:"installed"`
	Version        string `json:"version"`
	Headless       bool   `json:"headless"`
	Role           string `json:"role"`
	ApprovedForRun bool   `json:"approved_for_run"`
}

var runtimeAdapters = func() []adapter.Adapter {
	return []adapter.Adapter{adapter.Codex{}, adapter.Claude{}, adapter.Antigravity{}, adapter.Crush{}, adapter.Zai{}, adapter.Kimi{}}
}

func newAdaptersCommand() *cobra.Command {
	var repo string
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "adapters",
		Short: "List local agent adapters",
		RunE: func(cmd *cobra.Command, _ []string) error {
			manifest, err := loadManifest(repo)
			if err != nil {
				return err
			}
			statuses, err := detectAdapterStatuses(cmd.Context(), manifest, map[string]struct{}{})
			if jsonOut {
				data, jsonErr := json.Marshal(statuses)
				if jsonErr != nil {
					return jsonErr
				}
				fmt.Fprintln(cmd.OutOrStdout(), string(data))
			} else {
				for _, s := range statuses {
					fmt.Fprintf(cmd.OutOrStdout(), "%s | %t | %t | %s | %t\n", s.Name, s.Installed, s.Headless, s.Role, s.ApprovedForRun)
				}
			}
			return err
		},
	}
	cmd.Flags().StringVar(&repo, "repo", "", "target repository")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit JSON")
	return cmd
}

func detectAdapters(ctx context.Context, manifest config.Manifest) ([]adapterStatus, error) {
	return detectAdapterStatuses(ctx, manifest, nil)
}

func detectAdapterStatuses(ctx context.Context, manifest config.Manifest, required map[string]struct{}) ([]adapterStatus, error) {
	var statuses []adapterStatus
	var blocked []string
	seen := map[string]struct{}{}
	for _, a := range runtimeAdapters() {
		d, err := a.Detect(ctx)
		installed := err == nil
		cfg := manifest.Adapters[a.Name()]
		seen[a.Name()] = struct{}{}
		role := a.Role()
		if cfg.Role != "" {
			role = cfg.Role
		}
		approved := installed && d.HeadlessCapable && role == config.AdapterRoleOrchestrable
		statuses = append(statuses, adapterStatus{Name: a.Name(), Installed: installed, Version: d.Version, Headless: d.HeadlessCapable, Role: string(role), ApprovedForRun: approved})
		if shouldBlockAdapter(a.Name(), cfg, required) && !approved {
			blocked = append(blocked, a.Name())
		}
	}
	var missing []string
	for name := range manifest.Adapters {
		if _, ok := seen[name]; !ok {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	for _, name := range missing {
		cfg := manifest.Adapters[name]
		statuses = append(statuses, adapterStatus{Name: name, Installed: false, Role: string(cfg.Role)})
		if shouldBlockAdapter(name, cfg, required) {
			blocked = append(blocked, name)
		}
	}
	if len(blocked) > 0 {
		return statuses, fmt.Errorf("orchestrable adapters not approved for run: %v", blocked)
	}
	return statuses, nil
}

func shouldBlockAdapter(name string, cfg config.AdapterConfig, required map[string]struct{}) bool {
	if cfg.Role != config.AdapterRoleOrchestrable {
		return false
	}
	if required != nil {
		_, ok := required[name]
		return ok
	}
	return cfg.Enabled
}
