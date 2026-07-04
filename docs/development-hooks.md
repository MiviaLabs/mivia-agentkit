# Development Hooks

Install the repo hooks once per clone:

```bash
make install-hooks
```

This sets `core.hooksPath=.githooks`, so Git runs the committed hooks in this repo. The underlying installer remains `scripts/install_git_hooks.sh` so automation can call it directly.

## Make Targets

- `make verify` runs the full local quality gate.
- `make pre-commit` runs the committed pre-commit hook.
- `make pre-push` runs the committed pre-push hook.
- `make semgrep` runs the repo Semgrep policy scan.
- `make semgrep-test` runs Semgrep rule contract tests.
- `make hook-test` runs Git hook contract tests.
- `make go-check` runs Go format/test/vet/build checks when `go.mod` exists.

## Pre-Commit

- `python3 scripts/verify_agent_config.py`
- `gofmt -w` on staged Go files, then re-stage those files
- `git diff --check --cached`
- Semgrep config validation
- Semgrep rule contract tests
- Git hook contract tests
- `semgrep --config semgrep/agent-standards.yml --error --skip-unknown-extensions --metrics off` on staged files
- writes a fresh `.git/mivia-agent-precommit-summary` record for `prepare-commit-msg`
- records the exact `agent config verification passed` result in the commit-message `Quality:` line

## Prepare-Commit-Msg

- appends one short `Quality:` line to regular commit messages when pre-commit passed
- skips merge and squash messages
- refuses stale summaries by comparing the current staged tree to the pre-commit tree

## Commit-Msg

- validates regular commit subjects as `type(scope): imperative subject`
- allowed types and scopes are centralized in `.ai/policy/commit-message.json`
- expand commit types or scopes only by updating `.ai/policy/commit-message.json`, then running `make hook-test`
- rejects subjects longer than 72 characters or ending with a period
- appends `commit message passed` to the `Quality:` line after validation succeeds
- allows Git-generated merge/revert subjects and `fixup!`/`squash!` autosquash subjects

## Pre-Push

- `python3 scripts/verify_agent_config.py`
- `git diff --check`
- Semgrep config validation
- Semgrep rule contract tests
- Git hook contract tests
- full-repo Semgrep policy scan
- when `go.mod` exists: `gofmt -l`, `go test ./...`, `go vet ./...`, and `go build ./cmd/mivia-agent` once that command exists

Pre-push intentionally keeps the full Semgrep scan. Pre-commit only proves the staged snapshot for one commit; pre-push proves the branch state before it leaves the machine.

## Policy Shape

Semgrep is used for repo-specific agent drift rules that are cheap to run locally:

- no wildcard or metacharacter-bearing `Bash(...)` tool permissions
- no Semgrep suppressions or unresolved drift markers in guarded code/instructions
- no panics or process exits from future `internal/` Go packages
- no shell execution, `syscall.Exec`, direct network calls, or world-writable file modes
- no direct network calls from future product code
- no raw prompt, provider payload, or model-output artifact writes
- no real Codex/Claude/OpenCode process execution in adapter tests
- no temp directories outside `t.TempDir()` or sleeps in tests

When a repo standard is added, changed, or repeatedly violated, agents must update `semgrep/agent-standards.yml` when the standard can be checked statically, update `scripts/test_semgrep_rules.py`, and run `make semgrep-test`.

Sources: https://git-scm.com/docs/githooks, https://git-scm.com/docs/git-config, https://pkg.go.dev/cmd/gofmt, https://docs.semgrep.dev/extensions/pre-commit, https://docs.semgrep.dev/writing-rules/rule-syntax, https://docs.semgrep.dev/writing-rules/testing-rules, https://docs.semgrep.dev/cli-reference.
