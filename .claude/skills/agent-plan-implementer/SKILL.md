---
name: agent-plan-implementer
description: Claude Code adapter for implementing a validated mivia-agent-plan/v1 DAG with scoped edits, verifiers, mutation proofs, hooks, and audit loops.
triggers:
  - implement this plan
  - implement the plan
  - execute dag plan
  - run the plan
  - plan implementation
---

# Agent Plan Implementer

Read `.ai/skills/agent-plan-implementer/SKILL.md` first. Follow `AGENTS.md`, `.ai/INDEX.md`, and the active `.ai/plans/*.plan.json`.

Use `mivia-agent-report/v1` from `.ai/templates/agent-report-v1.md`; do not define a Claude-specific report shape.

Claude-specific behavior:

- State intended edits before modifying files.
- Validate the plan before edits.
- Keep edits inside the active DAG node scope unless the plan is corrected first.
- Run focused verification after each node.
