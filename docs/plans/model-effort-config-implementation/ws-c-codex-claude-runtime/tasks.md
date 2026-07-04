# WS-C — Codex And Claude Runtime

## T1 — Codex model and effort pass-through

Create:
- `internal/adapter/codex.go` — thread `Request.Model` and `Request.Effort` into the Codex subprocess contract.
- `internal/adapter/codex_test.go` — argv/config override coverage.

Spec:
- `Run` passes `--model <id>` when model is set.
- `Run` passes the documented effort override through Codex's config override surface.
- Existing approval, timeout, and scrubbed metadata behavior stays intact.

Tests that must pass:
- `TestCodexRunPassesModelFlag`
- `TestCodexRunPassesReasoningEffortOverride`

Dependencies:
- `internal/adapter`

Mutation proof:
- Remove the model flag or effort override; the matching Codex test must fail.

## T2 — Claude model and effort pass-through

Create:
- `internal/adapter/claude.go` — thread `Request.Model` and `Request.Effort` into Claude CLI argv.
- `internal/adapter/claude_test.go` — argv coverage.

Spec:
- `Run` passes `--model <id>` when model is set.
- `Run` passes `--effort <level>` when effort is set.
- Existing permission-mode, JSON, and review behavior stays intact.

Tests that must pass:
- `TestClaudeRunPassesModelFlag`
- `TestClaudeRunPassesEffortFlag`

Dependencies:
- `internal/adapter`

Mutation proof:
- Remove `--model` or `--effort`; the matching Claude test must fail.
