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

## Verification

```bash
go test ./internal/adapter/... -count=1
go vet ./internal/adapter/...
```

WS ws-c-codex-claude-runtime is ☑ when:
- [x] all listed tests pass
- [x] all mutation proofs executed and reverted (results in completion report)
- [x] `go vet` clean for this WS's packages
- [x] no network calls added (grep for `http.`, `net.Dial`, `os/exec` outside adapter fakes)

## Completion — 2026-07-05

- Tests: 50 passing.
- Mutation proofs: T1 Codex `--model` fail-then-revert ok; T1 Codex `model_reasoning_effort` override fail-then-revert ok; T2 Claude `--model` fail-then-revert ok; T2 Claude `--effort` fail-then-revert ok.
- Files: 5 updated.
- Residual risk: none.
- Follow-ups: continue to the next scoped node for downstream runtime consumers and doc surface.
