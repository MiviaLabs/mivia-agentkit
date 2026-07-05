# WS-F — Audit Review

## T1 — Deep bug audit and verifier closure

Create:
- `docs/plans/model-effort-config-implementation/ws-f-audit-review/tasks.md` — audit checklist and closure record.
- `docs/plans/model-effort-config-implementation.md` — update if audit changes scope or residual risk.

Spec:
- Audit the cumulative implementation delta across config, adapter, orchestrator, CLI, templates, and docs.
- Re-run the node-specific load-bearing mutations before final closure.
- Close only with zero open gaps and `ResidualRisk: none`.

Tests that must pass:
- `TestManifestRejectsUnknownEffort`
- `TestStepOverrideWinsOverAdapterDefault`
- `TestCodexRunPassesReasoningEffortOverride`
- `TestClaudeRunPassesEffortFlag`
- `TestRunDryRunPrintsModelAndEffort`

Dependencies:
- `internal/config`
- `internal/adapter`
- `internal/orchestrator`
- `internal/cli`
- `internal/templates`

Mutation proof:
- Re-run the prior node mutations; at least one named regression test per guard must fail before revert.

## Verification

```bash
go test ./internal/config/... ./internal/adapter/... ./internal/orchestrator/... ./internal/cli/... ./internal/templates/... -count=1
go vet ./internal/config/... ./internal/adapter/... ./internal/orchestrator/... ./internal/cli/... ./internal/templates/...
```

WS ws-f-audit-review is ☑ when:
- [x] deep bug audit found no open in-scope gaps
- [x] test coverage audit found no missing or shallow in-scope tests
- [x] adversarial test review found no surviving regression path
- [x] all named load-bearing mutations failed before revert
- [x] full package verifier and `go vet` set passed

## Completion — 2026-07-05

- Tests: 170 passing.
- Mutation proofs: config effort validation fail-then-revert ok; orchestrator step-override precedence fail-then-revert ok; Codex effort override fail-then-revert ok; Claude effort flag fail-then-revert ok; dry-run runtime surface fail-then-revert ok; Crush guidance removal fail-then-revert ok.
- Files: 1 updated.
- Residual risk: none.
- Follow-ups: none.

## Audit Loop Fix — 2026-07-05

- Tests: expanded `TestAntigravityReviewRejectsUnsupportedRuntimeKnobs`; added `TestExecuteProducerStepValidatesGenericRequestBeforeRun` and `TestExecuteReviewStepValidatesGenericRequestBeforeReview`.
- Coverage gap fixed: Antigravity review now proves every unsupported runtime knob is rejected before `agy`; orchestrator fallback validation now covers adapters without `RequestValidator`.
- Mutation proofs: remove Antigravity effort/params rejection or bypass generic request validation; the matching focused tests must fail before revert.
- Files: 3 updated.
- Residual risk: none.
- Follow-ups: none.

## Audit Fix — 2026-07-05

- Tests: added adapter-specific unsupported-effort regressions for Codex and Claude `Run` and `Review`.
- Mutation proofs: remove Codex/Claude adapter-specific effort validation; the matching test must fail.
- Files: 9 updated.
- Residual risk: none.
- Follow-ups: none.

## Audit Hardening — 2026-07-05

- Tests: added `TestExecuteReviewStepValidatesRequestsBeforeFanout`, `TestExecuteProducerStepValidatesRequestBeforeRun`, Codex/Claude unsupported-params tests, and Antigravity unsupported runtime-knob tests.
- Runtime guard: added adapter request preflight so every producer request and every reviewer request validates before any adapter subprocess or fan-out starts.
- Adapter guard: Antigravity now rejects `model`, `effort`, and `params` because the current `agy -p` contract has no documented mapping for those fields.
- Mutation proofs: remove producer/reviewer preflight, allow unsupported params, or allow Antigravity model; the matching focused tests must fail before revert.
- Files: 14 updated.
- Residual risk: none.
- Follow-ups: none.
