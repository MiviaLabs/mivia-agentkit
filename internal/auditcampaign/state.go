// Package auditcampaign implements the supervised audit-repair campaign runtime.
// Plan: WS15. PRD: safe state machine, redacted durable state.
package auditcampaign

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Phase is a campaign state-machine phase.
type Phase string

const (
	PhaseCreated        Phase = "created"
	PhaseAuditing       Phase = "auditing"
	PhaseConfirming     Phase = "confirming"
	PhaseFixing         Phase = "fixing"
	PhaseVerifying      Phase = "verifying"
	PhasePreflighting   Phase = "preflighting"
	PhaseCommitting     Phase = "committing"
	PhaseCompletedCycle Phase = "completed_cycle"
	PhaseTerminal       Phase = "terminal"
)

// TerminalReason ends a campaign.
type TerminalReason string

const (
	TerminalClean              TerminalReason = "clean"
	TerminalNoProgress         TerminalReason = "no_progress"
	TerminalCycleCap           TerminalReason = "cycle_cap"
	TerminalDurationCap        TerminalReason = "duration_cap"
	TerminalVerificationFailed TerminalReason = "verification_failed"
	TerminalPolicyDenied       TerminalReason = "policy_denied"
	TerminalCommitFailed       TerminalReason = "commit_failed"
	TerminalConflictOrDirty    TerminalReason = "conflict_or_dirty"
	TerminalCancelled          TerminalReason = "cancelled"
	TerminalMalformedState     TerminalReason = "malformed_state"
	TerminalUnauthorizedHead   TerminalReason = "unauthorized_head_advance"
)

// ErrIllegalTransition is returned for non-monotonic transitions.
var ErrIllegalTransition = errors.New("illegal campaign state transition")

// ErrLocked is returned when another owner holds the campaign lock.
var ErrLocked = errors.New("campaign state is locked by another owner")

// Snapshot is redacted durable campaign state.
type Snapshot struct {
	Schema          string         `json:"schema"`
	CampaignID      string         `json:"campaign_id"`
	Phase           Phase          `json:"phase"`
	Cycle           int            `json:"cycle"`
	CleanStreak     int            `json:"clean_streak"`
	NoProgressCount int            `json:"no_progress_count"`
	BaselineHead    string         `json:"baseline_head"`
	BranchRef       string         `json:"branch_ref"`
	OwnerID         string         `json:"owner_id"`
	ResumeToken     string         `json:"resume_token"`
	TerminalReason  TerminalReason `json:"terminal_reason,omitempty"`
	Fingerprints    []string       `json:"fingerprints,omitempty"`
	LastCommitSHA   string         `json:"last_commit_sha,omitempty"`
	UpdatedAt       string         `json:"updated_at"`
	// Budget is cumulative remaining cycle/duration budget.
	MaxCycles      int   `json:"max_cycles"`
	CyclesUsed     int   `json:"cycles_used"`
	MaxDurationMS  int64 `json:"max_duration_ms"`
	DurationUsedMS int64 `json:"duration_used_ms"`
}

// StateSchema versions durable state.
const StateSchema = "mivia-agent-campaign-state/v1"

// Store persists redacted campaign state under .ai/runs/<id>/.
type Store struct {
	mu     sync.Mutex
	root   string // repo root
	ID     string
	Owner  string
	locked bool
}

// NewStore creates a campaign state store under repo/.ai/runs/<campaignID>.
func NewStore(repo, campaignID, owner string) *Store {
	return &Store{root: repo, ID: campaignID, Owner: owner}
}

func (s *Store) dir() string {
	return filepath.Join(s.root, ".ai", "runs", s.ID)
}

func (s *Store) snapshotPath() string {
	return filepath.Join(s.dir(), "campaign-state.json")
}

func (s *Store) lockPath() string {
	return filepath.Join(s.dir(), "campaign.lock")
}

// Lock acquires an exclusive owner lock.
func (s *Store) Lock() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(s.dir(), 0o755); err != nil {
		return err
	}
	if b, err := os.ReadFile(s.lockPath()); err == nil {
		if string(b) != s.Owner && string(b) != "" {
			return fmt.Errorf("%w: %s", ErrLocked, string(b))
		}
	}
	if err := os.WriteFile(s.lockPath(), []byte(s.Owner), 0o644); err != nil {
		return err
	}
	s.locked = true
	return nil
}

// Unlock releases the lock if owned.
func (s *Store) Unlock() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := os.ReadFile(s.lockPath())
	if err != nil {
		return nil
	}
	if string(b) != s.Owner {
		return fmt.Errorf("%w: not owner", ErrLocked)
	}
	_ = os.Remove(s.lockPath())
	s.locked = false
	return nil
}

// Load reads the snapshot.
func (s *Store) Load() (Snapshot, error) {
	b, err := os.ReadFile(s.snapshotPath())
	if err != nil {
		return Snapshot{}, err
	}
	var snap Snapshot
	if err := json.Unmarshal(b, &snap); err != nil {
		return Snapshot{}, fmt.Errorf("malformed state: %w", err)
	}
	if snap.Schema != StateSchema {
		return Snapshot{}, fmt.Errorf("%w: schema", ErrIllegalTransition)
	}
	return snap, nil
}

// Save writes the snapshot atomically.
func (s *Store) Save(snap Snapshot) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(s.dir(), 0o755); err != nil {
		return err
	}
	snap.Schema = StateSchema
	snap.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	b, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.snapshotPath() + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.snapshotPath())
}

// Transition validates and applies a phase transition.
func Transition(from, to Phase) error {
	if from == PhaseTerminal {
		return fmt.Errorf("%w: already terminal", ErrIllegalTransition)
	}
	allowed := map[Phase][]Phase{
		PhaseCreated:        {PhaseAuditing, PhaseTerminal},
		PhaseAuditing:       {PhaseConfirming, PhaseCompletedCycle, PhaseTerminal},
		PhaseConfirming:     {PhaseFixing, PhaseCompletedCycle, PhaseTerminal},
		PhaseFixing:         {PhaseVerifying, PhaseTerminal},
		PhaseVerifying:      {PhasePreflighting, PhaseTerminal},
		PhasePreflighting:   {PhaseCommitting, PhaseTerminal},
		PhaseCommitting:     {PhaseCompletedCycle, PhaseTerminal},
		PhaseCompletedCycle: {PhaseAuditing, PhaseTerminal},
	}
	for _, next := range allowed[from] {
		if next == to {
			return nil
		}
	}
	return fmt.Errorf("%w: %s -> %s", ErrIllegalTransition, from, to)
}

// ApplyTransition loads, transitions, and saves state.
func (s *Store) ApplyTransition(to Phase) (Snapshot, error) {
	snap, err := s.Load()
	if err != nil {
		return Snapshot{}, err
	}
	if err := Transition(snap.Phase, to); err != nil {
		return Snapshot{}, err
	}
	snap.Phase = to
	if err := s.Save(snap); err != nil {
		return Snapshot{}, err
	}
	return snap, nil
}

// ResumePreconditions checks branch/HEAD/owner/terminal consistency.
func (s *Store) ResumePreconditions(wantBranch, wantHead, wantOwner string) error {
	snap, err := s.Load()
	if err != nil {
		return err
	}
	if snap.Phase == PhaseTerminal {
		return fmt.Errorf("%w: terminal campaign", ErrIllegalTransition)
	}
	if snap.OwnerID != "" && snap.OwnerID != wantOwner {
		return fmt.Errorf("%w: owner mismatch", ErrLocked)
	}
	if snap.BranchRef != "" && snap.BranchRef != wantBranch {
		return fmt.Errorf("resume rejected: branch mismatch")
	}
	if snap.BaselineHead != "" && snap.BaselineHead != wantHead {
		return fmt.Errorf("resume rejected: head mismatch")
	}
	return nil
}

// RecordFingerprint tracks progress; returns true if duplicate (no progress).
func RecordFingerprint(snap *Snapshot, fp string) bool {
	for _, existing := range snap.Fingerprints {
		if existing == fp {
			snap.NoProgressCount++
			return true
		}
	}
	snap.Fingerprints = append(snap.Fingerprints, fp)
	return false
}
