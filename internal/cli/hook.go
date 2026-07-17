// Package cli implements the mivia-agent command surface.
// Plan: WS5. PRD: FR-7.1, FR-8.1, FR-8.2, FR-8.3.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
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
	hookEvent := normalizeEvent(adapter, event)
	// Fail closed for host CLIs: Claude treats bare exit 1 as non-blocking, so
	// every setup failure must emit an adapter-native deny before returning.
	failClosed := func(reason string) error {
		return emitHookDeny(ctx, adapter, hookEvent, reason)
	}

	data, err := io.ReadAll(io.LimitReader(r, 4<<20+1)) // 4MB max
	if err != nil {
		return failClosed(fmt.Sprintf("read hook stdin: %v", err))
	}
	if len(data) > 4<<20 {
		return failClosed("hook payload exceeds 4MB limit")
	}
	raw, parseErr := hooks.RawPayload(data)
	if parseErr != nil {
		raw = map[string]any{
			"malformed": string(data),
		}
	}
	absRepo := absRepoPath(repo)
	p := hooks.Payload{
		Event:   hookEvent,
		Adapter: adapter,
		Tool:    toolName(raw),
		Repo:    absRepo,
		Raw:     raw,
	}
	pol, err := hookPolicy(absRepo)
	if err != nil {
		return failClosed(fmt.Sprintf("governance: %v", err))
	}
	out, err := hooks.Decide(ctx, p, hooks.StampCheckerFunc(preflight.CheckStamp), pol)
	if err != nil {
		return failClosed(fmt.Sprintf("hook decision: %v", err))
	}
	if parseErr != nil && out.Allow {
		fmt.Fprintln(os.Stderr, "warning: ignored malformed non-protected hook payload")
		return nil
	}
	return emitHookOutcome(ctx, adapter, p, out)
}

// emitHookDeny emits an adapter-native deny for setup/policy failures so host
// CLIs never treat a bare exit 1 as "continue with the protected tool".
func emitHookDeny(ctx context.Context, adapter string, event hooks.Event, reason string) error {
	if reason == "" {
		reason = "blocked by repository policy"
	}
	p := hooks.Payload{Event: event, Adapter: adapter}
	out := hooks.Outcome{Allow: false, Reason: reason}
	return emitHookOutcome(ctx, adapter, p, out)
}

func emitHookOutcome(ctx context.Context, adapter string, p hooks.Payload, out hooks.Outcome) error {
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

// hookPolicy loads the project governance provider for the hook surface.
// Missing manifests fall back to defaults (noop). Unknown or unavailable
// providers fail closed so hooks never silently ignore configured policy.
func hookPolicy(repo string) (policy.Provider, error) {
	manifest, err := loadManifest(repo)
	if err != nil {
		return nil, fmt.Errorf("load governance for hook: %w", err)
	}
	auditPath := filepath.Join(absRepoPath(repo), ".ai", "audit.jsonl")
	if manifest.Governance.AuditLog != "" {
		// Only accept project-relative .ai/audit.jsonl style paths via provider checks.
		if filepath.IsAbs(manifest.Governance.AuditLog) {
			auditPath = manifest.Governance.AuditLog
		} else {
			auditPath = filepath.Join(absRepoPath(repo), filepath.FromSlash(manifest.Governance.AuditLog))
		}
	}
	return policy.New(manifest.Governance.Provider, auditPath)
}
