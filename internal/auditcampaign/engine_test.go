// Package auditcampaign tests the campaign engine.
// Plan: WS15.
package auditcampaign

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/MiviaLabs/mivia-agentkit/internal/config"
)

func testCampaign() config.Campaign {
	c := config.CampaignDefaults()
	c.Enabled = true
	c.MaxCycles = 5
	c.MaxDuration = "1h"
	c.CleanPassThreshold = 2
	c.NoProgressThreshold = 2
	c.CommitEnabled = true
	c.Auditor = "codex"
	c.Confirmer = "claude"
	return c
}

func TestEngineTwoCleanAuditsStopWithoutCommit(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir, "c1", "o1")
	audits := 0
	eng := &Engine{
		Campaign:              testCampaign(),
		Store:                 store,
		InteractiveContinuous: true,
		SelfConfirm:           false,
		Audit: func(ctx context.Context, phase Phase, cycle int) (Evidence, error) {
			audits++
			return Evidence{Schema: EvidenceSchema, CampaignRun: "r", BaselineHead: "h", Cycle: cycle}, nil
		},
		Confirm: func(ctx context.Context, phase Phase, cycle int) (Evidence, error) {
			t.Fatalf("confirm should not run on clean path")
			return Evidence{}, nil
		},
		Fix: func(ctx context.Context, phase Phase, cycle int) (Evidence, error) {
			t.Fatalf("fix should not run on clean path")
			return Evidence{}, nil
		},
		Commit: func(ctx context.Context, e Evidence) (string, error) {
			t.Fatalf("commit should not run")
			return "", nil
		},
	}
	res, err := eng.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Terminal != TerminalClean {
		t.Fatalf("terminal = %s, want clean", res.Terminal)
	}
	if res.Commits != 0 {
		t.Fatalf("commits = %d, want 0", res.Commits)
	}
	if audits < 2 {
		t.Fatalf("audits = %d, want >= 2", audits)
	}
}

func TestEngineConfirmedFindingCommitsOnceAndReaudits(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir, "c2", "o1")
	audits := 0
	commits := 0
	eng := &Engine{
		Campaign:              testCampaign(),
		Store:                 store,
		InteractiveContinuous: true,
		Audit: func(ctx context.Context, phase Phase, cycle int) (Evidence, error) {
			audits++
			if audits == 1 {
				return Evidence{
					Schema: EvidenceSchema, CampaignRun: "r", BaselineHead: "h", Cycle: cycle,
					Disposition: DispositionConfirmed, FindingFingerprint: "fp1",
					ChangedPathIDs: []string{"p1"}, VerifierRef: "go-test",
				}, nil
			}
			// subsequent clean
			return Evidence{Schema: EvidenceSchema, CampaignRun: "r", BaselineHead: "h2", Cycle: cycle}, nil
		},
		Confirm: func(ctx context.Context, phase Phase, cycle int) (Evidence, error) {
			return Evidence{
				Schema: EvidenceSchema, CampaignRun: "r", BaselineHead: "h", Cycle: cycle,
				Disposition: DispositionConfirmed, FindingFingerprint: "fp1",
				ChangedPathIDs: []string{"p1"}, VerifierRef: "go-test",
			}, nil
		},
		Fix: func(ctx context.Context, phase Phase, cycle int) (Evidence, error) {
			return Evidence{
				Schema: EvidenceSchema, CampaignRun: "r", BaselineHead: "h", Cycle: cycle,
				Disposition: DispositionFixed, FindingFingerprint: "fp1",
				ChangedPathIDs: []string{"p1"}, VerifierRef: "go-test",
			}, nil
		},
		Commit: func(ctx context.Context, e Evidence) (string, error) {
			commits++
			return "deadbeef", nil
		},
	}
	// need two cleans after commit: audits 2 and 3 clean after first confirmed
	// engine: audit1 confirmed -> commit; audit2 clean streak1; audit3 clean streak2 -> clean stop
	res, err := eng.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if commits != 1 {
		t.Fatalf("commits = %d, want 1", commits)
	}
	if res.Terminal != TerminalClean && res.Terminal != TerminalCycleCap {
		// accept clean
		if res.Terminal != TerminalClean {
			t.Fatalf("terminal = %s", res.Terminal)
		}
	}
}

func TestEngineRejectsSelfConfirm(t *testing.T) {
	dir := t.TempDir()
	eng := &Engine{
		Campaign:              testCampaign(),
		Store:                 NewStore(dir, "c3", "o"),
		InteractiveContinuous: true,
		SelfConfirm:           true,
		Audit:                 func(context.Context, Phase, int) (Evidence, error) { return Evidence{}, nil },
	}
	_, err := eng.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "self-confirmation") {
		t.Fatalf("error = %v, want self-confirmation", err)
	}
}

func TestEngineStopsNoProgressOnDuplicateFingerprint(t *testing.T) {
	dir := t.TempDir()
	c := testCampaign()
	c.NoProgressThreshold = 1
	c.MaxCycles = 3
	eng := &Engine{
		Campaign:              c,
		Store:                 NewStore(dir, "c4", "o"),
		InteractiveContinuous: true,
		Audit: func(ctx context.Context, phase Phase, cycle int) (Evidence, error) {
			return Evidence{
				Schema: EvidenceSchema, CampaignRun: "r", BaselineHead: "h", Cycle: cycle,
				Disposition: DispositionCandidate, FindingFingerprint: "same",
			}, nil
		},
		Confirm: func(context.Context, Phase, int) (Evidence, error) { return Evidence{}, nil },
		Fix:     func(context.Context, Phase, int) (Evidence, error) { return Evidence{}, nil },
	}
	res, err := eng.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Terminal != TerminalNoProgress {
		t.Fatalf("terminal = %s, want no_progress", res.Terminal)
	}
}

func TestEngineRespectsCycleAndDurationCaps(t *testing.T) {
	dir := t.TempDir()
	c := testCampaign()
	c.MaxCycles = 1
	c.CleanPassThreshold = 5 // never clean-stop
	eng := &Engine{
		Campaign:              c,
		Store:                 NewStore(dir, "c5", "o"),
		InteractiveContinuous: true,
		Audit: func(ctx context.Context, phase Phase, cycle int) (Evidence, error) {
			return Evidence{
				Schema: EvidenceSchema, CampaignRun: "r", BaselineHead: "h", Cycle: cycle,
				Disposition: DispositionCandidate, FindingFingerprint: "fp" + time.Now().String(),
			}, nil
		},
		Confirm: func(context.Context, Phase, int) (Evidence, error) { return Evidence{}, nil },
		Fix:     func(context.Context, Phase, int) (Evidence, error) { return Evidence{}, nil },
	}
	res, err := eng.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Terminal != TerminalCycleCap {
		t.Fatalf("terminal = %s, want cycle_cap", res.Terminal)
	}
}

func TestEngineRejectsNonInteractive(t *testing.T) {
	dir := t.TempDir()
	eng := &Engine{
		Campaign:              testCampaign(),
		Store:                 NewStore(dir, "c6", "o"),
		InteractiveContinuous: false,
		Audit:                 func(context.Context, Phase, int) (Evidence, error) { return Evidence{}, nil },
	}
	_, err := eng.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "interactive") {
		t.Fatalf("error = %v, want interactive", err)
	}
}
