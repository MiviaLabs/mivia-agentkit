// Package hooks implements protected-action hook decisions.
// Plan: WS5. PRD: FR-7.1, FR-8.1, FR-8.2, FR-8.3.
package hooks

import (
	"context"
	"errors"
	"testing"

	"github.com/MiviaLabs/mivia-agentkit/internal/policy"
	"github.com/MiviaLabs/mivia-agentkit/internal/preflight"
)

func TestIsProtectedDetectsCommit(t *testing.T) {
	got, ok := IsProtected(map[string]any{"tool": "bash", "command": "git commit -m test"})
	if !ok || got != policy.ProtectedCommit {
		t.Fatalf("IsProtected() = %q, %v; want commit, true", got, ok)
	}
}

func TestIsProtectedDetectsPush(t *testing.T) {
	got, ok := IsProtected(map[string]any{"tool_input": map[string]any{"command": "git push origin dev"}})
	if !ok || got != policy.ProtectedPush {
		t.Fatalf("IsProtected() = %q, %v; want push, true", got, ok)
	}
}

func TestIsProtectedDetectsDeploy(t *testing.T) {
	got, ok := IsProtected(map[string]any{"command": "deploy production"})
	if !ok || got != policy.ProtectedDeploy {
		t.Fatalf("IsProtected() = %q, %v; want deploy, true", got, ok)
	}
}

func TestIsProtectedReturnsFalseForBenign(t *testing.T) {
	if got, ok := IsProtected(map[string]any{"command": "go test ./..."}); ok || got != "" {
		t.Fatalf("IsProtected() = %q, %v; want empty, false", got, ok)
	}
}

func TestIsProtectedParsesGitGlobalOptions(t *testing.T) {
	for _, command := range []string{
		"git -c user.name=test commit -m message",
		"git --no-pager push origin main",
		"git -C /tmp/repo --git-dir .git --work-tree . push --force origin feature:main",
		"git --config-env http.extraHeader=GIT_HEADER push origin feature:main",
	} {
		kind, ok := IsProtected(map[string]any{"tool_name": "Bash", "tool_input": map[string]any{"command": command}})
		if !ok || (kind != policy.ProtectedCommit && kind != policy.ProtectedPush) {
			t.Fatalf("IsProtected(%q) = %q, %v; want protected git action", command, kind, ok)
		}
	}
}

func TestIsProtectedIgnoresNonCommandContent(t *testing.T) {
	for _, raw := range []map[string]any{
		{"tool_name": "Write", "tool_input": map[string]any{"file_path": "docs/release-notes.md", "content": "deploy release notes"}},
		{"prompt": "please prepare a release", "path": "deploy/guide.md"},
	} {
		if kind, ok := IsProtected(raw); ok || kind != "" {
			t.Fatalf("IsProtected(%v) = %q, %v; want unprotected non-command content", raw, kind, ok)
		}
	}
}

func TestIsProtectedHandlesShellWrappers(t *testing.T) {
	kind, ok := IsProtected(map[string]any{"command": "env CI=1 git -c user.name=test commit -m message && git push"})
	if !ok || kind != policy.ProtectedCommit {
		t.Fatalf("IsProtected() = %q, %v; want commit, true", kind, ok)
	}
}

func TestProtectedActionSetRejectsUnknown(t *testing.T) {
	if _, err := ProtectedActionSet([]string{"commit", "surprise"}); err == nil {
		t.Fatal("ProtectedActionSet() error = nil; want unknown action rejection")
	}
}

func TestDecidePushUsesSourceRefChecker(t *testing.T) {
	for _, command := range []string{
		"git -C /tmp/repo --git-dir .git --work-tree . push --force origin feature:main",
		"git push --receive-pack custom origin feature:main",
		"git push --receive-pack=custom --push-option trace --repo mirror origin feature:main",
		"git push -otrace origin feature:main",
	} {
		checker := &refStampOK{}
		out, err := Decide(context.Background(), Payload{Raw: map[string]any{"command": command}}, checker, policy.Noop{AuditPath: tempAudit(t)})
		if err != nil || !out.Allow {
			t.Fatalf("Decide(%q) = %+v, %v; want allow", command, out, err)
		}
		if checker.ref != "feature" {
			t.Fatalf("push source ref for %q = %q; want feature", command, checker.ref)
		}
	}
}

func TestDecideMalformedProtectedGitCommandFailsClosed(t *testing.T) {
	out, err := Decide(context.Background(), Payload{Raw: map[string]any{"command": "git push --git-dir"}}, stampOK{}, policy.Noop{AuditPath: tempAudit(t)})
	if err != nil {
		t.Fatalf("Decide() error = %v", err)
	}
	if out.Allow || out.Reason != "malformed protected payload" {
		t.Fatalf("Decide() = %+v; want malformed protected denial", out)
	}
}

func TestDecideAllowsBenign(t *testing.T) {
	out, err := Decide(context.Background(), Payload{Raw: map[string]any{"command": "go test ./..."}}, stampOK{}, policy.Noop{AuditPath: tempAudit(t)})
	if err != nil {
		t.Fatalf("Decide() error = %v", err)
	}
	if !out.Allow {
		t.Fatalf("Decide().Allow = false; want true")
	}
}

func TestDecideDeniesProtectedWithoutStamp(t *testing.T) {
	out, err := Decide(context.Background(), Payload{Raw: map[string]any{"command": "git push"}}, stampErr{preflight.ErrNoStamp}, policy.Noop{AuditPath: tempAudit(t)})
	if err != nil {
		t.Fatalf("Decide() error = %v", err)
	}
	if out.Allow || out.Reason == "" {
		t.Fatalf("Decide() = %+v; want denied with reason", out)
	}
}

func TestDecideDeniesProtectedOnStaleStamp(t *testing.T) {
	out, err := Decide(context.Background(), Payload{Raw: map[string]any{"command": "git push"}}, stampErr{preflight.ErrStaleStamp{Reason: "head changed"}}, policy.Noop{AuditPath: tempAudit(t)})
	if err != nil {
		t.Fatalf("Decide() error = %v", err)
	}
	if out.Allow || out.Reason == "" {
		t.Fatalf("Decide() = %+v; want denied with stale reason", out)
	}
}

func TestDecideDeniesProtectedOnPolicyDeny(t *testing.T) {
	out, err := Decide(context.Background(), Payload{Raw: map[string]any{"command": "git push"}}, stampOK{}, denyProvider{})
	if err != nil {
		t.Fatalf("Decide() error = %v", err)
	}
	if out.Allow || out.Reason != "denied" {
		t.Fatalf("Decide() = %+v; want policy denial", out)
	}
}

func TestDecideAllowsProtectedWithFreshStampAndPolicyAllow(t *testing.T) {
	out, err := Decide(context.Background(), Payload{Raw: map[string]any{"command": "git push"}}, stampOK{}, policy.Noop{AuditPath: tempAudit(t)})
	if err != nil {
		t.Fatalf("Decide() error = %v", err)
	}
	if !out.Allow {
		t.Fatalf("Decide().Allow = false; want true")
	}
}

func TestMalformedPayloadFailsClosedForProtectedAction(t *testing.T) {
	out, err := Decide(context.Background(), Payload{Raw: map[string]any{"tool": "bash", "malformed": "git commit"}}, stampOK{}, policy.Noop{AuditPath: tempAudit(t)})
	if err != nil {
		t.Fatalf("Decide() error = %v", err)
	}
	if out.Allow || out.Reason != "malformed protected payload" {
		t.Fatalf("Decide() = %+v; want malformed denial", out)
	}
}

type stampOK struct{}

func (stampOK) CheckStamp(string) (preflight.Stamp, error) {
	return preflight.NewStamp("head", "diff", []string{"x.go"}), nil
}

type stampErr struct{ err error }

func (s stampErr) CheckStamp(string) (preflight.Stamp, error) {
	return preflight.Stamp{}, s.err
}

type refStampOK struct{ ref string }

func (s *refStampOK) CheckStamp(repo string) (preflight.Stamp, error) {
	return stampOK{}.CheckStamp(repo)
}

func (s *refStampOK) CheckStampForRef(repo, ref string) (preflight.Stamp, error) {
	s.ref = ref
	return stampOK{}.CheckStamp(repo)
}

type denyProvider struct{}

func (denyProvider) Name() string { return "deny" }

func (denyProvider) Decide(context.Context, policy.Action) (policy.Decision, error) {
	return policy.Decision{Allowed: false, Reason: "denied"}, nil
}

func (denyProvider) Record(context.Context, policy.Event) error {
	return errors.New("not used")
}

func tempAudit(t *testing.T) string {
	t.Helper()
	return t.TempDir() + "/.ai/audit.jsonl"
}
