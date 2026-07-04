# WS-A — Config Schema

## T1 — Adapter defaults for model and effort

Create:
- `internal/config/manifest.go` — extend `AdapterConfig` with `Model`, `Effort`, and `Params`.
- `internal/config/manifest_test.go` — manifest parsing and validation coverage.

Spec:
- Adapter defaults accept optional `model` and `effort`.
- Adapter defaults accept optional string-string `params`.
- Unknown effort values fail closed during manifest validation.

Tests that must pass:
- `TestManifestParsesAdapterModelDefaults`
- `TestManifestRejectsUnknownEffort`
- `TestManifestParsesCrushParams`

Dependencies:
- `internal/config`

Mutation proof:
- Remove effort validation; `TestManifestRejectsUnknownEffort` must fail.

## T2 — Step-level model and effort overrides

Create:
- `internal/config/loop.go` — extend `Step` with `Model` and `Effort`.
- `internal/config/loop_test.go` — step override coverage.

Spec:
- Workflow steps accept optional `model` and `effort`.
- Step parsing preserves existing validation and bounded-loop behavior.

Tests that must pass:
- `TestLoopParsesStepModelOverrides`

Dependencies:
- `internal/config`

Mutation proof:
- Remove step-level field parsing; `TestLoopParsesStepModelOverrides` must fail.

## Verification

```bash
go test ./internal/config/... -count=1
go vet ./internal/config/...
```

WS ws-a-config-schema is ☑ when:
- [x] all listed tests pass
- [x] all mutation proofs executed and reverted (results in completion report)
- [x] `go vet` clean for this WS's packages
- [x] no network calls added (grep for `http.`, `net.Dial`, `os/exec` outside adapter fakes)
- [ ] status updated in `00-overview.md`

## Completion — 2026-07-05

- Tests: 20 passing.
- Mutation proofs: T1 adapter-effort guard fail-then-revert ok; T1 step-effort guard fail-then-revert ok; T2 step-field parsing fail-then-revert ok.
- Files: 5 updated.
- Residual risk: none.
- Follow-ups: audit hardening broadened the shared effort validator to the documented cross-adapter set `none|minimal|low|medium|high|xhigh|max`; update the parent overview when the broader model-effort workstream set is complete.
