# User Guide

This guide covers the implemented local CLI surface: `init`, `doctor`, and `audit`.

## Run From Source

From this checkout:

```bash
go run ./cmd/mivia-agent --help
```

Use `--repo` on commands to point at the target Git repository. If omitted, commands use the current directory.

## Initialize A Repo

Preview generated files first:

```bash
go run ./cmd/mivia-agent init --repo /path/to/repo --profile standard \
  --adapter codex --adapter claude --adapter copilot --dry-run
```

Write after reviewing the plan:

```bash
go run ./cmd/mivia-agent init --repo /path/to/repo --profile standard \
  --adapter codex --adapter claude --adapter copilot --write
```

`init` creates:

- `.ai/` canonical control-surface files.
- Root adapter files such as `AGENTS.md` and `CLAUDE.md`.
- Tool adapter files under `.codex/`, `.claude/`, and `.github/` for selected adapters.
- `.agents/skills.json`, including project skills and any readable global skills from `~/.agents/skills/`.

`init --write` is idempotent for the same inputs. Existing user-owned files are reported as conflicts and are not overwritten unless `--force` is passed. Managed files preserve text outside the `mivia-agent:managed` block.

## Validate With Doctor

Run:

```bash
go run ./cmd/mivia-agent doctor --repo /path/to/repo
```

For CI or scripts:

```bash
go run ./cmd/mivia-agent doctor --repo /path/to/repo --json
```

`doctor` is read-only. It validates:

- `mivia-agent.yaml` exists and parses.
- `.ai/INDEX.md` exists.
- Root and tool adapter files point back to the canonical `.ai` surface.
- Enabled adapter files exist, including `.codex/AGENTS.md` and `.codex/hooks.json` for Codex.
- Hook configs invoke `mivia-agent hook`.
- Skill files have required frontmatter.
- Managed-block markers are balanced.
- Loop definitions are bounded and reference known orchestrable adapters.
- Review steps have satisfiable `min_reviewers`.
- Governance provider is known.
- Global rule files that conflict with project rules are reported as warnings.

Exit codes:

- `0`: no error-severity findings.
- `1`: at least one error-severity finding.
- `2`: warning-only findings when `--strict` is set.

## Audit Quality Gaps

Run:

```bash
go run ./cmd/mivia-agent audit --repo /path/to/repo
```

For structured output:

```bash
go run ./cmd/mivia-agent audit --repo /path/to/repo --json
```

`audit` is read-only and advisory by default. It reports quality gaps such as:

- Missing canonical `.ai/`.
- Duplicated long policy text in adapter files.
- Missing control workflow that runs `mivia-agent doctor --json`.
- Missing contract matrix or empty verifier matrix.
- Unsafe MCP wildcard config.
- Generated managed files edited outside managed blocks.
- Protect-bound loops without a review step.
- Strict-profile loops using weak consensus.
- `min_reviewers` exceeding enabled orchestrable adapters.
- `noop` governance under strict profile.
- Global rule conflicts with project rules.

By default, `audit` exits `0` even when it reports warnings. Use `--strict` to promote warning-only reports to exit code `2`.

## Recommended Local Flow

```bash
go run ./cmd/mivia-agent init --repo /path/to/repo --profile standard \
  --adapter codex --adapter claude --adapter copilot --dry-run

go run ./cmd/mivia-agent init --repo /path/to/repo --profile standard \
  --adapter codex --adapter claude --adapter copilot --write

go run ./cmd/mivia-agent doctor --repo /path/to/repo --json
go run ./cmd/mivia-agent audit --repo /path/to/repo --json
```

Fix `doctor` errors before treating the repo as configured. Treat `audit` warnings as cleanup work or run with `--strict` when the repository should enforce those gaps as failures.
