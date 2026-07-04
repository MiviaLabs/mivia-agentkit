# Security And Privacy

## Secrets

- Never commit `.env`, credential files, API keys, tokens, private keys, provider payloads, raw prompts, or raw model outputs.
- Test fixtures and samples must use obviously fake values such as `example-token` or `test-secret-placeholder`.
- Do not log environment variables wholesale. Log explicit allowlisted names only.

## Network

- `mivia-agent` itself must not make network calls.
- Tests must not hit the network. Adapter behavior must use fake runners or temp local processes.
- Any future network-capable adapter must make the boundary explicit and scrub results before persistence.

## Hooks

- Codex and Claude hook handlers must parse JSON from stdin and reject malformed protected-action payloads once enforcement exists.
- Until `mivia-agent` exists, hook config must be safe no-op stubs.
- Hook output must not include raw prompt or model output. Return short decisions and scrubbed reasons only.

## Global Config

- Future product behavior reads `~/.agents/` as lowest-priority defaults and never writes it.
- Project `.ai/` rules and skills win over global rules and skills of the same name.
- User-level config is machine-local unless the user explicitly asks to sync or commit it elsewhere.
