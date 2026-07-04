# Mivia AgentKit

<img src="docs/logo-mivia-readme.webp" alt="Mivia logo" width="160">

Mivia AgentKit is a greenfield Go CLI project for managing a local agent-control surface across Codex, Claude Code, GitHub Copilot, and future agent adapters.

## Quick Start

Install prerequisites from `docs/setup/development-environment.md`.

Run the CLI from this checkout:

```bash
go run ./cmd/mivia-agent --help
```

Preview the agent-control files that `init` would add to a target Git repo:

```bash
go run ./cmd/mivia-agent init --repo /path/to/repo --profile standard \
  --adapter codex --adapter claude --adapter copilot --dry-run
```

Write the files after reviewing the dry run:

```bash
go run ./cmd/mivia-agent init --repo /path/to/repo --profile standard \
  --adapter codex --adapter claude --adapter copilot --write
```

Validate the generated setup:

```bash
go run ./cmd/mivia-agent doctor --repo /path/to/repo --json
```

Review advisory quality gaps:

```bash
go run ./cmd/mivia-agent audit --repo /path/to/repo --json
```

`init` creates the `.ai/` control surface, root adapter files, and selected tool-adapter files. Re-running the same command is idempotent: generated files with identical content are skipped. Existing user-owned files with different content are reported as conflicts and are not overwritten unless `--force` is passed. Files with `mivia-agent` managed-block markers are updated only inside the managed block.

Useful `init` flags:

- `--repo`: target repository; defaults to the current directory.
- `--profile`: `starter`, `standard`, or `strict`; defaults to `standard`.
- `--adapter`: repeat for each adapter to enable: `codex`, `claude`, `copilot`, `gemini`, or `crush`.
- `--dry-run`: print planned actions without writing.
- `--write`: write files.
- `--json`: emit a structured report with created, skipped, and conflicted files.
- `--force`: overwrite user-owned files that do not have managed-block markers.

`doctor` is the read-only validation gate. It checks the manifest, `.ai/` index, adapter files, hook wiring, skill frontmatter, managed-block markers, loop bounds, consensus satisfiability, governance provider, and global rule conflicts. It exits non-zero for error-severity findings; `--strict` also treats warnings as failures.

`audit` is the read-only quality-gap report. It flags duplicated policy, missing control checks, missing verifier or contract surfaces, unsafe MCP wildcard config, managed-file edits outside generated blocks, weak strict-profile consensus, protect-bound loops without review, and global rule conflicts. By default it is advisory and exits zero; `--strict` promotes warnings to failure.

Install hooks and run the local gate:

```bash
make install-hooks
make verify
```

See available targets:

```bash
make help
```

## Docs

- [User guide](docs/user-guide.md) - current implemented CLI workflow for `init`, `doctor`, and `audit`
- [Development environment](docs/setup/development-environment.md) - local prerequisites and Ubuntu setup
- [Development hooks](docs/development-hooks.md) - hook behavior and policy shape
- [Agent hooks](docs/agent-hooks.md) - agent hook surfaces, triggers, policies, and audit-loop behavior
- [Agent planning](docs/agent-planning.md) - DAG planning skill, machine plan contract, and implementation hooks
- [Product requirements](docs/prd/0001-mivia-agentkit.md) - product requirements
- [Workstream roadmap](docs/plans/agentkit-implementation-roadmap/00-overview.md) - AgentKit implementation roadmap

![Mivia AgentKit](docs/mivia-agentkit.jpeg)
