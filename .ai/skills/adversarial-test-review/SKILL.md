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

## Output

- Findings first with severity, file, line or symbol, and failing scenario.
- Explicitly state "No blocking findings" only when the reviewed behavior and tests are both adequate.
- List unrun verifiers and remaining risk.
