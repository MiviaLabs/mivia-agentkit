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
    role: orchestrable
    model: ollama/qwen3-coder:latest

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

## Example `.ai/workflows/crush-research-loop.yaml`

This mirrors the working go-mivia pattern: Crush/Qwen produces a local context artifact, then Codex reviews it.

```yaml
version: 1
name: crush-research-loop
description: Local Crush/Qwen context research with Codex review.
bound: iterations
max_iterations: 1
steps:
  - id: research
    producer: crush
    artifact: go-mivia-context.md
  - id: review
    reviewers:
      - codex
    artifact: go-mivia-context.md
    consensus:
      mode: majority
      tie_breaker: strict
      min_reviewers: 1
    on_fail: fail
exit_when: review-pass
on_exhausted: fail
```

Preview:

```bash
go run ./cmd/mivia-agent run --repo /path/to/repo \
  --workflow crush-research-loop --dry-run --json
```

Execute:

```bash
go run ./cmd/mivia-agent run --repo /path/to/repo \
  --workflow crush-research-loop --json
```

## Example `.ai/workflows/codex-bug-audit-crush-review.yaml`

Use this when Codex should produce an audit artifact and local Crush/Qwen should independently review artifact quality.

```yaml
version: 1
name: codex-bug-audit-crush-review
description: Codex bug audit artifact reviewed by local Crush/Qwen.
bound: iterations
max_iterations: 1
steps:
  - id: bug-audit
    producer: codex
    artifact: bug-audit.md
  - id: crush-review
    reviewers:
      - crush
    artifact: bug-audit.md
    consensus:
      mode: majority
      tie_breaker: strict
      min_reviewers: 1
    on_fail: fail
exit_when: review-pass
on_exhausted: fail
```

For this pattern, keep the reviewer prompt clear that `pass=true` means the artifact is accepted as useful workflow output. It does not mean the repository is approved for merge or release.

## Example `~/.agents/mivia-agent.yaml`

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
- Effort values are adapter-specific at runtime: Codex supports `minimal`, `low`, `medium`, `high`, and `xhigh`; Claude supports `low`, `medium`, `high`, `xhigh`, and `max`.
- Crush supports `model` and rejects unsupported `effort` until a tested effort mapping exists. Crush provider setup belongs in Crush/Ollama configuration, not in `adapters.crush.params`.
- Antigravity has no documented runtime mapping for `model`, `effort`, or `params`, so those fields are rejected before `agy` runs.
- `run --dry-run --json` reports a per-step `runtime` list so you can inspect the resolved adapter, model, and effort before execution.
- Supported profiles are `starter`, `standard`, and `strict`.
- Workflow `bound: budget` is not supported in MVP.
- Separate `run` executions are isolated under unique `.ai/runs/<run-id>/` directories.
- Retried steps within one run are stored under per-iteration subdirectories such as `iter-001` and `iter-002`.
- Producer and reviewer adapters in loops must be enabled and `orchestrable`.
- `copilot` is guidance-only and cannot be a workflow producer or reviewer.
- `init --with-loop`, `run --step`, `run --input-artifact`, and `run --var` are reserved surface today.
