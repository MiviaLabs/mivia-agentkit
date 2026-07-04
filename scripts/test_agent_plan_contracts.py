#!/usr/bin/env python3
"""Contract tests for machine-readable agent DAG plans and planning skills."""

from __future__ import annotations

import json
import subprocess
import sys
import tempfile
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
VALIDATOR = ROOT / "scripts" / "validate_agent_plan.py"
SCHEMA = ROOT / ".ai" / "schemas" / "agent-plan-v1.schema.json"
TEMPLATE = ROOT / ".ai" / "templates" / "agent-plan-v1.json"


VALID_PLAN = {
    "PlanFormat": "mivia-agent-plan/v1",
    "plan_id": "agent-planning-contracts",
    "version": 1,
    "baseline_commit": "HEAD",
    "scope": {
        "in": ["scripts/validate_agent_plan.py"],
        "out": ["cmd/"],
        "files": ["scripts/validate_agent_plan.py", "scripts/test_agent_plan_contracts.py"],
    },
    "source_evidence": [
        {
            "path": "scripts/validate_agent_plan.py",
            "reason": "validator implementation",
            "checked_at": "2026-07-04",
        }
    ],
    "external_docs": [
        {
            "url": "https://developers.openai.com/codex/hooks",
            "reason": "Codex hook behavior",
            "checked_at": "2026-07-04",
        }
    ],
    "dag": {
        "nodes": [
            {
                "id": "plan",
                "title": "Create validated plan",
                "skill": "agent-dag-planner",
                "agent": "codex",
                "depends_on": [],
                "files_read": ["README.md"],
                "files_edit": ["docs/agent-planning.md"],
                "allowed_mcp_tools": ["openaiDeveloperDocs.search"],
                "tests": ["scripts/test_agent_plan_contracts.py::test_valid_plan_passes"],
                "verifiers": ["python3 scripts/validate_agent_plan.py <plan>"],
                "mutation": "Remove DAG cycle check; cycle test must fail.",
                "outputs": [".ai/plans/agent-planning-contracts.plan.json"],
            },
            {
                "id": "implement",
                "title": "Implement from plan",
                "skill": "agent-plan-implementer",
                "agent": "codex",
                "depends_on": ["plan"],
                "files_read": [".ai/plans/agent-planning-contracts.plan.json"],
                "files_edit": ["scripts/plan_hook_guard.py"],
                "allowed_mcp_tools": [],
                "tests": ["scripts/test_plan_hook_guard.py::test_implementation_prompt_requires_plan"],
                "verifiers": ["make plan-contract-test"],
                "mutation": "Remove active-plan requirement; implementation prompt test must fail.",
                "outputs": ["scripts/plan_hook_guard.py"],
            },
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
    "correction_log": [
        {
            "source": "docs/plans/00-overview.md",
            "gap": "human-only roadmap",
            "correction": "added machine-readable DAG contract",
        }
    ],
    "stop_conditions": [
        "python3 scripts/validate_agent_plan.py <plan>",
        "make plan-contract-test",
        "make verify",
    ],
}


def run_validator(plan: dict[str, object]) -> subprocess.CompletedProcess[str]:
    with tempfile.TemporaryDirectory() as tmp:
        path = Path(tmp) / "plan.json"
        path.write_text(json.dumps(plan, indent=2) + "\n", encoding="utf-8")
        return subprocess.run(
            [sys.executable, str(VALIDATOR), str(path)],
            cwd=ROOT,
            text=True,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            check=False,
        )


def assert_fails(plan: dict[str, object], contains: str) -> None:
    proc = run_validator(plan)
    if proc.returncode == 0:
        raise AssertionError("validator accepted invalid plan")
    if "Traceback" in proc.stderr or "agent plan validation failed:" not in proc.stderr:
        raise AssertionError(f"validator failed outside the validation path: {proc.stderr!r}")
    if contains not in proc.stderr:
        raise AssertionError(f"validator error missing {contains!r}: {proc.stderr!r}")


def test_valid_plan_passes() -> None:
    proc = run_validator(VALID_PLAN)
    if proc.returncode != 0:
        raise AssertionError(f"validator rejected valid plan: {proc.stderr}")
    if "agent plan validation passed" not in proc.stdout:
        raise AssertionError(f"unexpected validator stdout: {proc.stdout!r}")


def test_cycle_rejected() -> None:
    plan = json.loads(json.dumps(VALID_PLAN))
    plan["dag"]["nodes"][0]["depends_on"] = ["implement"]
    assert_fails(plan, "cycle")


def test_open_gap_rejected() -> None:
    plan = json.loads(json.dumps(VALID_PLAN))
    plan["gaps"] = [
        {
            "id": "PLAN-1",
            "status": "open",
            "severity": "low",
            "description": "missing verifier",
            "required_fix": "add verifier",
            "required_test": "scripts/test_agent_plan_contracts.py::test_open_gap_rejected",
        }
    ]
    assert_fails(plan, "open gap")


def test_missing_verifier_rejected() -> None:
    plan = json.loads(json.dumps(VALID_PLAN))
    plan["dag"]["nodes"][0]["verifiers"] = []
    assert_fails(plan, "verifier")


def test_missing_correction_log_rejected() -> None:
    plan = json.loads(json.dumps(VALID_PLAN))
    plan["correction_log"] = []
    assert_fails(plan, "correction_log")


def test_plan_contract_files_exist() -> None:
    for path in [VALIDATOR, SCHEMA, TEMPLATE]:
        if not path.is_file():
            raise AssertionError(f"{path.relative_to(ROOT)} is missing")


def test_planning_skills_and_docs_are_registered() -> None:
    required = [
        ".ai/skills/agent-dag-planner/SKILL.md",
        ".ai/skills/agent-plan-implementer/SKILL.md",
        ".agents/skills/agent-dag-planner/SKILL.md",
        ".agents/skills/agent-plan-implementer/SKILL.md",
        ".claude/skills/agent-dag-planner/SKILL.md",
        ".claude/skills/agent-plan-implementer/SKILL.md",
        "docs/agent-planning.md",
    ]
    for rel in required:
        if not (ROOT / rel).is_file():
            raise AssertionError(f"{rel} is missing")

    registry = json.loads((ROOT / ".agents" / "skills.json").read_text(encoding="utf-8"))
    paths = {item["path"] for item in registry["skills"]}
    for rel in required[:6]:
        if rel not in paths:
            raise AssertionError(f".agents/skills.json missing {rel}")

    readme = (ROOT / "README.md").read_text(encoding="utf-8")
    if "[Agent planning](docs/agent-planning.md)" not in readme:
        raise AssertionError("README Docs TOC missing Agent planning link")


def main() -> int:
    test_valid_plan_passes()
    test_cycle_rejected()
    test_open_gap_rejected()
    test_missing_verifier_rejected()
    test_missing_correction_log_rejected()
    test_plan_contract_files_exist()
    test_planning_skills_and_docs_are_registered()
    print("agent plan contract tests passed")
    return 0


if __name__ == "__main__":
    sys.exit(main())
