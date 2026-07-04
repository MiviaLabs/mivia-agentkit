#!/usr/bin/env python3
"""Validate mivia-agent-plan/v1 artifacts."""

from __future__ import annotations

import json
import sys
from pathlib import Path
from typing import Any


PLAN_FORMAT = "mivia-agent-plan/v1"
GAP_STATUSES = {"open", "missing", "shallow", "gated"}
REQUIRED_TOP_LEVEL = [
    "PlanFormat",
    "plan_id",
    "version",
    "baseline_commit",
    "scope",
    "source_evidence",
    "external_docs",
    "dag",
    "gaps",
    "correction_log",
    "stop_conditions",
]
REQUIRED_NODE_FIELDS = [
    "id",
    "title",
    "skill",
    "agent",
    "depends_on",
    "files_read",
    "files_edit",
    "allowed_mcp_tools",
    "tests",
    "verifiers",
    "mutation",
    "outputs",
]


class ValidationError(ValueError):
    """Plan validation failed."""


def require(condition: bool, message: str) -> None:
    if not condition:
        raise ValidationError(message)


def require_non_empty_list(value: Any, name: str) -> list[Any]:
    require(isinstance(value, list), f"{name} must be a list")
    require(bool(value), f"{name} must not be empty")
    return value


def require_string_list(value: Any, name: str, *, allow_empty: bool = False) -> list[str]:
    require(isinstance(value, list), f"{name} must be a list")
    if not allow_empty:
        require(bool(value), f"{name} must not be empty")
    for item in value:
        require(isinstance(item, str), f"{name} entries must be strings")
    return value


def validate_scope(plan: dict[str, Any]) -> None:
    scope = plan.get("scope")
    require(isinstance(scope, dict), "scope must be an object")
    for key in ["in", "out", "files"]:
        require_string_list(scope.get(key), f"scope.{key}")


def validate_evidence(plan: dict[str, Any]) -> None:
    for field in ["source_evidence", "correction_log"]:
        rows = require_non_empty_list(plan.get(field), field)
        for index, row in enumerate(rows):
            require(isinstance(row, dict), f"{field}[{index}] must be an object")
    external_docs = plan.get("external_docs")
    require(isinstance(external_docs, list), "external_docs must be a list")
    for index, row in enumerate(external_docs):
        require(isinstance(row, dict), f"external_docs[{index}] must be an object")
        require(isinstance(row.get("url"), str) and row["url"], f"external_docs[{index}].url is required")
        require(isinstance(row.get("checked_at"), str) and row["checked_at"], f"external_docs[{index}].checked_at is required")


def validate_gaps(plan: dict[str, Any]) -> None:
    gaps = require_non_empty_list(plan.get("gaps"), "gaps")
    for index, gap in enumerate(gaps):
        require(isinstance(gap, dict), f"gaps[{index}] must be an object")
        status = str(gap.get("status", "")).lower()
        gap_id = str(gap.get("id", f"#{index}"))
        require(status, f"gaps[{index}].status is required")
        require(status not in GAP_STATUSES, f"open gap {gap_id} has status {status}")
        for key in ["severity", "description", "required_fix", "required_test"]:
            require(isinstance(gap.get(key), str) and gap[key], f"gaps[{index}].{key} is required")


def validate_dag(plan: dict[str, Any]) -> None:
    dag = plan.get("dag")
    require(isinstance(dag, dict), "dag must be an object")
    nodes = require_non_empty_list(dag.get("nodes"), "dag.nodes")
    seen: set[str] = set()
    by_id: dict[str, dict[str, Any]] = {}
    for index, node in enumerate(nodes):
        require(isinstance(node, dict), f"dag.nodes[{index}] must be an object")
        for field in REQUIRED_NODE_FIELDS:
            require(field in node, f"dag.nodes[{index}].{field} is required")
        node_id = node["id"]
        require(isinstance(node_id, str) and node_id, f"dag.nodes[{index}].id is required")
        require(node_id not in seen, f"duplicate node id {node_id}")
        seen.add(node_id)
        by_id[node_id] = node
        require_string_list(node["depends_on"], f"node {node_id}.depends_on", allow_empty=True)
        require_string_list(node["files_read"], f"node {node_id}.files_read")
        require_string_list(node["files_edit"], f"node {node_id}.files_edit")
        require_string_list(node["allowed_mcp_tools"], f"node {node_id}.allowed_mcp_tools", allow_empty=True)
        require_string_list(node["tests"], f"node {node_id}.tests")
        require_string_list(node["verifiers"], f"node {node_id}.verifiers")
        require_string_list(node["outputs"], f"node {node_id}.outputs")
        require(isinstance(node["mutation"], str) and node["mutation"], f"node {node_id}.mutation is required")
        require(isinstance(node["skill"], str) and node["skill"], f"node {node_id}.skill is required")
        require(isinstance(node["agent"], str) and node["agent"], f"node {node_id}.agent is required")

    for node_id, node in by_id.items():
        for dep in node["depends_on"]:
            require(dep in by_id, f"node {node_id} depends on unknown node {dep}")

    visiting: set[str] = set()
    visited: set[str] = set()

    def visit(node_id: str) -> None:
        if node_id in visited:
            return
        require(node_id not in visiting, f"cycle detected at node {node_id}")
        visiting.add(node_id)
        for dep in by_id[node_id]["depends_on"]:
            visit(dep)
        visiting.remove(node_id)
        visited.add(node_id)

    for node_id in by_id:
        visit(node_id)


def validate_plan(plan: dict[str, Any]) -> None:
    for field in REQUIRED_TOP_LEVEL:
        require(field in plan, f"{field} is required")
    require(plan["PlanFormat"] == PLAN_FORMAT, f"PlanFormat must be {PLAN_FORMAT}")
    require(plan["version"] == 1, "version must be 1")
    require(isinstance(plan["plan_id"], str) and plan["plan_id"], "plan_id is required")
    require(isinstance(plan["baseline_commit"], str) and plan["baseline_commit"], "baseline_commit is required")
    validate_scope(plan)
    validate_evidence(plan)
    validate_dag(plan)
    validate_gaps(plan)
    require_string_list(plan["stop_conditions"], "stop_conditions")


def main(argv: list[str]) -> int:
    if len(argv) != 2:
        print("usage: validate_agent_plan.py <plan.json>", file=sys.stderr)
        return 2
    path = Path(argv[1])
    try:
        parsed = json.loads(path.read_text(encoding="utf-8"))
        require(isinstance(parsed, dict), "plan must be a JSON object")
        validate_plan(parsed)
    except (OSError, json.JSONDecodeError, ValidationError) as exc:
        print(f"agent plan validation failed: {exc}", file=sys.stderr)
        return 1
    print("agent plan validation passed")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
