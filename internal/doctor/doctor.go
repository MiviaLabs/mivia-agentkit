// Package doctor validates installed mivia-agent control surfaces.
// Plan: WS3. PRD: FR-2.1, FR-5.4, FR-10.5.
package doctor

import (
	"os"
	"path/filepath"

	"github.com/MiviaLabs/mivia-agentkit/internal/report"
)

// Context carries read-only doctor inputs.
type Context struct {
	Repo       string
	GlobalDir  string
	Strict     bool
	manifest   manifestResult
	manifestOK bool
}

// Check is one doctor validation.
type Check struct {
	ID  string
	Run func(*Context) []report.Finding
}

// Run executes the default check registry.
func Run(ctx Context) report.Report {
	if ctx.Repo == "" {
		ctx.Repo, _ = os.Getwd()
	}
	ctx.Repo, _ = filepath.Abs(ctx.Repo)
	if ctx.GlobalDir == "" {
		if home, err := os.UserHomeDir(); err == nil {
			ctx.GlobalDir = filepath.Join(home, ".agents")
		}
	}
	ctx.manifest = readManifest(ctx.Repo)
	ctx.manifestOK = ctx.manifest.err == nil
	var findings []report.Finding
	for _, check := range DefaultChecks() {
		findings = append(findings, check.Run(&ctx)...)
	}
	return report.New(findings, ctx.Strict)
}

// DefaultChecks returns doctor checks in deterministic order.
func DefaultChecks() []Check {
	return []Check{
		{ID: "manifest.exists_parses", Run: checkManifest},
		{ID: "ai.index_exists", Run: checkAIIndex},
		{ID: "adapters.point_to_index", Run: checkAdaptersPointToIndex},
		{ID: "adapter_files_present", Run: checkAdapterFiles},
		{ID: "hooks.call_mivia_agent", Run: checkHooksCallMiviaAgent},
		{ID: "skills.valid_frontmatter", Run: checkSkillsFrontmatter},
		{ID: "generated_markers.valid", Run: checkManagedMarkers},
		{ID: "ci.calls_doctor_json", Run: checkCICallsDoctor},
		{ID: "no_generated_artifacts_staged", Run: checkGeneratedArtifactsStaged},
		{ID: "no_secret_paths_generated", Run: checkSecretPaths},
		{ID: "loops.bound", Run: checkLoopsBound},
		{ID: "loops.known_adapters", Run: checkLoopsKnownAdapters},
		{ID: "consensus.satisfiable", Run: checkConsensusSatisfiable},
		{ID: "governance.provider_known", Run: checkGovernanceKnown},
		{ID: "global.readable", Run: checkGlobalReadable},
		{ID: "global.no_rule_conflict", Run: checkGlobalRuleConflict},
	}
}

func finding(severity report.Severity, code, path, message string) report.Finding {
	return report.Finding{Severity: severity, Code: code, Path: path, Message: message}
}
