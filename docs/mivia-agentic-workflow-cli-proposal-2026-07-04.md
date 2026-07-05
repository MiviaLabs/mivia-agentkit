# Mivia AgentKit Greenfield Implementation Plan

Status: standalone implementation plan (revision 2 — orchestrator + cross-CLI routing + consensus review + configurable loops).

Audience: agents and engineers starting a brand-new repository from zero.

Copy target: copy this file into the new repository as `PROJECT_PLAN.md` or `docs/plans/mivia-agentkit-implementation-plan.md`. The product-level PRD lives at `docs/prd/0001-mivia-agentkit.md`.

Project name: **Mivia AgentKit**.

Binary name: `mivia-agent`.

Repository name: `github.com/MiviaLabs/mivia-agentkit`.

## What changed in this revision

Earlier drafts framed `mivia-agent` as a manifest + deterministic-gate generator: it wrote instruction/skill/hook files and blocked protected actions via local stamps, but it did not execute agent work. This revision promotes it to a **multi-CLI orchestrator** while keeping every original safety property:

1. **Cross-CLI routing** — `mivia-agent run` shells out headlessly to Codex, Claude Code, Antigravity CLI, and Crush, in sequence or in parallel, passing artifacts between steps. The system is **adapter-based**: every CLI is behind an `Adapter` interface, so adding or swapping a tool is one adapter, not a rewrite.
2. **Consensus review/verification** — any step can fan out the same artifact to multiple CLI reviewers in parallel; a configurable voting/tie-breaker policy decides pass/fail/iterate. This is the verification primitive between CLIs.
3. **Configurable loops** — `mivia-agent.yaml` declares named loops (research, bug-audit, fix-review, release-audit) with bounded iterations by default and an opt-in budget mode. Loops are safe to run in CI and in hooks.
4. **Adapter set** — Codex, Claude Code, Antigravity CLI, and Crush (Charmbracelet). The "pi agent" reference from earlier discussion is removed.
5. **Governance backbone** — policy enforcement, structured decisions, and tamper-evident audit logging are delegated to the Microsoft Agent Governance Toolkit (AGT) Go SDK, wrapped behind an internal `policy` interface with a no-op fallback so the binary still ships standalone.

Everything else — `.ai/` canonical model, `~/.agents/` global config layer, thin root/vendor adapters, quality stamp under `.git/`, idempotent init, no network in MVP, no live connectors by default — is unchanged.

## Objective

Build a standalone Mivia-branded CLI that configures any Git repository for high-rigor agentic software workflows **and orchestrates those workflows across multiple agent CLIs**.

The CLI installs and validates a generic agent-control surface (instructions, skills, hooks, quality gates, contract matrices, loop definitions, review policies) and a thin, swappable adapter layer for agent tools. It reads a global user config from `~/.agents/` (the emerging universal agent directory) and layers it under the project-level `.ai/` surface. It executes loops by invoking adapters headlessly, routes artifacts between them, and lets deterministic local gates decide whether risky work can finish.

Core promise:

> Agent guidance is advisory; agent execution is orchestrated; deterministic local gates decide whether risky work can finish.

## Product Shape

`mivia-agent` is a local CLI, not a hosted service.

Primary commands:

- `mivia-agent init`: create the agent workflow files in a target repository.
- `mivia-agent doctor`: validate installed files, generated markers, adapter wiring, loop definitions, and local policy.
- `mivia-agent audit`: report missing or weak agent-workflow controls.
- `mivia-agent preflight`: validate current diff and write a local quality stamp.
- `mivia-agent run`: execute a named workflow/loop by orchestrating adapters headlessly.
- `mivia-agent review`: run a one-off consensus review of an artifact across selected adapters.
- `mivia-agent adapters`: list, describe, and validate installed adapters (which CLIs are present, headless-capable, and approved).
- `mivia-agent hook codex <event>`: Codex hook entrypoint.
- `mivia-agent hook claude <event>`: Claude Code hook entrypoint.
- `mivia-agent import`: inspect an existing repo setup and propose migration.
- `mivia-agent update`: update managed templates without overwriting user-owned content.

The generated target-repo model:

- `.ai/` is canonical (project-level).
- `~/.agents/` provides global user-level config that is layered under `.ai/`.
- Root and vendor files are thin adapters.
- Hooks call `mivia-agent`, not repo-local ad hoc scripts.
- CI calls `mivia-agent doctor --json`, then optionally `mivia-agent run`.
- No live connector or credential setup is enabled by default.

## Config Hierarchy (`~/.agents/` + `.ai/`)

The agent config follows a two-layer model:

| Layer | Location | Scope | Ownership | When read |
|---|---|---|---|---|
| **Global** | `~/.agents/` | Per-user, all repos | User (never committed) | Every command; layered first (lowest priority) |
| **Project** | `.ai/` + root adapters | Per-repo | Team (committed to repo) | Every command; layered second (highest priority) |

### Global layer (`~/.agents/`)

The `~/.agents/` directory is the emerging universal config layer for AI coding agents (see [dot-agents.com](https://dot-agents.com/) and [AGENTS.md Issue #91](https://github.com/agentsmd/agents.md/issues/91)). Different tools currently fragment global config across `~/.claude/`, `~/.codex/`, `.cursor/`, etc. `mivia-agent` reads from `~/.agents/` and unifies them.

What `mivia-agent` reads from `~/.agents/`:

- `~/.agents/rules/` — global rules (operating doctrine, security/privacy, quality). These are layered under the project-level `.ai/rules/` — project rules win on conflict.
- `~/.agents/skills/` — global skills available in every repo (e.g. personal coding preferences, organization-wide audit skills). Project-level skills of the same name override.
- `~/.agents/mivia-agent.yaml` — mivia-agent-specific global preferences:
  ```yaml
  version: 1
  defaults:
    profile: standard          # default profile when --profile is omitted
    adapters:                  # default adapter set when --adapter is omitted
      codex:   { enabled: true,  role: orchestrable }
      claude:  { enabled: true,  role: orchestrable }
      copilot: { enabled: false, role: guidance }
  ``` 

The global layer is **never written by `init`**. It is user-managed. `mivia-agent` reads it but does not modify it. If `~/.agents/` does not exist, `mivia-agent` silently proceeds with project-level config only — no errors, no warnings.

### Layering rules

1. **Project wins on conflict.** If `.ai/rules/00-operating-doctrine.md` exists, it overrides `~/.agents/rules/00-operating-doctrine.md`. If only the global version exists, it is used.
2. **Skills merge by name.** If `~/.agents/skills/deep-bug-audit/SKILL.md` exists and `.ai/skills/deep-bug-audit/SKILL.md` does not, the global skill is available. If both exist, the project version wins.
3. **Manifest fields merge.** Global `mivia-agent.yaml` `defaults` provide fallback values for fields not set in the project's `mivia-agent.yaml`. Explicit project values always override.
4. **No secret leakage.** The global layer is subject to the same path policy and secret-scrubbing rules as the project layer.
5. **`doctor` validates both.** `doctor` checks for conflicts between global and project config (e.g. a global rule that contradicts a project rule) and reports them as warnings.

## Distribution Model

The `mivia-agent` binary is **fully self-contained**. All templates, loop definitions, review-policy defaults, and skill markdown are embedded in the binary at build time via `//go:embed`. Nothing external is required for any command to work.

### Where templates live

The template source files live in the **agentkit source repo** at `templates/` (see "Repository Architecture"). This directory is source-controlled, committed alongside the Go code, and contains:

- `templates/core/` — the canonical `.ai/` surface (INDEX.md, rules, skills, contracts, review policies).
- `templates/adapters/<cli>/` — per-CLI adapter templates (AGENTS.md, CLAUDE.md, hook configs, Copilot instructions, etc.).
- `templates/workflows/` — loop definitions (research-loop.yaml, bug-audit-loop.yaml).
- `templates/prompts/` — default prompt templates for producer and reviewer steps.
- `templates/ci/` — CI workflow templates.

These are **not** `.ai/` files. They are the raw material that `init` renders (with variable substitution) and writes into a target repo's `.ai/` and root-adapter locations. The `templates/` directory is created during WS0 (bootstrap) alongside the Go module, and populated during WS2 (templates + init).

### What this means concretely

- **The binary ships alone.** A user installs via `brew install mivialabs/tap/mivia-agent`, downloads a release binary, or `go install`; no companion data directory, no `.ai/` bundle, no config file, no separate `templates/` directory on disk. The binary just works.
- **`.ai/` does not exist until `init` creates it.** The `.ai/` canonical surface (rules, skills, workflows, contracts, review policies) is **generated** by `init` into the target repository. It does not come from the user's machine or from the internet — it comes from the templates embedded in the binary, which in turn came from the agentkit source repo's `templates/` directory.
- **`~/.agents/` is optional and user-managed.** `mivia-agent` reads `~/.agents/rules/`, `~/.agents/skills/`, and `~/.agents/mivia-agent.yaml` if they exist, layering them under `.ai/`. It never writes to `~/.agents/`. If absent, commands work with project-level config only.
- **`mivia-agent` itself does not have an `.ai/`.** The binary is a tool that generates `.ai/` for other repos. It does not require or create `.ai/` for its own build/runtime. (Developers working on the agentkit repo may run `init` on it for dogfooding, but the binary never needs it.)
- **`update` refreshes from the binary, not the internet.** When a user gets a newer `mivia-agent` binary, `update` compares the new embedded templates against the repo's existing managed blocks — no network, no download.
- **Adapters invoke CLIs that the user already has installed.** `mivia-agent run --workflow research` requires `codex` and/or `claude` on PATH — but `mivia-agent` does not install them or verify their versions beyond the `adapters` command's `Detect` probe. The binary is the orchestrator; the CLIs are the worker processes.

### Installation flow

```
1. User installs binary (brew / release download / go install)
2. User optionally sets up ~/.agents/ with global rules/skills/preferences
3. User cd's into their repo
4. User runs: mivia-agent init --profile standard --adapter codex --adapter claude --write
5. .ai/ is generated in that repo from the binary's embedded templates
6. Global ~/.agents/ config is layered under .ai/ (user's rules/skills/preferences applied as defaults)
7. User's existing CLIs (codex, claude, etc.) are detected via adapters command
8. Everything works — no internet, no external data, no companion files
```

## Non-Goals

- Do not depend on any existing application codebase.
- Do not require another service, database, workflow engine, MCP server, or cloud account. (AGT is an embedded library, not a service.)
- Do not assume a target repo uses Go, Node, Python, Docker, GitHub Actions, or any specific test framework.
- Do not install Jira, Confluence, Slack, GitHub, Google, or other live connectors by default.
- Do not automatically push branches, create PRs, deploy, or alter remote settings.
- Do not claim deterministic enforcement for tools that only support advisory instructions.
- Do not store secrets, raw prompts, raw model output, private source dumps, or provider payloads.
- Do not require Dagger, Temporal, Kubernetes, or any container/runtime engine to run loops.
- Do not implement an "expert"/unbounded loop profile in MVP. Unbounded budget loops are an opt-in profile added after the bounded engine is stable.

## External Facts To Re-Verify Before Implementation

Agent vendor behavior changes. At the start of implementation, re-check the current official docs for these surfaces and update adapters/tests if needed. Each adapter must cite the exact doc URL + version it targets in its package doc.

Headless invocation surfaces (the part most likely to drift):

- Codex CLI — `codex exec`, `--output-last-message`, approval/sandbox modes, JSON output:
  - https://github.com/openai/codex
  - https://developers.openai.com/codex/noninteractive
  - https://developers.openai.com/codex/agents-md
  - https://developers.openai.com/codex/skills
  - https://developers.openai.com/codex/hooks
  - https://developers.openai.com/codex/mcp
  - https://developers.openai.com/codex/plugins/build
- Claude Code — `-p`/`--print`, `--output-format json|stream-json`, `--permission-mode`, `--max-turns`, allowed/disallowed tools:
  - https://code.claude.com/docs/en/cli
  - https://code.claude.com/docs/en/sdk
  - https://code.claude.com/docs/en/hooks
  - https://code.claude.com/docs/en/permissions
  - https://code.claude.com/docs/en/skills
  - https://code.claude.com/docs/en/settings
  - https://code.claude.com/docs/en/sub-agents
- Antigravity CLI — `agy`, one-shot prompt mode (`agy -p`), install/update, permissions, rules, skills, hooks:
  - https://developers.googleblog.com/an-important-update-transitioning-gemini-cli-to-antigravity-cli/
  - https://antigravity.google/docs/cli/using
  - https://antigravity.google/docs/cli/reference
  - https://antigravity.google/docs/cli/best-practices
- Crush (Charmbracelet) — non-interactive mode, approval/permission handling, config (`crush.json`), multi-model config. **Re-verify whether Crush supports a true headless/non-TUI mode; if not, mark the adapter interactive-only and exclude it from CI loops.**
  - https://github.com/charmbracelet/crush
- GitHub Copilot — repo/path instructions, custom agents, firewall, MCP governance:
  - https://docs.github.com/copilot/customizing-copilot/adding-custom-instructions-for-github-copilot
  - https://docs.github.com/en/copilot/how-tos/copilot-cli/customize-copilot/add-custom-instructions
  - https://docs.github.com/en/copilot/how-tos/use-copilot-agents/coding-agent/customize-the-agent-firewall
  - https://docs.github.com/en/copilot/concepts/context/mcp
- MCP:
  - https://modelcontextprotocol.io/docs/getting-started/intro
  - https://modelcontextprotocol.io/specification/2025-06-18/server/tools
- Governance backbone (re-verify the Go SDK import path and API shape before wiring):
  - https://github.com/microsoft/agent-governance-toolkit
  - Go SDK path (verify): `github.com/microsoft/agent-governance-toolkit/agent-governance-golang`
- Go libraries we depend on (re-verify current module paths and APIs):
  - Cobra: https://github.com/spf13/cobra
  - go-cmd (child process streaming + exit codes): https://github.com/go-cmd/go-cmd
  - oklog/run (goroutine fan-out + graceful shutdown): https://github.com/oklog/run

Implementation must cite any changed external behavior in the PR or handoff.

## Contract

Caller:

- Developer, CI job, automation script, or agent session running in a local Git repository.

Callee:

- `mivia-agent` CLI.

Inputs:

- Target repository path.
- Strictness profile: `starter`, `standard`, or `strict`. (Loop profile `expert`/unbounded is post-MVP.)
- Selected adapters: `codex`, `claude`, `copilot`, `antigravity`, `crush`. Of these, the orchestrating adapters (those `run` can invoke headlessly) are `codex`, `claude`, `antigravity`, `crush`; `copilot` is guidance-only.
- User-supplied or auto-detected command matrix: format, lint, typecheck, unit, integration, build, smoke.
- Workflow/loop definitions in `mivia-agent.yaml` (named loops with steps, routing, review policy, bounds).
- Current Git state: HEAD, changed files, staged files, untracked files.
- Optional existing agent files to import.

Outputs:

- Generated workflow files inside the target repository.
- `mivia-agent.yaml` manifest.
- Quality stamp at `.git/mivia-agent-quality-stamp.json`.
- Run artifacts under `.ai/runs/<run-id>/` (plans, reviews, decisions, traces). Never raw secrets or full provider payloads.
- Concise text reports.
- Machine-readable JSON reports.
- Exit codes suitable for hooks and CI.

Allowed side effects:

- Local filesystem writes inside the target repo when explicitly requested.
- Local Git reads and, during `run`, local Git-safe scratch writes under `.ai/runs/`.
- Local stamp and run-artifact writes under `.git/` and `.ai/`.
- Local subprocess execution of configured agent CLIs during `run`/`review`, with approval modes set to non-interactive and bounded turns/timeouts.
- Optional local CI workflow file generation.

Forbidden side effects in MVP:

- Network calls during `init`, `doctor`, `audit`, `preflight`, or hook execution. (Agent CLIs make their own network calls during `run`/`review`; mivia-agent itself does not.)
- Remote Git writes.
- PR creation.
- Credential creation.
- Secret persistence.
- Live external connector calls initiated by mivia-agent.

Idempotency:

- `init --write` followed by `init --write` with the same options must produce no diff.
- `update --write` may modify only managed template blocks.
- Hooks must not mutate target files.
- `preflight` may only write the stamp.
- `run` is replayable from a fixed manifest + Git state; re-running the same step with the same inputs and adapter versions produces a deterministic routing/decision trace (model outputs themselves are non-deterministic and are treated as evidence, not as state).

Safety:

- Never overwrite user-owned files silently.
- Never follow symlinks outside the target repo for writes.
- Never emit raw secrets or raw source dumps in hook or run output.
- Fail closed for malformed hook payloads that request protected actions.
- Every adapter invocation runs with an explicit timeout, a turn budget, and a non-interactive approval mode. If an adapter cannot be made non-interactive, it is excluded from `run`.
- Every protected action (commit/push/PR/deploy/release/live-smoke) inside a loop is gated by a fresh stamp and a passing policy decision.

## Generated Target Repo Files

Default `standard` profile:

```text
AGENTS.md
CLAUDE.md
mivia-agent.yaml
.ai/INDEX.md
.ai/rules/00-operating-doctrine.md
.ai/rules/01-output-budget.md
.ai/rules/10-security-privacy.md
.ai/rules/20-agent-quality.md
.ai/skills/airtight-feature-delivery/SKILL.md
.ai/skills/test-coverage-audit/SKILL.md
.ai/skills/deep-bug-audit/SKILL.md
.ai/skills/adversarial-test-review/SKILL.md
.ai/workflows/research-loop.yaml
.ai/workflows/bug-audit-loop.yaml
.ai/quality/contracts/project-runtime.yaml
.ai/quality/review-policies/default.yaml
.agents/skills.json        # skill registry (for tools that read it; references .ai/skills/ + ~/.agents/skills/)
.codex/hooks.json
.claude/settings.json
.claude/skills/airtight-feature-delivery/SKILL.md
.claude/skills/test-coverage-audit/SKILL.md
.claude/skills/deep-bug-audit/SKILL.md
.claude/skills/adversarial-test-review/SKILL.md
.github/copilot-instructions.md
.github/instructions/agent-quality.instructions.md
.github/workflows/agent-control.yml
```

Optional:

```text
GEMINI.md
.crush/README.md                      # adapter shim + config note (Crush, if headless-capable)
.github/agents/mivia-quality.agent.md
.codex/config.toml
```

## Manifest

`mivia-agent.yaml`:

```yaml
version: 1
profile: standard          # starter | standard | strict  (expert = post-MVP, opt-in unbounded)
template_version: 0.1.0

project:
  name: ""
  language: auto
  default_branch: auto

adapters:
  codex:   { enabled: true,  role: orchestrable }   # can be invoked headlessly by `run`
  claude:  { enabled: true,  role: orchestrable }
  antigravity:  { enabled: false, role: orchestrable }
  crush:   { enabled: false, role: orchestrable }   # only if headless mode verified
  copilot: { enabled: true,  role: guidance }       # instructions only, never invoked by `run`

# Default routing policy applied when a loop step does not override it.
routing:
  default_producer: claude
  default_reviewers: [codex, claude]
  consensus:
    mode: majority          # majority | unanimous | weighted | first-pass
    weights: { codex: 1.0, claude: 1.0, antigravity: 1.0, crush: 1.0 }
    tie_breaker: strict     # strict (fail on tie) | manual | prefer:<adapter>
    min_reviewers: 2
  on_review_fail: iterate   # iterate (route back to producer with notes) | fail | proceed
  max_iterations: 3         # default bound for any loop that does not declare one

loops:
  research:
    description: "Produce a grounded research brief, then cross-review."
    bound: iterations       # iterations | budget  (budget = post-MVP)
    max_iterations: 3
    steps:
      - id: brief
        producer: claude
        artifact: brief.md
      - id: review
        reviewers: [codex, antigravity]
        consensus: { mode: majority, min_reviewers: 2 }
        on_fail: iterate    # send review notes back to `brief`
    exit_when: { gate: review-pass }

  bug-audit:
    description: "Audit for bugs, then consensus review, then iterate."
    bound: iterations
    max_iterations: 5
    steps:
      - id: audit
        producer: codex
        artifact: audit.md
      - id: review
        reviewers: [claude, codex]
        consensus: { mode: majority, min_reviewers: 2, tie_breaker: strict }
        on_fail: iterate
    exit_when: { gate: review-pass }
    on_exhausted: fail      # fail | warn | proceed

  fix-review:
    description: "Apply a fix, then require reviewer consensus before protect."
    bound: iterations
    max_iterations: 3
    steps:
      - id: fix
        producer: claude
        artifact: diff
      - id: review
        reviewers: [codex, crush]
        consensus: { mode: unanimous, min_reviewers: 2 }
        on_fail: iterate
    exit_when: { gate: review-pass }

commands:
  format: []
  lint: []
  typecheck: []
  unit: []
  integration: []
  build: []
  smoke: []

protected_actions:
  commit: true
  push: true
  pull_request: true
  deploy: true
  release: true
  live_smoke: true

quality:
  require_contract_rows_for: [hooks, ci, scripts, workflows, runner, deploy, auth, security]
  require_mutation_proof: true
  stamp_file: .git/mivia-agent-quality-stamp.json
  runs_dir: .ai/runs

paths:
  generated_markers: true
  allow_overwrite: false
  forbidden:
    - .env
    - .env.*
    - secrets/**
    - "**/*private*key*"

governance:
  provider: agt             # agt | noop
  audit_log: .ai/audit.jsonl
  policy_decisions: true    # intercept protected actions via policy provider

mcp:
  enabled: false
  servers: []

# Global config layer — mivia-agent reads ~/.agents/ but never writes it.
# If ~/.agents/mivia-agent.yaml exists, its defaults are layered under this
# manifest (explicit values here always win). ~/.agents/rules/ and
# ~/.agents/skills/ are layered under .ai/rules/ and .ai/skills/.
global:
  layer: ~/.agents
  merge: project_wins   # global provides defaults; explicit project values override
```

## Loops, Routing, And Review

This is the heart of the orchestrator. Three primitives compose every workflow.

### Step

A step is a single adapter invocation that produces an artifact. A step has:

- `id`, `producer` (adapter name) or `reviewers` (list of adapter names),
- `artifact` (path or well-known token like `diff`),
- optional `approval_mode`, `max_turns`, `timeout`,
- optional prompt template path under `.ai/`.

### Routing

Routing decides which adapter runs next and how artifacts flow. Default routing lives at `routing:` in the manifest; any step can override it. Routing supports:

- **Sequential handoff**: step A's artifact becomes step B's input.
- **Parallel fan-out**: one artifact dispatched to N reviewers concurrently.
- **Conditional edges**: route based on a gate outcome (`review-pass`, `review-fail`, `gate:<name>`).

Routing is implemented as a small in-process DAG evaluator. There is deliberately no external workflow engine in MVP.

### Review (consensus)

A review step dispatches the same artifact to multiple reviewer adapters in parallel and applies a consensus policy:

- `mode`:
  - `majority` — pass if >50% of reviewers pass (respecting `min_reviewers`).
  - `unanimous` — pass only if all reviewers pass.
  - `weighted` — pass if `sum(weights of passers) >= threshold`.
  - `first-pass` — pass as soon as any reviewer passes (fast, weak; opt-in).
- `tie_breaker`: `strict` (fail on tie), `manual` (pause for a human), or `prefer:<adapter>`.
- `on_fail`: `iterate` (route review notes back to the producing step), `fail` (end the loop as failed), or `proceed` (continue with a warning recorded).

Each reviewer returns a structured verdict `{ adapter, pass: bool, severity, notes, evidence_ref }`. Verdicts are written to the run trace and (if governance is enabled) recorded as policy evidence.

### Loop bounds

- `bound: iterations` (default): the loop runs at most `max_iterations` times. `exit_when.gate` short-circuits on pass. `on_exhausted` decides what happens if the bound is hit without passing: `fail` (non-zero exit), `warn` (success with warning), `proceed` (success, record note).
- `bound: budget` (**post-MVP**, `expert` profile): the loop runs until a `budget` of tokens/minutes/cost is exhausted. Not in MVP. The MVP engine must reject `bound: budget` with a clear error.

### Built-in loops shipped as templates

- `research-loop.yaml` — produce a brief, consensus-review it, iterate on failure.
- `bug-audit-loop.yaml` — produce an audit, consensus-review, iterate, fail on exhaustion.

Users add loops by dropping a YAML file into `.ai/workflows/` and referencing it from `mivia-agent.yaml`, or by passing `--workflow <name>` to `run`.

## Adapter System

The orchestrator never calls a CLI directly. It calls an `Adapter`.

```go
// internal/adapter/adapter.go (sketch)
type Adapter interface {
    Name() string                                          // "codex" | "claude" | "antigravity" | "crush"
    Role() Role                                            // orchestrable | guidance
    Detect(ctx) (Presence, error)                          // is the binary installed + headless-capable?
    Run(ctx, Request) (Result, error)                      // headless invocation
    Review(ctx, ReviewRequest) (Verdict, error)            // structured review of an artifact
}

type Request struct {
    Prompt      string        // rendered prompt
    Workdir     string        // repo root
    Approval    ApprovalMode  // non-interactive mode enforced
    MaxTurns    int
    Timeout     time.Duration
    ArtifactOut string        // path the adapter should write its artifact to
}

type Result struct {
    ExitCode    int
    Stdout      []byte        // truncated, secret-scrubbed
    Stderr      []byte
    Artifact    string        // path actually written
    Turns       int
    ProviderMeta map[string]any  // model id, tokens (no prompts/outputs)
}
```

Adapter responsibilities:

- Encapsulate the exact headless invocation for one CLI (flags, approval mode, output format, exit-code mapping).
- Refuse to run if a non-interactive approval mode cannot be set (returns `Presence{ HeadlessCapable: false }`).
- Scrub secrets and never persist raw prompts or raw model output; only structured metadata and the declared artifact path are returned.
- Map the CLI's exit codes to `Result.ExitCode` and a human-readable status.

`mivia-agent adapters` lists each adapter's presence, headless capability, configured approval mode, and whether it is approved for `run`. Adapters that are present but not headless-capable are flagged and excluded from orchestration; they may still receive guidance/instruction files via `init`. Crush is approved only when local detection confirms `crush run` noninteractive support.

## Governance Backbone (AGT)

Policy enforcement and audit logging are delegated to the Microsoft Agent Governance Toolkit, wrapped behind an internal interface so the provider is swappable and the binary still works without it.

```go
// internal/policy/policy.go (sketch)
type Provider interface {
    Decide(ctx, Action) (Decision, error)   // allowed + evidence, called before protected actions and before each loop step
    Record(ctx, Event) error                // append to tamper-evidant audit log
}

type Decision struct {
    Allowed  bool
    Reason   string
    Evidence map[string]any
}
```

- `governance.provider: agt` wires the AGT Go SDK as the provider. Every protected action and every loop step produces a `Decision`.
- `governance.provider: noop` is the default fallback: it allows all actions but still records them to `.ai/audit.jsonl`. This keeps the binary standalone and dependency-optional for `init`/`doctor`/`preflight`.
- AGT integration is gated behind a Go build tag or a lazy import so that users who only want the file-generation surface don't need the dependency at runtime.

Mapping of original plan concepts onto AGT:

- "deterministic local gates decide" → AGT `Decide()` before protected actions.
- "contract matrix / require rows" → AGT policy inputs derived from `.ai/quality/contracts/*.yaml`.
- "quality stamp required before protect" → stamp freshness is one input to `Decide()`.
- "no raw prompts/outputs stored" → AGT audit stores `Decision` evidence, not provider payloads.

## Profiles

### Starter

Use for light adoption.

- Generate `.ai/`, `AGENTS.md`, `CLAUDE.md`, and selected instruction adapters.
- Generate skills, command matrix, and the built-in loop templates (disabled by default).
- No blocking hooks by default. Governance provider: `noop`.
- `doctor` reports warnings.

### Standard

Recommended default.

- Generate `.ai/`, adapters, skills, Codex hooks, Claude hooks, Copilot instructions, CI check, loop templates, and review policies.
- Require quality stamp before protected actions.
- Require contract rows for high-risk surfaces.
- Require mutation proof for high-risk guards.
- Enable consensus review for shipped loops. Governance provider: `noop` by default, `agt` opt-in.
- `doctor` fails on broken generated wiring, on a loop that references an unknown adapter, or on a loop with no bound.

### Strict

Use for automation-heavy repos.

- Everything in `standard`.
- Block deploy/release/live-smoke without full preflight.
- Require command matrix to have at least one verification command.
- Require explicit not-run reasons for missing broad verifiers.
- Block final handoffs with missing audit/review outcome summaries.
- Require `consensus.mode` of `majority` or `unanimous` for any loop that ends in a protected action.
- Governance provider: `agt` required (doctor fails if AGT is unavailable).

### Expert (post-MVP)

- Everything in `strict`, plus `bound: budget` loops.
- Not in MVP. The MVP engine rejects `expert` and `bound: budget`.

## Repository Architecture

Create this from zero:

```text
mivia-agentkit/
  go.mod
  README.md
  LICENSE
  PROJECT_PLAN.md
  cmd/mivia-agent/main.go
  internal/
    cli/
    config/
    globalconfig/      # ~/.agents/ reading + layering under project config
    detect/
    render/
    templates/
    doctor/
    audit/
    preflight/
    hooks/
    gitstate/
    pathpolicy/
    report/
    adapter/           # Adapter interface + codex/claude/antigravity/crush impls
      adapter.go
      codex.go
      claude.go
      antigravity.go
      crush.go
    orchestrator/      # DAG eval, fan-out, loop bounds, stamp gates
    consensus/         # voting + tie-breaker policies
    policy/            # Provider interface + agt provider + noop provider
    runstore/          # .ai/runs/<id>/ artifact + trace storage
  templates/               # SOURCE-CONTROLLED template files. These are the canonical
    core/                  #   source that gets embedded into the binary at build time
      INDEX.md             #   via //go:embed. They are NOT .ai/ files; they are the
      rules/*.md           #   raw material that `init` renders into a target repo's
      skills/*/SKILL.md    #   .ai/ and root-adapter files. This directory lives in the
      quality/contracts/   #   agentkit source repo and is committed alongside the code.
      quality/review-policies/
    adapters/
      codex/               # Templates that render into target-repo adapter files
      claude/              # (AGENTS.md, CLAUDE.md, .codex/hooks.json, etc.)
      copilot/
      antigravity/
      crush/
    workflows/             # Loop definitions (research-loop.yaml, bug-audit-loop.yaml)
      review-loop.yaml     #   that `init` copies into .ai/workflows/
      bug-audit-loop.yaml
    review-policies/       # Default consensus policies copied into .ai/quality/
    prompts/               # Default prompt templates for producer/reviewer steps
    ci/github-actions/     # agent-control.yml template for target repos
  testdata/repos/
  docs/
    adr/
    prd/
    user-guide.md
    template-authoring.md  # How to write/edit templates in this directory
    adapter-authoring.md
    loop-authoring.md
```

Use Go and Cobra, with deliberately minimal dependencies:

- `cobra`, `yaml.v3` — CLI and manifest parsing.
- `go-cmd/go-cmd` — child-process execution with live streaming and reliable exit codes.
- `oklog/run` — concurrent fan-out of reviewers with graceful shutdown.
- AGT Go SDK — optional, lazy, behind the `policy` interface.
- Build the orchestrator DAG, consensus evaluator, and loop engine in-house. They are small and domain-specific (bounded iterations, parallel verdicts, stamp gates); pulling a durable-workflow engine would add a runtime the binary does not need.

## Command Specification

### `mivia-agent init`

Purpose: install agent workflow files.

Flags:

- `--repo <path>` default `.`
- `--profile starter|standard|strict`
- `--adapter <name>` repeated (`codex`|`claude`|`copilot`|`antigravity`|`crush`)
- `--with-loop <name>` repeated (enable a shipped loop template)
- `--dry-run`
- `--write`
- `--force`
- `--json`

Behavior:

1. Validate target path.
2. Detect Git root.
3. Detect language/tooling signals.
4. Build command matrix suggestions.
5. Render templates (including loop templates and review policies).
6. Compare generated output to existing files.
7. Refuse unsafe overwrites.
8. Write only with `--write`.
9. Run doctor after write.

### `mivia-agent doctor`

Purpose: validate installed setup.

Checks (additions to the original list in **bold**):

- Manifest exists and parses.
- `.ai/INDEX.md` exists.
- Root adapters point to `.ai/INDEX.md`.
- Selected adapter files exist.
- Hook configs call `mivia-agent hook ...`.
- Skills have valid frontmatter.
- Generated files have valid markers/checksums.
- CI file calls `mivia-agent doctor --json`.
- No generated cache files are staged.
- No known secret paths are generated or referenced.
- **Every loop references only known adapters and known steps.**
- **Every loop has a bound (`iterations` in MVP; `budget` rejected).**
- **Every review step has a valid consensus mode and `min_reviewers` that is satisfiable by the enabled reviewers.**
- **`governance.provider` is `noop` or `agt`; if `agt`, the AGT dependency is importable.**
- **Every orchestrable adapter passes `Detect()` as headless-capable, OR is explicitly marked guidance-only.**
- **Global config (`~/.agents/`) is readable (if present) and does not conflict with project config; report warnings on conflicts.**

Exit codes:

- `0`: ok.
- `1`: invalid setup.
- `2`: warnings only when `--strict` is set.

### `mivia-agent audit`

Purpose: report workflow quality gaps without writing.

Findings (additions in **bold**):

- Missing canonical `.ai/`.
- Duplicated policy in adapters.
- Missing hooks for selected adapters.
- Missing CI control check.
- Missing quality stamp gate.
- Missing contract matrix.
- Weak or empty verifier command matrix.
- Unsafe MCP config.
- Generated files modified outside managed blocks.
- **Loop with no review step before a protected action.**
- **Consensus policy weaker than profile requires (e.g. `first-pass` under strict).**
- **Reviewer fan-out where `min_reviewers` exceeds the number of enabled headless adapters.**
- **Governance provider `noop` under strict profile.**
- **Global rule conflicts with project rule (same file name, divergent content).**

### `mivia-agent preflight`

Purpose: validate current diff and write stamp. Unchanged from the original spec, with one addition: the stamp now optionally embeds the most recent governance `Decision` refs so hooks can verify that policy was satisfied, not just that a stamp exists.

Stamp:

```json
{
  "head": "<sha>",
  "diff_sha256": "<sha256>",
  "changed_files": [],
  "contract_rows": [],
  "focused_verifiers": [],
  "broad_verifiers": [],
  "mutation_proofs": [],
  "not_run": [],
  "policy_decision_refs": [],
  "created_at": "<rfc3339>"
}
```

### `mivia-agent run`

Purpose: execute a named workflow/loop by orchestrating adapters headlessly.

Flags:

- `--repo <path>` default `.`
- `--workflow <name>` (a loop id from `mivia-agent.yaml` or a file under `.ai/workflows/`)
- `--step <id>` (run a single step; otherwise run the whole loop)
- `--input-artifact <path>` (seed input for the first step)
- `--var key=value` repeated (template variables for prompt rendering)
- `--max-iterations <n>` (override the loop bound, cannot exceed manifest value)
- `--dry-run` (resolve and print the execution plan + adapter invocations without invoking)
- `--json` (stream run events as JSONL to stdout)
- `--strict`

Behavior:

1. Load manifest + selected loop; validate it via the same rules as `doctor`.
2. Build the execution DAG (steps, edges, gates).
3. For each step, in order:
   - Render the prompt; write inputs to `.ai/runs/<run-id>/<step>/iter-<nnn>/input/`.
   - If the step is a producer: pick the adapter, enforce non-interactive approval mode, max-turns, timeout; invoke via `Adapter.Run`; capture artifact + structured metadata.
   - If the step is a review: fan out to all reviewers concurrently (`oklog/run`); collect `Verdict`s; apply the consensus policy; record the decision.
   - On review fail with `on_fail: iterate`: route reviewer notes back to the producing step, increment iteration counter, continue.
   - On `exit_when.gate` pass: stop the loop successfully.
   - On bound exhaustion: apply `on_exhausted`.
4. Before any protected action inside a loop, require a fresh stamp and a passing `policy.Decide()`.
5. Write the full trace to `.ai/runs/<run-id>/trace.jsonl` and a concise summary to stdout/JSON.

Run artifacts never include raw prompts or raw model output — only declared artifacts, structured verdicts, decision refs, and scrubbed metadata.

Exit codes:

- `0`: loop completed and exit gate passed (or `on_exhausted: proceed|warn`).
- `1`: loop failed (`on_exhausted: fail`, adapter error, or policy denial).
- `2`: warnings (e.g. `on_exhausted: warn`).

### `mivia-agent review`

Purpose: run a one-off consensus review without defining a full loop.

Flags:

- `--repo <path>`, `--artifact <path>`, `--reviewers <name>,...`, `--mode <consensus mode>`, `--min-reviewers <n>`, `--json`.

Behavior: a thin wrapper over the review step of `run`, useful for ad-hoc cross-CLI verification.

### `mivia-agent adapters`

Purpose: list installed/present adapters and their capabilities.

Behavior:

- For each known adapter, run `Detect()`: binary present? version? headless-capable? configured approval mode?
- Print a table (or JSON) of `name | present | headless | role | approved_for_run`.
- Exit non-zero if any adapter enabled as `orchestrable` is not headless-capable.

### `mivia-agent hook codex <event>`

Events: `user-prompt-submit`, `pre-tool-use`, `permission-request`, `stop`.

Behavior:

- Read JSON payload from stdin.
- Parse Codex hook fields defensively.
- For implementation/review-shaped prompts, emit concise additional context (including available loops the user can invoke).
- For protected commands, deny if stamp is missing/stale or if `policy.Decide()` denies.
- For done-shaped final responses, block if stamp, required handoff fields, or a passing review decision is missing.

### `mivia-agent hook claude <event>`

Events: `pre-tool-use`, `stop`.

Behavior:

- Read JSON payload from stdin.
- Emit Claude-compatible decisions.
- Share policy with the Codex hook engine.
- Report when project hooks may be unavailable due to managed settings only if detectable locally.

### `mivia-agent import`

Purpose: inspect existing agent setup and produce a migration plan. Unchanged from the original, plus: detect existing loop/workflow definitions in common formats and propose mappings into `.ai/workflows/`.

### `mivia-agent update`

Purpose: update generated files to newer template versions. Unchanged; managed-block-only updates, preserve user edits, run doctor after.

## Implementation Workstreams

Original WS0–WS8 are preserved. New workstreams (WS9–WS13) add the orchestrator, consensus, loops, governance, and adapter breadth. Sequencing: WS0–WS4 first (repo, manifest, templates, doctor/audit, preflight stamp), then the new WS9–WS13 in parallel where possible, then WS5 (hooks) which now depends on WS12 (policy), then WS6–WS8 (adapter templates, import/update, distribution).

### WS0 - Bootstrap Repository

Unchanged. Create `go.mod`, `cmd/mivia-agent/main.go`, `internal/cli/root.go`, `README.md`, `PROJECT_PLAN.md`, `.gitignore`, `.github/workflows/ci.yml`.

Tests: `TestRootCommandShowsHelp`, `TestVersionCommandPrintsVersion`.

### WS1 - Manifest, Git State, Path Policy, And Global Config

Unchanged, plus manifest fields for `adapters` (with roles), `routing`, `loops`, `governance`, and `global` layer. Global config (`~/.agents/`) reading is added: parse `~/.agents/mivia-agent.yaml` if present and merge defaults under the project manifest. Tests must cover the new fields:

- `TestManifestDefaultsIncludeRoutingAndLoopDefaults`
- `TestManifestRejectsUnknownAdapterRole`
- `TestManifestRejectsBudgetBoundInMVP`
- `TestManifestRejectsExpertProfileInMVP`
- `TestGlobalConfigMergesUnderProjectManifest` (global defaults layered, explicit project values win)
- `TestGlobalConfigAbsentSilentlyIgnored` (no `~/.agents/` → no error, no warnings)

### WS2 - Template System, Init, And Global Layer

Unchanged, plus render `templates/workflows/*` and `templates/review-policies/*`. Init reads global `~/.agents/rules/` and `~/.agents/skills/` and layers them into the effective config (project wins on conflict). `.agents/skills.json` in the target repo includes both global and project skills.

### WS3 - Doctor And Audit

Unchanged, plus the new doctor checks and audit findings listed in those sections (including global config conflict detection).

### WS4 - Preflight Stamp

Unchanged, plus `policy_decision_refs` in the stamp.

### WS5 - Hook Engine

Unchanged in shape; depends on WS12. Hooks must call `policy.Decide()` for protected actions when governance is enabled.

### WS6 - Adapter Templates

Add a Crush adapter shim template (`templates/adapters/crush/`), pending headless verification. Otherwise unchanged.

### WS7 - Import And Update

Unchanged, plus loop/workflow migration detection.

### WS8 - CI, Release, And Docs

Add `docs/adapter-authoring.md` and `docs/loop-authoring.md`. CI additionally runs `mivia-agent adapters --json` and a fixture `mivia-agent run --dry-run`.

### WS9 - Adapter System

Create:

- `internal/adapter/adapter.go` (interface, types, `Detect`/`Run`/`Review` contracts)
- `internal/adapter/codex.go`, `claude.go`, `antigravity.go`, `crush.go`
- `internal/adapter/*_test.go`

Behavior:

- Each adapter encapsulates exact headless flags for one CLI.
- `Detect()` reports presence + headless capability.
- `Run()` enforces non-interactive approval mode, turn/time budgets; returns scrubbed `Result`.
- `Review()` returns a structured `Verdict`.

Tests first (per adapter, behind a fake-runner so no real CLI is required in CI):

- `TestCodexDetectHeadlessCapability`
- `TestCodexRunEnforcesNonInteractiveApproval`
- `TestCodexRunMapsExitCode`
- `TestCodexRunScrubsSecretsFromStdout`
- (same four for `claude`, `antigravity`, `crush`)
- `TestAdapterReturnsErrorWhenHeadlessNotCapable` (covers Crush-if-not-headless)

Mutation proof:

- Remove the non-interactive approval enforcement; the approval test must fail.
- Break exit-code mapping; the exit-code test must fail.

### WS10 - Orchestrator

Create:

- `internal/orchestrator/orchestrator.go`
- `internal/orchestrator/dag.go`
- `internal/orchestrator/loop.go`
- `internal/orchestrator/*_test.go`
- `internal/runstore/runstore.go` (`.ai/runs/<id>/` storage)

Behavior:

- Resolve a loop into a DAG of steps.
- Execute producer steps sequentially; review steps with parallel fan-out via `oklog/run`.
- Enforce loop bounds (`iterations` only in MVP; reject `budget`).
- Apply `exit_when` gates and `on_exhausted`.
- Write a JSONL trace and a summary.

Tests first:

- `TestDAGResolvesSequentialHandoff`
- `TestReviewFansOutToAllReviewersConcurrently`
- `TestLoopExitsWhenGatePasses`
- `TestLoopFailsOnExhaustionWithOnExhaustedFail`
- `TestLoopRejectsBudgetBoundInMVP`
- `TestOrchestratorRequiresFreshStampBeforeProtectedAction`
- `TestRunArtifactContainsNoRawPromptsOrOutputs`

Mutation proof:

- Remove the bound check; the budget-rejection / exhaustion test must fail.
- Remove the stamp-before-protect check; the protected-action test must fail.

### WS11 - Consensus

Create:

- `internal/consensus/consensus.go`
- `internal/consensus/*_test.go`

Behavior:

- Implement `majority`, `unanimous`, `weighted`, `first-pass` over a set of `Verdict`s.
- Implement tie-breakers: `strict`, `manual`, `prefer:<adapter>`.

Tests first:

- `TestMajorityPassesAboveThreshold`
- `TestUnanimousFailsOnOneReject`
- `TestStrictTieBreakerFailsOnTie`
- `TestWeightedRespectsWeights`
- `TestMinReviewersUnsatisfiableFails`

Mutation proof:

- Flip majority threshold direction; majority test must fail.

### WS12 - Governance Provider

Create:

- `internal/policy/policy.go` (interface, `Decision`, `Action`, `Event`)
- `internal/policy/noop.go`
- `internal/policy/agt.go` (lazy AGT wiring, build-tagged)
- `internal/policy/*_test.go`

Behavior:

- `noop` allows all, records to `.ai/audit.jsonl`.
- `agt` delegates to the AGT Go SDK; maps `Action` → AGT input; stores `Decision` evidence (no raw payloads).

Tests first:

- `TestNoopAllowsAndRecords`
- `TestAGTProviderMapsActionToDecision` (behind build tag; skip if dependency absent)
- `TestProviderDecisionRefPersistsInStamp`

Mutation proof:

- Make `noop` not record; audit-log test must fail.

### WS13 - `run` / `review` / `adapters` Commands

Create:

- `internal/cli/run.go`, `review.go`, `adapters.go`
- corresponding `_test.go`

Behavior:

- Wire `run` to orchestrator + adapter registry + policy + runstore.
- Wire `review` as a one-off consensus step.
- Wire `adapters` to the adapter registry's `Detect()`.

Tests first:

- `TestRunDryRunPrintsPlanWithoutInvoking`
- `TestRunExecutesResearchLoopFixture`
- `TestReviewOneOffConsensus`
- `TestAdaptersReportsHeadlessCapability`

Mutation proof:

- Make `--dry-run` invoke; dry-run test must fail.

## Test Map

Unit:

- Manifest parsing (including routing/loops/governance).
- Path policy.
- Template variable validation.
- Command classification.
- Stamp validation (including policy refs).
- Hook payload parsing.
- Report formatting.
- Adapter `Detect`/`Run`/`Review` behind fakes.
- DAG resolution, fan-out, loop bounds.
- Consensus modes and tie-breakers.
- Policy provider decisions + recording.

Contract:

- Generated file set per profile and adapter.
- Codex/Claude hook output shape.
- Copilot instruction output shape.
- Doctor/audit finding categories (including loop/consensus/governance findings).
- `run` trace JSONL shape.
- `review` verdict shape.
- `adapters` report shape.

Integration:

- Temp Git repo init.
- Idempotent init.
- Existing-file conflict.
- Preflight stamp over real Git diff.
- Generated hook invoked as subprocess with sample stdin.
- Doctor over generated fixture repo.
- Full `run` of `research-loop` using faked adapters over a temp repo, asserting artifacts, trace, and exit code for pass/fail/iterate.
- `review` one-off over faked adapters asserting consensus outcome.
- `adapters` over a fixture with some adapters present and some absent.

Negative:

- No Git repo.
- Dirty worktree.
- Missing `.ai/INDEX.md`.
- Stale stamp.
- Malformed manifest.
- Unsupported adapter / adapter not headless-capable.
- Loop references unknown adapter or step.
- Loop has no bound / uses `budget` in MVP.
- Consensus `min_reviewers` unsatisfiable.
- Symlink escape.
- Secret path.
- Missing hook binary.
- Read-only file.
- Windows-style path.
- Policy denial under AGT provider on a protected action.

## MVP Definition

MVP is done when:

1. A new empty Git repo can run:
   ```bash
   mivia-agent init --profile standard --adapter codex --adapter claude --adapter copilot --write
   ```
2. `mivia-agent doctor` passes.
3. Re-running `init --write` creates no diff.
4. `mivia-agent preflight` writes a valid stamp.
5. Generated Codex and Claude hooks deny protected actions without a fresh stamp (and, if enabled, without a passing policy decision).
6. Copilot instructions are generated as guidance only.
7. `mivia-agent adapters` correctly reports which enabled adapters are headless-capable.
8. `mivia-agent run --workflow research --dry-run` prints a valid execution plan; with faked adapters, `mivia-agent run --workflow research` completes a pass/fail/iterate cycle and writes a correct trace + summary.
9. `mivia-agent review --artifact <path> --reviewers codex,claude --mode majority` returns a correct consensus verdict with faked adapters.
10. `go test ./...` passes.
11. Mutation proof exists for: path policy, stale stamp, protected command, overwrite guards, non-interactive approval enforcement, loop bound enforcement, stamp-before-protect, and consensus threshold.

Do not implement `expert`/budget loops, the AGT production wiring beyond the interface + noop, live MCP auth, import, update, or plugin packaging before MVP is green.

## Release Plan

### Phase 0 - Empty Repo Setup

- Initialize Go module.
- Add Cobra root command.
- Add CI.
- Add this plan.

### Phase 1 - Init And Doctor

- Manifest (incl. routing/loops/governance fields).
- Git state.
- Path policy.
- Templates (incl. workflow + review-policy templates).
- Init.
- Doctor.

### Phase 2 - Adapter System And Preflight

- Adapter interface + Codex + Claude adapters (headless).
- Preflight stamp (with policy refs).
- `adapters` command.

### Phase 3 - Orchestrator, Consensus, `run`/`review`

- DAG + loop engine.
- Consensus evaluator.
- `run` and `review` commands.
- Runstore + trace.
- Antigravity + Crush adapters (Crush gated on headless verification).

### Phase 4 - Governance, Hooks, Strict Profile

- Policy provider (noop + AGT interface).
- Hook engine wired to policy.
- Strict profile consensus/governance requirements.
- Copilot adapter templates.

### Phase 5 - Import, Update, Distribution

- Existing setup import (incl. loop migration).
- Managed template update.
- Release binaries + checksums.
- Install docs (user-guide, adapter-authoring, loop-authoring).
- Optional Homebrew tap.
- Optional Codex plugin packaging only after CLI proves stable.

### Phase 6 (post-MVP) - Budget Loops And Expert Profile

- `bound: budget` engine with token/minute/cost budgets.
- `expert` profile.
- Production AGT wiring (non-lazy) for strict repos that want full OWASP-Agentic-Top-10 coverage.

## Agent Operating Rules For This New Repo

Use these rules from the first commit:

- Read this plan before work.
- Work one phase at a time.
- Write tests first where feasible.
- Use temp Git repos for boundary tests.
- Do not mock Git behavior when the risk is Git behavior.
- Do not mock hook output shape when the risk is agent hook compatibility.
- Do not mock adapter headless flags when the risk is unattended approval leakage — use fakes that still enforce the approval-mode contract.
- Preserve user-owned files in fixtures.
- Mutation-proof guard tests before claiming done.
- Keep adapters thin and `.ai/` canonical.
- Cite official docs when changing vendor-specific behavior; each adapter cites its targeted doc URL + version.
- Keep final reports short: changed files, verification, mutation proof, residual risk.

## Ready-To-Paste First Agent Prompt

```text
You are starting a brand-new repository for Mivia AgentKit.

Read first:
1. PROJECT_PLAN.md

Goal:
Implement Phase 0 and Phase 1 only.

Exact scope:
- Initialize Go module github.com/MiviaLabs/mivia-agentkit.
- Create cmd/mivia-agent.
- Add Cobra root command and version command.
- Add manifest parsing, INCLUDING the new fields: adapters (with roles),
  routing, loops, and governance. Reject expert profile and bound:budget in MVP.
- Add Git root/state and diff hash support.
- Add path safety policy.
- Add embedded starter templates (core, adapters, workflows, review-policies).
- Implement global config reading from ~/.agents/ (rules, skills, mivia-agent.yaml defaults); layer under project config.
- Implement init --dry-run/--write for starter profile.
- Implement doctor for starter profile.

Tests first:
- TestRootCommandShowsHelp
- TestVersionCommandPrintsVersion
- TestManifestDefaultsIncludeRoutingAndLoopDefaults
- TestManifestRejectsUnknownAdapterRole
- TestManifestRejectsBudgetBoundInMVP
- TestManifestRejectsExpertProfileInMVP
- TestGlobalConfigMergesUnderProjectManifest
- TestGlobalConfigAbsentSilentlyIgnored
- TestGitStateDetectsChangedTrackedAndUntrackedFiles
- TestDiffHashChangesWhenFileChanges
- TestPathPolicyRejectsTraversalAndSecretPaths
- TestInitDryRunWritesNothing
- TestInitWriteCreatesExpectedFiles
- TestInitWriteIsIdempotent
- TestDoctorPassesFreshInit

Constraints:
- No network calls.
- No live connectors.
- No dependency on any existing Mivia repo.
- No hidden local paths.
- Do not implement adapters, orchestrator, consensus, policy providers, hooks,
  preflight stamp, import, update, plugins, or release yet.
- Implement ~/.agents/ reading but do not write to it; silently ignore if absent.
- Preserve user-owned files; never overwrite without explicit force.

Verification:
- go test ./...
- go run ./cmd/mivia-agent --help
- go run ./cmd/mivia-agent init --repo <temp-git-repo> --profile starter --write
- go run ./cmd/mivia-agent doctor --repo <temp-git-repo>
- git diff --check

Mutation proof:
- Disable path traversal rejection; path-policy test must fail.
- Disable init overwrite guard; overwrite/idempotency test must fail.

Final report:
- Changed files.
- Verification commands and result.
- Mutation proof result.
- Residual risk.
```

## Ready-To-Paste Second Agent Prompt

```text
Continue Mivia AgentKit after Phase 1 is green.

Read first:
1. PROJECT_PLAN.md
2. Current README.md
3. Current tests under internal/

Goal:
Implement Phase 2 and Phase 3 only: adapter system, preflight stamp,
orchestrator, consensus, and run/review commands.

Exact scope:
- Define the Adapter interface (Detect/Run/Review) and Result/Verdict types.
- Implement Codex and Claude adapters headlessly behind fake runners in tests.
- Implement mivia-agent preflight with policy_decision_refs in the stamp.
- Implement the DAG/loop orchestrator with iteration bounds (reject budget).
- Implement consensus (majority/unanimous/weighted/first-pass + tie-breakers).
- Implement mivia-agent run (--workflow, --dry-run, --json) and mivia-agent review.
- Implement runstore under .ai/runs/<id>/ with trace.jsonl. Global config layering applies to loop/workflow resolution.
- Implement mivia-agent adapters (Detect + headless capability report).

Tests first:
- TestCodexDetectHeadlessCapability
- TestCodexRunEnforcesNonInteractiveApproval
- TestCodexRunMapsExitCode
- TestClaudeRunEnforcesNonInteractiveApproval
- TestAdapterReturnsErrorWhenHeadlessNotCapable
- TestPreflightWritesStampWithPolicyRefs
- TestDAGResolvesSequentialHandoff
- TestReviewFansOutToAllReviewersConcurrently
- TestLoopExitsWhenGatePasses
- TestLoopFailsOnExhaustionWithOnExhaustedFail
- TestLoopRejectsBudgetBoundInMVP
- TestOrchestratorRequiresFreshStampBeforeProtectedAction
- TestMajorityPassesAboveThreshold
- TestUnanimousFailsOnOneReject
- TestStrictTieBreakerFailsOnTie
- TestRunDryRunPrintsPlanWithoutInvoking
- TestRunExecutesResearchLoopFixture
- TestReviewOneOffConsensus
- TestAdaptersReportsHeadlessCapability
- TestRunArtifactContainsNoRawPromptsOrOutputs

Constraints:
- No network calls from mivia-agent itself.
- No live connectors.
- Adapters must enforce a non-interactive approval mode, max turns, and timeout.
- Run artifacts must never include raw prompts or raw model output.
- Do not implement AGT wiring (noop provider only), hooks, import, update, release,
  expert/budget loops, or plugins yet.

Verification:
- go test ./...
- Full research-loop fixture run with faked adapters over a temp repo.
- git diff --check

Mutation proof:
- Remove non-interactive approval enforcement; approval test must fail.
- Remove loop bound check; budget-rejection/exhaustion test must fail.
- Remove stamp-before-protect; protected-action test must fail.
- Flip majority threshold; consensus test must fail.

Final report:
- Changed files.
- Verification commands and result.
- Mutation proof result.
- Residual risk.
```
