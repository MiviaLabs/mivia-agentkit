# AgentKit Implementation Roadmap

ReportFormat: mivia-agent-report/v1
Skill: agent-dag-planner
Result: PASS
Scope: docs/plans/agentkit-implementation-roadmap/**, .ai/plans/agentkit-implementation-roadmap.plan.json, docs/agent-planning.md, .ai/skills/agent-dag-planner/SKILL.md, scripts/validate_agent_plan.py
Baseline: dev/97e6b4c plus current working-tree revalidation
Summary: The roadmap DAG now requires an explicit task directory for every node and validates the existing AgentKit workstream directories.

| ID | Severity | Status | File:Line | Finding | Required Fix | Required Test | Mutation |
| --- | --- | --- | --- | --- | --- | --- | --- |
| PLAN-TASK-DIR | high | closed | docs/agent-planning.md:3 | Planning docs and the machine DAG did not require a task directory per DAG node. | Added `task_dir` to the plan template, validator, schema, skill, AgentKit machine plan, and contract tests. | `python3 scripts/test_agent_plan_contracts.py` | Remove `task_dir` from one node; validator and contract tests must fail. |

| Command | Result | Notes |
| --- | --- | --- |
| python3 scripts/validate_agent_plan.py .ai/plans/agentkit-implementation-roadmap.plan.json | PASS | Machine DAG validates. |
| python3 scripts/test_agent_plan_contracts.py | PASS | Roadmap task-directory and plan artifact contracts pass. |
| python3 scripts/test_plan_hook_guard.py | PASS | PlanArtifact Stop-hook contract passes. |
| python3 scripts/verify_agent_config.py | PASS | README and agent-config references pass. |
| mutation: remove machine-plan task_dir | PASS | Plan validator rejected missing node task_dir, then mutation was reverted. |

ResidualRisk: none
NextAction: Implement the next dependency-ready node, `docs/plans/agentkit-implementation-roadmap/ws-11-consensus/tasks.md`.
PlanArtifact: .ai/plans/agentkit-implementation-roadmap.plan.json
