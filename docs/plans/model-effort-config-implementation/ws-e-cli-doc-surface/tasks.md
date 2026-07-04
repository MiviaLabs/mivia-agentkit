# WS-E — CLI And Docs

## T1 — Dry-run output includes resolved model and effort

Create:
- `internal/cli/run.go` — include resolved model and effort in `run --dry-run --json`.
- `internal/cli/run_test.go` — dry-run output coverage.

Spec:
- Dry-run output shows resolved model and effort for each step.
- Empty values remain empty instead of inventing defaults.

Tests that must pass:
- `TestRunDryRunPrintsModelAndEffort`

Dependencies:
- `internal/cli`
- `internal/orchestrator`

Mutation proof:
- Omit model or effort from dry-run output; `TestRunDryRunPrintsModelAndEffort` must fail.

## T2 — User docs and examples

Create:
- `docs/config-examples.md` — model and effort examples for Codex, Claude, and Crush.
- `docs/user-guide.md` — supported runtime/config behavior.

Spec:
- Docs explain adapter defaults vs step overrides.
- Docs explain Codex/Claude runtime pass-through.
- Docs explain Crush config guidance now and runtime orchestration later.

Tests that must pass:
- `TestRunDryRunPrintsModelAndEffort`

Dependencies:
- `docs`

Mutation proof:
- Remove the documented override precedence; the dry-run behavior test remains the executable guard and docs must be updated to match it.
