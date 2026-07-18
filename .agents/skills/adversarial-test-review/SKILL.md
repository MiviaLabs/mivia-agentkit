---
name: adversarial-test-review
description: Adversarial pre-merge review for implementation and tests. Use for code review, PR review, or safe-to-ship questions.
triggers:
  - adversarial test review
  - code review
  - review this
  - safe to ship
---

# Adversarial Test Review

Read `.ai/skills/adversarial-test-review/SKILL.md` first. Follow `AGENTS.md` and `.ai/rules/20-agent-quality.md`.

Use `mivia-agent-report/v1` from `.ai/templates/agent-report-v1.md`; do not define a Codex-specific report shape.

Codex-specific behavior:

- Lead with blocking findings.
- Treat missing mutation proof as a coverage gap.
- State unrun verifiers plainly.
