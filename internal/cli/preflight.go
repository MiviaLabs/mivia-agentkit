// Package cli implements the mivia-agent command surface.
// Plan: WS4. PRD: FR-2.4, FR-7.1.
package cli

import (
	"fmt"

	"github.com/MiviaLabs/mivia-agentkit/internal/preflight"
	"github.com/spf13/cobra"
)

func newPreflightCommand() *cobra.Command {
	var ctx preflight.Context
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "preflight",
		Short: "Write a quality stamp for the current diff",
		RunE: func(cmd *cobra.Command, _ []string) error {
			manifest, err := loadManifest(ctx.Repo)
			if err != nil {
				return err
			}
			ctx.Verifiers = manifest.Quality.Verifiers
			if len(ctx.FocusedVerifiers) == 0 && len(ctx.BroadVerifiers) == 0 {
				ctx.BroadVerifiers = append([]string(nil), manifest.Quality.RequiredVerifiers...)
			}
			stamp, err := preflight.Run(ctx)
			if err != nil {
				return err
			}
			if jsonOut {
				data, err := stamp.Marshal()
				if err != nil {
					return err
				}
				fmt.Fprint(cmd.OutOrStdout(), string(data))
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "preflight stamp written for %s\n", stamp.Head)
			return nil
		},
	}
	cmd.Flags().StringVar(&ctx.Repo, "repo", "", "target repository")
	cmd.Flags().StringArrayVar(&ctx.ContractRows, "contract-row", nil, "verified contract row")
	cmd.Flags().StringArrayVar(&ctx.FocusedVerifiers, "focused-verifier", nil, "trusted focused verifier ID")
	cmd.Flags().StringArrayVar(&ctx.BroadVerifiers, "broad-verifier", nil, "trusted broad verifier ID")
	cmd.Flags().StringArrayVar(&ctx.MutationProofs, "mutation-proof", nil, "mutation proof")
	cmd.Flags().StringArrayVar(&ctx.NotRun, "not-run", nil, "not-run reason")
	cmd.Flags().BoolVar(&ctx.PipelinePreflight, "pipeline-preflight", false, "broad verifier runs outside this command")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit JSON")
	return cmd
}
