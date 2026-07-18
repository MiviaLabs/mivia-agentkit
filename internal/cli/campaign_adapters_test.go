// Package cli tests campaign adapter host wiring.
// Plan: WS15. PRD: orchestrable campaign adapters, typed evidence, independent confirm.
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

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
	return []byte(fmt.Sprintf(
		`{"schema":"mivia-agent-campaign-evidence/v1","campaign_run":"placeholder","cycle":0,"baseline_head":"placeholder"%s%s%s}`,
		disp, fp, extra,
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

	aev, err := h.Audit(context.Background(), auditcampaign.PhaseAuditing, 1)
	if err != nil {
		t.Fatalf("Audit: %v", err)
	}
	if aev.Disposition != auditcampaign.DispositionCandidate {
		t.Fatalf("audit disposition = %q, want candidate", aev.Disposition)
	}
	if aev.CampaignRun != "run-1" || aev.Cycle != 1 {
		t.Fatalf("audit runtime bind = %+v", aev)
	}

	cev, err := h.Confirm(context.Background(), auditcampaign.PhaseConfirming, 1)
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
	ev, err := h.Fix(context.Background(), auditcampaign.PhaseFixing, 1)
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
	ev, err := h.Audit(context.Background(), auditcampaign.PhaseAuditing, 1)
	if err != nil {
		t.Fatalf("Audit local: %v", err)
	}
	if ev.Disposition != auditcampaign.DispositionCandidate || ev.FindingFingerprint != "fp-local" {
		t.Fatalf("ev = %+v", ev)
	}
	// Confirmer without fixture rejects.
	cev, err := h.Confirm(context.Background(), auditcampaign.PhaseConfirming, 1)
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
	_, err = h.Confirm(context.Background(), auditcampaign.PhaseConfirming, 1)
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
	_, err = h.Audit(context.Background(), auditcampaign.PhaseAuditing, 1)
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
	_, err = h.Audit(context.Background(), auditcampaign.PhaseAuditing, 1)
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
	_, err = h.Audit(context.Background(), auditcampaign.PhaseAuditing, 1)
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
	ev, err := h.Audit(context.Background(), auditcampaign.PhaseAuditing, 1)
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
	ev, err := h.Audit(context.Background(), auditcampaign.PhaseAuditing, 1)
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
	_, err = h.Audit(context.Background(), auditcampaign.PhaseAuditing, 1)
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

func (a *headAdvancingAdapter) Name() string                  { return a.name }
func (a *headAdvancingAdapter) Role() config.AdapterRole      { return config.AdapterRoleOrchestrable }
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

	aev, err := h.Audit(context.Background(), auditcampaign.PhaseAuditing, 1)
	if err != nil {
		t.Fatalf("Audit codex: %v", err)
	}
	if aev.Disposition != auditcampaign.DispositionCandidate {
		t.Fatalf("audit = %+v", aev)
	}
	cev, err := h.Confirm(context.Background(), auditcampaign.PhaseConfirming, 1)
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
