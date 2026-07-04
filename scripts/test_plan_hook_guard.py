#!/usr/bin/env python3
"""Contract tests for agent planning hook guard behavior."""

from __future__ import annotations

import json
import os
import subprocess
import sys
import tempfile
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
GUARD = ROOT / "scripts" / "plan_hook_guard.py"
RUNNER = ROOT / "scripts" / "run_agent_hook_guard.sh"


VALID_REPORT = """ReportFormat: mivia-agent-report/v1
Skill: agent-dag-planner
Result: PASS
Scope: docs/agent-planning.md
Baseline: HEAD
Summary: plan artifact validated

| ID | Severity | Status | File:Line | Finding | Required Fix | Required Test | Mutation |
| --- | --- | --- | --- | --- | --- | --- | --- |
| none | none | closed | none | none | none | none | none |

| Command | Result | Notes |
| --- | --- | --- |
| make plan-contract-test | PASS | none |

ResidualRisk: none
NextAction: none
PlanArtifact: .ai/plans/test-valid.plan.json
"""


def run_guard(agent: str, event: str, payload: dict[str, object], *, state_file: Path) -> subprocess.CompletedProcess[str]:
    env = os.environ.copy()
    env["MIVIA_PLAN_HOOK_STATE"] = str(state_file)
    return subprocess.run(
        [sys.executable, str(GUARD), agent, event],
        input=json.dumps(payload),
        cwd=ROOT,
        env=env,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        check=False,
    )


def write_valid_plan(path: Path) -> None:
    plan = {
        "PlanFormat": "mivia-agent-plan/v1",
        "plan_id": "test-valid",
        "version": 1,
        "baseline_commit": "HEAD",
        "scope": {"in": ["scripts"], "out": ["cmd"], "files": ["scripts/plan_hook_guard.py"]},
        "source_evidence": [{"path": "scripts/plan_hook_guard.py", "reason": "hook guard", "checked_at": "2026-07-04"}],
        "external_docs": [],
        "dag": {
            "nodes": [
                {
                    "id": "guard",
                    "title": "guard",
                    "skill": "agent-dag-planner",
                    "agent": "codex",
                    "depends_on": [],
                    "files_read": ["scripts/plan_hook_guard.py"],
                    "files_edit": ["scripts/plan_hook_guard.py"],
                    "allowed_mcp_tools": [],
                    "tests": ["scripts/test_plan_hook_guard.py::test_stop_allows_valid_planning_report"],
                    "verifiers": ["python3 scripts/test_plan_hook_guard.py"],
                    "mutation": "Remove PlanArtifact check; test must fail.",
                    "outputs": ["scripts/plan_hook_guard.py"],
                }
            ]
        },
        "gaps": [
            {
                "id": "none",
                "status": "closed",
                "severity": "none",
                "description": "none",
                "required_fix": "none",
                "required_test": "none",
            }
        ],
        "correction_log": [{"source": "test", "gap": "none", "correction": "none"}],
        "stop_conditions": ["python3 scripts/validate_agent_plan.py .ai/plans/test-valid.plan.json"],
    }
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(plan, indent=2) + "\n", encoding="utf-8")


def parse_stdout(proc: subprocess.CompletedProcess[str]) -> dict[str, object]:
    try:
        parsed = json.loads(proc.stdout or "{}")
    except json.JSONDecodeError as exc:
        raise AssertionError(f"stdout was not JSON: {proc.stdout!r}") from exc
    if not isinstance(parsed, dict):
        raise AssertionError(f"stdout JSON was not an object: {parsed!r}")
    return parsed


def assert_context(proc: subprocess.CompletedProcess[str], contains: str) -> None:
    if proc.returncode != 0:
        raise AssertionError(f"guard failed unexpectedly: {proc.stderr}")
    payload = parse_stdout(proc)
    context = str(payload.get("hookSpecificOutput", {}).get("additionalContext", ""))
    if contains not in context:
        raise AssertionError(f"context missing {contains!r}: {payload!r}")


def assert_continue(proc: subprocess.CompletedProcess[str], contains: str) -> None:
    if proc.returncode != 0:
        raise AssertionError(f"guard failed unexpectedly: {proc.stderr}")
    payload = parse_stdout(proc)
    if payload.get("decision") != "block":
        raise AssertionError(f"guard did not block stop: {payload!r}")
    if contains not in str(payload.get("reason", "")):
        raise AssertionError(f"reason missing {contains!r}: {payload!r}")


def test_planning_prompt_requires_machine_plan(state_file: Path) -> None:
    proc = run_guard(
        "codex",
        "user-prompt-submit",
        {
            "session_id": "s1",
            "hook_event_name": "UserPromptSubmit",
            "prompt": "create implementation plan and decompose into DAG for agents",
        },
        state_file=state_file,
    )
    assert_context(proc, "agent-dag-planner")
    assert_context(proc, "mivia-agent-plan/v1")


def test_implementation_prompt_requires_plan(state_file: Path) -> None:
    proc = run_guard(
        "codex",
        "user-prompt-submit",
        {
            "session_id": "s1",
            "hook_event_name": "UserPromptSubmit",
            "prompt": "implement this plan with hooks and bug audits",
        },
        state_file=state_file,
    )
    assert_context(proc, "agent-plan-implementer")
    assert_context(proc, "validated .ai/plans/*.plan.json")


def test_stop_blocks_planning_without_plan_artifact(state_file: Path) -> None:
    run_guard(
        "codex",
        "user-prompt-submit",
        {
            "session_id": "s1",
            "hook_event_name": "UserPromptSubmit",
            "prompt": "make an implementation plan",
        },
        state_file=state_file,
    )
    proc = run_guard(
        "codex",
        "stop",
        {
            "session_id": "s1",
            "hook_event_name": "Stop",
            "last_assistant_message": "ReportFormat: mivia-agent-report/v1\nSkill: agent-dag-planner\nResult: PASS\n",
        },
        state_file=state_file,
    )
    assert_continue(proc, "PlanArtifact")


def test_stop_blocks_implementer_without_plan_artifact(state_file: Path) -> None:
    run_guard(
        "codex",
        "user-prompt-submit",
        {
            "session_id": "s1",
            "hook_event_name": "UserPromptSubmit",
            "prompt": "implement this plan with hooks",
        },
        state_file=state_file,
    )
    proc = run_guard(
        "codex",
        "stop",
        {
            "session_id": "s1",
            "hook_event_name": "Stop",
            "last_assistant_message": "ReportFormat: mivia-agent-report/v1\nSkill: agent-plan-implementer\nResult: PASS\n",
        },
        state_file=state_file,
    )
    assert_continue(proc, "PlanArtifact")


def test_stop_allows_valid_planning_report(state_file: Path) -> None:
    plan_path = ROOT / ".ai" / "plans" / "test-valid.plan.json"
    write_valid_plan(plan_path)
    try:
        _test_stop_allows_valid_planning_report(state_file)
    finally:
        plan_path.unlink(missing_ok=True)


def _test_stop_allows_valid_planning_report(state_file: Path) -> None:
    run_guard(
        "codex",
        "user-prompt-submit",
        {
            "session_id": "s1",
            "hook_event_name": "UserPromptSubmit",
            "prompt": "make an implementation plan",
        },
        state_file=state_file,
    )
    proc = run_guard(
        "codex",
        "stop",
        {
            "session_id": "s1",
            "hook_event_name": "Stop",
            "last_assistant_message": VALID_REPORT,
        },
        state_file=state_file,
    )
    if proc.returncode != 0 or proc.stdout.strip():
        raise AssertionError(f"expected silent pass for valid planning report: stdout={proc.stdout!r} stderr={proc.stderr!r}")


def test_runner_applies_plan_guard_after_audit_guard(state_file: Path) -> None:
    env = os.environ.copy()
    env["MIVIA_PLAN_HOOK_STATE"] = str(state_file)
    proc = subprocess.run(
        [str(RUNNER), "codex", "user-prompt-submit"],
        input=json.dumps(
            {
                "session_id": "s1",
                "hook_event_name": "UserPromptSubmit",
                "prompt": "create a DAG implementation plan",
            }
        ),
        cwd=ROOT,
        env=env,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        check=False,
    )
    assert_context(proc, "agent-dag-planner")


def main() -> int:
    tests = [
        test_planning_prompt_requires_machine_plan,
        test_implementation_prompt_requires_plan,
        test_stop_blocks_planning_without_plan_artifact,
        test_stop_blocks_implementer_without_plan_artifact,
        test_stop_allows_valid_planning_report,
        test_runner_applies_plan_guard_after_audit_guard,
    ]
    for test in tests:
        with tempfile.TemporaryDirectory() as tmp:
            test(Path(tmp) / "plan-hook-state.json")
    print("plan hook guard tests passed")
    return 0


if __name__ == "__main__":
    sys.exit(main())
