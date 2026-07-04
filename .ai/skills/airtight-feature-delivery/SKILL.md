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
- `docs/plans/_conventions.md`
- The target workstream `tasks.md`

## Method

1. Confirm the exact task boundary: one production file plus its test.
2. Read the PRD and plan sections named by the task.
3. Write or update tests before or alongside production code.
4. Implement the smallest code path that satisfies the task.
5. Run focused tests and `go vet` for the affected package.
6. Execute and revert mutation proofs.
7. Append the required completion report only when the task or workstream is actually complete.

## Output

- Changed files.
- Tests and mutation proofs run.
- Completion report location or reason it was not updated.
- Residual risk.
