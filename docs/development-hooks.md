# Development Hooks

Install the repo hooks once per clone:

```bash
scripts/install_git_hooks.sh
```

This sets `core.hooksPath=.githooks`, so Git runs the committed hooks in this repo.

## Pre-Commit

- `python3 scripts/verify_agent_config.py`
- `gofmt -w` on staged Go files, then re-stage those files
- `git diff --check --cached`
- Semgrep config validation
- `semgrep --config semgrep/agent-standards.yml --error --skip-unknown-extensions --metrics off` on staged files

## Pre-Push

- `python3 scripts/verify_agent_config.py`
- `git diff --check`
- Semgrep config validation
- full-repo Semgrep policy scan
- when `go.mod` exists: `gofmt -l`, `go test ./...`, `go vet ./...`, and `go build ./cmd/mivia-agent` once that command exists

## Policy Shape

Semgrep is used for repo-specific agent drift rules that are cheap to run locally:

- no wildcard or metacharacter-bearing `Bash(...)` tool permissions
- no panics or process exits from future `internal/` Go packages
- no direct network calls from future product code
- no raw prompt, provider payload, or model-output artifact writes
- no real Codex/Claude/OpenCode process execution in adapter tests

Sources: https://git-scm.com/docs/githooks, https://git-scm.com/docs/git-config, https://pkg.go.dev/cmd/gofmt, https://docs.semgrep.dev/extensions/pre-commit, https://docs.semgrep.dev/writing-rules/rule-syntax.
