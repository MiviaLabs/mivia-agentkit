# Claude Code Adapter

Root `AGENTS.md` is canonical. Read it first, then read `.ai/INDEX.md` and the relevant `.ai/rules/*.md` files for the task.

Claude-specific notes:

- Before editing files, state the intended edits briefly. After editing, show or summarize the diff before finalizing.
- Run the relevant tests or validation commands after changes. If tests cannot run because no Go module or code exists yet, say that explicitly.
- Keep `.claude/settings.json` as the project settings and hook reference. Current hooks are no-op stubs until `mivia-agent` exists.
- Use `.claude/skills/` for Claude Code skill adapters. Canonical project workflows live under `.ai/skills/`.

Claude hook mapping:

- `PreToolUse` -> `mivia-agent hook claude pre-tool-use`
- `Stop` -> `mivia-agent hook claude stop`

Claude hook payloads arrive as JSON on stdin. Future handlers must inspect event name, tool name and arguments for `PreToolUse`, and stop-state fields for `Stop`; they must return a scrubbed decision without persisting raw prompts, raw tool input containing secrets, or raw model output.

Do not add policy here that conflicts with `AGENTS.md`.
