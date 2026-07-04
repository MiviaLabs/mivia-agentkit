# Operating Doctrine

## Canonical Source Order

- `AGENTS.md` is the repo-level source of truth for agent behavior.
- `.ai/` is the canonical project control surface for rules and skills.
- Tool files are adapters. If an adapter conflicts with `AGENTS.md` or `.ai/`, follow `AGENTS.md` and fix the adapter.

## Scope Control

- Before implementation, read `docs/plans/_conventions.md` and the relevant `docs/plans/ws-XX/tasks.md`.
- Do not implement product code unless a specific workstream task is in scope.
- Stay inside the named workstream, task, branch, or file boundary unless the user expands scope.
- Preserve existing docs and user changes unless the task explicitly requires editing them.

## Documentation-First Work

- Every code change must update or explicitly reference the relevant workstream task file.
- If implementation reveals a task split, update the plan before writing the second production file.
- Completion reports belong at the end of the relevant `tasks.md`, using the repo convention exactly.

## Idempotency

- Any writer, generator, init command, importer, or update command must be rerunnable with no diff for the same inputs.
- Every writer needs an idempotency test.
- Generated file order must be deterministic: sort map keys, filenames, hook names, and registry entries before writing.
