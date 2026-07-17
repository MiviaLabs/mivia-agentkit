// Package auditcampaign implements the supervised audit-repair campaign runtime.
// Plan: WS15. PRD: finite campaign engine.
package auditcampaign

import (
	"context"
	"fmt"
	"time"

	"github.com/MiviaLabs/mivia-agentkit/internal/config"
)

// AdapterFunc runs one campaign phase and returns evidence.
type AdapterFunc func(ctx context.Context, phase Phase, cycle int) (Evidence, error)

// CommitFunc performs a scoped commit for confirmed findings.
type CommitFunc func(ctx context.Context, e Evidence) (string, error)

// Engine runs finite audit/confirm/fix/verify/commit cycles.
type Engine struct {
	Campaign config.Campaign
	Store    *Store
	Audit    AdapterFunc
	Confirm  AdapterFunc
	Fix      AdapterFunc
	Commit   CommitFunc
	Clock    func() time.Time
	// Continuous is true when the operator requested --continuous mode.
	Continuous bool
	// InteractiveContinuous is true only when operator confirmed TTY continuous.
	InteractiveContinuous bool
	// SelfConfirm is true when auditor==confirmer; rejected for commit campaigns.
	SelfConfirm bool
}

// Result is a redacted campaign outcome.
type Result struct {
	Terminal TerminalReason `json:"terminal"`
	Cycles   int            `json:"cycles"`
	Commits  int            `json:"commits"`
	Message  string         `json:"message"`
}

// Run executes the campaign until a terminal reason.
func (e *Engine) Run(ctx context.Context) (Result, error) {
	if e.Campaign.CommitEnabled && e.SelfConfirm {
		return Result{}, fmt.Errorf("commit-capable campaign rejects self-confirmation")
	}
	// Continuous mode requires interactive operator confirmation when the campaign demands it.
	// Finite non-continuous runs do not require a TTY.
	if e.Continuous {
		req := true
		if e.Campaign.RequireInteractiveContinuous != nil {
			req = *e.Campaign.RequireInteractiveContinuous
		}
		if req && !e.InteractiveContinuous {
			return Result{}, fmt.Errorf("continuous requires interactive operator confirmation")
		}
	}
	now := e.now
	start := now()
	maxDur, err := time.ParseDuration(e.Campaign.MaxDuration)
	if err != nil {
		return Result{}, err
	}
	maxCycles := e.Campaign.MaxCycles
	if maxCycles <= 0 {
		maxCycles = 1
	}
	cleanNeed := e.Campaign.CleanPassThreshold
	if cleanNeed < 2 {
		cleanNeed = 2
	}

	snap, err := e.ensureSnapshot()
	if err != nil {
		return Result{}, err
	}
	commits := 0
	for snap.CyclesUsed < maxCycles {
		if err := ctx.Err(); err != nil {
			return e.terminate(snap, TerminalCancelled, "cancelled", commits)
		}
		if now().Sub(start) > maxDur {
			return e.terminate(snap, TerminalDurationCap, "duration cap", commits)
		}
		snap.CyclesUsed++
		snap.Cycle = snap.CyclesUsed

		// Audit
		if err := Transition(snap.Phase, PhaseAuditing); err == nil || snap.Phase == PhaseCreated || snap.Phase == PhaseCompletedCycle {
			snap.Phase = PhaseAuditing
		}
		_ = e.Store.Save(snap)
		ev, err := e.Audit(ctx, PhaseAuditing, snap.Cycle)
		if err != nil {
			return e.terminate(snap, TerminalVerificationFailed, err.Error(), commits)
		}
		if ev.Disposition == "" || ev.Disposition == DispositionRejected {
			// treat empty as clean audit for stop predicate when ResidualRisk none path
			if isCleanEvidence(ev) {
				snap.CleanStreak++
				if snap.CleanStreak >= cleanNeed {
					return e.terminate(snap, TerminalClean, "two consecutive clean audits", commits)
				}
				snap.Phase = PhaseCompletedCycle
				_ = e.Store.Save(snap)
				continue
			}
		}
		snap.CleanStreak = 0

		// Candidate-only: do not fix/commit
		if ev.Disposition == DispositionCandidate || !ev.CommitEligible() {
			if RecordFingerprint(&snap, ev.FindingFingerprint) {
				if snap.NoProgressCount >= e.Campaign.NoProgressThreshold {
					return e.terminate(snap, TerminalNoProgress, "duplicate fingerprint", commits)
				}
			}
			snap.Phase = PhaseCompletedCycle
			_ = e.Store.Save(snap)
			continue
		}

		// Confirm
		snap.Phase = PhaseConfirming
		_ = e.Store.Save(snap)
		cev, err := e.Confirm(ctx, PhaseConfirming, snap.Cycle)
		if err != nil {
			return e.terminate(snap, TerminalVerificationFailed, err.Error(), commits)
		}
		if cev.Disposition != DispositionConfirmed && !cev.CommitEligible() {
			snap.Phase = PhaseCompletedCycle
			_ = e.Store.Save(snap)
			continue
		}

		// Fix + verify + commit once
		snap.Phase = PhaseFixing
		_ = e.Store.Save(snap)
		fev, err := e.Fix(ctx, PhaseFixing, snap.Cycle)
		if err != nil {
			return e.terminate(snap, TerminalVerificationFailed, err.Error(), commits)
		}
		if !fev.CommitEligible() && fev.Disposition != DispositionFixed {
			return e.terminate(snap, TerminalVerificationFailed, "fix did not produce commit-eligible evidence", commits)
		}
		snap.Phase = PhaseVerifying
		_ = e.Store.Save(snap)
		snap.Phase = PhasePreflighting
		_ = e.Store.Save(snap)
		snap.Phase = PhaseCommitting
		_ = e.Store.Save(snap)
		if e.Commit == nil {
			return e.terminate(snap, TerminalCommitFailed, "commit function missing", commits)
		}
		if !e.Campaign.CommitEnabled {
			return e.terminate(snap, TerminalCommitFailed, "commit_enabled is false", commits)
		}
		sha, err := e.Commit(ctx, fev)
		if err != nil {
			return e.terminate(snap, TerminalCommitFailed, err.Error(), commits)
		}
		commits++
		snap.LastCommitSHA = sha
		snap.BaselineHead = sha
		snap.Phase = PhaseCompletedCycle
		_ = e.Store.Save(snap)
		// schedule next audit only after recorded commit (loop continues)
	}
	return e.terminate(snap, TerminalCycleCap, "cycle cap", commits)
}

func (e *Engine) ensureSnapshot() (Snapshot, error) {
	if e.Store == nil {
		return Snapshot{}, fmt.Errorf("store required")
	}
	if err := e.Store.Lock(); err != nil {
		return Snapshot{}, err
	}
	snap, err := e.Store.Load()
	if err != nil {
		snap = Snapshot{
			Schema:     StateSchema,
			CampaignID: e.Store.ID,
			Phase:      PhaseCreated,
			OwnerID:    e.Store.Owner,
			MaxCycles:  e.Campaign.MaxCycles,
		}
		if err := e.Store.Save(snap); err != nil {
			return Snapshot{}, err
		}
	}
	return snap, nil
}

func (e *Engine) terminate(snap Snapshot, reason TerminalReason, msg string, commits int) (Result, error) {
	snap.Phase = PhaseTerminal
	snap.TerminalReason = reason
	_ = e.Store.Save(snap)
	_ = e.Store.Unlock()
	return Result{Terminal: reason, Cycles: snap.CyclesUsed, Commits: commits, Message: msg}, nil
}

func isCleanEvidence(e Evidence) bool {
	if e.Disposition == DispositionCandidate || e.Disposition == DispositionConfirmed {
		return false
	}
	if e.FindingFingerprint != "" && e.Disposition != DispositionRejected && e.Disposition != "" {
		return false
	}
	return e.Disposition == "" || e.Disposition == DispositionRejected
}

func (e *Engine) now() time.Time {
	if e.Clock != nil {
		return e.Clock()
	}
	return time.Now()
}
