# WS6 — Adapter Templates (incl. Gemini, Crush)

- **Phase:** 4
- **Depends on:** WS2 (templates), WS9 (adapter interface)
- **PRD:** FR-1.1, FR-3.1, FR-3.4
- **Plan:** WS6, "Adapter Templates"
- **Exit gate (Phase 4):** Gemini adapter template renders when selected; Crush template renders only when headless-verified (else ships as `interactive-only` shim with a clear note); adapters do not duplicate long policy.

Goal: the file-generation side for the remaining adapters (Gemini, Crush) plus the runtime adapter implementations for Gemini/Crush that WS9 deferred. WS9 already shipped Codex + Claude.

## T0 — Re-verify external surfaces

Before coding, confirm:
- **Gemini CLI** — `-p`/`--prompt`, `--output-format json`, sandbox/`--yolo`/approval mode, checkpointing, exit codes. Record in `gemini.go`.
- **Crush** — **does a true headless/non-TUI mode exist?** (`crush run`? `--once`? confirm from `github.com/charmbracelet/crush` README + source). This is the gating question. Record findings in `crush.go`.

## T1 — Gemini runtime adapter

Create:
- `internal/adapter/gemini.go` — `type Gemini struct{ Runner Runner }`, implements `Adapter`. Mirror WS9 T4/T5 structure.
- `internal/adapter/gemini_test.go`

Spec:
- `Name()` = `"gemini"`, `Role()` = `RoleOrchestrable`.
- `Run` uses the headless command per T0; non-interactive approval; `--output-format json`; scrub stdout; drop prompt/completion from `ProviderMeta`.
- `Review` symmetric to WS9.
- `Detect.HeadlessCapable` per T0.

Tests that must pass (FakeRunner, mirroring WS9):
- `TestGeminiDetectHeadlessCapability`
- `TestGeminiRunEnforcesNonInteractiveApproval`
- `TestGeminiRunMapsExitCode`
- `TestGeminiRunScrubsSecretsFromStdout`
- `TestGeminiReviewParsesVerdict`

Mutation proof:
- Remove the non-interactive flag; `TestGeminiRunEnforcesNonInteractiveApproval` must fail.

## T2 — Gemini template

Create:
- `templates/adapters/gemini/GEMINI.md` (root adapter pointing to `.ai/INDEX.md`).
- Update WS2 `templates.List` to include Gemini files when `gemini` enabled.

Spec:
- `GEMINI.md` is thin: one paragraph + pointer to `.ai/INDEX.md`. No long policy duplication.

Tests that must pass (extend WS2):
- `TestGeminiAdapterOnlyRendersWhenSelected`
- `TestGeminiAdapterIsThinPointer` (assert it does not duplicate `.ai/rules/*` text verbatim)

Mutation proof:
- Render Gemini always; `TestGeminiAdapterOnlyRendersWhenSelected` must fail.

## T3 — Crush runtime adapter (headless-gated)

Create:
- `internal/adapter/crush.go` — `type Crush struct{ Runner Runner }`.
- `internal/adapter/crush_test.go`

Spec — split by T0 finding:
- **If Crush has a headless mode:** implement like WS9 (`Name="crush"`, `Role=Orchestrable`, `Detect.HeadlessCapable=true`, headless command per T0).
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
- `templates/adapters/crush/README.md` — a note placed at `<repo>/.crush/README.md` explaining mivia-agent manages Crush config and pointing to `.ai/INDEX.md`. No policy duplication.
- Update WS2 `templates.List` to include Crush files when `crush` enabled (regardless of headless — guidance files are still useful).

Spec:
- The Crush template renders for `crush` enabled even when Crush is not headless (it's guidance/config, not orchestration).

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
  --adapter codex --adapter claude --adapter gemini --adapter crush --write
go run ./cmd/mivia-agent doctor --repo "$tmp"
go run ./cmd/mivia-agent adapters --repo "$tmp"
```

WS6 is ☑ when:
- [ ] T0 findings recorded for Gemini AND Crush (headless yes/no explicit)
- [ ] Gemini adapter + template tests pass
- [ ] Crush adapter test path matches T0 finding (headless OR not)
- [ ] non-duplication audit check + mutation proof
- [ ] status updated in `00-overview.md`
