---
name: deep-bug-audit
description: Broad bug-finding workflow for correctness, reliability, security, data loss, concurrency, UX, persistence, hooks, adapters, and test gaps. Use before risky merges or when asked for a deep bug hunt.
triggers:
  - deep bug audit
  - bug hunt
  - production readiness audit
  - broad risk review
---

# Deep Bug Audit

## Read First

- `AGENTS.md`
- `.ai/INDEX.md`
- `.ai/rules/00-operating-doctrine.md`
- `.ai/rules/10-security-privacy.md`
- `.ai/rules/20-agent-quality.md`
- The relevant `docs/plans/agentkit-implementation-roadmap/ws-XX/tasks.md`

## Method

1. Define the exact scope and excluded areas.
2. Map inputs, outputs, persistence, protected actions, and error boundaries.
3. Inspect fail-closed behavior, idempotency, path traversal, secret handling, concurrency, and no-network invariants.
4. Check that tests exercise real boundaries rather than mocks of the thing under test.
5. For each finding, provide file, symbol, failure mode, user impact, and the test that should catch it.

## Supervised Campaign Boundary

Ordinary deep-bug-audit is **report-only**. It must not stage, commit, push, open a PR, or rewrite Git history.

Commit-capable audit→confirm→fix→verify→scoped-commit cycles use the separate supervised campaign surface only:

```bash
./mivia-agent campaign run --repo . --campaign deep-bug-audit-repair --json
./mivia-agent campaign status --repo . --run <id> --json
```

Campaigns are disabled by default in `mivia-agent.yaml`, require an independently configured confirmer for commit-capable mode, and are never activated by prompt substring matching or the host audit-loop hook. A one-adapter self-hosted setup fails closed for commit-capable campaigns.

## Required Report

Never invent elapsed time, duration, tokens, cost, throughput, or efficiency numbers; use runtime-owned metrics or `NOT_MEASURED`.

Always use `mivia-agent-report/v1` from `.ai/templates/agent-report-v1.md`. Keep the report strict and concise; do not add free-form sections unless the user asks for a long artifact.

Result semantics:

- `PASS` means no concrete bug path remains in scope and required verification ran.
- `BLOCK` means at least one concrete bug, missing test, unsafe bypass, or failed verifier remains.
- `PARTIAL` means the audit found useful evidence but scope, tooling, or gated runtime proof is incomplete.
- `NOT_RUN` means the response is only a plan or the audit could not start.

Severity never gates approval; every open gap must be fixed. Low-severity gaps still require `BLOCK` or `PARTIAL` until fixed.

```md
ReportFormat: mivia-agent-report/v1
Skill: deep-bug-audit
Result: PASS|BLOCK|PARTIAL|NOT_RUN
Scope: <exact files/packages>
Baseline: <branch/commit/diff>
Summary: <one sentence>

| ID | Severity | Status | File:Line | Finding | Required Fix | Required Test | Mutation |
| --- | --- | --- | --- | --- | --- | --- | --- |
| none | none | closed | none | none | none | none | none |

| Command | Result | Notes |
| --- | --- | --- |
| none | NOT_RUN | none |

ResidualRisk: none|<short exact risk>
NextAction: none|<exact task>
```
