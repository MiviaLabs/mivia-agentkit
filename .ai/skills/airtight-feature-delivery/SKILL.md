---
name: airtight-feature-delivery
description: End-to-end feature delivery checklist for implementing one scoped workstream task with discovery, tests, mutation proofs, verification, and completion reporting.
triggers:
  - implement feature
  - airtight delivery
  - workstream implementation
  - finish this task
---

# Airtight Feature Delivery

## Read First

- `AGENTS.md`
- `.ai/INDEX.md`
- `.ai/rules/00-operating-doctrine.md`
- `.ai/rules/20-agent-quality.md`
- `.ai/rules/30-go-standards.md`
- `docs/plans/human/_conventions.md`
- The target workstream `tasks.md`

## Method

1. Confirm the exact task boundary: one production file plus its test.
2. Read the PRD and plan sections named by the task.
3. Write or update tests before or alongside production code.
4. Implement the smallest code path that satisfies the task.
5. Run focused tests and `go vet` for the affected package.
6. Execute and revert mutation proofs.
7. Append the required completion report only when the task or workstream is actually complete.

## Required Report

Always use `mivia-agent-report/v1` from `.ai/templates/agent-report-v1.md`. Keep the report strict and concise; do not add free-form sections unless the user asks for a long artifact.

Result semantics:

- `PASS` means the scoped work is implemented, verified, mutation-proofed, and committed or ready for the requested handoff.
- `BLOCK` means any implementation, test, verifier, or mutation-proof blocker remains.
- `PARTIAL` means a useful slice landed but a named dependency, user decision, or gated proof remains.
- `NOT_RUN` means the response is only a plan or implementation could not start.

Severity never gates approval; every open gap must be fixed. Low-severity gaps still require `BLOCK` or `PARTIAL` until fixed.

```md
ReportFormat: mivia-agent-report/v1
Skill: airtight-feature-delivery
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
