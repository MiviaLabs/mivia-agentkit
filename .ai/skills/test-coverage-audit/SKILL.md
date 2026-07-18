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
- `docs/plans/agentkit-implementation-roadmap/_conventions.md`
- Relevant workstream task files

## Method

1. List each behavior promised by the task, PRD section, or changed code.
2. Map each behavior to a named test.
3. Mark gaps for success path, error path, malformed input, idempotency, secret hygiene, and no-network behavior.
4. Identify shallow tests that mock the thing under test or only assert no error.
5. Convert each gap into a concrete test name and expected failing mutation.

## Required Report

Never invent elapsed time, duration, tokens, cost, throughput, or efficiency numbers; use runtime-owned metrics or `NOT_MEASURED`.

Always use `mivia-agent-report/v1` from `.ai/templates/agent-report-v1.md`. Keep the report strict and concise; do not add free-form sections unless the user asks for a long artifact.

Result semantics:

- `PASS` means every behavior has a named real test, verifier, and mutation target with no gap rows remaining.
- `BLOCK` means any behavior is missing, shallow, unverified, or falsely mocked.
- `PARTIAL` means the coverage map is useful but gated proof or source access is incomplete.
- `NOT_RUN` means the response is only a plan or coverage mapping could not start.

Severity never gates approval; every open gap must be fixed. Low-severity gaps still require `BLOCK` or `PARTIAL` until fixed.

```md
ReportFormat: mivia-agent-report/v1
Skill: test-coverage-audit
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
