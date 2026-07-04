# Codex Adapter

Root `AGENTS.md` is canonical. Read it first, then `.ai/INDEX.md`.

`.codex/hooks.json` declares no-op hook stubs until `mivia-agent` exists. Intended Codex event mapping:

- `UserPromptSubmit` -> `mivia-agent hook codex user-prompt-submit`
- `PreToolUse` -> `mivia-agent hook codex pre-tool-use`
- `PermissionRequest` -> `mivia-agent hook codex permission-request`
- `Stop` -> `mivia-agent hook codex stop`

Hook payloads are JSON on stdin. Handlers must read common fields such as `session_id`, `cwd`, `hook_event_name`, `permission_mode`, and event-specific fields such as `prompt`, `tool_name`, `tool_input`, or `last_assistant_message`. Handlers must return JSON decisions or exit 2 with a scrubbed reason when blocking.
