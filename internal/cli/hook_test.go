// Package cli implements the mivia-agent command surface.
// Plan: WS5. PRD: FR-7.1, FR-8.1, FR-8.2, FR-8.3.
package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MiviaLabs/mivia-agentkit/internal/preflight"
)

func TestHookCodexSubprocessDeniesProtectedWithoutStamp(t *testing.T) {
	stdout, _, err := runAgentHook(t, "codex", "pre-tool-use", `{"tool":"bash","command":"git push"}`)
	if err != nil {
		t.Fatalf("hook codex error = %v", err)
	}
	if !strings.Contains(stdout, `"permissionDecision": "deny"`) {
		t.Fatalf("stdout = %s; want codex deny", stdout)
	}
}

func TestHookClaudeSubprocessDeniesProtectedWithoutStamp(t *testing.T) {
	stdout, stderr, err := runAgentHook(t, "claude", "pre-tool-use", `{"tool":"bash","command":"git push"}`)
	if err != nil {
		t.Fatalf("hook claude error = %v; want structured denial", err)
	}
	if !strings.Contains(stdout, `"permissionDecision": "deny"`) || stderr != "" {
		t.Fatalf("stdout=%q stderr=%q; want structured denial only", stdout, stderr)
	}
}

func TestHookCodexSubprocessAllowsBenign(t *testing.T) {
	stdout, _, err := runAgentHook(t, "codex", "pre-tool-use", `{"tool":"bash","command":"go test ./..."}`)
	if err != nil {
		t.Fatalf("hook codex error = %v", err)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q; want empty allow output", stdout)
	}
}

func TestHookCodexSubprocessAcceptsPipelinePreflightStamp(t *testing.T) {
	repo := newHookRepo(t)
	writeHookManifest(t, repo, "")
	writeHookFile(t, repo, "docs/readme.md", "hello\n")
	runHookGit(t, repo, "add", "docs/readme.md")
	_, err := preflight.Run(preflight.Context{Repo: repo, BroadVerifiers: []string{"ok"}})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	runHookGit(t, repo, "commit", "-q", "-m", "validated")
	payload := `{"tool":"bash","command":"` + strings.Join([]string{"git", "push"}, " ") + `"}`
	stdout, stderr, err := runAgentHookInRepo(t, repo, "codex", "pre-tool-use", payload)
	if err != nil {
		t.Fatalf("hook codex error = %v stderr=%q stdout=%q", err, stderr, stdout)
	}
	if stdout != "" || stderr != "" {
		t.Fatalf("hook output = stdout %q stderr %q; want allow with no output", stdout, stderr)
	}
}

func TestHookCodexSubprocessAcceptsOptionPrefixedPushRef(t *testing.T) {
	repo := newHookRepo(t)
	writeHookManifest(t, repo, "")
	writeHookFile(t, repo, "docs/readme.md", "hello\n")
	runHookGit(t, repo, "add", "docs/readme.md")
	if _, err := preflight.Run(preflight.Context{Repo: repo, BroadVerifiers: []string{"ok"}}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	runHookGit(t, repo, "commit", "-q", "-m", "validated")
	payload := `{"tool_name":"Bash","tool_input":{"command":"git -C /tmp --git-dir .git --work-tree . push --receive-pack custom --push-option=trace --repo mirror origin HEAD:main"}}`
	stdout, stderr, err := runAgentHookInRepo(t, repo, "codex", "pre-tool-use", payload)
	if err != nil || stdout != "" || stderr != "" {
		t.Fatalf("option-prefixed push = stdout %q stderr %q err %v; want allow", stdout, stderr, err)
	}
}

func TestHookCodexSubprocessDeniesMalformedProtectedGitCommand(t *testing.T) {
	stdout, stderr, err := runAgentHook(t, "codex", "pre-tool-use", `{"tool_name":"Bash","tool_input":{"command":"git push --git-dir"}}`)
	if err != nil || !strings.Contains(stdout, "malformed protected payload") || stderr != "" {
		t.Fatalf("malformed protected command = stdout %q stderr %q err %v; want denial", stdout, stderr, err)
	}
}

func TestHookMalformedStdinFailsClosedForProtected(t *testing.T) {
	stdout, _, err := runAgentHook(t, "codex", "pre-tool-use", `{"tool":"bash","command":"git commit"`)
	if err != nil {
		t.Fatalf("hook codex malformed error = %v", err)
	}
	if !strings.Contains(stdout, "malformed protected payload") {
		t.Fatalf("stdout = %s; want malformed denial", stdout)
	}
}

func TestHookMalformedBenignStdinAllowsWithWarning(t *testing.T) {
	stdout, stderr, err := runAgentHook(t, "codex", "pre-tool-use", `{"tool":"bash","command":"go test ./..."`)
	if err != nil {
		t.Fatalf("hook codex malformed benign error = %v", err)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q; want empty allow output", stdout)
	}
	if !strings.Contains(stderr, "warning: ignored malformed non-protected hook payload") {
		t.Fatalf("stderr = %q; want malformed warning", stderr)
	}
}

func TestHookHonorsConfiguredProtectedActions(t *testing.T) {
	repo := newHookRepo(t)
	writeHookFile(t, repo, "mivia-agent.yaml", "protected_actions:\n  - commit\n")
	stdout, stderr, err := runAgentHookInRepo(t, repo, "codex", "pre-tool-use", `{"tool_name":"Bash","tool_input":{"command":"git --no-pager push origin main"}}`)
	if err != nil || stdout != "" || stderr != "" {
		t.Fatalf("unconfigured push = stdout %q stderr %q err %v; want unguarded", stdout, stderr, err)
	}
	stdout, _, err = runAgentHookInRepo(t, repo, "codex", "pre-tool-use", `{"tool_name":"Bash","tool_input":{"command":"git -c user.name=test commit -m message"}}`)
	if err != nil || !strings.Contains(stdout, `"permissionDecision": "deny"`) {
		t.Fatalf("configured commit = stdout %q err %v; want denial", stdout, err)
	}
}

func TestHookLoadsManifestGovernanceProvider(t *testing.T) {
	repo := newHookRepo(t)
	writeHookManifest(t, repo, "governance:\n  provider: noop\n  audit_log: .ai/configured-hook-audit.jsonl\n")
	writeHookFile(t, repo, "docs/change.md", "change\n")
	runHookGit(t, repo, "add", "docs/change.md")
	if _, err := preflight.Run(preflight.Context{Repo: repo, BroadVerifiers: []string{"ok"}}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	runHookGit(t, repo, "commit", "-q", "-m", "validated")
	stdout, stderr, err := runAgentHookInRepo(t, repo, "codex", "pre-tool-use", `{"tool_name":"Bash","tool_input":{"command":"git push"}}`)
	if err != nil || stdout != "" || stderr != "" {
		t.Fatalf("configured noop hook = stdout %q stderr %q err %v; want allow", stdout, stderr, err)
	}
	if _, err := os.Stat(filepath.Join(repo, ".ai", "configured-hook-audit.jsonl")); err != nil {
		t.Fatalf("configured audit log: %v", err)
	}
}

func TestHookStrictAGTUnavailableFailsClosed(t *testing.T) {
	repo := newHookRepo(t)
	writeHookManifest(t, repo, "governance:\n  provider: agt\n")
	writeHookFile(t, repo, "docs/change.md", "change\n")
	runHookGit(t, repo, "add", "docs/change.md")
	if _, err := preflight.Run(preflight.Context{Repo: repo, BroadVerifiers: []string{"ok"}}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	_, stderr, err := runAgentHookInRepo(t, repo, "claude", "pre-tool-use", `{"tool_name":"Bash","tool_input":{"command":"git push"}}`)
	if err == nil {
		t.Fatal("strict AGT hook error = nil; want fail closed")
	}
	if !strings.Contains(stderr, "agt provider is not compiled") {
		t.Fatalf("strict AGT stderr = %q; want unavailable provider", stderr)
	}
}

func writeHookManifest(t *testing.T, repo, extra string) {
	t.Helper()
	writeHookFile(t, repo, "mivia-agent.yaml", "quality:\n  required_verifiers:\n    - ok\n  verifiers:\n    ok:\n      command: [\"true\"]\n"+extra)
}

func runAgentHook(t *testing.T, adapter, event, input string) (string, string, error) {
	t.Helper()
	return runAgentHookInRepo(t, t.TempDir(), adapter, event, input)
}

func runAgentHookInRepo(t *testing.T, repo, adapter, event, input string) (string, string, error) {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "mivia-agent")
	build := exec.Command("go", "build", "-o", bin, "./cmd/mivia-agent")
	build.Dir = "../.."
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build mivia-agent: %v\n%s", err, out)
	}
	cmd := exec.Command(bin, "hook", adapter, event, "--repo", repo)
	cmd.Stdin = strings.NewReader(input)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

func newHookRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	runHookGit(t, repo, "init", "-q")
	runHookGit(t, repo, "config", "user.email", "test@example.invalid")
	runHookGit(t, repo, "config", "user.name", "Test User")
	runHookGit(t, repo, "config", "commit.gpgsign", "false")
	runHookGit(t, repo, "commit", "-q", "--allow-empty", "-m", "init")
	return repo
}

func writeHookFile(t *testing.T, repo, rel, content string) {
	t.Helper()
	path := filepath.Join(repo, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
}

func runHookGit(t *testing.T, repo string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v error = %v output = %s", args, err, out)
	}
}
