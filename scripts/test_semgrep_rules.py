#!/usr/bin/env python3
"""Run contract tests for repo-local Semgrep rules."""

from __future__ import annotations

import json
import subprocess
import sys
import tempfile
import textwrap
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
CONFIG = ROOT / "semgrep" / "agent-standards.yml"

EXPECTED_RULES = {
    "mivia.generic.no-wildcard-bash-allow",
    "mivia.generic.no-shell-metachar-bash-allow",
    "mivia.generic.no-semgrep-suppression",
    "mivia.generic.no-unresolved-drift-markers",
    "mivia.generic.brand-mivialabs",
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
}


def write(path: Path, content: str) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(textwrap.dedent(content).lstrip(), encoding="utf-8")


def run_semgrep(target: Path) -> tuple[int, set[str], str]:
    files = sorted(str(path.relative_to(target)) for path in target.rglob("*") if path.is_file())
    proc = subprocess.run(
        [
            "semgrep",
            "--json",
            "--config",
            str(CONFIG),
            "--error",
            "--skip-unknown-extensions",
            "--metrics",
            "off",
            "--disable-nosem",
            *files,
        ],
        cwd=target,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        check=False,
    )
    try:
        payload = json.loads(proc.stdout or "{}")
    except json.JSONDecodeError as exc:
        return proc.returncode, set(), f"invalid Semgrep JSON: {exc}\n{proc.stderr}"
    rule_ids = set()
    for item in payload.get("results", []):
        check_id = item["check_id"]
        if ".semgrep." in check_id:
            check_id = check_id.split(".semgrep.", 1)[1]
        rule_ids.add(check_id)
    return proc.returncode, rule_ids, proc.stderr


def create_bad_fixture(root: Path) -> None:
    write(
        root / ".claude" / "settings.json",
        """
        {
          "permissions": {
            "allow": [
              "Bash(go test *)",
              "Bash(git status; git push)"
            ]
          }
        }
        """,
    )
    write(
        root / "AGENTS.md",
        """
        # Agent Rules

        TODO: remove this drift marker.

        The brand must not be written as Mivia Labs.
        """,
    )
    write(
        root / "internal" / "bad" / "bad.go",
        """
        package bad

        import (
          "log"
          "net/http"
          "os"
          "os/exec"
          "syscall"
        )

        func bad() {
          // nosemgrep: mivia.go.no-panic-in-internal
          panic("bad")
          log.Fatal("bad")
          os.Exit(1)
          _, _ = http.Get("https://example.invalid")
          _ = exec.Command("sh", "-c", "echo bad").Run()
          _ = syscall.Exec("/bin/sh", []string{"sh", "-c", "echo bad"}, nil)
          _ = os.WriteFile("artifact.txt", []byte("bad"), 0777)
        }

        func rawWrite(rawPrompt []byte) {
          _ = os.WriteFile("raw.txt", rawPrompt, 0600)
        }
        """,
    )
    write(
        root / "internal" / "bad" / "bad_test.go",
        """
        package bad

        import (
          "os"
          "os/exec"
          "testing"
          "time"
        )

        func TestBad(t *testing.T) {
          _, _ = os.MkdirTemp("", "bad")
          _ = exec.Command("codex", "run").Run()
          time.Sleep(time.Millisecond)
        }
        """,
    )


def create_good_fixture(root: Path) -> None:
    write(
        root / ".claude" / "settings.json",
        """
        {
          "permissions": {
            "allow": [
              "Bash(go test ./...)",
              "Bash(git status)"
            ]
          }
        }
        """,
    )
    write(
        root / "internal" / "good" / "good.go",
        """
        package good

        import "errors"

        func run() error {
          return errors.New("not implemented")
        }
        """,
    )
    write(
        root / "README.md",
        """
        # MiviaLabs

        This fixture uses the correct brand spelling.
        """,
    )
    write(
        root / "internal" / "good" / "good_test.go",
        """
        package good

        import "testing"

        func TestGood(t *testing.T) {
          dir := t.TempDir()
          if dir == "" {
            t.Fatal("empty temp dir")
          }
        }
        """,
    )


def main() -> int:
    with tempfile.TemporaryDirectory() as tmp:
        bad_root = Path(tmp) / "bad"
        create_bad_fixture(bad_root)
        bad_code, bad_rules, bad_stderr = run_semgrep(bad_root)
        if bad_code != 1:
            print(f"FAIL: bad fixture expected Semgrep exit 1, got {bad_code}", file=sys.stderr)
            print(bad_stderr, file=sys.stderr)
            return 1
        missing = EXPECTED_RULES - bad_rules
        if missing:
            print(f"FAIL: bad fixture missed rules: {sorted(missing)}", file=sys.stderr)
            print(f"found: {sorted(bad_rules)}", file=sys.stderr)
            return 1

        good_root = Path(tmp) / "good"
        create_good_fixture(good_root)
        good_code, good_rules, good_stderr = run_semgrep(good_root)
        if good_code != 0 or good_rules:
            print(f"FAIL: good fixture expected no findings, got {sorted(good_rules)}", file=sys.stderr)
            print(good_stderr, file=sys.stderr)
            return 1

    print("semgrep rule tests passed")
    return 0


if __name__ == "__main__":
    sys.exit(main())
