# WS5 — Hook Engine (Codex + Claude)

- **Phase:** 4
- **Depends on:** WS4 (stamp), WS12 (policy)
- **PRD:** FR-7.1, FR-8.1, FR-8.2, FR-8.3
- **Plan:** WS5, "Hook Engine", `hook codex <event>` / `hook claude <event>`
- **Exit gate (Phase 4):** hooks deny protected actions on missing/stale stamp or policy denial; share one policy engine; fail closed on malformed protected payloads.

Goal: the entrypoints invoked by Codex/Claude hook configs. Read JSON from stdin, decide, emit the adapter-specific decision shape. Zero persistence beyond what WS12 records.

## T0 — Re-verify hook payload shapes first

Before coding, confirm against current official docs (URLs in plan "External Facts"):
- **Codex** — exact JSON fields for `user-prompt-submit`, `pre-tool-use`, `permission-request`, `stop`; and the response shape Codex expects (additional-context injection vs deny). Record in `codex.go` doc.
- **Claude Code** — exact JSON fields for `PreToolUse`, `Stop`; the hook output schema (`{continue: bool, stopReason, systemMessage}` or current equivalent); exit-code semantics (0 = continue, 2 = block). Record in `claude.go` doc.

If shapes drifted, update tests + emitters; cite in completion report.

## T1 — Shared engine + protected-action detection

Create:
- `internal/hooks/hooks.go` — `type Event string`, `type Payload struct{ Tool, Adapter, Raw map[string]any }`, `type Outcome struct{ Allow bool; Context map[string]string; Reason string }`, `func Decide(ctx, Payload, stamp preflight.Checker, pol policy.Provider) (Outcome, error)`, `func IsProtected(raw map[string]any) (string, bool)`.
- `internal/hooks/hooks_test.go`

Spec:
- `IsProtected` scans the raw payload for protected-action patterns: `git commit`, `git push`, `gh pr`, `gh release`, deploy commands, live-smoke triggers, and the Claude/Codex tool names that wrap them. Returns the matched `ProtectedKind` (commit/push/pull_request/deploy/release/live_smoke) or `("", false)`.
- `Decide`: if not protected → `Allow: true` (+ optional context injection for implementation-shaped prompts). If protected → check stamp (`preflight.CheckStamp`); missing/stale → deny. Then `pol.Decide`; denied → deny. Both checks pass → allow.
- Malformed payload that requests a protected action and cannot be parsed → deny (fail closed), with `Reason="malformed protected payload"`.

Tests that must pass:
- `TestIsProtectedDetectsCommit`
- `TestIsProtectedDetectsPush`
- `TestIsProtectedDetectsDeploy`
- `TestIsProtectedReturnsFalseForBenign`
- `TestDecideAllowsBenign`
- `TestDecideDeniesProtectedWithoutStamp`
- `TestDecideDeniesProtectedOnStaleStamp`
- `TestDecideDeniesProtectedOnPolicyDeny`
- `TestDecideAllowsProtectedWithFreshStampAndPolicyAllow`
- `TestMalformedPayloadFailsClosedForProtectedAction`

Mutation proof:
- Remove `git commit` from protected patterns; `TestIsProtectedDetectsCommit` must fail.
- Remove the malformed→deny branch; `TestMalformedPayloadFailsClosedForProtectedAction` must fail.

## T2 — Codex emitter

Create:
- `internal/hooks/codex.go` — `func EmitCodex(ctx, event Event, payload Payload, out Outcome) error` — writes the Codex-expected JSON to stdout.
- `internal/hooks/codex_test.go`

Spec (per T0):
- For `user-prompt-submit`: emit an `additional_context`-shaped JSON (current Codex schema per T0).
- For `pre-tool-use`/`permission-request` with a protected action: emit the deny shape (e.g. `{"decision":"deny","reason":...}` — confirm exact field names per T0).
- For `stop`: emit block shape when required handoff fields/stamp are missing.

Tests that must pass (golden-file assertions against fixtures in `testdata/codex/`):
- `TestCodexPreToolUseDenyShape`
- `TestCodexUserPromptSubmitAddsImplementationContext`
- `TestCodexStopBlocksDoneWithoutStamp`
- `TestCodexEmitStableOrder`

Mutation proof:
- Rename the deny field; `TestCodexPreToolUseDenyShape` (golden-file compare) must fail.

Notes:
- Do NOT infer Codex's schema from memory. T0 golden files come from the documented schema.

## T3 — Claude emitter

Create:
- `internal/hooks/claude.go` — `func EmitClaude(ctx, event Event, payload Payload, out Outcome) error`.
- `internal/hooks/claude_test.go`

Spec (per T0):
- `PreToolUse` deny → exit code 2 + the JSON shape Claude expects (per T0; typically `{"decision":"block","reason":...}` — confirm).
- `Stop` block → the stop-blocking shape when stamp/handoff missing.
- Allow → exit 0, minimal/no stdout.

Tests that must pass (golden files in `testdata/claude/`):
- `TestClaudePreToolUseDenyShape`
- `TestClaudeStopBlocksDoneWithoutStamp`
- `TestClaudeAllowEmitsMinimal`
- `TestClaudeEmitStableOrder`

Mutation proof:
- Change the exit code on deny to 0; `TestClaudePreToolUseDenyShape` must fail.

## T4 — CLI wiring

Create:
- `internal/cli/hook.go` — `hookCmd` with subcommands `codex <event>` and `claude <event>`. Each reads stdin, parses, calls shared `Decide`, calls the right emitter.
- `internal/cli/hook_test.go`

Spec:
- Stdin parse is defensive: a non-JSON or truncated payload requesting a protected action → deny (fail closed). A non-JSON payload not requesting a protected action → allow with a warning.
- Exit code from the emitter (Claude) / 0-with-stdout (Codex).

Tests that must pass (subprocess: invoke the built `mivia-agent` binary with sample stdin):
- `TestHookCodexSubprocessDeniesProtectedWithoutStamp`
- `TestHookClaudeSubprocessDeniesProtectedWithoutStamp`
- `TestHookCodexSubprocessAllowsBenign`
- `TestHookMalformedStdinFailsClosedForProtected`

Mutation proof:
- Make malformed-stdin allow; `TestHookMalformedStdinFailsClosedForProtected` must fail.

## Verification

```bash
go test ./internal/hooks/... ./internal/cli/... -count=1
go vet ./internal/hooks/...
# Subprocess smoke:
echo '{"tool":"bash","command":"git push"}' | go run ./cmd/mivia-agent hook claude pre-tool-use ; echo "exit=$?"
```

WS5 is ☑ when:
- [ ] T0 re-verification done; golden fixtures from documented schemas
- [ ] all listed tests pass (golden-file + subprocess)
- [ ] protected-pattern, fail-closed, deny-exit mutation proofs executed (≥3)
- [ ] status updated in `00-overview.md`

## Completion — 2026-07-05

- Tests: 19 WS5 hook tests passing, plus focused `internal/cli` package tests passing.
- Mutation proofs: `git commit` detector removal fail-then-revert ok; malformed protected payload allow fail-then-revert ok; Claude deny exit code `0` fail-then-revert ok.
- Files: 8 created, 1 updated.
- Residual risk: `go run` wraps Claude `os.Exit(2)` as process exit `1`; built-binary smoke and subprocess tests assert real exit code 2.
- Follow-ups: none.
