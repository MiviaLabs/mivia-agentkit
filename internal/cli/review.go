// Package cli implements the mivia-agent command surface.
// Plan: WS13. PRD: FR-5.3.
package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/MiviaLabs/mivia-agentkit/internal/adapter"
	"github.com/MiviaLabs/mivia-agentkit/internal/config"
	"github.com/MiviaLabs/mivia-agentkit/internal/consensus"
	"github.com/spf13/cobra"
)

func newReviewCommand() *cobra.Command {
	var repo, artifactPath, reviewers, mode, weights, tieBreaker string
	var minReviewers int
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "review",
		Short: "Run one-off consensus review",
		RunE: func(cmd *cobra.Command, _ []string) error {
			artifactPath = resolveArtifactPath(absRepoPath(repo), artifactPath)
			if _, err := os.Stat(artifactPath); err != nil {
				return fmt.Errorf("artifact must exist: %w", err)
			}
			manifest, err := loadManifest(repo)
			if err != nil {
				return err
			}
			names := splitCSV(reviewers)
			if len(names) == 0 {
				names = append([]string(nil), manifest.Routing.DefaultReviewers...)
			}
			reg, err := approvedRegistry(cmd.Context(), manifest, names...)
			if err != nil {
				return err
			}
			var verdicts []adapter.Verdict
			builder := PromptBuilder{Repo: absRepoPath(repo), Vars: map[string]string{"project": manifest.Project.Name}}
			for _, name := range names {
				a, ok := reg.Lookup(name)
				if !ok {
					return fmt.Errorf("adapter %q not approved for review", name)
				}
				prompt, err := builder.Reviewer(config.Step{ID: "review", Artifact: artifactPath}, artifactPath)
				if err != nil {
					return err
				}
				cfg := manifest.Adapters[name]
				v, err := a.Review(cmd.Context(), adapter.Request{
					Prompt:   prompt,
					Workdir:  absRepoPath(repo),
					Approval: "never",
					Model:    cfg.Model,
					Effort:   cfg.Effort,
					Params:   cfg.Params,
				})
				if err != nil {
					return err
				}
				v.Adapter = name
				verdicts = append(verdicts, v)
			}
			policy := consensus.Policy{Mode: consensus.Mode(defaultString(mode, manifest.Routing.Consensus.Mode)), MinReviewers: defaultInt(minReviewers, manifest.Routing.Consensus.MinReviewers), Weights: parseWeights(weights), TieBreaker: consensus.TieBreaker(defaultString(tieBreaker, manifest.Routing.Consensus.TieBreaker))}
			outcome, err := consensus.Evaluate(policy, verdicts)
			if err != nil {
				return err
			}
			if jsonOut {
				_ = json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{"verdicts": verdicts, "outcome": outcome})
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "pass=%t reason=%s\n", outcome.Pass, outcome.Reason)
			}
			if !outcome.Pass {
				return fmt.Errorf("review failed: %s", outcome.Reason)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&repo, "repo", "", "target repository")
	cmd.Flags().StringVar(&artifactPath, "artifact", "", "artifact to review")
	cmd.Flags().StringVar(&reviewers, "reviewers", "", "comma-separated reviewers")
	cmd.Flags().StringVar(&mode, "mode", "", "consensus mode")
	cmd.Flags().IntVar(&minReviewers, "min-reviewers", 0, "minimum reviewers")
	cmd.Flags().StringVar(&weights, "weights", "", "weights k=v,...")
	cmd.Flags().StringVar(&tieBreaker, "tie-breaker", "", "tie breaker")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit JSON")
	return cmd
}

func resolveArtifactPath(repo, artifactPath string) string {
	if artifactPath == "" || filepath.IsAbs(artifactPath) {
		return artifactPath
	}
	return filepath.Join(repo, artifactPath)
}

func splitCSV(raw string) []string {
	var out []string
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func parseWeights(raw string) map[string]float64 {
	out := map[string]float64{}
	for _, part := range splitCSV(raw) {
		k, v, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		f, err := strconv.ParseFloat(v, 64)
		if err == nil {
			out[k] = f
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func defaultString(v, fallback string) string {
	if v != "" {
		return v
	}
	return fallback
}

func defaultInt(v, fallback int) int {
	if v != 0 {
		return v
	}
	return fallback
}
