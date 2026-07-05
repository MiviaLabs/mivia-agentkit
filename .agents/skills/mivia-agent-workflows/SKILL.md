---
name: mivia-agent-workflows
description: Codex adapter for mivia-agent workflow execution, run artifacts, and desktop-agent workflow routing.
triggers:
  - mivia-agent workflow
  - workflow artifacts
  - desktop agent workflow
  - run-store
  - crush research loop
---

# Mivia Agent Workflows

Read `.ai/skills/mivia-agent-workflows/SKILL.md` first. Follow `AGENTS.md`, `.ai/INDEX.md`, and `docs/desktop-agent-workflows.md`.

Use `mivia-agent-report/v1` from `.ai/templates/agent-report-v1.md`; do not define a Codex-specific report shape.

Codex-specific behavior:

- Use the real CLI boundary for workflow proof.
- Run `mivia-agent run --repo . --workflow <name> --dry-run --json` before live workflow execution.
- Treat `.ai/runs/` as ignored runtime output and do not commit run artifacts.
- Report workflow pass as artifact acceptance only, not ship approval.
