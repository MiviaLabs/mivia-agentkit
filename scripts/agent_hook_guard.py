#!/usr/bin/env python3
"""Shared guard for agent hook-bypass attempts."""

from __future__ import annotations

import json
import re
import sys
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[1]
POLICY_PATH = ROOT / ".ai" / "policy" / "agent-hook-bypass.json"

EVENT_NAMES = {
    "user-prompt-submit": "UserPromptSubmit",
    "pre-tool-use": "PreToolUse",
    "permission-request": "PermissionRequest",
    "stop": "Stop",
}
SUPPORTED_AGENTS = {"agents", "claude", "codex"}
PROMPT_EVENTS = {"UserPromptSubmit"}
BLOCK_EVENTS = {"PreToolUse", "PermissionRequest"}
SHELL_TOOLS = {"bash", "shell", "command"}


def load_policy() -> dict[str, Any]:
    try:
        policy = json.loads(POLICY_PATH.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as exc:
        raise ValueError(f"invalid hook bypass policy: {exc}") from exc
    if not isinstance(policy, dict) or policy.get("version") != 1:
        raise ValueError("invalid hook bypass policy: expected version 1 object")
    message = policy.get("correctiveMessage")
    if not isinstance(message, str) or "Do not bypass Git hooks" not in message:
        raise ValueError("invalid hook bypass policy: missing corrective message")
    return policy


def event_name(raw: str, payload: dict[str, Any] | None = None) -> str:
    payload_event = payload.get("hook_event_name") if payload else None
    if isinstance(payload_event, str) and payload_event:
        return payload_event
    return EVENT_NAMES.get(raw, raw)


def iter_strings(value: Any) -> list[str]:
    values: list[str] = []
    if isinstance(value, str):
        values.append(value)
    elif isinstance(value, dict):
        for key, item in value.items():
            if isinstance(key, str):
                values.append(key)
            values.extend(iter_strings(item))
    elif isinstance(value, list):
        for item in value:
            values.extend(iter_strings(item))
    return values


def has_blocked_env(value: Any, policy: dict[str, Any]) -> bool:
    blocked_env = policy.get("blockedEnv", {})
    legacy_env = policy.get("blockedLegacyEnv", [])
    if not isinstance(blocked_env, dict) or not isinstance(legacy_env, list):
        return False

    if isinstance(value, dict):
        for key, item in value.items():
            if isinstance(key, str):
                upper_key = key.upper()
                if upper_key in blocked_env and str(item).strip().strip("'\"") == str(blocked_env[upper_key]):
                    return True
                if upper_key in {str(name).upper() for name in legacy_env} and str(item).strip().lower() not in {
                    "",
                    "0",
                    "false",
                    "none",
                }:
                    return True
            if has_blocked_env(item, policy):
                return True
    elif isinstance(value, list):
        return any(has_blocked_env(item, policy) for item in value)
    return False


def bypass_reasons(payload: dict[str, Any], policy: dict[str, Any]) -> list[str]:
    reasons: list[str] = []
    texts = iter_strings(payload)

    blocked_flags = policy.get("blockedFlags", [])
    if isinstance(blocked_flags, list):
        for flag in blocked_flags:
            if isinstance(flag, str) and any(flag in text for text in texts):
                reasons.append(f"blocked flag {flag}")

    husky_zero = re.compile(r"(?i)(?:^|[\s;])(?:export\s+|env\s+)?HUSKY\s*=\s*['\"]?0['\"]?(?=$|[\s;])")
    if any(husky_zero.search(text) for text in texts) or has_blocked_env(payload, policy):
        reasons.append("blocked Husky skip environment")

    legacy_env = policy.get("blockedLegacyEnv", [])
    if isinstance(legacy_env, list):
        for name in legacy_env:
            if not isinstance(name, str):
                continue
            assign = re.compile(rf"(?i)(?:^|[\s;])(?:export\s+|env\s+)?{re.escape(name)}\s*=\s*['\"]?(?:1|true|yes)['\"]?(?=$|[\s;])")
            if any(assign.search(text) for text in texts):
                reasons.append(f"blocked legacy {name} environment")

    return sorted(set(reasons))


def is_shell_tool(payload: dict[str, Any]) -> bool:
    tool_name = payload.get("tool_name") or payload.get("toolName") or payload.get("tool")
    if isinstance(tool_name, str) and tool_name.lower() in SHELL_TOOLS:
        return True
    tool_input = payload.get("tool_input") or payload.get("toolInput")
    return isinstance(tool_input, dict) and isinstance(tool_input.get("command"), str)


def correction(policy: dict[str, Any], reasons: list[str]) -> str:
    reason_text = ", ".join(reasons) if reasons else "hook bypass attempt"
    return f"{policy['correctiveMessage']} Detected: {reason_text}."


def emit_context(agent: str, event: str, message: str) -> int:
    if agent == "claude":
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


def emit_block(agent: str, event: str, message: str) -> int:
    if agent == "claude":
        print(message, file=sys.stderr)
        return 2

    payload: dict[str, Any] = {
        "hookSpecificOutput": {
            "hookEventName": event,
            "permissionDecision": "deny",
            "permissionDecisionReason": message,
        }
    }
    if agent == "agents":
        payload["decision"] = "block"
        payload["reason"] = message
    print(json.dumps(payload, separators=(",", ":")))
    return 0


def emit_malformed(agent: str, event: str, message: str) -> int:
    if agent == "claude":
        print(message, file=sys.stderr)
        return 2
    print(json.dumps({"decision": "block", "reason": message, "hookSpecificOutput": {"hookEventName": event}}))
    return 0


def main(argv: list[str]) -> int:
    if len(argv) != 3:
        print("usage: agent_hook_guard.py <agents|claude|codex> <hook-event>", file=sys.stderr)
        return 2

    agent = argv[1]
    event_arg = argv[2]
    if agent not in SUPPORTED_AGENTS:
        print(f"unsupported agent surface: {agent}", file=sys.stderr)
        return 2

    event = event_name(event_arg)
    try:
        policy = load_policy()
        payload = json.load(sys.stdin)
    except (ValueError, json.JSONDecodeError) as exc:
        return emit_malformed(agent, event, f"Malformed agent hook payload; protected action denied: {exc}")

    if not isinstance(payload, dict):
        return emit_malformed(agent, event, "Malformed agent hook payload; protected action denied: expected JSON object")

    event = event_name(event_arg, payload)
    if event in BLOCK_EVENTS and not is_shell_tool(payload):
        return 0

    reasons = bypass_reasons(payload, policy)
    if not reasons:
        return 0

    message = correction(policy, reasons)
    if event in PROMPT_EVENTS:
        return emit_context(agent, event, message)
    if event in BLOCK_EVENTS:
        return emit_block(agent, event, message)
    return emit_context(agent, event, message)


if __name__ == "__main__":
    sys.exit(main(sys.argv))
