// Package hooks implements protected-action hook decisions.
// Plan: WS5. PRD: FR-7.1, FR-8.1, FR-8.2, FR-8.3.
package hooks

import (
	"context"
	"errors"
	"strings"
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

func TestIsProtectedDetectsGitWithGlobalFlags(t *testing.T) {
	cases := []struct {
		name    string
		payload map[string]any
		want    policy.ProtectedKind
	}{
		{
			"git -C path push",
			map[string]any{"command": "git -C /tmp/repo push origin main"},
			policy.ProtectedPush,
		},
		{
			"git --no-pager commit",
			map[string]any{"command": "git --no-pager commit -m x"},
			policy.ProtectedCommit,
		},
		{
			"git -c commit.gpgsign=false commit",
			map[string]any{"command": "git -c commit.gpgsign=false commit -m x"},
			policy.ProtectedCommit,
		},
		{
			"structured program git with -C and push",
			map[string]any{"program": "git", "args": []any{"-C", "/tmp/repo", "push", "origin"}},
			policy.ProtectedPush,
		},
		{
			"git --git-dir=.git push",
			map[string]any{"command": "git --git-dir=.git push"},
			policy.ProtectedPush,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := IsProtected(tc.payload)
			if !ok || got != tc.want {
				t.Fatalf("IsProtected() = %q, %v; want %q, true", got, ok, tc.want)
			}
		})
	}
}

func TestIsProtectedDetectsWindowsGitExe(t *testing.T) {
	cases := []struct {
		name    string
		payload map[string]any
		want    policy.ProtectedKind
	}{
		{
			"git.exe push command string",
			map[string]any{"command": "git.exe push origin main"},
			policy.ProtectedPush,
		},
		{
			"git.cmd commit command string",
			map[string]any{"command": "git.cmd commit -m x"},
			policy.ProtectedCommit,
		},
		{
			"program git.exe with push args",
			map[string]any{"program": "git.exe", "args": []any{"push", "origin"}},
			policy.ProtectedPush,
		},
		{
			"program path with git.cmd",
			map[string]any{"program": `C:\Program Files\Git\cmd\git.cmd`, "args": []any{"commit", "-m", "x"}},
			policy.ProtectedCommit,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := IsProtected(tc.payload)
			if !ok || got != tc.want {
				t.Fatalf("IsProtected() = %q, %v; want %q, true", got, ok, tc.want)
			}
		})
	}
}

func TestIsProtectedDoesNotTripOnDeployPathSegment(t *testing.T) {
	cases := []struct {
		name    string
		payload map[string]any
	}{
		{"go test package path", map[string]any{"command": "go test ./internal/deploy"}},
		{"list deploy directory", map[string]any{"command": "ls ./deploy"}},
		{"cat deploy yaml", map[string]any{"command": "cat charts/deploy/values.yaml"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got, ok := IsProtected(tc.payload); ok {
				t.Fatalf("IsProtected() = %q, true; want false for path-only deploy token", got)
			}
		})
	}
	// Real deploy verbs still trip the gate.
	if got, ok := IsProtected(map[string]any{"command": "deploy production"}); !ok || got != policy.ProtectedDeploy {
		t.Fatalf("IsProtected(deploy production) = %q, %v; want deploy, true", got, ok)
	}
	if got, ok := IsProtected(map[string]any{"command": "kubectl apply -f manifest.yaml"}); !ok || got != policy.ProtectedDeploy {
		t.Fatalf("IsProtected(kubectl apply) = %q, %v; want deploy, true", got, ok)
	}
}

func TestIsProtectedReturnsFalseForBenign(t *testing.T) {
	if got, ok := IsProtected(map[string]any{"command": "go test ./..."}); ok || got != "" {
		t.Fatalf("IsProtected() = %q, %v; want empty, false", got, ok)
	}
}

func TestIsProtectedDetectsStructuredPayload(t *testing.T) {
	// Keywords split across "program" and "args" fields must still be caught.
	cases := []struct {
		name    string
		payload map[string]any
		want    policy.ProtectedKind
	}{
		{
			"git push split across program and args",
			map[string]any{"tool": "bash", "program": "git", "args": []any{"push", "origin"}},
			policy.ProtectedPush,
		},
		{
			"git commit in tool_input.command",
			map[string]any{"tool": "bash", "tool_input": map[string]any{"program": "git", "args": []any{"commit", "-m", "x"}}},
			policy.ProtectedCommit,
		},
		{
			"deploy in args array",
			map[string]any{"program": "kubectl", "args": []any{"apply", "-f", "deploy.yaml"}},
			policy.ProtectedDeploy,
		},
		{
			"benign structured payload",
			map[string]any{"program": "go", "args": []any{"test", "./..."}},
			"",
		},
		{
			"keywords split across non-command sibling keys are prose, not execution",
			map[string]any{"a": "git", "b": "irrelevant", "c": "push"},
			"",
		},
		{
			"command value as argv list",
			map[string]any{"tool_input": map[string]any{"command": []any{"git", "push", "origin"}}},
			policy.ProtectedPush,
		},
		{
			"file content mentioning protected action is not protected",
			map[string]any{"tool": "Write", "tool_input": map[string]any{"file_path": "plan.md", "content": "then run git commit -m 'x' and git push"}},
			"",
		},
		{
			"assistant prose in stop payload is not protected",
			map[string]any{"last_assistant_message": "next step is git commit and git push", "stop_hook_active": "false"},
			"",
		},
		{
			"agent prompt mentioning protected action is not protected",
			map[string]any{"tool": "Task", "tool_input": map[string]any{"prompt": "afterwards git push the branch and open a pull request"}},
			"",
		},
		{
			"malformed payload text still scanned fail-closed",
			map[string]any{"malformed": "{\"command\": \"git push origin\""},
			policy.ProtectedPush,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := IsProtected(tc.payload)
			if tc.want == "" {
				if ok {
					t.Fatalf("IsProtected() = %q, %v; want false", got, ok)
				}
				return
			}
			if !ok || got != tc.want {
				t.Fatalf("IsProtected() = %q, %v; want %q, true", got, ok, tc.want)
			}
		})
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

func TestDecidePolicyErrorFailsClosed(t *testing.T) {
	out, err := Decide(context.Background(), Payload{Raw: map[string]any{"command": "git push"}}, stampOK{}, errProvider{err: errors.New("audit write failed")})
	if err != nil {
		t.Fatalf("Decide() error = %v; want deny outcome without error", err)
	}
	if out.Allow {
		t.Fatalf("Decide().Allow = true; want fail-closed deny")
	}
	if !strings.Contains(out.Reason, "policy decision failed") {
		t.Fatalf("Decide().Reason = %q; want policy decision failed", out.Reason)
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

type denyProvider struct{}

func (denyProvider) Name() string { return "deny" }

func (denyProvider) Decide(context.Context, policy.Action) (policy.Decision, error) {
	return policy.Decision{Allowed: false, Reason: "denied"}, nil
}

func (denyProvider) Record(context.Context, policy.Event) error {
	return errors.New("not used")
}

type errProvider struct{ err error }

func (e errProvider) Name() string { return "err" }

func (e errProvider) Decide(context.Context, policy.Action) (policy.Decision, error) {
	return policy.Decision{}, e.err
}

func (e errProvider) Record(context.Context, policy.Event) error {
	return e.err
}

func tempAudit(t *testing.T) string {
	t.Helper()
	return t.TempDir() + "/.ai/audit.jsonl"
}
