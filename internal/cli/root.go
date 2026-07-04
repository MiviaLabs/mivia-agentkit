// Package cli implements the mivia-agent command surface.
// Plan: WS0. PRD: §1, §4, §9.
package cli

import (
	"io"
	"os"

	"github.com/spf13/cobra"
)

// Execute runs the root command using process stdio.
func Execute() error {
	cmd := NewRootCommand()
	cmd.SetOut(os.Stdout)
	cmd.SetErr(os.Stderr)
	return cmd.Execute()
}

// NewRootCommand constructs the root mivia-agent command.
func NewRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "mivia-agent",
		Short:         "Manage local agent-control workflows",
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.AddCommand(newAuditCommand())
	cmd.AddCommand(newDoctorCommand())
	cmd.AddCommand(newInitCommand())
	cmd.AddCommand(newPreflightCommand())
	cmd.AddCommand(newVersionCommand())
	return cmd
}
