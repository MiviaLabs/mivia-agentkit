# Mivia AgentKit Agent Instructions

AGENTS.md is the canonical repo-level instruction file. Tool-specific files in `CLAUDE.md`, `.codex/`, `.claude/`, `.github/`, and `.agents/` must point back here and to `.ai/`; they must not introduce conflicting policy.

Sources: https://agents.md/, https://developers.openai.com/codex/guides/agents-md, https://code.claude.com/docs/en/claude-directory, https://docs.github.com/en/copilot/how-tos/copilot-on-github/customize-copilot/add-custom-instructions/add-repository-instructions.

## Project Overview

Mivia AgentKit is a greenfield Go CLI named `mivia-agent`. It will be Cobra-based and will install, validate, and orchestrate a canonical agent-control surface for multi-CLI agent workflows. The product goal is not another hosted service: it is a single local binary that configures `.ai/`, wraps agent CLIs behind adapters, runs bounded workflows, and gates protected actions with deterministic local checks.

Do not write Go product code unless a specific `docs/plans/agentkit-implementation-roadmap/ws-XX/tasks.md` task is in scope. This repo currently starts from docs and agent infrastructure.

Sources: https://agents.md/, https://go.dev/doc/modules/layout. Repo sources: `docs/prd/0001-mivia-agentkit.md`, `docs/plans/agentkit-implementation-roadmap/00-overview.md`.

## Repository Structure

- `docs/` contains the PRD, product proposal, workstream roadmap, and task files. Treat it as the source for product behavior.
- `.ai/` is the project-level canonical control surface for rules, skills, workflows, quality contracts, and future run artifacts.
- `.agents/` contains generic cross-agent registries and hook declarations that delegate back to `.ai/` policy.
- `.claude/`, `.codex/`, and `.github/` are tool adapters that reference `AGENTS.md` and `.ai/`.
- Future Go code belongs in `cmd/mivia-agent/` and `internal/` following the package map in `docs/plans/agentkit-implementation-roadmap/_conventions.md`.
- Runtime output belongs under `.ai/runs/` and must stay gitignored.

Sources: https://www.dot-agents.com/, https://code.claude.com/docs/en/claude-directory, https://developers.openai.com/codex/guides/agents-md, https://go.dev/doc/modules/layout. Repo source: `docs/plans/agentkit-implementation-roadmap/_conventions.md`.

## Operating Doctrine

- Read `docs/plans/agentkit-implementation-roadmap/_conventions.md` before any implementation workstream.
- Work in dependency order from `docs/plans/agentkit-implementation-roadmap/00-overview.md`; do not start a workstream whose dependencies are not green.
- Each task is one production file plus its test file. If a task needs more, split the task before implementing.
- Every code change must either update or explicitly reference the relevant `docs/plans/agentkit-implementation-roadmap/ws-XX/tasks.md`.
- Generated or updated files must be idempotent: rerunning the same writer with the same inputs produces no diff.
- Agents must report what was verified and what remains unverified before claiming completion.

Sources: https://agents.md/, https://developers.openai.com/codex/learn/best-practices. Repo source: `docs/plans/agentkit-implementation-roadmap/_conventions.md`.

## Go Standards

- Use lowercase, single-word package names; keep command entrypoints under `cmd/mivia-agent/` and shared implementation under `internal/`.
- Every `.go` file starts with the package doc header required by `docs/plans/agentkit-implementation-roadmap/_conventions.md`, including WS and PRD references.
- Every exported package, type, function, var, and const needs a doc comment. Package comments start with `Package <name>`.
- Return errors instead of panicking in library code. Use `%w` for wrapping and `errors.Is` / `errors.As` for checks across error chains.
- Keep initialisms consistent: `URL`, `HTTP`, `ID`, and `API`, not `Url`, `Http`, `Id`, or `Api`.
- Use `//go:embed` only at package scope; patterns must be module-local and deterministic.
- Do not add network calls to `mivia-agent` itself. Fake runners are valid for unit tests, but shipped command and adapter behavior must also be covered by real subprocess or built-binary integration tests wherever the product surface is implemented.

Sources: https://go.dev/doc/effective_go, https://go.dev/doc/modules/layout, https://go.dev/doc/comment, https://go.dev/blog/go1.13-errors, https://go.dev/wiki/CodeReviewComments, https://pkg.go.dev/embed. Repo source: `docs/plans/agentkit-implementation-roadmap/_conventions.md`.

## Testing Standards

- Write tests before or alongside production code. Tests are not deferred.
- Use table-driven tests for multi-case behavior, with named subtests and failure messages that include got/want context.
- Use `t.TempDir()` for any test that writes files; never write fixtures into the repo tree during tests.
- Do not mock the thing under test. If Git behavior is the risk, use a real temp Git repo. If hook output shape is the risk, assert the real shape.
- Fake runners, stub executables, and in-process fakes may support unit coverage, but they are not sufficient closure for implemented commands or approved adapters; add at least one real subprocess or built-binary integration path for each shipped surface.
- Test helpers must call `t.Helper()` and must not hide failures behind booleans or swallowed errors.
- Every guard, rejection path, fail-closed path, and idempotent writer needs a mutation proof recorded in the workstream completion report.

Sources: https://go.dev/wiki/TableDrivenTests, https://go.dev/doc/comment. Repo source: `docs/plans/agentkit-implementation-roadmap/_conventions.md`.

## Security And Privacy

- No raw prompts, raw model output, provider payloads, plausible secrets, tokens, `.env` contents, or credential files may be persisted.
- Project config in `.ai/` wins over global config in `~/.agents/`; global config is read as defaults only by the future product.
- Hooks must fail closed for malformed payloads that request protected actions once the hook engine exists.
- Protected actions are commit, push, PR, deploy, release, and live smoke. Future hooks must require a fresh quality stamp and policy decision before allowing them.
- Do not add CI, hooks, docs, samples, or fixtures that require network access by default.

Sources: https://developers.openai.com/codex/hooks, https://code.claude.com/docs/en/hooks-guide, https://www.dot-agents.com/. Repo sources: `docs/prd/0001-mivia-agentkit.md`, `docs/plans/agentkit-implementation-roadmap/_conventions.md`.

## Development Commands

Use these once Go code exists:

```bash
go test ./...
go vet ./...
go build ./cmd/mivia-agent
```

For focused workstreams, run the package-specific commands listed in that workstream's `Verification` block before broader checks.

For changes to the agent configuration surface, run:

```bash
python3 scripts/verify_agent_config.py
```

Install repo-managed Git hooks once per clone:

```bash
make install-hooks
```

The committed hooks run `gofmt`, Semgrep rule tests, and Semgrep policy checks before commit, then agent-config validation, full Semgrep, and Go test/vet/build checks before push. The Go checks no-op until `go.mod` exists. `make verify` runs the full local gate.

Agent tool hooks in `.agents/`, `.claude/`, and `.codex/` run a shared guard that rejects verification-bypass attempts and tells the model to fix the failed validation or report the blocker after one focused repair attempt.

When adding or changing a durable repo standard, forbidden pattern, hook policy, security invariant, or repeated agent failure mode, update `semgrep/agent-standards.yml` if the rule can be checked statically and add coverage in `scripts/test_semgrep_rules.py`.

Sources: https://agents.md/, https://go.dev/doc/modules/layout, https://git-scm.com/docs/githooks, https://git-scm.com/docs/git-config, https://pkg.go.dev/cmd/gofmt, https://docs.semgrep.dev/extensions/pre-commit, https://docs.semgrep.dev/writing-rules/testing-rules, https://docs.semgrep.dev/cli-reference. Repo sources: `docs/plans/agentkit-implementation-roadmap/_conventions.md`, `docs/development-hooks.md`.

## Git Workflow

- Use branch names like `codex/<short-scope>` for agent-created branches unless the user asks otherwise.
- Commit messages must follow `type(scope): imperative subject`; allowed types, scopes, and subject length live in `.ai/policy/commit-message.json`.
- Do not force-push `dev`.
- Do not commit product code without green relevant tests and mutation proofs for guards.
- If existing unrelated changes are present, do not revert them. Work around them or ask only when they block the task.

Sources: https://agents.md/, https://developers.openai.com/codex/guides/agents-md. Repo sources: `docs/plans/agentkit-implementation-roadmap/_conventions.md`, `.ai/policy/commit-message.json`.

## Tool Adapters

- `CLAUDE.md` is a thin Claude Code adapter; it must keep Claude-specific notes only.
- `.agents/hooks.json` declares the generic hook guard surface for cross-agent tooling.
- `.claude/settings.json` configures project permissions and delegates Claude Code hooks through the shared guard before falling through to the future `mivia-agent` hook implementation.
- `.codex/hooks.json` configures Codex hooks for `UserPromptSubmit`, `PreToolUse`, `PermissionRequest`, and `Stop`, all delegated through the shared guard before falling through to the future `mivia-agent` hook implementation.
- `.github/copilot-instructions.md` and `.github/instructions/*.instructions.md` are Copilot adapters that point back to this file.
- Skills use `SKILL.md` frontmatter with at least `name`, `description`, and `triggers`. The canonical project skills live under `.ai/skills/`; `.claude/skills/` adapts them for Claude Code.

Sources: https://developers.openai.com/codex/hooks, https://developers.openai.com/codex/skills, https://code.claude.com/docs/en/settings, https://code.claude.com/docs/en/hooks-guide, https://code.claude.com/docs/en/skills, https://docs.github.com/en/copilot/how-tos/copilot-on-github/customize-copilot/add-custom-instructions/add-repository-instructions.

## Documentation Workflow

- Before implementing any WS task, read the task file, `docs/plans/agentkit-implementation-roadmap/_conventions.md`, the PRD sections named by the task, and any upstream files the task depends on.
- When a task completes, append its completion report to that task file and update `docs/plans/agentkit-implementation-roadmap/00-overview.md` only when the workstream is complete.
- Do not free-form `tasks.md`; preserve the required task, verification, mutation proof, and completion formats.

Sources: https://agents.md/, https://developers.openai.com/codex/learn/best-practices. Repo sources: `docs/plans/agentkit-implementation-roadmap/_conventions.md`, `docs/plans/agentkit-implementation-roadmap/00-overview.md`.
