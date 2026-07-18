# AgentKit Examples

Copy-paste starting points for common `mivia-agent` workflows. Each example is a
complete, runnable configuration — validate it with `doctor` before you run it.

## When to read this

You are an agent (or a human operator) about to drive `mivia-agent`. Pick the
example that matches the **kind of work**, because that decides two things:

1. **Does the step need to write files?** Production work (patching code,
   generating artifacts, scaffolding) needs a producer with file-write tools and
   a named run artifact. Pure review or research does not — it reads code and
   returns a verdict or notes, and should not be given a write target.
2. **Which adapter and model?** The `zai` adapter (Z.ai GLM models) is the
   default covered here. Other adapters (`codex`, `claude`, `crush`) follow the
   same shape; see [../adapter-authoring.md](../adapter-authoring.md).

## File-writing vs. read-only — the rule agents must follow

| Step role | Writes files? | Typical adapter flags | Artifact |
|---|---|---|---|
| **Producer** (implement / patch / scaffold) | **Yes** | producer step; use `approval: protect:commit` when the step needs the stamp/policy gate | required (`artifact: <name>.md`) |
| **Reviewer** | No | reviewer step, read-only | the artifact under review |
| **Researcher** | No | producer step, **no** write tools, short max-turns | notes artifact only |

**Default assumption: a producer step may write run artifacts.** Configure it
with a real `artifact`. When the step is a protected write path, set
`approval: protect:commit` so `mivia-agent` runs a fresh quality stamp plus
policy decision **before the adapter runs**. That field is a stamp/policy gate
only and is **not a Git commit**: ordinary `run` workflows do not stage or
`git commit`. Only drop write tools when the step is explicitly review or
research; then keep `max_turns` low and do not set `approval: protect:commit`.

Agents: if a task says "fix", "implement", "refactor", or "scaffold", it is a
**write** step. If it says "review", "audit", "investigate", or "summarize", it
is **read-only** — do not grant it `approval: protect:commit`.

## Examples

- [zai-glm-examples.md](zai-glm-examples.md) — ZAI adapter with GLM-5.2 and
  GLM-5-Turbo: install/auth, headless one-shot, write (patch) loop, read-only
  review loop, and research loop.
- [../config-examples.md](../config-examples.md) — full `mivia-agent.yaml`
  reference with Codex/Claude/Crush loops.
- [../loop-authoring.md](../loop-authoring.md) — loop semantics (bounds,
  consensus, exit conditions).

## Verify before running

```bash
go run ./cmd/mivia-agent doctor --repo . --json
go run ./cmd/mivia-agent adapters --repo . --json
go run ./cmd/mivia-agent run --repo . --workflow <name> --dry-run --json
```
