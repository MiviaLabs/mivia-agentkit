# Model And Effort Config Implementation Plan

Date: 2026-07-05
Scope: add first-class model and intelligence-effort configuration to `mivia-agent` workflows and adapter config, with Codex and Claude runtime pass-through now, and Crush config support split from Crush orchestration support.

## Contract

`mivia-agent` should let users declare model-selection and effort-level intent in repo config instead of relying only on each CLI's external global config.

Planned contract:

- Global defaults may live under adapter config in `mivia-agent.yaml`.
- Workflow steps may override those defaults per producer or review step.
- The orchestrator passes resolved settings into adapters through `adapter.Request`.
- Adapters fail closed when a workflow asks for unsupported runtime knobs.
- Codex and Claude get real runtime pass-through.
- Crush gets project config/template support for provider/model and documented config fields, but remains excluded from orchestrated `run` until Crush has a documented headless run contract.

Proposed config shape:

```yaml
adapters:
  codex:
    enabled: true
    role: orchestrable
    model: gpt-5.5
    effort: high
  claude:
    enabled: true
    role: orchestrable
    model: sonnet
    effort: medium
  crush:
    enabled: true
    role: guidance
    model: openai/gpt-5.5
    params:
      provider: openai
      base_url: https://api.openai.com/v1

loops:
  research:
    bound: iterations
    max_iterations: 3
    steps:
      - id: produce
        producer: codex
        model: gpt-5.5
        effort: high
      - id: review
        reviewers: [claude]
        model: sonnet
        effort: low
```

Proposed runtime rule:

- Step-level `model` / `effort` win over adapter defaults.
- Adapter defaults win over external CLI defaults.
- Empty value means "use the CLI's default."
- Adapter-specific support is enforced before execution; unsupported runtime knobs fail before any subprocess starts.

## Discovery Evidence

Current repo state:

- Manifest and loop schema do not currently expose `model` or `effort` fields in [internal/config/manifest.go](../internal/config/manifest.go) and [internal/config/loop.go](../internal/config/loop.go).
- Orchestrator requests currently only pass `Prompt`, `Workdir`, `Approval`, `ArtifactOut`, `Timeout`, and `MaxTurns` in [internal/orchestrator/engine.go](../internal/orchestrator/engine.go).
- Adapter request types do not yet carry model/effort in [internal/adapter/adapter.go](../internal/adapter/adapter.go).
- Codex and Claude adapters currently parse returned model metadata but do not accept requested model settings in [internal/adapter/codex.go](../internal/adapter/codex.go) and [internal/adapter/claude.go](../internal/adapter/claude.go).
- Crush is currently guidance-only and explicitly blocked from orchestrated `Run`/`Review` in [internal/adapter/crush.go](../internal/adapter/crush.go).

Current external docs:

- Codex documents `--model` for CLI selection and config-level `model_reasoning_effort`, with one-off `-c/--config` overrides: [Models](https://developers.openai.com/codex/models), [Config basics](https://developers.openai.com/codex/config-basic), [Configuration reference](https://developers.openai.com/codex/config-reference), [Advanced config](https://developers.openai.com/codex/config-advanced).
- Claude Code documents `--model`, `--effort`, `-p`, and model-capability handling for effort/thinking: [CLI reference](https://code.claude.com/docs/en/cli-reference), [Run Claude Code programmatically](https://code.claude.com/docs/en/headless), [Model configuration](https://code.claude.com/docs/en/model-config), [Commands](https://code.claude.com/docs/en/commands), [Environment variables](https://code.claude.com/docs/en/env-vars).
- Crush documents provider/model configuration through `crush.json` and provider model lists, but this repo still lacks a documented non-TUI run surface for orchestrated execution: [Crush README](https://github.com/charmbracelet/crush), especially config and model/provider sections, plus the existing headless-gap issue already referenced in repo code: [issue #1862](https://github.com/charmbracelet/crush/issues/1862).

Inference from those docs:

- Codex and Claude implementation is a runtime pass-through problem.
- Crush support should be split:
  - config/template support now
  - orchestrated runtime support later, after a stable headless contract exists

## Workstreams

### WS-A — Config Schema And Resolution

Files to read:

- [internal/config/manifest.go](../internal/config/manifest.go)
- [internal/config/loop.go](../internal/config/loop.go)
- [internal/config/manifest_test.go](../internal/config/manifest_test.go)
- [internal/config/loop_test.go](../internal/config/loop_test.go)

Planned edits:

- Add optional `Model string`, `Effort string`, and `Params map[string]string` to `config.AdapterConfig`.
- Add optional `Model string` and `Effort string` to `config.Step`.
- Validate allowed effort enum centrally.
- Preserve current fail-closed unknown-field behavior.

RED tests:

- `internal/config/manifest_test.go::TestManifestParsesAdapterModelDefaults`
- `internal/config/loop_test.go::TestLoopParsesStepModelOverrides`
- `internal/config/manifest_test.go::TestManifestRejectsUnknownEffort`

Mutation proof:

- Remove effort validation; `TestManifestRejectsUnknownEffort` must fail.

### WS-B — Adapter Request Contract

Files to read:

- [internal/adapter/adapter.go](../internal/adapter/adapter.go)
- [internal/adapter/adapter_test.go](../internal/adapter/adapter_test.go)

Planned edits:

- Extend `adapter.Request` with `Model string`, `Effort string`, and `Params map[string]string`.
- Validate supported effort values at request boundary only after config normalization decides a value exists.

RED tests:

- `internal/adapter/adapter_test.go::TestRequestAcceptsModelAndEffort`
- `internal/adapter/adapter_test.go::TestRequestRejectsUnknownEffort`

Mutation proof:

- Allow unknown effort through request validation; `TestRequestRejectsUnknownEffort` must fail.

### WS-C — Runtime Resolution In Orchestrator

Files to read:

- [internal/orchestrator/engine.go](../internal/orchestrator/engine.go)
- [internal/orchestrator/engine_test.go](../internal/orchestrator/engine_test.go)
- [internal/orchestrator/loop.go](../internal/orchestrator/loop.go)

Planned edits:

- Resolve effective `model` and `effort` from:
  - step override
  - adapter default
  - empty fallback
- Pass the resolved values into both producer and reviewer adapter requests.

RED tests:

- `internal/orchestrator/engine_test.go::TestExecuteProducerStepPassesResolvedModelAndEffort`
- `internal/orchestrator/engine_test.go::TestExecuteReviewStepPassesResolvedModelAndEffort`
- `internal/orchestrator/engine_test.go::TestStepOverrideWinsOverAdapterDefault`

Mutation proof:

- Ignore step override and always use adapter defaults; `TestStepOverrideWinsOverAdapterDefault` must fail.

### WS-D — Codex Adapter Pass-Through

Files to read:

- [internal/adapter/codex.go](../internal/adapter/codex.go)
- [internal/adapter/codex_test.go](../internal/adapter/codex_test.go)

Planned edits:

- Pass `--model <id>` when `Request.Model` is set.
- Pass a one-off config override for reasoning effort when `Request.Effort` is set, using the documented Codex config override surface instead of mutating global config.
- Keep existing approval, sandbox, JSON, and scrubbed metadata behavior.

RED tests:

- `internal/adapter/codex_test.go::TestCodexRunPassesModelFlag`
- `internal/adapter/codex_test.go::TestCodexRunPassesReasoningEffortOverride`

Mutation proof:

- Drop the model or effort argv/config override; the matching Codex test must fail.

### WS-E — Claude Adapter Pass-Through

Files to read:

- [internal/adapter/claude.go](../internal/adapter/claude.go)
- [internal/adapter/claude_test.go](../internal/adapter/claude_test.go)

Planned edits:

- Pass `--model <id>` when `Request.Model` is set.
- Pass `--effort <level>` when `Request.Effort` is set.
- Preserve existing non-interactive and JSON behavior.

RED tests:

- `internal/adapter/claude_test.go::TestClaudeRunPassesModelFlag`
- `internal/adapter/claude_test.go::TestClaudeRunPassesEffortFlag`

Mutation proof:

- Drop `--model` or `--effort`; the matching Claude test must fail.

### WS-F — Crush Config And Template Support

Files to read:

- [internal/adapter/crush.go](../internal/adapter/crush.go)
- [internal/config/manifest.go](../internal/config/manifest.go)
- [internal/templates/source/adapters/crush/README.md.tmpl](../internal/templates/source/adapters/crush/README.md.tmpl)
- [docs/config-examples.md](../config-examples.md)
- [docs/user-guide.md](../user-guide.md)

Planned edits:

- Keep Crush as `guidance` and not approved for `run`.
- Document and render repo-owned Crush config guidance for:
  - provider/model defaults
  - optional provider params carried through adapter config `params`
- Do not pretend these params are an orchestrated runtime feature yet.

RED tests:

- `internal/config/manifest_test.go::TestManifestParsesCrushParams`
- `internal/templates/templates_test.go::TestCrushTemplateIncludesModelConfigGuidance`

Mutation proof:

- Drop Crush param rendering/guidance; `TestCrushTemplateIncludesModelConfigGuidance` must fail.

### WS-G — CLI Dry-Run And User Docs

Files to read:

- [internal/cli/run.go](../internal/cli/run.go)
- [internal/cli/run_test.go](../internal/cli/run_test.go)
- [docs/config-examples.md](../config-examples.md)
- [docs/user-guide.md](../user-guide.md)
- [docs/loop-authoring.md](../loop-authoring.md)

Planned edits:

- Include `model` and `effort` in `run --dry-run --json` output so users can verify resolved runtime intent before invoking adapters.
- Add config examples for Codex, Claude, and Crush.

RED tests:

- `internal/cli/run_test.go::TestRunDryRunPrintsModelAndEffort`

Mutation proof:

- Omit resolved `model`/`effort` from dry-run output; `TestRunDryRunPrintsModelAndEffort` must fail.

## Test Map

Happy path:

- Manifest parses adapter defaults plus per-step overrides.
- Orchestrator resolves step override over adapter default.
- Codex adapter emits model and effort overrides into the subprocess argv/config.
- Claude adapter emits model and effort flags into argv.
- `run --dry-run --json` shows the resolved knobs a user asked for.

Negative path:

- Unknown effort value rejected by config parsing and request validation.
- Guidance-only Crush cannot become a producer/reviewer just because it has `model` or `params`.
- Empty model/effort values do not inject bogus flags.

Stale/reused state path:

- A review step after a producer step receives the same resolved per-step settings on every iteration.

Downstream handoff shape:

- Fake-runner tests must assert exact argv/config fragments for Codex and Claude.

No-op path:

- If model/effort are absent, existing adapter argv remains unchanged.

Conflict path:

- Step-level override must beat adapter default every time.

## Implementation Steps

1. Extend config schema and validation.
2. Extend `adapter.Request`.
3. Thread resolved model/effort through orchestrator producer/review calls.
4. Add Codex adapter pass-through with tests.
5. Add Claude adapter pass-through with tests.
6. Add Crush config/template guidance support without changing its guidance-only runtime role.
7. Update `run --dry-run` output and docs/examples.

## Verification

Focused:

```bash
go test ./internal/config/... ./internal/adapter/... ./internal/orchestrator/... ./internal/cli/... ./internal/templates/... -count=1
go vet ./internal/config/... ./internal/adapter/... ./internal/orchestrator/... ./internal/cli/... ./internal/templates/...
```

Behavior-specific:

```bash
go test ./internal/adapter/... -run 'TestCodexRunPassesModelFlag|TestCodexRunPassesReasoningEffortOverride|TestClaudeRunPassesModelFlag|TestClaudeRunPassesEffortFlag' -count=1
go test ./internal/orchestrator/... -run 'TestExecuteProducerStepPassesResolvedModelAndEffort|TestExecuteReviewStepPassesResolvedModelAndEffort|TestStepOverrideWinsOverAdapterDefault' -count=1
go test ./internal/cli/... -run 'TestRunDryRunPrintsModelAndEffort' -count=1
git diff --check
```

## Mutation Proof

- Unknown effort must fail config/request validation.
- Step override must beat adapter default.
- Codex must lose its model/effort tests if either override is removed.
- Claude must lose its model/effort tests if either flag is removed.
- Dry-run output must fail if `model`/`effort` are not surfaced.
- Crush guidance rendering must fail if provider/model params disappear from generated guidance.

## Residual Risks

- Crush runtime orchestration remains blocked until Crush publishes a stable non-TUI/headless run contract that this repo can verify and test offline.
- Codex effort pass-through depends on the documented config override path rather than a dedicated `codex exec --effort` flag, so the implementation should prefer the documented dedicated flag if OpenAI adds one later.
