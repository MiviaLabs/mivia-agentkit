# Go Standards

## Layout

- Keep command entrypoints under `cmd/mivia-agent/`.
- Keep reusable implementation under `internal/` according to `docs/plans/_conventions.md`.
- Do not create public packages until a task requires an external API.

## Errors

- Return errors from library code; do not `panic` for expected failures.
- Wrap errors with `%w` when callers need to test them.
- Use `errors.Is` for sentinel checks and `errors.As` for typed errors.
- Error strings are lowercase sentence fragments without trailing punctuation unless a proper noun requires capitalization.

## Naming

- Package names are lowercase, short, and single-word.
- Use `URL`, `HTTP`, `ID`, `API`, `JSON`, and `YAML` consistently in exported names.
- Keep file names lower-case with underscores only when needed for clarity.

## Comments And Headers

- Every `.go` file starts with the package doc header required by `docs/plans/_conventions.md`.
- Every exported identifier has a doc comment that starts with the identifier name.
- Comments explain contracts, edge cases, and invariants; they do not narrate obvious assignments.

## Embedding

- Use `//go:embed` only for static templates or fixtures required by the binary.
- Embed patterns must be relative to the package directory and must not include `.git`, symlinks, parent traversals, or generated runtime artifacts.
- Add tests that fail when an embedded template is missing or malformed.
