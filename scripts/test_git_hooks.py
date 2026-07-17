#!/usr/bin/env python3
"""Contract tests for repo-managed Git hooks."""

from __future__ import annotations

import subprocess
import sys
import tempfile
import shutil
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
PREPARE_HOOK = ROOT / "scripts" / "git-hooks" / "prepare-commit-msg"
COMMIT_MSG_HOOK = ROOT / "scripts" / "git-hooks" / "commit-msg"
COMMIT_POLICY = ROOT / ".ai" / "policy" / "commit-message.json"


def run(args: list[str], cwd: Path, *, check: bool = True) -> subprocess.CompletedProcess[str]:
    proc = subprocess.run(args, cwd=cwd, text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=False)
    if check and proc.returncode != 0:
        raise AssertionError(f"{args!r} failed with {proc.returncode}\nstdout={proc.stdout}\nstderr={proc.stderr}")
    return proc


def init_repo(root: Path) -> None:
    root.mkdir(parents=True, exist_ok=True)
    run(["git", "init"], root)
    run(["git", "config", "user.email", "hook-test@example.invalid"], root)
    run(["git", "config", "user.name", "Hook Test"], root)
    run(["git", "config", "commit.gpgsign", "false"], root)
    (root / "file.txt").write_text("content\n", encoding="utf-8")
    policy_path = root / ".ai" / "policy" / "commit-message.json"
    policy_path.parent.mkdir(parents=True, exist_ok=True)
    shutil.copyfile(COMMIT_POLICY, policy_path)
    run(["git", "add", "file.txt"], root)


def write_summary(root: Path, summary: str, *, tree: str | None = None) -> None:
    if tree is None:
        tree = run(["git", "write-tree"], root).stdout.strip()
    git_dir = run(["git", "rev-parse", "--git-dir"], root).stdout.strip()
    (root / git_dir / "mivia-agent-precommit-summary").write_text(
        f"tree={tree}\nsummary={summary}\n",
        encoding="utf-8",
    )


def test_prepare_commit_msg_appends_summary(root: Path) -> None:
    init_repo(root)
    summary = "Quality: pre-commit passed (agent config verification passed, whitespace passed, semgrep rules passed, hook tests passed, agent hook tests passed, audit loop tests passed, plan contract tests passed, skill contract tests passed, telemetry contract tests passed, staged semgrep 0 findings; gofmt skipped)"
    write_summary(root, summary)
    msg = root / "COMMIT_MSG"
    msg.write_text("chore: test hooks\n", encoding="utf-8")

    run([str(PREPARE_HOOK), str(msg), "message"], root)
    first = msg.read_text(encoding="utf-8")
    if summary not in first:
        raise AssertionError("prepare-commit-msg did not append quality summary")

    run([str(PREPARE_HOOK), str(msg), "message"], root)
    second = msg.read_text(encoding="utf-8")
    if second.count(summary) != 1:
        raise AssertionError("prepare-commit-msg duplicated quality summary")


def test_prepare_commit_msg_rejects_stale_summary(root: Path) -> None:
    init_repo(root)
    old_tree = run(["git", "write-tree"], root).stdout.strip()
    (root / "other.txt").write_text("new\n", encoding="utf-8")
    run(["git", "add", "other.txt"], root)
    summary = "Quality: pre-commit passed (stale)"
    write_summary(root, summary, tree=old_tree)
    msg = root / "COMMIT_MSG_STALE"
    msg.write_text("chore: stale\n", encoding="utf-8")

    run([str(PREPARE_HOOK), str(msg), "message"], root)
    if summary in msg.read_text(encoding="utf-8"):
        raise AssertionError("prepare-commit-msg appended stale quality summary")


def test_commit_msg_accepts_conventional_subject(root: Path) -> None:
    init_repo(root)
    msg = root / "COMMIT_MSG_VALID"
    msg.write_text(
        "chore(hooks): enforce commit message convention\n\n"
        "Quality: pre-commit passed (agent config verification passed)\n",
        encoding="utf-8",
    )

    run([str(COMMIT_MSG_HOOK), str(msg)], root)
    content = msg.read_text(encoding="utf-8")
    if "commit message passed" not in content:
        raise AssertionError("commit-msg did not record commit message result")


def test_commit_msg_rejects_invalid_subject(root: Path) -> None:
    init_repo(root)
    msg = root / "COMMIT_MSG_INVALID"
    msg.write_text("bad message\n", encoding="utf-8")

    proc = run([str(COMMIT_MSG_HOOK), str(msg)], root, check=False)
    if proc.returncode == 0:
        raise AssertionError("commit-msg accepted invalid subject")
    if "expected format" not in proc.stderr:
        raise AssertionError(f"commit-msg error did not explain convention: {proc.stderr}")


def test_commit_msg_requires_scope(root: Path) -> None:
    init_repo(root)
    msg = root / "COMMIT_MSG_NO_SCOPE"
    msg.write_text("chore: missing scope\n", encoding="utf-8")

    proc = run([str(COMMIT_MSG_HOOK), str(msg)], root, check=False)
    if proc.returncode == 0:
        raise AssertionError("commit-msg accepted subject without scope")


def test_commit_msg_rejects_unknown_scope(root: Path) -> None:
    init_repo(root)
    msg = root / "COMMIT_MSG_BAD_SCOPE"
    msg.write_text("chore(random): unknown scope\n", encoding="utf-8")

    proc = run([str(COMMIT_MSG_HOOK), str(msg)], root, check=False)
    if proc.returncode == 0:
        raise AssertionError("commit-msg accepted unknown scope")


def main() -> int:
    with tempfile.TemporaryDirectory() as tmp:
        test_prepare_commit_msg_appends_summary(Path(tmp) / "append")
        test_prepare_commit_msg_rejects_stale_summary(Path(tmp) / "stale")
        test_commit_msg_accepts_conventional_subject(Path(tmp) / "valid")
        test_commit_msg_rejects_invalid_subject(Path(tmp) / "invalid")
        test_commit_msg_requires_scope(Path(tmp) / "no-scope")
        test_commit_msg_rejects_unknown_scope(Path(tmp) / "bad-scope")
    print("git hook tests passed")
    return 0


if __name__ == "__main__":
    sys.exit(main())
