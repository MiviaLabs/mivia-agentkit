---
name: airtight-feature-delivery
description: End-to-end feature delivery checklist for one scoped workstream task. Use when asked to implement or finish a task.
triggers:
  - implement feature
  - airtight delivery
  - workstream implementation
---

# Airtight Feature Delivery

Read `.ai/skills/airtight-feature-delivery/SKILL.md` first. Follow `AGENTS.md`, `.ai/rules/00-operating-doctrine.md`, and `.ai/rules/30-go-standards.md`.

Use `mivia-agent-report/v1` from `.ai/templates/agent-report-v1.md`; do not define a Codex-specific report shape.

Codex-specific behavior:

- State intended edits before modifying files.
- Run focused verification after changes.
- Do not mark a task complete without tests and mutation-proof status.
