# Claude Code Adapter

Root `AGENTS.md` is canonical. Read it first, then read `.ai/INDEX.md` and the relevant `.ai/rules/*.md` files for the task.

Claude-specific notes:

- Before editing files, state the intended edits briefly. After editing, show or summarize the diff before finalizing.
- Run the relevant tests or validation commands after changes. If tests cannot run because no Go module or code exists yet, say that explicitly.
- Keep `.claude/settings.json` as the project settings and hook reference. Current hooks delegate through `scripts/run_agent_hook_guard.sh`, which applies the repo guard before any future `mivia-agent` handler.
- Use `.claude/skills/` for Claude Code skill adapters. Canonical project workflows live under `.ai/skills/`.

Claude hook mapping:

- `UserPromptSubmit` -> `scripts/run_agent_hook_guard.sh claude user-prompt-submit`
- `PreToolUse` -> `scripts/run_agent_hook_guard.sh claude pre-tool-use`
- `PermissionRequest` -> `scripts/run_agent_hook_guard.sh claude permission-request`
- `Stop` -> `scripts/run_agent_hook_guard.sh claude stop`

Claude hook payloads arrive as JSON on stdin. Handlers must inspect event name, tool name and arguments for `PreToolUse` and `PermissionRequest`, prompt text for `UserPromptSubmit`, and stop-state fields for `Stop`; they must return a scrubbed decision without persisting raw prompts, raw tool input containing secrets, or raw model output.

Do not add policy here that conflicts with `AGENTS.md`.
