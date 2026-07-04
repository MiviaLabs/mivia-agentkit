// Package cli implements the mivia-agent command surface.
// Plan: WS7. PRD: FR-1.4.
package cli

import (
	"encoding/json"
	"fmt"

	"github.com/MiviaLabs/mivia-agentkit/internal/update"
	"github.com/spf13/cobra"
)

func newUpdateCommand() *cobra.Command {
	var repo string
	var write bool
	var force bool
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Refresh managed template blocks",
		RunE: func(cmd *cobra.Command, _ []string) error {
			repo = absRepoPath(repo)
			if !write {
				changes, err := update.Diff(repo)
				if err != nil {
					return err
				}
				if jsonOut {
					return jsonOutChanges(cmd, changes)
				}
				for _, change := range changes {
					fmt.Fprintf(cmd.OutOrStdout(), "%s %s\n", change.Kind, change.Path)
				}
				return nil
			}
			report, err := update.Apply(repo, force)
			if err != nil {
				return err
			}
			if jsonOut {
				data, jsonErr := report.JSON()
				if jsonErr != nil {
					return jsonErr
				}
				fmt.Fprintln(cmd.OutOrStdout(), string(data))
			} else {
				for _, path := range report.Updated {
					fmt.Fprintf(cmd.OutOrStdout(), "updated %s\n", path)
				}
				for _, conflict := range report.Conflicts {
					fmt.Fprintf(cmd.OutOrStdout(), "conflict %s: %s\n", conflict.Path, conflict.Reason)
				}
			}
			return runDoctorAfterWrite(repo)
		},
	}
	cmd.Flags().StringVar(&repo, "repo", "", "target repository")
	cmd.Flags().BoolVar(&write, "write", false, "write updated managed blocks")
	cmd.Flags().BoolVar(&force, "force", false, "overwrite conflicted managed blocks")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit JSON")
	return cmd
}

func jsonOutChanges(cmd *cobra.Command, changes []update.Change) error {
	data, err := json.MarshalIndent(changes, "", "  ")
	if err != nil {
		return err
	}
	fmt.Fprintln(cmd.OutOrStdout(), string(data))
	return nil
}
