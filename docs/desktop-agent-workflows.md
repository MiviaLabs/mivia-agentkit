# Desktop Agent Workflows

This guide describes the recommended way to make desktop agents use `mivia-agent` workflows instead of relying on chat memory.

## Recommended Contract

Use the CLI as the runtime boundary:

```bash
mivia-agent adapters --repo . --json
mivia-agent run --repo . --workflow <name> --dry-run --json
mivia-agent run --repo . --workflow <name> --json
```

Workflow `artifact` values are logical artifact names, not repo paths. The run engine writes them under:

```text
.ai/runs/<run-id>/<step-id>/iter-<nnn>/<artifact>
```

Keep `.ai/runs/` ignored. A future portable config should expose only the base directory, for example:

```yaml
run_store:
  base_dir: .ai/runs
```

The base directory must stay repo-relative, auto-created, and ignored. It must reject absolute paths, `..`, `.git`, `.env`, secret paths, and provider payload dumps.

## Skills Vs Hooks

Use skills for workflow intent. A repo skill such as `mivia-agent-workflows` should tell Codex, Claude, Crush-aware operators, or generic `.agents` clients which workflows exist, when to use each workflow, and which dry-run and live commands prove the boundary.

Use hooks for fast deterministic policy. Hooks should add context, block protected actions without evidence, or point the agent to the skill. They should not start long model workflows on every prompt or tool event.

To ask a desktop agent to use the generated skill:

```text
Use $mivia-agent-workflows. Check adapters, run the workflow dry-run, then run the workflow only if the dry-run resolves the expected producer, reviewer, model, and effort.
```

Short desktop prompts:

```text
Use $mivia-agent-workflows. Run workflow research-loop for objective: audit auth timeout handling.
```

```text
Use $mivia-agent-workflows. Dry-run workflow crush-research-loop, verify Crush/Qwen and Codex are resolved, then run it for objective: collect repo context for the billing refactor.
```

```text
Use $mivia-agent-workflows. Inspect workflow outputs from the latest run and report the artifact path and review consensus.
```

Free-text objectives are passed as workflow variables. The desktop agent should translate the first two prompts into:

```bash
mivia-agent run --repo . --workflow <name> --dry-run --json
mivia-agent run --repo . --workflow <name> --var objective="<free-text objective>" --json
```

## Codex

Codex should read `AGENTS.md`, then the repo skill. Project prompt hooks can inject a short reminder such as:

```text
Use $mivia-agent-workflows. Run mivia-agent adapters, then run the workflow dry-run before live execution. Artifacts belong under .ai/runs/<run-id>/...
```

Codex hooks are a lifecycle framework and skills are reusable instruction bundles; keep long workflow instructions in the skill, not in the hook.

Official docs:

- [Codex hooks](https://developers.openai.com/codex/hooks)
- [Codex skills](https://developers.openai.com/codex/skills)
- [AGENTS.md](https://developers.openai.com/codex/guides/agents-md)

## Claude And Generic Agents

Claude project skills should be concise discovery pointers that route to the canonical `.ai/skills/<name>/SKILL.md` file. Generic `.agents` clients should expose the same canonical skill under `.agents/skills/`.

Recommended files:

```text
.ai/skills/mivia-agent-workflows/SKILL.md
.agents/skills/mivia-agent-workflows/SKILL.md
.claude/skills/mivia-agent-workflows/SKILL.md
```

The `.ai` and `.agents` copies can be byte-identical. The Claude file can be a short pointer when the project already uses pointer-style Claude skills.

## Ship Criteria

A desktop integration is usable when:

- `mivia-agent adapters --repo . --json` reports the intended adapters.
- `mivia-agent run --repo . --workflow <name> --dry-run --json` resolves the producer, reviewer, model, and effort.
- Live workflow artifacts land only under `.ai/runs/`.
- Hooks mention the skill for workflow prompts but do not auto-run long workflows.
- A passing workflow is described as artifact acceptance, not merge or release approval.
