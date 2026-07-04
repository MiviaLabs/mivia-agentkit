# Mivia AgentKit

Mivia AgentKit is a greenfield Go CLI project for managing a local agent-control surface across Codex, Claude Code, GitHub Copilot, and future agent adapters.

The repo is currently in docs and agent-infrastructure setup. Go product code should start only from the scoped workstream tasks under `docs/plans/`.

## Quick Start

Install local Git hooks once per clone:

```bash
make install-hooks
```

Run the full local quality gate:

```bash
make verify
```

See available targets:

```bash
make help
```

## Make Targets

- `make install-hooks` - configure Git to use the committed hooks in `.githooks/`.
- `make verify` - run agent-config validation, Semgrep config validation, Semgrep rule tests, full Semgrep policy scan, and Go checks when `go.mod` exists.
- `make pre-commit` - run the committed pre-commit hook.
- `make pre-push` - run the committed pre-push hook.
- `make semgrep` - run the repo Semgrep policy scan.
- `make semgrep-test` - run contract tests for repo-local Semgrep rules.
- `make go-check` - run `gofmt` check, `go test ./...`, `go vet ./...`, and `go build ./cmd/mivia-agent` when Go code exists.

## Hook Behavior

Pre-commit runs:

- agent-config validation
- `gofmt -w` on staged Go files
- staged whitespace checks
- Semgrep config validation
- Semgrep rule contract tests
- Semgrep policy scan on staged files

Pre-push runs:

- agent-config validation
- full whitespace checks
- Semgrep config validation
- Semgrep rule contract tests
- full-repo Semgrep policy scan
- Go format/test/vet/build checks once `go.mod` exists

Go checks intentionally skip while the repo has no `go.mod`.

## Semgrep Policy

Repo-local policy lives in `semgrep/agent-standards.yml`; its contract tests live in `scripts/test_semgrep_rules.py`.

When adding or changing a durable repo standard, forbidden pattern, hook policy, security invariant, or repeated agent failure mode, update the Semgrep rules if the rule can be checked statically. Add one bad fixture and one clean fixture to the rule test script, then run:

```bash
make semgrep-test
make verify
```

Do not bypass policy with Semgrep suppression comments. Fix the code, fix the rule, or document a reviewed policy exception outside the scanned code path.
