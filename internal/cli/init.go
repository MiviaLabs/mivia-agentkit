// Package cli implements the mivia-agent command surface.
// Plan: WS2. PRD: FR-1.1, FR-1.2, FR-1.3.
package cli

import (
	"fmt"

	"github.com/MiviaLabs/mivia-agentkit/internal/render"
	"github.com/spf13/cobra"
)

func newInitCommand() *cobra.Command {
	var cfg render.InitConfig
	var dryRun bool
	var write bool
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Install the mivia-agent control surface",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if dryRun == write {
				return fmt.Errorf("choose exactly one of --dry-run or --write")
			}
			var (
				report render.InitReport
				err    error
			)
			if dryRun {
				report, err = render.PreviewInit(cfg)
			} else {
				report, err = render.WriteInit(cfg)
			}
			if jsonOut {
				data, jsonErr := report.JSON()
				if jsonErr != nil {
					return jsonErr
				}
				fmt.Fprintln(cmd.OutOrStdout(), string(data))
			} else {
				for _, action := range report.Actions {
					fmt.Fprintf(cmd.OutOrStdout(), "%s %s\n", action.Action, action.Path)
				}
			}
			return err
		},
	}
	cmd.Flags().StringVar(&cfg.Repo, "repo", "", "target repository")
	cmd.Flags().StringVar(&cfg.Profile, "profile", "standard", "profile")
	cmd.Flags().StringArrayVar(&cfg.Adapters, "adapter", nil, "adapter to enable")
	cmd.Flags().StringArray("with-loop", nil, "workflow template to enable")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "preview writes")
	cmd.Flags().BoolVar(&write, "write", false, "write files")
	cmd.Flags().BoolVar(&cfg.Force, "force", false, "overwrite user-owned files")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit JSON")
	return cmd
}
