#!/usr/bin/env python3
"""Contract tests for machine-readable agent DAG plans and planning skills."""

from __future__ import annotations

import json
import re
import subprocess
import sys
import tempfile
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
VALIDATOR = ROOT / "scripts" / "validate_agent_plan.py"
SCHEMA = ROOT / ".ai" / "schemas" / "agent-plan-v1.schema.json"
TEMPLATE = ROOT / ".ai" / "templates" / "agent-plan-v1.json"
ROADMAP_PLAN_ROOT = ROOT / "docs" / "plans" / "agentkit-implementation-roadmap"
MACHINE_PLAN_ROOT = ROOT / ".ai" / "plans"
AGENTKIT_PLAN = MACHINE_PLAN_ROOT / "agentkit-implementation-roadmap.plan.json"
EXPECTED_ROADMAP_PLAN_FILES = [
    "00-overview.md",
    "_conventions.md",
    "ws-00-bootstrap/tasks.md",
    "ws-01-manifest-git-pathpolicy/tasks.md",
    "ws-02-templates-init/tasks.md",
    "ws-03-doctor-audit/tasks.md",
    "ws-04-preflight-stamp/tasks.md",
    "ws-05-hooks/tasks.md",
    "ws-06-adapter-templates/tasks.md",
    "ws-07-import-update/tasks.md",
    "ws-08-ci-release-docs/tasks.md",
    "ws-09-adapters/tasks.md",
    "ws-10-orchestrator/tasks.md",
    "ws-11-consensus/tasks.md",
    "ws-12-governance/tasks.md",
    "ws-13-run-review-adapters/tasks.md",
]

MARKDOWN_LINK = re.compile(r"\[[^\]]+\]\(([^)\n]+)\)")


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
                "task_dir": "docs/plans/agent-planning-contracts/plan/",
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
                "task_dir": "docs/plans/agent-planning-contracts/implement/",
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
            "source": "docs/plans/agentkit-implementation-roadmap/00-overview.md",
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


def run_validator_file(path: Path) -> subprocess.CompletedProcess[str]:
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


def test_missing_task_dir_rejected() -> None:
    plan = json.loads(json.dumps(VALID_PLAN))
    del plan["dag"]["nodes"][0]["task_dir"]
    assert_fails(plan, "task_dir")


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


def test_roadmap_files_moved_under_named_root() -> None:
    for rel in EXPECTED_ROADMAP_PLAN_FILES:
        moved = ROADMAP_PLAN_ROOT / rel
        if not moved.is_file():
            raise AssertionError(f"moved roadmap file missing: {moved.relative_to(ROOT)}")
        old = ROOT / "docs" / "plans" / rel
        if old.exists():
            raise AssertionError(f"roadmap file still exists at old path: {old.relative_to(ROOT)}")


def test_readme_docs_toc_points_to_moved_roadmap() -> None:
    readme = (ROOT / "README.md").read_text(encoding="utf-8")
    stale = "[Workstream roadmap](docs/plans/00-overview.md)"
    if stale in readme:
        raise AssertionError("README Docs TOC still points to the old workstream roadmap path")


def test_planning_markdown_links_resolve() -> None:
    docs = [
        ROOT / "README.md",
        ROOT / ".ai" / "INDEX.md",
        ROOT / "docs" / "agent-planning.md",
    ]
    docs.extend(sorted(ROADMAP_PLAN_ROOT.rglob("*.md")))

    for path in docs:
        if not path.is_file():
            raise AssertionError(f"planning doc missing: {path.relative_to(ROOT)}")
        content = path.read_text(encoding="utf-8")
        for raw_target in MARKDOWN_LINK.findall(content):
            target = raw_target.strip().strip("<>")
            if not target or target.startswith(("#", "http://", "https://", "mailto:")):
                continue
            target = target.split("#", 1)[0]
            if not target:
                continue
            resolved = (path.parent / target).resolve()
            if not resolved.exists():
                raise AssertionError(
                    f"{path.relative_to(ROOT)} has broken markdown link {raw_target!r} -> {resolved}"
                )


def test_committed_machine_plan_artifacts_are_real_and_validated() -> None:
    plans = sorted(MACHINE_PLAN_ROOT.glob("*.plan.json"))
    if not plans:
        raise AssertionError(".ai/plans must contain at least one real validated plan artifact")

    forbidden_markers = [
        "<short-stable-id>",
        "<repo path>",
        "<plan>",
        "test-valid",
        "agent-planning-contracts",
    ]
    for path in plans:
        raw = path.read_text(encoding="utf-8")
        lowered = raw.lower()
        for marker in forbidden_markers:
            if marker in lowered:
                raise AssertionError(f"{path.relative_to(ROOT)} contains placeholder marker {marker!r}")
        parsed = json.loads(raw)
        if parsed.get("PlanFormat") != "mivia-agent-plan/v1":
            raise AssertionError(f"{path.relative_to(ROOT)} is not mivia-agent-plan/v1")
        expected_name = f"{parsed.get('plan_id')}.plan.json"
        if path.name != expected_name:
            raise AssertionError(f"{path.relative_to(ROOT)} name does not match plan_id {parsed.get('plan_id')!r}")
        proc = run_validator_file(path)
        if proc.returncode != 0:
            raise AssertionError(f"validator rejected {path.relative_to(ROOT)}: {proc.stderr}")


def test_agentkit_implementation_plan_is_named_and_referenced() -> None:
    if not AGENTKIT_PLAN.is_file():
        raise AssertionError("missing .ai/plans/agentkit-implementation-roadmap.plan.json")
    if not ROADMAP_PLAN_ROOT.is_dir():
        raise AssertionError("missing docs/plans/agentkit-implementation-roadmap/")

    parsed = json.loads(AGENTKIT_PLAN.read_text(encoding="utf-8"))
    if parsed.get("plan_id") != "agentkit-implementation-roadmap":
        raise AssertionError("AgentKit machine plan is not named for the implementation roadmap")
    if parsed.get("gaps") != [
        {
            "id": "none",
            "status": "closed",
            "severity": "none",
            "description": "none",
            "required_fix": "none",
            "required_test": "none",
        }
    ]:
        raise AssertionError("AgentKit machine plan must report zero open gaps")


def test_agentkit_plan_nodes_have_existing_task_dirs() -> None:
    parsed = json.loads(AGENTKIT_PLAN.read_text(encoding="utf-8"))
    for node in parsed["dag"]["nodes"]:
        task_dir = ROOT / node["task_dir"]
        if not task_dir.is_dir():
            raise AssertionError(f"node {node['id']} task_dir missing: {node['task_dir']}")
        if not (task_dir / "tasks.md").is_file():
            raise AssertionError(f"node {node['id']} task_dir has no tasks.md: {node['task_dir']}")


def main() -> int:
    test_valid_plan_passes()
    test_cycle_rejected()
    test_open_gap_rejected()
    test_missing_verifier_rejected()
    test_missing_task_dir_rejected()
    test_missing_correction_log_rejected()
    test_plan_contract_files_exist()
    test_planning_skills_and_docs_are_registered()
    test_roadmap_files_moved_under_named_root()
    test_readme_docs_toc_points_to_moved_roadmap()
    test_planning_markdown_links_resolve()
    test_committed_machine_plan_artifacts_are_real_and_validated()
    test_agentkit_implementation_plan_is_named_and_referenced()
    test_agentkit_plan_nodes_have_existing_task_dirs()
    print("agent plan contract tests passed")
    return 0


if __name__ == "__main__":
    sys.exit(main())
