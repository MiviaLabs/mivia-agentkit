# User Guide

This guide covers the current local CLI surface and the commands that are fully implemented today.

Related PRD requirements:
- Setup and update: `FR-1.1` to `FR-1.4`
- Validation and adapters: `FR-2.1` to `FR-3.2`
- Workflows and review: `FR-4.1` to `FR-5.3`
- Hooks and governance: `FR-7.1` to `FR-8.3`
- Portability: `NFR-1`

## Install

From source:

```bash
go install github.com/MiviaLabs/mivia-agentkit/cmd/mivia-agent@latest
```

From a checkout:

```bash
go run ./cmd/mivia-agent --help
```

Use `--repo` on commands to point at the target Git repository. If omitted, commands use the current directory unless the command says otherwise.

Example manifest and workflow files live in [config-examples.md](./config-examples.md).

## Command Matrix

| Command | PRD | Current behavior |
| --- | --- | --- |
| `init` | `FR-1.1` to `FR-1.3` | Installs canonical repo files and adapter-specific files. |
| `doctor` | `FR-2.1`, `FR-5.4`, `FR-10.5` | Validates setup and returns structured findings. |
| `audit` | `FR-2.3`, `FR-6.4` | Reports advisory quality gaps. |
| `preflight` | `FR-2.4`, `FR-7.1` | Writes a quality stamp for the current Git diff. |
| `adapters` | `FR-3.1`, `FR-3.2` | Detects adapters and whether they are approved for `run`. |
| `run` | `FR-4.1` to `FR-4.4` | Executes a bounded workflow from manifest or workflow file. |
| `review` | `FR-5.1` to `FR-5.3` | Runs a one-off consensus review for one artifact. |
| `hook` | `FR-7.1`, `FR-8.1` to `FR-8.3` | Enforces hook policy for Codex and Claude events. |
| `import` | `FR-9.1`, `FR-9.2` | Reads an existing setup and plans or writes `.ai/` migration files. |
| `update` | `FR-1.4` | Refreshes managed template regions in an initialized repo. |
| `version` | `NFR-1` | Prints the build version. |

Flags that exist but are not fully wired yet:

- `init --with-loop`
- `run --step`
- `run --input-artifact`
- `run --var`
- `preflight --pipeline-preflight`

Treat those as reserved surface. They are accepted by the CLI but do not materially change behavior yet.

## Initialize A Repo

PRD: `FR-1.1`, `FR-1.2`, `FR-1.3`, `FR-10.1` to `FR-10.3`, `FR-10.6`

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

## Validate With Doctor

PRD: `FR-2.1`, `FR-5.4`, `FR-10.5`

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

PRD: `FR-2.3`, `FR-6.4`

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

## Write A Quality Stamp

PRD: `FR-2.4`, `FR-7.1`

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

Current notes:

- `--not-run` is only accepted when no `--broad-verifier` was provided.
- High-risk changes require contract rows, focused verifiers, and mutation proofs.
- `--pipeline-preflight` is currently accepted but does not materially change validation behavior.

## Inspect Adapters

PRD: `FR-3.1`, `FR-3.2`

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

PRD: `FR-4.1` to `FR-4.4`, `FR-6.1` to `FR-6.3`

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
- Resolves runtime model and effort as `step override -> adapter default -> CLI default`.
- `run --dry-run --json` includes a per-step `runtime` list with resolved `adapter`, `model`, and `effort` values.
- Detects required orchestrable adapters.
- Executes the loop through the orchestrator.
- Writes run artifacts under `.ai/runs/`.

Current adapter runtime support:

- Codex passes `--model` and a one-off `model_reasoning_effort` override when configured.
- Claude passes `--model` and `--effort` when configured.
- Runtime effort values fail closed if they are valid globally but unsupported by the selected adapter. Codex supports `minimal`, `low`, `medium`, `high`, and `xhigh`; Claude supports `low`, `medium`, `high`, `xhigh`, and `max`.
- Antigravity runs through `agy -p` and rejects `model`, `effort`, and `params` because this repo has no documented Antigravity runtime mapping for those knobs.
- Crush may carry repo-owned `model` and `params` config, but it remains guidance-only and is never invoked by `run`.

Loop authoring details live in [loop-authoring.md](./loop-authoring.md).

## Run A One-Off Review

PRD: `FR-5.1` to `FR-5.3`

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

## Hook Entry Points

PRD: `FR-7.1`, `FR-8.1` to `FR-8.3`

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

PRD: `FR-9.1`, `FR-9.2`

Preview the migration plan:

```bash
go run ./cmd/mivia-agent import --repo /path/to/repo --json
```

Apply it:

```bash
go run ./cmd/mivia-agent import --repo /path/to/repo --write
```

Current import behavior:

- Reads legacy files such as `AGENTS.md`, `CLAUDE.md`, `GEMINI.md`, `.codex/`, `.claude/`, `.agents/skills/`, GitHub instruction files, and simple workflow-shaped files.
- Classifies reusable versus manual-migration findings.
- Creates canonical bootstrap files plus `.ai/imported/...` mapped files for reusable content.
- Preserves source files instead of deleting or rewriting them in place.
- Runs `doctor` after `--write`.

## Update Managed Files

PRD: `FR-1.4`, `NFR-1`

Preview differences:

```bash
go run ./cmd/mivia-agent update --repo /path/to/repo
```

Write updates:

```bash
go run ./cmd/mivia-agent update --repo /path/to/repo --write
```

Current update behavior:

- Compares rendered embedded templates against the repo.
- Updates managed blocks only for managed files.
- Preserves user text outside managed markers.
- Treats `mivia-agent.yaml` as a whole-file managed artifact.
- Reports conflicts instead of overwriting locally edited managed content unless `--force`.
- Runs `doctor` after `--write`.

## Recommended Local Flow

```bash
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
