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

// protectedPatterns are compiled once and checked against flattened payloads,
// individual field values, and all field-value pairs.
var protectedPatterns = func() []struct {
	kind policy.ProtectedKind
	re   *regexp.Regexp
} {
	p := []struct {
		kind policy.ProtectedKind
		re   *regexp.Regexp
	}{
		{policy.ProtectedCommit, regexp.MustCompile(`\bgit\s+commit\b`)},
		{policy.ProtectedPush, regexp.MustCompile(`\bgit\s+push\b`)},
		{policy.ProtectedPullRequest, regexp.MustCompile(`\bgh\s+pr\b|\bpull[-_ ]request\b|\bcreate[-_ ]pr\b`)},
		{policy.ProtectedRelease, regexp.MustCompile(`\bgh\s+release\b`)},
		{policy.ProtectedDeploy, regexp.MustCompile(`\bdeploy\b|\bkubectl\s+apply\b|\bterraform\s+apply\b`)},
		{policy.ProtectedLiveSmoke, regexp.MustCompile(`\blive[-_ ]smoke\b|\bsmoke\s+live\b`)},
	}
	return p
}()

// IsProtected detects protected actions in raw hook payloads.
// Only executable content is inspected: values of command-carrying keys
// ("command", "cmd"), structured program+args payloads (e.g.
// {"program":"git","args":["push"]}), and unparseable payloads captured
// under "malformed" (fail closed when we cannot tell what would run).
// Prose fields — descriptions, file contents, agent prompts, assistant
// messages — are deliberately excluded so *mentioning* a protected action
// never trips the gate; the action itself is still caught when its tool
// call executes.
func IsProtected(raw map[string]any) (policy.ProtectedKind, bool) {
	for _, text := range commandTexts(raw) {
		for _, pattern := range protectedPatterns {
			if pattern.re.MatchString(text) {
				return pattern.kind, true
			}
		}
	}
	return "", false
}

// commandTexts extracts the strings that could actually execute from a
// raw hook payload, recursing through nested maps and lists.
func commandTexts(raw map[string]any) []string {
	var out []string
	var walk func(v any)
	walk = func(v any) {
		switch t := v.(type) {
		case map[string]any:
			for key, val := range t {
				switch strings.ToLower(key) {
				case "command", "cmd", "malformed":
					out = append(out, strings.ToLower(flatten(val)))
				}
			}
			if prog, ok := t["program"].(string); ok {
				parts := []string{prog}
				if args, ok := t["args"].([]any); ok {
					for _, a := range args {
						parts = append(parts, flatten(a))
					}
				}
				out = append(out, strings.ToLower(strings.Join(parts, " ")))
			}
			for _, val := range t {
				walk(val)
			}
		case []any:
			for _, item := range t {
				walk(item)
			}
		}
	}
	walk(raw)
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
