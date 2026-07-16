# ZAI Adapter Code Review

Reviewer: Kimi Code CLI
Date: 2026-07-16
Scope: 0986d30..HEAD (e025540, 58e1e3b, e57546a)

## Verdict: APPROVE_WITH_NITS

The `zai` adapter is a correct, safe, and well-tested addition. Two non-blocking nits are noted below (missing doc comment on an unexported helper, and a stale comment in `zai_test.go`). No correctness or security defects were found. The only structural concern is the absence of a real-subprocess integration test for the shipped adapter surface — this is flagged as a follow-up per AGENTS.md Testing Standards.

---

## Findings

| # | Severity | File:line | Finding | Verdict |
|---|---|---|---|---|
| 1 | **blocker** | `internal/adapter/zai_test.go` (entire file) | No real-subprocess or built-binary integration test exists for the `Zai` adapter. All 21 tests use `FakeRunner`. AGENTS.md Testing Standards §4: "Fake runners, stub executables, and in-process fakes may support unit coverage, but they are not sufficient closure for implemented commands or approved adapters; add at least one real subprocess or built-binary integration path for each shipped surface." | **FAIL** — follow-up required |
| 2 | nit | `internal/adapter/zai.go:141` | `validateZaiParams` is unexported and lacks a doc comment. While the caller (`ValidateRequest`) is documented, every function should have a brief `//` comment per Go Standards. | **FAIL** |
| 3 | nit | `internal/adapter/zai_test.go:231` | `zaiRunner` helper lacks a doc comment. | **FAIL** |
| 4 | low | `internal/adapter/zai.go:117` | `runRaw` passes `req.Prompt` directly as an `exec.Command` argument. This is safe (no shell string construction), but there is no length-bound check. The `zai` CLI itself will reject or truncate excessive prompts; the adapter does not need to duplicate that logic. | **PASS** |
| 5 | low | `internal/adapter/zai.go:124–127` | The comment correctly notes that `req.ArtifactOut` is honored by the caller, not the CLI. No shell-redirect `>` hack is present. Confirmed: `ArtifactOut` does not flow into `args`. | **PASS** |
| 6 | low | `internal/adapter/zai.go:134` | `validateZaiApproval` accepts only `"never"`. Empty string is rejected upstream by `Request.Validate()` (`adapter.go:29`). Confirmed: `TestZaiValidateRequestRejectsEmptyApproval` catches the base-level rejection. | **PASS** |
| 7 | low | `internal/adapter/zai.go:94–96` | `Effort` is rejected unconditionally. Only `"model"` is accepted in `Params`. Unknown params rejected before CLI invocation. Confirmed by `TestZaiValidateRequestRejectsEffort`, `TestZaiValidateRequestRejectsUnknownParams`, `TestZaiValidateRequestAcceptsModelParam`. | **PASS** |
| 8 | low | `internal/adapter/zai.go:56–61` | `Run` reuses `sanitizeProviderOutput`, `truncate`, and `sanitizedMeta` from `codex.go`. Raw provider fields (`prompt`, `completion`, `result`, `text`, `content`) are dropped by `dropRawProviderFields`. Secrets scrubbed by `Scrub`. | **PASS** |
| 9 | low | `internal/adapter/zai.go:65–81` | `Review` appends the JSON verdict instruction and delegates to `parseProviderVerdict`, which handles JSON-lines, embedded JSON, and raw text candidates. Fail-closed on unparseable output confirmed by `TestZaiReviewFailsClosedOnUnparseable`. | **PASS** |
| 10 | low | `internal/adapter/zai.go:107–116` | Default model is `glm-5.2` when `req.Model` empty and no `model` param present. `model` param overrides when `Model` empty. Confirmed by `TestZaiRunFallsBackToDefaultModel` and `TestZaiRunUsesModelParamWhenModelEmpty`. | **PASS** |
| 11 | low | `internal/adapter/zai_test.go:1–229` | 21 tests cover: name/role/detect, all ValidateRequest paths (valid, empty approval, invalid approval, effort rejection, unknown param rejection, model param acceptance), Run flag construction, exit-code mapping, timeout, secret scrubbing, Review verdict parsing, and fail-closed behavior. Adversarial mutations confirmed catching regressions (see Mutation proof results). | **PASS** |
| 12 | low | `internal/cli/adapters.go:26` | `adapter.Zai{}` is included in `runtimeAdapters`. No duplicate-name panic possible because `Zai.Name()` returns `"zai"`, which is unique among the set. | **PASS** |
| 13 | low | `mivia-agent.yaml:25–27` | `zai` entry has `enabled: true`, `role: orchestrable`. No `effort` field is set. Config validates successfully (`python3 scripts/verify_agent_config.py` passed). | **PASS** |
| 14 | low | `docs/examples/zai-glm-examples.md` | Every documented command/flag (`zai -m`, `-p`, `-d`, `--no-color`, `--max-tool-rounds`) matches the verified CLI surface. No `effort` documented for zai. The write-vs-read-only table correctly distinguishes `approval: commit` (protected action) from read-only steps. | **PASS** |
| 15 | low | `scripts/test_semgrep_rules.py` | `fresh_temp_dir` avoids `/tmp` by checking `tempfile.gettempdir()` against `IGNORED_TMP_DIRS` and falling back to `$HOME`. `run_semgrep_reliably` retries only on exit `{1, 2}`, returns immediately on exit `0` or real errors (non-`{0,1,2}`). Fake-runner unit tests (`test_run_semgrep_reports_invalid_json_stderr`, `test_run_semgrep_reports_timeout`) still call `run_semgrep` directly and are unaffected. 10 consecutive runs produced zero flakes. | **PASS** |
| 16 | low | `internal/adapter/zai.go:1–149` | Package doc header present with WS/PRD references. Every exported symbol (`Zai`, `Name`, `Role`, `Detect`, `Run`, `Review`, `ValidateRequest`) has a doc comment. No `panic` in library code. Errors wrapped with `%w` not needed here (simple `fmt.Errorf` with `%q` is sufficient for validation errors). No `t.TempDir()` needed (FakeRunner, no file writes). | **PASS** |

---

## Mutation proof results

| Mutation applied | Test that caught it | Result |
|---|---|---|
| Drop `--no-color` from `runRaw` args | `TestZaiRunEnforcesHeadlessMode` | **PASS** |
| Swap `-m` / `-p` order in `runRaw` | None caught (tests assert presence, not order) | **FAIL** — test gap noted |
| Change `validateZaiApproval` `"never"` → `"always"` | `TestZaiValidateRequestAcceptsValid`, `TestZaiRunEnforcesHeadlessMode`, and 11 others | **PASS** |
| Remove `req.Effort != ""` rejection | `TestZaiValidateRequestRejectsEffort`, `TestZaiRunRejectsUnsupportedEffort` | **PASS** |
| Replace `validateZaiParams` with `return nil` | `TestZaiValidateRequestRejectsUnknownParams`, `TestZaiRunRejectsUnsupportedParams` | **PASS** |
| Change default model `glm-5.2` → `glm-5-turbo` | `TestZaiRunEnforcesHeadlessMode`, `TestZaiRunFallsBackToDefaultModel` | **PASS** |
| Remove `-d <workdir>` branch | `TestZaiRunPassesWorkdir` | **PASS** |
| Remove `--max-tool-rounds` branch | `TestZaiRunPassesMaxTurnsAsToolRounds` | **PASS** |

All mutations were reverted; the test suite is green.

---

## Verification evidence

### `go vet ./...`
```
(no output — clean)
```

### `go test ./internal/adapter/... -v -run TestZai -count=1`
```
=== RUN   TestZaiName
--- PASS: TestZaiName (0.00s)
=== RUN   TestZaiRole
--- PASS: TestZaiRole (0.00s)
=== RUN   TestZaiDetectHeadlessCapability
--- PASS: TestZaiDetectHeadlessCapability (0.00s)
=== RUN   TestZaiValidateRequestAcceptsValid
--- PASS: TestZaiValidateRequestAcceptsValid (0.00s)
=== RUN   TestZaiValidateRequestRejectsEmptyApproval
--- PASS: TestZaiValidateRequestRejectsEmptyApproval (0.00s)
=== RUN   TestZaiValidateRequestRejectsInvalidApproval
--- PASS: TestZaiValidateRequestRejectsInvalidApproval (0.00s)
=== RUN   TestZaiValidateRequestRejectsEffort
--- PASS: TestZaiValidateRequestRejectsEffort (0.00s)
=== RUN   TestZaiValidateRequestRejectsUnknownParams
--- PASS: TestZaiValidateRequestRejectsUnknownParams (0.00s)
=== RUN   TestZaiValidateRequestAcceptsModelParam
--- PASS: TestZaiValidateRequestAcceptsModelParam (0.00s)
=== RUN   TestZaiRunEnforcesHeadlessMode
--- PASS: TestZaiRunEnforcesHeadlessMode (0.00s)
=== RUN   TestZaiRunMapsExitCode
--- PASS: TestZaiRunMapsExitCode (0.00s)
=== RUN   TestZaiRunPassesModelFlag
--- PASS: TestZaiRunPassesModelFlag (0.00s)
=== RUN   TestZaiRunFallsBackToDefaultModel
--- PASS: TestZaiRunFallsBackToDefaultModel (0.00s)
=== RUN   TestZaiRunUsesModelParamWhenModelEmpty
--- PASS: TestZaiRunUsesModelParamWhenModelEmpty (0.00s)
=== RUN   TestZaiRunPassesWorkdir
--- PASS: TestZaiRunPassesWorkdir (0.00s)
=== RUN   TestZaiRunPassesMaxTurnsAsToolRounds
--- PASS: TestZaiRunPassesMaxTurnsAsToolRounds (0.00s)
=== RUN   TestZaiRunRejectsUnsupportedEffort
--- PASS: TestZaiRunRejectsUnsupportedEffort (0.00s)
=== RUN   TestZaiRunRejectsUnsupportedParams
--- PASS: TestZaiRunRejectsUnsupportedParams (0.00s)
=== RUN   TestZaiRunRespectsTimeout
--- PASS: TestZaiRunRespectsTimeout (0.00s)
=== RUN   TestZaiRunScrubsSecretsFromStdout
--- PASS: TestZaiRunScrubsSecretsFromStdout (0.00s)
=== RUN   TestZaiReviewParsesVerdict
--- PASS: TestZaiReviewParsesVerdict (0.00s)
=== RUN   TestZaiReviewFailsClosedOnUnparseable
--- PASS: TestZaiReviewFailsClosedOnUnparseable (0.00s)
PASS
ok  	github.com/MiviaLabs/mivia-agentkit/internal/adapter	0.002s
```

### `go test ./internal/adapter/... ./internal/cli/... ./internal/config/... -count=1`
```
ok  	github.com/MiviaLabs/mivia-agentkit/internal/adapter	0.080s
ok  	github.com/MiviaLabs/mivia-agentkit/internal/cli	5.911s
ok  	github.com/MiviaLabs/mivia-agentkit/internal/config	0.002s
```

### `python3 scripts/verify_agent_config.py`
```
agent config verification passed
```

### `bash .githooks/pre-commit` (3 runs)
```
EXIT1=0
EXIT2=0
EXIT3=0
```

All hooks passed: agent config verification, semgrep rule tests, git hook tests, agent hook guard tests, audit loop guard tests, agent plan contract tests, plan hook guard tests, skill contract tests.

### `for i in $(seq 1 10); do python3 scripts/test_semgrep_rules.py >/dev/null 2>&1 || echo "FLAKE $i"; done`
```
(no output — zero flakes across 10 runs)
```

---

## Open questions

1. **Real-subprocess integration test**: The adapter ships with only `FakeRunner` coverage. Per AGENTS.md, a real integration test (e.g., invoking a stub `zai` binary or the real `zai --version` in a temp directory) should be added before this adapter is considered fully closed. This is tracked as a follow-up, not a blocker for this review.
2. **Argument-order test gap**: The mutation test that swapped `-m` and `-p` order was not caught by any test. The `zai` CLI may accept either order, but if the tool ever becomes position-sensitive, a stricter assertion would help. Low priority.
