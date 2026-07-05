package policy

import (
	"errors"
	"testing"
)

func TestActionValidateRejectsUnknownKind(t *testing.T) {
	err := Action{Kind: "surprise"}.Validate()
	if !errors.Is(err, ErrInvalidAction) {
		t.Fatalf("Validate() error = %v, want ErrInvalidAction", err)
	}
}

func TestProtectActionRequiresProtectedKind(t *testing.T) {
	err := Action{Kind: ActionProtect}.Validate()
	if !errors.Is(err, ErrInvalidAction) {
		t.Fatalf("Validate() error = %v, want ErrInvalidAction", err)
	}
}

func TestDecisionRefIsStable(t *testing.T) {
	action := Action{
		Kind:          ActionProtect,
		ProtectedKind: ProtectedPush,
		RunID:         "run-1",
		Stamp:         "stamp-1",
		Vars:          map[string]any{"b": 2, "a": 1},
	}
	left := StableDecisionRef("noop", action, true, "allowed")
	right := StableDecisionRef("noop", action, true, "allowed")
	if left == "" || left != right {
		t.Fatalf("StableDecisionRef() = %q and %q, want stable non-empty refs", left, right)
	}
}
