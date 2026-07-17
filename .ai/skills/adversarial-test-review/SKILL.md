---
name: adversarial-test-review
description: Pre-merge test review that tries to falsify the implementation and prove tests fail for meaningful mutations. Use for code review, PR review, or before declaring a change safe.
triggers:
  - adversarial test review
  - code review
  - review this
  - safe to ship
---

# Adversarial Test Review

## Read First

- `AGENTS.md`
- `.ai/rules/20-agent-quality.md`
- `.ai/rules/30-go-standards.md`
- Relevant diff and task file

## Method

1. Review the diff from the user's intended base, not just the final files.
2. For each guard or branch, ask what implementation deletion would still pass.
3. Check for tests that assert setup, mocks, or snapshots instead of behavior.
4. Check error paths with `errors.Is` / `errors.As` where applicable.
5. Require mutation proof for fail-closed behavior and idempotency.

## Required Report

Never invent elapsed time, duration, tokens, cost, throughput, or efficiency numbers; use runtime-owned metrics or `NOT_MEASURED`.

Always use `mivia-agent-report/v1` from `.ai/templates/agent-report-v1.md`. Keep the report strict and concise; do not add free-form sections unless the user asks for a long artifact.

Result semantics:

- `PASS` means reviewed behavior and tests are airtight, verifiers ran, and mutations are caught or clearly not applicable.
- `BLOCK` means any regression can ship, any verifier failed, or mutation proof is missing for a load-bearing guard.
- `PARTIAL` means the review is useful but the suite, baseline, or mutation proof could not be completed.
- `NOT_RUN` means the response is only a plan or review could not start.

Severity never gates approval; every open gap must be fixed. Low-severity gaps still require `BLOCK` or `PARTIAL` until fixed.

```md
ReportFormat: mivia-agent-report/v1
Skill: adversarial-test-review
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
