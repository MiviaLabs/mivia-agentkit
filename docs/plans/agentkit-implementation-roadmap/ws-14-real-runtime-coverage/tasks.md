# WS14 — Real Runtime Coverage

- **Phase:** 5
- **Depends on:** WS2, WS3, WS4, WS5, WS6, WS7, WS9, WS10, WS11, WS12, WS13
- **PRD:** §3, §4, §5, §6, §7, §9, §14 (Phase 5 gate)
- **Plan:** WS14
- **Exit gate (Phase 5):** every implemented user-facing command and every approved-for-run adapter has real runtime coverage beyond fake-only unit tests.

Goal: close the remaining proof gap between fake-runner unit coverage and the real shipped surfaces by adding deterministic built-binary and opt-in local-CLI integration coverage.

## T1 — Real integration gate contract

Create:
- `internal/integration/gate.go` — shared gate for real runtime tests (`Gate`, `ToolStatus`, `RequireBinary`, `RequireEnv`)
- `internal/integration/gate_test.go` — gate contract tests

Spec:
- Centralize skip-or-run decisions for real integration coverage.
- Distinguish missing binaries, missing env, and explicit opt-in disabled states.
- Produce deterministic skip messages that name the missing prerequisite.

Tests that must pass:
- `TestGateRequireBinaryReportsMissingTool`
- `TestGateRequireEnvReportsMissingVariable`
- `TestGateAllowsExplicitlyEnabledRun`

Dependencies:
- `stdlib only`

Mutation proof:
- Return success when a required binary is missing; `TestGateRequireBinaryReportsMissingTool` must fail.

## T2 — Built-binary command harness

Create:
- `internal/cli/integration_harness.go` — builds and executes the real `mivia-agent` binary in temp repos (`BuildBinary`, `RunBinary`)
- `internal/cli/integration_harness_test.go` — harness tests

Spec:
- Build once per test package run and execute the produced binary as a subprocess.
- Allow per-test temp repo setup and explicit environment shaping.
- Capture stdout, stderr, exit code, and scrubbed fixture paths for assertions.

Tests that must pass:
- `TestBuildBinaryProducesRunnableExecutable`
- `TestRunBinaryCapturesExitCodeAndStreams`

Dependencies:
- `internal/integration`

Mutation proof:
- Fall back to in-process command execution instead of the built binary; `TestBuildBinaryProducesRunnableExecutable` must fail.

## T3 — Core command real coverage

Create:
- `test/integration/core_commands_test.go` — built-binary integration tests for `init`, `doctor`, `audit`, `preflight`, `import`, `update`, `version`
- `test/integration/core_commands_test_helpers.go` — shared fixture helpers for the command integration suite

Spec:
- Use the real built binary only.
- Cover happy path, fail-closed path, and idempotency where the command writes files.
- Assert generated `.ai/` surface shape, doctor/audit exit behavior, preflight stamp write, import inspection, update managed-region preservation, and non-`dev`/default-safe `version` output shape.

Tests that must pass:
- `TestInitDoctorAuditPreflightFlow`
- `TestUpdatePreservesUserContentOutsideManagedRegions`
- `TestImportInspectsWithoutWriting`
- `TestVersionCommandOutputsStructuredBuildInfo`

Dependencies:
- `internal/cli`
- `internal/integration`

Mutation proof:
- Skip the subprocess path and call package helpers directly; `TestInitDoctorAuditPreflightFlow` must fail.

## T4 — Hook, run, and review real coverage

Create:
- `test/integration/orchestrated_commands_test.go` — built-binary integration tests for `hook`, `run`, and `review`
- `test/integration/orchestrated_commands_test_helpers.go` — orchestrated fixture helpers

Spec:
- Execute the real binary against temp repositories and temp manifests.
- Cover bounded workflow execution, hook policy rejection/allow behavior, and one-off review consensus output without network access.
- Assert stamp enforcement, bounded-loop reporting, and structured review/report outputs.

Tests that must pass:
- `TestHookRejectsProtectedActionWithoutFreshStamp`
- `TestRunDryRunProducesBoundedPlan`
- `TestReviewProducesConsensusReport`

Dependencies:
- `internal/cli`
- `internal/integration`

Mutation proof:
- Remove stamp enforcement from the hook integration path; `TestHookRejectsProtectedActionWithoutFreshStamp` must fail.

## T5 — Real adapter subprocess coverage

Create:
- `test/integration/adapters_real_test.go` — opt-in subprocess integration suite for approved adapters (`codex`, `claude`, `antigravity`, `crush`)
- `test/integration/adapters_real_test_helpers.go` — adapter fixture helpers and stub executable support

Spec:
- Cover the real adapter subprocess boundary with explicit gating for local tool availability and required env.
- Verify detect, non-interactive run shaping, review shaping, approval-mode enforcement, and scrubbed artifact handling at the subprocess boundary.
- Use stub executables where the risk is argv/env shaping; use installed CLIs only when the contract under test requires the real binary.

Tests that must pass:
- `TestCodexAdapterRealSubprocessContract`
- `TestClaudeAdapterRealSubprocessContract`
- `TestAntigravityAdapterRealSubprocessContract`
- `TestCrushAdapterRealSubprocessContract`

Dependencies:
- `internal/adapter`
- `internal/integration`

Mutation proof:
- Stop passing the adapter non-interactive flag set through the subprocess boundary; the matching adapter real-contract test must fail.

## T6 — Policy and docs closure for real coverage

Create / extend:
- `semgrep/agent-standards.yml` — ban fake-only closure guidance and fake-runner usage inside real integration test files
- `scripts/test_semgrep_rules.py` — rule fixtures for the real-coverage policy

Spec:
- Repo policy must reject guidance that claims fake-only coverage is sufficient for adapters or shipped commands.
- Real integration tests must not close over `FakeRunner` or equivalent fake adapter helpers.
- Keep unit-test fake coverage allowed outside the dedicated real integration suites.

Tests that must pass:
- `python3 scripts/test_semgrep_rules.py`

Dependencies:
- `Semgrep`
- `Python 3`

Mutation proof:
- Reintroduce the stale fake-only guidance text or use `FakeRunner` in a real integration fixture; `python3 scripts/test_semgrep_rules.py` must fail.

## Verification

```bash
go test ./internal/integration ./internal/cli/... ./test/integration/... -count=1
go vet ./internal/integration ./internal/cli/... ./test/integration/...
MIVIA_AGENT_REAL_CLI_TESTS=1 go test ./test/integration/... -count=1
python3 scripts/test_semgrep_rules.py
```

WS14 is ☑ when:
- [ ] all listed tests pass
- [ ] built-binary command coverage exists for every implemented user-facing command
- [ ] opt-in subprocess adapter coverage exists for every approved-for-run adapter
- [ ] all mutation proofs executed and reverted (results in completion report)
- [ ] `go vet` clean for the covered packages
- [ ] no network calls added
- [ ] status updated in `00-overview.md`

## Completion — 2026-07-05

- Tests: 16 passing.
- Mutation proofs: integration gate missing-binary bypass fail-then-revert ok; built-binary harness in-process bypass fail-then-revert ok; hook stamp-enforcement bypass fail-then-revert ok; Codex non-interactive flag removal fail-then-revert ok; real-integration FakeRunner Semgrep bypass fail-then-revert ok.
- Files: 10 created.
- Residual risk: installed third-party CLI checks remain opt-in and may skip locally when the named binary is absent.
- Follow-ups: none.
