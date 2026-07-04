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
        "docs/development-hooks.md",
    ]:
        require(path in content, f".ai/INDEX.md: missing hook verification path {path}")


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
    require(set(hooks) == {"PreToolUse", "Stop"}, ".claude/settings.json: unexpected hook events")

    event_commands = {
        "PreToolUse": "mivia-agent hook claude pre-tool-use",
        "Stop": "mivia-agent hook claude stop",
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
        "UserPromptSubmit": "mivia-agent hook codex user-prompt-submit",
        "PreToolUse": "mivia-agent hook codex pre-tool-use",
        "PermissionRequest": "mivia-agent hook codex permission-request",
        "Stop": "mivia-agent hook codex stop",
    }
    for event, expected in event_commands.items():
        commands = [
            hook.get("command", "")
            for group in hooks.get(event, [])
            for hook in group.get("hooks", [])
            if isinstance(hook, dict)
        ]
        require(any(expected in command for command in commands), f".codex/hooks.json: missing {expected}")


def verify_gitignore() -> None:
    entries = set(line.strip() for line in text(".gitignore").splitlines() if line.strip())
    for entry in [
        ".ai/runs/",
        ".git/mivia-agent-quality-stamp.json",
        ".claude/settings.local.json",
        ".semgrep-cache/",
        ".env",
        ".env.*",
        "secrets/",
    ]:
        require(entry in entries, f".gitignore: missing {entry}")


def verify_git_hooks() -> None:
    required_files = [
        ".githooks/pre-commit",
        ".githooks/pre-push",
        "scripts/git-hooks/pre-commit",
        "scripts/git-hooks/pre-push",
        "scripts/install_git_hooks.sh",
        "semgrep/agent-standards.yml",
        "docs/development-hooks.md",
    ]
    for path in required_files:
        require(repo_path(path).is_file(), f"{path}: missing")

    for path in required_files[:5]:
        require(os.access(repo_path(path), os.X_OK), f"{path}: must be executable")

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

    pre_commit = text("scripts/git-hooks/pre-commit")
    for needle in [
        "scripts/verify_agent_config.py",
        "gofmt -w",
        "git diff --check --cached",
        "semgrep --validate --config semgrep/agent-standards.yml",
        "semgrep --config semgrep/agent-standards.yml",
    ]:
        require(needle in pre_commit, f"scripts/git-hooks/pre-commit: missing {needle}")

    pre_push = text("scripts/git-hooks/pre-push")
    for needle in [
        "scripts/verify_agent_config.py",
        "git diff --check",
        "semgrep --validate --config semgrep/agent-standards.yml",
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
        "mivia.go.no-panic-in-internal",
        "mivia.go.no-fatal-exit-in-internal",
        "mivia.go.no-network-calls",
        "mivia.go.no-raw-artifact-write",
        "mivia.go.tests-no-real-agent-cli",
    ]:
        require(rule_id in semgrep_config, f"semgrep/agent-standards.yml: missing {rule_id}")


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
        "docs/development-hooks.md",
        "scripts/git-hooks",
        "scripts/install_git_hooks.sh",
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
    verify_skills()
    verify_adapters()
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
