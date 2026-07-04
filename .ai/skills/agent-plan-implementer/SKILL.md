---
name: agent-plan-implementer
description: Implement a validated mivia-agent-plan/v1 DAG plan node-by-node with hooks, test-first changes, verifier gates, mutation proofs, and mandatory audit/review loops. Use when asked to implement a saved plan, execute a DAG plan, or carry a planning artifact through code/docs/hook changes.
triggers:
  - implement this plan
  - implement the plan
  - execute dag plan
  - run the plan
  - plan implementation
---

# Agent Plan Implementer

## Read First

- `AGENTS.md`
- `.ai/INDEX.md`
- `.ai/rules/00-operating-doctrine.md`
- `.ai/rules/20-agent-quality.md`
- `.ai/templates/agent-report-v1.md`
- The requested `.ai/plans/*.plan.json`
- The human plan referenced by the machine plan

## Method

1. Validate the plan first with `python3 scripts/validate_agent_plan.py <plan>`.
2. Refuse to start if the plan has open gaps, a cycle, missing verifiers, missing tests, missing mutation proof, or missing correction log.
3. Execute one DAG node at a time in dependency order.
4. Before each node, re-read the node's `files_read`, confirm `files_edit` scope, and state the intended edit.
5. Write focused RED tests where feasible before implementation.
6. Patch narrowly inside the node scope.
7. Run the node's verifiers exactly.
8. Run and revert the node's mutation proof.
9. For code, hooks, skills, policies, or verifier changes, run the repo's bug/coverage/review chain:
   - `deep-bug-audit`
   - `test-coverage-audit`
   - `adversarial-test-review`
10. Update the machine plan or human docs if implementation discovers a real plan gap.

## Fail-Closed Rules

- Do not implement from a free-form Markdown plan alone.
- Do not edit files outside the active DAG node unless the plan is corrected first.
- Do not skip verifiers because a gap is low severity.
- Do not report `PASS` while audit, coverage, review, mutation, or plan-validation gaps remain.
- Do not use MCP tools unless the active node lists them in `allowed_mcp_tools` or the plan is corrected first.

## Required Report

Always use `mivia-agent-report/v1` from `.ai/templates/agent-report-v1.md`. Keep the report strict and concise; do not add free-form sections unless the user asks for a long artifact.

Result semantics:

- `PASS` means every requested DAG node is implemented, verified, mutation-proofed, and audit/review clean.
- `BLOCK` means any implementation, test, verifier, audit, review, mutation, or plan-validation blocker remains.
- `PARTIAL` means a useful DAG slice landed but a named dependency, user decision, or gated proof remains.
- `NOT_RUN` means the response is only a plan or implementation could not start.

Severity never gates approval; every open gap must be fixed. Low-severity gaps still require `BLOCK` or `PARTIAL` until fixed.

```md
ReportFormat: mivia-agent-report/v1
Skill: agent-plan-implementer
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
PlanArtifact: .ai/plans/<id>.plan.json
```
