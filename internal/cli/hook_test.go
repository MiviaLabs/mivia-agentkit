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

func TestHookHonorsManifestGovernanceProvider(t *testing.T) {
	repo := t.TempDir()
	// Non-noop provider must be selected from the manifest; agt fails closed
	// when the SDK is not compiled in, proving hooks no longer hardcode Noop.
	manifest := "version: \"1\"\nprofile: standard\ngovernance:\n  provider: agt\n"
	if err := os.WriteFile(filepath.Join(repo, "mivia-agent.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	stdout, stderr, err := runAgentHookInRepo(t, repo, "codex", "pre-tool-use", `{"tool":"bash","command":"git push"}`)
	if err == nil {
		t.Fatalf("hook with agt provider error = nil; want fail-closed provider error (stdout=%q stderr=%q)", stdout, stderr)
	}
	combined := stdout + stderr + err.Error()
	if !strings.Contains(combined, "agt") && !strings.Contains(combined, "governance") && !strings.Contains(combined, "not compiled") && !strings.Contains(combined, "unavailable") {
		// Accept any explicit provider-construction failure; reject silent Noop stamp path.
		if strings.Contains(combined, "quality stamp required") && !strings.Contains(combined, "agt") {
			t.Fatalf("hook used stamp-only Noop path; want governance provider failure: %q", combined)
		}
	}
	// Hardcoding Noop would emit a codex deny JSON with stamp reason and exit 0.
	if err == nil && strings.Contains(stdout, "quality stamp required") {
		t.Fatalf("hook ignored manifest provider and fell back to Noop stamp gate")
	}
}

func runAgentHook(t *testing.T, adapter, event, input string) (string, string, error) {
	t.Helper()
	return runAgentHookInRepo(t, t.TempDir(), adapter, event, input)
}

func runAgentHookInRepo(t *testing.T, repo, adapter, event, input string) (string, string, error) {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "mivia-agent"+binarySuffix())
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
