---
name: agent-dag-planner
description: Re-verify existing plans, current code, hooks, skills, MCP needs, tests, and official docs, then correct plan gaps and emit a machine-readable DAG implementation plan for agents. Use when asked to plan implementation, decompose work for agents, create a DAG, convert docs/plans into agent-executable work, or prepare strict implementation handoff.
triggers:
  - implementation plan
  - dag planning
  - dag decomposition
  - agent plan
  - reverify plans
---

# Agent DAG Planner

## Read First

- `AGENTS.md`
- `.ai/INDEX.md`
- `.ai/rules/00-operating-doctrine.md`
- `.ai/rules/20-agent-quality.md`
- `.ai/templates/agent-report-v1.md`
- `.ai/templates/agent-plan-v1.json`
- `.ai/schemas/agent-plan-v1.schema.json`
- `docs/plans/agentkit-implementation-roadmap/_conventions.md`
- Existing roadmap files under `docs/plans/agentkit-implementation-roadmap/`

## Method

1. Define exact scope and exclusions before writing a plan.
2. Re-read current source, tests, hooks, skills, rules, policies, docs, and existing `docs/plans/agentkit-implementation-roadmap/**/*.md` relevant to the requested scope.
3. Re-verify current external behavior from official docs when agent surfaces, Codex, Claude, MCP, GitHub, Semgrep, Go, or other tool behavior affects the plan.
4. Identify stale, vague, missing, or conflicting plan content.
5. Correct those gaps in the new plan artifact; do not merely list them.
6. Emit a machine plan using `mivia-agent-plan/v1` in `.ai/plans/<id>.plan.json`.
7. Create or update task directories under `docs/plans/<plan-id>/` for the relevant DAG tasks.
8. Save or update a human implementation plan in `docs/plans/`.
9. Run `python3 scripts/validate_agent_plan.py <plan>`.

## DAG Requirements

Every DAG node must include:

- stable `id`
- `depends_on`
- `task_dir`
- responsible `skill`
- target `agent`
- `files_read`
- `files_edit`
- `allowed_mcp_tools`
- `tests`
- `verifiers`
- `mutation`
- `outputs`

Every implementation node must include at least one verifier and one mutation proof. Nodes that edit code or hooks must include test-first instructions and an audit/review follow-up node or explicit reason why it is not applicable.

## Plan Gap Policy

The planner must fill and correct gaps before reporting success.

Examples of gaps that must be corrected:

- human-only prose without machine-readable DAG fields
- missing source evidence
- missing current external docs for changing agent/tool behavior
- missing test names
- missing verifier commands
- missing mutation proof
- missing MCP tool allow list
- stale or conflicting dependency order
- ambiguous scope

If any gap remains `open`, `missing`, `shallow`, or `gated`, the final report must be `BLOCK` or `PARTIAL`.

## Required Report

Always use `mivia-agent-report/v1` from `.ai/templates/agent-report-v1.md`. Keep the report strict and concise; do not add free-form sections unless the user asks for a long artifact.

Result semantics:

- `PASS` means the Markdown plan and `.ai/plans/*.plan.json` artifact were created or updated, validated, and have zero open gaps.
- `BLOCK` means a plan gap, verifier gap, stale source, invalid DAG, or missing artifact remains.
- `PARTIAL` means useful planning work landed but a named user decision or external blocker remains.
- `NOT_RUN` means the response is only an explanation and no planning work started.

Severity never gates approval; every open gap must be fixed. Low-severity gaps still require `BLOCK` or `PARTIAL` until fixed.

```md
ReportFormat: mivia-agent-report/v1
Skill: agent-dag-planner
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
