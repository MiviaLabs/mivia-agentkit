// Package integration runs real built-binary and subprocess coverage for shipped command surfaces.
// Plan: WS14. PRD: §3, §4, §5, §7, §14.
package integration

import (
	"path/filepath"
	"testing"
)

func writeResearchWorkflow(t *testing.T, repo string) string {
	t.Helper()
	path := filepath.Join(repo, ".ai", "workflows", "research.yaml")
	mustWriteFile(t, path, "bound: iterations\nmax_iterations: 2\nsteps:\n- id: research\n  producer: codex\n  artifact: research.md\n- id: review\n  reviewers: [codex, claude]\n  artifact: research.md\n  on_fail: iterate\nexit_when: review-pass\non_exhausted: fail\n")
	return path
}

func writeTrustedVerifierManifest(t *testing.T, repo string) {
	t.Helper()
	mustWriteFile(t, filepath.Join(repo, "mivia-agent.yaml"), "quality:\n  required_verifiers: ['true']\n  verifiers:\n    'true':\n      command: ['true']\n")
}
