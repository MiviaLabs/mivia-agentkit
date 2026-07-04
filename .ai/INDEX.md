# .ai Control Surface

`.ai/` is the canonical project-level control surface for agentic development in this repo. Root `AGENTS.md` is the canonical instruction file; `.ai/` contains the durable rules and reusable skills that adapters reference.

## Read Order

1. `AGENTS.md`
2. `.ai/INDEX.md`
3. Relevant `.ai/rules/*.md`
4. Relevant `.ai/skills/*/SKILL.md`
5. Tool adapter files only when running that tool: `CLAUDE.md`, `.claude/settings.json`, `.codex/hooks.json`, `.github/copilot-instructions.md`

## Rules

- `.ai/rules/00-operating-doctrine.md` - scope control, docs-first work, idempotency, and verification contracts.
- `.ai/rules/01-output-budget.md` - terse status, final-answer shape, and task-slicing expectations.
- `.ai/rules/10-security-privacy.md` - secret hygiene, no raw prompt/output persistence, network limits, and hook safety.
- `.ai/rules/20-agent-quality.md` - test-first delivery, mutation proofs, review gates, and coverage expectations.
- `.ai/rules/30-go-standards.md` - Go package layout, errors, naming, comments, embedding, and tests.

## Skills

Canonical project skills:

- `.ai/skills/deep-bug-audit/SKILL.md`
- `.ai/skills/test-coverage-audit/SKILL.md`
- `.ai/skills/adversarial-test-review/SKILL.md`
- `.ai/skills/airtight-feature-delivery/SKILL.md`

Claude Code adapters:

- `.claude/skills/deep-bug-audit/SKILL.md`
- `.claude/skills/test-coverage-audit/SKILL.md`
- `.claude/skills/adversarial-test-review/SKILL.md`
- `.claude/skills/airtight-feature-delivery/SKILL.md`

The registry at `.agents/skills.json` lists all committed project skill files from both locations.

## Runtime Artifacts

`.ai/runs/` is reserved for future workflow traces and summaries and is gitignored. Do not persist raw prompts, raw model outputs, provider payloads, credentials, or plausible secrets there.

## Verification

Run `python3 scripts/verify_agent_config.py` after changing `AGENTS.md`, `.ai/`, `.claude/`, `.codex/`, `.github/`, `.agents/`, or `.gitignore`.
