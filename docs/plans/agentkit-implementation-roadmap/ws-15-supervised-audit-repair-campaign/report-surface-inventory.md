# WS15 Phase 0 — Claim-bearing report surface inventory

Last verified: 2026-07-17 against `40f6aa9` + working tree revalidation.

Purpose: list every surface that can emit or narrate time, token, cost, throughput, or efficiency claims so the telemetry contract can require runtime provenance or `NOT_MEASURED`.

## Canonical report / skill surfaces

| Path | Emits claims? | Notes |
| --- | --- | --- |
| `.ai/templates/agent-report-v1.md` | yes | Canonical Markdown report; must forbid unproven metrics |
| `.ai/skills/*/SKILL.md` | yes | Must use report v1; no invented efficiency numbers |
| `.agents/skills/*/SKILL.md` | thin | Delegates to canonical `.ai` skill/template |
| `.claude/skills/*/SKILL.md` | thin | Delegates to canonical `.ai` skill/template |
| `.ai/policy/audit-loop.json` | guidance | Report-only loop policy text |
| `scripts/audit_loop_guard.py` | parses report | Must not invent metrics; report-only continuation |
| `scripts/test_skill_contracts.py` | contract | Skill/report shape gates |
| `scripts/test_report_telemetry_contracts.py` | contract | Telemetry provenance gates |

## Workflow / dogfood surfaces

| Path | Emits claims? | Notes |
| --- | --- | --- |
| `.ai/workflows/*.yaml` | no metrics | Workflow bounds only |
| `.ai/workflows/prompts/*.tmpl` | possible | Prompt templates must not require invented numbers |
| `mivia-agent.yaml` | no metrics | Project manifest |

## Generated / embedded target-repo surfaces

| Path | Emits claims? | Notes |
| --- | --- | --- |
| `templates/**` | possible | Init/update outputs for target repos |
| `internal/templates/source/**` | possible | Embedded source of truth for templates |
| `internal/templates/templates.go` | registration | Must register campaign/report files in Phase 5 |

## Runtime / CLI / storage surfaces

| Path | Emits claims? | Notes |
| --- | --- | --- |
| `internal/cli/run.go` | outcome only | Must not invent elapsed/tokens without runtime records |
| `internal/report/report.go` | doctor/audit findings | Not agent-report v1 |
| `internal/runstore/runstore.go` | timestamps | Trace events; Phase 2 extends metrics |
| `internal/adapter/*.go` | ProviderMeta | Scrubbed provider meta only with provenance |
| future `internal/cli/campaign.go` | campaign reports | Phase 4; runtime-backed only |
| future `internal/auditcampaign/*` | evidence/metrics | Phase 2 |

## Docs / examples

| Path | Risk | Notes |
| --- | --- | --- |
| `docs/examples/**` | false commit claims | Phase 0 corrected to `protect:commit` semantics |
| `docs/config-examples.md` | false commit claims | Phase 0 corrected |
| `docs/loop-authoring.md` | incomplete protect docs | Phase 6 expands |
| `README.md` | false commit claims | Phase 0 corrected |

## Contract

- Agent prose is never trusted telemetry.
- Runtime-owned measurements only; otherwise render `NOT_MEASURED`.
- Enforced by:
  - `scripts/test_report_telemetry_contracts.py` (`make telemetry-contract-test`)
  - `scripts/git-hooks/pre-commit` and `scripts/git-hooks/pre-push` (both execute the telemetry contract)
  - `python3 scripts/verify_agent_config.py` (requires Measurement Rules, skill invent-metrics ban, and hook needles that run the telemetry contract)
- Generated target-repo template report surfaces remain deferred until Phase 5 parity.
