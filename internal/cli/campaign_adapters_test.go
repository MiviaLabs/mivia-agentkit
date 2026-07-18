// Package cli tests campaign adapter host wiring.
// Plan: WS15. PRD: orchestrable campaign adapters, typed evidence, independent confirm.
package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MiviaLabs/mivia-agentkit/internal/adapter"
	"github.com/MiviaLabs/mivia-agentkit/internal/auditcampaign"
	"github.com/MiviaLabs/mivia-agentkit/internal/config"
	"github.com/MiviaLabs/mivia-agentkit/internal/gitstate"
)

// scriptedCampaignAdapter is a test double that records Run calls and returns scripted stdout.
type scriptedCampaignAdapter struct {
	name    string
	role    config.AdapterRole
	stdout  []byte
	exit    int
	runErr  error
	mu      sync.Mutex
	calls   int
	prompts []string
}

func (s *scriptedCampaignAdapter) Name() string { return s.name }
func (s *scriptedCampaignAdapter) Role() config.AdapterRole {
	if s.role == "" {
		return config.AdapterRoleOrchestrable
	}
	return s.role
}
func (s *scriptedCampaignAdapter) Detect(context.Context) (adapter.Detection, error) {
	return adapter.Detection{Name: s.name, Version: "test", HeadlessCapable: true}, nil
}
func (s *scriptedCampaignAdapter) Run(_ context.Context, req adapter.Request) (adapter.Result, error) {
	s.mu.Lock()
	s.calls++
	s.prompts = append(s.prompts, req.Prompt)
	s.mu.Unlock()
	if s.runErr != nil {
		return adapter.Result{}, s.runErr
	}
	// Mirror real adapters that honor ArtifactOut for last-message files.
	if req.ArtifactOut != "" && len(s.stdout) > 0 {
		_ = os.WriteFile(req.ArtifactOut, s.stdout, 0o644)
	}
	return adapter.Result{ExitCode: s.exit, Stdout: s.stdout}, nil
}
func (s *scriptedCampaignAdapter) Review(context.Context, adapter.Request) (adapter.Verdict, error) {
	return adapter.Verdict{Adapter: s.name, Pass: true}, nil
}
func (s *scriptedCampaignAdapter) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func evidenceJSON(disposition, fingerprint string) []byte {
	fp := ""
	if fingerprint != "" {
		fp = fmt.Sprintf(`,"finding_fingerprint":%q`, fingerprint)
	}
	disp := ""
	if disposition != "" {
		disp = fmt.Sprintf(`,"disposition":%q`, disposition)
	}
	extra := ""
	if disposition == "confirmed" || disposition == "fixed" {
		extra = `,"changed_path_ids":["p1"],"verifier_ref":"true"`
	}
	// Re-verifiable handoff so engine hard-gate and host allowlist path stay green in fixtures.
	handoff := ""
	switch disposition {
	case "candidate", "confirmed", "fixed":
		handoff = `,"finding_claim":"test re-verifiable finding claim","path_hints":["internal/seed.go"]`
	default:
		handoff = `,"finding_claim":"","path_hints":[]`
	}
	return []byte(fmt.Sprintf(
		`{"schema":"mivia-agent-campaign-evidence/v1","campaign_run":"placeholder","cycle":0,"baseline_head":"placeholder"%s%s%s%s}`,
		disp, fp, handoff, extra,
	))
}

func testManifestWithAdapters(auditor, confirmer, fixer string) config.Manifest {
	m := config.Defaults()
	m.Adapters = map[string]config.AdapterConfig{
		auditor:   {Enabled: true, Role: config.AdapterRoleOrchestrable},
		confirmer: {Enabled: true, Role: config.AdapterRoleOrchestrable},
		fixer:     {Enabled: true, Role: config.AdapterRoleOrchestrable},
		"local":   {Enabled: true, Role: config.AdapterRoleOrchestrable},
	}
	if auditor == confirmer {
		// still set both keys
	}
	m.Loops = map[string]config.Loop{
		"bug-audit-loop": {
			Bound: "iterations", MaxIterations: 2,
			Steps: []config.Step{{ID: "audit", Producer: auditor}},
		},
		"fix-loop": {
			Bound: "iterations", MaxIterations: 1,
			Steps: []config.Step{{ID: "fix", Producer: fixer}},
		},
	}
	camp := config.CampaignDefaults()
	camp.Enabled = true
	camp.AuditWorkflow = "bug-audit-loop"
	camp.FixWorkflow = "fix-loop"
	camp.Auditor = auditor
	camp.Confirmer = confirmer
	camp.CommitEnabled = false
	camp.MaxCycles = 2
	camp.MaxDuration = "5m"
	m.Campaigns = map[string]config.Campaign{"deep-bug-audit-repair": camp}
	return m
}

func TestCampaignHostInvokesIndependentOrchestrableAdapters(t *testing.T) {
	repo := t.TempDir()
	auditor := &scriptedCampaignAdapter{name: "codex", stdout: evidenceJSON("candidate", "fp-audit-1")}
	confirmer := &scriptedCampaignAdapter{name: "claude", stdout: evidenceJSON("confirmed", "fp-audit-1")}
	reg, err := adapter.NewRegistry(auditor, confirmer)
	if err != nil {
		t.Fatal(err)
	}
	m := testManifestWithAdapters("codex", "claude", "local")
	camp := m.Campaigns["deep-bug-audit-repair"]
	h := &campaignHost{
		repo:         repo,
		runID:        "run-1",
		name:         "deep-bug-audit-repair",
		camp:         camp,
		manifest:     m,
		adapters:     reg,
		expectedHead: "unknown", // non-git unit path; built-binary tests cover real HEAD checks
	}

	aev, err := h.Audit(context.Background(), auditcampaign.PhaseAuditing, 1, auditcampaign.Evidence{})
	if err != nil {
		t.Fatalf("Audit: %v", err)
	}
	if aev.Disposition != auditcampaign.DispositionCandidate {
		t.Fatalf("audit disposition = %q, want candidate", aev.Disposition)
	}
	if aev.CampaignRun != "run-1" || aev.Cycle != 1 {
		t.Fatalf("audit runtime bind = %+v", aev)
	}

	cev, err := h.Confirm(context.Background(), auditcampaign.PhaseConfirming, 1, auditcampaign.Evidence{})
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if cev.Disposition != auditcampaign.DispositionConfirmed {
		t.Fatalf("confirm disposition = %q, want confirmed", cev.Disposition)
	}

	if auditor.callCount() != 1 {
		t.Fatalf("auditor calls = %d, want 1", auditor.callCount())
	}
	if confirmer.callCount() != 1 {
		t.Fatalf("confirmer calls = %d, want 1 (independent invocation)", confirmer.callCount())
	}
	// Prompts must identify roles distinctly.
	if !strings.Contains(auditor.prompts[0], "campaign auditor") {
		t.Fatalf("auditor prompt missing role: %q", auditor.prompts[0])
	}
	if !strings.Contains(confirmer.prompts[0], "independent confirmer") {
		t.Fatalf("confirmer prompt missing independent role: %q", confirmer.prompts[0])
	}
}

func TestCampaignHostInvokesFixOrchestrableAdapter(t *testing.T) {
	repo := t.TempDir()
	fixer := &scriptedCampaignAdapter{name: "codex", stdout: evidenceJSON("fixed", "fp-1")}
	reg, err := adapter.NewRegistry(fixer)
	if err != nil {
		t.Fatal(err)
	}
	m := testManifestWithAdapters("local", "local-confirm", "codex")
	camp := m.Campaigns["deep-bug-audit-repair"]
	camp.AllowedPaths = []string{"fixme.txt"}
	camp.VerifierProfile = "true"
	h := &campaignHost{
		repo: repo, runID: "run-fix", name: "c", camp: camp, manifest: m,
		adapters: reg, expectedHead: "unknown",
	}
	ev, err := h.Fix(context.Background(), auditcampaign.PhaseFixing, 1, auditcampaign.Evidence{})
	if err != nil {
		t.Fatalf("Fix: %v", err)
	}
	if ev.Disposition != auditcampaign.DispositionFixed {
		t.Fatalf("disposition = %q, want fixed", ev.Disposition)
	}
	if fixer.callCount() != 1 {
		t.Fatalf("fixer calls = %d, want 1", fixer.callCount())
	}
	if !strings.Contains(fixer.prompts[0], "campaign fixer") {
		t.Fatalf("fix prompt = %q", fixer.prompts[0])
	}
}

func TestCampaignHostLocalFixtureStillWorks(t *testing.T) {
	repo := t.TempDir()
	fixtureDir := filepath.Join(repo, ".ai", "campaign-fixtures", "deep-bug-audit-repair")
	if err := os.MkdirAll(fixtureDir, 0o755); err != nil {
		t.Fatal(err)
	}
	raw := evidenceJSON("candidate", "fp-local")
	if err := os.WriteFile(filepath.Join(fixtureDir, "audit-cycle-1.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	m := testManifestWithAdapters("local", "local-confirm", "local")
	camp := m.Campaigns["deep-bug-audit-repair"]
	h := &campaignHost{
		repo: repo, runID: "run-local", name: "deep-bug-audit-repair",
		camp: camp, manifest: m, expectedHead: "unknown",
	}
	ev, err := h.Audit(context.Background(), auditcampaign.PhaseAuditing, 1, auditcampaign.Evidence{})
	if err != nil {
		t.Fatalf("Audit local: %v", err)
	}
	if ev.Disposition != auditcampaign.DispositionCandidate || ev.FindingFingerprint != "fp-local" {
		t.Fatalf("ev = %+v", ev)
	}
	// Confirmer without fixture rejects.
	cev, err := h.Confirm(context.Background(), auditcampaign.PhaseConfirming, 1, auditcampaign.Evidence{})
	if err != nil {
		t.Fatalf("Confirm local: %v", err)
	}
	if cev.Disposition != auditcampaign.DispositionRejected {
		t.Fatalf("confirm = %q, want rejected without fixture", cev.Disposition)
	}
}

func TestCampaignHostRejectsMissingAdapterInRegistry(t *testing.T) {
	m := testManifestWithAdapters("codex", "claude", "local")
	camp := m.Campaigns["deep-bug-audit-repair"]
	// Registry has only codex; confirmer claude missing.
	codex := &scriptedCampaignAdapter{name: "codex", stdout: evidenceJSON("", "")}
	reg, err := adapter.NewRegistry(codex)
	if err != nil {
		t.Fatal(err)
	}
	h := &campaignHost{
		repo: t.TempDir(), runID: "r", name: "c", camp: camp, manifest: m,
		adapters: reg, expectedHead: "unknown",
	}
	_, err = h.Confirm(context.Background(), auditcampaign.PhaseConfirming, 1, auditcampaign.Evidence{})
	if err == nil || !strings.Contains(err.Error(), "not installed or not approved") {
		t.Fatalf("error = %v, want not approved/installed", err)
	}
}

func TestCampaignHostRejectsNonOrchestrableRole(t *testing.T) {
	a := &scriptedCampaignAdapter{
		name:   "copilot",
		role:   config.AdapterRoleGuidance,
		stdout: evidenceJSON("candidate", "fp1"),
	}
	reg, err := adapter.NewRegistry(a)
	if err != nil {
		t.Fatal(err)
	}
	m := testManifestWithAdapters("copilot", "claude", "local")
	camp := m.Campaigns["deep-bug-audit-repair"]
	camp.Auditor = "copilot"
	h := &campaignHost{
		repo: t.TempDir(), runID: "r", name: "c", camp: camp, manifest: m,
		adapters: reg, expectedHead: "unknown",
	}
	_, err = h.Audit(context.Background(), auditcampaign.PhaseAuditing, 1, auditcampaign.Evidence{})
	if err == nil || !strings.Contains(err.Error(), "not orchestrable") {
		t.Fatalf("error = %v, want not orchestrable", err)
	}
}

func TestCampaignHostRejectsRawMarkdownAsEvidence(t *testing.T) {
	a := &scriptedCampaignAdapter{
		name:   "codex",
		stdout: []byte("# Report\n\nLooks fine. ResidualRisk: none\n"),
	}
	reg, err := adapter.NewRegistry(a)
	if err != nil {
		t.Fatal(err)
	}
	m := testManifestWithAdapters("codex", "claude", "local")
	camp := m.Campaigns["deep-bug-audit-repair"]
	h := &campaignHost{
		repo: t.TempDir(), runID: "r", name: "c", camp: camp, manifest: m,
		adapters: reg, expectedHead: "unknown",
	}
	_, err = h.Audit(context.Background(), auditcampaign.PhaseAuditing, 1, auditcampaign.Evidence{})
	if err == nil || !strings.Contains(err.Error(), "typed campaign evidence") {
		t.Fatalf("error = %v, want typed campaign evidence rejection", err)
	}
}

func TestCampaignHostRejectsNonZeroExit(t *testing.T) {
	a := &scriptedCampaignAdapter{name: "codex", stdout: evidenceJSON("", ""), exit: 2}
	reg, err := adapter.NewRegistry(a)
	if err != nil {
		t.Fatal(err)
	}
	m := testManifestWithAdapters("codex", "claude", "local")
	camp := m.Campaigns["deep-bug-audit-repair"]
	h := &campaignHost{
		repo: t.TempDir(), runID: "r", name: "c", camp: camp, manifest: m,
		adapters: reg, expectedHead: "unknown",
	}
	_, err = h.Audit(context.Background(), auditcampaign.PhaseAuditing, 1, auditcampaign.Evidence{})
	if err == nil || !strings.Contains(err.Error(), "exited 2") {
		t.Fatalf("error = %v, want exited 2", err)
	}
}

func TestDecodeAdapterCampaignEvidenceExtractsWrappedSchema(t *testing.T) {
	inner := evidenceJSON("candidate", "fp-wrap")
	// Provider wrappers may nest the envelope object; schema marker extraction must still bind runtime fields.
	wrapped := []byte(`{"type":"result","payload":` + string(inner) + `}`)
	ev, err := decodeAdapterCampaignEvidence(wrapped, "run-x", 3, "deadbeef")
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if ev.Disposition != auditcampaign.DispositionCandidate || ev.FindingFingerprint != "fp-wrap" {
		t.Fatalf("ev = %+v", ev)
	}
	if ev.CampaignRun != "run-x" || ev.Cycle != 3 || ev.BaselineHead != "deadbeef" {
		t.Fatalf("runtime bind = %+v", ev)
	}
}

func TestDecodeAdapterCampaignEvidenceExtractsZaiRoleEnvelope(t *testing.T) {
	// Live Zai JSONL puts evidence inside content with outer role field.
	// Outer object must not fail closed as "unknown field role".
	inner := evidenceJSON("confirmed", "fp-zai")
	content, err := json.Marshal(string(inner))
	if err != nil {
		t.Fatal(err)
	}
	wrapped := []byte(`{"role":"assistant","content":` + string(content) + `}`)
	ev, err := decodeAdapterCampaignEvidence(wrapped, "run-zai", 2, "head1")
	if err != nil {
		t.Fatalf("decode zai role envelope: %v", err)
	}
	if ev.Disposition != auditcampaign.DispositionConfirmed || ev.FindingFingerprint != "fp-zai" {
		t.Fatalf("ev = %+v", ev)
	}
	if ev.CampaignRun != "run-zai" || ev.Cycle != 2 || ev.BaselineHead != "head1" {
		t.Fatalf("runtime bind = %+v", ev)
	}
}

func TestCampaignHostZaiRoleArtifactStillDecodes(t *testing.T) {
	inner := evidenceJSON("candidate", "fp-zai-host")
	content, _ := json.Marshal(string(inner))
	stdout := []byte(`{"role":"assistant","content":` + string(content) + `}`)
	a := &scriptedCampaignAdapter{name: "zai", stdout: stdout}
	reg, err := adapter.NewRegistry(a)
	if err != nil {
		t.Fatal(err)
	}
	m := testManifestWithAdapters("zai", "codex", "zai")
	camp := m.Campaigns["deep-bug-audit-repair"]
	camp.Auditor = "zai"
	h := &campaignHost{
		repo: t.TempDir(), runID: "r-zai", name: "c", camp: camp, manifest: m,
		adapters: reg, expectedHead: "unknown",
	}
	ev, err := h.Audit(context.Background(), auditcampaign.PhaseAuditing, 1, auditcampaign.Evidence{})
	if err != nil {
		t.Fatalf("Audit zai role envelope: %v", err)
	}
	if ev.Disposition != auditcampaign.DispositionCandidate || ev.FindingFingerprint != "fp-zai-host" {
		t.Fatalf("ev = %+v", ev)
	}
}

func TestNewCampaignHostFailsClosedWhenExternalNotApproved(t *testing.T) {
	// Force Detect failures by pointing PATH at empty dir so codex/claude are missing.
	empty := t.TempDir()
	t.Setenv("PATH", empty)
	repo := t.TempDir()
	m := testManifestWithAdapters("codex", "claude", "local")
	camp := m.Campaigns["deep-bug-audit-repair"]
	_, err := newCampaignHost(context.Background(), repo, "run", "deep-bug-audit-repair", camp, m)
	if err == nil {
		t.Fatal("want error when external adapters are not installed")
	}
	if !strings.Contains(err.Error(), "unavailable") && !strings.Contains(err.Error(), "not approved") && !strings.Contains(err.Error(), "not installed") {
		t.Fatalf("error = %v, want unavailable/not approved/not installed", err)
	}
}

func TestNewCampaignHostAllowsLocalOnlyWithoutRegistry(t *testing.T) {
	repo := t.TempDir()
	m := testManifestWithAdapters("local", "local-confirm", "local")
	// local-confirm must be in adapters map for Validate; host does not require Detect.
	m.Adapters["local-confirm"] = config.AdapterConfig{Enabled: true, Role: config.AdapterRoleOrchestrable}
	camp := m.Campaigns["deep-bug-audit-repair"]
	camp.Confirmer = "local-confirm"
	h, err := newCampaignHost(context.Background(), repo, "run", "deep-bug-audit-repair", camp, m)
	if err != nil {
		t.Fatalf("newCampaignHost local-only: %v", err)
	}
	if h.adapters != nil {
		t.Fatalf("expected nil registry for local-only host, got %#v", h.adapters)
	}
}

func TestCampaignRequiredAdaptersSkipsLocal(t *testing.T) {
	m := testManifestWithAdapters("codex", "local-confirm", "claude")
	m.Adapters["local-confirm"] = config.AdapterConfig{Enabled: true, Role: config.AdapterRoleOrchestrable}
	camp := m.Campaigns["deep-bug-audit-repair"]
	camp.Confirmer = "local-confirm"
	got := campaignRequiredAdapters(camp, m)
	joined := strings.Join(got, ",")
	if !strings.Contains(joined, "codex") {
		t.Fatalf("got %v, want codex", got)
	}
	if !strings.Contains(joined, "claude") {
		t.Fatalf("got %v, want claude (fix producer)", got)
	}
	for _, n := range got {
		if isLocalCampaignAdapter(n) {
			t.Fatalf("local adapter %q should not be required for Detect", n)
		}
	}
}

func TestExtractCampaignEvidenceBytesRejectsProse(t *testing.T) {
	_, err := extractCampaignEvidenceBytes([]byte("all good, ResidualRisk: none"))
	if err == nil || !strings.Contains(err.Error(), "typed campaign evidence") {
		t.Fatalf("error = %v", err)
	}
}

// TestCampaignHostRealClaudeAdapterEnvelope proves invokeOrchestrable works through
// adapter.Claude (sanitizeProviderOutput drops result) via ArtifactOut materialization.
func TestCampaignHostRealClaudeAdapterEnvelope(t *testing.T) {
	inner := evidenceJSON("confirmed", "fp-claude-host")
	enc, err := json.Marshal(string(inner))
	if err != nil {
		t.Fatal(err)
	}
	raw := []byte(`{"type":"result","subtype":"success","result":` + string(enc) + `}`)
	r := &adapter.FakeRunner{Scripts: map[string]adapter.FakeResponse{
		"claude": {Result: adapter.RunResult{ExitCode: 0, Stdout: raw}},
	}}
	claude := adapter.Claude{Runner: r}
	reg, err := adapter.NewRegistry(claude)
	if err != nil {
		t.Fatal(err)
	}
	m := testManifestWithAdapters("claude", "claude", "local")
	// Self-confirm allowed only when commit disabled; here we only exercise Audit path.
	camp := m.Campaigns["deep-bug-audit-repair"]
	camp.Auditor = "claude"
	camp.Confirmer = "local-confirm"
	m.Adapters["local-confirm"] = config.AdapterConfig{Enabled: true, Role: config.AdapterRoleOrchestrable}
	h := &campaignHost{
		repo: t.TempDir(), runID: "run-claude", name: "c", camp: camp, manifest: m,
		adapters: reg, expectedHead: "unknown",
	}
	ev, err := h.Audit(context.Background(), auditcampaign.PhaseAuditing, 1, auditcampaign.Evidence{})
	if err != nil {
		t.Fatalf("Audit through real Claude adapter: %v", err)
	}
	if ev.Disposition != auditcampaign.DispositionConfirmed || ev.FindingFingerprint != "fp-claude-host" {
		t.Fatalf("ev = %+v", ev)
	}
	if len(r.Calls) != 1 {
		t.Fatalf("claude Run calls = %d, want 1", len(r.Calls))
	}
}

// writingFixAdapter returns fixed evidence and writes a file under allowed_paths
// during the fix prompt so CommitScoped has a real scoped diff.
type writingFixAdapter struct {
	scriptedCampaignAdapter
	writeRel string
	body     string
}

func (w *writingFixAdapter) Run(_ context.Context, req adapter.Request) (adapter.Result, error) {
	w.mu.Lock()
	w.calls++
	w.prompts = append(w.prompts, req.Prompt)
	stdout := w.stdout
	if strings.Contains(req.Prompt, "campaign fixer") && w.writeRel != "" && req.Workdir != "" {
		abs := filepath.Join(req.Workdir, filepath.FromSlash(w.writeRel))
		_ = os.MkdirAll(filepath.Dir(abs), 0o755)
		_ = os.WriteFile(abs, []byte(w.body), 0o644)
		stdout = evidenceJSON("fixed", "fp-orch-1")
	}
	w.mu.Unlock()
	if req.ArtifactOut != "" && len(stdout) > 0 {
		_ = os.WriteFile(req.ArtifactOut, stdout, 0o644)
	}
	return adapter.Result{ExitCode: w.exit, Stdout: stdout}, nil
}

func TestCampaignHostEngineOrchestrableScopedCommit(t *testing.T) {
	// Offline commit-capable path: codex audit candidate → zai confirm → zai fix writes → CommitScoped.
	repo := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	run("init")
	run("config", "user.email", "t@example.com")
	run("config", "user.name", "t")
	if err := os.MkdirAll(filepath.Join(repo, "internal"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "internal", "seed.go"), []byte("package internal\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "internal/seed.go")
	run("commit", "-m", "init")
	head, err := gitstate.Head(repo)
	if err != nil {
		t.Fatal(err)
	}

	auditor := &scriptedCampaignAdapter{name: "codex", stdout: evidenceJSON("candidate", "fp-orch-1")}
	// Confirmed without verifier_ref/paths — normalize must fill from campaign.
	confirmer := &scriptedCampaignAdapter{
		name:   "zai",
		stdout: evidenceJSON("confirmed", "fp-orch-1"),
	}
	fixer := &writingFixAdapter{
		scriptedCampaignAdapter: scriptedCampaignAdapter{name: "zai-fix", stdout: evidenceJSON("fixed", "fp-orch-1")},
		writeRel:                "internal/repair.go",
		body:                    "package internal\n// repaired\n",
	}
	// Confirmer and fixer are both zai name for product; use distinct adapters by
	// injecting a registry that maps "zai" once — host looks up by name, so one zai
	// adapter must handle confirm then fix via prompt branching.
	zaiBoth := &writingFixAdapter{
		scriptedCampaignAdapter: scriptedCampaignAdapter{
			name: "zai",
			// default stdout for confirm path
			stdout: evidenceJSON("confirmed", "fp-orch-1"),
		},
		writeRel: "internal/repair.go",
		body:     "package internal\n// repaired\n",
	}
	// Override Run to branch on phase.
	// Use separate registration: confirmer name zai, fix producer must be zai — same Lookup.
	// writingFixAdapter already switches on "campaign fixer" in prompt.
	_ = confirmer
	_ = fixer
	reg, err := adapter.NewRegistry(auditor, zaiBoth)
	if err != nil {
		t.Fatal(err)
	}
	m := testManifestWithAdapters("codex", "zai", "zai")
	camp := m.Campaigns["deep-bug-audit-repair"]
	camp.CommitEnabled = true
	camp.VerifierProfile = "true"
	camp.AllowedPaths = []string{"internal"}
	camp.CommitMessageTemplate = "fix(quality): campaign scoped repair"
	camp.MaxCycles = 4
	camp.CleanPassThreshold = 2
	camp.NoProgressThreshold = 2
	h := &campaignHost{
		repo: repo, runID: "orch-run", name: "deep-bug-audit-repair",
		camp: camp, manifest: m, adapters: reg, expectedHead: head,
		phaseTimeout: time.Minute,
	}
	// clean audits after first commit so engine can stop clean
	auditN := 0
	store := auditcampaign.NewStore(repo, "orch-run", "test")
	eng := &auditcampaign.Engine{
		Campaign:     camp,
		Store:        store,
		BaselineHead: head,
		Audit: func(ctx context.Context, phase auditcampaign.Phase, cycle int, prior auditcampaign.Evidence) (auditcampaign.Evidence, error) {
			auditN++
			if auditN == 1 {
				return h.Audit(ctx, phase, cycle, prior)
			}
			// subsequent cleans
			return h.baseEvidence(cycle), nil
		},
		Confirm: h.Confirm,
		Fix:     h.Fix,
		// Drive real CommitScoped (allowlist staging, argv verifier). Stamp/policy
		// hooks are explicit no-ops here so the test isolates host orchestrable
		// evidence+write path; host.Commit stamp wiring is covered elsewhere.
		Commit: func(ctx context.Context, e auditcampaign.Evidence) (string, error) {
			cur, err := gitstate.Head(repo)
			if err != nil {
				return "", err
			}
			res, err := gitstate.CommitScoped(ctx, gitstate.CommitRequest{
				Repo:         repo,
				AllowedPaths: []string{"internal"},
				Message:      "fix(quality): campaign scoped repair",
				Verifier:     []string{"true"},
				BaseHead:     cur,
				StampCheck:   func(string, string, string, []string) error { return nil },
				PolicyCheck:  func(string, string, string) error { return nil },
			})
			if err != nil {
				return "", err
			}
			h.expectedHead = res.SHA
			return res.SHA, nil
		},
	}
	// First audit uses host which invokes codex scripted adapter.
	res, err := eng.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Commits != 1 {
		t.Fatalf("commits = %d terminal=%s msg=%s, want 1", res.Commits, res.Terminal, res.Message)
	}
	after, err := gitstate.Head(repo)
	if err != nil {
		t.Fatal(err)
	}
	if after == head {
		t.Fatal("HEAD did not advance")
	}
	if _, err := os.Stat(filepath.Join(repo, "internal", "repair.go")); err != nil {
		t.Fatalf("repair file missing: %v", err)
	}
	// Confirm prompt must include prior audit fingerprint.
	if zaiBoth.callCount() < 2 {
		t.Fatalf("zai calls = %d, want >=2 (confirm+fix)", zaiBoth.callCount())
	}
	if !strings.Contains(zaiBoth.prompts[0], "fp-orch-1") {
		t.Fatalf("confirm prompt missing prior fp: %q", zaiBoth.prompts[0])
	}
}

func TestNormalizeCommitEvidenceFillsVerifierAndPaths(t *testing.T) {
	h := &campaignHost{
		camp: config.Campaign{
			CommitEnabled:   true,
			VerifierProfile: "go-test",
			AllowedPaths:    []string{"internal"},
		},
	}
	prior := auditcampaign.Evidence{
		Disposition:        auditcampaign.DispositionCandidate,
		FindingFingerprint: "fp-n1",
	}
	got := h.normalizeCommitEvidence(auditcampaign.Evidence{
		Disposition:        auditcampaign.DispositionConfirmed,
		FindingFingerprint: "fp-n1",
	}, prior, auditcampaign.DispositionConfirmed)
	if got.VerifierRef != "go-test" || len(got.ChangedPathIDs) != 1 || got.ChangedPathIDs[0] != "p1" {
		t.Fatalf("got = %+v", got)
	}
	// Placeholder fp1 from prompt examples rebinds to prior.
	got = h.normalizeCommitEvidence(auditcampaign.Evidence{
		Disposition:        auditcampaign.DispositionConfirmed,
		FindingFingerprint: "fp1",
	}, prior, auditcampaign.DispositionConfirmed)
	if got.FindingFingerprint != "fp-n1" {
		t.Fatalf("placeholder fp not rebound: %+v", got)
	}
	// Confirmed with a different non-placeholder still rebinds to prior chain id.
	got = h.normalizeCommitEvidence(auditcampaign.Evidence{
		Disposition:        auditcampaign.DispositionConfirmed,
		FindingFingerprint: "fp-other-invented",
	}, prior, auditcampaign.DispositionConfirmed)
	if got.FindingFingerprint != "fp-n1" {
		t.Fatalf("confirm fp must rebind to prior: %+v", got)
	}
	// Non-commit campaigns must not invent verifier/path fields.
	h.camp.CommitEnabled = false
	got = h.normalizeCommitEvidence(auditcampaign.Evidence{
		Disposition:        auditcampaign.DispositionConfirmed,
		FindingFingerprint: "fp-n1",
	}, prior, auditcampaign.DispositionConfirmed)
	if got.VerifierRef != "" || len(got.ChangedPathIDs) != 0 {
		t.Fatalf("non-commit normalize leaked fields: %+v", got)
	}
}

func TestCampaignPhasePromptIncludesPriorFinding(t *testing.T) {
	camp := config.CampaignDefaults()
	camp.AllowedPaths = []string{"internal"}
	camp.VerifierProfile = "go-test"
	prior := auditcampaign.Evidence{
		Disposition:        auditcampaign.DispositionCandidate,
		FindingFingerprint: "fp-prior-1",
		ChangedPathIDs:     []string{"p1"},
		FindingClaim:       "nil deref in verifier argv builder",
		PathHints:          []string{"internal/cli/campaign_adapters.go"},
	}
	confirmPrompt := campaignPhasePrompt(auditcampaign.PhaseConfirming, "c", "run1", 2, "abc", camp, prior)
	if !strings.Contains(confirmPrompt, "PRIOR_AUDIT") || !strings.Contains(confirmPrompt, "fp-prior-1") {
		t.Fatalf("confirm prompt missing prior finding: %s", confirmPrompt)
	}
	if !strings.Contains(confirmPrompt, "PRIOR_FINDING_CLAIM=nil deref in verifier argv builder") {
		t.Fatalf("confirm prompt missing PRIOR_FINDING_CLAIM: %s", confirmPrompt)
	}
	if !strings.Contains(confirmPrompt, "PRIOR_PATH_HINTS=internal/cli/campaign_adapters.go") {
		t.Fatalf("confirm prompt missing PRIOR_PATH_HINTS: %s", confirmPrompt)
	}
	if !strings.Contains(confirmPrompt, "finding_claim") || !strings.Contains(confirmPrompt, "path_hints") {
		t.Fatalf("confirm prompt missing handoff field names in examples: %s", confirmPrompt)
	}
	if !strings.Contains(confirmPrompt, "mivia-agent-campaign-evidence/v1") {
		t.Fatalf("confirm prompt missing schema example")
	}
	fixPrior := prior
	fixPrior.Disposition = auditcampaign.DispositionConfirmed
	fixPrior.VerifierRef = "go-test"
	fixPrompt := campaignPhasePrompt(auditcampaign.PhaseFixing, "c", "run1", 2, "abc", camp, fixPrior)
	if !strings.Contains(fixPrompt, "PRIOR_CONFIRM") || !strings.Contains(fixPrompt, "fp-prior-1") {
		t.Fatalf("fix prompt missing prior confirm: %s", fixPrompt)
	}
	if !strings.Contains(fixPrompt, "PRIOR_FINDING_CLAIM=") || !strings.Contains(fixPrompt, "PRIOR_PATH_HINTS=") {
		t.Fatalf("fix prompt missing handoff priors: %s", fixPrompt)
	}
	if !strings.Contains(fixPrompt, "allowed_paths=internal") {
		t.Fatalf("fix prompt missing allowed_paths")
	}
	auditPrompt := campaignPhasePrompt(auditcampaign.PhaseAuditing, "c", "run1", 1, "abc", camp, auditcampaign.Evidence{})
	if !strings.Contains(auditPrompt, "finding_claim") || !strings.Contains(auditPrompt, "path_hints") {
		t.Fatalf("audit prompt must require handoff for candidates: %s", auditPrompt)
	}
}

func TestCampaignHostConfirmPromptReceivesAuditPrior(t *testing.T) {
	confirmer := &scriptedCampaignAdapter{
		name:   "zai",
		stdout: evidenceJSON("confirmed", "fp-prior-1"),
	}
	reg, err := adapter.NewRegistry(confirmer)
	if err != nil {
		t.Fatal(err)
	}
	m := testManifestWithAdapters("codex", "zai", "zai")
	camp := m.Campaigns["deep-bug-audit-repair"]
	camp.Auditor = "codex"
	camp.Confirmer = "zai"
	h := &campaignHost{
		repo: t.TempDir(), runID: "r", name: "c", camp: camp, manifest: m,
		adapters: reg, expectedHead: "unknown",
	}
	prior := auditcampaign.Evidence{
		Schema: auditcampaign.EvidenceSchema, CampaignRun: "r", BaselineHead: "h", Cycle: 1,
		Disposition: auditcampaign.DispositionCandidate, FindingFingerprint: "fp-prior-1",
		FindingClaim: "nil deref in host confirm path", PathHints: []string{"internal/cli/campaign_adapters.go"},
	}
	_, err = h.Confirm(context.Background(), auditcampaign.PhaseConfirming, 1, prior)
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if confirmer.callCount() != 1 {
		t.Fatalf("calls = %d", confirmer.callCount())
	}
	if !strings.Contains(confirmer.prompts[0], "fp-prior-1") {
		t.Fatalf("confirmer prompt missing prior fingerprint: %q", confirmer.prompts[0])
	}
	if !strings.Contains(confirmer.prompts[0], "PRIOR_FINDING_CLAIM=nil deref in host confirm path") {
		t.Fatalf("confirmer prompt missing claim handoff: %q", confirmer.prompts[0])
	}
	if !strings.Contains(confirmer.prompts[0], "PRIOR_PATH_HINTS=internal/cli/campaign_adapters.go") {
		t.Fatalf("confirmer prompt missing path_hints handoff: %q", confirmer.prompts[0])
	}
}

func TestCampaignHostFiltersPathHintsToAllowedPaths(t *testing.T) {
	body := []byte(`{"schema":"mivia-agent-campaign-evidence/v1","campaign_run":"placeholder","cycle":0,"baseline_head":"placeholder","disposition":"candidate","finding_fingerprint":"fp-scope","finding_claim":"","path_hints":["internal/cli/x.go","docs/out.md","cmd/mivia-agent/main.go"],"changed_path_ids":["p1"],"verifier_ref":"","progress":0}`)
	auditor := &scriptedCampaignAdapter{name: "codex", stdout: body}
	reg, err := adapter.NewRegistry(auditor)
	if err != nil {
		t.Fatal(err)
	}
	m := testManifestWithAdapters("codex", "kimi", "kimi")
	camp := m.Campaigns["deep-bug-audit-repair"]
	camp.AllowedPaths = []string{"internal", "cmd"}
	h := &campaignHost{
		repo: t.TempDir(), runID: "r-scope", name: "c", camp: camp, manifest: m,
		adapters: reg, expectedHead: "unknown",
	}
	ev, err := h.Audit(context.Background(), auditcampaign.PhaseAuditing, 1, auditcampaign.Evidence{})
	if err != nil {
		t.Fatalf("Audit: %v", err)
	}
	if len(ev.PathHints) != 2 {
		t.Fatalf("path_hints = %v, want 2 in-scope", ev.PathHints)
	}
	for _, p := range ev.PathHints {
		if p == "docs/out.md" {
			t.Fatalf("out-of-scope path_hint survived: %v", ev.PathHints)
		}
	}
	if !ev.HasReverifiableHandoff() {
		t.Fatalf("in-scope path_hints must count as handoff: %+v", ev)
	}
}

func TestCampaignHostOutOfScopeHintsOnlyLoseHandoff(t *testing.T) {
	// Empty claim + only out-of-scope path_hints → after filter, no handoff.
	body := []byte(`{"schema":"mivia-agent-campaign-evidence/v1","campaign_run":"placeholder","cycle":0,"baseline_head":"placeholder","disposition":"candidate","finding_fingerprint":"fp-oos","finding_claim":"","path_hints":["docs/out.md"],"changed_path_ids":["p1"],"verifier_ref":"","progress":0}`)
	auditor := &scriptedCampaignAdapter{name: "codex", stdout: body}
	reg, err := adapter.NewRegistry(auditor)
	if err != nil {
		t.Fatal(err)
	}
	m := testManifestWithAdapters("codex", "kimi", "kimi")
	camp := m.Campaigns["deep-bug-audit-repair"]
	camp.AllowedPaths = []string{"internal"}
	h := &campaignHost{
		repo: t.TempDir(), runID: "r-oos", name: "c", camp: camp, manifest: m,
		adapters: reg, expectedHead: "unknown",
	}
	ev, err := h.Audit(context.Background(), auditcampaign.PhaseAuditing, 1, auditcampaign.Evidence{})
	if err != nil {
		t.Fatalf("Audit: %v", err)
	}
	if len(ev.PathHints) != 0 {
		t.Fatalf("path_hints = %v, want empty after filter", ev.PathHints)
	}
	if ev.HasReverifiableHandoff() {
		t.Fatalf("expected no handoff after dropping out-of-scope hints: %+v", ev)
	}
}

func TestCampaignHostClaimOnlyKeepsHandoffAfterFilter(t *testing.T) {
	body := []byte(`{"schema":"mivia-agent-campaign-evidence/v1","campaign_run":"placeholder","cycle":0,"baseline_head":"placeholder","disposition":"candidate","finding_fingerprint":"fp-claim","finding_claim":"nil deref in path filter","path_hints":["docs/out.md"],"changed_path_ids":["p1"],"verifier_ref":"","progress":0}`)
	auditor := &scriptedCampaignAdapter{name: "codex", stdout: body}
	reg, err := adapter.NewRegistry(auditor)
	if err != nil {
		t.Fatal(err)
	}
	m := testManifestWithAdapters("codex", "kimi", "kimi")
	camp := m.Campaigns["deep-bug-audit-repair"]
	camp.AllowedPaths = []string{"internal"}
	h := &campaignHost{
		repo: t.TempDir(), runID: "r-claim", name: "c", camp: camp, manifest: m,
		adapters: reg, expectedHead: "unknown",
	}
	ev, err := h.Audit(context.Background(), auditcampaign.PhaseAuditing, 1, auditcampaign.Evidence{})
	if err != nil {
		t.Fatalf("Audit: %v", err)
	}
	if len(ev.PathHints) != 0 {
		t.Fatalf("path_hints should be filtered: %v", ev.PathHints)
	}
	if !ev.HasReverifiableHandoff() || ev.FindingClaim == "" {
		t.Fatalf("claim-only must remain handoff: %+v", ev)
	}
}

func TestCampaignHostHandoffRoundTripCommitEligible(t *testing.T) {
	// Shipped host path: audit candidate with re-verifiable handoff → confirm
	// bound to same fingerprint becomes CommitEligible (paths/verifier filled).
	auditBody := []byte(`{"schema":"mivia-agent-campaign-evidence/v1","campaign_run":"placeholder","cycle":0,"baseline_head":"placeholder","disposition":"candidate","finding_fingerprint":"fp-handoff-1","finding_claim":"off-by-one in path policy join","path_hints":["internal/pathpolicy/pathpolicy.go"],"changed_path_ids":["p1"],"verifier_ref":"","progress":0}`)
	// Confirmer omits claim/path_hints; normalize must rebind from prior for handoff continuity
	// and fill verifier for commit eligibility.
	confirmBody := []byte(`{"schema":"mivia-agent-campaign-evidence/v1","campaign_run":"placeholder","cycle":0,"baseline_head":"placeholder","disposition":"confirmed","finding_fingerprint":"fp-handoff-1","finding_claim":"","path_hints":[],"changed_path_ids":["p1"],"verifier_ref":"","progress":1}`)
	auditor := &scriptedCampaignAdapter{name: "codex", stdout: auditBody}
	confirmer := &scriptedCampaignAdapter{name: "kimi", stdout: confirmBody}
	reg, err := adapter.NewRegistry(auditor, confirmer)
	if err != nil {
		t.Fatal(err)
	}
	m := testManifestWithAdapters("codex", "kimi", "kimi")
	camp := m.Campaigns["deep-bug-audit-repair"]
	camp.CommitEnabled = true
	camp.VerifierProfile = "go-test"
	camp.AllowedPaths = []string{"internal", "cmd"}
	h := &campaignHost{
		repo: t.TempDir(), runID: "r-handoff", name: "c", camp: camp, manifest: m,
		adapters: reg, expectedHead: "unknown",
	}
	aud, err := h.Audit(context.Background(), auditcampaign.PhaseAuditing, 1, auditcampaign.Evidence{})
	if err != nil {
		t.Fatalf("Audit: %v", err)
	}
	if !aud.HasReverifiableHandoff() {
		t.Fatalf("audit missing handoff: %+v", aud)
	}
	cev, err := h.Confirm(context.Background(), auditcampaign.PhaseConfirming, 1, aud)
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if !cev.CommitEligible() {
		t.Fatalf("confirm not CommitEligible after handoff path: %+v", cev)
	}
	if cev.FindingFingerprint != "fp-handoff-1" {
		t.Fatalf("fingerprint rebind failed: %+v", cev)
	}
	if cev.FindingClaim != "off-by-one in path policy join" {
		t.Fatalf("finding_claim not carried from prior: %+v", cev)
	}
	if len(cev.PathHints) != 1 || cev.PathHints[0] != "internal/pathpolicy/pathpolicy.go" {
		t.Fatalf("path_hints not carried from prior: %+v", cev)
	}
	if !strings.Contains(confirmer.prompts[0], "PRIOR_FINDING_CLAIM=off-by-one in path policy join") {
		t.Fatalf("confirm prompt missing audit claim: %q", confirmer.prompts[0])
	}
}

func TestVerifierArgvRejectsMultiWord(t *testing.T) {
	_, err := verifierArgv("go test ./...")
	if err == nil || !strings.Contains(err.Error(), "multi-word") {
		t.Fatalf("error = %v, want multi-word rejection", err)
	}
	argv, err := verifierArgv("go-test")
	if err != nil {
		t.Fatalf("go-test: %v", err)
	}
	if len(argv) < 2 || argv[0] != "go" {
		t.Fatalf("argv = %v, want go test ...", argv)
	}
	argv, err = verifierArgv("true")
	if err != nil || len(argv) != 1 || argv[0] != "true" {
		t.Fatalf("true argv = %v err=%v", argv, err)
	}
}

func TestCampaignHostRejectsUnauthorizedHeadAdvance(t *testing.T) {
	repo := t.TempDir()
	// Minimal git repo so Head works and assertHeadUnchanged can detect advance.
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	run("init")
	run("config", "user.email", "t@example.com")
	run("config", "user.name", "t")
	if err := os.WriteFile(filepath.Join(repo, "README"), []byte("hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "README")
	run("commit", "-m", "init")
	head, err := gitstate.Head(repo)
	if err != nil {
		t.Fatal(err)
	}

	// Adapter that advances HEAD during Run (simulates unauthorized commit).
	advancing := &headAdvancingAdapter{name: "codex", repo: repo, stdout: evidenceJSON("candidate", "fp-head")}
	reg, err := adapter.NewRegistry(advancing)
	if err != nil {
		t.Fatal(err)
	}
	m := testManifestWithAdapters("codex", "claude", "local")
	camp := m.Campaigns["deep-bug-audit-repair"]
	h := &campaignHost{
		repo: repo, runID: "run-head", name: "c", camp: camp, manifest: m,
		adapters: reg, expectedHead: head,
	}
	_, err = h.Audit(context.Background(), auditcampaign.PhaseAuditing, 1, auditcampaign.Evidence{})
	if err == nil || !strings.Contains(err.Error(), string(auditcampaign.TerminalUnauthorizedHead)) {
		t.Fatalf("error = %v, want unauthorized_head_advance", err)
	}
}

// headAdvancingAdapter is a test double that commits during Run to simulate unauthorized HEAD advance.
type headAdvancingAdapter struct {
	name   string
	repo   string
	stdout []byte
}

func (a *headAdvancingAdapter) Name() string             { return a.name }
func (a *headAdvancingAdapter) Role() config.AdapterRole { return config.AdapterRoleOrchestrable }
func (a *headAdvancingAdapter) Detect(context.Context) (adapter.Detection, error) {
	return adapter.Detection{Name: a.name, Version: "test", HeadlessCapable: true}, nil
}
func (a *headAdvancingAdapter) Run(_ context.Context, req adapter.Request) (adapter.Result, error) {
	// Unauthorized commit inside adapter.
	_ = os.WriteFile(filepath.Join(a.repo, "rogue.txt"), []byte("x\n"), 0o644)
	cmd := exec.Command("git", "-C", a.repo, "add", "rogue.txt")
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com",
	)
	_ = cmd.Run()
	cmd = exec.Command("git", "-C", a.repo, "commit", "-m", "rogue")
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com",
	)
	_ = cmd.Run()
	if req.ArtifactOut != "" && len(a.stdout) > 0 {
		_ = os.WriteFile(req.ArtifactOut, a.stdout, 0o644)
	}
	return adapter.Result{ExitCode: 0, Stdout: a.stdout}, nil
}
func (a *headAdvancingAdapter) Review(context.Context, adapter.Request) (adapter.Verdict, error) {
	return adapter.Verdict{Adapter: a.name, Pass: true}, nil
}

func TestCampaignHostClaudePassesJSONSchema(t *testing.T) {
	body := evidenceJSON("candidate", "fp-claude-schema")
	// Claude envelope with structured_output (schema path).
	env, _ := json.Marshal(map[string]any{
		"type":              "result",
		"structured_output": json.RawMessage(body),
		"result":            "ignored prose",
	})
	r := &adapter.FakeRunner{Scripts: map[string]adapter.FakeResponse{
		"claude": {Result: adapter.RunResult{ExitCode: 0, Stdout: env}},
	}}
	reg, err := adapter.NewRegistry(adapter.Claude{Runner: r})
	if err != nil {
		t.Fatal(err)
	}
	m := testManifestWithAdapters("claude", "codex", "claude")
	camp := m.Campaigns["deep-bug-audit-repair"]
	camp.Auditor = "claude"
	h := &campaignHost{
		repo: t.TempDir(), runID: "r-cl-schema", name: "c", camp: camp, manifest: m,
		adapters: reg, expectedHead: "unknown",
	}
	ev, err := h.Audit(context.Background(), auditcampaign.PhaseAuditing, 1, auditcampaign.Evidence{})
	if err != nil {
		t.Fatalf("Audit: %v", err)
	}
	if ev.FindingFingerprint != "fp-claude-schema" {
		t.Fatalf("ev = %+v", ev)
	}
	if len(r.Calls) != 1 {
		t.Fatalf("claude calls = %d", len(r.Calls))
	}
	args := strings.Join(r.Calls[0].Args, " ")
	if !strings.Contains(args, "--json-schema") {
		t.Fatalf("claude args missing --json-schema: %s", args)
	}
	if !strings.Contains(args, "mivia-agent-campaign-evidence/v1") {
		t.Fatalf("claude --json-schema missing campaign schema body: %s", args)
	}
}

func TestSupportsCampaignOutputSchema(t *testing.T) {
	if !supportsCampaignOutputSchema("codex") || !supportsCampaignOutputSchema("claude") {
		t.Fatal("codex and claude must support schema")
	}
	if supportsCampaignOutputSchema("zai") || supportsCampaignOutputSchema("crush") || supportsCampaignOutputSchema("antigravity") {
		t.Fatal("zai/crush/antigravity must not claim schema support")
	}
}

func TestCampaignEvidenceSchemaTwinsMatch(t *testing.T) {
	// Prevent drift between package-local schema copies.
	cliSchema := campaignEvidenceJSONSchema
	other, err := os.ReadFile(filepath.Join("..", "auditcampaign", "evidence_schema.json"))
	if err != nil {
		t.Fatalf("read auditcampaign schema: %v", err)
	}
	if string(cliSchema) != string(other) {
		t.Fatal("cli/campaign_evidence_schema.json and auditcampaign/evidence_schema.json diverged")
	}
	if !bytes.Contains(cliSchema, []byte(`"required"`)) {
		t.Fatal("schema missing required (OpenAI strict needs all properties required)")
	}
	if !bytes.Contains(cliSchema, []byte(`"finding_claim"`)) || !bytes.Contains(cliSchema, []byte(`"path_hints"`)) {
		t.Fatal("schema missing handoff fields finding_claim/path_hints")
	}
}

func TestCampaignHostCodexPassesOutputSchema(t *testing.T) {
	// Provider-enforced structured output: host must pass --output-schema for codex.
	body := evidenceJSON("candidate", "fp-schema")
	r := &adapter.FakeRunner{Scripts: map[string]adapter.FakeResponse{
		"codex": {Result: adapter.RunResult{ExitCode: 0, Stdout: body}},
	}}
	reg, err := adapter.NewRegistry(adapter.Codex{Runner: r})
	if err != nil {
		t.Fatal(err)
	}
	m := testManifestWithAdapters("codex", "zai", "zai")
	camp := m.Campaigns["deep-bug-audit-repair"]
	h := &campaignHost{
		repo: t.TempDir(), runID: "r-schema", name: "c", camp: camp, manifest: m,
		adapters: reg, expectedHead: "unknown",
	}
	ev, err := h.Audit(context.Background(), auditcampaign.PhaseAuditing, 1, auditcampaign.Evidence{})
	if err != nil {
		t.Fatalf("Audit: %v", err)
	}
	if ev.FindingFingerprint != "fp-schema" {
		t.Fatalf("ev = %+v", ev)
	}
	if len(r.Calls) != 1 {
		t.Fatalf("codex calls = %d", len(r.Calls))
	}
	args := strings.Join(r.Calls[0].Args, " ")
	if !strings.Contains(args, "--output-schema") {
		t.Fatalf("codex args missing --output-schema: %s", args)
	}
	if !strings.Contains(args, "--output-last-message") {
		t.Fatalf("codex args missing --output-last-message: %s", args)
	}
}

// TestCampaignHostRealCodexAndClaudeIndependentConfirm drives real Codex+Claude adapters
// with FakeRunner envelopes end-to-end through Audit then Confirm (distinct invocations).
func TestCampaignHostRealCodexAndClaudeIndependentConfirm(t *testing.T) {
	auditBody := evidenceJSON("candidate", "fp-dual")
	confirmBody := evidenceJSON("confirmed", "fp-dual")
	claudeEnc, _ := json.Marshal(string(confirmBody))
	claudeRaw := []byte(`{"type":"result","result":` + string(claudeEnc) + `}`)
	// Codex FakeRunner returns NDJSON with text field (CLI would also write --output-last-message).
	codexLine, _ := json.Marshal(map[string]any{"type": "message", "text": string(auditBody)})

	codexRunner := &adapter.FakeRunner{Scripts: map[string]adapter.FakeResponse{
		"codex": {Result: adapter.RunResult{ExitCode: 0, Stdout: codexLine}},
	}}
	claudeRunner := &adapter.FakeRunner{Scripts: map[string]adapter.FakeResponse{
		"claude": {Result: adapter.RunResult{ExitCode: 0, Stdout: claudeRaw}},
	}}
	reg, err := adapter.NewRegistry(
		adapter.Codex{Runner: codexRunner},
		adapter.Claude{Runner: claudeRunner},
	)
	if err != nil {
		t.Fatal(err)
	}
	m := testManifestWithAdapters("codex", "claude", "local")
	camp := m.Campaigns["deep-bug-audit-repair"]
	h := &campaignHost{
		repo: t.TempDir(), runID: "run-dual", name: "deep-bug-audit-repair",
		camp: camp, manifest: m, adapters: reg, expectedHead: "unknown",
	}

	aev, err := h.Audit(context.Background(), auditcampaign.PhaseAuditing, 1, auditcampaign.Evidence{})
	if err != nil {
		t.Fatalf("Audit codex: %v", err)
	}
	if aev.Disposition != auditcampaign.DispositionCandidate {
		t.Fatalf("audit = %+v", aev)
	}
	cev, err := h.Confirm(context.Background(), auditcampaign.PhaseConfirming, 1, auditcampaign.Evidence{})
	if err != nil {
		t.Fatalf("Confirm claude: %v", err)
	}
	if cev.Disposition != auditcampaign.DispositionConfirmed {
		t.Fatalf("confirm = %+v", cev)
	}
	if len(codexRunner.Calls) != 1 || len(claudeRunner.Calls) != 1 {
		t.Fatalf("calls codex=%d claude=%d, want 1 each (independent confirmer)", len(codexRunner.Calls), len(claudeRunner.Calls))
	}
	// Codex must receive --output-last-message (ArtifactOut wiring).
	codexArgs := strings.Join(codexRunner.Calls[0].Args, " ")
	if !strings.Contains(codexArgs, "--output-last-message") {
		t.Fatalf("codex args missing ArtifactOut flag: %s", codexArgs)
	}
}
