# User Guide

This guide covers the current implemented CLI surface and the flags that actually matter today.

## Run From Source

From this checkout:

```bash
go run ./cmd/mivia-agent --help
```

Use `--repo` on commands to point at the target Git repository. If omitted, commands use the current directory unless the command says otherwise.

## Command Matrix

| Command | Current behavior |
| --- | --- |
| `init` | Installs canonical repo files and adapter-specific files. |
| `doctor` | Validates setup and returns structured findings. |
| `audit` | Reports advisory quality gaps. |
| `preflight` | Writes a quality stamp for the current Git diff. |
| `adapters` | Detects adapters and whether they are approved for `run`. |
| `run` | Executes a bounded workflow from manifest or workflow file. |
| `review` | Runs a one-off consensus review for one artifact. |
| `hook` | Enforces hook policy for Codex and Claude events. |
| `import` | Reads an existing setup and plans or writes `.ai/` migration files. |
| `update` | Refreshes managed template regions in an initialized repo. |
| `version` | Prints the build version. |

Flags that exist but are not fully wired yet:

- `init --with-loop`
- `run --step`
- `run --input-artifact`
- `run --var`
- `preflight --pipeline-preflight`

Treat those as reserved surface. They are accepted by the CLI but do not materially change behavior yet.

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
- `--adapter antigravity` targets Google Antigravity CLI (`agy`), not the retired consumer Gemini CLI.
- `.agents/skills.json`, including project skills and any readable global skills from `~/.agents/skills/`.

`init --write` is idempotent for the same inputs. Existing user-owned files are reported as conflicts and are not overwritten unless `--force` is passed. Managed files preserve text outside the `mivia-agent:managed` block.

Current `init` flags:

- `--repo <path>`: target repository.
- `--profile <starter|standard|strict>`: profile to render; default `standard`.
- `--adapter <name>` repeated: `codex`, `claude`, `copilot`, `antigravity`, `crush`.
- `--dry-run`: preview file actions.
- `--write`: write files.
- `--force`: overwrite conflicting user-owned files.
- `--json`: emit structured output.
- `--with-loop <name>` repeated: accepted, but currently reserved.

Example JSON preview:

```bash
go run ./cmd/mivia-agent init --repo /path/to/repo --profile strict \
  --adapter codex --adapter claude --dry-run --json
```

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

Flags:

- `--repo <path>`
- `--json`
- `--strict`

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

Flags:

- `--repo <path>`
- `--json`
- `--strict`

## Write A Quality Stamp

Run:

```bash
go run ./cmd/mivia-agent preflight --repo /path/to/repo \
  --contract-row hooks \
  --focused-verifier "go test ./internal/cli/... -count=1" \
  --broad-verifier "go test ./... -count=1" \
  --mutation-proof "drop protected-action guard fails"
```

JSON output:

```bash
go run ./cmd/mivia-agent preflight --repo /path/to/repo \
  --contract-row hooks \
  --focused-verifier "go test ./internal/cli/... -count=1" \
  --mutation-proof "drop protected-action guard fails" \
  --json
```

What `preflight` does now:

- Detects the Git root.
- Captures current `HEAD`, changed files, and diff hash.
- Validates proof inputs for the current change set.
- Writes `.git/mivia-agent-quality-stamp.json`.

Current flags:

- `--repo <path>`
- `--contract-row <name>` repeated
- `--focused-verifier <command>` repeated
- `--broad-verifier <command>` repeated
- `--mutation-proof <note>` repeated
- `--not-run <reason>` repeated
- `--pipeline-preflight`
- `--json`

Current notes:

- `--not-run` is only accepted when no `--broad-verifier` was provided.
- High-risk changes require contract rows, focused verifiers, and mutation proofs.
- `--pipeline-preflight` is currently accepted but does not materially change validation behavior.

## Inspect Adapters

Run:

```bash
go run ./cmd/mivia-agent adapters --repo /path/to/repo --json
```

The report includes:

- `name`
- `installed`
- `version`
- `headless`
- `role`
- `approved_for_run`

Current adapter expectations:

- `codex` and `claude` are orchestrable when installed and headless-capable.
- `antigravity` is orchestrable through `agy`.
- `crush` is guidance-only and never approved for `run`.
- `copilot` is guidance-only template surface, not a runtime adapter.

## Run A Workflow

Preview the plan:

```bash
go run ./cmd/mivia-agent run --repo /path/to/repo --workflow research --dry-run --json
```

Execute it:

```bash
go run ./cmd/mivia-agent run --repo /path/to/repo --workflow research
```

Current `run` behavior:

- Reads the workflow from `mivia-agent.yaml` or `.ai/workflows/<name>.yaml`.
- Rejects budget-bound loops.
- Detects required orchestrable adapters.
- Executes the loop through the orchestrator.
- Writes run artifacts under `.ai/runs/`.

Flags:

- `--repo <path>`
- `--workflow <name>`
- `--max-iterations <n>`
- `--dry-run`
- `--json`
- `--strict`
- `--step <id>` reserved
- `--input-artifact <path>` reserved
- `--var key=value` repeated, reserved

## Run A One-Off Review

Example:

```bash
go run ./cmd/mivia-agent review --repo /path/to/repo \
  --artifact internal/cli/root.go \
  --reviewers codex,claude \
  --mode majority \
  --min-reviewers 2 \
  --json
```

Current review behavior:

- Requires the artifact to exist.
- Uses the given reviewers, or manifest defaults if omitted.
- Runs one review request per reviewer.
- Applies the configured consensus policy.

Flags:

- `--repo <path>`
- `--artifact <path>`
- `--reviewers codex,claude`
- `--mode <majority|unanimous|weighted|first-pass>`
- `--min-reviewers <n>`
- `--weights codex=2,claude=1`
- `--tie-breaker <strict|manual|prefer:adapter>`
- `--json`

## Hook Entry Points

Codex example:

```bash
printf '{"tool":"bash","command":"git push"}' | \
  go run ./cmd/mivia-agent hook codex pre-tool-use --repo /path/to/repo
```

Claude example:

```bash
printf '{"tool":"bash","command":"git push"}' | \
  go run ./cmd/mivia-agent hook claude pre-tool-use --repo /path/to/repo
```

Current hook behavior:

- Supports `codex` and `claude`.
- Reads event payload from stdin.
- Denies protected actions when stamp or policy requirements are missing.
- Fails closed for malformed protected payloads.

## Import An Existing Setup

Preview the migration plan:

```bash
go run ./cmd/mivia-agent import --repo /path/to/repo --json
```

Apply it:

```bash
go run ./cmd/mivia-agent import --repo /path/to/repo --write
```

Force conflicting mapped writes:

```bash
go run ./cmd/mivia-agent import --repo /path/to/repo --write --force
```

Current import behavior:

- Reads legacy files such as `AGENTS.md`, `CLAUDE.md`, `GEMINI.md`, `.codex/`, `.claude/`, `.agents/skills/`, GitHub instruction files, and simple workflow-shaped files.
- Classifies reusable versus manual-migration findings.
- Creates canonical bootstrap files plus `.ai/imported/...` mapped files for reusable content.
- Preserves source files instead of deleting or rewriting them in place.
- Runs `doctor` after `--write`.

Flags:

- `--repo <path>`
- `--write`
- `--force`
- `--json`

## Update Managed Files

Preview differences:

```bash
go run ./cmd/mivia-agent update --repo /path/to/repo
```

Write updates:

```bash
go run ./cmd/mivia-agent update --repo /path/to/repo --write
```

Force conflicted updates:

```bash
go run ./cmd/mivia-agent update --repo /path/to/repo --write --force
```

Current update behavior:

- Compares rendered embedded templates against the repo.
- Updates managed blocks only for managed files.
- Preserves user text outside managed markers.
- Treats `mivia-agent.yaml` as a whole-file managed artifact.
- Reports conflicts instead of overwriting locally edited managed content unless `--force`.
- Runs `doctor` after `--write`.

Flags:

- `--repo <path>`
- `--write`
- `--force`
- `--json`

## Recommended Local Flow

```bash
go run ./cmd/mivia-agent init --repo /path/to/repo --profile standard \
  --adapter codex --adapter claude --adapter copilot --dry-run

go run ./cmd/mivia-agent init --repo /path/to/repo --profile standard \
  --adapter codex --adapter claude --adapter copilot --write

go run ./cmd/mivia-agent doctor --repo /path/to/repo --json
go run ./cmd/mivia-agent audit --repo /path/to/repo --json
go run ./cmd/mivia-agent preflight --repo /path/to/repo \
  --contract-row hooks \
  --focused-verifier "go test ./internal/cli/... -count=1" \
  --mutation-proof "drop protected-action guard fails" \
  --json
```

Fix `doctor` errors before treating the repo as configured. Treat `audit` warnings as cleanup work or run with `--strict` when the repository should enforce those gaps as failures.
