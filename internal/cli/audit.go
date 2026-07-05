// Package cli implements the mivia-agent command surface.
// Plan: WS3. PRD: FR-2.3, FR-6.4.
package cli

import (
	"fmt"

	"github.com/MiviaLabs/mivia-agentkit/internal/audit"
	"github.com/spf13/cobra"
)

func newAuditCommand() *cobra.Command {
	var repo string
	var jsonOut bool
	var strict bool
	cmd := &cobra.Command{
		Use:   "audit",
		Short: "Report mivia-agent quality gaps",
		RunE: func(cmd *cobra.Command, _ []string) error {
			result := audit.Run(audit.Context{Repo: repo, Strict: strict})
			if jsonOut {
				data, err := result.JSON()
				if err != nil {
					return err
				}
				fmt.Fprintln(cmd.OutOrStdout(), string(data))
			} else {
				fmt.Fprint(cmd.OutOrStdout(), result.Text())
			}
			if result.ExitCode != 0 {
				return fmt.Errorf("audit failed with exit code %d", result.ExitCode)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&repo, "repo", "", "target repository")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit JSON")
	cmd.Flags().BoolVar(&strict, "strict", false, "treat warnings as failures")
	return cmd
}
