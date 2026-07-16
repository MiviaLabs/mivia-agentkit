# ZAI Adapter Examples (GLM-5.2 / GLM-5-Turbo)

The `zai` adapter drives the [`@guizmo-ai/zai-cli`](https://www.npmjs.com/package/@guizmo-ai/zai-cli)
(binary `zai`) against Z.ai's GLM models. It runs in headless mode (`zai -p`),
so it is fully non-interactive and safe for `mivia-agent` workflows.

Verified 2026-07-16 against `zai` v0.3.5: `glm-5.2` and `glm-5-turbo` are both
accepted by the Z.ai `api/coding/paas/v4` endpoint.

## Adapter behavior at a glance

| Field | `zai` support |
|---|---|
| `model` | yes — passed as `-m`. Defaults to `glm-5.2`. Override per-step or via the `model` param. |
| `effort` | **no** — `zai` has no reasoning-effort flag; any value is rejected before the CLI runs. |
| `approval` | `never` only. zai headless mode never prompts, so there is no other valid value. |
| `params` | `model` only. Unknown params are rejected. |
| `max_turns` | mapped to `--max-tool-rounds` (bounds agentic tool use). |
| `artifact` | surfaced via `Result.Stdout`; zai has no native `--output-file` flag, so the engine writes the returned bytes to the run-local artifact path. |

## 1. Install and authenticate (one-time, on the host)

`mivia-agent` never makes network calls itself — the `zai` CLI does. Install it
once per machine:

```bash
# Requires Node.js 18+
npm install -g @guizmo-ai/zai-cli
zai --version   # expect: 0.3.5 or newer
```

Authenticate with a Z.ai API key from https://z.ai/manage-apikey:

```bash
# Option A — persisted (recommended for a dedicated runner):
zai config --set-key <your-key>

# Option B — per-process env var (recommended for CI/agents):
export ZAI_API_KEY=<your-key>
```

Smoke-test both models headlessly before wiring them into a workflow:

```bash
zai -m glm-5.2     -p "Reply with exactly: OK" --no-color
zai -m glm-5-turbo -p "Reply with exactly: OK" --no-color
```

## 2. Headless one-shot (no workflow file)

For a single prompt with no loop, invoke `zai` directly. Use this for quick
tasks where you do not need review/iteration:

```bash
# Read-only research — note: no approval: commit, short rounds.
zai -m glm-5.2 -d /path/to/repo --max-tool-rounds 4 --no-color \
  -p "Summarize the auth timeout risk in internal/auth. Do not modify files."

# Write task — let the agent edit files in the workdir.
zai -m glm-5-turbo -d /path/to/repo --max-tool-rounds 12 --no-color \
  -p "Add a unit test for ParseDuration error paths in internal/config."
```

`mivia-agent` itself is the right tool when you want review, iteration, or
protected-action gating — see the loops below.

## 3. `mivia-agent.yaml` — enable the zai adapter

```yaml
adapters:
  zai:
    enabled: true
    role: orchestrable
    model: glm-5.2          # default model for zai steps; override per-step
```

Do not set `effort` on the `zai` adapter — it is unsupported and config
validation will reject it.

## 4. Write loop — produce a patch, then review it (GLM-5-Turbo)

This is the **common case**: the producer writes files, the reviewer is
read-only. The producer needs `approval: commit` because the artifact is meant
to land in the repo. Put this in `.ai/workflows/zai-patch-review.yaml`:

```yaml
version: 1
name: zai-patch-review
description: GLM-5-Turbo produces a patch artifact; GLM-5.2 reviews it.
bound: iterations
max_iterations: 2
exit_when: review-pass
on_exhausted: fail
steps:
  - id: patch
    producer: zai
    model: glm-5-turbo
    artifact: patch.md
    approval: commit          # WRITE step: artifact may persist to the repo
    max_turns: 12
    timeout: 15m
  - id: review
    reviewers:
      - zai
    model: glm-5.2
    artifact: patch.md         # READ-ONLY: reviewer reads, does not write
    consensus:
      mode: majority
      tie_breaker: strict
      min_reviewers: 1
    on_fail: iterate
```

Run it:

```bash
go run ./cmd/mivia-agent run --repo /path/to/repo \
  --workflow zai-patch-review --dry-run --json

go run ./cmd/mivia-agent run --repo /path/to/repo \
  --workflow zai-patch-review \
  --var objective="harden ParseDuration against negative values" --json
```

`patch.md` (and any files the producer edited) land under
`.ai/runs/<run-id>/patch/iter-NNN/`. `approval: commit` is a protected action —
`mivia-agent` requires a fresh quality stamp before it allows the commit.

## 5. Read-only review loop (GLM-5.2 only)

Use this when an artifact already exists and you only want a verdict — **no
file writing anywhere**. Neither step gets `approval: commit`:

```yaml
version: 1
name: zai-review-only
description: GLM-5.2 reviews an existing artifact. No writes.
bound: iterations
max_iterations: 1
exit_when: review-pass
on_exhausted: warn
steps:
  - id: review
    reviewers:
      - zai
    model: glm-5.2
    artifact: existing-audit.md
    max_turns: 4               # read-only: keep the tool budget small
    consensus:
      mode: majority
      tie_breaker: strict
      min_reviewers: 1
    on_fail: warn
```

A reviewer returns a structured verdict (`{pass, severity, notes}`). `pass=true`
means the artifact is accepted as workflow output — it does **not** mean the
repo is approved for merge or release.

## 6. Research loop (read-only notes, then review)

Research produces notes, not repo changes. The producer is **read-only**: no
`approval: commit`, low `max_turns`, and its artifact is a notes file rather
than a patch:

```yaml
version: 1
name: zai-research
description: GLM-5-Turbo gathers read-only research notes; GLM-5.2 reviews them.
bound: iterations
max_iterations: 3
exit_when: review-pass
on_exhausted: warn
steps:
  - id: research
    producer: zai
    model: glm-5-turbo
    artifact: research.md      # notes artifact, not a repo patch
    max_turns: 6               # READ-ONLY: no approval: commit
    timeout: 10m
  - id: review
    reviewers:
      - zai
    model: glm-5.2
    artifact: research.md
    consensus:
      mode: majority
      tie_breaker: strict
      min_reviewers: 1
    on_fail: iterate
```

```bash
go run ./cmd/mivia-agent run --repo /path/to/repo \
  --workflow zai-research \
  --var objective="map every call site of ValidateEffortValue" --json
```

## Choosing the model

- **`glm-5.2`** — flagship reasoning model. Use for review, audit, and any step
  where judgment matters. Set as the adapter default.
- **`glm-5-turbo`** — faster, cheaper tier. Use for high-volume production
  (patch generation across many files) and research gathering where speed
  matters more than depth.

A common split: `glm-5-turbo` produces, `glm-5.2` reviews (as in the loops
above).

## Notes

- `zai` exits `0` even on Z.ai API errors (e.g. `400 Unknown Model`); the error
  appears on stderr and is surfaced in `Result.Stderr`. Always set a valid
  `model`.
- zai output is JSON-lines on stdout. `mivia-agent` scrubs secrets and drops raw
  provider fields before writing artifacts — never disable that path.
- Invalid `effort`, unknown `params`, or any approval other than `never` are
  rejected before the `zai` CLI is invoked.
- `ZAI_API_KEY` must be present in the environment of the process running
  `mivia-agent`; the binary does not manage credentials.
