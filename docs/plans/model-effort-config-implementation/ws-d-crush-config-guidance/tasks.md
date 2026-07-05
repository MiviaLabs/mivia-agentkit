# WS-D — Crush Config Guidance

## T1 — Crush params in generated config surface

Create:
- `templates/core/mivia-agent.yaml.tmpl` — expose Crush config keys in the generated manifest example surface.
- `internal/config/manifest_test.go` — parsing coverage for Crush params.

Spec:
- Crush adapter config can carry `model` and provider `params`.
- Crush remains guidance-only and is not promoted into orchestrated runtime support.

Tests that must pass:
- `TestManifestParsesCrushParams`

Dependencies:
- `internal/config`
- `templates/core`

Mutation proof:
- Drop Crush param parsing; `TestManifestParsesCrushParams` must fail.

## T2 — Crush template guidance

Create:
- `templates/adapters/crush/README.md.tmpl` — explain model/provider config guidance without claiming runtime orchestration.
- `internal/templates/templates_test.go` — template content coverage.

Spec:
- Crush guidance includes model/provider config direction.
- Crush guidance stays a thin pointer and does not duplicate long policy.
- No claim is made that Crush participates in `run`.

Tests that must pass:
- `TestCrushTemplateIncludesModelConfigGuidance`

Dependencies:
- `internal/templates`
- `templates/adapters/crush`

Mutation proof:
- Remove model/provider guidance from the template; `TestCrushTemplateIncludesModelConfigGuidance` must fail.

## Verification

```bash
go test ./internal/config/... ./internal/templates/... -count=1
go vet ./internal/config/... ./internal/templates/...
```

WS ws-d-crush-config-guidance is ☑ when:
- [x] all listed tests pass
- [x] all mutation proofs executed and reverted (results in completion report)
- [x] `go vet` clean for this WS's packages
- [x] no network calls added (grep for `http.`, `net.Dial`, `os/exec` outside adapter fakes)

## Completion — 2026-07-05

- Tests: 42 passing.
- Mutation proofs: T1 Crush config guidance removal fail-then-revert ok; T2 Crush README guidance removal fail-then-revert ok.
- Files: 7 updated.
- Residual risk: none.
- Follow-ups: none.
