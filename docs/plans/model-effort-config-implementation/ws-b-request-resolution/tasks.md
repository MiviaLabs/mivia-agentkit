# WS-B — Request Resolution

## T1 — Adapter request fields

Create:
- `internal/adapter/adapter.go` — extend `Request` with `Model`, `Effort`, and `Params`.
- `internal/adapter/adapter_test.go` — request validation coverage.

Spec:
- Request accepts optional model and effort fields.
- Unknown effort values are rejected at request validation.

Tests that must pass:
- `TestRequestAcceptsModelAndEffort`
- `TestRequestRejectsUnknownEffort`

Dependencies:
- `internal/adapter`

Mutation proof:
- Allow unknown effort; `TestRequestRejectsUnknownEffort` must fail.

## T2 — Orchestrator resolves precedence

Create:
- `internal/orchestrator/engine.go` — resolve step override over adapter default and pass into adapter requests.
- `internal/orchestrator/engine_test.go` — precedence and producer/reviewer request coverage.

Spec:
- Effective settings resolve in this order: step override, adapter default, empty fallback.
- Producer and reviewer requests both receive the resolved model and effort.

Tests that must pass:
- `TestExecuteProducerStepPassesResolvedModelAndEffort`
- `TestExecuteReviewStepPassesResolvedModelAndEffort`
- `TestStepOverrideWinsOverAdapterDefault`

Dependencies:
- `internal/orchestrator`
- `internal/config`
- `internal/adapter`

Mutation proof:
- Ignore step override; `TestStepOverrideWinsOverAdapterDefault` must fail.

## Verification

```bash
go test ./internal/adapter/... ./internal/orchestrator/... -count=1
go vet ./internal/adapter/... ./internal/orchestrator/...
```

WS ws-b-request-resolution is ☑ when:
- [x] all listed tests pass
- [x] all mutation proofs executed and reverted (results in completion report)
- [x] `go vet` clean for this WS's packages
- [x] no network calls added (grep for `http.`, `net.Dial`, `os/exec` outside adapter fakes)

## Completion — 2026-07-05

- Tests: 74 passing.
- Mutation proofs: T1 request-effort guard fail-then-revert ok; T2 step-override precedence fail-then-revert ok.
- Files: 5 updated.
- Residual risk: none.
- Follow-ups: audit hardening wired `manifest.Adapters` into `internal/cli/run.go` and added a CLI regression proving manifest defaults reach runtime adapter requests.
