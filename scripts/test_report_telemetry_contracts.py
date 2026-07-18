#!/usr/bin/env python3
"""Cross-surface contract tests for runtime-owned report telemetry."""

from __future__ import annotations

import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "scripts"))
import agent_contract_lib as contracts  # noqa: E402

TEMPLATE = ROOT / ".ai" / "templates" / "agent-report-v1.md"
INVENTORY = (
    ROOT
    / "docs"
    / "plans"
    / "agentkit-implementation-roadmap"
    / "ws-15-supervised-audit-repair-campaign"
    / "report-surface-inventory.md"
)
DEVELOPMENT_HOOKS = ROOT / "docs" / "development-hooks.md"
PRE_COMMIT = ROOT / "scripts" / "git-hooks" / "pre-commit"
PRE_PUSH = ROOT / "scripts" / "git-hooks" / "pre-push"
CANONICAL_SKILLS = sorted((ROOT / ".ai" / "skills").glob("*/SKILL.md"))

REQUIRED_TEMPLATE_NEEDLES = [
    "## Measurement Rules",
    "Never invent elapsed time, duration, tokens, cost, throughput, or efficiency numbers.",
    "NOT_MEASURED",
    "TimingSource",
    "token_source",
    "Agent prose, Markdown tables, and review consensus are not trusted telemetry channels.",
]
SKILL_METRIC_BAN = (
    "Never invent elapsed time, duration, tokens, cost, throughput, or efficiency numbers"
)
UNPROVEN_METRIC_PATTERNS = [
    re.compile(r"\btook\s+\d+\s*(ms|s|sec|seconds|minutes)\b", re.I),
    re.compile(r"\b\d+(\.\d+)?%\s+efficient\b", re.I),
    re.compile(r"\belapsed\s*[:=]\s*\d+", re.I),
    re.compile(r"\btokens?\s*[:=]\s*\d+", re.I),
    re.compile(r"\bcost\s*[:=]\s*\$?\d+", re.I),
]


def read(path: Path) -> str:
    return path.read_text(encoding="utf-8")


def fail(message: str) -> None:
    raise AssertionError(message)


def test_inventory_exists() -> None:
    if not INVENTORY.is_file():
        fail("report-surface-inventory.md is missing")
    content = read(INVENTORY)
    for needle in [
        "agent-report-v1.md",
        "NOT_MEASURED",
        "runtime",
        "internal/cli/run.go",
        "scripts/test_report_telemetry_contracts.py",
        "pre-commit",
        "pre-push",
        "make telemetry-contract-test",
    ]:
        if needle not in content:
            fail(f"inventory missing required surface marker: {needle}")


def test_report_template_forbids_unproven_telemetry() -> None:
    if not TEMPLATE.is_file():
        fail(".ai/templates/agent-report-v1.md is missing")
    content = read(TEMPLATE)
    for needle in REQUIRED_TEMPLATE_NEEDLES:
        if needle not in content:
            fail(f"report template missing telemetry contract: {needle}")


def test_canonical_skills_ban_invented_metrics() -> None:
    if not CANONICAL_SKILLS:
        fail("no canonical .ai skills found")
    for path in CANONICAL_SKILLS:
        content = read(path)
        rel = path.relative_to(ROOT)
        if "## Required Report" not in content and "ReportFormat: mivia-agent-report/v1" not in content:
            fail(f"{rel} does not declare a report contract")
        if SKILL_METRIC_BAN not in content:
            fail(f"{rel} missing invent-metrics ban line")
        if "NOT_MEASURED" not in content:
            fail(f"{rel} missing NOT_MEASURED requirement")
        for pattern in UNPROVEN_METRIC_PATTERNS:
            if pattern.search(content):
                fail(f"{rel} contains unproven metric claim matching {pattern.pattern}")


def test_false_commit_claims_removed() -> None:
    failures = contracts.check_false_commit_surfaces()
    if failures:
        fail("; ".join(failures))
    fixture_failures = contracts.positive_false_commit_fixtures_blocked()
    if fixture_failures:
        fail("; ".join(fixture_failures))


def test_main_invokes_all_tests() -> None:
    content = read(Path(__file__))
    missing = contracts.missing_main_test_calls(content)
    if missing:
        fail(f"main() missing real AST calls for: {', '.join(missing)}")
    if not contracts.entrypoint_calls_main(content):
        fail("entrypoint must unconditionally call main()/sys.exit(main())")


def test_makefile_and_verifier_wire_telemetry_contract() -> None:
    makefile = read(ROOT / "Makefile")
    if "scripts/test_report_telemetry_contracts.py" not in makefile:
        fail("Makefile must run scripts/test_report_telemetry_contracts.py")
    if "telemetry-contract-test:" not in makefile:
        fail("Makefile must define telemetry-contract-test target")

    verifier = read(ROOT / "scripts" / "verify_agent_config.py")
    for needle in [
        "agent_contract_lib",
        "check_false_commit_surfaces",
        "check_supervised_plan_allowlist",
        "verify_report_telemetry_contract",
        "scripts/test_report_telemetry_contracts.py",
        "telemetry contract tests passed",
    ]:
        if needle not in verifier:
            fail(f"scripts/verify_agent_config.py missing telemetry guard: {needle}")
    calls = contracts.main_calls_via_ast(verifier, straight_line=False)
    if "verify_report_telemetry_contract" not in calls:
        fail("scripts/verify_agent_config.py main() must AST-call verify_report_telemetry_contract")
    if "verify_supervised_plan_allowlist" not in calls:
        fail("scripts/verify_agent_config.py main() must AST-call verify_supervised_plan_allowlist")
    telemetry_body = contracts.function_body_calls(verifier, "verify_report_telemetry_contract")
    for required in [
        "contracts.check_false_commit_surfaces",
        "contracts.positive_false_commit_fixtures_blocked",
    ]:
        if required not in telemetry_body:
            fail(f"verify_report_telemetry_contract body must call {required}")
    allowlist_body = contracts.function_body_calls(verifier, "verify_supervised_plan_allowlist")
    if "contracts.check_supervised_plan_allowlist" not in allowlist_body:
        fail("verify_supervised_plan_allowlist body must call contracts.check_supervised_plan_allowlist")
    plan_script = read(ROOT / "scripts" / "test_agent_plan_contracts.py")
    if not contracts.entrypoint_calls_main(plan_script):
        fail("plan contracts entrypoint must call main()")
    subset_body = contracts.function_body_calls(
        plan_script, "test_supervised_campaign_plan_files_edit_subset_of_scope_in"
    )
    if "contracts.check_supervised_plan_allowlist" not in subset_body:
        fail("plan allowlist test body must call contracts.check_supervised_plan_allowlist")

    for path, label in [(PRE_COMMIT, "pre-commit"), (PRE_PUSH, "pre-push")]:
        content = read(path)
        if "scripts/test_report_telemetry_contracts.py" not in content:
            fail(f"{label} must run scripts/test_report_telemetry_contracts.py")
    if "telemetry contract tests passed" not in read(PRE_COMMIT):
        fail("pre-commit Quality summary must record telemetry contract tests passed")

    hooks_doc = read(DEVELOPMENT_HOOKS)
    for needle in [
        "make telemetry-contract-test",
        "scripts/test_report_telemetry_contracts.py",
        "telemetry contract",
    ]:
        if needle not in hooks_doc:
            fail(f"docs/development-hooks.md missing telemetry gate docs: {needle}")


def main() -> int:
    test_inventory_exists()
    test_report_template_forbids_unproven_telemetry()
    test_canonical_skills_ban_invented_metrics()
    test_false_commit_claims_removed()
    test_main_invokes_all_tests()
    test_makefile_and_verifier_wire_telemetry_contract()
    print("report telemetry contract tests passed")
    return 0


if __name__ == "__main__":
    sys.exit(main())
