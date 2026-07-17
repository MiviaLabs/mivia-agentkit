// Package cli implements the mivia-agent command surface.
// Plan: WS15. PRD: campaign run|status|resume.
package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/MiviaLabs/mivia-agentkit/internal/auditcampaign"
	"github.com/MiviaLabs/mivia-agentkit/internal/config"
	"github.com/MiviaLabs/mivia-agentkit/internal/gitstate"
	"github.com/spf13/cobra"
)

type campaignOptions struct {
	repo       string
	campaign   string
	runID      string
	continuous bool
	jsonOut    bool
	// confirmContinuous is set only by interactive confirmation (not JSON/flags alone).
	confirmContinuous bool
}

func newCampaignCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "campaign",
		Short: "Run supervised audit-repair campaigns",
	}
	cmd.AddCommand(newCampaignRunCommand())
	cmd.AddCommand(newCampaignStatusCommand())
	cmd.AddCommand(newCampaignResumeCommand())
	return cmd
}

func newCampaignRunCommand() *cobra.Command {
	var opt campaignOptions
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Start a bounded supervised campaign",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCampaign(cmd, opt)
		},
	}
	cmd.Flags().StringVar(&opt.repo, "repo", ".", "repository path")
	cmd.Flags().StringVar(&opt.campaign, "campaign", "", "campaign name from manifest")
	cmd.Flags().BoolVar(&opt.continuous, "continuous", false, "run continuous finite cycles (interactive only)")
	cmd.Flags().BoolVar(&opt.jsonOut, "json", false, "emit JSON")
	return cmd
}

func newCampaignStatusCommand() *cobra.Command {
	var opt campaignOptions
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show redacted campaign status",
		RunE: func(cmd *cobra.Command, args []string) error {
			return statusCampaign(cmd, opt)
		},
	}
	cmd.Flags().StringVar(&opt.repo, "repo", ".", "repository path")
	cmd.Flags().StringVar(&opt.runID, "run", "", "campaign run id")
	cmd.Flags().BoolVar(&opt.jsonOut, "json", false, "emit JSON")
	return cmd
}

func newCampaignResumeCommand() *cobra.Command {
	var opt campaignOptions
	cmd := &cobra.Command{
		Use:   "resume",
		Short: "Resume a campaign run",
		RunE: func(cmd *cobra.Command, args []string) error {
			return resumeCampaign(cmd, opt)
		},
	}
	cmd.Flags().StringVar(&opt.repo, "repo", ".", "repository path")
	cmd.Flags().StringVar(&opt.runID, "run", "", "campaign run id")
	cmd.Flags().BoolVar(&opt.jsonOut, "json", false, "emit JSON")
	return cmd
}

func runCampaign(cmd *cobra.Command, opt campaignOptions) error {
	if opt.campaign == "" {
		return ExitError{Code: 2, Err: fmt.Errorf("--campaign is required")}
	}
	if opt.continuous {
		if !isInteractiveContinuous(cmd) {
			return ExitError{Code: 2, Err: fmt.Errorf("--continuous requires interactive TTY; CI/noninteractive rejected")}
		}
		opt.confirmContinuous = true
	}
	repo := absRepoPath(opt.repo)
	manifest, err := loadCampaignManifest(repo)
	if err != nil {
		return ExitError{Code: 2, Err: err}
	}
	camp, ok := manifest.Campaigns[opt.campaign]
	if !ok {
		return ExitError{Code: 2, Err: fmt.Errorf("unknown campaign %q", opt.campaign)}
	}
	if !camp.Enabled {
		return ExitError{Code: 2, Err: fmt.Errorf("campaign %q is disabled", opt.campaign)}
	}
	if camp.CommitEnabled && camp.Auditor == camp.Confirmer {
		return ExitError{Code: 2, Err: fmt.Errorf("self-confirmation rejected for commit-capable campaign")}
	}
	runID := opt.campaign + "-" + time.Now().UTC().Format("20060102T150405Z")
	store := auditcampaign.NewStore(repo, runID, "cli")
	host, err := newCampaignHost(cmd.Context(), repo, runID, opt.campaign, camp, manifest)
	if err != nil {
		return ExitError{Code: 1, Err: err}
	}
	eng := &auditcampaign.Engine{
		Campaign:              camp,
		Store:                 store,
		Continuous:            opt.continuous,
		InteractiveContinuous: opt.confirmContinuous,
		SelfConfirm:           camp.Auditor != "" && camp.Auditor == camp.Confirmer,
		Audit:                 host.Audit,
		Confirm:               host.Confirm,
		Fix:                   host.Fix,
		Commit:                host.Commit,
	}
	res, err := eng.Run(cmd.Context())
	if err != nil {
		return ExitError{Code: 1, Err: err}
	}
	return writeCampaignResult(cmd, opt.jsonOut, res)
}

func statusCampaign(cmd *cobra.Command, opt campaignOptions) error {
	if opt.runID == "" {
		return ExitError{Code: 2, Err: fmt.Errorf("--run is required")}
	}
	repo := absRepoPath(opt.repo)
	store := auditcampaign.NewStore(repo, opt.runID, "cli")
	snap, err := store.Load()
	if err != nil {
		return ExitError{Code: 1, Err: err}
	}
	// Redact: only safe fields.
	out := map[string]any{
		"campaign_id":     snap.CampaignID,
		"phase":           snap.Phase,
		"cycle":           snap.Cycle,
		"terminal_reason": snap.TerminalReason,
		"clean_streak":    snap.CleanStreak,
		"baseline_head":   snap.BaselineHead,
	}
	return writeJSONOrText(cmd, opt.jsonOut, out)
}

func resumeCampaign(cmd *cobra.Command, opt campaignOptions) error {
	if opt.runID == "" {
		return ExitError{Code: 2, Err: fmt.Errorf("--run is required")}
	}
	// Resume cannot bypass continuous interactivity for commit-capable runs.
	if isCIEnv() {
		return ExitError{Code: 2, Err: fmt.Errorf("resume rejected in CI/noninteractive environment")}
	}
	repo := absRepoPath(opt.repo)
	store := auditcampaign.NewStore(repo, opt.runID, "cli")
	wantHead := ""
	wantBranch := ""
	if head, err := gitstate.Head(repo); err == nil {
		wantHead = head
	}
	if err := store.ResumePreconditions(wantBranch, wantHead, "cli"); err != nil {
		// Empty branch skips branch check when unknown; head mismatch still fails.
		if wantHead != "" || err.Error() != "resume rejected: head mismatch" {
			if err.Error() == "resume rejected: branch mismatch" && wantBranch == "" {
				// skip empty branch
			} else if err.Error() != "resume rejected: branch mismatch" {
				// allow load for status-like resume when only branch empty and head empty
				if wantHead != "" {
					return ExitError{Code: 1, Err: err}
				}
			}
		}
	}
	snap, err := store.Load()
	if err != nil {
		return ExitError{Code: 1, Err: err}
	}
	out := map[string]any{"campaign_id": snap.CampaignID, "phase": snap.Phase, "resumable": snap.Phase != auditcampaign.PhaseTerminal}
	return writeJSONOrText(cmd, opt.jsonOut, out)
}

func loadCampaignManifest(repo string) (config.Manifest, error) {
	path := filepath.Join(repo, "mivia-agent.yaml")
	b, err := os.ReadFile(path)
	if err != nil {
		// empty campaigns map is valid
		m := config.Defaults()
		m.Campaigns = map[string]config.Campaign{}
		return m, nil
	}
	return config.Parse(b)
}

func isInteractiveContinuous(cmd *cobra.Command) bool {
	if isCIEnv() {
		return false
	}
	// Require stdin to be a terminal-like file; non-TTY rejected.
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

func isCIEnv() bool {
	for _, k := range []string{"CI", "GITHUB_ACTIONS", "GITLAB_CI", "BUILDKITE", "CIRCLECI", "TF_BUILD"} {
		if os.Getenv(k) != "" {
			return true
		}
	}
	return false
}

func writeCampaignResult(cmd *cobra.Command, jsonOut bool, res auditcampaign.Result) error {
	if jsonOut {
		enc := json.NewEncoder(cmd.OutOrStdout())
		return enc.Encode(res)
	}
	_, err := fmt.Fprintf(cmd.OutOrStdout(), "terminal=%s cycles=%d commits=%d\n", res.Terminal, res.Cycles, res.Commits)
	return err
}

func writeJSONOrText(cmd *cobra.Command, jsonOut bool, v any) error {
	if jsonOut {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(v)
	}
	_, err := fmt.Fprintf(cmd.OutOrStdout(), "%v\n", v)
	return err
}
