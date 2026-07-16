// Package hooks implements protected-action hook decisions.
// Plan: WS5. PRD: FR-7.1, FR-8.1, FR-8.2, FR-8.3.
package hooks

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/MiviaLabs/mivia-agentkit/internal/policy"
	"github.com/MiviaLabs/mivia-agentkit/internal/preflight"
)

// Event names an agent hook lifecycle event.
type Event string

const (
	// EventUserPromptSubmit runs before a prompt is accepted.
	EventUserPromptSubmit Event = "user-prompt-submit"
	// EventPreToolUse runs before a tool executes.
	EventPreToolUse Event = "pre-tool-use"
	// EventPermissionRequest runs before an approval prompt is shown.
	EventPermissionRequest Event = "permission-request"
	// EventStop runs before the agent stops.
	EventStop Event = "stop"
)

// Payload is the normalized hook input used by the shared engine.
type Payload struct {
	Event   Event
	Tool    string
	Adapter string
	Repo    string
	Raw     map[string]any
}

// Outcome is the shared hook decision before adapter-specific emission.
type Outcome struct {
	Allow   bool
	Context map[string]string
	Reason  string
	Kind    policy.ProtectedKind
}

// StampChecker validates a repository quality stamp.
type StampChecker interface {
	CheckStamp(repo string) (preflight.Stamp, error)
}

// StampCheckerFunc adapts a function to StampChecker.
type StampCheckerFunc func(repo string) (preflight.Stamp, error)

// CheckStamp validates a repository quality stamp.
func (f StampCheckerFunc) CheckStamp(repo string) (preflight.Stamp, error) {
	return f(repo)
}

// Decide gates protected actions on a fresh stamp and policy approval.
func Decide(ctx context.Context, p Payload, stamp StampChecker, pol policy.Provider) (Outcome, error) {
	kind, protected := IsProtected(p.Raw)
	if !protected {
		return Outcome{Allow: true, Context: implementationContext(p)}, nil
	}
	if malformedProtected(p.Raw) {
		return Outcome{Allow: false, Reason: "malformed protected payload", Kind: kind}, nil
	}
	s, err := stamp.CheckStamp(p.Repo)
	if err != nil {
		return Outcome{Allow: false, Reason: fmt.Sprintf("quality stamp required: %v", err), Kind: kind}, nil
	}
	data, err := s.Marshal()
	if err != nil {
		return Outcome{}, err
	}
	decision, err := pol.Decide(ctx, policy.Action{
		Kind:          policy.ActionProtect,
		ProtectedKind: kind,
		Stamp:         strings.TrimSpace(string(data)),
	})
	if err != nil {
		return Outcome{}, err
	}
	if !decision.Allowed {
		return Outcome{Allow: false, Reason: decision.Reason, Kind: kind}, nil
	}
	return Outcome{Allow: true, Kind: kind}, nil
}

// IsProtected detects protected actions in raw hook payloads.
// It checks the flattened concatenation, individual field values, and
// adjacent field-value pairs so structured payloads (e.g.
// {"program":"git","args":["push"]}) are caught even when keywords are
// split across separate fields.
func IsProtected(raw map[string]any) (policy.ProtectedKind, bool) {
	text := strings.ToLower(flatten(raw))
	fields := fieldValues(raw)
	patterns := []struct {
		kind policy.ProtectedKind
		re   *regexp.Regexp
	}{
		{policy.ProtectedCommit, regexp.MustCompile(`\bgit\s+commit\b`)},
		{policy.ProtectedPush, regexp.MustCompile(`\bgit\s+push\b`)},
		{policy.ProtectedPullRequest, regexp.MustCompile(`\bgh\s+pr\b|\bpull[-_ ]request\b|\bcreate[-_ ]pr\b`)},
		{policy.ProtectedRelease, regexp.MustCompile(`\bgh\s+release\b|\brelease\b`)},
		{policy.ProtectedDeploy, regexp.MustCompile(`\bdeploy\b|\bkubectl\s+apply\b|\bterraform\s+apply\b`)},
		{policy.ProtectedLiveSmoke, regexp.MustCompile(`\blive[-_ ]smoke\b|\bsmoke\s+live\b`)},
	}
	for _, pattern := range patterns {
		if pattern.re.MatchString(text) {
			return pattern.kind, true
		}
		for _, field := range fields {
			if pattern.re.MatchString(field) {
				return pattern.kind, true
			}
		}
	}
	// Check all field pairs: "git" in one field and "push" in another
	// won't be caught by single-field checks when they are not adjacent in
	// the extraction order. Use all-pairs to avoid map iteration order
	// dependence.
	for i := 0; i < len(fields); i++ {
		for j := i + 1; j < len(fields); j++ {
			joined := fields[i] + " " + fields[j]
			for _, pattern := range patterns {
				if pattern.re.MatchString(joined) {
					return pattern.kind, true
				}
			}
		}
	}
	return "", false
}

// fieldValues extracts all string leaf values from a map recursively.
func fieldValues(raw map[string]any) []string {
	var out []string
	var extract func(v any)
	extract = func(v any) {
		switch t := v.(type) {
		case map[string]any:
			for _, v := range t {
				extract(v)
			}
		case []any:
			for _, v := range t {
				extract(v)
			}
		case string:
			out = append(out, strings.ToLower(t))
		}
	}
	extract(raw)
	return out
}

// RawPayload decodes JSON stdin into a map.
func RawPayload(data []byte) (map[string]any, error) {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	if raw == nil {
		raw = map[string]any{}
	}
	return raw, nil
}

func implementationContext(p Payload) map[string]string {
	if p.Event != EventUserPromptSubmit {
		return nil
	}
	return map[string]string{
		"mivia_agent": "Follow the active workstream task, run focused verifiers, and report residual risk before completion.",
	}
}

func malformedProtected(raw map[string]any) bool {
	if len(raw) == 0 {
		return true
	}
	if _, ok := raw["malformed"]; ok {
		return true
	}
	if _, hasTool := raw["tool"]; hasTool {
		if _, hasCommand := raw["command"]; !hasCommand {
			if input, ok := raw["tool_input"].(map[string]any); !ok || len(input) == 0 {
				return true
			}
		}
	}
	return false
}

func flatten(v any) string {
	switch t := v.(type) {
	case map[string]any:
		var b strings.Builder
		for k, v := range t {
			b.WriteString(k)
			b.WriteByte(' ')
			b.WriteString(flatten(v))
			b.WriteByte(' ')
		}
		return b.String()
	case []any:
		var b strings.Builder
		for _, v := range t {
			b.WriteString(flatten(v))
			b.WriteByte(' ')
		}
		return b.String()
	case string:
		return t
	default:
		return fmt.Sprint(t)
	}
}
