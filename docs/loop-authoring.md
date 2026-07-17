# Loop Authoring

PRD references: `FR-4.1` to `FR-4.4`, `FR-5.1` to `FR-5.3`, `FR-6.1` to `FR-6.4`

## Shape

Loops live in `mivia-agent.yaml` or `.ai/workflows/*.yaml`. MVP supports `bound: iterations` only.

Core fields:

- `name`
- `description`
- `bound`
- `max_iterations`
- `steps`
- `exit_when`
- `on_exhausted`

Step fields come from the `config.Step` contract in [internal/config/loop.go](../internal/config/loop.go), including producer, reviewers, consensus, artifact name, optional `model`, optional `effort`, and protected-action approval hints.

For `run` workflows, `artifact` is a run-local artifact name. The engine writes it under:

```text
.ai/runs/<run-id>/<step-id>/iter-<nnn>/<artifact>
```

Separate runs therefore do not overwrite each other across terminals, and repeated iterations within one run also keep separate artifact files.
The artifact name is the handoff contract between producer and reviewer; it should not include a shared repo path.

Future config should expose the run store base directory only, for example:

```yaml
run_store:
  base_dir: .ai/runs
```

That base directory must remain repo-relative, ignored, and guarded against absolute paths, `..`, `.git`, `.env`, secrets, and raw provider payload dumps.

Model/effort precedence is:

- step `model` / `effort`
- adapter default `model` / `effort`
- adapter CLI default when neither field is set

## Control Fields

- `bound: iterations` limits total loop passes.
- `exit_when: review-pass` ends the loop once review consensus passes.
- `on_exhausted: fail|warn|proceed` decides what happens when the iteration cap is hit.
- `on_fail: iterate` routes reviewer feedback back to the producer for another pass.

Strict-profile loops that end in a protected action must use `majority` or `unanimous`; `first-pass` is rejected.

### Protected-Action Steps

Steps that should trigger the hook governance gate before execution use `approval: protect:<kind>`:

- `protect:commit` — producer step requires a fresh quality stamp before `git commit`.
- `protect:push` — requires stamp before `git push`.
- `protect:pr` — requires stamp before pull request creation.
- `protect:release` — requires stamp before release.
- `protect:deploy` — requires stamp before deployment (kubectl/helm/terraform apply).

Example:

```yaml
steps:
  - id: produce
    producer: codex
    artifact: patch.md
  - id: gate-commit
    producer: codex
    approval: protect:commit
```

`protected_action` loops exit successfully after the protect step executes. Review steps that fail before the protect step (with `on_fail: fail` or `on_fail: iterate`) prevent the protect step from running in that iteration.

Strict-profile protect-bound loops require `majority` or `unanimous` consensus and at least 2 reviewers; `first-pass` and `min_reviewers: 1` are rejected at validation time.

## Consensus Modes

- `majority`
- `unanimous`
- `weighted`
- `first-pass`

Tie-breakers:

- `strict`
- `manual`
- `prefer:<adapter>`

## Canonical Flow

```text
┌─────────┐   artifact    ┌────────────────────┐  verdicts   ┌────────────┐
│ produce │ ────────────▶ │ review (fan-out)   │ ──────────▶ │ consensus  │
│ (1 CLI) │               │ (N CLIs in parallel│             │ policy     │
└─────────┘               │  via oklog/run)    │             └─────┬──────┘
     ▲                     └────────────────────┘                   │
     │                                                              │
     │  iterate: reviewer notes fed back as input                   ▼
     └──────────────────────────  on_fail: iterate   ◀── pass?  ─── fail/warn/proceed (on_exhausted)

exit_when.gate = review-pass  ──▶ loop ends successfully
bound (iterations) hit        ──▶ on_exhausted: fail | warn | proceed
```

## Research Example

```yaml
version: 1
name: research
description: Research a change, review it, then iterate until consensus passes.
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
    reviewers: [codex, claude]
    artifact: research.md
    consensus:
      mode: majority
      min_reviewers: 2
      tie_breaker: strict
    on_fail: iterate
```

## Crush/Qwen Research Example

This example uses Crush as a local producer and Codex as the reviewer.

```yaml
version: 1
name: crush-research-loop
description: Local Crush/Qwen context research with Codex review.
bound: iterations
max_iterations: 1
exit_when: review-pass
on_exhausted: fail
steps:
  - id: research
    producer: crush
    artifact: go-mivia-context.md
  - id: review
    reviewers: [codex]
    artifact: go-mivia-context.md
    consensus:
      mode: majority
      min_reviewers: 1
      tie_breaker: strict
    on_fail: fail
```

The matching manifest must enable both adapters as orchestrable:

```yaml
adapters:
  codex:
    enabled: true
    role: orchestrable
  crush:
    enabled: true
    role: orchestrable
    model: ollama/qwen3-coder:latest
```

## Practical Checks

- Prefer one producer step followed by one review step.
- Keep reviewer count satisfiable by enabled headless adapters.
- Use step-level `model` or `effort` only when you want to override the adapter default for that specific step.
- Keep `effort` compatible with every adapter selected by the step. Codex supports `minimal`, `low`, `medium`, `high`, and `xhigh`; Claude supports `low`, `medium`, `high`, `xhigh`, and `max`.
- Do not set `effort` on Crush steps until a tested effort mapping exists.
- Do not set `model`, `effort`, or `params` on Antigravity workflow steps; `agy -p` has no documented mapping for those runtime knobs here.
- Use stable artifact names so run traces stay stable.
- Do not point loop artifacts at shared repo paths such as `notes/foo.md` or `.ai/runs/latest/...`; the runstore already places them under the per-run directory.
- Test new loops with `mivia-agent run --workflow <name> --dry-run --json` before real execution, and inspect the `runtime` entries for the resolved adapter/model/effort combination.
