# ZCode Future Adapter Research

Date: 2026-07-05
Scope: whether ZCode should be added alongside Codex, Claude, and the existing adapter set as a real runtime adapter.

## Recommendation

Do not add ZCode as an `approved_for_run` or other orchestrable adapter yet.

Current official docs describe ZCode as a desktop Agent Development Environment with a first-party in-app agent, commands, skills, MCP configuration, safety-confirmation UI, and remote-control flows. They do not document a stable headless CLI/runtime contract comparable to the current Codex and Claude adapter boundary.

If ZCode support is needed before an official CLI contract exists, keep it in one of these lower-risk buckets:

- guidance-only documentation
- template/import guidance for `AGENTS.md`, skills, commands, or MCP setup
- a future research-gated adapter task, not an implementation task

## Evidence

### What the current docs do show

- ZCode is positioned as a desktop ADE with a first-party `ZCode Agent`, execution modes, workspace state, and in-app task flow, not as a documented external headless CLI runtime: [ZCode overview](https://zcode.z.ai/en/docs), [ZCode Agent](https://zcode.z.ai/en/docs/agents).
- Commands are saved prompts invoked inside the ZCode task UI via `/`, not a documented subprocess command surface: [Command](https://zcode.z.ai/en/docs/commands).
- Skills are managed as `SKILL.md` directories inside ZCode settings and chat, again as in-app agent features: [Skill](https://zcode.z.ai/en/docs/skill).
- MCP setup is documented through the ZCode settings UI. The docs emphasize importing external-agent MCP config from Claude Code, Codex CLI, OpenCode, and generic `.agents`, which indicates ZCode currently consumes those ecosystems rather than exposing the same runtime shape itself: [MCP Servers](https://zcode.z.ai/en/docs/mcp-services).
- Safety confirmation is described as a permission and confirmation workflow inside the product UI: [Safety Confirmation](https://zcode.z.ai/en/docs/safety-confirm).
- Remote development and ADE tooling are described as workspace features, not a standalone local runner contract: [Remote Development](https://zcode.z.ai/en/docs/remote-development), [ADE Tools](https://zcode.z.ai/en/docs/ADE-tools).

### What the current docs do not show

No official page was found that documents:

- a stable ZCode local binary name for adapter detection
- a `--version` contract suitable for `Detect`
- a headless `run` command that accepts prompt input non-interactively
- approval or sandbox flags suitable for fail-closed `Run`
- machine-readable stdout/stderr contract and exit-code mapping
- a headless `review` contract comparable to existing adapter `Review`

Under the current repo rules in [docs/adapter-authoring.md](../adapter-authoring.md), that missing contract is enough to keep ZCode out of the orchestrable runtime set for now.

## Future Adapter Gate

Promote ZCode into a real adapter only after official docs or a shipped binary surface prove all of the following:

1. Detectable local binary and version contract.
2. Headless non-interactive execution path.
3. Approval and permission settings that can be enforced by `mivia-agent`.
4. Structured output or another stable parseable result contract.
5. A review/verdict path, or an explicit decision to support `Run`-only first.
6. Real subprocess integration coverage through the same boundary used for Codex and Claude.

## Suggested Next Step

If we want ZCode represented sooner, add a docs-only follow-up that explains how ZCode can consume this repo's `AGENTS.md`, skills, commands, and MCP config without claiming runtime-adapter parity.
