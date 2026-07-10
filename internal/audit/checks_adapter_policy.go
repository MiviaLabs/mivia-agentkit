// Package audit reports advisory mivia-agent quality gaps.
// Plan: WS6. PRD: FR-1.1, FR-3.1.
package audit

import (
	"bytes"
	"os"
	"path/filepath"

	"github.com/MiviaLabs/mivia-agentkit/internal/report"
)

const adapterDuplicateBlockBytes = 80

func duplicatedAdapterPolicy(ctx Context) []report.Finding {
	canonical, err := readMarkdownBlocksChecked(filepath.Join(ctx.Repo, ".ai"))
	if err != nil {
		return []report.Finding{{Severity: report.SeverityError, Code: "policy.tree_unreadable", Path: ".ai", Message: err.Error()}}
	}
	if len(canonical) == 0 {
		return nil
	}
	var findings []report.Finding
	for _, rel := range []string{"AGENTS.md", "CLAUDE.md", "GEMINI.md", ".crush/README.md", ".github/copilot-instructions.md"} {
		data, err := os.ReadFile(filepath.Join(ctx.Repo, filepath.FromSlash(rel)))
		if err != nil {
			continue
		}
		for _, block := range canonical {
			if len(block) >= adapterDuplicateBlockBytes && bytes.Contains(data, block) {
				findings = append(findings, warn("policy.duplicated_in_adapters", rel, "adapter duplicates canonical policy verbatim"))
				break
			}
		}
	}
	return findings
}
