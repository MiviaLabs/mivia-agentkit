#!/usr/bin/env python3
"""Agent hook guard for planning and plan implementation workflows."""

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
POLICY_PATH = ROOT / ".ai" / "policy" / "agent-plan.json"
EVENT_NAMES = {
    "user-prompt-submit": "UserPromptSubmit",
    "stop": "Stop",
}
SUPPORTED_AGENTS = {"agents", "claude", "codex"}
REPORT_GAP_STATUSES = {"open", "missing", "shallow", "gated"}


def event_name(raw: str, payload: dict[str, Any] | None = None) -> str:
    payload_event = payload.get("hook_event_name") if payload else None
    if isinstance(payload_event, str) and payload_event:
        return payload_event
    return EVENT_NAMES.get(raw, raw)


def load_policy() -> dict[str, Any]:
    try:
        policy = json.loads(POLICY_PATH.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as exc:
        raise ValueError(f"invalid agent plan policy: {exc}") from exc
    if not isinstance(policy, dict) or policy.get("version") != 1:
        raise ValueError("invalid agent plan policy: expected version 1 object")
    for key in [
        "planFormat",
        "plannerSkill",
        "implementerSkill",
        "plannerPromptTriggers",
        "implementerPromptTriggers",
        "plannerInstruction",
        "implementerInstruction",
    ]:
        if key not in policy:
            raise ValueError(f"invalid agent plan policy: missing {key}")
    return policy


def state_path() -> Path:
    override = os.environ.get("MIVIA_PLAN_HOOK_STATE")
    if override:
        return Path(override)
    proc = subprocess.run(
        ["git", "rev-parse", "--git-path", "mivia-agent-plan-hook-state.json"],
        cwd=ROOT,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.DEVNULL,
        check=False,
    )
    if proc.returncode == 0 and proc.stdout.strip():
        return ROOT / proc.stdout.strip()
    return ROOT / ".git" / "mivia-agent-plan-hook-state.json"


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


def prompt_matches(prompt: str, triggers: Any) -> bool:
    lowered = prompt.lower()
    return isinstance(triggers, list) and any(isinstance(item, str) and item.lower() in lowered for item in triggers)


def emit_context(event: str, message: str) -> int:
    print(
        json.dumps(
            {"hookSpecificOutput": {"hookEventName": event, "additionalContext": message}},
            separators=(",", ":"),
        )
    )
    return 0


def emit_continue(reason: str) -> int:
    print(json.dumps({"decision": "block", "reason": reason}, separators=(",", ":")))
    return 0


def split_row(line: str) -> list[str]:
    return [cell.strip() for cell in line.strip().strip("|").split("|")]


def report_has_open_gap(text: str) -> bool:
    header = "| ID | Severity | Status | File:Line | Finding | Required Fix | Required Test | Mutation |"
    lines = text.splitlines()
    try:
        start = next(index for index, line in enumerate(lines) if line.strip() == header)
    except StopIteration:
        return False
    for line in lines[start + 2 :]:
        stripped = line.strip()
        if not stripped or not stripped.startswith("|") or stripped == "| Command | Result | Notes |":
            break
        cells = split_row(stripped)
        if len(cells) >= 3 and cells[0].lower() != "none" and cells[2].lower() in REPORT_GAP_STATUSES:
            return True
    residual = re.search(r"^ResidualRisk:\s*(.+?)\s*$", text, flags=re.MULTILINE)
    return bool(residual and residual.group(1).strip().lower() != "none")


def plan_artifact(text: str) -> str | None:
    match = re.search(r"^PlanArtifact:\s*(\.ai/plans/[A-Za-z0-9_.-]+\.plan\.json)\s*$", text, flags=re.MULTILINE)
    return match.group(1) if match else None


def validate_plan_artifact(path_text: str) -> tuple[bool, str]:
    path = ROOT / path_text
    if not path.is_file():
        return False, f"PlanArtifact does not exist: {path_text}"
    proc = subprocess.run(
        [sys.executable, str(ROOT / "scripts" / "validate_agent_plan.py"), path_text],
        cwd=ROOT,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        check=False,
    )
    if proc.returncode != 0:
        return False, proc.stderr.strip() or "PlanArtifact validation failed"
    return True, ""


def handle_prompt(payload: dict[str, Any], policy: dict[str, Any], state: dict[str, Any], path: Path) -> int:
    prompt = payload.get("prompt", "")
    if not isinstance(prompt, str):
        return 0

    record = active_record(state, payload)
    if prompt_matches(prompt, policy.get("plannerPromptTriggers")):
        record.clear()
        record.update({"active": True, "mode": "planning"})
        save_state(path, state)
        return emit_context(
            "UserPromptSubmit",
            f"{policy['plannerInstruction']} Required format: {policy['planFormat']}. Required skill: {policy['plannerSkill']}.",
        )

    if prompt_matches(prompt, policy.get("implementerPromptTriggers")):
        record.clear()
        record.update({"active": True, "mode": "implementation"})
        save_state(path, state)
        return emit_context(
            "UserPromptSubmit",
            f"{policy['implementerInstruction']} Required skill: {policy['implementerSkill']}; start only from a validated .ai/plans/*.plan.json.",
        )
    return 0


def handle_stop(payload: dict[str, Any], policy: dict[str, Any], state: dict[str, Any], path: Path) -> int:
    message = payload.get("last_assistant_message", "")
    if not isinstance(message, str) or not message.strip():
        return 0

    record = active_record(state, payload)
    was_planning = record.get("mode") == "planning"
    was_implementation = record.get("mode") == "implementation"
    tracked_skill = f"Skill: {policy['plannerSkill']}" in message or f"Skill: {policy['implementerSkill']}" in message
    if not was_planning and not was_implementation and not tracked_skill:
        return 0

    if report_has_open_gap(message):
        return emit_continue("Planning workflow still reports open gaps or residual risk. Fix every gap and re-emit the structured report.")

    artifact = plan_artifact(message)
    if tracked_skill or was_planning or was_implementation:
        if not artifact:
            if not policy.get("planArtifactsRequired", False):
                clear_record(state, payload)
                save_state(path, state)
                return 0
            return emit_continue("Planning or implementation report must include PlanArtifact: .ai/plans/<id>.plan.json and validate mivia-agent-plan/v1.")
        valid, reason = validate_plan_artifact(artifact)
        if not valid:
            return emit_continue(reason)

    clear_record(state, payload)
    save_state(path, state)
    return 0


def main(argv: list[str]) -> int:
    if len(argv) != 3:
        print("usage: plan_hook_guard.py <agents|claude|codex> <hook-event>", file=sys.stderr)
        return 2
    agent = argv[1]
    if agent not in SUPPORTED_AGENTS:
        print(f"unsupported agent surface: {agent}", file=sys.stderr)
        return 2
    try:
        payload = json.load(sys.stdin)
        policy = load_policy()
    except (json.JSONDecodeError, ValueError) as exc:
        print(f"plan hook guard ignored malformed input: {exc}", file=sys.stderr)
        return 0
    if not isinstance(payload, dict):
        return 0

    event = event_name(argv[2], payload)
    path = state_path()
    state = load_state(path)
    if event == "UserPromptSubmit":
        return handle_prompt(payload, policy, state, path)
    if event == "Stop":
        return handle_stop(payload, policy, state, path)
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
