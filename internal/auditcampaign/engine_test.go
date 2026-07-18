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
		Continuous:            true,
		InteractiveContinuous: false,
		Audit:                 func(context.Context, Phase, int) (Evidence, error) { return Evidence{}, nil },
	}
	_, err := eng.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "interactive") {
		t.Fatalf("error = %v, want interactive", err)
	}
}

func TestEngineFiniteRunWithoutContinuousTTY(t *testing.T) {
	dir := t.TempDir()
	eng := &Engine{
		Campaign:              testCampaign(),
		Store:                 NewStore(dir, "c7", "o"),
		Continuous:            false,
		InteractiveContinuous: false,
		Audit: func(ctx context.Context, phase Phase, cycle int) (Evidence, error) {
			return Evidence{Schema: EvidenceSchema, CampaignRun: "r", BaselineHead: "h", Cycle: cycle}, nil
		},
		Confirm: func(context.Context, Phase, int) (Evidence, error) {
			t.Fatalf("confirm should not run")
			return Evidence{}, nil
		},
		Fix: func(context.Context, Phase, int) (Evidence, error) {
			t.Fatalf("fix should not run")
			return Evidence{}, nil
		},
		Commit: func(context.Context, Evidence) (string, error) {
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
}

func TestEngineCandidateInvokesIndependentConfirm(t *testing.T) {
	dir := t.TempDir()
	c := testCampaign()
	c.CommitEnabled = false
	confirms := 0
	eng := &Engine{
		Campaign: c,
		Store:    NewStore(dir, "c-cand", "o"),
		Audit: func(ctx context.Context, phase Phase, cycle int) (Evidence, error) {
			if cycle == 1 {
				return Evidence{
					Schema: EvidenceSchema, CampaignRun: "r", BaselineHead: "h", Cycle: cycle,
					Disposition: DispositionCandidate, FindingFingerprint: "fp-cand",
				}, nil
			}
			return Evidence{Schema: EvidenceSchema, CampaignRun: "r", BaselineHead: "h", Cycle: cycle}, nil
		},
		Confirm: func(ctx context.Context, phase Phase, cycle int) (Evidence, error) {
			confirms++
			return Evidence{
				Schema: EvidenceSchema, CampaignRun: "r", BaselineHead: "h", Cycle: cycle,
				Disposition: DispositionRejected, FindingFingerprint: "fp-cand",
			}, nil
		},
		Fix: func(context.Context, Phase, int) (Evidence, error) {
			t.Fatalf("fix must not run when commit_enabled=false")
			return Evidence{}, nil
		},
		Commit: func(context.Context, Evidence) (string, error) {
			t.Fatalf("commit must not run when commit_enabled=false")
			return "", nil
		},
	}
	res, err := eng.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if confirms < 1 {
		t.Fatalf("confirms = %d, want >= 1 (candidate must invoke confirmer)", confirms)
	}
	if res.Terminal == TerminalCommitFailed {
		t.Fatalf("terminal = commit_failed, want non-failure path when commit_enabled=false")
	}
	if res.Commits != 0 {
		t.Fatalf("commits = %d, want 0", res.Commits)
	}
}

func TestEngineCommitDisabledDoesNotFailOnConfirmed(t *testing.T) {
	dir := t.TempDir()
	c := testCampaign()
	c.CommitEnabled = false
	c.MaxCycles = 4
	c.CleanPassThreshold = 2
	fixes := 0
	eng := &Engine{
		Campaign: c,
		Store:    NewStore(dir, "c-nocommit", "o"),
		Audit: func(ctx context.Context, phase Phase, cycle int) (Evidence, error) {
			if cycle <= 1 {
				return Evidence{
					Schema: EvidenceSchema, CampaignRun: "r", BaselineHead: "h", Cycle: cycle,
					Disposition: DispositionCandidate, FindingFingerprint: "fp1",
				}, nil
			}
			return Evidence{Schema: EvidenceSchema, CampaignRun: "r", BaselineHead: "h", Cycle: cycle}, nil
		},
		Confirm: func(ctx context.Context, phase Phase, cycle int) (Evidence, error) {
			return Evidence{
				Schema: EvidenceSchema, CampaignRun: "r", BaselineHead: "h", Cycle: cycle,
				Disposition: DispositionConfirmed, FindingFingerprint: "fp1",
				ChangedPathIDs: []string{"p1"}, VerifierRef: "true",
			}, nil
		},
		Fix: func(context.Context, Phase, int) (Evidence, error) {
			fixes++
			return Evidence{}, nil
		},
		Commit: func(context.Context, Evidence) (string, error) {
			t.Fatalf("commit must not run")
			return "", nil
		},
	}
	res, err := eng.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if fixes != 0 {
		t.Fatalf("fixes = %d, want 0 when commit_enabled=false", fixes)
	}
	if res.Terminal == TerminalCommitFailed {
		t.Fatalf("terminal = %s message=%s, must not be commit_failed", res.Terminal, res.Message)
	}
	if !SuccessTerminal(res.Terminal) {
		t.Fatalf("terminal = %s, want success terminal", res.Terminal)
	}
}

func TestEngineBareConfirmedWithoutPathsSkipsFix(t *testing.T) {
	dir := t.TempDir()
	fixes := 0
	eng := &Engine{
		Campaign: testCampaign(),
		Store:    NewStore(dir, "c-bare", "o"),
		Audit: func(ctx context.Context, phase Phase, cycle int) (Evidence, error) {
			return Evidence{
				Schema: EvidenceSchema, CampaignRun: "r", BaselineHead: "h", Cycle: cycle,
				Disposition: DispositionCandidate, FindingFingerprint: "fp-bare",
			}, nil
		},
		Confirm: func(ctx context.Context, phase Phase, cycle int) (Evidence, error) {
			// confirmed without paths/verifier is not CommitEligible
			return Evidence{
				Schema: EvidenceSchema, CampaignRun: "r", BaselineHead: "h", Cycle: cycle,
				Disposition: DispositionConfirmed, FindingFingerprint: "fp-bare",
			}, nil
		},
		Fix: func(context.Context, Phase, int) (Evidence, error) {
			fixes++
			return Evidence{}, nil
		},
		Commit: func(context.Context, Evidence) (string, error) {
			t.Fatalf("commit must not run")
			return "", nil
		},
	}
	// NoProgressThreshold 1 → second cycle same fp stops
	c := testCampaign()
	c.NoProgressThreshold = 1
	c.MaxCycles = 3
	eng.Campaign = c
	res, err := eng.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if fixes != 0 {
		t.Fatalf("fixes = %d, want 0", fixes)
	}
	if res.Terminal != TerminalNoProgress && res.Terminal != TerminalCycleCap {
		t.Fatalf("terminal = %s, want no_progress or cycle_cap", res.Terminal)
	}
}

func TestEngineFixedWithoutPathsRejectsCommit(t *testing.T) {
	dir := t.TempDir()
	commits := 0
	eng := &Engine{
		Campaign: testCampaign(),
		Store:    NewStore(dir, "c-fixbare", "o"),
		Audit: func(ctx context.Context, phase Phase, cycle int) (Evidence, error) {
			return Evidence{
				Schema: EvidenceSchema, CampaignRun: "r", BaselineHead: "h", Cycle: cycle,
				Disposition: DispositionCandidate, FindingFingerprint: "fp1",
			}, nil
		},
		Confirm: func(ctx context.Context, phase Phase, cycle int) (Evidence, error) {
			return Evidence{
				Schema: EvidenceSchema, CampaignRun: "r", BaselineHead: "h", Cycle: cycle,
				Disposition: DispositionConfirmed, FindingFingerprint: "fp1",
				ChangedPathIDs: []string{"p1"}, VerifierRef: "true",
			}, nil
		},
		Fix: func(ctx context.Context, phase Phase, cycle int) (Evidence, error) {
			// Fixed alone without paths/verifier
			return Evidence{
				Schema: EvidenceSchema, CampaignRun: "r", BaselineHead: "h", Cycle: cycle,
				Disposition: DispositionFixed, FindingFingerprint: "fp1",
			}, nil
		},
		Commit: func(context.Context, Evidence) (string, error) {
			commits++
			return "deadbeef", nil
		},
	}
	res, err := eng.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if commits != 0 {
		t.Fatalf("commits = %d, want 0", commits)
	}
	if res.Terminal != TerminalVerificationFailed {
		t.Fatalf("terminal = %s, want verification_failed", res.Terminal)
	}
	if SuccessTerminal(res.Terminal) {
		t.Fatalf("verification_failed must not be SuccessTerminal")
	}
}

func TestIsCleanEvidenceRejectsFingerprintOnly(t *testing.T) {
	if isCleanEvidence(Evidence{FindingFingerprint: "fp1"}) {
		t.Fatal("fingerprint without disposition must not be clean")
	}
	if !isCleanEvidence(Evidence{}) {
		t.Fatal("empty evidence should be clean")
	}
	if !isCleanEvidence(Evidence{Disposition: DispositionRejected}) {
		t.Fatal("rejected without fingerprint should be clean")
	}
	if isCleanEvidence(Evidence{Disposition: DispositionCandidate, FindingFingerprint: "x"}) {
		t.Fatal("candidate must not be clean")
	}
	// ConfirmedFindings alone must not false-stop as clean (skeptic F-clean).
	if isCleanEvidence(Evidence{
		ConfirmedFindings: []FindingRef{{Fingerprint: "fp-list", Disposition: DispositionCandidate}},
	}) {
		t.Fatal("ConfirmedFindings without top-level disposition must not be clean")
	}
}

func TestEngineConfirmedFindingsOnlyNotCleanStop(t *testing.T) {
	dir := t.TempDir()
	confirms := 0
	c := testCampaign()
	c.CommitEnabled = false
	c.MaxCycles = 3
	c.CleanPassThreshold = 2
	eng := &Engine{
		Campaign: c,
		Store:    NewStore(dir, "c-cf", "o"),
		Audit: func(ctx context.Context, phase Phase, cycle int) (Evidence, error) {
			// Schema-valid envelope with ConfirmedFindings but empty top-level disposition.
			return Evidence{
				Schema: EvidenceSchema, CampaignRun: "r", BaselineHead: "h", Cycle: cycle,
				ConfirmedFindings: []FindingRef{{Fingerprint: "fp-list", Disposition: DispositionCandidate}},
			}, nil
		},
		Confirm: func(ctx context.Context, phase Phase, cycle int) (Evidence, error) {
			confirms++
			return Evidence{
				Schema: EvidenceSchema, CampaignRun: "r", BaselineHead: "h", Cycle: cycle,
				Disposition: DispositionRejected, FindingFingerprint: "fp-list",
			}, nil
		},
		Fix: func(context.Context, Phase, int) (Evidence, error) {
			t.Fatalf("fix must not run")
			return Evidence{}, nil
		},
		Commit: func(context.Context, Evidence) (string, error) {
			t.Fatalf("commit must not run")
			return "", nil
		},
	}
	res, err := eng.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if confirms == 0 {
		t.Fatal("Confirm must run; ConfirmedFindings-only must not TerminalClean without confirm")
	}
	if res.Terminal == TerminalClean {
		t.Fatalf("terminal=clean with ConfirmedFindings-only is a fail-open")
	}
}

func TestEngineEmptyAuditFingerprintBlocksInventedConfirm(t *testing.T) {
	dir := t.TempDir()
	fixes := 0
	commits := 0
	eng := &Engine{
		Campaign: testCampaign(),
		Store:    NewStore(dir, "c-invent", "o"),
		Audit: func(ctx context.Context, phase Phase, cycle int) (Evidence, error) {
			// Non-clean candidate with empty fingerprint — confirmer must not invent commit authority.
			return Evidence{
				Schema: EvidenceSchema, CampaignRun: "r", BaselineHead: "h", Cycle: cycle,
				Disposition: DispositionCandidate,
			}, nil
		},
		Confirm: func(ctx context.Context, phase Phase, cycle int) (Evidence, error) {
			return Evidence{
				Schema: EvidenceSchema, CampaignRun: "r", BaselineHead: "h", Cycle: cycle,
				Disposition: DispositionConfirmed, FindingFingerprint: "invented-fp",
				ChangedPathIDs: []string{"p1"}, VerifierRef: "true",
			}, nil
		},
		Fix: func(_ context.Context, _ Phase, cycle int) (Evidence, error) {
			fixes++
			return Evidence{
				Schema: EvidenceSchema, CampaignRun: "r", BaselineHead: "h", Cycle: cycle,
				Disposition: DispositionFixed, FindingFingerprint: "invented-fp",
				ChangedPathIDs: []string{"p1"}, VerifierRef: "true",
			}, nil
		},
		Commit: func(context.Context, Evidence) (string, error) {
			commits++
			return "deadbeef", nil
		},
	}
	res, err := eng.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if fixes != 0 || commits != 0 {
		t.Fatalf("invented confirm must not fix/commit; fixes=%d commits=%d terminal=%s", fixes, commits, res.Terminal)
	}
}

func TestEngineFixFingerprintMismatchRejectsCommit(t *testing.T) {
	dir := t.TempDir()
	commits := 0
	var commitFP string
	eng := &Engine{
		Campaign: testCampaign(),
		Store:    NewStore(dir, "c-fixfp", "o"),
		Audit: func(ctx context.Context, phase Phase, cycle int) (Evidence, error) {
			return Evidence{
				Schema: EvidenceSchema, CampaignRun: "r", BaselineHead: "h", Cycle: cycle,
				Disposition: DispositionCandidate, FindingFingerprint: "audit-fp",
			}, nil
		},
		Confirm: func(ctx context.Context, phase Phase, cycle int) (Evidence, error) {
			return Evidence{
				Schema: EvidenceSchema, CampaignRun: "r", BaselineHead: "h", Cycle: cycle,
				Disposition: DispositionConfirmed, FindingFingerprint: "audit-fp",
				ChangedPathIDs: []string{"p1"}, VerifierRef: "true",
			}, nil
		},
		Fix: func(ctx context.Context, phase Phase, cycle int) (Evidence, error) {
			return Evidence{
				Schema: EvidenceSchema, CampaignRun: "r", BaselineHead: "h", Cycle: cycle,
				Disposition: DispositionFixed, FindingFingerprint: "other-fp",
				ChangedPathIDs: []string{"p1"}, VerifierRef: "true",
			}, nil
		},
		Commit: func(_ context.Context, e Evidence) (string, error) {
			commits++
			commitFP = e.FindingFingerprint
			return "deadbeef", nil
		},
	}
	res, err := eng.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if commits != 0 {
		t.Fatalf("commits=%d commitFP=%s, want 0 (fix fp must match confirm)", commits, commitFP)
	}
	if res.Terminal != TerminalVerificationFailed {
		t.Fatalf("terminal=%s, want verification_failed", res.Terminal)
	}
	if !strings.Contains(res.Message, "fingerprint") {
		t.Fatalf("message=%q, want fingerprint mismatch", res.Message)
	}
}
