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

Step fields come from the `config.Step` contract in [internal/config/loop.go](../internal/config/loop.go), including producer, reviewers, consensus, artifact name, and protected-action approval hints.

For `run` workflows, `artifact` is a run-local artifact name. The engine writes it under:

```text
.ai/runs/<run-id>/<step-id>/iter-<nnn>/<artifact>
```

Separate runs therefore do not overwrite each other across terminals, and repeated iterations within one run also keep separate artifact files.

## Control Fields

- `bound: iterations` limits total loop passes.
- `exit_when: review-pass` ends the loop once review consensus passes.
- `on_exhausted: fail|warn|proceed` decides what happens when the iteration cap is hit.
- `on_fail: iterate` routes reviewer feedback back to the producer for another pass.

Strict-profile loops that end in a protected action must use `majority` or `unanimous`; `first-pass` is rejected.

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

## Practical Checks

- Prefer one producer step followed by one review step.
- Keep reviewer count satisfiable by enabled headless adapters.
- Use stable artifact names so run traces stay stable.
- Do not point loop artifacts at shared repo paths such as `notes/foo.md` or `.ai/runs/latest/...`; the runstore already places them under the per-run directory.
- Test new loops with `mivia-agent run --workflow <name> --dry-run --json` before real execution.
