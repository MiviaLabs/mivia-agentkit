// Package hooks implements protected-action hook decisions.
// Plan: WS5. PRD: FR-7.1, FR-8.1, FR-8.2, FR-8.3.
package hooks

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
)

// EmitCodex writes the current Codex hook JSON shape to stdout.
//
// Re-verified 2026-07-10 against OpenAI Codex hooks docs: PreToolUse denies
// use hookSpecificOutput.permissionDecision=deny, while PermissionRequest has
// no decision output and therefore receives only systemMessage. UserPromptSubmit
// injects hookSpecificOutput.additionalContext and Stop can return decision=block.
func EmitCodex(ctx context.Context, event Event, payload Payload, out Outcome) error {
	_ = ctx
	_ = payload
	doc := codexDocument(event, out)
	if doc == nil {
		return nil
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(doc)
}

func codexDocument(event Event, out Outcome) map[string]any {
	if out.Allow {
		if event == EventUserPromptSubmit && len(out.Context) > 0 {
			return map[string]any{
				"hookSpecificOutput": map[string]any{
					"additionalContext": out.Context["mivia_agent"],
					"hookEventName":     "UserPromptSubmit",
				},
			}
		}
		return nil
	}
	reason := out.Reason
	if reason == "" {
		reason = "blocked by repository policy"
	}
	switch event {
	case EventPermissionRequest:
		return map[string]any{
			"systemMessage": reason,
		}
	case EventStop:
		return map[string]any{
			"decision": "block",
			"reason":   reason,
		}
	default:
		return map[string]any{
			"hookSpecificOutput": map[string]any{
				"hookEventName":            "PreToolUse",
				"permissionDecision":       "deny",
				"permissionDecisionReason": fmt.Sprintf("%s", reason),
			},
		}
	}
}
