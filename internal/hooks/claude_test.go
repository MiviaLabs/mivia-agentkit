// Package hooks implements protected-action hook decisions.
// Plan: WS5. PRD: FR-7.1, FR-8.1, FR-8.2, FR-8.3.
package hooks

import (
	"errors"
	"testing"
)

func TestClaudePreToolUseDenyShape(t *testing.T) {
	got := mustJSON(t, claudeDocument(EventPreToolUse, "blocked"))
	want := "{\n  \"hookSpecificOutput\": {\n    \"hookEventName\": \"PreToolUse\",\n    \"permissionDecision\": \"deny\",\n    \"permissionDecisionReason\": \"blocked\"\n  }\n}"
	if got != want {
		t.Fatalf("claude deny shape:\ngot  %s\nwant %s", got, want)
	}
	err := EmitClaude(t.Context(), EventPreToolUse, Payload{}, Outcome{Allow: false, Reason: "blocked"})
	var exit ClaudeExitError
	if !errors.As(err, &exit) || exit.Code != 2 {
		t.Fatalf("EmitClaude() error = %#v; want ClaudeExitError code 2", err)
	}
}

func TestClaudeStopBlocksDoneWithoutStamp(t *testing.T) {
	got := mustJSON(t, claudeDocument(EventStop, "stamp required"))
	want := "{\n  \"continue\": false,\n  \"stopReason\": \"stamp required\"\n}"
	if got != want {
		t.Fatalf("claude stop shape:\ngot  %s\nwant %s", got, want)
	}
}

func TestClaudeAllowEmitsMinimal(t *testing.T) {
	if err := EmitClaude(t.Context(), EventPreToolUse, Payload{}, Outcome{Allow: true}); err != nil {
		t.Fatalf("EmitClaude() error = %v; want nil", err)
	}
}

func TestClaudeEmitStableOrder(t *testing.T) {
	first := mustJSON(t, claudeDocument(EventPreToolUse, "blocked"))
	second := mustJSON(t, claudeDocument(EventPreToolUse, "blocked"))
	if first != second {
		t.Fatalf("claude output order changed:\nfirst %s\nsecond %s", first, second)
	}
}
