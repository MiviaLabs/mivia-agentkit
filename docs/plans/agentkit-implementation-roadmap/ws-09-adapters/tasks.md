# WS9 — Adapter System

- **Phase:** 2
- **Depends on:** WS1
- **PRD:** FR-3.1, FR-3.2, FR-3.3, FR-3.4, FR-3.5, FR-7.4
- **Plan:** WS9, "Adapter System"
- **Exit gate (Phase 2):** Codex + Claude adapters implemented behind a fake runner; non-interactive approval enforced; results scrubbed; `adapters` command reports capability.

Goal: the swappable `Adapter` interface and the first two concrete adapters (Codex, Claude), with Gemini/Crush following the same shape in WS6. Every adapter is testable without the real CLI via a fake runner.

## T0 — Re-verify external surfaces (do this FIRST, before coding)

Before implementing, confirm against current official docs (URLs in plan "External Facts"):

- **Codex CLI** — exact `codex exec` flags: prompt input (stdin vs `--prompt`), `--output-last-message <path>`, approval mode flag (`--full-auto` / `--dangerously-bypass-approvals-and-sandbox` / `--ask-for-approval never` — whichever is current), sandbox mode, `--json` output, exit codes. Record the doc URL + version in `codex.go` package doc.
- **Claude Code** — `claude -p`/`--print`, `--output-format json|stream-json`, `--permission-mode {default,acceptEdits,bypassPermissions,plan}`, `--max-turns`, `--allowedTools`/`--disallowedTools`. Record doc URL + version in `claude.go`.
- **Gemini CLI** (deferred to WS6) — note for later.
- **Crush** (deferred to WS6) — note headless capability question for later.

If a flag set has drifted from the plan's assumptions, update `codex.go`/`claude.go` and the tests; cite the change in the WS completion report. **Do not guess.**

## T1 — Adapter interface + types

Create:
- `internal/adapter/adapter.go` — `type Role int` (`RoleOrchestrable`, `RoleGuidance`), `type Presence struct{ Binary string; Version string; Installed bool; HeadlessCapable bool }`, `type ApprovalMode string` (constants), `type Request struct{...}`, `type Result struct{...}`, `type ReviewRequest struct{...}`, `type Verdict struct{ Adapter string; Pass bool; Severity string; Notes string; EvidenceRef string }`, `type Adapter interface{ Name() string; Role() Role; Detect(ctx) (Presence,error); Run(ctx,Request) (Result,error); Review(ctx,ReviewRequest) (Verdict,error) }`.
- `internal/adapter/adapter_test.go`

Spec:
- `Request`: `Prompt, Workdir string; Approval ApprovalMode; MaxTurns int; Timeout time.Duration; ArtifactOut string; Env []string`.
- `Result`: `ExitCode int; Stdout, Stderr []byte; Artifact string; Turns int; ProviderMeta map[string]any`.
- `ReviewRequest`: `ArtifactPath, Prompt string; Approval ApprovalMode; MaxTurns int; Timeout time.Duration`.
- `Approval` MUST be one of the non-interactive constants; a request with empty `Approval` is invalid.
- A package-level `Registry` registers adapters by name.

Tests that must pass:
- `TestRegistryLookupByName`
- `TestRequestRejectsEmptyApproval`

Mutation proof:
- Allow empty approval in validation; `TestRequestRejectsEmptyApproval` must fail.

## T2 — Process runner + fake runner

Create:
- `internal/adapter/runner.go` — `type Runner interface{ Run(ctx, args, env, workdir) (ExitCode int, Stdout, Stderr []byte, err error) }`, default `osRunner` using `go-cmd/go-cmd` (live streaming captured into buffers; respects ctx; returns on timeout).
- `internal/adapter/fake_runner.go` — `type FakeRunner struct{...}` for tests: scripted responses by command name; records every invocation (`Calls []RecordedCall`).
- `internal/adapter/runner_test.go`

Spec:
- `osRunner` enforces a `context`-derived deadline; on timeout kills the process and returns a sentinel error.
- `osRunner` captures stdout/stderr fully (bounded — truncate at e.g. 1 MiB to avoid runaway memory).
- `FakeRunner.Run(ctx, args, env, workdir)` looks up the scripted response for `args[0]`, returns it, and appends a `RecordedCall{Args, Env, Workdir, Approval}`.

Tests that must pass:
- `TestOSRunnerRespectsTimeout` (use `sleep 30` with a short ctx)
- `TestOSRunnerTruncatesLargeStdout`
- `TestFakeRunnerRecordsInvocation`
- `TestFakeRunnerScriptsByCommandName`

Mutation proof:
- Ignore ctx in `osRunner`; `TestOSRunnerRespectsTimeout` must fail.

Notes:
- Real CLI binaries are never invoked in tests. Every adapter test injects a `FakeRunner`.

## T3 — Secret scrubber

Create:
- `internal/adapter/scrub.go` — `Scrub(b []byte) []byte`, `var SecretPatterns = [][]byte{...}`.
- `internal/adapter/scrub_test.go`

Spec:
- Patterns: AWS key-shaped (`AKIA[0-9A-Z]{16}`), generic `Bearer <token>`, GitHub PAT-shaped (`gh[pousr]_[A-Za-z0-9]{36,}`), JWT-shaped (`eyJ...`), explicit-looking env assignments (`(SECRET|TOKEN|PASSWORD|API_KEY)=\S+`).
- Matches replaced with `<redacted:kind>`.
- A scrubbed buffer re-scrubbed yields the same bytes (idempotent).

Tests that must pass:
- `TestScrubAWSKey`
- `TestScrubBearerToken`
- `TestScrubGitHubPAT`
- `TestScrubEnvAssignment`
- `TestScrubIdempotent`
- `TestScrubLeavesNonSecrets`

Mutation proof:
- Empty the pattern list; `TestScrubAWSKey` must fail.

## T4 — Codex adapter

Create:
- `internal/adapter/codex.go` — `type Codex struct{ Runner Runner; ... }`, implements `Adapter`.
- `internal/adapter/codex_test.go`

Spec:
- `Name()` = `"codex"`, `Role()` = `RoleOrchestrable`.
- `Detect`: shell out to `codex --version` (via runner); parse version; assume `HeadlessCapable=true` (confirm per T0).
- `Run`: build the headless command (`codex exec ...` per T0 findings) with the non-interactive approval flag, `--output-last-message <ArtifactOut>`, `--json` for metadata; enforce `Request.Timeout` and `Request.MaxTurns`. Invoke via `Runner.Run`. Scrub `Stdout`/`Stderr`. Populate `Result.ProviderMeta` with `model_id`, `total_tokens` (parsed from JSON, never the prompt/output). If the parsed JSON includes any prompt or completion text, drop it.
- `Review`: render a review prompt, `Run` it, parse a structured `Verdict` from the artifact (define a strict JSON schema the review prompt must produce; if parsing fails, return `Verdict{Pass:false, Severity:"error", Notes:"unparseable review output"}`).

Tests that must pass (all via `FakeRunner`):
- `TestCodexDetectHeadlessCapability`
- `TestCodexRunEnforcesNonInteractiveApproval` (FakeRunner records the approval flag in argv; assert its presence)
- `TestCodexRunMapsExitCode`
- `TestCodexRunScrubsSecretsFromStdout`
- `TestCodexRunTruncatesLargeStdout`
- `TestCodexRunRespectsTimeout`
- `TestCodexRunDropsPromptAndCompletionFromMeta`
- `TestCodexReviewParsesVerdict`
- `TestCodexReviewFailsClosedOnUnparseable`

Mutation proof:
- Remove the approval flag from the Codex argv; `TestCodexRunEnforcesNonInteractiveApproval` must fail.
- Skip scrubbing; `TestCodexRunScrubsSecretsFromStdout` must fail.

## T5 — Claude adapter

Create:
- `internal/adapter/claude.go` — `type Claude struct{ Runner Runner; ... }`.
- `internal/adapter/claude_test.go`

Spec:
- Same structure as Codex (T4). `Name()` = `"claude"`.
- `Run` uses `claude -p --output-format json --permission-mode <Request.Approval> --max-turns <n>` (confirm per T0). Parse the JSON event stream for `total_tokens`, `model`, exit reason; drop any text/content fields from `ProviderMeta`.
- `Review` symmetric to Codex.

Tests that must pass (mirror T4):
- `TestClaudeDetectHeadlessCapability`
- `TestClaudeRunEnforcesNonInteractiveApproval`
- `TestClaudeRunMapsExitCode`
- `TestClaudeRunScrubsSecretsFromStdout`
- `TestClaudeRunDropsPromptAndCompletionFromMeta`
- `TestClaudeReviewParsesVerdict`
- `TestClaudeReviewFailsClosedOnUnparseable`

Mutation proof:
- Remove the `--permission-mode` flag; `TestClaudeRunEnforcesNonInteractiveApproval` must fail.

## Verification

```bash
go test ./internal/adapter/... -count=1
go vet ./internal/adapter/...
# Real CLIs are NOT exercised in tests; FakeRunner only.
# Confirm no test reaches a real binary:
grep -rn 'exec.Command\|os/exec' ./internal/adapter/*_test.go || echo "no real exec in tests (good)"
```

WS9 is ☑ when:
- [x] T0 re-verification done; doc URLs + versions cited in adapter package docs
- [x] all listed tests pass (Codex + Claude behind FakeRunner)
- [x] approval-enforcement mutation proofs executed (2)
- [x] scrubbing mutation proof executed (1)
- [x] no real CLI invocation in tests
- [x] status updated in `00-overview.md`

## Completion — 2026-07-05

- Tests: 35 passing in `go test ./internal/adapter/... -count=1`.
- Mutation proofs: empty approval validation failed `TestRequestRejectsEmptyApproval`; Codex approval flag removal failed `TestCodexRunEnforcesNonInteractiveApproval`; Claude permission flag removal failed `TestClaudeRunEnforcesNonInteractiveApproval`; scrub pattern removal failed `TestScrubAWSKey`; runner timeout guard removal failed `TestOSRunnerRespectsTimeout`; raw JSON/JSONL output bypass failed `TestClaudeRunRemovesRawResultFromStdout` and `TestCodexRunRemovesAgentMessageTextFromStdout`; plain provider output bypass failed `TestClaudeRunRedactsPlainProviderStdout` and `TestCodexRunRedactsPlainProviderStdout`; review wrapper parsing bypass failed `TestClaudeReviewParsesVerdictFromJSONResult` and `TestCodexReviewParsesVerdictFromJSONLMessageText`.
- Files: 11 created.
- Residual risk: none.
- Follow-ups: none.
