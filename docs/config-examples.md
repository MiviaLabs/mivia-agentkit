# Configuration Examples

This file shows example configuration for the current implemented CLI surface.

Use these as starting points, then validate with:

```bash
go run ./cmd/mivia-agent doctor --repo /path/to/repo --json
```

## Example `mivia-agent.yaml`

This example keeps the current supported manifest fields and a simple review loop.

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
  claude:
    enabled: true
    role: orchestrable
  copilot:
    enabled: true
    role: guidance
  antigravity:
    enabled: false
    role: orchestrable
  crush:
    enabled: false
    role: guidance

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
        artifact: notes/research.md
        max_turns: 4
      - id: review
        reviewers:
          - codex
          - claude
        artifact: notes/research.md
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
        artifact: .ai/runs/latest/patch.md
        approval: commit
        max_turns: 4
        timeout: 10m
      - id: review
        reviewers:
          - claude
        artifact: .ai/runs/latest/patch.md
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
    artifact: notes/research.md
    max_turns: 4
  - id: review
    reviewers:
      - codex
      - claude
    artifact: notes/research.md
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

- Supported profiles are `starter`, `standard`, and `strict`.
- Workflow `bound: budget` is not supported in MVP.
- Producer and reviewer adapters in loops must be enabled and `orchestrable`.
- `copilot` and `crush` are guidance-only and cannot be workflow producers or reviewers.
- `init --with-loop`, `run --step`, `run --input-artifact`, and `run --var` are reserved surface today.
