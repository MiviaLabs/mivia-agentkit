# WS13 — `run`, `review`, `adapters` Commands

- **Phase:** 2 (`adapters`), 3 (`run`/`review`)
- **Depends on:** WS9 (adapters), WS10 (orchestrator), WS11 (consensus), WS12 (policy)
- **PRD:** FR-3.2, FR-4.1–4.5, FR-5.3
- **Plan:** WS13, "run / review / adapters commands"
- **Exit gate (Phase 2):** `adapters` reports presence + headless capability. **(Phase 3):** `run --dry-run` prints a plan without invoking; `run` executes the research-loop fixture end-to-end behind fake adapters; `review` returns a consensus verdict.

Goal: the user-facing CLI for orchestration. Thin: build the objects from WS9–WS12, delegate, render output.

## T1 — Prompt building

Create:
- `internal/cli/prompt.go` — `type PromptBuilder struct{ Repo string; TplFS fs.FS; Vars map[string]string }`, `(b PromptBuilder) Producer(step config.Step, priorNotes []adapter.Verdict) (string, error)`, `(b PromptBuilder) Reviewer(step config.Step, artifactPath string) (string, error)`.
- `internal/cli/prompt_test.go`

Spec:
- Templates live under `.ai/workflows/<loop>/prompts/` if present, else embedded defaults under `templates/prompts/`.
- `Producer` merges prior-iteration reviewer notes into the prompt when `priorNotes` is non-empty (the iterate loop's input).
- `Reviewer` builds a prompt that instructs the adapter to emit the strict JSON verdict schema defined in WS9.
- Undefined referenced variable → error (same rule as WS2 renderer).

Tests that must pass:
- `TestProducerPromptIncludesPriorNotesOnIterate`
- `TestReviewerPromptRequestsJSONVerdict`
- `TestProducerPromptErrorsOnUndefinedVar`

Mutation proof:
- Drop the prior-notes injection; `TestProducerPromptIncludesPriorNotesOnIterate` must fail.

## T2 — `adapters` command

Create:
- `internal/cli/adapters.go` — `adaptersCmd`, flags `--repo`, `--json`. Builds a `adapter.Registry` of all known adapters (Codex, Claude, Gemini stub, Crush stub from WS6), calls `Detect` on each.
- `internal/cli/adapters_test.go`

Spec:
- Output columns: `name | installed | headless | role | approved_for_run`.
- `approved_for_run` = installed && headless && role==orchestrable.
- `--json` emits `[{name, installed, version, headless, role, approved_for_run}]`.
- Re-runs `Detect` on every invocation (no caching) so probe-time reflects the current machine.
- Exit non-zero if any adapter enabled as `orchestrable` in the manifest is not headless-capable.

Tests that must pass (with fake-CLI shims on PATH pointing to a script that prints a known version):
- `TestAdaptersReportsHeadlessCapability`
- `TestAdaptersExitsNonZeroWhenOrchestrableAdapterNotHeadless`
- `TestAdaptersJSONShape`

Mutation proof:
- Make `approved_for_run` ignore `headless`; `TestAdaptersExitsNonZeroWhenOrchestrableAdapterNotHeadless` must fail.

Notes:
- Tests must not require real Codex/Claude binaries. Use a fake-binary fixture (a small script that responds to `--version`) placed on a temp PATH.

## T3 — `run` command

Create:
- `internal/cli/run.go` — `runCmd`, flags `--repo`, `--workflow`, `--step`, `--input-artifact`, `--var key=value` (repeated), `--max-iterations`, `--dry-run`, `--json`, `--strict`.
- `internal/cli/run_test.go`

Spec:
- Load manifest + the named workflow (from `mivia-agent.yaml.loops` or `.ai/workflows/<name>.yaml`). Validate via WS1.
- Build the orchestrator `Engine` (WS10) with: registry from `adapters` (only headless-approved), policy provider from manifest (WS12), stamp checker (WS4), runstore.
- `--dry-run`: build the DAG, print `[{step, type, adapter(s), max_turns, timeout, artifact}]`, write nothing, invoke nothing. Exit 0.
- Without `--dry-run`: `Engine.RunLoop`. Stream JSONL events to stdout when `--json`. On finish, print a concise summary (iterations, outcome, trace path, exit code).
- `--max-iterations` caps the loop but cannot exceed the manifest value (WS10 enforces; CLI surfaces the error).
- Exit codes: 0 success, 1 loop failed / adapter error / policy denial, 2 warn-only.

Tests that must pass (all behind fake adapters + fake-CLI fixtures; no real CLIs):
- `TestRunDryRunPrintsPlanWithoutInvoking` (assert zero subprocess invocations recorded by the fake runner)
- `TestRunExecutesResearchLoopFixture` (fake adapters scripted pass-on-iteration-2; assert trace JSONL shape + outcome + exit code 0)
- `TestRunIteratesOnReviewFail` (fake adapters scripted fail-then-pass; assert iterations==2 and reviewer notes fed back)
- `TestRunFailsOnExhaustion`
- `TestRunRejectsUnknownWorkflow`
- `TestRunRejectsBudgetBoundLoopInMVP`
- `TestRunArtifactContainsNoRawPromptsOrOutputs` (uses WS10 `AssertNoLeaks`)
- `TestRunStrictFailsOnFirstPassConsensusForProtectBound`

Mutation proof:
- Make `--dry-run` invoke; `TestRunDryRunPrintsPlanWithoutInvoking` must fail.

## T4 — `review` command

Create:
- `internal/cli/review.go` — `reviewCmd`, flags `--repo`, `--artifact`, `--reviewers a,b,c`, `--mode`, `--min-reviewers`, `--weights k=v,...`, `--tie-breaker`, `--json`.
- `internal/cli/review_test.go`

Spec:
- A one-off: build a synthetic single-review `Step`, fan out via the engine's review path (or directly via `adapter.Review` + `oklog/run`), apply `consensus.Evaluate`.
- Default `--mode` from manifest routing; default `--min-reviewers` from manifest.
- Output: verdicts + outcome + reason.
- Exit 0 on pass, 1 on fail, 2 on tie-with-manual.

Tests that must pass (fake adapters):
- `TestReviewOneOffConsensus`
- `TestReviewRespectsWeights`
- `TestReviewRejectsMinReviewersUnsatisfied`
- `TestReviewArtifactMustExist`

Mutation proof:
- Bypass consensus (return pass on any verdict); `TestReviewOneOffConsensus` (scripted fail) must fail.

## Verification

```bash
go test ./internal/cli/... -count=1
go vet ./internal/cli/...
# Smoke (uses fakes):
tmp=$(mktemp -d) && (cd "$tmp" && git init -q && git commit -q --allow-empty -m init)
go run ./cmd/mivia-agent init     --repo "$tmp" --profile standard --adapter codex --adapter claude --write
go run ./cmd/mivia-agent adapters --repo "$tmp" --json
go run ./cmd/mivia-agent run      --repo "$tmp" --workflow research --dry-run --json
```

WS13 is ☑ when:
- [ ] all listed tests pass
- [ ] `--dry-run` invokes nothing (proven by fake-runner call count)
- [ ] research-loop fixture completes pass/fail/iterate cycle
- [ ] no-leak assertion green over the runstore
- [ ] mutation proofs executed (run dry-run, review-bypass) — ≥2
- [ ] status updated in `00-overview.md`
