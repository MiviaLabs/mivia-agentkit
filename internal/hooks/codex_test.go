// Package hooks implements protected-action hook decisions.
// Plan: WS5. PRD: FR-7.1, FR-8.1, FR-8.2, FR-8.3.
package hooks

import (
	"encoding/json"
	"testing"
)

func TestCodexPreToolUseDenyShape(t *testing.T) {
	got := mustJSON(t, codexDocument(EventPreToolUse, Outcome{Allow: false, Reason: "blocked"}))
	want := "{\n  \"hookSpecificOutput\": {\n    \"hookEventName\": \"PreToolUse\",\n    \"permissionDecision\": \"deny\",\n    \"permissionDecisionReason\": \"blocked\"\n  }\n}"
	if got != want {
		t.Fatalf("codex deny shape:\ngot  %s\nwant %s", got, want)
	}
}

func TestCodexUserPromptSubmitAddsImplementationContext(t *testing.T) {
	got := mustJSON(t, codexDocument(EventUserPromptSubmit, Outcome{Allow: true, Context: map[string]string{"mivia_agent": "ctx"}}))
	want := "{\n  \"hookSpecificOutput\": {\n    \"additionalContext\": \"ctx\",\n    \"hookEventName\": \"UserPromptSubmit\"\n  }\n}"
	if got != want {
		t.Fatalf("codex context shape:\ngot  %s\nwant %s", got, want)
	}
}

func TestCodexStopBlocksDoneWithoutStamp(t *testing.T) {
	got := mustJSON(t, codexDocument(EventStop, Outcome{Allow: false, Reason: "stamp required"}))
	want := "{\n  \"decision\": \"block\",\n  \"reason\": \"stamp required\"\n}"
	if got != want {
		t.Fatalf("codex stop shape:\ngot  %s\nwant %s", got, want)
	}
}

func TestCodexEmitStableOrder(t *testing.T) {
	first := mustJSON(t, codexDocument(EventPermissionRequest, Outcome{Allow: false, Reason: "blocked"}))
	second := mustJSON(t, codexDocument(EventPermissionRequest, Outcome{Allow: false, Reason: "blocked"}))
	if first != second {
		t.Fatalf("codex output order changed:\nfirst %s\nsecond %s", first, second)
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent() error = %v", err)
	}
	return string(data)
}
