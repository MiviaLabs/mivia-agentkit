# WS-F — Audit Review

## T1 — Deep bug audit and verifier closure

Create:
- `docs/plans/model-effort-config-implementation/ws-f-audit-review/tasks.md` — audit checklist and closure record.
- `docs/plans/model-effort-config-implementation.md` — update if audit changes scope or residual risk.

Spec:
- Audit the cumulative implementation delta across config, adapter, orchestrator, CLI, templates, and docs.
- Re-run the node-specific load-bearing mutations before final closure.
- Close only with zero open gaps and `ResidualRisk: none`.

Tests that must pass:
- `TestManifestRejectsUnknownEffort`
- `TestStepOverrideWinsOverAdapterDefault`
- `TestCodexRunPassesReasoningEffortOverride`
- `TestClaudeRunPassesEffortFlag`
- `TestRunDryRunPrintsModelAndEffort`

Dependencies:
- `internal/config`
- `internal/adapter`
- `internal/orchestrator`
- `internal/cli`
- `internal/templates`

Mutation proof:
- Re-run the prior node mutations; at least one named regression test per guard must fail before revert.
