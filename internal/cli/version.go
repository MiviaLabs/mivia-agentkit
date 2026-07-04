// Package cli implements the mivia-agent command surface.
// Plan: WS0. PRD: §1, §4, §9.
package cli

import (
	"encoding/json"
	"fmt"

	"github.com/MiviaLabs/mivia-agentkit/internal/version"
	"github.com/spf13/cobra"
)

func newVersionCommand() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print the mivia-agent version",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if jsonOut {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]string{
					"version": version.Version,
					"commit":  version.Commit,
					"date":    version.Date,
				})
			}
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "mivia-agent %s\n", version.Version)
			return err
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit build info as JSON")
	return cmd
}
