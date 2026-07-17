# Supervised Deep-Bug-Audit Repair Campaign

Status: Phase 0 complete — unanimous deep-bug-audit PASS; Phase 1 next

Current phase: 1

Last verified: 2026-07-17 Phase 0 audit PASS on `40f6aa9..67f8df9`

Next action: Implement Phase 1 typed campaign configuration.

## Decision

Implement an **optional, supervised audit-repair campaign**, not a literal unbounded self-committing loop.

"Continuous" means an operator explicitly starts a campaign which runs **finite, individually bounded cycles** while it finds *novel, independently confirmed* bugs. A completed cycle is:

```text
audit -> independent confirmation -> scoped fix -> regression/verifiers
      -> fresh quality stamp + policy decision -> scoped commit -> new baseline audit
```

The campaign stops `clean` after two consecutive independent audits satisfy the same canonical clean predicate: a valid, schema-conforming audit with zero candidate, confirmed, or duplicate findings and `ResidualRisk: none`. It stops `blocked` or `exhausted` on no progress, unsafe Git state, failed verification/stamp/policy/commit, cancellation, duration/cycle limit, or resume inconsistency. An operator may resume or start another bounded campaign; templates, hooks, and CI must never start it by default.

This is the compatible interpretation of the request to continue indefinitely. The present PRD explicitly rejects `bound: budget`, requires positive iteration bounds, and identifies unbounded CI loops as a high risk. Do not weaken those invariants or reinterpret a large numeric cap as unbounded execution.

## Evidence and scope

| Confirmed fact | Source | Consequence |
| --- | --- | --- |
| `config.Loop` requires positive `max_iterations`; the runner rejects `budget` and loops only through that cap. | `internal/config/loop.go`, `internal/orchestrator/loop.go`, PRD FR-4.2 | A campaign is a separate, resumable runtime; do not overload `Loop`. |
| `bug-audit-loop` performs audit then review only. | `.ai/workflows/bug-audit-loop.yaml`, `internal/templates/source/workflows/bug-audit-loop.yaml.tmpl` | It cannot repair, verify, commit, or re-audit a changed baseline. |
| `approval: protect:commit` performs a stamp/policy gate before an adapter step; it does not stage or commit. | `internal/orchestrator/engine.go` (`decide`/`approval`/`protectedKind`), `internal/orchestrator/loop.go` | Add a deterministic, scoped Git commit boundary; agent prose is not commit evidence. |
| The hook audit loop is substring-triggered, capped, report-only, and clears state at its cap. | `.ai/policy/audit-loop.json`, `scripts/audit_loop_guard.py` | It must not become the campaign executor or accidentally activate the campaign. |
| Report v1 has no campaign/baseline/confirmation/verifier/commit/progress fields. | `.ai/templates/agent-report-v1.md` | Add a typed, safe campaign evidence envelope and stricter transition validation. |
| Initialised repositories receive a minimal audit skill and omit report template, audit policy, audit adapters, and guard runtime. | `internal/templates/templates.go`, `internal/templates/source/**` | Align embedded templates, source templates, adapters, and AgentKit dogfood surface. |
| Run traces have timestamped events and adapters may return scrubbed token metadata, but no contract records per-phase start/end/elapsed time or forbids agent-invented efficiency numbers. | `internal/runstore/runstore.go`, `internal/adapter/adapter.go`, `internal/orchestrator/engine.go` | Add runtime-owned measurement and report provenance; never accept model-authored duration, token, cost, or efficiency claims as facts. |

In scope: config, campaign runtime, redacted persisted state, deterministic local Git commits, deep-audit/report contracts, template/init/update parity, hooks/docs, and tests.

Out of scope: push, PR creation, deployment, remote network calls, raw prompt/model-output persistence, auto-start from a generic prompt, removal of existing bounded loop restrictions, and committing the current unrelated dirty worktree changes.

## Product contract

### Explicit opt-in

- Add a distinct `campaigns.<name>` manifest contract, disabled by default; do not infer it from `deep bug audit`, `review this`, or a workflow name.
- Prefer a separate command surface such as `mivia-agent campaign run --campaign deep-bug-audit-repair`; plain `run` preserves current behaviour.
- `--continuous` requires both manifest opt-in and an explicit interactive/operator flag. Reject non-interactive/CI execution unless a future explicit override has its own documented guard.
- Every process invocation has positive cycle and wall-clock limits. Resume is explicit, checks recorded ownership/baseline, and cannot silently prolong a run.
- Require an audit producer and independently executable confirmer/reviewer; reject self-confirmation for a commit-capable campaign.

### Measured telemetry, never invented efficiency

Time, token, cost, throughput, and "efficiency" fields are evidence only when the runtime records them. Campaign evidence parsers must reject agent-authored values without a matching runtime reference. Ordinary Markdown is not a trusted telemetry channel: instructions must require it to omit unproven numeric claims, and report renderers must display `NOT_MEASURED` where the runtime has no source; no generic prose scanner may pretend to validate an assistant's number.

- Add a runtime-owned `RunMetrics`/phase metric contract with `started_at`, `finished_at`, `elapsed_ms`, outcome, run/cycle/step IDs, and a safe metric source/version. Measure elapsed time around the actual coordinator/adapter invocation using the process clock; persist UTC timestamps plus the measured duration, never a model estimate.
- Retain provider token totals only when a scrubbed adapter result actually supplies them. Record `token_source: provider|unavailable`; do not estimate tokens, cost, model latency, parallel efficiency, or user time.
- Aggregate campaign duration from recorded phase metrics, preserving the distinction between wall time, individual step elapsed time, and parallel work. Do not sum overlapping review durations and call it elapsed time.
- Extend human/JSON reports with `TimingSource`, `MeasuredElapsed`, and optional provider-token fields. If a metric cannot be derived from safe runtime records, render `NOT_MEASURED`, not `0`, an approximation, or prose such as "efficient".
- Include metrics in the redacted campaign state/run trace, while excluding prompt/model content, command arguments, paths outside safe repo-relative refs, stderr, secrets, and provider payloads. Metrics must be informational and never relax a safety gate.

Use opaque path IDs and hashes in persisted/reportable evidence. Fingerprints must derive from a canonical, allowlisted rule/category plus a normalized location hash—not raw source text, free-form finding prose, or a literal path which could include personal or secret data. Keep any raw paths only transiently in the coordinator while it applies the scoped diff.

### Safe state machine

Persist redacted state only under the existing ignored `.ai/runs/<campaign-id>/` surface:

```text
created -> auditing -> confirming -> fixing -> verifying -> preflighting
        -> committing -> completed_cycle -> auditing ... -> terminal
```

Store safe IDs, opaque affected-path IDs, baseline/current HEAD and diff hashes, normalized finding fingerprints, verifier/stamp/policy references, commit SHA/message digest, progress count, terminal reason, and a resume token. Never persist raw prompts, model responses, reviewer prose, command stderr, secrets, absolute paths, raw paths, or provider payloads.

Terminal reasons are `clean`, `no_progress`, `cycle_cap`, `duration_cap`, `verification_failed`, `policy_denied`, `commit_failed`, `conflict_or_dirty`, `cancelled`, and `malformed_state`. State transitions must be monotonic, lock-protected, and recorded before each side effect; resume rejects an unexpected branch/HEAD, dirty tree, concurrent owner, malformed/terminal record, or a commit not accounted for in campaign state.

### Finding and report semantics

Keep `mivia-agent-report/v1` for ordinary audits. Add a campaign envelope with safe fields such as `CampaignRun`, `Cycle`, `BaselineHead`, `FindingFingerprint`, `ConfirmedFindings`, `Progress`, `ChangedPaths`, `VerifierRef`, `Commit`, and `Resume`.

- `candidate`, `confirmed`, `duplicate`, `fixed`, and `rejected` are distinct dispositions.
- A commit-eligible finding requires independent confirmation plus an allowed relative affected-path set and verifier identity; a model's assertion or a review consensus pass is insufficient.
- `PASS` is valid for a campaign only after the configured consecutive clean threshold; no candidate, malformed, blocked, or duplicate finding is a clean pass.
- A repeated normalized fingerprint without a campaign commit, a no-diff repair, or a finding surviving its allowed repair attempts terminates `no_progress`.

### Commit boundary

Only the coordinator commits. The first release uses an interactive-only launcher and a dedicated, campaign-owned worktree/branch; a single-adapter/self-confirming setup is unavailable. Derive the branch/ref and worktree identity deterministically from the campaign ID, record only safe branch/ref/ownership-marker data (not an absolute worktree path), locate it through the canonical Git common directory on resume, and retain it until explicit owner cleanup. Never delete, clean, or adopt an unowned worktree.

Audit and confirmation run read-only where the adapter supports it and must leave expected HEAD/status unchanged; otherwise the campaign is unavailable. A fixer runs outside the owned Git worktree and returns only a validated typed patch/evidence channel; the coordinator applies that patch. After every adapter phase and immediately before staging, compare expected HEAD/status and terminate `unauthorized_head_advance` if an adapter committed or wrote outside its authorised boundary. Document any adapter capability the first release cannot technically enforce rather than claiming exclusivity.

The coordinator must perform this exact order:

1. Verify the confirmed finding's scoped change and its focused regression verifier.
2. Reject no diff, pre-existing/unrelated staged or untracked changes, merge/rebase/cherry-pick state, denied paths, `.ai/runs/**`, `.git/**`, and paths outside the accepted allowlist.
3. Run declared verifier profiles as validated argv arrays (no shell), with bounded timeout, safe working directory/environment, exit-code proof, and redacted output digest. A claimed verifier string or `NotRun`/pipeline exception is never sufficient.
4. Stage only validated paths with `git add -- <paths>`. Never use `git add -A`. Compute the candidate index/tree hash and verify it still equals the accepted scoped diff.
5. Write/check a commit-specific fresh stamp bound to base HEAD, the exact staged tree/index hash, and path IDs; then obtain `policy.Decide` for `protect:commit`. The existing status-based stamp must not be reused because staging changes status.
6. Commit with no mutation window, verify exactly one expected HEAD advance and a clean owned worktree, and record only safe commit metadata.

Never push or create a PR. Failed verification, policy, stamp, or commit leaves a resumable blocked state and cannot schedule another audit.

## Implementation phases

### Phase 0 — Establish governed plan artifacts, telemetry inventory, and correct contract drift

Create `docs/plans/agentkit-implementation-roadmap/ws-15-supervised-audit-repair-campaign/tasks.md` in the required one-production-file-plus-test format, add WS15 to `00-overview.md`, and create the validated `.ai/plans/supervised-deep-bug-audit-repair-campaign.plan.json` DAG required by the repository planning contract. Record that all prior workstreams are complete and this workstream must not rewrite historical WS tasks.

Inventory every claim-bearing reporting surface before changing code: canonical `.ai` skills/rules/workflows/templates, `.agents`/`.claude`/`.codex` adapters, `templates/**`, `internal/templates/source/**`, embedded output registration, hooks/scripts, docs/examples, command JSON/text output, and generated target-repo output. Add a cross-surface contract test that fails fabricated or unlabelled elapsed/time/token/cost/efficiency wording and requires runtime-metric provenance or `NOT_MEASURED` guidance everywhere a report is emitted.

Correct documented `approval: commit` claims that do not match the implementation: inspect `docs/examples/README.md`, `docs/examples/zai-glm-examples.md`, `docs/config-examples.md`, `README.md`, and `.ai/workflows/zai-smoke-patch.yaml`; convert them to the real coordinator-managed `protect:commit` stamp/policy gate contract and remove any claim that the approval field itself stages or commits Git.

Acceptance: the workstream/DAG validate; every report surface is inventoried; no documentation says an adapter permission or audit report is itself a Git commit; and no surface treats agent prose as measured telemetry.

### Phase 1 — Typed campaign configuration

Create `internal/config/campaign.go` and `internal/config/campaign_test.go`; extend `internal/config/manifest.go` and its tests with `Manifest.Campaigns`.

The model must represent: disabled-by-default enablement, named audit/fix workflows, independent confirmation adapters, clean-pass threshold, finite cycle/duration/per-cycle limits, no-progress threshold, explicit commit enablement/message template, verifier profile, and allowed paths. Do not add campaign-only fields to generic `config.Loop` except reusable validation helpers.

Reject default-enabled, unbounded/non-interactive, zero/negative duration or limits, missing independent confirmation, unknown workflow/adapter, `on_exhausted: proceed`, unsafe message template, commit without verifier/path scope/protected policy, and clean threshold below two.

Tests/mutations: table-driven parse/default/rejection coverage; remove the independent-reviewer, finite-limit, or disabled-default guard and prove its named test fails.

### Phase 2 — Campaign evidence, telemetry, and durable state

Create `internal/auditcampaign/evidence.go` + test, `internal/auditcampaign/metrics.go` + test, and `internal/auditcampaign/state.go` + test. Extend `internal/runstore/runstore.go` and its tests only with the minimum structured trace/metric fields needed by every workflow, extend `internal/adapter` only to preserve already-scrubbed provider telemetry with explicit provenance, and update the actual command/report renderers (`internal/cli/run.go`, new campaign CLI, and `internal/report` where applicable) to emit only runtime-backed values.

Use a versioned JSON campaign-evidence schema, strict decoder, bounded byte size, and separate per-phase evidence channel rather than parsing unconstrained Markdown for commit authority. Validate opaque affected-path IDs, normalized fingerprints, disposition transitions, allowed verifier IDs, baseline binding, and sensitive-content exclusions; reject duplicate/unknown fields, malformed/oversize payloads, raw prose/secret-like fields, and unproven telemetry values. Generic `ExecuteStep`/`ArtifactOut` is not permitted for commit-capable campaign evidence because it persists raw adapter output. Implement atomic journal/snapshot writes, a lock, monotonic transitions, runtime-owned phase start/finish/elapsed measurements, and resume preconditions. Template/report validation must render `NOT_MEASURED` when the runtime has no source.

Tests/mutations: confirmed versus candidate/duplicate/rejected; malformed/unknown/duplicate-key/oversize envelope; illegal transition; sensitive/path/fingerprint redaction; duplicate fingerprint/no-progress; crash before/after commit; changed HEAD/branch; concurrent lock; cancellation; phase metrics have ordered runtime timestamps and non-negative measured elapsed; missing provider usage renders unavailable; parallel review aggregation does not double-count; no raw adapter prose or secret-like value reaches campaign state. Removing transition validation, baseline matching, locking, strict decoding, or metric provenance must fail focused tests.

### Phase 3 — Deterministic Git operations and policy integration

Create `internal/gitstate/commit.go` + test. Modify `internal/preflight` and `internal/policy` only for safe campaign references; modify `internal/hooks` only to recognise coordinator metadata without weakening existing protected-command handling.

Implement `CommitScoped` using real local Git subprocesses. It must enforce owned-worktree lifecycle, clean baseline/worktree isolation, exact allowlisted staging, actual argv verifier execution, a post-stage index-specific fresh stamp and immediate policy decision, deterministic message validation, one commit, and no network operations.

Tests/mutations use real `git init` repositories in `t.TempDir()`: success; deterministic worktree/ref lifecycle and collision/tamper; unrelated dirty/staged/untracked file; denied/generated run artifact; no diff; changed path outside scope; argv injection/timeout/nonzero verifier; stale pre-stage stamp; index mutation after candidate stamp; policy denial; invalid message; fake adapter commit/write attempt; commit failure and safe resume; cancellation/crash after staging before commit and after commit before journal finalisation. Replacing scoped staging with broad staging, bypassing verifier/stamp/policy, allowing staged-index mutation, or loosening dirty-path/HEAD checks must fail a named test.

### Phase 4 — Campaign engine and CLI

Create `internal/auditcampaign/engine.go` + test and `internal/cli/campaign.go` + test; update `internal/cli/root.go`. Refactor `internal/cli/run.go` only to expose reusable bounded-workflow execution, and update `internal/orchestrator` only to return typed per-cycle evidence when required.

Command contract:

```bash
mivia-agent campaign run --repo . --campaign deep-bug-audit-repair --continuous --json
mivia-agent campaign status --repo . --run <id> --json
mivia-agent campaign resume --repo . --run <id> --json
```

The engine executes finite audit/confirmation/fix/verify/commit phases, respects context cancellation/deadline everywhere, and schedules the next audit only after a verified recorded commit. It must never recursively call Cobra or treat `max_iterations` as a continuation escape hatch. `--continuous` requires actual stdin TTY confirmation and rejects known CI/noninteractive environments; `--json`, a synthetic interactive flag, and resume cannot bypass it. The original deadline/cycle budget is persisted and consumed cumulatively across resumes.

Tests: two canonical-clean audits stop with no commit; candidate-only finding does not fix/commit; one confirmed finding fixes, verifies, commits once, and re-audits the new baseline; failed verification/policy/stamp/no diff/dirty state never commit; repeated fingerprint stops; cap/duration/cancel/resume are safe and cumulative; piped stdin/CI/non-TTY and self-review/missing CLI opt-in reject; fake adapter HEAD advance rejects; JSON is redacted. Add a built-binary/local-fake-adapter integration test using a real temp Git repo.

### Phase 5 — Skill, report, hook, template, init, and update parity

Update this repository's canonical surfaces:

- `.ai/templates/agent-report-v1.md`, `.ai/skills/deep-bug-audit/SKILL.md`, `.agents/skills/deep-bug-audit/SKILL.md`, `.claude/skills/deep-bug-audit/SKILL.md`
- `.ai/workflows/bug-audit-loop.yaml`, `.ai/policy/audit-loop.json`, `.ai/INDEX.md`, `mivia-agent.yaml`
- `scripts/audit_loop_guard.py`, `scripts/test_audit_loop_guard.py`, `scripts/test_skill_contracts.py`, and `scripts/verify_agent_config.py`

Keep ordinary deep-bug-audit report-only. If the host hook remains, it may provide continuation guidance only after explicit campaign opt-in; it cannot execute Git actions or activate from broad prompt substring matching. Prefer porting durable campaign control to the Go binary so generated hooks and dogfood use one runtime.

Ship the complete contract to target repositories by updating both `templates/**` and `internal/templates/source/**`, then register every emitted file in `internal/templates/templates.go` and its tests. This includes the canonical skill, thin `.agents`/`.claude` adapters, report template, disabled campaign policy/config example, and workflow guidance. Add only adapter hook wiring that the binary actually supports; do not install Python guards that the binary cannot execute.

Update init/update/template parity and integration coverage so fresh Codex-only, Claude-only, and combined targets receive every referenced file, preserve user changes on update, stay idempotent, and pass generated configuration validation. AgentKit must use the same disabled-by-default contract itself.

### Phase 6 — Documentation and verification closure

Update `docs/loop-authoring.md`, `docs/agent-hooks.md`, `docs/template-authoring.md`, user/config documentation, examples, README/INDEX links, and PRD/roadmap only where product scope or acceptance criteria changes. State operator responsibility, stop/resume behaviour, no auto-push/PR, supported adapter independence, and the current limitation that a one-adapter self-hosted setup cannot run a commit-capable independent confirmation campaign.

Run narrow checks first, then:

```bash
go test ./internal/config ./internal/auditcampaign ./internal/gitstate ./internal/preflight ./internal/policy ./internal/orchestrator ./internal/cli ./internal/templates ./internal/render -count=1
python3 scripts/verify_agent_config.py
make agent-hook-test
make audit-loop-test
make skill-contract-test
go test ./... -count=1
go vet ./...
go build ./cmd/mivia-agent
git diff --check
```

Run `make verify` if its external prerequisites are present. Record every mutation proof in WS15's completion report; do not claim proof from inspection.

## Security, privacy, and operational review

- Protected action: commits require fresh stamp plus policy decision; human/operator consent remains explicit at campaign launch.
- Git safety: dedicated worktree/branch, scoped staging, no unrelated changes, no push/PR, no force/reset/clean.
- Data safety: campaign state/report evidence is allowlisted metadata only; `.ai/runs/` remains ignored and cannot be staged.
- Measurement integrity: only runtime-recorded timings and scrubbed provider telemetry may be reported. Agent estimates are rejected or displayed as `NOT_MEASURED`; no efficiency claim is an acceptance criterion.
- Availability/cost: cycle, duration, repair-attempt, and no-progress caps prevent runaway agent work; cancellation stops before side effects and is resumable.
- Compatibility: existing `run` workflows and ordinary audit reports remain bounded and unchanged unless the campaign is selected.

## Chosen first-release boundaries

- `--continuous` is interactive-only; there is no CI/noninteractive bypass in this release.
- Each campaign owns a deterministic dedicated worktree/branch; the caller branch is never used for fixes or commits.
- Commit-capable campaigns require a configured, independently executable confirmer and phase capabilities that can be verified locally. A one-adapter setup fails closed; it does not self-confirm.
