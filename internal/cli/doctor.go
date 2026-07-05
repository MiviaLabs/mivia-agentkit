// Package cli implements the mivia-agent command surface.
// Plan: WS3. PRD: FR-2.1, FR-5.4, FR-10.5.
package cli

import (
	"fmt"

	"github.com/MiviaLabs/mivia-agentkit/internal/doctor"
	"github.com/spf13/cobra"
)

func newDoctorCommand() *cobra.Command {
	var repo string
	var jsonOut bool
	var strict bool
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Validate the mivia-agent control surface",
		RunE: func(cmd *cobra.Command, _ []string) error {
			result := doctor.Run(doctor.Context{Repo: repo, Strict: strict})
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
				return fmt.Errorf("doctor failed with exit code %d", result.ExitCode)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&repo, "repo", "", "target repository")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit JSON")
	cmd.Flags().BoolVar(&strict, "strict", false, "treat warnings as failures")
	return cmd
}
