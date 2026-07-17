---
name: deep-bug-audit
description: Structured bug-finding methodology for this repo. Use when asked for a deep bug audit, broad bug hunt, or production-readiness review.
triggers:
  - deep bug audit
  - bug hunt
  - production readiness audit
---

# Deep Bug Audit

Read `.ai/skills/deep-bug-audit/SKILL.md` first. Follow `AGENTS.md` and `.ai/rules/20-agent-quality.md`.

Use `mivia-agent-report/v1` from `.ai/templates/agent-report-v1.md`; do not define a Codex-specific report shape.

Codex-specific behavior:

- Inspect the live diff before findings.
- Report concrete failure paths only.
- Put findings first, then verification gaps.
