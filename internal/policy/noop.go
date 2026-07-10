// Package policy defines mivia-agent governance provider contracts.
// Plan: WS12. PRD: FR-7.1, FR-7.2.
package policy

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/MiviaLabs/mivia-agentkit/internal/pathpolicy"
)

// Noop is the standalone governance provider. It allows all actions and audits them.
type Noop struct {
	AuditPath string
}

// Name returns the provider name.
func (n Noop) Name() string {
	return "noop"
}

// Decide allows every valid action and records the decision event.
func (n Noop) Decide(ctx context.Context, action Action) (Decision, error) {
	if err := action.Validate(); err != nil {
		return Decision{}, err
	}
	decision := (Decision{
		Allowed: true,
		Reason:  "noop provider allows all actions",
		Evidence: map[string]any{
			"provider": "noop",
			"action":   string(action.Kind),
		},
	}).EnsureRef(n.Name(), action)
	if err := n.Record(ctx, Event{
		Kind: "decision-made",
		Payload: map[string]any{
			"provider": n.Name(),
			"ref":      decision.Ref,
			"allowed":  decision.Allowed,
			"reason":   decision.Reason,
		},
	}); err != nil {
		return Decision{}, err
	}
	return decision, nil
}

// Record appends a governance event to the audit log.
func (n Noop) Record(ctx context.Context, event Event) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	path := n.AuditPath
	if path == "" {
		path = filepath.FromSlash(".ai/audit.jsonl")
	}
	repo, relative, err := checkedAuditPath(path)
	if err != nil {
		return err
	}
	if event.When == "" {
		event.When = time.Now().UTC().Format(time.RFC3339)
	}
	line, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal audit event: %w", err)
	}
	if err := pathpolicy.AppendFile(repo, relative, append(line, '\n'), 0o644); err != nil {
		return fmt.Errorf("write audit event: %w", err)
	}
	return nil
}

func checkedAuditPath(path string) (string, string, error) {
	clean := filepath.Clean(path)
	if filepath.IsAbs(clean) {
		for parent := filepath.Dir(clean); parent != filepath.Dir(parent); parent = filepath.Dir(parent) {
			if filepath.Base(parent) != ".ai" {
				continue
			}
			repo := filepath.Dir(parent)
			rel, err := filepath.Rel(repo, clean)
			if err != nil || rel == ".ai" || !strings.HasPrefix(filepath.ToSlash(rel), ".ai/") {
				break
			}
			if err := pathpolicy.NewDefault().Check(repo, rel); err != nil {
				return "", "", fmt.Errorf("audit path: %w", err)
			}
			return repo, rel, nil
		}
		return "", "", fmt.Errorf("audit path %q must stay under .ai", path)
	}
	if err := pathpolicy.NewDefault().Check(".", clean); err != nil {
		return "", "", fmt.Errorf("audit path: %w", err)
	}
	if clean == ".ai" || !strings.HasPrefix(filepath.ToSlash(clean), ".ai/") {
		return "", "", fmt.Errorf("audit path %q must stay under .ai", path)
	}
	return ".", clean, nil
}
