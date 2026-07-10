package policy

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestNoopAllowsAll(t *testing.T) {
	dir := t.TempDir()
	provider := Noop{AuditPath: filepath.Join(dir, ".ai", "audit.jsonl")}
	decision, err := provider.Decide(context.Background(), Action{Kind: ActionProtect, ProtectedKind: ProtectedCommit})
	if err != nil {
		t.Fatalf("Decide() error = %v, want nil", err)
	}
	if !decision.Allowed || decision.Ref == "" {
		t.Fatalf("Decide() = %+v, want allowed with ref", decision)
	}
}

func TestNoopRejectsInvalidAction(t *testing.T) {
	provider := Noop{AuditPath: filepath.Join(t.TempDir(), ".ai", "audit.jsonl")}
	_, err := provider.Decide(context.Background(), Action{Kind: "unknown"})
	if !errors.Is(err, ErrInvalidAction) {
		t.Fatalf("Decide() error = %v, want ErrInvalidAction", err)
	}
}

func TestNoopRecordsToAuditLog(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".ai", "audit.jsonl")
	provider := Noop{AuditPath: path}
	if err := provider.Record(context.Background(), Event{Kind: "test", Payload: map[string]any{"ok": true}}); err != nil {
		t.Fatalf("Record() error = %v, want nil", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if len(data) == 0 {
		t.Fatalf("audit log is empty, want JSONL event")
	}
}

func TestNoopRecordsToConfiguredAuditLogUnderAI(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".ai", "governance", "hook.jsonl")
	provider := Noop{AuditPath: path}
	if err := provider.Record(context.Background(), Event{Kind: "test"}); err != nil {
		t.Fatalf("Record() error = %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("configured audit log: %v", err)
	}
}

func TestNoopDecideAppendsDecisionEvent(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".ai", "audit.jsonl")
	provider := Noop{AuditPath: path}
	decision, err := provider.Decide(context.Background(), Action{Kind: ActionReview, Step: "review"})
	if err != nil {
		t.Fatalf("Decide() error = %v, want nil", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	var event Event
	if err := json.Unmarshal(data[:len(data)-1], &event); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if event.Kind != "decision-made" || event.Payload["ref"] != decision.Ref {
		t.Fatalf("event = %+v, decision = %+v; want matching decision-made ref", event, decision)
	}
}

func TestNoopAuditPathStaysUnderAI(t *testing.T) {
	provider := Noop{AuditPath: filepath.Join(t.TempDir(), "audit.jsonl")}
	err := provider.Record(context.Background(), Event{Kind: "test"})
	if err == nil {
		t.Fatalf("Record() error = nil, want audit path rejection")
	}
}

func TestNoopRejectsAISymlinkEscape(t *testing.T) {
	repo := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(repo, ".ai")); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}
	provider := Noop{AuditPath: filepath.Join(repo, ".ai", "audit.jsonl")}
	if err := provider.Record(context.Background(), Event{Kind: "test"}); err == nil {
		t.Fatal("Record() error = nil, want symlink rejection")
	}
	if _, err := os.Stat(filepath.Join(outside, "audit.jsonl")); !os.IsNotExist(err) {
		t.Fatalf("outside audit log exists or Stat failed: %v", err)
	}
}
