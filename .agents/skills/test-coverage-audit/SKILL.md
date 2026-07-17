---
name: test-coverage-audit
description: Coverage gap analysis for this repo. Use when asked what is untested, whether tests are sufficient, or how to improve coverage.
triggers:
  - test coverage audit
  - missing tests
  - coverage gaps
---

# Test Coverage Audit

Read `.ai/skills/test-coverage-audit/SKILL.md` first. Follow `AGENTS.md` and `.ai/rules/20-agent-quality.md`.

Use `mivia-agent-report/v1` from `.ai/templates/agent-report-v1.md`; do not define a Codex-specific report shape.

Codex-specific behavior:

- Build a behavior-to-test matrix.
- Flag tests that mock the thing under test.
- Name the exact missing test and mutation it should catch.
