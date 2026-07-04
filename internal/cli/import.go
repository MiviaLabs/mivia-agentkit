// Package cli implements the mivia-agent command surface.
// Plan: WS7. PRD: FR-9.1, FR-9.2.
package cli

import (
	"fmt"

	"github.com/MiviaLabs/mivia-agentkit/internal/doctor"
	"github.com/MiviaLabs/mivia-agentkit/internal/importer"
	"github.com/spf13/cobra"
)

func newImportCommand() *cobra.Command {
	var repo string
	var write bool
	var force bool
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "import",
		Short: "Inspect an existing setup and plan .ai migration",
		RunE: func(cmd *cobra.Command, _ []string) error {
			manifest, err := loadManifest(repo)
			if err != nil {
				return err
			}
			plan, err := importer.BuildPlan(absRepoPath(repo), manifest)
			if err != nil {
				return err
			}
			if !write {
				return printImportPlan(cmd, plan, jsonOut)
			}
			report, err := plan.Apply(absRepoPath(repo), force)
			if err != nil {
				return err
			}
			if err := printImportReport(cmd, report, jsonOut); err != nil {
				return err
			}
			return runDoctorAfterWrite(absRepoPath(repo))
		},
	}
	cmd.Flags().StringVar(&repo, "repo", "", "target repository")
	cmd.Flags().BoolVar(&write, "write", false, "write mapped .ai files")
	cmd.Flags().BoolVar(&force, "force", false, "overwrite conflicting mapped files")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit JSON")
	return cmd
}

func printImportPlan(cmd *cobra.Command, plan importer.Plan, jsonOut bool) error {
	if jsonOut {
		data, err := plan.JSON()
		if err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), string(data))
		return nil
	}
	for _, action := range plan.Actions {
		fmt.Fprintf(cmd.OutOrStdout(), "%s -> %s\n", action.Source, action.Path)
	}
	for _, conflict := range plan.Conflicts {
		fmt.Fprintf(cmd.OutOrStdout(), "conflict %s: %s\n", conflict.Path, conflict.Reason)
	}
	return nil
}

func printImportReport(cmd *cobra.Command, report importer.Report, jsonOut bool) error {
	if jsonOut {
		data, err := report.JSON()
		if err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), string(data))
		return nil
	}
	for _, path := range report.Written {
		fmt.Fprintf(cmd.OutOrStdout(), "wrote %s\n", path)
	}
	for _, path := range report.Skipped {
		fmt.Fprintf(cmd.OutOrStdout(), "skipped %s\n", path)
	}
	for _, conflict := range report.Conflicts {
		fmt.Fprintf(cmd.OutOrStdout(), "conflict %s: %s\n", conflict.Path, conflict.Reason)
	}
	return nil
}

func runDoctorAfterWrite(repo string) error {
	result := doctor.Run(doctor.Context{Repo: repo})
	if result.ExitCode != 0 {
		return fmt.Errorf("doctor failed with exit code %d", result.ExitCode)
	}
	return nil
}
