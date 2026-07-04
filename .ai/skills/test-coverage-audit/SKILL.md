---
name: test-coverage-audit
description: Coverage-gap workflow for mapping implemented behavior to required tests, mutation proofs, and workstream contracts. Use when asked what is untested or whether tests are sufficient.
triggers:
  - test coverage audit
  - missing tests
  - is this fully tested
  - coverage gaps
---

# Test Coverage Audit

## Read First

- `AGENTS.md`
- `.ai/rules/20-agent-quality.md`
- `.ai/rules/30-go-standards.md`
- `docs/plans/_conventions.md`
- Relevant workstream task files

## Method

1. List each behavior promised by the task, PRD section, or changed code.
2. Map each behavior to a named test.
3. Mark gaps for success path, error path, malformed input, idempotency, secret hygiene, and no-network behavior.
4. Identify shallow tests that mock the thing under test or only assert no error.
5. Convert each gap into a concrete test name and expected failing mutation.

## Output

- Coverage matrix: behavior, existing test, gap, required test.
- Fail closed: do not approve coverage unless every critical behavior has a real test or a justified exception.
