// Package audit reports advisory mivia-agent quality gaps.
// Plan: WS3. PRD: FR-2.3, FR-5.4, FR-6.4.
package audit

import (
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/MiviaLabs/mivia-agentkit/internal/config"
	"github.com/MiviaLabs/mivia-agentkit/internal/doctor"
	"github.com/MiviaLabs/mivia-agentkit/internal/render"
	"github.com/MiviaLabs/mivia-agentkit/internal/report"
	"gopkg.in/yaml.v3"
)

// Context carries read-only audit inputs.
type Context struct {
	Repo      string
	GlobalDir string
	Strict    bool
}

// Run executes advisory audit checks.
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
	var findings []report.Finding
	findings = append(findings, missingAI(ctx)...)
	findings = append(findings, duplicatedAdapterPolicy(ctx)...)
	findings = append(findings, missingCI(ctx)...)
	findings = append(findings, missingContracts(ctx)...)
	findings = append(findings, unsafeMCP(ctx)...)
	findings = append(findings, editedManagedOutsideBlocks(ctx)...)
	findings = append(findings, weakLoops(ctx)...)
	findings = append(findings, globalRuleConflict(ctx)...)
	if len(findings) == 0 {
		return report.New(nil, false)
	}
	out := report.New(findings, ctx.Strict)
	if !ctx.Strict {
		out.ExitCode = 0
	}
	return out
}

func missingAI(ctx Context) []report.Finding {
	if _, err := os.Stat(filepath.Join(ctx.Repo, ".ai")); os.IsNotExist(err) {
		return []report.Finding{warn("canonical.missing_ai", ".ai", "canonical .ai surface is missing")}
	}
	return nil
}

func missingCI(ctx Context) []report.Finding {
	data, err := os.ReadFile(filepath.Join(ctx.Repo, ".github", "workflows", "agent-control.yml"))
	if err != nil || !bytes.Contains(data, []byte("mivia-agent doctor --json")) {
		return []report.Finding{warn("ci.missing_control_check", ".github/workflows/agent-control.yml", "strict profile should gate on doctor --json")}
	}
	return nil
}

func missingContracts(ctx Context) []report.Finding {
	var findings []report.Finding
	if _, err := os.Stat(filepath.Join(ctx.Repo, ".ai", "quality", "contracts", "project-runtime.yaml")); err != nil {
		findings = append(findings, warn("contracts.missing_matrix", ".ai/quality/contracts/project-runtime.yaml", "contract matrix is missing"))
	}
	data, err := os.ReadFile(filepath.Join(ctx.Repo, "mivia-agent.yaml"))
	if err == nil && !bytes.Contains(data, []byte("required_verifiers")) {
		findings = append(findings, warn("commands.empty_verifier_matrix", "mivia-agent.yaml", "required verifier matrix is empty"))
	}
	return findings
}

func unsafeMCP(ctx Context) []report.Finding {
	data, err := os.ReadFile(filepath.Join(ctx.Repo, "mivia-agent.yaml"))
	if err != nil {
		return nil
	}
	if bytes.Contains(data, []byte("mcp:")) && bytes.Contains(data, []byte("*")) {
		return []report.Finding{warn("mcp.unsafe_config", "mivia-agent.yaml", "MCP config must not use wildcard allow-list")}
	}
	return nil
}

func editedManagedOutsideBlocks(ctx Context) []report.Finding {
	expected, err := expectedRendered(ctx)
	if err != nil {
		return nil
	}
	var findings []report.Finding
	for _, rel := range sortedKeys(expected) {
		data, err := os.ReadFile(filepath.Join(ctx.Repo, filepath.FromSlash(rel)))
		if err != nil {
			continue
		}
		pre, _, post, ok := extractHTMLOrHashManaged(data)
		expectedPre, _, expectedPost, expectedOK := extractHTMLOrHashManaged(expected[rel])
		if ok && expectedOK && (!bytes.Equal(pre, expectedPre) || !bytes.Equal(post, expectedPost)) {
			findings = append(findings, warn("generated.edited_outside_managed_blocks", rel, "managed file contains edited content outside managed block"))
		}
	}
	return findings
}

func expectedRendered(ctx Context) (map[string][]byte, error) {
	data, err := os.ReadFile(filepath.Join(ctx.Repo, "mivia-agent.yaml"))
	if err != nil {
		return nil, err
	}
	manifest, err := config.Parse(data)
	if err != nil {
		return nil, err
	}
	var adapters []string
	for name, adapter := range manifest.Adapters {
		if adapter.Enabled {
			adapters = append(adapters, name)
		}
	}
	sort.Strings(adapters)
	plan, vars, err := render.PlanInit(render.InitConfig{Repo: ctx.Repo, Profile: manifest.Profile, Adapters: adapters})
	if err != nil {
		return nil, err
	}
	rendered, err := render.New().RenderAll(plan, vars)
	if err != nil {
		return nil, err
	}
	return rendered, nil
}

func weakLoops(ctx Context) []report.Finding {
	data, err := os.ReadFile(filepath.Join(ctx.Repo, "mivia-agent.yaml"))
	if err != nil {
		return nil
	}
	var parsed struct {
		Profile  string `yaml:"profile"`
		Adapters map[string]struct {
			Enabled bool   `yaml:"enabled"`
			Role    string `yaml:"role"`
		} `yaml:"adapters"`
		Loops map[string]struct {
			ExitWhen string `yaml:"exit_when"`
			Steps    []struct {
				ID        string   `yaml:"id"`
				Approval  string   `yaml:"approval"`
				Reviewers []string `yaml:"reviewers"`
				Consensus struct {
					Mode         string `yaml:"mode"`
					MinReviewers int    `yaml:"min_reviewers"`
				} `yaml:"consensus"`
			} `yaml:"steps"`
		} `yaml:"loops"`
		Governance struct {
			Provider string `yaml:"provider"`
		} `yaml:"governance"`
	}
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		return nil
	}
	enabled := 0
	for _, adapter := range parsed.Adapters {
		if adapter.Enabled && adapter.Role == "orchestrable" {
			enabled++
		}
	}
	var findings []report.Finding
	if parsed.Profile == "strict" && parsed.Governance.Provider == "noop" {
		findings = append(findings, warn("governance.noop_under_strict", "mivia-agent.yaml", "strict profile should not use noop governance"))
	}
	for loopName, loop := range parsed.Loops {
		hasReview := false
		protectBound := loop.ExitWhen == "protected_action"
		for _, step := range loop.Steps {
			if len(step.Reviewers) > 0 {
				hasReview = true
			}
			if step.Approval == "protected" {
				protectBound = true
			}
			if len(step.Reviewers) > 0 {
				min := step.Consensus.MinReviewers
				if min == 0 {
					min = len(step.Reviewers)
				}
				if min > enabled {
					findings = append(findings, warn("consensus.min_reviewers_exceeds_enabled", "mivia-agent.yaml", "min_reviewers exceeds enabled orchestrable adapters"))
				}
			}
			if parsed.Profile == "strict" && protectBound && (step.Consensus.Mode == "first-pass" || step.Consensus.Mode == "") {
				findings = append(findings, warn("consensus.weaker_than_profile_requires", "mivia-agent.yaml", "strict protect-bound loops require majority or unanimous consensus"))
			}
		}
		if protectBound && !hasReview {
			findings = append(findings, warn("loop.no_review_before_protect", "mivia-agent.yaml", "loop "+loopName+" reaches protected action without review step"))
		}
	}
	return findings
}

func globalRuleConflict(ctx Context) []report.Finding {
	dctx := doctor.Context{Repo: ctx.Repo, GlobalDir: ctx.GlobalDir}
	var findings []report.Finding
	for _, check := range doctor.DefaultChecks() {
		if check.ID != "global.no_rule_conflict" {
			continue
		}
		for _, finding := range check.Run(&dctx) {
			if finding.Code == "global.rule_conflict" {
				finding.Code = "global.rule_conflict_with_project"
				findings = append(findings, finding)
			}
		}
	}
	return findings
}

func readMarkdownBlocks(root string) [][]byte {
	var blocks [][]byte
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(path) != ".md" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err == nil {
			for _, block := range strings.Split(string(data), "\n\n") {
				trimmed := strings.TrimSpace(block)
				if len(trimmed) >= 80 {
					blocks = append(blocks, []byte(trimmed))
				}
			}
		}
		return nil
	})
	return blocks
}

func extractHTMLOrHashManaged(data []byte) (pre, managed, post []byte, ok bool) {
	pairs := [][2][]byte{
		{[]byte("<!-- mivia-agent:managed:start -->"), []byte("<!-- mivia-agent:managed:end -->")},
		{[]byte("# mivia-agent:managed:start"), []byte("# mivia-agent:managed:end")},
	}
	for _, pair := range pairs {
		start := bytes.Index(data, pair[0])
		if start < 0 {
			continue
		}
		after := start + len(pair[0])
		endRel := bytes.Index(data[after:], pair[1])
		if endRel < 0 {
			return nil, nil, nil, false
		}
		end := after + endRel
		return data[:start], data[after:end], data[end+len(pair[1]):], true
	}
	return nil, nil, nil, false
}

func warn(code, path, message string) report.Finding {
	return report.Finding{Severity: report.SeverityWarn, Code: code, Path: path, Message: message}
}

func sortedKeys[V any](in map[string]V) []string {
	keys := make([]string, 0, len(in))
	for key := range in {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
