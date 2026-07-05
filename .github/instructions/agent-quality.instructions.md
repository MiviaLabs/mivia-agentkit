---
applyTo: "**"
---

# Agent Quality Instructions

Follow root `AGENTS.md` and `.ai/rules/20-agent-quality.md`.

- Every guard and fail-closed path needs a named test and mutation proof.
- Do not mock the thing under test.
- Use `t.TempDir()` for filesystem writes and fake runners for adapter CLIs.
- Do not persist raw prompts, raw model outputs, provider payloads, or plausible secrets.
- Keep generated output deterministic and idempotent.
