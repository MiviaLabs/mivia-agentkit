---
name: agent-dag-planner
description: Codex adapter for re-verifying existing plans and current code, correcting plan gaps, and emitting a machine-readable DAG implementation plan.
triggers:
  - implementation plan
  - dag planning
  - dag decomposition
  - agent plan
  - reverify plans
---

# Agent DAG Planner

Read `.ai/skills/agent-dag-planner/SKILL.md` first. Follow `AGENTS.md`, `.ai/INDEX.md`, and `.ai/templates/agent-plan-v1.json`.

Use `mivia-agent-report/v1` from `.ai/templates/agent-report-v1.md`; do not define a Codex-specific report shape.

Codex-specific behavior:

- Save the human plan under `docs/plans/`.
- Save the machine plan under `.ai/plans/*.plan.json`.
- Create or update the relevant DAG task directories under `docs/plans/<plan-id>/`.
- Validate the plan with `python3 scripts/validate_agent_plan.py <plan>`.
- End planner reports with `PlanArtifact: .ai/plans/<id>.plan.json`.
