#!/usr/bin/env python3
"""Stop-hook controller for structured audit loops."""

from __future__ import annotations

import hashlib
import json
import os
import re
import subprocess
import sys
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[1]
POLICY_PATH = ROOT / ".ai" / "policy" / "audit-loop.json"
EVENT_NAMES = {
    "user-prompt-submit": "UserPromptSubmit",
    "pre-tool-use": "PreToolUse",
    "permission-request": "PermissionRequest",
    "stop": "Stop",
}
SUPPORTED_AGENTS = {"agents", "claude", "codex"}


class ReportParseError(ValueError):
    """Raised when a candidate audit report is malformed."""


def event_name(raw: str, payload: dict[str, Any] | None = None) -> str:
    payload_event = payload.get("hook_event_name") if payload else None
    if isinstance(payload_event, str) and payload_event:
        return payload_event
    return EVENT_NAMES.get(raw, raw)


def load_policy() -> dict[str, Any]:
    try:
        policy = json.loads(POLICY_PATH.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as exc:
        raise ValueError(f"invalid audit loop policy: {exc}") from exc
    if not isinstance(policy, dict) or policy.get("version") != 1:
        raise ValueError("invalid audit loop policy: expected version 1 object")
    for key in ["reportFormat", "skills", "promptTriggers", "gapStatuses", "instruction"]:
        if key not in policy:
            raise ValueError(f"invalid audit loop policy: missing {key}")
    for key in ["maxIterations", "cleanReportsToStop", "maxMalformedReports"]:
        value = policy.get(key)
        if not isinstance(value, int) or value < 1:
            raise ValueError(f"invalid audit loop policy: {key} must be a positive integer")
    host_caps = policy.get("hostContinuationCaps", {})
    if not isinstance(host_caps, dict):
        raise ValueError("invalid audit loop policy: hostContinuationCaps must be an object")
    for host, value in host_caps.items():
        if not isinstance(host, str) or not isinstance(value, int) or value < 1:
            raise ValueError("invalid audit loop policy: hostContinuationCaps entries must be positive integers")
    return policy


def state_path() -> Path:
    override = os.environ.get("MIVIA_AUDIT_LOOP_STATE")
    if override:
        return Path(override)
    proc = subprocess.run(
        ["git", "rev-parse", "--git-path", "mivia-agent-audit-loop-state.json"],
        cwd=ROOT,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.DEVNULL,
        check=False,
    )
    if proc.returncode == 0 and proc.stdout.strip():
        return ROOT / proc.stdout.strip()
    return ROOT / ".git" / "mivia-agent-audit-loop-state.json"


def load_state(path: Path) -> dict[str, Any]:
    if not path.exists():
        return {"version": 1, "sessions": {}}
    try:
        state = json.loads(path.read_text(encoding="utf-8"))
    except json.JSONDecodeError:
        return {"version": 1, "sessions": {}}
    if not isinstance(state, dict) or not isinstance(state.get("sessions"), dict):
        return {"version": 1, "sessions": {}}
    return state


def save_state(path: Path, state: dict[str, Any]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(state, sort_keys=True, separators=(",", ":")) + "\n", encoding="utf-8")


def session_key(payload: dict[str, Any]) -> str:
    raw = f"{payload.get('session_id', '')}|{payload.get('cwd', '')}"
    return hashlib.sha256(raw.encode("utf-8")).hexdigest()[:24]


def active_record(state: dict[str, Any], payload: dict[str, Any]) -> dict[str, Any]:
    sessions = state.setdefault("sessions", {})
    assert isinstance(sessions, dict)
    key = session_key(payload)
    record = sessions.setdefault(key, {})
    if not isinstance(record, dict):
        record = {}
        sessions[key] = record
    return record


def clear_record(state: dict[str, Any], payload: dict[str, Any]) -> None:
    sessions = state.setdefault("sessions", {})
    if isinstance(sessions, dict):
        sessions.pop(session_key(payload), None)


def prompt_matches(prompt: str, policy: dict[str, Any]) -> bool:
    lowered = prompt.lower()
    triggers = policy.get("promptTriggers", [])
    return isinstance(triggers, list) and any(isinstance(item, str) and item.lower() in lowered for item in triggers)


def emit_context(event: str, message: str) -> int:
    print(
        json.dumps(
            {
                "hookSpecificOutput": {
                    "hookEventName": event,
                    "additionalContext": message,
                }
            },
            separators=(",", ":"),
        )
    )
    return 0


def emit_continue(reason: str) -> int:
    print(json.dumps({"decision": "block", "reason": reason}, separators=(",", ":")))
    return 0


def max_iterations_for_agent(agent: str, policy: dict[str, Any]) -> int:
    max_iterations = int(policy["maxIterations"])
    host_caps = policy.get("hostContinuationCaps", {})
    if isinstance(host_caps, dict) and isinstance(host_caps.get(agent), int):
        return min(max_iterations, int(host_caps[agent]))
    return max_iterations


def split_row(line: str) -> list[str]:
    return [cell.strip() for cell in line.strip().strip("|").split("|")]


def field_value(text: str, field: str) -> str | None:
    match = re.search(rf"^{re.escape(field)}:\s*(.+?)\s*$", text, flags=re.MULTILINE)
    return match.group(1).strip() if match else None


def parse_report(text: str, policy: dict[str, Any]) -> dict[str, Any] | None:
    report_format = str(policy["reportFormat"])
    if report_format not in text:
        return None

    skill = field_value(text, "Skill")
    if not skill:
        raise ReportParseError("missing Skill field")
    skills = policy.get("skills", [])
    if not isinstance(skills, list) or skill not in skills:
        return None

    result = field_value(text, "Result")
    residual = field_value(text, "ResidualRisk")
    if not result or residual is None:
        raise ReportParseError("missing Result or ResidualRisk field")

    header = "| ID | Severity | Status | File:Line | Finding | Required Fix | Required Test | Mutation |"
    lines = text.splitlines()
    try:
        start = next(index for index, line in enumerate(lines) if line.strip() == header)
    except StopIteration as exc:
        raise ReportParseError("missing finding table") from exc

    rows: list[list[str]] = []
    for line in lines[start + 2 :]:
        stripped = line.strip()
        if not stripped:
            if rows:
                break
            continue
        if stripped == "| Command | Result | Notes |":
            break
        if not stripped.startswith("|"):
            break
        cells = split_row(stripped)
        if len(cells) >= 8:
            rows.append(cells[:8])

    if not rows:
        raise ReportParseError("finding table has no rows")

    gap_statuses = {str(item).lower() for item in policy.get("gapStatuses", [])}
    gap_rows = []
    for cells in rows:
        finding_id, severity, status, file_line, finding, required_fix, required_test, mutation = cells
        if finding_id.lower() == "none":
            continue
        if status.lower() in gap_statuses:
            gap_rows.append(
                {
                    "severity": severity.lower(),
                    "status": status.lower(),
                    "file_line": file_line,
                    "has_fix": required_fix.lower() != "none",
                    "has_test": required_test.lower() != "none",
                    "has_mutation": mutation.lower() != "none",
                    "finding_hash": hashlib.sha256(
                        f"{file_line}|{finding}|{required_fix}|{required_test}|{mutation}".encode("utf-8")
                    ).hexdigest()[:12],
                }
            )

    residual_gap = residual.strip().lower() != "none"
    return {
        "skill": skill,
        "result": result,
        "gap_count": len(gap_rows) + (1 if residual_gap else 0),
        "row_count": len(rows),
        "residual_gap": residual_gap,
        "gap_hashes": [row["finding_hash"] for row in gap_rows],
    }


def handle_prompt(
    agent: str,
    payload: dict[str, Any],
    policy: dict[str, Any],
    state: dict[str, Any],
    path: Path,
) -> int:
    prompt = payload.get("prompt", "")
    if not isinstance(prompt, str) or not prompt_matches(prompt, policy):
        return 0

    record = active_record(state, payload)
    record.clear()
    record.update({"active": True, "iterations": 0, "clean_streak": 0, "malformed": 0})
    save_state(path, state)
    max_iterations = max_iterations_for_agent(agent, policy)
    return emit_context(
        "UserPromptSubmit",
        f"{policy['instruction']} Stop after {policy['cleanReportsToStop']} consecutive clean reports or {max_iterations} total audit reports on this agent surface.",
    )


def handle_stop(
    agent: str,
    payload: dict[str, Any],
    policy: dict[str, Any],
    state: dict[str, Any],
    path: Path,
) -> int:
    message = payload.get("last_assistant_message", "")
    if not isinstance(message, str) or not message.strip():
        return 0

    record = active_record(state, payload)
    was_active = bool(record.get("active"))
    try:
        report = parse_report(message, policy)
    except ReportParseError as exc:
        if not was_active and str(policy["reportFormat"]) not in message:
            return 0
        malformed = int(record.get("malformed", 0)) + 1
        record.update({"active": True, "malformed": malformed})
        save_state(path, state)
        if malformed <= int(policy["maxMalformedReports"]):
            return emit_continue(
                f"Audit loop report malformed ({exc}). Re-emit exact mivia-agent-report/v1 and continue fixing all gaps."
            )
        clear_record(state, payload)
        save_state(path, state)
        return 0

    if report is None:
        if not was_active:
            return 0
        malformed = int(record.get("malformed", 0)) + 1
        record.update({"active": True, "malformed": malformed})
        save_state(path, state)
        if malformed <= int(policy["maxMalformedReports"]):
            return emit_continue("Audit loop requires mivia-agent-report/v1. Re-emit the structured report.")
        clear_record(state, payload)
        save_state(path, state)
        return 0

    iterations = int(record.get("iterations", 0)) + 1
    max_iterations = max_iterations_for_agent(agent, policy)
    gap_count = int(report["gap_count"])
    clean_streak = int(record.get("clean_streak", 0))
    if gap_count == 0:
        clean_streak += 1
    else:
        clean_streak = 0

    record.update(
        {
            "active": True,
            "iterations": iterations,
            "clean_streak": clean_streak,
            "malformed": 0,
            "skill": report["skill"],
            "last_gap_count": gap_count,
            "last_gap_hashes": report["gap_hashes"],
        }
    )
    save_state(path, state)

    if iterations >= max_iterations:
        clear_record(state, payload)
        save_state(path, state)
        return 0

    clean_target = int(policy["cleanReportsToStop"])
    if gap_count > 0:
        plural = "gap" if gap_count == 1 else "gaps"
        return emit_continue(
            f"Audit loop round {iterations}/{max_iterations}: {gap_count} {plural} remain. Fix every gap regardless of severity, then emit mivia-agent-report/v1 again."
        )

    if clean_streak < clean_target:
        return emit_continue(
            f"Audit loop round {iterations}/{max_iterations}: clean report {clean_streak}/{clean_target}. Run one more independent pass to confirm zero gaps."
        )

    clear_record(state, payload)
    save_state(path, state)
    return 0


def main(argv: list[str]) -> int:
    if len(argv) != 3:
        print("usage: audit_loop_guard.py <agents|claude|codex> <hook-event>", file=sys.stderr)
        return 2
    agent = argv[1]
    event_arg = argv[2]
    if agent not in SUPPORTED_AGENTS:
        print(f"unsupported agent surface: {agent}", file=sys.stderr)
        return 2

    try:
        payload = json.load(sys.stdin)
        policy = load_policy()
    except (json.JSONDecodeError, ValueError) as exc:
        print(f"audit loop guard ignored malformed input: {exc}", file=sys.stderr)
        return 0
    if not isinstance(payload, dict):
        return 0

    event = event_name(event_arg, payload)
    path = state_path()
    state = load_state(path)
    if event == "UserPromptSubmit":
        return handle_prompt(agent, payload, policy, state, path)
    if event == "Stop":
        return handle_stop(agent, payload, policy, state, path)
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
