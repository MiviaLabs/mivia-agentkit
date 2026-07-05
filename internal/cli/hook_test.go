// Package cli implements the mivia-agent command surface.
// Plan: WS5. PRD: FR-7.1, FR-8.1, FR-8.2, FR-8.3.
package cli

import (
	"bytes"
	"encoding/json"
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
	_, stderr, err := runAgentHook(t, "claude", "pre-tool-use", `{"tool":"bash","command":"git push"}`)
	if err == nil {
		t.Fatalf("hook claude error = nil; want exit 2")
	}
	if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 2 {
		t.Fatalf("hook claude error = %v; want exit 2", err)
	}
	if !strings.Contains(stderr, "quality stamp required") {
		t.Fatalf("stderr = %q; want quality stamp denial", stderr)
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
	writeHookFile(t, repo, "docs/readme.md", "hello\n")
	runHookGit(t, repo, "add", "docs/readme.md")
	stamp, err := preflight.Run(preflight.Context{Repo: repo, BroadVerifiers: []string{"go test ./..."}})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	data, err := stamp.Marshal()
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal(stamp) error = %v", err)
	}
	raw["pipeline_preflight"] = map[string]any{
		"passed":          true,
		"contract_sha256": "contract",
		"categories":      []string{"pipeline"},
		"stages":          []string{"preflight"},
		"verifiers":       []string{"scripts/preflight-v2-pipeline"},
		"created_at":      "2026-07-05T00:00:00Z",
		"future_metadata": map[string]any{"accepted": true},
	}
	data, err = json.MarshalIndent(raw, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent(stamp) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".git", "mivia-agent-quality-stamp.json"), append(data, '\n'), 0o600); err != nil {
		t.Fatalf("WriteFile(stamp) error = %v", err)
	}
	payload := `{"tool":"bash","command":"` + strings.Join([]string{"git", "push"}, " ") + `"}`
	stdout, stderr, err := runAgentHookInRepo(t, repo, "codex", "pre-tool-use", payload)
	if err != nil {
		t.Fatalf("hook codex error = %v stderr=%q stdout=%q", err, stderr, stdout)
	}
	if strings.Contains(stdout+stderr, `unknown field "pipeline_preflight"`) {
		t.Fatalf("hook rejected pipeline_preflight metadata: stdout=%q stderr=%q", stdout, stderr)
	}
	if stdout != "" || stderr != "" {
		t.Fatalf("hook output = stdout %q stderr %q; want allow with no output", stdout, stderr)
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
