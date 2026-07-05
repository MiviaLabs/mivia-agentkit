#!/usr/bin/env python3
"""Contract tests for agent hook bypass guards."""

from __future__ import annotations

import json
import os
import subprocess
import sys
import tempfile
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
GUARD = ROOT / "scripts" / "agent_hook_guard.py"
RUNNER = ROOT / "scripts" / "run_agent_hook_guard.sh"


def run_guard(agent: str, event: str, payload: dict[str, object]) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        [sys.executable, str(GUARD), agent, event],
        input=json.dumps(payload),
        cwd=ROOT,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        check=False,
    )


def parse_stdout(proc: subprocess.CompletedProcess[str]) -> dict[str, object]:
    try:
        parsed = json.loads(proc.stdout or "{}")
    except json.JSONDecodeError as exc:
        raise AssertionError(f"stdout was not JSON: {proc.stdout!r}") from exc
    if not isinstance(parsed, dict):
        raise AssertionError(f"stdout JSON was not an object: {parsed!r}")
    return parsed


def test_codex_blocks_no_verify_tool_call() -> None:
    proc = run_guard(
        "codex",
        "pre-tool-use",
        {
            "hook_event_name": "PreToolUse",
            "tool_name": "Bash",
            "tool_input": {"command": "git commit -m test --no-verify"},
        },
    )

    if proc.returncode != 0:
        raise AssertionError(f"codex guard failed unexpectedly: {proc.stderr}")
    payload = parse_stdout(proc)
    hook_output = payload.get("hookSpecificOutput")
    if not isinstance(hook_output, dict):
        raise AssertionError(f"missing hookSpecificOutput: {payload!r}")
    if hook_output.get("permissionDecision") != "deny":
        raise AssertionError(f"codex guard did not deny bypass: {payload!r}")
    reason = str(hook_output.get("permissionDecisionReason", ""))
    if "fix the failing hook" not in reason or "notify the user" not in reason:
        raise AssertionError(f"codex denial did not give repair guidance: {reason!r}")


def test_claude_blocks_husky_zero_tool_call() -> None:
    proc = run_guard(
        "claude",
        "pre-tool-use",
        {
            "hook_event_name": "PreToolUse",
            "tool_name": "Bash",
            "tool_input": {"command": "HUSKY=0 git commit -m test"},
        },
    )

    if proc.returncode != 2:
        raise AssertionError(f"claude guard should exit 2, got {proc.returncode}: {proc.stderr}")
    if "Do not bypass Git hooks" not in proc.stderr:
        raise AssertionError(f"claude guard did not explain bypass denial: {proc.stderr!r}")


def test_agents_context_for_user_prompt_bypass_request() -> None:
    proc = run_guard(
        "agents",
        "user-prompt-submit",
        {
            "hook_event_name": "UserPromptSubmit",
            "prompt": "commit this with --no-verify if the hooks are noisy",
        },
    )

    if proc.returncode != 0:
        raise AssertionError(f"agents prompt guard failed unexpectedly: {proc.stderr}")
    payload = parse_stdout(proc)
    hook_output = payload.get("hookSpecificOutput")
    if not isinstance(hook_output, dict):
        raise AssertionError(f"missing hookSpecificOutput: {payload!r}")
    context = str(hook_output.get("additionalContext", ""))
    if "Do not bypass Git hooks" not in context:
        raise AssertionError(f"agents prompt guard did not add corrective context: {payload!r}")


def test_env_payload_blocks_husky_skip() -> None:
    proc = run_guard(
        "codex",
        "permission-request",
        {
            "hook_event_name": "PermissionRequest",
            "tool_name": "Bash",
            "tool_input": {
                "command": "git commit -m test",
                "env": {"HUSKY": "0"},
            },
        },
    )

    if proc.returncode != 0:
        raise AssertionError(f"codex permission guard failed unexpectedly: {proc.stderr}")
    payload = parse_stdout(proc)
    hook_output = payload.get("hookSpecificOutput")
    if not isinstance(hook_output, dict) or hook_output.get("permissionDecision") != "deny":
        raise AssertionError(f"env-based Husky skip was not denied: {payload!r}")


def test_safe_command_is_silent() -> None:
    proc = run_guard(
        "codex",
        "pre-tool-use",
        {
            "hook_event_name": "PreToolUse",
            "tool_name": "Bash",
            "tool_input": {"command": "git status --short --branch"},
        },
    )

    if proc.returncode != 0:
        raise AssertionError(f"safe command failed: {proc.stderr}")
    if proc.stdout.strip():
        raise AssertionError(f"safe command should not emit output: {proc.stdout!r}")


def test_runner_applies_guard_before_future_binary() -> None:
    with tempfile.TemporaryDirectory() as tmp:
        fake_bin = Path(tmp)
        fake_agent = fake_bin / "mivia-agent"
        fake_agent.write_text("#!/usr/bin/env sh\nprintf 'future binary ran\\n'\n", encoding="utf-8")
        fake_agent.chmod(0o755)
        env = os.environ.copy()
        env["PATH"] = f"{fake_bin}:{env['PATH']}"

        proc = subprocess.run(
            [str(RUNNER), "codex", "pre-tool-use"],
            input=json.dumps(
                {
                    "hook_event_name": "PreToolUse",
                    "tool_name": "Bash",
                    "tool_input": {"command": "git commit -m test --no-verify"},
                }
            ),
            cwd=ROOT,
            env=env,
            text=True,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            check=False,
        )

    if proc.returncode != 0:
        raise AssertionError(f"runner guard failed unexpectedly: {proc.stderr}")
    if "future binary ran" in proc.stdout:
        raise AssertionError("runner called future binary before applying repo guard")
    payload = parse_stdout(proc)
    hook_output = payload.get("hookSpecificOutput")
    if not isinstance(hook_output, dict) or hook_output.get("permissionDecision") != "deny":
        raise AssertionError(f"runner did not block bypass before future binary: {payload!r}")


def main() -> int:
    test_codex_blocks_no_verify_tool_call()
    test_claude_blocks_husky_zero_tool_call()
    test_agents_context_for_user_prompt_bypass_request()
    test_env_payload_blocks_husky_skip()
    test_safe_command_is_silent()
    test_runner_applies_guard_before_future_binary()
    print("agent hook guard tests passed")
    return 0


if __name__ == "__main__":
    sys.exit(main())
