#!/usr/bin/env python3
"""Contract tests for repo-local skill report formats."""

from __future__ import annotations

import re
import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
TEMPLATE = ROOT / ".ai" / "templates" / "agent-report-v1.md"
CANONICAL_SKILLS = sorted((ROOT / ".ai" / "skills").glob("*/SKILL.md"))
CLAUDE_ADAPTERS = sorted((ROOT / ".claude" / "skills").glob("*/SKILL.md"))

REPORT_FORMAT = "mivia-agent-report/v1"
RESULT_ENUM = "PASS|BLOCK|PARTIAL|NOT_RUN"
FINDINGS_HEADER = "| ID | Severity | Status | File:Line | Finding | Required Fix | Required Test | Mutation |"
COMMAND_HEADER = "| Command | Result | Notes |"
REQUIRED_TEMPLATE_NEEDLES = [
    f"ReportFormat: {REPORT_FORMAT}",
    "Skill: <name>",
    f"Result: {RESULT_ENUM}",
    "Scope: <exact files/packages>",
    "Baseline: <branch/commit/diff>",
    "Summary: <one sentence>",
    FINDINGS_HEADER,
    COMMAND_HEADER,
    "ResidualRisk: none|<short exact risk>",
    "NextAction: none|<exact task>",
]
GAP_POLICY_NEEDLES = [
    "Severity never gates approval; every open gap must be fixed.",
    "`PASS` requires zero gap rows and `ResidualRisk: none`.",
    "Gap statuses are `open`, `missing`, `shallow`, and `gated`.",
    "Low-severity gaps still require `BLOCK` or `PARTIAL` until fixed.",
    "`Status` values are `open`, `fixed`, `closed`, `missing`, `shallow`, `gated`, or `none`.",
]


def read(path: Path) -> str:
    return path.read_text(encoding="utf-8")


def fail(message: str) -> None:
    raise AssertionError(message)


def test_report_template_has_required_contract() -> None:
    if not TEMPLATE.is_file():
        fail(".ai/templates/agent-report-v1.md is missing")
    content = read(TEMPLATE)
    for needle in REQUIRED_TEMPLATE_NEEDLES:
        if needle not in content:
            fail(f"{TEMPLATE.relative_to(ROOT)} missing required template field: {needle}")
    if "Result enum is exactly `PASS`, `BLOCK`, `PARTIAL`, or `NOT_RUN`." not in content:
        fail("report template does not define the strict result enum")
    if "Keep every cell to one short sentence or `none`." not in content:
        fail("report template does not require terse parseable cells")
    for needle in GAP_POLICY_NEEDLES:
        if needle not in content:
            fail(f"report template missing gap-fix policy: {needle}")


def test_canonical_skills_require_report_v1_template() -> None:
    if not CANONICAL_SKILLS:
        fail("no canonical .ai skills found")
    for path in CANONICAL_SKILLS:
        content = read(path)
        rel = path.relative_to(ROOT)
        for needle in [
            "## Required Report",
            f"ReportFormat: {REPORT_FORMAT}",
            ".ai/templates/agent-report-v1.md",
            f"Result: {RESULT_ENUM}",
            FINDINGS_HEADER,
            COMMAND_HEADER,
            "Severity never gates approval; every open gap must be fixed.",
            "ResidualRisk:",
            "NextAction:",
        ]:
            if needle not in content:
                fail(f"{rel} missing required report contract marker: {needle}")
        if re.search(r"^## Output\s*$", content, re.MULTILINE):
            fail(f"{rel} still uses free-form ## Output instead of ## Required Report")


def test_claude_adapters_delegate_report_contract() -> None:
    if not CLAUDE_ADAPTERS:
        fail("no Claude skill adapters found")
    for path in CLAUDE_ADAPTERS:
        content = read(path)
        rel = path.relative_to(ROOT)
        if REPORT_FORMAT not in content or ".ai/templates/agent-report-v1.md" not in content:
            fail(f"{rel} must delegate to the canonical report template")
        if re.search(r"^## Output\s*$", content, re.MULTILINE):
            fail(f"{rel} must not define a separate ## Output section")
        if FINDINGS_HEADER in content or COMMAND_HEADER in content:
            fail(f"{rel} must not duplicate the report table shape")


def test_verify_and_make_run_skill_contracts() -> None:
    makefile = read(ROOT / "Makefile")
    for needle in [
        "skill-contract-test:",
        "scripts/test_skill_contracts.py",
        "verify: verify-agent semgrep-validate semgrep-test hook-test agent-hook-test skill-contract-test semgrep go-check",
    ]:
        if needle not in makefile:
            fail(f"Makefile missing skill contract gate: {needle}")

    verifier = read(ROOT / "scripts" / "verify_agent_config.py")
    for needle in [
        "verify_skill_report_contract",
        "scripts/test_skill_contracts.py",
        ".ai/templates/agent-report-v1.md",
        REPORT_FORMAT,
        RESULT_ENUM,
        "skill contract tests passed",
    ]:
        if needle not in verifier:
            fail(f"scripts/verify_agent_config.py missing skill contract guard: {needle}")


def main() -> int:
    test_report_template_has_required_contract()
    test_canonical_skills_require_report_v1_template()
    test_claude_adapters_delegate_report_contract()
    test_verify_and_make_run_skill_contracts()
    print("skill contract tests passed")
    return 0


if __name__ == "__main__":
    sys.exit(main())
