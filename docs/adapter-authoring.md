# Adapter Authoring

PRD references: `FR-3.1` to `FR-3.5`, `FR-5.1`, `FR-5.3`, `FR-7.4`

## Contract

The runtime adapter boundary lives in [internal/adapter/adapter.go](../internal/adapter/adapter.go). Every adapter implements:

- `Name() string`
- `Role() adapter.Role`
- `Detect(context.Context) (adapter.Detection, error)`
- `Run(context.Context, adapter.Request) (adapter.Result, error)`
- `Review(context.Context, adapter.Request) (adapter.Verdict, error)`

`Detect` reports whether the CLI is installed, its version, and whether it can run headlessly. `Run` and `Review` must enforce non-interactive operation and return scrubbed artifacts only.

## Implementation Rules

- Put the adapter in `internal/adapter/<name>.go`.
- Keep network behavior inside the invoked CLI, never in `mivia-agent` itself.
- Reject non-headless configurations for orchestrable adapters.
- Do not persist raw prompts, raw model output, or provider payloads.
- Return structured metadata that can be written safely into `.ai/runs/`.

## FakeRunner Pattern

Tests use [internal/adapter/fake_runner.go](../internal/adapter/fake_runner.go) instead of real CLIs. The fake runner lets tests assert:

- command arguments
- environment shaping
- prompt scrubbing
- timeout and approval-mode enforcement
- returned artifacts and verdicts

Adapter tests should prove both the success path and the fail-closed path where headless execution is unavailable.

## Adding A CLI

1. Add `<name>.go` and `<name>_test.go` under `internal/adapter/`.
2. Implement `Detect`, `Run`, and `Review`.
3. Register the adapter in the CLI runtime list in [internal/cli/adapters.go](../internal/cli/adapters.go).
4. Add any generated adapter templates in `templates/adapters/<name>/`.
5. Update `docs/user-guide.md` and, if the adapter is project-facing, `docs/template-authoring.md`.

## Review Checklist

- `Detect` distinguishes installed from missing binaries cleanly.
- `Run` enforces non-interactive approval settings.
- `Review` emits structured verdicts with no raw model text.
- Guidance-only adapters never become `approved_for_run`.
- Mutation proof exists for the load-bearing reject path.
