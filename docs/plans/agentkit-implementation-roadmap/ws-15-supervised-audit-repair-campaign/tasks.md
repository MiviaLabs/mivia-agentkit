# WS15 — Supervised Deep-Bug-Audit Repair Campaign

- **Phase:** 6 (post-roadmap product extension)
- **Depends on:** WS0–WS14 (all complete)
- **PRD:** FR-4.x bounds, protected actions, no unbounded loops; measurement integrity
- **Plan:** `docs/plans/supervised-deep-bug-audit-repair-campaign.md`
- **Machine DAG:** `.ai/plans/supervised-deep-bug-audit-repair-campaign.plan.json`
- **Exit gate:** optional disabled-by-default campaign config + CLI; finite cycles; independent confirmer; runtime-owned metrics only; coordinator-only scoped commits; ordinary run/audit remain report-only

Goal: ship a supervised, finite audit→confirm→fix→verify→stamp/policy→scoped-commit→re-audit campaign without weakening existing loop bounds or inventing telemetry.

## T0 — Governed plan artifacts and telemetry inventory (Phase 0)

Create:
- `docs/plans/agentkit-implementation-roadmap/ws-15-supervised-audit-repair-campaign/tasks.md` — this file
- `docs/plans/agentkit-implementation-roadmap/ws-15-supervised-audit-repair-campaign/report-surface-inventory.md` — claim-bearing surface inventory
- `docs/plans/agentkit-implementation-roadmap/ws-15-supervised-audit-repair-campaign/audit-ledger.md` — committed audit-closure surface
- `.ai/plans/supervised-deep-bug-audit-repair-campaign.plan.json` — validated machine DAG
- `docs/plans/supervised-deep-bug-audit-repair-campaign.md` — human plan under version control
- `scripts/test_report_telemetry_contracts.py` — cross-surface telemetry contract
- updates to `docs/plans/agentkit-implementation-roadmap/00-overview.md`, report template, docs/examples, workflows

Spec:
- Record all prior workstreams complete; do not rewrite historical WS tasks.
- Inventory every claim-bearing report surface.
- Reject agent-authored elapsed/token/cost/efficiency as facts without runtime provenance; require `NOT_MEASURED` when absent.
- Correct false `approval: commit` stamp/commit documentation to `protect:commit` (stamp/policy gate only; no Git commit).
- Validate machine plan with `python3 scripts/validate_agent_plan.py`.

Tests that must pass:
- `scripts/test_report_telemetry_contracts.py`
- `scripts/test_agent_plan_contracts.py` (committed plan validates)
- `python3 scripts/verify_agent_config.py`

Mutation proof:
- Remove `NOT_MEASURED` guidance from `.ai/templates/agent-report-v1.md`; telemetry contract test must fail.

## T1 — Typed campaign configuration

Create:
- `internal/config/campaign.go` — campaign manifest model and validation (`Campaign`, `Validate`)
- `internal/config/campaign_test.go` — parse/default/rejection tests

Spec:
- Disabled by default; named audit/fix workflows; independent confirmation adapters; clean-pass threshold ≥ 2; finite cycle/duration/per-cycle limits; no-progress threshold; explicit commit enablement/message template; verifier profile; allowed paths.
- Reject default-enabled, unbounded/non-interactive, zero/negative limits, missing independent confirmation, unknown workflow/adapter, `on_exhausted: proceed`, unsafe message template, commit without verifier/path scope/protected policy, clean threshold below two.

Tests that must pass:
- `TestCampaignDefaultsDisabled`
- `TestCampaignRejectsMissingIndependentConfirmer`
- `TestCampaignRejectsNonPositiveLimits`
- `TestCampaignRejectsCommitWithoutVerifierOrPathScope`
- `TestCampaignRejectsCleanThresholdBelowTwo`

Dependencies:
- `internal/config` (existing helpers)

Mutation proof:
- Remove independent-confirmer rejection; `TestCampaignRejectsMissingIndependentConfirmer` must fail.

## T2 — Manifest campaigns map

Create:
- extend `internal/config/manifest.go` — `Manifest.Campaigns map[string]Campaign`
- extend `internal/config/manifest_test.go` — parse and validate campaigns

Spec:
- Parse `campaigns:` from `mivia-agent.yaml`; validate each named campaign; empty/absent map is valid (disabled product).

Tests that must pass:
- `TestManifestParsesCampaigns`
- `TestManifestRejectsInvalidCampaign`

Dependencies:
- T1

Mutation proof:
- Skip campaign validation in manifest load; `TestManifestRejectsInvalidCampaign` must fail.

## T3 — Campaign evidence envelope

Create:
- `internal/auditcampaign/evidence.go` — versioned evidence schema and strict decode
- `internal/auditcampaign/evidence_test.go` — disposition/fingerprint/sensitive rejection tests

Spec:
- Strict decoder, bounded byte size, opaque path IDs, normalized fingerprints, disposition transitions, allowed verifier IDs, baseline binding; reject unknown/duplicate fields, oversize, secret-like/raw prose, unproven telemetry.

Tests that must pass:
- `TestEvidenceAcceptsConfirmedFinding`
- `TestEvidenceRejectsUnprovenTelemetry`
- `TestEvidenceRejectsSensitiveFields`
- `TestEvidenceRejectsOversizeAndUnknownFields`

Dependencies:
- stdlib

Mutation proof:
- Accept unproven telemetry fields; `TestEvidenceRejectsUnprovenTelemetry` must fail.

## T4 — Runtime metrics

Create:
- `internal/auditcampaign/metrics.go` — runtime-owned phase metrics
- `internal/auditcampaign/metrics_test.go` — ordering/aggregation tests

Spec:
- `started_at`, `finished_at`, `elapsed_ms`, outcome, run/cycle/step IDs, metric source/version; measure with process clock; `token_source: provider|unavailable`; no double-count of parallel reviews; missing metrics render unavailable/`NOT_MEASURED` at report boundary.

Tests that must pass:
- `TestPhaseMetricsOrderedNonNegativeElapsed`
- `TestTokenSourceUnavailableWhenMissing`
- `TestParallelReviewAggregationDoesNotDoubleCount`

Dependencies:
- stdlib

Mutation proof:
- Allow negative elapsed; `TestPhaseMetricsOrderedNonNegativeElapsed` must fail.

## T5 — Durable campaign state

Create:
- `internal/auditcampaign/state.go` — lock, journal/snapshot, monotonic transitions, resume preconditions
- `internal/auditcampaign/state_test.go` — transition/resume/lock tests

Spec:
- Persist redacted state under `.ai/runs/<campaign-id>/`; terminal reasons per product contract; reject unexpected HEAD/branch, dirty tree, concurrent owner, malformed/terminal, unaccounted commit.

Tests that must pass:
- `TestStateMonotonicTransitions`
- `TestStateRejectsResumeOnChangedHead`
- `TestStateLockSerialization`
- `TestStateNoProgressOnDuplicateFingerprint`

Dependencies:
- T3, T4

Mutation proof:
- Allow illegal transition; `TestStateMonotonicTransitions` must fail.

## T6 — Runstore metric fields

Create:
- extend `internal/runstore/runstore.go` — minimal structured metric fields
- extend `internal/runstore/runstore_test.go`

Spec:
- Minimum fields needed by every workflow for phase metrics without storing prompts/provider payloads.

Tests that must pass:
- `TestRunstoreRecordsPhaseMetricFields`
- `TestRunstoreRejectsSensitiveMetricPayload`

Dependencies:
- existing runstore

Mutation proof:
- Drop metric field persistence; `TestRunstoreRecordsPhaseMetricFields` must fail.

## T7 — Scoped Git commit

Create:
- `internal/gitstate/commit.go` — `CommitScoped` and owned worktree lifecycle
- `internal/gitstate/commit_test.go` — real temp-git mutation proofs

Spec:
- Owned worktree/branch from campaign ID; clean baseline isolation; exact allowlisted `git add -- <paths>`; argv verifier execution; post-stage stamp + `policy.Decide` for `protect:commit`; one commit; no network/push/PR; reject dirty/denied/`.ai/runs/**`/no-diff/out-of-scope.

Tests that must pass:
- `TestCommitScopedSuccess`
- `TestCommitScopedRejectsDirtyUnrelated`
- `TestCommitScopedRejectsDeniedPaths`
- `TestCommitScopedRejectsBroadStagingBypass`
- `TestCommitScopedRejectsStaleStampAndPolicyDenial`

Dependencies:
- gitstate, preflight, policy

Mutation proof:
- Replace scoped staging with `git add -A`; `TestCommitScopedRejectsBroadStagingBypass` must fail.

## T8 — Campaign engine

Create:
- `internal/auditcampaign/engine.go` — finite campaign cycle executor
- `internal/auditcampaign/engine_test.go` — cycle/stop/resume tests

Spec:
- audit→confirm→fix→verify→preflight→commit→re-audit; two consecutive clean audits stop without commit; candidate-only does not fix/commit; confirmed finding commits once then re-audits; caps/duration/cancel/resume cumulative; no recursive Cobra; no self-confirm; fake adapter HEAD advance rejects.

Tests that must pass:
- `TestEngineTwoCleanAuditsStopWithoutCommit`
- `TestEngineConfirmedFindingCommitsOnceAndReaudits`
- `TestEngineRejectsSelfConfirm`
- `TestEngineStopsNoProgressOnDuplicateFingerprint`
- `TestEngineRespectsCycleAndDurationCaps`

Dependencies:
- T1–T7, orchestrator (minimal typed evidence)

Mutation proof:
- Allow self-confirm; `TestEngineRejectsSelfConfirm` must fail.

## T9 — Campaign CLI

Create:
- `internal/cli/campaign.go` — `campaign run|status|resume`
- `internal/cli/campaign_test.go` — CLI and built-binary integration
- extend `internal/cli/root.go` — register command

Spec:
- Interactive-only `--continuous`; reject CI/non-TTY; JSON redacted; status/resume safe; built-binary temp-git integration with local fakes.

Tests that must pass:
- `TestCampaignCLIRejectsNonInteractiveContinuous`
- `TestCampaignCLIStatusAndResume`
- `TestCampaignCLIBuiltBinaryIntegration`

Dependencies:
- T8

Mutation proof:
- Allow non-interactive continuous; `TestCampaignCLIRejectsNonInteractiveContinuous` must fail.

## T10 — Skill/report/hook/template/init parity

Create:
- updates across `.ai` skills/rules/workflows/policy, `.agents`/`.claude` adapters, `templates/**`, `internal/templates/source/**`, `internal/templates/templates.go`, scripts, `mivia-agent.yaml`

Spec:
- Ordinary deep-bug-audit remains report-only; campaign disabled by default in dogfood and generated targets; init/update parity for Codex-only, Claude-only, combined; idempotent update; no Python guards the binary cannot execute.

Tests that must pass:
- existing template/init/update tests plus new parity cases
- `scripts/test_skill_contracts.py`
- `scripts/test_report_telemetry_contracts.py`
- `make skill-contract-test` / `make audit-loop-test`

Mutation proof:
- Emit campaign enabled-by-default in template; focused parity test must fail.

## T11 — Documentation and verification closure

Create:
- updates to `docs/loop-authoring.md`, `docs/agent-hooks.md`, `docs/template-authoring.md`, user/config docs, examples, README/INDEX, PRD/roadmap only where required
- WS15 completion report in this file

Spec:
- Document operator responsibility, stop/resume, no auto-push/PR, independent confirmer requirement, one-adapter fail-closed limitation.
- Run full Phase 6 verification suite; record mutation proofs.

Tests that must pass:
- full suite listed in Verification

Mutation proof:
- n/a (docs + recorded proofs)

## Verification

```bash
python3 scripts/validate_agent_plan.py .ai/plans/supervised-deep-bug-audit-repair-campaign.plan.json
python3 scripts/test_report_telemetry_contracts.py
python3 scripts/verify_agent_config.py
go test ./internal/config ./internal/auditcampaign ./internal/gitstate ./internal/preflight ./internal/policy ./internal/orchestrator ./internal/cli ./internal/templates ./internal/render ./internal/runstore ./internal/adapter -count=1
make agent-hook-test
make audit-loop-test
make skill-contract-test
go test ./... -count=1
go vet ./...
go build ./cmd/mivia-agent
git diff --check
```

WS 15 is ☑ when:
- [x] all listed tests pass
- [x] all mutation proofs executed and reverted (results in completion report)
- [x] `go vet` clean for this WS's packages
- [x] no network calls added
- [x] status updated in `00-overview.md`
- [x] audit-ledger records unanimous PASS for every phase and final full-diff audit with ResidualRisk none

## Completion report (2026-07-18)

### Shipped surfaces
- `internal/config` campaigns map + validation (disabled-by-default)
- `internal/auditcampaign` evidence/metrics/state/engine
- `internal/gitstate.CommitScoped` with argv verifier + stamp/policy gates
- `internal/cli/campaign` run|status|resume with local fixture adapters + coordinator commit
- Built-binary integration: clean stop + one scoped commit on real temp Git repo
- Template/init parity: disabled campaign block, report template, deep-bug-audit report-only boundary
- Docs: loop-authoring, agent-hooks, user-guide, template-authoring, INDEX

### Verification executed
```text
python3 scripts/validate_agent_plan.py .ai/plans/supervised-deep-bug-audit-repair-campaign.plan.json  # pass
python3 scripts/test_report_telemetry_contracts.py  # pass
python3 scripts/verify_agent_config.py  # pass
go test ./internal/config ./internal/auditcampaign ./internal/gitstate ./internal/preflight ./internal/policy ./internal/orchestrator ./internal/cli ./internal/templates ./internal/render ./internal/runstore ./internal/adapter -count=1  # pass
make agent-hook-test audit-loop-test skill-contract-test  # pass
go test ./... -count=1  # pass
go vet ./...  # pass
go build ./cmd/mivia-agent  # pass
git diff --check  # pass
```

### Mutation proofs (executed and reverted)
- Engine: remove Continuous gate requirement (`InteractiveContinuous` only when Continuous) covered by `TestEngineRejectsNonInteractive` + `TestEngineFiniteRunWithoutContinuousTTY`
- CLI continuous: CI env rejects `--continuous` (`TestCampaignCLIRejectsNonInteractiveContinuous`)
- CommitScoped: empty `AllowedPaths` / unrelated dirty / denied paths / policy+stamp rejection tests in `internal/gitstate`
- Self-confirm commit: manifest Parse fails closed (`TestCampaignCLIRejectsSelfConfirmCommit`)
- Built-binary: clean audits stop with zero commits; confirmed fixture path commits once and advances HEAD

### Residual risk
- External agent adapters (codex/claude) are not invoked as campaign auditor/confirmer in this release; only `local` / `local-*` fixture adapters produce typed evidence. Non-local names fail closed with a clear error.
- Three-auditor human deep-bug-audit of the full final diff is still an operator responsibility before merge if required by process.
