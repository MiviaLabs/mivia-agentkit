# WS6 — Adapter Templates (incl. Antigravity, Crush)

- **Phase:** 4
- **Depends on:** WS2 (templates), WS9 (adapter interface)
- **PRD:** FR-1.1, FR-3.1, FR-3.4
- **Plan:** WS6, "Adapter Templates"
- **Exit gate (Phase 4):** Antigravity adapter template renders when selected; Crush template renders when selected and can be orchestrated only when local `crush run --help` verifies noninteractive support; adapters do not duplicate long policy.

Goal: the file-generation side for the remaining adapters (Antigravity, Crush) plus the runtime adapter implementations for Antigravity/Crush that WS9 deferred. WS9 already shipped Codex + Claude.

## T0 — Re-verify external surfaces

Before coding, confirm:
- **Antigravity CLI** — current Google transition state and `agy -p` one-shot mode. Gemini CLI is not a supported target for this workstream. Record in `antigravity.go`.
- **Crush** — **does a true headless/non-TUI mode exist?** Current local v0.79.1 exposes `crush run [prompt...]` with stdin, `--cwd`, `--model`, and `--quiet`. The adapter must still gate headless capability on `crush run --help` at detection time and record findings in `crush.go`.

## T1 — Antigravity runtime adapter

Create:
- `internal/adapter/antigravity.go` — current Antigravity CLI adapter implementation, implements `Adapter`. Mirror WS9 T4/T5 structure.
- `internal/adapter/antigravity_test.go`

Spec:
- `Name()` = `"antigravity"`, `Role()` = `RoleOrchestrable`.
- `Run` uses the current Antigravity CLI command per T0 (`agy -p`) under the stable `"antigravity"` adapter name; scrub stdout; drop prompt/completion from `ProviderMeta`.
- `Review` symmetric to WS9.
- `Detect.HeadlessCapable` per T0.

Tests that must pass (FakeRunner, mirroring WS9):
- `TestAntigravityDetectHeadlessCapability`
- `TestAntigravityRunEnforcesNonInteractiveApproval`
- `TestAntigravityRunMapsExitCode`
- `TestAntigravityRunScrubsSecretsFromStdout`
- `TestAntigravityReviewParsesVerdict`

Mutation proof:
- Revert to legacy `gemini --output-format json --yolo`; `TestAntigravityRunEnforcesNonInteractiveApproval` must fail.

## T2 — Antigravity template

Create:
- `templates/adapters/antigravity/GEMINI.md` (Antigravity-readable root context file pointing to `.ai/INDEX.md`).
- Update WS2 `templates.List` to include Antigravity files when `antigravity` enabled.

Spec:
- `GEMINI.md` is thin: one paragraph + pointer to `.ai/INDEX.md`. No long policy duplication.

Tests that must pass (extend WS2):
- `TestAntigravityAdapterOnlyRendersWhenSelected`
- `TestAntigravityAdapterIsThinPointer` (assert it does not duplicate `.ai/rules/*` text verbatim)

Mutation proof:
- Render Antigravity always; `TestAntigravityAdapterOnlyRendersWhenSelected` must fail.

## T3 — Crush runtime adapter (headless-gated)

Create:
- `internal/adapter/crush.go` — `type Crush struct{ Runner Runner }`.
- `internal/adapter/crush_test.go`

Spec — split by T0 finding:
- **If Crush has a headless mode:** implement like WS9 (`Name="crush"`, `Role=Orchestrable`, `Detect.HeadlessCapable=true` only when `crush run --help` confirms support, headless command per T0).
- **If Crush has NO headless mode:** `Name="crush"`, `Role=Guidance`, `Detect.HeadlessCapable=false`. `Run` returns `ErrNotHeadlessCapable`. `Review` likewise. The adapter exists in the registry (so `adapters` lists it) but is excluded from `run` (WS9 FR-3.4 path).

Tests that must pass:
- `TestCrushDetectReportsHeadlessCapability` (asserts the T0 finding — headless true OR false, recorded)
- `TestCrushRunErrorsWhenNotHeadless` (skip if T0 says headless; else assert `ErrNotHeadlessCapable`)
- `TestCrushRunEnforcesNonInteractiveApproval` (skip if not headless)
- `TestCrushRunScrubsSecretsFromStdout` (skip if not headless)

Mutation proof:
- If headless: remove the approval flag; the approval test must fail.
- If not headless: make `Run` succeed anyway; `TestCrushRunErrorsWhenNotHeadless` must fail.

## T4 — Crush template (shim + README)

Create:
- `templates/adapters/crush/README.md` — a note placed at `<repo>/.crush/README.md` explaining mivia-agent can orchestrate Crush through `crush run`, documents Ollama/Qwen setup, and points to `.ai/INDEX.md`. No policy duplication.
- Update WS2 `templates.List` to include Crush files when `crush` enabled (regardless of headless — guidance files are still useful).

Spec:
- The Crush template renders for `crush` enabled and documents that orchestration is gated by local `crush run --help` support.

Tests that must pass:
- `TestCrushAdapterShimRendersWhenSelected`
- `TestCrushShimDoesNotDuplicateLongPolicy`

Mutation proof:
- Duplicate policy text into the shim; `TestCrushShimDoesNotDuplicateLongPolicy` must fail.

## T5 — Adapter-non-duplication audit check (extend WS3)

Create / extend:
- `internal/audit/checks_adapter_policy.go` — `policy.duplicated_in_adapters` now compares each adapter root file against `.ai/rules/*` blocks and flags any verbatim block > N lines.

Tests that must pass:
- `TestAdaptersDoNotDuplicateLongPolicy`

Mutation proof:
- Set N very high so nothing flags; `TestAdaptersDoNotDuplicateLongPolicy` (with a fixture that duplicates a known block) must fail.

## Verification

```bash
go test ./internal/adapter/... ./internal/render/... ./internal/audit/... -count=1
go vet ./internal/adapter/...
tmp=$(mktemp -d) && (cd "$tmp" && git init -q && git commit -q --allow-empty -m init)
go run ./cmd/mivia-agent init --repo "$tmp" --profile standard \
  --adapter codex --adapter claude --adapter antigravity --adapter crush --write
go run ./cmd/mivia-agent doctor --repo "$tmp"
go run ./cmd/mivia-agent adapters --repo "$tmp"
```

WS6 is ☑ when:
- [ ] T0 findings recorded for Antigravity AND Crush (headless yes/no explicit)
- [ ] Antigravity adapter + template tests pass
- [ ] Crush adapter test path matches T0 finding (headless OR not)
- [ ] non-duplication audit check + mutation proof
- [ ] status updated in `00-overview.md`

## Completion — 2026-07-05

- Tests: focused config, adapter, templates, render, audit, and CLI packages passing.
- Mutation proofs: Antigravity legacy-flag reintroduction fail-then-revert ok; Antigravity always-render fail-then-revert ok; Crush `Run` success fail-then-revert ok; duplicate-policy threshold raise fail-then-revert ok.
- Files: Antigravity runtime adapter, Crush guidance adapter, adapter-policy audit check, template/catalog updates, manifest parser fix, and CLI adapter listing update.
- Residual risk: `doctor` smoke reports the expected temp-repo CI warning; no WS6 blocker. Later local Crush v0.79.1 evidence found `crush run` support and superseded the previous guidance-only Crush branch.
- Follow-ups: none.

## Completion addendum — 2026-07-05

- Tests: focused adapter, CLI, and template packages passing with Crush as an orchestrable runtime adapter.
- Live smoke: default Crush model failed because local `codestral:latest` is not installed; explicit `--model ollama/qwen3-coder:latest` passed through `crush run --quiet --cwd <repo>` with stdin and returned parseable JSON.
- Mutation proofs: removing `crush run --help` detection failed `TestCrushDetectHeadlessRunSupport`; dropping `--cwd` failed `TestCrushRunInvokesCrushRunWithCWDModelAndPrompt`; ignoring unsupported effort failed `TestCrushRunRejectsUnsupportedEffort`; breaking JSON verdict parsing failed `TestCrushReviewParsesJSONVerdict`; all reverted.
- Files: Crush adapter, runner stdin support, CLI subprocess seam test, and Crush templates updated.
- Residual risk: installed Crush runtime still depends on local model/provider configuration; verified working with `ollama/qwen3-coder:latest`.
- Follow-ups: none.
