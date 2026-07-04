# Agent Planning

Agent planning uses two artifacts:

- Task directory: `docs/plans/<plan-id>/**/tasks.md`
- Human implementation plan: `docs/plans/*.md`
- Machine plan: `.ai/plans/*.plan.json`

The task directory contains executable task files for each DAG node or workstream. The human plan explains intent and tradeoffs. The machine plan is the execution contract for agents and hooks.

## Skills

`agent-dag-planner` creates or updates plans. It must:

- re-read current source, hooks, skills, rules, tests, and relevant docs
- re-verify existing `docs/plans/agentkit-implementation-roadmap/**/*.md`
- create or update `docs/plans/<plan-id>/**/tasks.md` for the relevant DAG tasks
- fill or correct stale, vague, missing, or conflicting plan gaps
- emit `mivia-agent-plan/v1` under `.ai/plans/`
- validate the plan with `scripts/validate_agent_plan.py`
- end with `PlanArtifact: .ai/plans/<id>.plan.json`

The canonical AgentKit implementation roadmap artifact is `.ai/plans/agentkit-implementation-roadmap.plan.json`, with its human report at `docs/plans/agentkit-implementation-roadmap.md`.

`agent-plan-implementer` executes validated plans. It must:

- validate the machine plan before edits
- execute one DAG node at a time
- stay inside the node's `files_edit` scope unless the plan is corrected first
- run node tests, verifiers, and mutation proof
- run deep bug, coverage, and adversarial review loops for implementation nodes

## Machine Plan

Required plan format: `mivia-agent-plan/v1`

Each node must define:

- `id`
- `depends_on`
- `skill`
- `agent`
- `files_read`
- `files_edit`
- `task_dir`
- `allowed_mcp_tools`
- `tests`
- `verifiers`
- `mutation`
- `outputs`

The validator rejects DAG cycles, unknown dependencies, missing task directories, missing node verifiers, missing tests, missing mutation proof, empty correction logs, and any gap status of `open`, `missing`, `shallow`, or `gated`.

## Hooks

`scripts/plan_hook_guard.py` runs through `scripts/run_agent_hook_guard.sh`.

Triggers:

- `UserPromptSubmit`: planning prompts receive `agent-dag-planner` context and machine-plan requirements.
- `UserPromptSubmit`: implementation prompts receive `agent-plan-implementer` context and validated-plan requirements.
- `Stop`: planner reports must include `PlanArtifact: .ai/plans/<id>.plan.json`.
- `Stop`: planner reports with open gaps or residual risk are blocked.

## Validation

```bash
make plan-contract-test
make verify
```

Semgrep also checks for plan drift patterns such as Markdown-only agent plans, planner skills that only list gaps, and implementer skills that make audit loops optional.

Sources: https://developers.openai.com/codex/skills, https://developers.openai.com/codex/hooks, https://code.claude.com/docs/en/hooks, https://modelcontextprotocol.io/specification/2025-06-18.
