#!/usr/bin/env python3
"""Verify the repository's agent configuration surface."""

from __future__ import annotations

import json
import os
import re
import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
FAILURES: list[str] = []


def repo_path(path: str) -> Path:
    return ROOT / path


def text(path: str) -> str:
    return repo_path(path).read_text(encoding="utf-8")


def require(condition: bool, message: str) -> None:
    if not condition:
        FAILURES.append(message)


def load_json(path: str) -> object:
    try:
        return json.loads(text(path))
    except json.JSONDecodeError as exc:
        FAILURES.append(f"{path}: invalid JSON: {exc}")
        return {}


def skill_frontmatter(path: str) -> dict[str, object]:
    content = text(path)
    if not content.startswith("---\n"):
        FAILURES.append(f"{path}: missing YAML frontmatter")
        return {}
    try:
        raw = content.split("---\n", 2)[1]
    except IndexError:
        FAILURES.append(f"{path}: unterminated YAML frontmatter")
        return {}

    data: dict[str, object] = {}
    current_list: str | None = None
    for line in raw.splitlines():
        if not line.strip():
            continue
        if line.startswith("  - ") and current_list:
            value = line.removeprefix("  - ").strip()
            data.setdefault(current_list, [])
            assert isinstance(data[current_list], list)
            data[current_list].append(value)
            continue
        current_list = None
        if ":" not in line:
            continue
        key, value = line.split(":", 1)
        key = key.strip()
        value = value.strip()
        if value:
            data[key] = value
        else:
            data[key] = []
            current_list = key
    return data


def bash_payload(rule: str) -> str | None:
    match = re.fullmatch(r"Bash\((.*)\)", rule)
    if not match:
        return None
    return match.group(1)


def verify_agents_md() -> None:
    content = text("AGENTS.md")
    sections = content.split("\n## ")[1:]
    require(sections != [], "AGENTS.md: no second-level sections found")
    for section in sections:
        title, _, body = section.partition("\n")
        require("Sources:" in body, f"AGENTS.md: section {title!r} is missing Sources")
    require(
        "scripts/verify_agent_config.py" in content,
        "AGENTS.md: missing committed agent-config verifier command",
    )


def verify_index() -> None:
    content = text(".ai/INDEX.md")
    rule_paths = sorted(str(p.relative_to(ROOT)) for p in repo_path(".ai/rules").glob("*.md"))
    skill_paths = sorted(
        str(p.relative_to(ROOT))
        for root in [repo_path(".ai/skills"), repo_path(".claude/skills")]
        for p in root.glob("*/SKILL.md")
    )
    for path in rule_paths + skill_paths:
        require(path in content, f".ai/INDEX.md: missing {path}")
    require(
        "scripts/verify_agent_config.py" in content,
        ".ai/INDEX.md: missing committed agent-config verifier command",
    )
    for path in [
        ".githooks/",
        "semgrep/",
        "scripts/",
        ".ai/templates/agent-report-v1.md",
        ".ai/policy/commit-message.json",
        ".ai/policy/agent-hook-bypass.json",
        ".ai/policy/audit-loop.json",
        ".agents/hooks.json",
        "docs/agent-hooks.md",
        "docs/development-hooks.md",
        "README.md",
        "make install-hooks",
        "make agent-hook-test",
        "make audit-loop-test",
        "make skill-contract-test",
    ]:
        require(path in content, f".ai/INDEX.md: missing hook verification path {path}")


def verify_agent_quality_rules() -> None:
    content = text(".ai/rules/20-agent-quality.md")
    for needle in [
        "Critical Drift Guard",
        "semgrep/agent-standards.yml",
        "scripts/test_semgrep_rules.py",
        "make semgrep-test",
        "Do not use Semgrep suppression",
    ]:
        require(needle in content, f".ai/rules/20-agent-quality.md: missing {needle}")


def verify_skills() -> None:
    registry = load_json(".agents/skills.json")
    skills = registry.get("skills", []) if isinstance(registry, dict) else []
    require(isinstance(skills, list), ".agents/skills.json: skills must be a list")

    listed = sorted(item.get("path") for item in skills if isinstance(item, dict))
    actual = sorted(
        str(p.relative_to(ROOT))
        for root in [repo_path(".ai/skills"), repo_path(".claude/skills")]
        for p in root.glob("*/SKILL.md")
    )
    require(listed == actual, f".agents/skills.json: listed skills {listed!r} != actual {actual!r}")

    for path in actual:
        frontmatter = skill_frontmatter(path)
        for key in ["name", "description", "triggers"]:
            require(key in frontmatter, f"{path}: missing frontmatter key {key!r}")
        triggers = frontmatter.get("triggers")
        require(isinstance(triggers, list) and bool(triggers), f"{path}: triggers must be non-empty")
        expected_name = Path(path).parent.name
        require(frontmatter.get("name") == expected_name, f"{path}: name must match directory")


def verify_skill_report_contract() -> None:
    report_format = "mivia-agent-report/v1"
    result_enum = "PASS|BLOCK|PARTIAL|NOT_RUN"
    findings_header = "| ID | Severity | Status | File:Line | Finding | Required Fix | Required Test | Mutation |"
    command_header = "| Command | Result | Notes |"

    template_path = ".ai/templates/agent-report-v1.md"
    require(repo_path(template_path).is_file(), f"{template_path}: missing")
    template = text(template_path)
    for needle in [
        f"ReportFormat: {report_format}",
        "Skill: <name>",
        f"Result: {result_enum}",
        "Scope: <exact files/packages>",
        "Baseline: <branch/commit/diff>",
        "Summary: <one sentence>",
        findings_header,
        command_header,
        "ResidualRisk: none|<short exact risk>",
        "NextAction: none|<exact task>",
        "Result enum is exactly `PASS`, `BLOCK`, `PARTIAL`, or `NOT_RUN`.",
        "Keep every cell to one short sentence or `none`.",
        "Severity never gates approval; every open gap must be fixed.",
        "`PASS` requires zero gap rows and `ResidualRisk: none`.",
        "Gap statuses are `open`, `missing`, `shallow`, and `gated`.",
        "Low-severity gaps still require `BLOCK` or `PARTIAL` until fixed.",
        "`Status` values are `open`, `fixed`, `closed`, `missing`, `shallow`, `gated`, or `none`.",
    ]:
        require(needle in template, f"{template_path}: missing {needle}")

    for path in sorted(str(p.relative_to(ROOT)) for p in repo_path(".ai/skills").glob("*/SKILL.md")):
        content = text(path)
        for needle in [
            "## Required Report",
            f"ReportFormat: {report_format}",
            ".ai/templates/agent-report-v1.md",
            f"Result: {result_enum}",
            findings_header,
            command_header,
            "Severity never gates approval; every open gap must be fixed.",
            "ResidualRisk:",
            "NextAction:",
        ]:
            require(needle in content, f"{path}: missing report contract marker {needle}")
        require(
            re.search(r"^## Output\s*$", content, re.MULTILINE) is None,
            f"{path}: must use ## Required Report instead of free-form ## Output",
        )

    for path in sorted(str(p.relative_to(ROOT)) for p in repo_path(".claude/skills").glob("*/SKILL.md")):
        content = text(path)
        require(report_format in content, f"{path}: missing {report_format}")
        require(".ai/templates/agent-report-v1.md" in content, f"{path}: missing report template reference")
        require(
            re.search(r"^## Output\s*$", content, re.MULTILINE) is None,
            f"{path}: must not define a separate ## Output section",
        )
        require(findings_header not in content, f"{path}: must not duplicate report findings table")
        require(command_header not in content, f"{path}: must not duplicate report command table")


def verify_adapters() -> None:
    for path in [
        "CLAUDE.md",
        ".codex/AGENTS.md",
        ".github/copilot-instructions.md",
        ".github/instructions/agent-quality.instructions.md",
    ]:
        require("AGENTS.md" in text(path), f"{path}: must reference root AGENTS.md")


def verify_claude_settings() -> None:
    settings = load_json(".claude/settings.json")
    if not isinstance(settings, dict):
        return

    hooks = settings.get("hooks", {})
    require(
        set(hooks) == {"UserPromptSubmit", "PreToolUse", "PermissionRequest", "Stop"},
        ".claude/settings.json: unexpected hook events",
    )

    event_commands = {
        "UserPromptSubmit": "scripts/run_agent_hook_guard.sh\" claude user-prompt-submit",
        "PreToolUse": "scripts/run_agent_hook_guard.sh\" claude pre-tool-use",
        "PermissionRequest": "scripts/run_agent_hook_guard.sh\" claude permission-request",
        "Stop": "scripts/run_agent_hook_guard.sh\" claude stop",
    }
    for event, expected in event_commands.items():
        commands = [
            hook.get("command", "")
            for group in hooks.get(event, [])
            for hook in group.get("hooks", [])
            if isinstance(hook, dict)
        ]
        require(any(expected in command for command in commands), f".claude/settings.json: missing {expected}")

    permissions = settings.get("permissions", {})
    allow = permissions.get("allow", []) if isinstance(permissions, dict) else []
    for rule in allow:
        if not isinstance(rule, str):
            FAILURES.append(".claude/settings.json: permission allow entries must be strings")
            continue
        payload = bash_payload(rule)
        if payload is None:
            continue
        require("*" not in payload, f".claude/settings.json: wildcard Bash allow is unsafe: {rule}")
        require(
            not re.search(r"[;&|`$<>]", payload),
            f".claude/settings.json: shell metacharacter in Bash allow is unsafe: {rule}",
        )


def verify_codex_hooks() -> None:
    config = load_json(".codex/hooks.json")
    if not isinstance(config, dict):
        return
    hooks = config.get("hooks", {})
    require(
        set(hooks) == {"UserPromptSubmit", "PreToolUse", "PermissionRequest", "Stop"},
        ".codex/hooks.json: unexpected hook events",
    )
    event_commands = {
        "UserPromptSubmit": "scripts/run_agent_hook_guard.sh\" codex user-prompt-submit",
        "PreToolUse": "scripts/run_agent_hook_guard.sh\" codex pre-tool-use",
        "PermissionRequest": "scripts/run_agent_hook_guard.sh\" codex permission-request",
        "Stop": "scripts/run_agent_hook_guard.sh\" codex stop",
    }
    for event, expected in event_commands.items():
        commands = [
            hook.get("command", "")
            for group in hooks.get(event, [])
            for hook in group.get("hooks", [])
            if isinstance(hook, dict)
        ]
        require(any(expected in command for command in commands), f".codex/hooks.json: missing {expected}")


def verify_agents_hooks() -> None:
    config = load_json(".agents/hooks.json")
    if not isinstance(config, dict):
        return
    require(config.get("version") == 1, ".agents/hooks.json: expected version 1")
    generated_from = config.get("generated_from", [])
    require(isinstance(generated_from, list), ".agents/hooks.json: generated_from must be a list")
    for path in [".ai/policy/agent-hook-bypass.json", ".ai/policy/audit-loop.json"]:
        require(path in generated_from, f".agents/hooks.json: missing policy source {path}")
    hooks = config.get("hooks", {})
    require(
        set(hooks) == {"UserPromptSubmit", "PreToolUse", "PermissionRequest", "Stop"},
        ".agents/hooks.json: unexpected hook events",
    )
    for event, expected in {
        "UserPromptSubmit": "scripts/run_agent_hook_guard.sh\" agents user-prompt-submit",
        "PreToolUse": "scripts/run_agent_hook_guard.sh\" agents pre-tool-use",
        "PermissionRequest": "scripts/run_agent_hook_guard.sh\" agents permission-request",
        "Stop": "scripts/run_agent_hook_guard.sh\" agents stop",
    }.items():
        commands = [hook.get("command", "") for hook in hooks.get(event, []) if isinstance(hook, dict)]
        require(any(expected in command for command in commands), f".agents/hooks.json: missing {expected}")


def verify_agent_hook_guard() -> None:
    policy = load_json(".ai/policy/agent-hook-bypass.json")
    if isinstance(policy, dict):
        require(policy.get("version") == 1, ".ai/policy/agent-hook-bypass.json: expected version 1")
        require(policy.get("blockedFlags") == ["--no-verify"], ".ai/policy/agent-hook-bypass.json: blockedFlags drifted")
        require(
            policy.get("blockedEnv") == {"HUSKY": "0"},
            ".ai/policy/agent-hook-bypass.json: blockedEnv drifted",
        )
        require(
            policy.get("blockedLegacyEnv") == ["HUSKY_SKIP_HOOKS"],
            ".ai/policy/agent-hook-bypass.json: blockedLegacyEnv drifted",
        )
        message = policy.get("correctiveMessage")
        require(
            isinstance(message, str)
            and "Do not bypass Git hooks" in message
            and "fix the failing hook" in message
            and "notify the user" in message,
            ".ai/policy/agent-hook-bypass.json: correctiveMessage missing required guidance",
        )

    guard = text("scripts/agent_hook_guard.py")
    for needle in [
        "agent-hook-bypass.json",
        "blockedFlags",
        "blockedEnv",
        "blockedLegacyEnv",
        "permissionDecision",
        "deny",
        "additionalContext",
        "Malformed agent hook payload; protected action denied",
    ]:
        require(needle in guard, f"scripts/agent_hook_guard.py: missing {needle}")

    runner = text("scripts/run_agent_hook_guard.sh")
    for needle in [
        "mivia-agent hook",
        "scripts/agent_hook_guard.py",
        "scripts/audit_loop_guard.py",
        "git rev-parse --show-toplevel",
    ]:
        require(needle in runner, f"scripts/run_agent_hook_guard.sh: missing {needle}")


def verify_audit_loop_guard() -> None:
    policy = load_json(".ai/policy/audit-loop.json")
    if isinstance(policy, dict):
        require(policy.get("version") == 1, ".ai/policy/audit-loop.json: expected version 1")
        require(
            policy.get("reportFormat") == "mivia-agent-report/v1",
            ".ai/policy/audit-loop.json: report format drifted",
        )
        require(
            policy.get("skills") == ["deep-bug-audit", "test-coverage-audit", "adversarial-test-review"],
            ".ai/policy/audit-loop.json: skills drifted",
        )
        triggers = policy.get("promptTriggers")
        require(isinstance(triggers, list), ".ai/policy/audit-loop.json: promptTriggers must be a list")
        if isinstance(triggers, list):
            for trigger in ["deep-bug-audit", "test-coverage-audit", "adversarial-test-review", "bug audit"]:
                require(trigger in triggers, f".ai/policy/audit-loop.json: missing trigger {trigger}")
        require(
            policy.get("gapStatuses") == ["open", "missing", "shallow", "gated"],
            ".ai/policy/audit-loop.json: gapStatuses drifted",
        )
        require(policy.get("maxIterations") == 10, ".ai/policy/audit-loop.json: maxIterations must be 10")
        require(
            policy.get("hostContinuationCaps") == {"claude": 8},
            ".ai/policy/audit-loop.json: hostContinuationCaps drifted",
        )
        require(
            policy.get("cleanReportsToStop") == 2,
            ".ai/policy/audit-loop.json: cleanReportsToStop must be 2",
        )
        require(
            policy.get("maxMalformedReports") == 2,
            ".ai/policy/audit-loop.json: maxMalformedReports must be 2",
        )
        instruction = policy.get("instruction")
        require(
            isinstance(instruction, str)
            and "fix every gap" in instruction
            and "two consecutive clean reports" in instruction
            and "ResidualRisk: none" in instruction,
            ".ai/policy/audit-loop.json: instruction missing strict loop contract",
        )

    guard = text("scripts/audit_loop_guard.py")
    for needle in [
        "last_assistant_message",
        "decision",
        "block",
        "mivia-agent-report/v1",
        "maxIterations",
        "hostContinuationCaps",
        "max_iterations_for_agent",
        "cleanReportsToStop",
        "ResidualRisk",
        "MIVIA_AUDIT_LOOP_STATE",
        "gapStatuses",
        "Fix every gap regardless of severity",
    ]:
        require(needle in guard, f"scripts/audit_loop_guard.py: missing {needle}")

    tests = text("scripts/test_audit_loop_guard.py")
    for needle in [
        "test_stop_continues_when_low_severity_gap_exists",
        "test_two_clean_reports_allow_stop",
        "test_max_iterations_allows_stop",
        "test_claude_host_cap_allows_stop_at_eight",
        "test_malformed_report_continues_once",
        "test_runner_runs_audit_loop_after_bypass_guard",
        "ResidualRisk: none",
        "low | open",
    ]:
        require(needle in tests, f"scripts/test_audit_loop_guard.py: missing {needle}")


def verify_gitignore() -> None:
    entries = set(line.strip() for line in text(".gitignore").splitlines() if line.strip())
    for entry in [
        ".ai/runs/",
        ".git/mivia-agent-quality-stamp.json",
        ".claude/settings.local.json",
        ".semgrep-cache/",
        ".pytest_cache/",
        ".env",
        ".env.*",
        "secrets/",
    ]:
        require(entry in entries, f".gitignore: missing {entry}")


def verify_git_hooks() -> None:
    required_files = [
        ".githooks/pre-commit",
        ".githooks/pre-push",
        ".githooks/prepare-commit-msg",
        ".githooks/commit-msg",
        "scripts/git-hooks/pre-commit",
        "scripts/git-hooks/pre-push",
        "scripts/git-hooks/prepare-commit-msg",
        "scripts/git-hooks/commit-msg",
        "scripts/install_git_hooks.sh",
        "scripts/test_semgrep_rules.py",
        "scripts/test_git_hooks.py",
        "scripts/agent_hook_guard.py",
        "scripts/run_agent_hook_guard.sh",
        "scripts/test_agent_hook_guard.py",
        "scripts/audit_loop_guard.py",
        "scripts/test_audit_loop_guard.py",
        "scripts/test_skill_contracts.py",
        "semgrep/agent-standards.yml",
        ".ai/templates/agent-report-v1.md",
        ".ai/policy/commit-message.json",
        ".ai/policy/agent-hook-bypass.json",
        ".ai/policy/audit-loop.json",
        ".agents/hooks.json",
        "docs/setup/development-environment.md",
        "docs/agent-hooks.md",
        "docs/development-hooks.md",
        "Makefile",
        "README.md",
    ]
    for path in required_files:
        require(repo_path(path).is_file(), f"{path}: missing")

    executable_files = [
        ".githooks/pre-commit",
        ".githooks/pre-push",
        ".githooks/prepare-commit-msg",
        ".githooks/commit-msg",
        "scripts/git-hooks/pre-commit",
        "scripts/git-hooks/pre-push",
        "scripts/git-hooks/prepare-commit-msg",
        "scripts/git-hooks/commit-msg",
        "scripts/install_git_hooks.sh",
        "scripts/test_semgrep_rules.py",
        "scripts/test_git_hooks.py",
        "scripts/agent_hook_guard.py",
        "scripts/run_agent_hook_guard.sh",
        "scripts/test_agent_hook_guard.py",
        "scripts/audit_loop_guard.py",
        "scripts/test_audit_loop_guard.py",
        "scripts/test_skill_contracts.py",
    ]
    for path in executable_files:
        require(os.access(repo_path(path), os.X_OK), f"{path}: must be executable")

    makefile = text("Makefile")
    for needle in [
        "install-hooks",
        "verify:",
        "semgrep-validate:",
        "semgrep-test:",
        "hook-test:",
        "agent-hook-test:",
        "audit-loop-test:",
        "skill-contract-test:",
        "pre-commit:",
        "pre-push:",
        "go-check:",
    ]:
        require(needle in makefile, f"Makefile: missing {needle}")

    readme = text("README.md")
    for needle in [
        "## Docs",
        "[Development environment](docs/setup/development-environment.md)",
        "[Development hooks](docs/development-hooks.md)",
        "[Agent hooks](docs/agent-hooks.md)",
        "make install-hooks",
        "make verify",
        "make help",
        "[Product requirements](docs/prd/0001-mivia-agentkit.md)",
        "[Workstream roadmap](docs/plans/00-overview.md)",
    ]:
        require(needle in readme, f"README.md: missing {needle}")

    setup_doc = text("docs/setup/development-environment.md")
    for needle in [
        "Python 3.10 or newer",
        "Semgrep CLI",
        "GNU Make",
        "sudo apt install -y git bash make python3 python3-venv python3-pip pipx",
        "pipx install semgrep",
        "sudo apt install -y golang-go",
        "sudo snap install go --classic",
        "go.dev/dl/",
        "make install-hooks",
        "make verify",
    ]:
        require(needle in setup_doc, f"docs/setup/development-environment.md: missing {needle}")

    require(
        'core.hooksPath .githooks' in text("scripts/install_git_hooks.sh"),
        "scripts/install_git_hooks.sh: must configure core.hooksPath .githooks",
    )
    require(
        "scripts/git-hooks/pre-commit" in text(".githooks/pre-commit"),
        ".githooks/pre-commit: must delegate to scripts/git-hooks/pre-commit",
    )
    require(
        "scripts/git-hooks/pre-push" in text(".githooks/pre-push"),
        ".githooks/pre-push: must delegate to scripts/git-hooks/pre-push",
    )
    require(
        "scripts/git-hooks/prepare-commit-msg" in text(".githooks/prepare-commit-msg"),
        ".githooks/prepare-commit-msg: must delegate to scripts/git-hooks/prepare-commit-msg",
    )
    require(
        "scripts/git-hooks/commit-msg" in text(".githooks/commit-msg"),
        ".githooks/commit-msg: must delegate to scripts/git-hooks/commit-msg",
    )

    pre_commit = text("scripts/git-hooks/pre-commit")
    for needle in [
        "scripts/verify_agent_config.py",
        "gofmt -w",
        "git diff --check --cached",
        "semgrep --validate --config semgrep/agent-standards.yml",
        "scripts/test_semgrep_rules.py",
        "scripts/test_git_hooks.py",
        "scripts/test_agent_hook_guard.py",
        "scripts/test_audit_loop_guard.py",
        "scripts/test_skill_contracts.py",
        "--disable-nosem",
        "semgrep --config semgrep/agent-standards.yml",
        "mivia-agent-precommit-summary",
        "git write-tree",
        "Quality: pre-commit passed",
        "agent config verification passed",
        "agent hook tests passed",
        "audit loop tests passed",
        "skill contract tests passed",
    ]:
        require(needle in pre_commit, f"scripts/git-hooks/pre-commit: missing {needle}")

    prepare_commit_msg = text("scripts/git-hooks/prepare-commit-msg")
    for needle in [
        "mivia-agent-precommit-summary",
        "Quality: pre-commit passed",
        "git write-tree",
        "merge | squash",
    ]:
        require(needle in prepare_commit_msg, f"scripts/git-hooks/prepare-commit-msg: missing {needle}")

    commit_msg = text("scripts/git-hooks/commit-msg")
    for needle in [
        ".ai/policy/commit-message.json",
        "expected format: type(scope): imperative subject",
        "allowed types/scopes are defined",
        "invalid commit policy",
        "subject is longer than",
        "commit message passed",
        "fixup!",
        "squash!",
    ]:
        require(needle in commit_msg, f"scripts/git-hooks/commit-msg: missing {needle}")

    commit_policy = load_json(".ai/policy/commit-message.json")
    if isinstance(commit_policy, dict):
        types = commit_policy.get("types")
        scopes = commit_policy.get("scopes")
        max_length = commit_policy.get("maxSubjectLength")
        expected_types = [
            "feat",
            "fix",
            "docs",
            "chore",
            "test",
            "refactor",
            "build",
            "ci",
            "perf",
            "style",
            "revert",
        ]
        expected_scopes = [
            "agent",
            "brand",
            "config",
            "docs",
            "hooks",
            "quality",
            "setup",
            "semgrep",
            "workflow",
        ]
        require(types == expected_types, ".ai/policy/commit-message.json: commit types drifted")
        require(scopes == expected_scopes, ".ai/policy/commit-message.json: commit scopes drifted")
        require(max_length == 72, ".ai/policy/commit-message.json: maxSubjectLength must be 72")

    pre_push = text("scripts/git-hooks/pre-push")
    for needle in [
        "scripts/verify_agent_config.py",
        "git diff --check",
        "semgrep --validate --config semgrep/agent-standards.yml",
        "scripts/test_semgrep_rules.py",
        "scripts/test_git_hooks.py",
        "scripts/test_agent_hook_guard.py",
        "scripts/test_audit_loop_guard.py",
        "scripts/test_skill_contracts.py",
        "--disable-nosem",
        "semgrep --config semgrep/agent-standards.yml",
        "go test ./...",
        "go vet ./...",
        "go build ./cmd/mivia-agent",
    ]:
        require(needle in pre_push, f"scripts/git-hooks/pre-push: missing {needle}")

    semgrep_config = text("semgrep/agent-standards.yml")
    for rule_id in [
        "mivia.generic.no-wildcard-bash-allow",
        "mivia.generic.no-shell-metachar-bash-allow",
        "mivia.generic.no-semgrep-suppression",
        "mivia.generic.no-unresolved-drift-markers",
        "mivia.generic.brand-mivialabs",
        "mivia.generic.commit-policy-no-optional-scope-wording",
        "mivia.generic.no-git-hook-bypass-in-agent-config",
        "mivia.generic.no-skill-freeform-output-heading",
        "mivia.generic.no-severity-gated-skill-approval",
        "mivia.go.no-panic-in-internal",
        "mivia.go.no-fatal-exit-in-internal",
        "mivia.go.no-shell-exec",
        "mivia.go.no-syscall-exec",
        "mivia.go.no-network-calls",
        "mivia.go.no-world-writable-mode",
        "mivia.go.no-raw-artifact-write",
        "mivia.go.tests-no-real-agent-cli",
        "mivia.go.tests-use-t-tempdir",
        "mivia.go.tests-no-time-sleep",
    ]:
        require(rule_id in semgrep_config, f"semgrep/agent-standards.yml: missing {rule_id}")

    semgrep_tests = text("scripts/test_semgrep_rules.py")
    for rule_id in [
        "mivia.generic.no-semgrep-suppression",
        "mivia.generic.no-unresolved-drift-markers",
        "mivia.generic.brand-mivialabs",
        "mivia.generic.commit-policy-no-optional-scope-wording",
        "mivia.generic.no-git-hook-bypass-in-agent-config",
        "mivia.generic.no-skill-freeform-output-heading",
        "mivia.generic.no-severity-gated-skill-approval",
        "mivia.go.no-shell-exec",
        "mivia.go.tests-no-time-sleep",
    ]:
        require(rule_id in semgrep_tests, f"scripts/test_semgrep_rules.py: missing {rule_id}")


def verify_secret_hygiene() -> None:
    scanned_roots = [
        "AGENTS.md",
        "CLAUDE.md",
        ".gitignore",
        ".agents",
        ".ai",
        ".claude",
        ".codex",
        ".github",
        ".githooks",
        "docs/setup/development-environment.md",
        "docs/agent-hooks.md",
        "docs/development-hooks.md",
        "README.md",
        "scripts/git-hooks",
        "scripts/install_git_hooks.sh",
        "scripts/test_semgrep_rules.py",
        "scripts/test_git_hooks.py",
        "scripts/agent_hook_guard.py",
        "scripts/run_agent_hook_guard.sh",
        "scripts/test_agent_hook_guard.py",
        "scripts/audit_loop_guard.py",
        "scripts/test_audit_loop_guard.py",
        "scripts/test_skill_contracts.py",
        "semgrep",
        "scripts/verify_agent_config.py",
    ]
    secret_patterns = [
        re.compile(r"sk-[A-Za-z0-9_-]{20,}"),
        re.compile(r"gh[pousr]_[A-Za-z0-9_]{20,}"),
        re.compile(r"AKIA[0-9A-Z]{16}"),
        re.compile(r"-----BEGIN (?:RSA |OPENSSH |EC |DSA )?PRIVATE KEY-----"),
    ]
    files: list[Path] = []
    for root in scanned_roots:
        path = repo_path(root)
        if path.is_file():
            files.append(path)
        elif path.is_dir():
            files.extend(p for p in path.rglob("*") if p.is_file())

    for path in files:
        content = path.read_text(encoding="utf-8")
        for pattern in secret_patterns:
            require(
                pattern.search(content) is None,
                f"{path.relative_to(ROOT)}: contains secret-shaped value matching {pattern.pattern}",
            )


def main() -> int:
    for path in [".claude/settings.json", ".codex/hooks.json", ".agents/skills.json"]:
        load_json(path)
    verify_agents_md()
    verify_index()
    verify_agent_quality_rules()
    verify_skills()
    verify_skill_report_contract()
    verify_adapters()
    verify_agents_hooks()
    verify_agent_hook_guard()
    verify_audit_loop_guard()
    verify_claude_settings()
    verify_codex_hooks()
    verify_gitignore()
    verify_git_hooks()
    verify_secret_hygiene()

    if FAILURES:
        for failure in FAILURES:
            print(f"FAIL: {failure}")
        return 1
    print("agent config verification passed")
    return 0


if __name__ == "__main__":
    sys.exit(main())
