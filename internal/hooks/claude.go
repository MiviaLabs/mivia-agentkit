// Package hooks implements protected-action hook decisions.
// Plan: WS5. PRD: FR-7.1, FR-8.1, FR-8.2, FR-8.3.
package hooks

import (
	"context"
	"encoding/json"
	"os"
)

// ClaudeExitError carries the exit code Claude Code expects for hook blocking.
type ClaudeExitError struct {
	Code   int
	Reason string
}

// Error returns the hook block reason.
func (e ClaudeExitError) Error() string {
	return e.Reason
}

// EmitClaude writes the current Claude Code hook JSON shape to stdout.
//
// Re-verified 2026-07-05 against Claude Code hook docs: PreToolUse structured
// denies use hookSpecificOutput.permissionDecision=deny on exit 0, while exit 2
// blocks with stderr and ignores stdout. This emitter returns ClaudeExitError{2}
// for denials so the CLI can provide the fail-closed exit code.
func EmitClaude(ctx context.Context, event Event, payload Payload, out Outcome) error {
	_ = ctx
	_ = payload
	if out.Allow {
		return nil
	}
	reason := out.Reason
	if reason == "" {
		reason = "blocked by repository policy"
	}
	doc := claudeDocument(event, reason)
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(doc); err != nil {
		return err
	}
	return ClaudeExitError{Code: 2, Reason: reason}
}

func claudeDocument(event Event, reason string) map[string]any {
	if event == EventStop {
		return map[string]any{
			"continue":   false,
			"stopReason": reason,
		}
	}
	return map[string]any{
		"hookSpecificOutput": map[string]any{
			"hookEventName":            "PreToolUse",
			"permissionDecision":       "deny",
			"permissionDecisionReason": reason,
		},
	}
}
