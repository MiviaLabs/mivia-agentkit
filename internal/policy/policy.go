// Package policy defines mivia-agent governance provider contracts.
// Plan: WS12. PRD: FR-2.2, FR-7.1, FR-7.2.
package policy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// ActionKind names a governance action category.
type ActionKind string

const (
	// ActionProtect gates a protected action.
	ActionProtect ActionKind = "protect"
	// ActionLoopStep records or gates a workflow loop step.
	ActionLoopStep ActionKind = "loop-step"
	// ActionReview records or gates a review action.
	ActionReview ActionKind = "review"
)

// ProtectedKind names an action that requires a fresh stamp and policy decision.
type ProtectedKind string

const (
	// ProtectedCommit gates commits.
	ProtectedCommit ProtectedKind = "commit"
	// ProtectedPush gates pushes.
	ProtectedPush ProtectedKind = "push"
	// ProtectedPullRequest gates pull requests.
	ProtectedPullRequest ProtectedKind = "pull_request"
	// ProtectedDeploy gates deployments.
	ProtectedDeploy ProtectedKind = "deploy"
	// ProtectedRelease gates releases.
	ProtectedRelease ProtectedKind = "release"
	// ProtectedLiveSmoke gates live smoke tests.
	ProtectedLiveSmoke ProtectedKind = "live_smoke"
)

// ErrInvalidAction means an action is malformed.
var ErrInvalidAction = errors.New("invalid policy action")

// ErrAGTNotCompiled means this binary lacks the AGT build-tagged provider.
var ErrAGTNotCompiled = errors.New("agt provider is not compiled into this binary")

// Action is the provider input for a governance decision.
type Action struct {
	Kind          ActionKind
	ProtectedKind ProtectedKind
	Step          string
	RunID         string
	Artifact      string
	Stamp         string
	Vars          map[string]any
}

// Validate checks the action contract before provider evaluation.
func (a Action) Validate() error {
	switch a.Kind {
	case ActionProtect:
		if !validProtectedKind(a.ProtectedKind) {
			return fmt.Errorf("%w: protect action requires protected kind", ErrInvalidAction)
		}
	case ActionLoopStep, ActionReview:
		if a.ProtectedKind != "" {
			return fmt.Errorf("%w: protected kind only applies to protect actions", ErrInvalidAction)
		}
	default:
		return fmt.Errorf("%w: unknown action kind %q", ErrInvalidAction, a.Kind)
	}
	return nil
}

// Decision is the provider output for a governance decision.
type Decision struct {
	Allowed  bool
	Reason   string
	Evidence map[string]any
	Ref      string
}

// EnsureRef returns a copy with a stable ref if the provider did not assign one.
func (d Decision) EnsureRef(provider string, action Action) Decision {
	if d.Ref != "" {
		return d
	}
	d.Ref = StableDecisionRef(provider, action, d.Allowed, d.Reason)
	return d
}

// Event is one governance audit event.
type Event struct {
	Kind    string
	When    string
	Payload map[string]any
}

// Provider decides governance actions and records audit events.
type Provider interface {
	Decide(ctx context.Context, action Action) (Decision, error)
	Record(ctx context.Context, event Event) error
	Name() string
}

// StableDecisionRef returns a deterministic opaque decision id.
func StableDecisionRef(provider string, action Action, allowed bool, reason string) string {
	var b strings.Builder
	b.WriteString(provider)
	b.WriteByte('|')
	b.WriteString(string(action.Kind))
	b.WriteByte('|')
	b.WriteString(string(action.ProtectedKind))
	b.WriteByte('|')
	b.WriteString(action.Step)
	b.WriteByte('|')
	b.WriteString(action.RunID)
	b.WriteByte('|')
	b.WriteString(action.Artifact)
	b.WriteByte('|')
	b.WriteString(action.Stamp)
	b.WriteByte('|')
	b.WriteString(fmt.Sprint(allowed))
	b.WriteByte('|')
	b.WriteString(reason)
	for _, key := range sortedVarKeys(action.Vars) {
		b.WriteByte('|')
		b.WriteString(key)
		b.WriteByte('=')
		b.WriteString(fmt.Sprint(action.Vars[key]))
	}
	sum := sha256.Sum256([]byte(b.String()))
	return "pol_" + hex.EncodeToString(sum[:12])
}

func validProtectedKind(kind ProtectedKind) bool {
	switch kind {
	case ProtectedCommit, ProtectedPush, ProtectedPullRequest, ProtectedDeploy, ProtectedRelease, ProtectedLiveSmoke:
		return true
	default:
		return false
	}
}

func sortedVarKeys(vars map[string]any) []string {
	keys := make([]string, 0, len(vars))
	for key := range vars {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
