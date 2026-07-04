# Mivia AgentKit

Mivia AgentKit is a greenfield Go CLI project for managing a local agent-control surface across Codex, Claude Code, GitHub Copilot, and future agent adapters.

The repo is currently in docs and agent-infrastructure setup. Go product code should start only from the scoped workstream tasks under `docs/plans/human/`.

## Quick Start

Install prerequisites from `docs/setup/development-environment.md`.

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

- [Development environment](docs/setup/development-environment.md) - local prerequisites and Ubuntu setup
- [Development hooks](docs/development-hooks.md) - hook behavior and policy shape
- [Agent hooks](docs/agent-hooks.md) - agent hook surfaces, triggers, policies, and audit-loop behavior
- [Agent planning](docs/agent-planning.md) - DAG planning skill, machine plan contract, and implementation hooks
- [Product requirements](docs/prd/0001-mivia-agentkit.md) - product requirements
- [Workstream roadmap](docs/plans/human/00-overview.md) - human workstream roadmap
