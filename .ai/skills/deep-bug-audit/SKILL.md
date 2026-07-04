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
- The relevant `docs/plans/ws-XX/tasks.md`

## Method

1. Define the exact scope and excluded areas.
2. Map inputs, outputs, persistence, protected actions, and error boundaries.
3. Inspect fail-closed behavior, idempotency, path traversal, secret handling, concurrency, and no-network invariants.
4. Check that tests exercise real boundaries rather than mocks of the thing under test.
5. For each finding, provide file, symbol, failure mode, user impact, and the test that should catch it.

## Output

- Findings first, ordered by severity.
- No speculative issues without a concrete failure path.
- Include residual risks and focused verifier commands.
