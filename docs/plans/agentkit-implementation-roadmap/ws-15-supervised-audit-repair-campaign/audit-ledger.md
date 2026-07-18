# WS15 Audit Ledger

Version: 1  
Format: mivia-agent-audit-ledger/v1  
Purpose: non-sensitive, committed closure evidence for supervised phase audits.  
Rules: no raw prompts, raw model output, secrets, absolute paths, or `.ai/runs/**` payloads.

| Phase | Commit range | Audit round | Auditors | Results | ResidualRisk | Closure commit | Notes |
| --- | --- | --- | --- | --- | --- | --- | --- |
| 0 | `40f6aa9..67f8df9` | final | correctness; security; tests | PASS; PASS; PASS | none | see phase0 | dual-home telemetry/plan gates; protect:commit≠Git; WS15 artifacts |
| 1–3 | prior tip..a51238b | implementation | tests | PASS | none | a51238b | config/evidence/state/CommitScoped; Windows runner fix |
| 4–6 | a51238b..HEAD | implementation+suite | tests; correctness self-check; security self-check | PASS; PASS; PASS | live dual-CLI install not exercised in CI (wiring covered by fakes) | this-commit | local + orchestrable adapters + scoped commit + built-binary |
| adapters | prior..HEAD | orchestrable wiring | tests; correctness self-check | PASS; PASS | live dual-CLI install not exercised in CI | this-commit | codex/claude/etc. invoke path + typed evidence; local fixtures retained |
| phase1-audit-fix | prior..HEAD | independent deep-bug-audit cycle 1 | correctness; security; evidence; tests; docs + independent confirm | PASS after fix | live dual-CLI not in CI; dedicated campaign worktree deferred (commits in --repo + allowlist); mid-phase resume restarts at audit boundary | this-commit | candidate→confirm gate; commit_enabled=false; multi-word verifier fail-closed; pathpolicy+globs; non-zero exit on fail; resume re-runs engine |
| phase1-audit-fix-2 | prior..HEAD | independent deep-bug-audit cycle 2 | correctness; security; evidence/tests; docs + confirm | PASS after residual fix | live dual-CLI not in CI; worktree deferred; opaque path-id→path map not yet binding (stages allowlist ∩ actual staged; secrets under dir deny); MaxRepairAttempts unused (cycle/no_progress caps still bound) | this-commit | cumulative DurationUsedMS; confirm fingerprint bind; path-like verifier reject; staged secret under dir allowlist; verifier required |
| phase1-stop-clean | 570afb1 | cycles 3+4 independent audit+confirm | multi-lens + independent confirmers | PASS; PASS | live dual-CLI cost/flakiness; worktree deferred; path-id map; MaxRepairAttempts unused under caps | 570afb1 | two consecutive empty full-scope rounds |
| phase2-live | 570afb1 | live dual-CLI finite run | operator + product fail-closed | PASS (fail-closed evidence) | providers may not emit schema-valid evidence; cost/flaky; dogfood left disabled | n/a product | codex+claude approved; terminal verification_failed unknown field; self-confirm misconfig exit 2; dogfood disabled |
| skeptic-repair | prior..HEAD | skeptic fail-open repair | tests + independent re-audit | PASS after fix | accepted residual unchanged | this-commit | ConfirmedFindings not clean; invent-confirm blocked; fix fp must match confirm |
| gap-closure | prior..HEAD | live thrash + commit path gaps | tests + offline orch commit | PASS | live dual-CLI may still reject findings; worktree deferred | bf7718a | no_progress on unique fps; prior evidence; normalize; ignore .ai/runs dirt |

## Phase 0 entry

```text
Phase: 0
CommitRange: 40f6aa9..67f8df9
AuditRound: final (after repair rounds)
Auditors: correctness | security | tests
Results: PASS ; PASS ; PASS
ResidualRisk: none
ClosureCommit: (this commit)
Verification:
  python3 scripts/validate_agent_plan.py .ai/plans/supervised-deep-bug-audit-repair-campaign.plan.json
  python3 scripts/test_report_telemetry_contracts.py
  python3 scripts/test_agent_plan_contracts.py
  python3 scripts/test_skill_contracts.py
  python3 scripts/test_git_hooks.py
  python3 scripts/verify_agent_config.py
  make telemetry-contract-test
FindingsFixed: Phase0 dual-home hook wiring, allowlist path encode/decode, false-commit synonym windows
MutationProofs:
  remove NOT_MEASURED from agent-report-v1 -> telemetry contract fails
  remove telemetry from pre-commit -> telemetry wiring test fails
  inject ws-00 files_edit -> verify_agent_config fails
```

## Phase 4–6 entry

```text
Phase: 4-6
CommitRange: a51238b..HEAD
AuditRound: final implementation suite
Auditors: correctness | security | tests
Results: PASS ; PASS ; PASS
ResidualRisk: live dual-CLI install not exercised in CI (orchestrable wiring covered by scripted adapters + local fixtures)
Verification:
  go test ./... -count=1
  go vet ./...
  go build ./cmd/mivia-agent
  python3 scripts/verify_agent_config.py
  make agent-hook-test audit-loop-test skill-contract-test
FindingsFixed:
  Phase4 placeholder adapters and unwired Commit
  continuous requiring TTY for finite non-continuous runs
  structural-only built-binary test
  template/docs campaign parity gaps
  external agent auditor/confirmer/fix adapters not invoked (now wired)
MutationProofs:
  TestEngineRejectsNonInteractive / TestEngineFiniteRunWithoutContinuousTTY
  TestCampaignCLIRejectsNonInteractiveContinuous
  TestCampaignCLIBuiltBinaryIntegration / TestCampaignCLIBuiltBinaryScopedCommit
  TestCampaignCLIRejectsSelfConfirmCommit
  TestCampaignHostInvokesIndependentOrchestrableAdapters
  TestCampaignHostRejectsRawMarkdownAsEvidence / TestNewCampaignHostFailsClosedWhenExternalNotApproved
  gitstate CommitScoped dirty/denied/stamp/policy tests
```

## Orchestrable adapter wiring entry

```text
Phase: adapters (post 4-6 residual clear)
CommitRange: prior..HEAD
AuditRound: implementation suite
Auditors: tests | correctness self-check
Results: PASS ; PASS
ResidualRisk: live dual-CLI install not exercised in CI
Verification:
  go test ./internal/cli -count=1
  go test ./... -count=1
  go vet ./...
  go build ./cmd/mivia-agent
FindingsFixed:
  campaignHost hard-failed non-local adapters with "external agent wiring is unavailable"
  residual risk claimed external adapters not wired
MutationProofs:
  TestCampaignHostInvokesIndependentOrchestrableAdapters (distinct audit+confirm Run calls)
  TestCampaignHostRejectsRawMarkdownAsEvidence
  TestCampaignHostRejectsMissingAdapterInRegistry
  TestNewCampaignHostFailsClosedWhenExternalNotApproved
  TestCampaignHostLocalFixtureStillWorks
```

## Phase 1 independent deep-bug-audit repair entry

```text
Phase: phase1-audit-fix
CommitRange: prior..HEAD
AuditRound: 1 (parallel auditors + independent confirmer)
Auditors: correctness | security | evidence | tests | docs
Confirmer: independent subagent (code proof)
Results: PASS after fix (12 confirmed findings repaired)
ResidualRisk: live dual-CLI install not exercised in CI; dedicated worktree lifecycle deferred (scoped commits in --repo); mid-phase resume restarts at next audit boundary with cumulative budget
FindingsFixed:
  F-CORR-3 candidates always Confirm
  F-CORR-1 commit_enabled=false audit→confirm only
  F-CORR-2 bare confirmed requires CommitEligible
  F-SEC-5 Fixed requires paths+verifier
  F-SEC-1 multi-word verifier_profile fail-closed
  F-SEC-2/3 pathpolicy secrets + literal paths only
  F-CORR-6 non-success terminal exit non-zero
  F-CORR-9 fingerprint-only not clean
  F-CORR-8 unauthorized HEAD test
  F-DOC-1 resume re-enters Engine.Run
  CommitScoped requires stamp+policy hooks; failed verifier redacted
Verification:
  go test ./internal/cli ./internal/adapter ./internal/auditcampaign ./internal/gitstate ./internal/config -count=1
  go test ./... -count=1
  go vet ./...
  go build ./cmd/mivia-agent
  python3 scripts/verify_agent_config.py
```

## Entry template

```text
Phase: <n>
CommitRange: <base>..<head>
AuditRound: <n>
Auditors: correctness | security | tests
Results: PASS|BLOCK|PARTIAL ; PASS|BLOCK|PARTIAL ; PASS|BLOCK|PARTIAL
ResidualRisk: none|<exact>
ClosureCommit: <sha>
Verification: <commands>
FindingsFixed: none|<ids>
```
