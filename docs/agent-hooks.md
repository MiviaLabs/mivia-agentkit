# Agent Hooks

This repo wires project hooks for `.agents`, Codex, and Claude Code through one shared runner:

- `.agents/hooks.json`
- `.codex/hooks.json`
- `.claude/settings.json`
- `scripts/run_agent_hook_guard.sh`

Each hook command resolves the Git repo root first, then delegates to the shared runner. Codex and Claude both send hook JSON on stdin for command hooks; Claude documents that `UserPromptSubmit` and `Stop` do not support matchers and always fire when configured. Codex documents the same practical constraint for `Stop`.

## Hook Flow

1. The agent host fires a hook event.
2. `scripts/run_agent_hook_guard.sh` receives the JSON payload.
3. `scripts/agent_hook_guard.py` blocks Git-verification bypass attempts.
4. `scripts/audit_loop_guard.py` controls strict audit loops.
5. `scripts/plan_hook_guard.py` controls planning and plan-implementation workflows.
6. If all guards pass silently and a future `mivia-agent` binary exists, the runner calls `mivia-agent hook <agent> <event>`.

## Verification Bypass Guard

Policy: `.ai/policy/agent-hook-bypass.json`

Script: `scripts/agent_hook_guard.py`

Triggers:

- `UserPromptSubmit`: adds corrective context when a prompt asks an agent to use `--no-verify`, `HUSKY=0`, or legacy Husky skip variables.
- `PreToolUse`: blocks shell/tool commands that try to bypass Git hooks.
- `PermissionRequest`: denies permission requests that try to bypass Git hooks.
- `Stop`: currently passes silently unless the payload shape later carries a bypass-bearing command.

Expected model behavior after a bypass warning: run hooks normally, fix the failing validation, retry once, then notify the user with the exact blocker if it cannot be fixed.

## Audit Loop Guard

Policy: `.ai/policy/audit-loop.json`

Script: `scripts/audit_loop_guard.py`

The audit loop applies to these structured skills:

- `deep-bug-audit`
- `test-coverage-audit`
- `adversarial-test-review`

Triggers:

- `UserPromptSubmit`: starts loop state and injects strict context when the prompt asks for a bug audit, coverage audit, adversarial review, safe-to-ship review, or names one of the tracked skills.
- `Stop`: parses the last assistant message for `mivia-agent-report/v1` and decides whether the agent must continue.

Stop rules:

- Continue when any finding row has status `open`, `missing`, `shallow`, or `gated`.
- Continue when `ResidualRisk` is anything except `none`.
- Continue after the first clean report so a second independent pass confirms zero gaps.
- Allow stop after two consecutive clean reports.
- Allow stop at 10 total audit reports to prevent infinite loops.
- Allow stop earlier when the host has a lower hard cap. Claude Code overrides Stop hooks after 8 consecutive blocks, so the Claude surface uses 8.
- Continue once for malformed audit reports and require the exact structured report.

Severity never gates approval. A low-severity row with an open gap still keeps the loop active.

State is stored at `.git/mivia-agent-audit-loop-state.json` by default. Tests can override it with `MIVIA_AUDIT_LOOP_STATE`. The state stores counters and hashes only, not raw prompts or raw reports.

## Planning Guard

Policy: `.ai/policy/agent-plan.json`

Script: `scripts/plan_hook_guard.py`

Triggers:

- `UserPromptSubmit`: planning prompts get `agent-dag-planner` context and `mivia-agent-plan/v1` requirements.
- `UserPromptSubmit`: implementation prompts get `agent-plan-implementer` context and validated `.ai/plans/*.plan.json` requirements.
- `Stop`: planner reports must include `PlanArtifact: .ai/plans/<id>.plan.json`.
- `Stop`: planner reports with open gaps or residual risk are blocked.

Details live in `docs/agent-planning.md`.

## Validation

Use these targets after changing hooks, policies, skills, or report formats:

```bash
make agent-hook-test
make audit-loop-test
make plan-contract-test
make verify
```

`make verify` checks the JSON hook surfaces, policy values, guard scripts, Make targets, docs links, and contract tests.

Sources: https://developers.openai.com/codex/hooks, https://code.claude.com/docs/en/hooks.
