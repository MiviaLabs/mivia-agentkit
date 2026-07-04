# WS10 — Orchestrator (DAG + Loops)

- **Phase:** 3
- **Depends on:** WS4 (stamp), WS9 (adapters)
- **PRD:** FR-4.1, FR-4.2, FR-4.3, FR-4.5, FR-6.3, FR-7.1 (partial)
- **Plan:** WS10, "Loops, Routing, And Review"
- **Exit gate (Phase 3, partial):** DAG resolves; review fans out concurrently; loops exit on gate pass; budget rejected; protected action requires fresh stamp; run artifacts contain no raw prompts/outputs.

Goal: the in-process engine that turns a `Loop` into executed steps. Uses `oklog/run` for fan-out. No CLI wiring here (that's WS13); this WS is the library the CLI calls.

## T1 — Runstore

Create:
- `internal/runstore/runstore.go` — `type RunID string`, `type Store struct{ Root string }`, `New(repo string) Store`, `(s Store) NewRun() RunID`, `(s Store) Dir(id RunID) string`, `(s Store) WriteArtifact(id RunID, step, name string, b []byte) (string, error)`, `(s Store) AppendTrace(id RunID, event TraceEvent) error`, `(s Store) ReadArtifact(id RunID, step, name string) ([]byte, error)`.
- `internal/runstore/runstore_test.go`

Spec:
- Root is `<repo>/.ai/runs/`. `NewRun()` returns a RunID = RFC3339-UTC-compact + short random suffix; creates `<Dir>`.
- `WriteArtifact` writes to `<Dir>/<step>/<name>` and returns the abs path. Validate via WS1 pathpolicy (must stay under `.ai/runs/`).
- `TraceEvent` is `{ts, kind, step, iteration, payload map}`. `AppendTrace` appends one JSON object per line to `<Dir>/trace.jsonl`. Stable key order.
- No file outside `.ai/runs/` is ever written.

Tests that must pass:
- `TestNewRunCreatesDir`
- `TestWriteArtifactStaysUnderRuns`
- `TestAppendTraceAppendsJSONL`
- `TestAppendTraceStableKeyOrder`
- `TestReadArtifactRoundTrip`
- `TestWriteArtifactRejectsTraversal`

Mutation proof:
- Drop the pathpolicy check in `WriteArtifact`; `TestWriteArtifactRejectsTraversal` must fail.

## T2 — DAG resolution

Create:
- `internal/orchestrator/dag.go` — `type Node struct{ Step config.Step; DependsOn []string }`, `func Resolve(loop config.Loop) ([]Node, error)`, `(ns []Node) Validate() error`.
- `internal/orchestrator/dag_test.go`

Spec:
- `Resolve` turns a `Loop.Steps` into an ordered node list. Sequential by default; `iterate` edges add a back-dependency from the producer to the review step's notes (the producer depends on review-notes output when re-running).
- `Validate` rejects: cycles, dangling `DependsOn`, missing step `ID`s, and a loop with both a producer step and `reviewers` on the same step (a step is either a producer or a review, not both — mirrors WS1).

Tests that must pass:
- `TestDAGResolvesSequentialHandoff`
- `TestDAGRejectsCycle`
- `TestDAGRejectsDanglingDep`
- `TestDAGRejectsProducerAndReviewOnSameStep`
- `TestDAGRejectsMissingStepID`

Mutation proof:
- Disable cycle detection; `TestDAGRejectsCycle` must fail.

## T3 — Engine: produce + fan-out review

Create:
- `internal/orchestrator/engine.go` — `type Engine struct{ Adapters adapter.Registry; Stamp preflight.Checker; Policy policy.Provider; Store runstore.Store; Clock func() time.Time }`, `func (e Engine) ExecuteStep(ctx, runID RunID, node Node, iteration int) (StepResult, error)`.
- `internal/orchestrator/engine_test.go`

Spec:
- For a producer node: build `adapter.Request` from the rendered prompt (prompt rendering itself is WS13; here accept a `PromptBuilder func(step) (string, error)` injection), call `Adapter.Run`, write the artifact via `Store.WriteArtifact`, append a `step.produced` trace event, return `StepResult{Artifact, Result}`.
- For a review node: fan out `adapter.ReviewRequest` to all `node.Step.Reviewers` concurrently using `oklog/run`. Collect `[]adapter.Verdict`. Append one `step.reviewed` event per reviewer + a `step.consensus` event. Return `StepResult{Verdicts, Consensus}`.
- If the policy provider is non-nil, call `Policy.Decide` before each protected action within the step and record the ref. (In MVP, WS12's noop records but never denies.)
- Enforce per-adapter `Timeout` from the step config (default 5m). Cancellation propagates via ctx.

Tests that must pass (using fake adapters from WS9 + a noop policy):
- `TestExecuteProducerStepWritesArtifact`
- `TestExecuteReviewStepFansOutConcurrently` (assert wall-clock parallelism: 2 fake adapters each sleep 200ms, total < 350ms)
- `TestExecuteReviewStepCollectsAllVerdicts`
- `TestExecuteProducerStepAppendsTrace`
- `TestExecuteStepRespectsTimeout`
- `TestExecuteStepRecordsPolicyDecisionRef`

Mutation proof:
- Serialize the review fan-out (run reviewers sequentially); `TestExecuteReviewStepFansOutConcurrently` must fail (it will exceed 350ms).
- Skip the policy Decide call; `TestExecuteStepRecordsPolicyDecisionRef` must fail.

## T4 — Loop runner

Create:
- `internal/orchestrator/loop.go` — `func (e Engine) RunLoop(ctx, loop config.Loop, pb PromptBuilder) (LoopResult, error)`, `type LoopResult struct{ Outcome, Iterations int; Trace RunID; Err error }`.
- `internal/orchestrator/loop_test.go`

Spec:
- `bound: iterations` only in MVP. `bound: budget` → `ErrBudgetNotSupportedInMVP` (do not even look at the budget fields).
- Loop body: execute nodes in DAG order. After the review node, apply `consensus.Evaluate` (WS11). On pass and `exit_when.gate == review-pass` → return success. On fail with `on_fail: iterate` → feed reviewer notes into the producer's next prompt (via the `PromptBuilder` injection, which receives the prior verdicts), increment iteration, continue. On `on_fail: fail` → return failure immediately.
- `max_iterations` reached without pass → apply `on_exhausted` (`fail` → non-zero, `warn`/`proceed` → success with a trace note).
- Stamp gate: before any step flagged `protect_bound`, call `preflight.CheckStamp`; stale/missing → return `ErrStaleStamp` and halt.
- Return `Iterations` count and the `Trace` RunID.

Tests that must pass (fake adapters scripted to fail-then-pass on iteration 2):
- `TestLoopExitsWhenGatePasses`
- `TestLoopIteratesOnReviewFail`
- `TestLoopFailsOnExhaustionWithOnExhaustedFail`
- `TestLoopWarnsOnExhaustionWithOnExhaustedWarn`
- `TestLoopRejectsBudgetBoundInMVP`
- `TestLoopRequiresFreshStampBeforeProtectedStep` (pre-seed a stale stamp; expect halt)
- `TestLoopHonorsMaxIterationsOverrideWithinManifestBound` (`--max-iterations` cannot exceed manifest)

Mutation proof:
- Remove the budget rejection; `TestLoopRejectsBudgetBoundInMVP` must fail.
- Remove the stamp-before-protect check; `TestLoopRequiresFreshStampBeforeProtectedStep` must fail.
- Allow `--max-iterations` to exceed manifest; `TestLoopHonorsMaxIterationsOverrideWithinManifestBound` must fail.

## T5 — No-leak assertion helper (shared)

Create:
- `internal/orchestrator/leakcheck.go` — `func AssertNoLeaks(t, dir string)` — scan all files under `dir` for raw-prompt / raw-output patterns and secrets (reuse WS9 scrub patterns + a "raw prompt" heuristic).
- (used by WS10 and WS13 tests)

Spec:
- Walks `<dir>`; for each file, asserts it does not contain a `ProviderMeta`-shaped `prompt:`/`completion:`/`content:` field, and no secret pattern.
- Designed to be called in tests over the runstore dir after a loop executes.

Tests that must pass:
- `TestAssertNoLeaksPassesOnCleanDir`
- `TestAssertNoLeaksFlagsSecret`
- `TestAssertNoLeaksFlagsRawPromptField`

Mutation proof:
- Empty the pattern set; `TestAssertNoLeaksFlagsSecret` must fail.

## Verification

```bash
go test ./internal/runstore/... ./internal/orchestrator/... -count=1
go vet ./internal/runstore/... ./internal/orchestrator/...
# Fan-out parallelism assertion:
go test ./internal/orchestrator/ -run TestExecuteReviewStepFansOutConcurrently -count=1 -v
```

WS10 is ☑ when:
- [x] all listed tests pass
- [x] fan-out genuinely concurrent (timing assertion green)
- [x] stamp-before-protect, budget-rejection, max-iterations mutation proofs executed (3)
- [x] no-leak check integrated
- [x] status updated in `00-overview.md`

## Completion — 2026-07-05

- Tests: 26 passing in `go test ./internal/runstore/... ./internal/orchestrator/... -count=1`.
- Mutation proofs: runstore traversal failed `TestWriteArtifactRejectsTraversal`; cycle detection removal failed `TestDAGRejectsCycle`; serialized review fan-out failed `TestExecuteReviewStepFansOutConcurrently`; skipped policy decision failed `TestExecuteStepRecordsPolicyDecisionRef`; budget support removal failed `TestLoopRejectsBudgetBoundInMVP`; stamp gate removal failed `TestLoopRequiresFreshStampBeforeProtectedStep`; max-iteration cap removal failed `TestLoopHonorsMaxIterationsOverrideWithinManifestBound`; empty leak patterns failed `TestAssertNoLeaksFlagsSecret`.
- Verification: `go test ./internal/runstore/... ./internal/orchestrator/... -count=1`, `go vet ./internal/runstore/... ./internal/orchestrator/...`, fan-out focused test, no-network grep, `python3 scripts/validate_agent_plan.py .ai/plans/agentkit-implementation-roadmap.plan.json`, and `git diff --check` passed.
- Files: 10 created, 2 docs updated.
- Residual risk: WS10 uses an internal all-reviewers-pass consensus gate until WS11 lands the full consensus package.
- Follow-ups: implement WS11 consensus modes and replace the temporary internal pass gate.
