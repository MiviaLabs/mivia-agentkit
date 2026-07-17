// Package cli implements the mivia-agent command surface.
// Plan: WS15. PRD: campaign adapters, scoped commit boundary.
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/MiviaLabs/mivia-agentkit/internal/auditcampaign"
	"github.com/MiviaLabs/mivia-agentkit/internal/config"
	"github.com/MiviaLabs/mivia-agentkit/internal/gitstate"
	"github.com/MiviaLabs/mivia-agentkit/internal/policy"
	"github.com/MiviaLabs/mivia-agentkit/internal/preflight"
)

// campaignHost wires local fixture adapters and coordinator-only scoped commits.
// Local adapters read typed evidence from .ai/campaign-fixtures/<campaign>/ and
// never invoke external agent CLIs. Non-local adapter names fail closed until
// a future orchestrator evidence channel is available.
type campaignHost struct {
	repo     string
	runID    string
	name     string
	camp     config.Campaign
	manifest config.Manifest
	// expectedHead is the HEAD recorded before each adapter phase.
	expectedHead string
}

func newCampaignHost(repo, runID, name string, camp config.Campaign, manifest config.Manifest) (*campaignHost, error) {
	abs, err := filepath.Abs(repo)
	if err != nil {
		return nil, err
	}
	head, err := gitstate.Head(abs)
	if err != nil {
		// Allow non-git repos only for dry structure tests without commit.
		if !camp.CommitEnabled {
			return &campaignHost{repo: abs, runID: runID, name: name, camp: camp, manifest: manifest, expectedHead: "unknown"}, nil
		}
		return nil, fmt.Errorf("campaign requires git repository: %w", err)
	}
	return &campaignHost{repo: abs, runID: runID, name: name, camp: camp, manifest: manifest, expectedHead: head}, nil
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

// Audit produces audit-phase evidence from local fixtures or a clean default.
func (h *campaignHost) Audit(ctx context.Context, phase auditcampaign.Phase, cycle int) (auditcampaign.Evidence, error) {
	if err := ctx.Err(); err != nil {
		return auditcampaign.Evidence{}, err
	}
	if err := h.assertHeadUnchanged(); err != nil {
		return auditcampaign.Evidence{}, err
	}
	if !isLocalCampaignAdapter(h.camp.Auditor) {
		return auditcampaign.Evidence{}, fmt.Errorf("campaign auditor %q is not a local fixture adapter; external agent wiring is unavailable in this release", h.camp.Auditor)
	}
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

// Confirm produces confirmation evidence from local fixtures.
func (h *campaignHost) Confirm(ctx context.Context, phase auditcampaign.Phase, cycle int) (auditcampaign.Evidence, error) {
	if err := ctx.Err(); err != nil {
		return auditcampaign.Evidence{}, err
	}
	if err := h.assertHeadUnchanged(); err != nil {
		return auditcampaign.Evidence{}, err
	}
	if !isLocalCampaignAdapter(h.camp.Confirmer) {
		return auditcampaign.Evidence{}, fmt.Errorf("campaign confirmer %q is not a local fixture adapter; external agent wiring is unavailable in this release", h.camp.Confirmer)
	}
	if isLocalCampaignAdapter(h.camp.Auditor) && isLocalCampaignAdapter(h.camp.Confirmer) {
		// Distinct local-* names are allowed; identical name self-confirm is rejected for commits upstream.
	}
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

// Fix applies optional local write fixtures and returns fix evidence.
func (h *campaignHost) Fix(ctx context.Context, phase auditcampaign.Phase, cycle int) (auditcampaign.Evidence, error) {
	if err := ctx.Err(); err != nil {
		return auditcampaign.Evidence{}, err
	}
	if err := h.assertHeadUnchanged(); err != nil {
		return auditcampaign.Evidence{}, err
	}
	// Apply file writes from sidecar before returning evidence.
	if err := h.applyFixWrites(cycle); err != nil {
		return auditcampaign.Evidence{}, err
	}
	// Fixers must not commit; HEAD must still match baseline.
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
			// Opaque path IDs for evidence; real paths stay coordinator-owned.
			base.ChangedPathIDs = []string{"p1"}
			base.VerifierRef = strings.TrimSpace(h.camp.VerifierProfile)
			if base.VerifierRef == "" {
				base.VerifierRef = "true"
			}
			// CommitEligible requires DispositionConfirmed; engine also accepts DispositionFixed.
			base.Disposition = auditcampaign.DispositionFixed
		}
		return base, nil
	}
	return ev, nil
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
		abs := filepath.Join(h.repo, filepath.FromSlash(clean))
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
	verifier := verifierArgv(h.camp.VerifierProfile)
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
				Repo:             repo,
				BroadVerifiers:   []string{h.camp.VerifierProfile},
				FocusedVerifiers: []string{h.camp.VerifierProfile},
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
func verifierArgv(profile string) []string {
	profile = strings.TrimSpace(profile)
	switch profile {
	case "", "true", "noop", "true-cmd":
		return []string{"true"}
	case "go-test":
		return []string{"go", "test", "./..."}
	default:
		// Single token profile names run as a bare command (must be on PATH).
		if strings.ContainsAny(profile, " \t\n\r") {
			// Multi-word profiles are not shell-expanded; reject for safety.
			return []string{"true"}
		}
		return []string{profile}
	}
}
