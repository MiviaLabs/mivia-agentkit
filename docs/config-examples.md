# Configuration Examples

This file shows example configuration for the current implemented CLI surface.

Use these as starting points, then validate with:

```bash
go run ./cmd/mivia-agent doctor --repo /path/to/repo --json
```

## Example `mivia-agent.yaml`

This example keeps the current supported manifest fields and a simple review loop.

For `run` workflows, treat each step `artifact` as a run-local name, not a shared repo path. The engine writes it under:

```text
.ai/runs/<run-id>/<step-id>/iter-<nnn>/<artifact>
```

That means separate `mivia-agent run` executions already get separate directories, and retried steps within one run also keep distinct artifact files instead of overwriting the prior iteration.

```yaml
version: "1"
profile: standard
template_version: dev

project:
  name: example-repo

adapters:
  codex:
    enabled: true
    role: orchestrable
    model: gpt-5.5
    effort: high
  claude:
    enabled: true
    role: orchestrable
    model: sonnet
    effort: medium
  copilot:
    enabled: true
    role: guidance
  antigravity:
    enabled: false
    role: orchestrable
  crush:
    enabled: true
    role: guidance
    model: openai/gpt-5.5
    params:
      provider: openai
      base_url: https://api.openai.com/v1

routing:
  default_producer: codex
  default_reviewers:
    - codex
    - claude
  consensus:
    mode: majority
    tie_breaker: strict
    min_reviewers: 2
  on_review_fail: iterate
  max_iterations: 3

loops:
  research:
    description: Produce notes, review them, and iterate until consensus passes.
    bound: iterations
    max_iterations: 3
    exit_when: review-pass
    on_exhausted: warn
    steps:
      - id: produce
        producer: codex
        artifact: research.md
        model: gpt-5.5
        effort: high
        max_turns: 4
      - id: review
        reviewers:
          - codex
          - claude
        artifact: research.md
        model: sonnet
        effort: low
        consensus:
          mode: majority
          min_reviewers: 2
          tie_breaker: strict
        on_fail: iterate
  patch-review:
    description: Draft a patch, review it, then repeat on review failure.
    bound: iterations
    max_iterations: 2
    exit_when: review-pass
    on_exhausted: fail
    steps:
      - id: patch
        producer: codex
        artifact: patch.md
        approval: commit
        max_turns: 4
        timeout: 10m
      - id: review
        reviewers:
          - claude
        artifact: patch.md
        consensus:
          mode: first-pass
          min_reviewers: 1
          tie_breaker: strict
        on_fail: iterate

commands:
  audit-local: go run ./cmd/mivia-agent audit --repo .
  doctor-local: go run ./cmd/mivia-agent doctor --repo . --json

protected_actions:
  - commit
  - push
  - pull_request

quality:
  required_verifiers:
    - go test ./internal/cli/... -count=1
    - go vet ./internal/cli/...

paths:
  allow:
    - .ai/**
    - docs/**
    - internal/**
  deny:
    - .env
    - secrets/**

governance:
  provider: noop
  audit_log: .ai/audit.jsonl
  policy_decisions: .ai/policy-decisions.jsonl

global:
  layer: ~/.agents
  merge: project_wins

mcp:
  servers:
    - filesystem
    - github
```

## Example `.ai/workflows/research.yaml`

Use a separate workflow file when you want to keep the manifest smaller.

```yaml
description: Standalone research loop.
bound: iterations
max_iterations: 3
exit_when: review-pass
on_exhausted: warn
steps:
  - id: produce
    producer: codex
    artifact: research.md
    max_turns: 4
  - id: review
    reviewers:
      - codex
      - claude
    artifact: research.md
    consensus:
      mode: majority
      min_reviewers: 2
      tie_breaker: strict
    on_fail: iterate
```

Run it with:

```bash
go run ./cmd/mivia-agent run --repo /path/to/repo --workflow research --dry-run --json
```

## Example `~/.agents/mivia.yaml`

This is the optional global defaults layer. Project config still wins.

```yaml
defaults:
  profile: standard
  template_version: dev
  adapters:
    codex:
      enabled: true
      role: orchestrable
    claude:
      enabled: true
      role: orchestrable
  governance:
    provider: noop
    audit_log: .ai/audit.jsonl
```

## Notes

- Precedence is `step model/effort` -> `adapter default model/effort` -> CLI default.
- `run --dry-run --json` reports a per-step `runtime` list so you can inspect the resolved adapter, model, and effort before execution.
- Supported profiles are `starter`, `standard`, and `strict`.
- Workflow `bound: budget` is not supported in MVP.
- Separate `run` executions are isolated under unique `.ai/runs/<run-id>/` directories.
- Retried steps within one run are stored under per-iteration subdirectories such as `iter-001` and `iter-002`.
- Producer and reviewer adapters in loops must be enabled and `orchestrable`.
- `copilot` and `crush` are guidance-only and cannot be workflow producers or reviewers, even when `crush` has `model` or `params` config.
- `init --with-loop`, `run --step`, `run --input-artifact`, and `run --var` are reserved surface today.
