// Package auditcampaign implements the supervised audit-repair campaign runtime.
// Plan: WS15. PRD: finite campaign engine.
package auditcampaign

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/MiviaLabs/mivia-agentkit/internal/config"
)

// AdapterFunc runs one campaign phase and returns evidence.
// prior carries the previous phase's evidence (zero for audit; audit for confirm; confirm for fix).
type AdapterFunc func(ctx context.Context, phase Phase, cycle int, prior Evidence) (Evidence, error)

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
	// BaselineHead seeds resume/HEAD checks when the store has no baseline yet.
	BaselineHead string
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

	snap, err := e.ensureSnapshot(maxCycles, maxDur)
	if err != nil {
		return Result{}, err
	}
	// Cumulative duration across resumes: budget already consumed is wall-clock used.
	usedBefore := time.Duration(snap.DurationUsedMS) * time.Millisecond
	commits := 0
	for snap.CyclesUsed < maxCycles {
		if err := ctx.Err(); err != nil {
			return e.terminate(snap, TerminalCancelled, "cancelled", commits, start, usedBefore)
		}
		elapsed := usedBefore + now().Sub(start)
		if elapsed > maxDur {
			return e.terminate(snap, TerminalDurationCap, "duration cap", commits, start, usedBefore)
		}
		snap.CyclesUsed++
		snap.Cycle = snap.CyclesUsed

		// Audit
		if err := e.setPhase(&snap, PhaseAuditing); err != nil {
			return e.terminate(snap, TerminalMalformedState, err.Error(), commits, start, usedBefore)
		}
		ev, err := e.Audit(ctx, PhaseAuditing, snap.Cycle, Evidence{})
		if err != nil {
			return e.terminate(snap, classifyAdapterErr(err), err.Error(), commits, start, usedBefore)
		}
		if isCleanEvidence(ev) {
			snap.CleanStreak++
			e.persistDuration(&snap, start, usedBefore)
			if snap.CleanStreak >= cleanNeed {
				return e.terminate(snap, TerminalClean, "two consecutive clean audits", commits, start, usedBefore)
			}
			if err := e.setPhase(&snap, PhaseCompletedCycle); err != nil {
				return e.terminate(snap, TerminalMalformedState, err.Error(), commits, start, usedBefore)
			}
			continue
		}
		snap.CleanStreak = 0

		// Non-clean findings always go through the independent confirmer.
		// Candidates are the normal path; pre-confirmed audit evidence is re-checked.
		if err := e.setPhase(&snap, PhaseConfirming); err != nil {
			return e.terminate(snap, TerminalMalformedState, err.Error(), commits, start, usedBefore)
		}
		cev, err := e.Confirm(ctx, PhaseConfirming, snap.Cycle, ev)
		if err != nil {
			return e.terminate(snap, classifyAdapterErr(err), err.Error(), commits, start, usedBefore)
		}
		// Confirm must be fully commit-eligible; bare confirmed without paths/verifier/fingerprint stops.
		if !cev.CommitEligible() {
			fp := cev.FindingFingerprint
			if fp == "" {
				fp = auditFindingFingerprint(ev)
			}
			if fp != "" && RecordFingerprint(&snap, fp) {
				if snap.NoProgressCount >= e.Campaign.NoProgressThreshold {
					return e.terminate(snap, TerminalNoProgress, "duplicate fingerprint", commits, start, usedBefore)
				}
			}
			e.persistDuration(&snap, start, usedBefore)
			if err := e.setPhase(&snap, PhaseCompletedCycle); err != nil {
				return e.terminate(snap, TerminalMalformedState, err.Error(), commits, start, usedBefore)
			}
			continue
		}
		// Bind confirmer finding to audit fingerprint. Confirm cannot invent a
		// commit-eligible finding that the auditor never fingerprinted.
		auditFP := auditFindingFingerprint(ev)
		if auditFP == "" || cev.FindingFingerprint != auditFP {
			fp := auditFP
			if fp == "" {
				fp = cev.FindingFingerprint
			}
			if fp != "" && RecordFingerprint(&snap, fp) {
				if snap.NoProgressCount >= e.Campaign.NoProgressThreshold {
					return e.terminate(snap, TerminalNoProgress, "duplicate fingerprint", commits, start, usedBefore)
				}
			}
			e.persistDuration(&snap, start, usedBefore)
			if err := e.setPhase(&snap, PhaseCompletedCycle); err != nil {
				return e.terminate(snap, TerminalMalformedState, err.Error(), commits, start, usedBefore)
			}
			continue
		}

		// Audit/confirm-only campaigns: do not fix or commit; do not fail closed as commit_failed.
		if !e.Campaign.CommitEnabled {
			e.persistDuration(&snap, start, usedBefore)
			if err := e.setPhase(&snap, PhaseCompletedCycle); err != nil {
				return e.terminate(snap, TerminalMalformedState, err.Error(), commits, start, usedBefore)
			}
			continue
		}

		// Fix + verify + commit once
		if err := e.setPhase(&snap, PhaseFixing); err != nil {
			return e.terminate(snap, TerminalMalformedState, err.Error(), commits, start, usedBefore)
		}
		fev, err := e.Fix(ctx, PhaseFixing, snap.Cycle, cev)
		if err != nil {
			return e.terminate(snap, classifyAdapterErr(err), err.Error(), commits, start, usedBefore)
		}
		if !fixCommitReady(fev) {
			return e.terminate(snap, TerminalVerificationFailed, "fix did not produce commit-ready evidence", commits, start, usedBefore)
		}
		// Fix evidence must rebind to the same confirmed fingerprint (no swap).
		if fev.FindingFingerprint != cev.FindingFingerprint {
			return e.terminate(snap, TerminalVerificationFailed, "fix fingerprint does not match confirmed finding", commits, start, usedBefore)
		}
		if err := e.setPhase(&snap, PhaseVerifying); err != nil {
			return e.terminate(snap, TerminalMalformedState, err.Error(), commits, start, usedBefore)
		}
		if err := e.setPhase(&snap, PhasePreflighting); err != nil {
			return e.terminate(snap, TerminalMalformedState, err.Error(), commits, start, usedBefore)
		}
		if err := e.setPhase(&snap, PhaseCommitting); err != nil {
			return e.terminate(snap, TerminalMalformedState, err.Error(), commits, start, usedBefore)
		}
		if e.Commit == nil {
			return e.terminate(snap, TerminalCommitFailed, "commit function missing", commits, start, usedBefore)
		}
		sha, err := e.Commit(ctx, fev)
		if err != nil {
			reason := TerminalCommitFailed
			if strings.Contains(err.Error(), string(TerminalUnauthorizedHead)) {
				reason = TerminalUnauthorizedHead
			} else if strings.Contains(err.Error(), "policy") {
				reason = TerminalPolicyDenied
			} else if strings.Contains(err.Error(), "verifier") {
				reason = TerminalVerificationFailed
			}
			return e.terminate(snap, reason, redactCommitMessage(err.Error()), commits, start, usedBefore)
		}
		commits++
		snap.LastCommitSHA = sha
		snap.BaselineHead = sha
		e.persistDuration(&snap, start, usedBefore)
		if err := e.setPhase(&snap, PhaseCompletedCycle); err != nil {
			return e.terminate(snap, TerminalMalformedState, err.Error(), commits, start, usedBefore)
		}
		// schedule next audit only after recorded commit (loop continues)
	}
	return e.terminate(snap, TerminalCycleCap, "cycle cap", commits, start, usedBefore)
}

// persistDuration records cumulative wall time so resume budgets are honest.
func (e *Engine) persistDuration(snap *Snapshot, start time.Time, usedBefore time.Duration) {
	snap.DurationUsedMS = (usedBefore + e.now().Sub(start)).Milliseconds()
}

func (e *Engine) ensureSnapshot(maxCycles int, maxDur time.Duration) (Snapshot, error) {
	if e.Store == nil {
		return Snapshot{}, fmt.Errorf("store required")
	}
	if err := e.Store.Lock(); err != nil {
		return Snapshot{}, err
	}
	snap, err := e.Store.Load()
	if err != nil {
		snap = Snapshot{
			Schema:         StateSchema,
			CampaignID:     e.Store.ID,
			Phase:          PhaseCreated,
			OwnerID:        e.Store.Owner,
			MaxCycles:      maxCycles,
			MaxDurationMS:  maxDur.Milliseconds(),
			BaselineHead:   e.BaselineHead,
		}
		if err := e.Store.Save(snap); err != nil {
			return Snapshot{}, err
		}
		return snap, nil
	}
	if snap.Phase == PhaseTerminal {
		return Snapshot{}, fmt.Errorf("%w: terminal campaign", ErrIllegalTransition)
	}
	// Resume restarts at a cycle boundary: mid-phase snapshots continue with the
	// next audit using remaining cycle/duration budget (no silent re-open of terminal).
	if snap.Phase != PhaseCreated && snap.Phase != PhaseCompletedCycle {
		snap.Phase = PhaseCompletedCycle
	}
	if snap.MaxCycles <= 0 {
		snap.MaxCycles = maxCycles
	}
	if snap.MaxDurationMS <= 0 {
		snap.MaxDurationMS = maxDur.Milliseconds()
	}
	if snap.BaselineHead == "" && e.BaselineHead != "" {
		snap.BaselineHead = e.BaselineHead
	}
	if err := e.Store.Save(snap); err != nil {
		return Snapshot{}, err
	}
	return snap, nil
}

func (e *Engine) setPhase(snap *Snapshot, to Phase) error {
	if err := Transition(snap.Phase, to); err != nil {
		// Allow Created/CompletedCycle → Auditing and CompletedCycle stays consistent.
		if snap.Phase == PhaseCreated || snap.Phase == PhaseCompletedCycle {
			if to == PhaseAuditing || to == PhaseTerminal {
				snap.Phase = to
				return e.Store.Save(*snap)
			}
		}
		// Mid-cycle phase jumps that are sequential within a cycle are applied when Transition allows.
		return err
	}
	snap.Phase = to
	return e.Store.Save(*snap)
}

func (e *Engine) terminate(snap Snapshot, reason TerminalReason, msg string, commits int, start time.Time, usedBefore time.Duration) (Result, error) {
	elapsed := usedBefore + e.now().Sub(start)
	snap.DurationUsedMS = elapsed.Milliseconds()
	snap.Phase = PhaseTerminal
	snap.TerminalReason = reason
	_ = e.Store.Save(snap)
	_ = e.Store.Unlock()
	return Result{Terminal: reason, Cycles: snap.CyclesUsed, Commits: commits, Message: msg}, nil
}

// isCleanEvidence reports whether audit evidence is a clean pass (no finding).
// Fingerprint without disposition is NOT clean (malformed partial finding).
// ConfirmedFindings list entries are also non-clean even when top-level disposition is empty.
func isCleanEvidence(e Evidence) bool {
	if e.FindingFingerprint != "" {
		return false
	}
	if len(e.ConfirmedFindings) > 0 {
		return false
	}
	switch e.Disposition {
	case DispositionCandidate, DispositionConfirmed, DispositionDuplicate, DispositionFixed:
		return false
	case DispositionRejected, "":
		return true
	default:
		return false
	}
}

// auditFindingFingerprint returns the auditor-bound finding id for confirm binding.
// Prefer top-level finding_fingerprint; else first ConfirmedFindings fingerprint.
func auditFindingFingerprint(e Evidence) string {
	if e.FindingFingerprint != "" {
		return e.FindingFingerprint
	}
	for _, f := range e.ConfirmedFindings {
		if f.Fingerprint != "" {
			return f.Fingerprint
		}
	}
	return ""
}

// fixCommitReady requires fixed disposition plus fingerprint, path IDs, and verifier ref.
// DispositionFixed alone is not sufficient (no path/verifier proof).
func fixCommitReady(e Evidence) bool {
	if e.Disposition != DispositionFixed {
		return false
	}
	if e.FindingFingerprint == "" || !isOpaqueID(e.FindingFingerprint) {
		return false
	}
	return e.VerifierRef != "" && len(e.ChangedPathIDs) > 0
}

func classifyAdapterErr(err error) TerminalReason {
	if err == nil {
		return TerminalVerificationFailed
	}
	msg := err.Error()
	if strings.Contains(msg, string(TerminalUnauthorizedHead)) {
		return TerminalUnauthorizedHead
	}
	return TerminalVerificationFailed
}

// redactCommitMessage keeps Result.Message free of raw subprocess bodies.
func redactCommitMessage(msg string) string {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return "commit failed"
	}
	// Keep short stable prefixes; drop long CombinedOutput tails.
	for _, prefix := range []string{
		"scoped commit rejected: verifier failed",
		"scoped commit rejected: stamp",
		"scoped commit rejected: policy",
		"scoped commit rejected: head",
		"scoped commit rejected: index",
		"scoped commit rejected: git add",
		"scoped commit rejected: commit",
		"scoped commit rejected",
		string(TerminalUnauthorizedHead),
		"policy denied",
		"commit_enabled is false",
	} {
		if strings.HasPrefix(msg, prefix) || strings.Contains(msg, prefix) {
			return prefix
		}
	}
	if len(msg) > 160 {
		return msg[:160]
	}
	return msg
}

// SuccessTerminal reports whether a terminal reason is an expected finite stop (exit 0).
func SuccessTerminal(r TerminalReason) bool {
	switch r {
	case TerminalClean, TerminalCycleCap, TerminalDurationCap, TerminalNoProgress:
		return true
	default:
		return false
	}
}

func (e *Engine) now() time.Time {
	if e.Clock != nil {
		return e.Clock()
	}
	return time.Now()
}
