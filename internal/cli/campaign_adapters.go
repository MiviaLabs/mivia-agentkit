// Package cli implements the mivia-agent command surface.
// Plan: WS15. PRD: campaign adapters, scoped commit boundary.
package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/MiviaLabs/mivia-agentkit/internal/adapter"
	"github.com/MiviaLabs/mivia-agentkit/internal/auditcampaign"
	"github.com/MiviaLabs/mivia-agentkit/internal/config"
	"github.com/MiviaLabs/mivia-agentkit/internal/gitstate"
	"github.com/MiviaLabs/mivia-agentkit/internal/pathpolicy"
	"github.com/MiviaLabs/mivia-agentkit/internal/policy"
	"github.com/MiviaLabs/mivia-agentkit/internal/preflight"
)

// campaignHost wires campaign phase adapters and coordinator-only scoped commits.
// Local adapters (local / local-*) read typed evidence from
// .ai/campaign-fixtures/<campaign>/. Non-local names invoke approved
// orchestrable adapters and accept only schema-valid
// mivia-agent-campaign-evidence/v1 as commit authority (never raw Markdown).
type campaignHost struct {
	repo     string
	runID    string
	name     string
	camp     config.Campaign
	manifest config.Manifest
	// adapters is the approved orchestrable registry; nil when only local fixtures are used.
	adapters *adapter.Registry
	// expectedHead is the HEAD recorded before each adapter phase.
	expectedHead string
	// phaseTimeout bounds each adapter invocation.
	phaseTimeout time.Duration
}

func newCampaignHost(ctx context.Context, repo, runID, name string, camp config.Campaign, manifest config.Manifest) (*campaignHost, error) {
	abs, err := filepath.Abs(repo)
	if err != nil {
		return nil, err
	}
	h := &campaignHost{
		repo:         abs,
		runID:        runID,
		name:         name,
		camp:         camp,
		manifest:     manifest,
		phaseTimeout: 5 * time.Minute,
		expectedHead: "unknown",
	}
	head, err := gitstate.Head(abs)
	if err != nil {
		if camp.CommitEnabled {
			return nil, fmt.Errorf("campaign requires git repository: %w", err)
		}
	} else {
		h.expectedHead = head
	}
	required := campaignRequiredAdapters(camp, manifest)
	if len(required) == 0 {
		return h, nil
	}
	reg, err := approvedRegistry(ctx, manifest, required...)
	if err != nil {
		return nil, fmt.Errorf("campaign adapters unavailable: %w", err)
	}
	for _, n := range required {
		if _, ok := reg.Lookup(n); !ok {
			return nil, fmt.Errorf("campaign adapter %q is not installed or not approved for run", n)
		}
	}
	h.adapters = reg
	return h, nil
}

// campaignRequiredAdapters lists non-local orchestrable adapter names that must be approved.
func campaignRequiredAdapters(camp config.Campaign, manifest config.Manifest) []string {
	seen := map[string]struct{}{}
	var out []string
	add := func(name string) {
		name = strings.TrimSpace(name)
		if name == "" || isLocalCampaignAdapter(name) {
			return
		}
		if _, ok := seen[name]; ok {
			return
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	add(camp.Auditor)
	add(camp.Confirmer)
	add(resolveFixAdapterName(manifest, camp))
	return out
}

func resolveFixAdapterName(manifest config.Manifest, camp config.Campaign) string {
	loop, ok := manifest.Loops[camp.FixWorkflow]
	if !ok {
		return ""
	}
	for _, step := range loop.Steps {
		if strings.TrimSpace(step.Producer) != "" {
			return strings.TrimSpace(step.Producer)
		}
	}
	return ""
}

func isLocalCampaignAdapter(name string) bool {
	name = strings.TrimSpace(name)
	return name == "local" || strings.HasPrefix(name, "local-")
}

func (h *campaignHost) fixtureDir() string {
	return filepath.Join(h.repo, ".ai", "campaign-fixtures", h.name)
}

func (h *campaignHost) fixturePath(phase auditcampaign.Phase, cycle int) string {
	var label string
	switch phase {
	case auditcampaign.PhaseAuditing:
		label = "audit"
	case auditcampaign.PhaseConfirming:
		label = "confirm"
	case auditcampaign.PhaseFixing:
		label = "fix"
	default:
		label = string(phase)
	}
	return filepath.Join(h.fixtureDir(), fmt.Sprintf("%s-cycle-%d.json", label, cycle))
}

func (h *campaignHost) assertHeadUnchanged() error {
	if h.expectedHead == "" || h.expectedHead == "unknown" {
		return nil
	}
	head, err := gitstate.Head(h.repo)
	if err != nil {
		return err
	}
	if head != h.expectedHead {
		return fmt.Errorf("%s: adapter advanced HEAD from %s to %s", auditcampaign.TerminalUnauthorizedHead, h.expectedHead, head)
	}
	return nil
}

func (h *campaignHost) baseEvidence(cycle int) auditcampaign.Evidence {
	head := h.expectedHead
	if head == "" {
		head = "unknown"
	}
	return auditcampaign.Evidence{
		Schema:       auditcampaign.EvidenceSchema,
		CampaignRun:  h.runID,
		Cycle:        cycle,
		BaselineHead: head,
	}
}

func (h *campaignHost) loadFixture(phase auditcampaign.Phase, cycle int) (auditcampaign.Evidence, bool, error) {
	path := h.fixturePath(phase, cycle)
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return auditcampaign.Evidence{}, false, nil
		}
		return auditcampaign.Evidence{}, false, err
	}
	ev, err := auditcampaign.DecodeEvidence(b)
	if err != nil {
		return auditcampaign.Evidence{}, false, fmt.Errorf("fixture %s: %w", path, err)
	}
	// Bind runtime fields; fixtures must not invent baseline/run identity.
	ev.CampaignRun = h.runID
	ev.Cycle = cycle
	if h.expectedHead != "" && h.expectedHead != "unknown" {
		ev.BaselineHead = h.expectedHead
	}
	if err := ev.Validate(); err != nil {
		return auditcampaign.Evidence{}, false, err
	}
	return ev, true, nil
}

// Audit produces audit-phase evidence from local fixtures or an orchestrable auditor.
func (h *campaignHost) Audit(ctx context.Context, phase auditcampaign.Phase, cycle int) (auditcampaign.Evidence, error) {
	if err := ctx.Err(); err != nil {
		return auditcampaign.Evidence{}, err
	}
	if err := h.assertHeadUnchanged(); err != nil {
		return auditcampaign.Evidence{}, err
	}
	name := strings.TrimSpace(h.camp.Auditor)
	if name == "" {
		return auditcampaign.Evidence{}, fmt.Errorf("campaign auditor is required")
	}
	if isLocalCampaignAdapter(name) {
		ev, ok, err := h.loadFixture(phase, cycle)
		if err != nil {
			return auditcampaign.Evidence{}, err
		}
		if ok {
			if err := h.assertHeadUnchanged(); err != nil {
				return auditcampaign.Evidence{}, err
			}
			return ev, nil
		}
		// Default: clean audit (no findings).
		return h.baseEvidence(cycle), nil
	}
	ev, err := h.invokeOrchestrable(ctx, name, phase, cycle)
	if err != nil {
		return auditcampaign.Evidence{}, err
	}
	if err := h.assertHeadUnchanged(); err != nil {
		return auditcampaign.Evidence{}, err
	}
	return ev, nil
}

// Confirm produces confirmation evidence from local fixtures or an independent confirmer adapter.
func (h *campaignHost) Confirm(ctx context.Context, phase auditcampaign.Phase, cycle int) (auditcampaign.Evidence, error) {
	if err := ctx.Err(); err != nil {
		return auditcampaign.Evidence{}, err
	}
	if err := h.assertHeadUnchanged(); err != nil {
		return auditcampaign.Evidence{}, err
	}
	name := strings.TrimSpace(h.camp.Confirmer)
	if name == "" {
		return auditcampaign.Evidence{}, fmt.Errorf("campaign confirmer is required")
	}
	if isLocalCampaignAdapter(name) {
		ev, ok, err := h.loadFixture(phase, cycle)
		if err != nil {
			return auditcampaign.Evidence{}, err
		}
		if !ok {
			// No confirmation fixture: reject the finding.
			base := h.baseEvidence(cycle)
			base.Disposition = auditcampaign.DispositionRejected
			return base, nil
		}
		if err := h.assertHeadUnchanged(); err != nil {
			return auditcampaign.Evidence{}, err
		}
		return ev, nil
	}
	// Independent confirmer is always a separate adapter invocation from audit.
	ev, err := h.invokeOrchestrable(ctx, name, phase, cycle)
	if err != nil {
		return auditcampaign.Evidence{}, err
	}
	if err := h.assertHeadUnchanged(); err != nil {
		return auditcampaign.Evidence{}, err
	}
	return ev, nil
}

// Fix applies optional local write fixtures or invokes the fix-workflow producer adapter.
func (h *campaignHost) Fix(ctx context.Context, phase auditcampaign.Phase, cycle int) (auditcampaign.Evidence, error) {
	if err := ctx.Err(); err != nil {
		return auditcampaign.Evidence{}, err
	}
	if err := h.assertHeadUnchanged(); err != nil {
		return auditcampaign.Evidence{}, err
	}
	fixName := resolveFixAdapterName(h.manifest, h.camp)
	if fixName == "" {
		// Fall back to local fixture path when no producer is declared.
		fixName = "local"
	}
	if isLocalCampaignAdapter(fixName) {
		if err := h.applyFixWrites(cycle); err != nil {
			return auditcampaign.Evidence{}, err
		}
		if err := h.assertHeadUnchanged(); err != nil {
			return auditcampaign.Evidence{}, err
		}
		ev, ok, err := h.loadFixture(phase, cycle)
		if err != nil {
			return auditcampaign.Evidence{}, err
		}
		if !ok {
			base := h.baseEvidence(cycle)
			base.Disposition = auditcampaign.DispositionFixed
			if len(h.camp.AllowedPaths) > 0 {
				base.ChangedPathIDs = []string{"p1"}
				base.VerifierRef = strings.TrimSpace(h.camp.VerifierProfile)
				if base.VerifierRef == "" {
					base.VerifierRef = "true"
				}
			}
			return base, nil
		}
		return ev, nil
	}
	ev, err := h.invokeOrchestrable(ctx, fixName, phase, cycle)
	if err != nil {
		return auditcampaign.Evidence{}, err
	}
	// Fixers must not commit; HEAD must still match baseline.
	if err := h.assertHeadUnchanged(); err != nil {
		return auditcampaign.Evidence{}, err
	}
	return ev, nil
}

// invokeOrchestrable runs an approved adapter and decodes typed campaign evidence only.
func (h *campaignHost) invokeOrchestrable(ctx context.Context, name string, phase auditcampaign.Phase, cycle int) (auditcampaign.Evidence, error) {
	if h.adapters == nil {
		return auditcampaign.Evidence{}, fmt.Errorf("campaign adapter %q is not wired: registry unavailable", name)
	}
	a, ok := h.adapters.Lookup(name)
	if !ok {
		return auditcampaign.Evidence{}, fmt.Errorf("campaign adapter %q is not installed or not approved for run", name)
	}
	if a.Role() != config.AdapterRoleOrchestrable {
		return auditcampaign.Evidence{}, fmt.Errorf("campaign adapter %q is not orchestrable", name)
	}
	cfg := h.manifest.Adapters[name]
	if !cfg.Enabled && cfg.Role == "" {
		// Missing from manifest defaults is still rejected unless present in registry via test injection.
		// Production registry only includes approved enabled adapters.
	}
	if cfg.Role != "" && cfg.Role != config.AdapterRoleOrchestrable {
		return auditcampaign.Evidence{}, fmt.Errorf("campaign adapter %q is not orchestrable", name)
	}
	if cfg.Role == config.AdapterRoleOrchestrable && !cfg.Enabled {
		return auditcampaign.Evidence{}, fmt.Errorf("campaign adapter %q is disabled", name)
	}

	prompt := campaignPhasePrompt(phase, h.name, h.runID, cycle, h.expectedHead, h.camp)
	timeout := h.phaseTimeout
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	req := adapter.Request{
		Prompt:   prompt,
		Workdir:  h.repo,
		Approval: "never",
		Model:    cfg.Model,
		Effort:   cfg.Effort,
		Timeout:  timeout,
		MaxTurns: 8,
	}
	// Temp artifact only for adapters that write last-message files; never used as
	// commit authority unless schema-decoded, and never persisted under .ai/runs.
	tmp, err := os.CreateTemp("", "mivia-campaign-evidence-*.json")
	if err != nil {
		return auditcampaign.Evidence{}, err
	}
	tmpPath := tmp.Name()
	_ = tmp.Close()
	defer func() { _ = os.Remove(tmpPath) }()
	req.ArtifactOut = tmpPath

	if v, ok := a.(adapter.RequestValidator); ok {
		if err := v.ValidateRequest(req); err != nil {
			return auditcampaign.Evidence{}, fmt.Errorf("campaign adapter %q request: %w", name, err)
		}
	}
	res, err := a.Run(ctx, req)
	if err != nil {
		return auditcampaign.Evidence{}, fmt.Errorf("campaign adapter %q failed: %w", name, err)
	}
	if res.ExitCode != 0 {
		return auditcampaign.Evidence{}, fmt.Errorf("campaign adapter %q exited %d", name, res.ExitCode)
	}

	raw := res.Stdout
	if written, readErr := os.ReadFile(tmpPath); readErr == nil && len(bytes.TrimSpace(written)) > 0 {
		// Prefer last-message artifact when present; still require typed evidence decode.
		raw = written
	}
	// Never accept raw Markdown/prose as commit authority.
	ev, err := decodeAdapterCampaignEvidence(raw, h.runID, cycle, h.expectedHead)
	if err != nil {
		return auditcampaign.Evidence{}, fmt.Errorf("campaign adapter %q: %w", name, err)
	}
	return ev, nil
}

func campaignPhasePrompt(phase auditcampaign.Phase, campaign, runID string, cycle int, head string, camp config.Campaign) string {
	head = strings.TrimSpace(head)
	if head == "" {
		head = "unknown"
	}
	base := []string{
		"Emit ONLY one JSON object with schema mivia-agent-campaign-evidence/v1.",
		"Do not emit Markdown reports as commit authority. Do not invent telemetry numbers.",
		fmt.Sprintf("campaign=%s campaign_run=%s cycle=%d baseline_head=%s", campaign, runID, cycle, head),
		"Fields must use opaque finding_fingerprint and changed_path_ids (no raw paths or secrets).",
		"Do not stage, commit, push, open a PR, or rewrite git history.",
	}
	switch phase {
	case auditcampaign.PhaseAuditing:
		return strings.Join(append([]string{
			"You are the campaign auditor for supervised deep-bug-audit repair.",
			"Follow the deep-bug-audit skill contract: findings only; ordinary audit is report-only outside this campaign evidence channel.",
			"If no concrete finding: omit disposition (clean) or set disposition rejected with empty fingerprint.",
			"If a finding is present: set disposition candidate (or confirmed only with verifier_ref and changed_path_ids) and opaque finding_fingerprint.",
		}, base...), "\n")
	case auditcampaign.PhaseConfirming:
		return strings.Join(append([]string{
			"You are the independent confirmer for supervised deep-bug-audit repair (separate adapter invocation from the auditor).",
			"Independently verify the candidate finding. Do not self-confirm without re-checking evidence.",
			"Set disposition confirmed only when independently verified, with opaque finding_fingerprint, changed_path_ids, and verifier_ref.",
			"Otherwise set disposition rejected.",
		}, base...), "\n")
	case auditcampaign.PhaseFixing:
		paths := strings.Join(camp.AllowedPaths, ",")
		if paths == "" {
			paths = "(none)"
		}
		return strings.Join(append([]string{
			"You are the campaign fixer for supervised deep-bug-audit repair.",
			"Apply a minimal scoped repair only under allowed paths; return typed evidence with disposition fixed.",
			fmt.Sprintf("allowed_paths=%s verifier_profile=%s", paths, camp.VerifierProfile),
			"Set disposition fixed with opaque finding_fingerprint, changed_path_ids, and verifier_ref.",
		}, base...), "\n")
	default:
		return strings.Join(base, "\n")
	}
}

// decodeAdapterCampaignEvidence extracts and validates typed campaign evidence from adapter output.
// Raw Markdown or non-schema JSON fails closed.
func decodeAdapterCampaignEvidence(raw []byte, runID string, cycle int, head string) (auditcampaign.Evidence, error) {
	payload, err := extractCampaignEvidenceBytes(raw)
	if err != nil {
		return auditcampaign.Evidence{}, err
	}
	ev, err := auditcampaign.DecodeEvidence(payload)
	if err != nil {
		return auditcampaign.Evidence{}, fmt.Errorf("typed campaign evidence required (raw Markdown is not commit authority): %w", err)
	}
	// Bind runtime identity; adapters must not invent run/cycle/baseline authority.
	ev.CampaignRun = runID
	ev.Cycle = cycle
	if head != "" && head != "unknown" {
		ev.BaselineHead = head
	} else if ev.BaselineHead == "" {
		ev.BaselineHead = "unknown"
	}
	if err := ev.Validate(); err != nil {
		return auditcampaign.Evidence{}, err
	}
	return ev, nil
}

func extractCampaignEvidenceBytes(raw []byte) ([]byte, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return nil, fmt.Errorf("typed campaign evidence required (empty adapter output)")
	}
	// Fast path: entire payload is the envelope.
	if _, err := auditcampaign.DecodeEvidence(raw); err == nil {
		return raw, nil
	}
	// Locate schema marker inside provider wrappers / mixed stdout.
	marker := []byte(auditcampaign.EvidenceSchema)
	idx := bytes.Index(raw, marker)
	if idx < 0 {
		return nil, fmt.Errorf("typed campaign evidence required (schema %s not found; raw Markdown is not commit authority)", auditcampaign.EvidenceSchema)
	}
	start := bytes.LastIndex(raw[:idx], []byte("{"))
	if start < 0 {
		return nil, fmt.Errorf("typed campaign evidence required (malformed JSON envelope)")
	}
	dec := json.NewDecoder(bytes.NewReader(raw[start:]))
	var msg json.RawMessage
	if err := dec.Decode(&msg); err != nil {
		return nil, fmt.Errorf("typed campaign evidence required: %w", err)
	}
	return msg, nil
}

type fixWrites struct {
	Files map[string]string `json:"files"`
}

func (h *campaignHost) applyFixWrites(cycle int) error {
	path := filepath.Join(h.fixtureDir(), fmt.Sprintf("fix-writes-cycle-%d.json", cycle))
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var w fixWrites
	if err := json.Unmarshal(b, &w); err != nil {
		return fmt.Errorf("fix writes fixture: %w", err)
	}
	allow := map[string]struct{}{}
	for _, p := range h.camp.AllowedPaths {
		allow[filepath.ToSlash(filepath.Clean(p))] = struct{}{}
	}
	for rel, content := range w.Files {
		clean := filepath.ToSlash(filepath.Clean(rel))
		if _, ok := allow[clean]; !ok {
			return fmt.Errorf("fix write path %q is outside allowed_paths", clean)
		}
		if strings.HasPrefix(clean, ".ai/runs/") || clean == ".ai/runs" || strings.HasPrefix(clean, ".git/") {
			return fmt.Errorf("fix write path %q is denied", clean)
		}
		// pathpolicy Abs rejects secret paths and symlink escape outside repo.
		abs, err := pathpolicy.NewDefault().Abs(h.repo, clean)
		if err != nil {
			return fmt.Errorf("fix write path %q: %w", clean, err)
		}
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// Commit performs a coordinator-owned scoped commit with stamp and policy gates.
func (h *campaignHost) Commit(ctx context.Context, e auditcampaign.Evidence) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if !h.camp.CommitEnabled {
		return "", fmt.Errorf("commit_enabled is false")
	}
	if len(h.camp.AllowedPaths) == 0 {
		return "", fmt.Errorf("allowed_paths required for commit")
	}
	msg := strings.TrimSpace(h.camp.CommitMessageTemplate)
	if msg == "" {
		return "", fmt.Errorf("commit_message_template required")
	}
	head, err := gitstate.Head(h.repo)
	if err != nil {
		return "", err
	}
	// Prefer evidence baseline when it matches; reject unauthorized advance.
	if e.BaselineHead != "" && e.BaselineHead != "unknown" && e.BaselineHead != head {
		return "", fmt.Errorf("%s: baseline %s != HEAD %s", auditcampaign.TerminalUnauthorizedHead, e.BaselineHead, head)
	}
	verifier, err := verifierArgv(h.camp.VerifierProfile)
	if err != nil {
		return "", err
	}
	auditPath := filepath.Join(h.repo, ".ai", "audit.jsonl")
	if h.manifest.Governance.AuditLog != "" {
		auditPath = filepath.Join(h.repo, filepath.FromSlash(h.manifest.Governance.AuditLog))
	}
	provName := h.manifest.Governance.Provider
	if provName == "" {
		provName = "noop"
	}
	prov, err := policy.New(provName, auditPath)
	if err != nil {
		return "", err
	}

	res, err := gitstate.CommitScoped(ctx, gitstate.CommitRequest{
		Repo:            h.repo,
		AllowedPaths:    append([]string(nil), h.camp.AllowedPaths...),
		Message:         msg,
		Verifier:        verifier,
		VerifierTimeout: 2 * time.Minute,
		BaseHead:        head,
		StampCheck: func(repo, headSHA, indexHash string, paths []string) error {
			// Post-stage fresh stamp then immediate freshness check.
			_, err := preflight.Run(preflight.Context{
				Repo:              repo,
				BroadVerifiers:    []string{h.camp.VerifierProfile},
				FocusedVerifiers:  []string{h.camp.VerifierProfile},
				PipelinePreflight: true,
			})
			if err != nil {
				return err
			}
			_, err = preflight.CheckStamp(repo)
			return err
		},
		PolicyCheck: func(repo, headSHA, indexHash string) error {
			stamp, err := preflight.CheckStamp(repo)
			if err != nil {
				return err
			}
			stampRef := stamp.Head + ":" + stamp.DiffSHA256
			d, err := prov.Decide(ctx, policy.Action{
				Kind:          policy.ActionProtect,
				ProtectedKind: policy.ProtectedCommit,
				Stamp:         stampRef,
				RunID:         h.runID,
				Step:          "campaign-commit",
			})
			if err != nil {
				return err
			}
			if !d.Allowed {
				return fmt.Errorf("policy denied: %s", d.Reason)
			}
			return nil
		},
	})
	if err != nil {
		return "", err
	}
	h.expectedHead = res.SHA
	return res.SHA, nil
}

// verifierArgv maps a campaign verifier_profile to an argv array (no shell).
// Multi-word free-form profiles fail closed; they never silently map to true.
func verifierArgv(profile string) ([]string, error) {
	profile = strings.TrimSpace(profile)
	switch profile {
	case "", "true", "noop", "true-cmd":
		return []string{"true"}, nil
	case "go-test":
		return []string{"go", "test", "./..."}, nil
	default:
		// Multi-word profiles are not shell-expanded; fail closed (no silent true).
		if strings.ContainsAny(profile, " \t\n\r") {
			return nil, fmt.Errorf("verifier_profile %q is multi-word; use a named profile (true, go-test) or a single PATH token", profile)
		}
		if strings.ContainsAny(profile, "*?[]{}") {
			return nil, fmt.Errorf("verifier_profile %q contains shell/glob metacharacters", profile)
		}
		return []string{profile}, nil
	}
}
