// Package auditcampaign tests durable campaign state.
// Plan: WS15.
package auditcampaign

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestStateMonotonicTransitions(t *testing.T) {
	if err := Transition(PhaseCreated, PhaseAuditing); err != nil {
		t.Fatalf("created->auditing: %v", err)
	}
	if err := Transition(PhaseAuditing, PhaseCommitting); err == nil {
		t.Fatalf("want illegal transition auditing->committing")
	}
	if !errors.Is(Transition(PhaseTerminal, PhaseAuditing), ErrIllegalTransition) {
		t.Fatalf("want ErrIllegalTransition from terminal")
	}
}

func TestStateRejectsResumeOnChangedHead(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir, "camp-1", "owner-a")
	if err := s.Lock(); err != nil {
		t.Fatalf("Lock: %v", err)
	}
	defer s.Unlock()
	snap := Snapshot{
		Schema:       StateSchema,
		CampaignID:   "camp-1",
		Phase:        PhaseAuditing,
		BaselineHead: "head-a",
		BranchRef:    "branch-a",
		OwnerID:      "owner-a",
	}
	if err := s.Save(snap); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := s.ResumePreconditions("branch-a", "head-b", "owner-a"); err == nil {
		t.Fatalf("want head mismatch rejection")
	}
	if err := s.ResumePreconditions("branch-a", "head-a", "owner-a"); err != nil {
		t.Fatalf("ResumePreconditions: %v", err)
	}
}

func TestStateLockSerialization(t *testing.T) {
	dir := t.TempDir()
	a := NewStore(dir, "camp-1", "owner-a")
	b := NewStore(dir, "camp-1", "owner-b")
	if err := a.Lock(); err != nil {
		t.Fatalf("a.Lock: %v", err)
	}
	if err := b.Lock(); err == nil {
		t.Fatalf("want b.Lock to fail while a holds lock")
	}
	if err := a.Unlock(); err != nil {
		t.Fatalf("Unlock: %v", err)
	}
	if err := b.Lock(); err != nil {
		t.Fatalf("b.Lock after unlock: %v", err)
	}
	_ = filepath.Join(dir, ".ai", "runs", "camp-1")
}

func TestStateNoProgressOnDuplicateFingerprint(t *testing.T) {
	snap := Snapshot{}
	if RecordFingerprint(&snap, "fp1") {
		t.Fatalf("first fingerprint should not be duplicate")
	}
	if !RecordFingerprint(&snap, "fp1") {
		t.Fatalf("second identical fingerprint should be duplicate")
	}
	if snap.NoProgressCount != 1 {
		t.Fatalf("NoProgressCount = %d, want 1", snap.NoProgressCount)
	}
}

func TestApplyTransitionPersists(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir, "camp-2", "owner")
	_ = s.Lock()
	defer s.Unlock()
	if err := s.Save(Snapshot{CampaignID: "camp-2", Phase: PhaseCreated, OwnerID: "owner"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	snap, err := s.ApplyTransition(PhaseAuditing)
	if err != nil {
		t.Fatalf("ApplyTransition: %v", err)
	}
	if snap.Phase != PhaseAuditing {
		t.Fatalf("phase = %s", snap.Phase)
	}
}
