#!/usr/bin/env python3
"""Contract tests for audit loop agent hooks."""

from __future__ import annotations

import json
import os
import subprocess
import sys
import tempfile
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
GUARD = ROOT / "scripts" / "audit_loop_guard.py"
RUNNER = ROOT / "scripts" / "run_agent_hook_guard.sh"


GAP_REPORT = """ReportFormat: mivia-agent-report/v1
Skill: deep-bug-audit
Result: BLOCK
Scope: scripts
Baseline: dev
Summary: found a gap

| ID | Severity | Status | File:Line | Finding | Required Fix | Required Test | Mutation |
| --- | --- | --- | --- | --- | --- | --- | --- |
| BUG-1 | low | open | scripts/example.py:12 | Low severity still must be fixed. | Fix it. | scripts/test_example.py::test_gap | Remove guard. |

| Command | Result | Notes |
| --- | --- | --- |
| make verify | PASS | none |

ResidualRisk: none
NextAction: Fix BUG-1
"""

CLEAN_REPORT = """ReportFormat: mivia-agent-report/v1
Skill: deep-bug-audit
Result: PASS
Scope: scripts
Baseline: dev
Summary: no gaps

| ID | Severity | Status | File:Line | Finding | Required Fix | Required Test | Mutation |
| --- | --- | --- | --- | --- | --- | --- | --- |
| none | none | closed | none | none | none | none | none |

| Command | Result | Notes |
| --- | --- | --- |
| make verify | PASS | none |

ResidualRisk: none
NextAction: none
"""

MALFORMED_REPORT = """ReportFormat: mivia-agent-report/v1
Skill: deep-bug-audit
Result: PASS
Summary: missing required tables
"""


def run_guard(
    agent: str,
    event: str,
    payload: dict[str, object],
    *,
    state_file: Path,
) -> subprocess.CompletedProcess[str]:
    env = os.environ.copy()
    env["MIVIA_AUDIT_LOOP_STATE"] = str(state_file)
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


def parse_stdout(proc: subprocess.CompletedProcess[str]) -> dict[str, object]:
    try:
        parsed = json.loads(proc.stdout or "{}")
    except json.JSONDecodeError as exc:
        raise AssertionError(f"stdout was not JSON: {proc.stdout!r}") from exc
    if not isinstance(parsed, dict):
        raise AssertionError(f"stdout JSON was not an object: {parsed!r}")
    return parsed


def assert_continue(proc: subprocess.CompletedProcess[str], *, contains: str) -> None:
    if proc.returncode != 0:
        raise AssertionError(f"guard failed unexpectedly: {proc.stderr}")
    payload = parse_stdout(proc)
    if payload.get("decision") != "block":
        raise AssertionError(f"guard did not continue loop: {payload!r}")
    if contains not in str(payload.get("reason", "")):
        raise AssertionError(f"continuation reason missing {contains!r}: {payload!r}")


def assert_silent(proc: subprocess.CompletedProcess[str]) -> None:
    if proc.returncode != 0:
        raise AssertionError(f"guard failed unexpectedly: {proc.stderr}")
    if proc.stdout.strip():
        raise AssertionError(f"expected silent allow-stop result, got {proc.stdout!r}")


def test_prompt_initializes_audit_loop_context(state_file: Path) -> None:
    proc = run_guard(
        "codex",
        "user-prompt-submit",
        {
            "session_id": "s1",
            "hook_event_name": "UserPromptSubmit",
            "prompt": "run deep-bug-audit and fix all gaps",
        },
        state_file=state_file,
    )

    if proc.returncode != 0:
        raise AssertionError(f"prompt guard failed: {proc.stderr}")
    payload = parse_stdout(proc)
    context = str(payload.get("hookSpecificOutput", {}).get("additionalContext", ""))
    if "Audit loop active" not in context or "two consecutive clean reports" not in context:
        raise AssertionError(f"prompt context missing loop guidance: {payload!r}")


def test_safe_prompt_is_silent(state_file: Path) -> None:
    proc = run_guard(
        "codex",
        "user-prompt-submit",
        {"session_id": "s1", "hook_event_name": "UserPromptSubmit", "prompt": "summarize README"},
        state_file=state_file,
    )

    assert_silent(proc)


def test_stop_continues_when_low_severity_gap_exists(state_file: Path) -> None:
    proc = run_guard(
        "codex",
        "stop",
        {
            "session_id": "s1",
            "hook_event_name": "Stop",
            "last_assistant_message": GAP_REPORT,
        },
        state_file=state_file,
    )

    assert_continue(proc, contains="1 gap")
    if "BUG-1" in state_file.read_text(encoding="utf-8"):
        raise AssertionError("state file persisted raw finding ID")


def test_two_clean_reports_allow_stop(state_file: Path) -> None:
    first = run_guard(
        "codex",
        "stop",
        {
            "session_id": "s1",
            "hook_event_name": "Stop",
            "last_assistant_message": CLEAN_REPORT,
        },
        state_file=state_file,
    )
    assert_continue(first, contains="clean report 1/2")

    second = run_guard(
        "codex",
        "stop",
        {
            "session_id": "s1",
            "hook_event_name": "Stop",
            "last_assistant_message": CLEAN_REPORT,
        },
        state_file=state_file,
    )
    assert_silent(second)


def test_max_iterations_allows_stop(state_file: Path) -> None:
    for _ in range(9):
        proc = run_guard(
            "codex",
            "stop",
            {
                "session_id": "s1",
                "hook_event_name": "Stop",
                "last_assistant_message": GAP_REPORT,
            },
            state_file=state_file,
        )
        assert_continue(proc, contains="gap")

    tenth = run_guard(
        "codex",
        "stop",
        {
            "session_id": "s1",
            "hook_event_name": "Stop",
            "last_assistant_message": GAP_REPORT,
        },
        state_file=state_file,
    )
    assert_silent(tenth)


def test_claude_host_cap_allows_stop_at_eight(state_file: Path) -> None:
    for _ in range(7):
        proc = run_guard(
            "claude",
            "stop",
            {
                "session_id": "s1",
                "hook_event_name": "Stop",
                "last_assistant_message": GAP_REPORT,
            },
            state_file=state_file,
        )
        assert_continue(proc, contains="/8")

    eighth = run_guard(
        "claude",
        "stop",
        {
            "session_id": "s1",
            "hook_event_name": "Stop",
            "last_assistant_message": GAP_REPORT,
        },
        state_file=state_file,
    )
    assert_silent(eighth)


def test_malformed_report_continues_once(state_file: Path) -> None:
    proc = run_guard(
        "codex",
        "stop",
        {
            "session_id": "s1",
            "hook_event_name": "Stop",
            "last_assistant_message": MALFORMED_REPORT,
        },
        state_file=state_file,
    )

    assert_continue(proc, contains="malformed")


def test_runner_runs_audit_loop_after_bypass_guard(state_file: Path) -> None:
    env = os.environ.copy()
    env["MIVIA_AUDIT_LOOP_STATE"] = str(state_file)
    proc = subprocess.run(
        [str(RUNNER), "codex", "stop"],
        input=json.dumps(
            {
                "session_id": "s1",
                "hook_event_name": "Stop",
                "last_assistant_message": GAP_REPORT,
            }
        ),
        cwd=ROOT,
        env=env,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        check=False,
    )

    assert_continue(proc, contains="gap")


def main() -> int:
    tests = [
        test_prompt_initializes_audit_loop_context,
        test_safe_prompt_is_silent,
        test_stop_continues_when_low_severity_gap_exists,
        test_two_clean_reports_allow_stop,
        test_max_iterations_allows_stop,
        test_claude_host_cap_allows_stop_at_eight,
        test_malformed_report_continues_once,
        test_runner_runs_audit_loop_after_bypass_guard,
    ]
    for test in tests:
        with tempfile.TemporaryDirectory() as tmp:
            test(Path(tmp) / "audit-loop-state.json")
    print("audit loop guard tests passed")
    return 0


if __name__ == "__main__":
    sys.exit(main())
