// Package policy defines mivia-agent governance provider contracts.
// Plan: WS12. PRD: FR-7.1, FR-7.2.
package policy

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
	checked, err := checkedAuditPath(path)
	if err != nil {
		return err
	}
	path = checked
	if event.When == "" {
		event.When = time.Now().UTC().Format(time.RFC3339)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create audit dir: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open audit log: %w", err)
	}
	defer file.Close()
	line, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal audit event: %w", err)
	}
	if _, err := file.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("write audit event: %w", err)
	}
	return nil
}

func checkedAuditPath(path string) (string, error) {
	clean := filepath.Clean(path)
	if filepath.Base(clean) != "audit.jsonl" || filepath.Base(filepath.Dir(clean)) != ".ai" {
		return "", fmt.Errorf("audit path %q must stay under .ai/audit.jsonl", path)
	}
	if filepath.IsAbs(clean) {
		repo := filepath.Dir(filepath.Dir(clean))
		if err := pathpolicy.NewDefault().Check(repo, filepath.Join(".ai", "audit.jsonl")); err != nil {
			return "", fmt.Errorf("audit path: %w", err)
		}
		return clean, nil
	}
	if err := pathpolicy.NewDefault().Check(".", clean); err != nil {
		return "", fmt.Errorf("audit path: %w", err)
	}
	if filepath.ToSlash(clean) != ".ai/audit.jsonl" {
		return "", fmt.Errorf("audit path %q must stay under .ai/audit.jsonl", path)
	}
	return clean, nil
}
