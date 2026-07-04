# WS12 — Governance Provider

- **Phase:** 2 (interface + noop), 4 (AGT wiring), 6 (production AGT)
- **Depends on:** WS1
- **PRD:** FR-2.2, FR-7.1, FR-7.2
- **Plan:** WS12, "Governance Backbone (AGT)"
- **Exit gate (Phase 2):** `policy.Provider` interface + `noop` provider; stamp carries `PolicyDecisionRefs`. **(Phase 4):** `agt` provider compiles and decides on protected actions; doctor fails when strict requires AGT but it's unavailable.

Goal: the swappable policy/audit surface. MVP ships `noop`; AGT is wired behind a build tag and a lazy import so the basic binary never requires the dependency.

## T1 — Provider interface + types

Create:
- `internal/policy/policy.go` — `type Action struct{ Kind, Step, RunID, Artifact, Stamp string; Vars map[string]any }`, `type Decision struct{ Allowed bool; Reason string; Evidence map[string]any; Ref string }`, `type Event struct{ Kind string; When string; Payload map[string]any }`, `type Provider interface{ Decide(ctx, Action) (Decision, error); Record(ctx, Event) error; Name() string }`.
- `internal/policy/policy_test.go`

Spec:
- `Action.Kind` ∈ a constant set: `protect`, `loop-step`, `review`, plus an `Action.ProtectedKind` ∈ `{commit, push, pull_request, deploy, release, live_smoke}` when `Kind == "protect"`.
- `Decision.Ref` is a stable opaque id (the provider generates it; recorded into the stamp's `PolicyDecisionRefs` by WS4/WS10 callers).
- `Provider.Name()` returns `"noop"` or `"agt"`.

Tests that must pass:
- `TestActionValidateRejectsUnknownKind`
- `TestProtectActionRequiresProtectedKind`
- `TestDecisionRefIsStable`

Mutation proof:
- Skip the `ProtectedKind` requirement; `TestProtectActionRequiresProtectedKind` must fail.

## T2 — Noop provider

Create:
- `internal/policy/noop.go` — `type Noop struct{ AuditPath string }`, implements `Provider`.
- `internal/policy/noop_test.go`

Spec:
- `Decide` always returns `Decision{Allowed: true, Ref: <generated>}`.
- `Record` appends one JSONL line to `AuditPath` (`.ai/audit.jsonl`) via WS1 pathpolicy. Creates the file if absent.
- `Name()` = `"noop"`.
- Every `Decide` is also recorded as an `Event` (decision-made), so the audit log shows what would have been enforced even when noop allows all.

Tests that must pass:
- `TestNoopAllowsAll`
- `TestNoopRecordsToAuditLog`
- `TestNoopDecideAppendsDecisionEvent`
- `TestNoopAuditPathStaysUnderAI`

Mutation proof:
- Make `Record` a no-op; `TestNoopRecordsToAuditLog` must fail.

## T3 — AGT provider (build-tagged, lazy)

Create:
- `internal/policy/agt.go` — `type AGT struct{...}`, build-tagged `//go:build agt`; lazy-imports the AGT Go SDK only inside its methods (not at package load).
- `internal/policy/agt_test.go` — `//go:build agt`; skips gracefully if SDK unavailable.
- `internal/policy/agt_stub.go` — `//go:build !agt`; a stub `NewAGT(...)` returning `ErrAGTNotCompiled`.

Spec (when built with `-tags agt`):
- `Decide` maps `Action` → AGT input, calls the SDK's policy evaluator (per AGT Go SDK API at impl time — re-verify import path and call shape from `github.com/microsoft/agent-governance-toolkit`), and maps the AGT `PolicyDecision` → our `Decision`. Stores only `Evidence` summary, never the raw provider payload.
- `Record` writes to the AGT tamper-evident audit (and/or mirrors to `.ai/audit.jsonl`).
- `Name()` = `"agt"`.

Tests that must pass (only when `-tags agt` and SDK present; otherwise skip with a clear message):
- `TestAGTProviderDecidesAllow`
- `TestAGTProviderDecidesDeny`
- `TestAGTProviderMapsActionToDecision`
- `TestAGTProviderStoresNoRawPayload`

Without `-tags agt`:
- `TestAGTStubReturnsErrAGTNotCompiled` (in `agt_stub_test.go`).

Mutation proof (agt build only):
- Make `Decide` always allow; `TestAGTProviderDecidesDeny` must fail.

Notes:
- **Do not guess the AGT Go SDK API.** T0 below gates this.

## T0 — Re-verify AGT SDK before coding (do this FIRST)

Before T3:
1. Confirm the Go SDK import path (plan notes `github.com/microsoft/agent-governance-toolkit/agent-governance-golang` but flags it for re-verification).
2. Confirm the evaluator entry point (the `govern()` call shape per AGT docs), the `PolicyDecision` type shape, and the audit API.
3. Confirm minimum Go version and license (MIT, per plan).
Record findings + URLs in `agt.go` package doc. If the API differs materially from assumptions, update this WS's tasks and flag in the completion report.

## T4 — Provider factory + doctor integration

Create:
- `internal/policy/factory.go` — `func New(name, auditPath string) (Provider, error)` — `"noop"` → `Noop`; `"agt"` → AGT (or stub error if not built with the tag).
- `internal/doctor/checks_governance.go` (extend WS3) — `governance.provider_compilable` check: if manifest `governance.provider == agt` and the binary was not built with `-tags agt`, emit an error finding; under strict profile, this is a hard doctor failure (exit 1).
- `internal/policy/factory_test.go`

Spec:
- `New("agt", ...)` without the build tag returns `ErrAGTNotCompiled`.
- The doctor check `governance.agt_required_unavailable` fires when `provider == agt` and `New` returns `ErrAGTNotCompiled`.
- Under `strict` profile, this finding is severity `error` (doctor fails); otherwise `warn`.

Tests that must pass:
- `TestFactoryReturnsNoop`
- `TestFactoryReturnsErrAGTNotCompiledWithoutTag`
- `TestDoctorFailsWhenStrictRequiresAGTButUnavailable`

Mutation proof:
- Make the strict-AGT check a warning; `TestDoctorFailsWhenStrictRequiresAGTButUnavailable` must fail (it will exit 0).

## Verification

```bash
go test ./internal/policy/... ./internal/doctor/... -count=1
go test -tags agt ./internal/policy/... -count=1   # only if SDK vendored
go vet ./internal/policy/...
```

WS12 is ☑ when:
- [ ] T0 AGT re-verification done; findings cited in `agt.go`
- [ ] noop provider tests pass (MVP path)
- [ ] AGT provider compiles under `-tags agt` OR the stub path is proven
- [ ] doctor strict-AGT failure mutation proof executed
- [ ] status updated in `00-overview.md`
