# AgentKit Implementation Roadmap

ReportFormat: mivia-agent-report/v1
Skill: agent-dag-planner
Result: PASS
Scope: AGENTS.md, .ai/rules/20-agent-quality.md, docs/plans/agentkit-implementation-roadmap/**, .ai/plans/agentkit-implementation-roadmap.plan.json, semgrep/agent-standards.yml, scripts/test_semgrep_rules.py, docs/adapter-authoring.md
Baseline: 83e9ba1 plus current working-tree revalidation
Summary: The roadmap and repo policy now require real runtime coverage for every implemented command and approved-for-run adapter, and Phase 5 closure is reopened until WS14 lands.

| ID | Severity | Status | File:Line | Finding | Required Fix | Required Test | Mutation |
| --- | --- | --- | --- | --- | --- | --- | --- |
| PLAN-REAL-COVERAGE | high | closed | AGENTS.md:45 | Canonical repo guidance and roadmap closure still allowed fake-only tests to stand in for shipped command and adapter proof. | Update canonical rules, add a dedicated real-runtime-coverage workstream, reopen WS8 behind it, and add static policy coverage that rejects stale fake-only guidance. | `python3 scripts/validate_agent_plan.py .ai/plans/agentkit-implementation-roadmap.plan.json`; `python3 scripts/test_agent_plan_contracts.py`; `python3 scripts/test_semgrep_rules.py`; `python3 scripts/verify_agent_config.py` | Reintroduce the stale fake-only guidance or remove WS14 from the DAG; validators or Semgrep contract tests must fail. |

| Command | Result | Notes |
| --- | --- | --- |
| python3 scripts/validate_agent_plan.py .ai/plans/agentkit-implementation-roadmap.plan.json | PASS | Machine DAG validates with WS14 and reopened WS8 dependency closure. |
| python3 scripts/test_agent_plan_contracts.py | PASS | Plan artifact and roadmap contract tests pass. |
| python3 scripts/test_plan_hook_guard.py | PASS | Planner Stop-hook PlanArtifact contract still passes. |
| python3 scripts/test_semgrep_rules.py | PASS | Static policy now rejects stale fake-only guidance and fake-runner use in real integration test files. |
| python3 scripts/verify_agent_config.py | PASS | Repo config and doc references still validate after the roadmap/rule updates. |
| mutation: reintroduce fake-only closure guidance | PASS | Semgrep contract fixtures failed until the mutation was reverted. |

ResidualRisk: WS14 is planned but not implemented; current repo coverage still relies on existing fake-runner-heavy suites until that workstream lands.
NextAction: Implement the next dependency-ready node, `docs/plans/agentkit-implementation-roadmap/ws-14-real-runtime-coverage/tasks.md`.
PlanArtifact: .ai/plans/agentkit-implementation-roadmap.plan.json
