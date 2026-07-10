// Package hooks implements protected-action hook decisions.
// Plan: WS5. PRD: FR-7.1, FR-8.1, FR-8.2, FR-8.3.
package hooks

import (
	"context"
	"encoding/json"
	"fmt"
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
	// ProtectedActions limits the kinds the repository elects to gate. An empty
	// set preserves the secure default of gating every supported kind.
	ProtectedActions map[policy.ProtectedKind]struct{}
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

// RefStampChecker validates a stamp for an explicit Git source ref. Push
// actions use it when available so their proof follows the pushed subject.
type RefStampChecker interface {
	CheckStampForRef(repo, ref string) (preflight.Stamp, error)
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
	if protected && len(p.ProtectedActions) > 0 {
		_, protected = p.ProtectedActions[kind]
	}
	if !protected {
		return Outcome{Allow: true, Context: implementationContext(p)}, nil
	}
	if malformedProtected(p.Raw) {
		return Outcome{Allow: false, Reason: "malformed protected payload", Kind: kind}, nil
	}
	var s preflight.Stamp
	var err error
	if kind == policy.ProtectedPush {
		if refs, ok := stamp.(RefStampChecker); ok {
			s, err = refs.CheckStampForRef(p.Repo, pushSourceRef(p.Raw))
		} else {
			s, err = stamp.CheckStamp(p.Repo)
		}
	} else {
		s, err = stamp.CheckStamp(p.Repo)
	}
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

// IsProtected detects protected executable actions from adapter command fields.
// It deliberately does not inspect arbitrary prompt, path, or content fields.
func IsProtected(raw map[string]any) (policy.ProtectedKind, bool) {
	command, ok := commandField(raw)
	if !ok {
		return "", false
	}
	return protectedCommand(command)
}

// ProtectedActionSet validates configured action names for the shared engine.
func ProtectedActionSet(actions []string) (map[policy.ProtectedKind]struct{}, error) {
	if len(actions) == 0 {
		return nil, nil
	}
	set := make(map[policy.ProtectedKind]struct{}, len(actions))
	for _, action := range actions {
		kind := policy.ProtectedKind(strings.TrimSpace(action))
		switch kind {
		case policy.ProtectedCommit, policy.ProtectedPush, policy.ProtectedPullRequest, policy.ProtectedDeploy, policy.ProtectedRelease, policy.ProtectedLiveSmoke:
			set[kind] = struct{}{}
		default:
			return nil, fmt.Errorf("unknown protected action %q", action)
		}
	}
	return set, nil
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
	if command, ok := commandField(raw); ok {
		_, _, malformed, protected := parseProtectedCommand(command)
		return protected && malformed
	}
	return false
}

func commandField(raw map[string]any) (string, bool) {
	for _, candidate := range []any{raw["command"], nestedCommand(raw["tool_input"])} {
		command, ok := candidate.(string)
		if ok && strings.TrimSpace(command) != "" {
			return command, true
		}
	}
	if malformed, ok := raw["malformed"].(string); ok && strings.TrimSpace(malformed) != "" {
		return malformed, true
	}
	return "", false
}

func nestedCommand(value any) any {
	input, _ := value.(map[string]any)
	return input["command"]
}

func protectedCommand(command string) (policy.ProtectedKind, bool) {
	kind, _, _, ok := parseProtectedCommand(command)
	return kind, ok
}

func parseProtectedCommand(command string) (policy.ProtectedKind, string, bool, bool) {
	words := commandWords(command)
	for i := 0; i < len(words); {
		word := strings.Trim(words[i], "'\"")
		switch word {
		case "", ";", "&&", "||":
			i++
		case "env", "command", "sudo":
			i++
		case "sh", "bash", "zsh":
			if i+2 < len(words) && words[i+1] == "-c" {
				if kind, ref, malformed, ok := parseProtectedCommand(strings.Trim(words[i+2], "'\"")); ok {
					return kind, ref, malformed, true
				}
			}
			i++
		case "git":
			invocation, next := parseGitInvocation(words, i+1)
			if invocation.kind != "" {
				return invocation.kind, invocation.pushRef, invocation.malformed, true
			}
			i = next
		case "gh":
			if i+1 < len(words) {
				switch strings.Trim(words[i+1], "'\"") {
				case "pr":
					return policy.ProtectedPullRequest, "", false, true
				case "release":
					return policy.ProtectedRelease, "", false, true
				}
			}
			i++
		case "deploy":
			return policy.ProtectedDeploy, "", false, true
		case "live-smoke", "live_smoke":
			return policy.ProtectedLiveSmoke, "", false, true
		case "kubectl":
			if i+1 < len(words) && strings.Trim(words[i+1], "'\"") == "apply" {
				return policy.ProtectedDeploy, "", false, true
			}
			i++
		case "terraform":
			if i+1 < len(words) && strings.Trim(words[i+1], "'\"") == "apply" {
				return policy.ProtectedDeploy, "", false, true
			}
			i++
		default:
			i++
		}
	}
	return "", "", false, false
}

// ProtectedCommandText recognizes a protected executable command. It is used
// only to fail closed when an otherwise-unparseable hook payload contains one.
func ProtectedCommandText(command string) (policy.ProtectedKind, bool) {
	return protectedCommand(command)
}

func pushSourceRef(raw map[string]any) string {
	command, ok := commandField(raw)
	if !ok {
		return "HEAD"
	}
	_, ref, _, protected := parseProtectedCommand(command)
	if protected && ref != "" {
		return ref
	}
	return "HEAD"
}

type gitInvocation struct {
	kind      policy.ProtectedKind
	pushRef   string
	malformed bool
}

func parseGitInvocation(words []string, start int) (gitInvocation, int) {
	i, malformed := skipGitOptions(words, start, false)
	if i >= len(words) || commandBoundary(words[i]) {
		return gitInvocation{malformed: malformed}, i
	}
	subcommand := strings.Trim(words[i], "'\"")
	i++
	switch subcommand {
	case "commit":
		return gitInvocation{kind: policy.ProtectedCommit, malformed: malformed}, i
	case "push":
		ref, malformedPush := parsePushRef(words, i)
		return gitInvocation{kind: policy.ProtectedPush, pushRef: ref, malformed: malformed || malformedPush}, i
	default:
		return gitInvocation{malformed: malformed}, i
	}
}

func parsePushRef(words []string, start int) (string, bool) {
	i, malformed := skipGitOptions(words, start, true)
	if malformed {
		return "", true
	}
	// The first positional argument is the remote; the next is the source
	// refspec. With no source refspec, Git pushes the current HEAD.
	if i >= len(words) || commandBoundary(words[i]) {
		return "HEAD", false
	}
	i++
	i, malformed = skipGitOptions(words, i, true)
	if malformed {
		return "", true
	}
	if i >= len(words) || commandBoundary(words[i]) {
		return "HEAD", false
	}
	ref := strings.Split(strings.Trim(words[i], "'\""), ":")[0]
	if ref == "" {
		return "", true
	}
	return ref, false
}

func skipGitOptions(words []string, start int, push bool) (int, bool) {
	for i := start; i < len(words); i++ {
		word := strings.Trim(words[i], "'\"")
		if commandBoundary(word) || !strings.HasPrefix(word, "-") {
			return i, false
		}
		if gitOptionNeedsValue(word, push) {
			if optionHasInlineValue(word) {
				continue
			}
			i++
			if i >= len(words) || commandBoundary(words[i]) {
				return i, true
			}
		}
	}
	return len(words), false
}

func gitOptionNeedsValue(option string, push bool) bool {
	base := strings.SplitN(option, "=", 2)[0]
	switch base {
	case "-C", "--git-dir", "--work-tree", "-c", "--config-env", "--exec-path":
		return true
	}
	if !push {
		return false
	}
	switch base {
	case "--receive-pack", "--push-option", "-o", "--repo":
		return true
	default:
		return strings.HasPrefix(base, "-o") && len(base) > 2
	}
}

func optionHasInlineValue(option string) bool {
	if strings.Contains(option, "=") {
		return true
	}
	for _, short := range []string{"-C", "-c", "-o"} {
		if strings.HasPrefix(option, short) && len(option) > len(short) {
			return true
		}
	}
	return false
}

func commandBoundary(word string) bool {
	return word == ";" || word == "&&" || word == "||"
}

func commandWords(command string) []string {
	return strings.Fields(strings.NewReplacer(";", " ; ", "&&", " && ", "||", " || ", "{", " ", "}", " ", "[", " ", "]", " ", ",", " ", ":", " ", "\"", " ", "\\", " ").Replace(command))
}
