// Package cli implements the mivia-agent command surface.
// Plan: WS5. PRD: FR-7.1, FR-8.1, FR-8.2, FR-8.3.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/MiviaLabs/mivia-agentkit/internal/hooks"
	"github.com/MiviaLabs/mivia-agentkit/internal/policy"
	"github.com/MiviaLabs/mivia-agentkit/internal/preflight"
	"github.com/spf13/cobra"
)

func newHookCommand() *cobra.Command {
	var repo string
	cmd := &cobra.Command{
		Use:   "hook",
		Short: "Run agent hook policy checks",
	}
	cmd.PersistentFlags().StringVar(&repo, "repo", ".", "repository path")
	cmd.AddCommand(newHookAdapterCommand("codex", &repo))
	cmd.AddCommand(newHookAdapterCommand("claude", &repo))
	return cmd
}

func newHookAdapterCommand(adapter string, repo *string) *cobra.Command {
	return &cobra.Command{
		Use:   adapter + " <event>",
		Short: "Run " + adapter + " hook",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runHook(cmd.Context(), cmd.InOrStdin(), adapter, args[0], *repo)
		},
	}
}

func runHook(ctx context.Context, r io.Reader, adapter, event, repo string) error {
	data, err := io.ReadAll(io.LimitReader(r, 4<<20)) // 4MB max
	if err != nil {
		return fmt.Errorf("read hook stdin: %w", err)
	}
	if len(data) >= 4<<20 {
		return fmt.Errorf("hook payload exceeds 4MB limit")
	}
	raw, parseErr := hooks.RawPayload(data)
	if parseErr != nil {
		raw = map[string]any{
			"malformed": string(data),
		}
	}
	p := hooks.Payload{
		Event:   normalizeEvent(adapter, event),
		Adapter: adapter,
		Tool:    toolName(raw),
		Repo:    repo,
		Raw:     raw,
	}
	out, err := hooks.Decide(ctx, p, hooks.StampCheckerFunc(preflight.CheckStamp), policy.Noop{AuditPath: repo + "/.ai/audit.jsonl"})
	if err != nil {
		return err
	}
	if parseErr != nil && out.Allow {
		fmt.Fprintln(os.Stderr, "warning: ignored malformed non-protected hook payload")
		return nil
	}
	switch adapter {
	case "codex":
		return hooks.EmitCodex(ctx, p.Event, p, out)
	case "claude":
		err := hooks.EmitClaude(ctx, p.Event, p, out)
		var exit hooks.ClaudeExitError
		if errors.As(err, &exit) {
			return ExitError{Code: exit.Code, Err: errors.New(exit.Reason)}
		}
		return err
	default:
		return fmt.Errorf("unknown hook adapter %q", adapter)
	}
}

func normalizeEvent(adapter, event string) hooks.Event {
	normalized := strings.ToLower(strings.ReplaceAll(event, "_", "-"))
	switch normalized {
	case "userpromptsubmit":
		return hooks.EventUserPromptSubmit
	case "pretooluse":
		return hooks.EventPreToolUse
	case "permissionrequest":
		return hooks.EventPermissionRequest
	case "stop":
		return hooks.EventStop
	default:
		return hooks.Event(normalized)
	}
}

func toolName(raw map[string]any) string {
	for _, key := range []string{"tool", "tool_name", "name"} {
		if value, ok := raw[key].(string); ok {
			return value
		}
	}
	return ""
}
