// Package preflight writes and validates quality stamps.
// Plan: WS4. PRD: FR-2.4, FR-7.1.
package preflight

import (
	"path/filepath"
	"strings"
)

// Risk classifies how much proof a changed file set requires.
type Risk int

const (
	// Low applies to docs and non-executable metadata.
	Low Risk = iota
	// Medium applies to code with focused verifiers but no protected surface.
	Medium
	// High applies to CI, hooks, deploy, runner, auth, and security surfaces.
	High
)

// ContractMatrix identifies configured high-risk contract rows.
type ContractMatrix struct {
	RequireContractRowsFor []string
}

// Classify returns the highest risk for files and configured contract surfaces.
func Classify(files []string, contractMatrix ContractMatrix) Risk {
	risk := Low
	for _, file := range files {
		slash := filepath.ToSlash(file)
		lower := strings.ToLower(slash)
		if isHighRiskPath(lower, contractMatrix.RequireContractRowsFor) {
			return High
		}
		if isCodePath(lower) {
			risk = Medium
		}
	}
	return risk
}

func isHighRiskPath(path string, configured []string) bool {
	if strings.HasPrefix(path, ".github/workflows/") ||
		strings.HasPrefix(path, ".agents/hooks") ||
		strings.HasPrefix(path, ".claude/settings") ||
		strings.HasPrefix(path, ".codex/hooks") ||
		strings.HasPrefix(path, "semgrep/") ||
		strings.Contains(path, "hook") ||
		strings.Contains(path, "deploy") ||
		strings.Contains(path, "runner") ||
		strings.Contains(path, "auth") ||
		strings.Contains(path, "security") {
		return true
	}
	if strings.HasPrefix(path, "scripts/") && (strings.HasSuffix(path, ".py") || strings.HasSuffix(path, ".sh")) {
		return true
	}
	for _, row := range configured {
		row = strings.ToLower(strings.TrimSpace(row))
		if row != "" && strings.Contains(path, row) {
			return true
		}
	}
	return false
}

func isCodePath(path string) bool {
	for _, suffix := range []string{".go", ".py", ".sh", ".js", ".ts", ".tsx", ".yml", ".yaml", ".json"} {
		if strings.HasSuffix(path, suffix) {
			return true
		}
	}
	return false
}
