// Package cli implements the mivia-agent command surface.
// Plan: WS0. PRD: §1, §4, §9.
package cli

import (
	"fmt"

	"github.com/MiviaLabs/mivia-agentkit/internal/version"
	"github.com/spf13/cobra"
)

func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the mivia-agent version",
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "mivia-agent %s\n", version.Version)
			return err
		},
	}
}
