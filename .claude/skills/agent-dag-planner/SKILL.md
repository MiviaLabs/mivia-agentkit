---
name: agent-dag-planner
description: Claude Code adapter for re-verifying existing plans and current code, correcting plan gaps, and emitting a machine-readable DAG implementation plan.
triggers:
  - implementation plan
  - dag planning
  - dag decomposition
  - agent plan
  - reverify plans
---

# Agent DAG Planner

Read `.ai/skills/agent-dag-planner/SKILL.md` first. Follow `AGENTS.md`, `.ai/INDEX.md`, and `.ai/templates/agent-plan-v1.json`.

Use `mivia-agent-report/v1` from `.ai/templates/agent-report-v1.md`; do not define a Claude-specific report shape.

Claude-specific behavior:

- State intended plan/doc edits before modifying files.
- Create or update the relevant DAG task directories under `docs/plans/<plan-id>/`.
- Validate `.ai/plans/*.plan.json` before reporting success.
- End planner reports with `PlanArtifact: .ai/plans/<id>.plan.json`.
