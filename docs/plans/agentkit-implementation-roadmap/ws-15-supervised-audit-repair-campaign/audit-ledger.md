# WS15 Audit Ledger

Version: 1  
Format: mivia-agent-audit-ledger/v1  
Purpose: non-sensitive, committed closure evidence for supervised phase audits.  
Rules: no raw prompts, raw model output, secrets, absolute paths, or `.ai/runs/**` payloads.

| Phase | Commit range | Audit round | Auditors | Results | ResidualRisk | Closure commit | Notes |
| --- | --- | --- | --- | --- | --- | --- | --- |
| 0 | `40f6aa9..67f8df9` | final | correctness; security; tests | PASS; PASS; PASS | none | pending-this-commit | dual-home telemetry/plan gates; protect:commit≠Git; WS15 artifacts |

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
