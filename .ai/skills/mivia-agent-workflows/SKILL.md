---
name: mivia-agent-workflows
description: Workflow execution skill for mivia-agent init/run/review behavior, desktop-agent workflow routing, run artifacts, and adapter-loop verification.
triggers:
  - mivia-agent workflow
  - workflow artifacts
  - desktop agent workflow
  - run-store
  - crush research loop
---

# Mivia Agent Workflows

## Read First

- `AGENTS.md`
- `.ai/INDEX.md`
- `.ai/rules/00-operating-doctrine.md`
- `.ai/rules/20-agent-quality.md`
- `.ai/rules/30-go-standards.md`
- `docs/desktop-agent-workflows.md`
- `docs/loop-authoring.md`
- `docs/user-guide.md`

## Method

Use this skill when changing, testing, or explaining `mivia-agent` workflows and their desktop-agent integration.

1. Treat the CLI as the runtime boundary. Prefer `mivia-agent adapters`, `mivia-agent run --dry-run --json`, and live `mivia-agent run --json` proof over chat-only reasoning.
2. Keep workflow `artifact` values logical. The run store owns physical paths under `.ai/runs/<run-id>/<step-id>/iter-<nnn>/<artifact>`.
3. Keep hooks deterministic and fast. Hooks may add context or block protected actions; they must not auto-run long model workflows.
4. Keep skills as the durable workflow instructions for Codex, Claude, `.agents`, Crush-aware operators, and future desktop agents.
5. Prove generated target repos through `init --write`, not only template helper tests.
6. Do not persist raw prompts, raw model output, provider payloads, secrets, credentials, or `.env` content.

## Desktop Prompts

Expected user prompts:

```text
Use $mivia-agent-workflows. Run workflow research-loop for objective: audit auth timeout handling.
```

```text
Use $mivia-agent-workflows. Dry-run workflow crush-research-loop, verify Crush/Qwen and Codex are resolved, then run it for objective: collect repo context for the billing refactor.
```

```text
Use $mivia-agent-workflows. Inspect workflow outputs from the latest run and report the artifact path and review consensus.
```

Translate free-text objectives into `--var objective="<free-text objective>"` when invoking `mivia-agent run`.

## Required Report

Always use `mivia-agent-report/v1` from `.ai/templates/agent-report-v1.md`. Keep the report strict and concise; do not add free-form sections unless the user asks for a long artifact.

Result semantics:

- `PASS` means the scoped work is implemented, verified, mutation-proofed, and committed or ready for the requested handoff.
- `BLOCK` means any implementation, test, verifier, or mutation-proof blocker remains.
- `PARTIAL` means a useful slice landed but a named dependency, user decision, or gated proof remains.
- `NOT_RUN` means the response is only a plan or implementation could not start.

Severity never gates approval; every open gap must be fixed. Low-severity gaps still require `BLOCK` or `PARTIAL` until fixed.

```md
ReportFormat: mivia-agent-report/v1
Skill: mivia-agent-workflows
Result: PASS|BLOCK|PARTIAL|NOT_RUN
Scope: <exact files/packages>
Baseline: <branch/commit/diff>
Summary: <one sentence>

| ID | Severity | Status | File:Line | Finding | Required Fix | Required Test | Mutation |
| --- | --- | --- | --- | --- | --- | --- | --- |
| none | none | closed | none | none | none | none | none |

| Command | Result | Notes |
| --- | --- | --- |
| none | NOT_RUN | none |

ResidualRisk: none|<short exact risk>
NextAction: none|<exact task>
```
