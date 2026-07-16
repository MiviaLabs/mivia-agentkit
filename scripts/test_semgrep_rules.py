#!/usr/bin/env python3
"""Run contract tests for repo-local Semgrep rules."""

from __future__ import annotations

import json
import shutil
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
    "mivia.generic.commit-policy-no-optional-scope-wording",
    "mivia.generic.no-git-hook-bypass-in-agent-config",
    "mivia.generic.no-skill-freeform-output-heading",
    "mivia.generic.no-severity-gated-skill-approval",
    "mivia.generic.agent-plan-docs-must-reference-machine-plan",
    "mivia.generic.agent-planner-must-correct-plan-gaps",
    "mivia.generic.agent-plan-implementation-must-run-audit-loop",
    "mivia.generic.no-fake-only-runtime-coverage-guidance",
    "mivia.go.no-panic-in-internal",
    "mivia.go.no-fatal-exit-in-internal",
    "mivia.go.no-shell-exec",
    "mivia.go.no-syscall-exec",
    "mivia.go.no-network-calls",
    "mivia.go.no-world-writable-mode",
    "mivia.go.no-raw-artifact-write",
    "mivia.generic.real-integration-tests-no-fake-runner",
    "mivia.go.tests-use-t-tempdir",
    "mivia.go.tests-no-time-sleep",
}

SEMGREP_TIMEOUT_SECONDS = 60

# Semgrep ships a default ignore list that excludes /tmp (and several other
# system paths). tempfile.TemporaryDirectory() defaults to /tmp on Linux, which
# makes semgrep silently skip every fixture file (exit 7, scanned=[]). Using a
# directory under the user's home keeps fixtures off the ignored paths while
# still cleaning up after the test run.
IGNORED_TMP_DIRS = ("/tmp", "/var/tmp")

# Semgrep 1.167.0 intermittently exits 2 (internal crash) on this rule set with
# an empty result, instead of reporting findings. The rules themselves are
# stable -- when the run does not crash it reliably reports all expected rules.
# A bounded retry turns the crashing tool into a deterministic contract check.
SEMGREP_MAX_ATTEMPTS = 8


def fresh_temp_dir(prefix: str) -> str:
    """Return a temp directory that semgrep will not silently ignore."""
    default = tempfile.gettempdir()
    base = default if default not in IGNORED_TMP_DIRS else str(Path.home())
    return tempfile.mkdtemp(prefix=prefix, dir=base)


def write(path: Path, content: str) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(textwrap.dedent(content).lstrip(), encoding="utf-8")


def run_semgrep(target: Path, runner=subprocess.run) -> tuple[int, set[str], str]:
    files = sorted(str(path.relative_to(target)) for path in target.rglob("*") if path.is_file())
    try:
        proc = runner(
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
            timeout=SEMGREP_TIMEOUT_SECONDS,
        )
    except subprocess.TimeoutExpired as exc:
        return 124, set(), f"Semgrep timed out after {SEMGREP_TIMEOUT_SECONDS}s\n{exc.stderr or ''}"
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


def run_semgrep_reliably(target: Path) -> tuple[int, set[str], str]:
    """Run semgrep until it reports findings or attempts are exhausted.

    Semgrep 1.167.0 nondeterministically crashes (exit 2, empty result) on this
    rule set. A successful run is one that returns findings (exit 1 with rules);
    we retry up to SEMGREP_MAX_ATTEMPTS so the contract test reflects rule
    correctness rather than tool flakiness. The non-finding path (exit 0) and
    real errors (any non-{0,1,2} code) are returned immediately.
    """
    last = (1, set(), "")
    for _ in range(SEMGREP_MAX_ATTEMPTS):
        code, rules, stderr = run_semgrep(target)
        last = code, rules, stderr
        if code == 1 and rules:
            return code, rules, stderr
        if code == 0:
            return code, rules, stderr
        if code not in (1, 2):
            return code, rules, stderr
    return last


def test_run_semgrep_reports_invalid_json_stderr() -> None:
    with tempfile.TemporaryDirectory() as tmp:
        root = Path(tmp)
        write(root / "bad.go", "package bad\n")

        def runner(*args, **kwargs):
            return subprocess.CompletedProcess(args[0], 2, stdout="not-json", stderr="settings error")

        code, rules, stderr = run_semgrep(root, runner=runner)
        if code != 2 or rules:
            raise AssertionError(f"run_semgrep returned code={code} rules={rules}")
        if "invalid Semgrep JSON" not in stderr or "settings error" not in stderr:
            raise AssertionError(f"stderr did not preserve startup detail: {stderr!r}")


def test_run_semgrep_reports_timeout() -> None:
    with tempfile.TemporaryDirectory() as tmp:
        root = Path(tmp)
        write(root / "bad.go", "package bad\n")

        def runner(*args, **kwargs):
            raise subprocess.TimeoutExpired(args[0], kwargs.get("timeout"), stderr="partial stderr")

        code, rules, stderr = run_semgrep(root, runner=runner)
        if code != 124 or rules:
            raise AssertionError(f"run_semgrep timeout returned code={code} rules={rules}")
        if "timed out" not in stderr or "partial stderr" not in stderr:
            raise AssertionError(f"timeout stderr missing detail: {stderr!r}")


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
        Adapter tests use fake runners, not real CLIs.
        """,
    )
    write(
        root / "docs" / "development-hooks.md",
        """
        Commit subjects may use type(optional-scope): subject.
        """,
    )
    write(
        root / ".codex" / "AGENTS.md",
        """
        # Bad Adapter

        If hooks fail, run git commit --no-verify.
        """,
    )
    write(
        root / ".ai" / "skills" / "bad-skill" / "SKILL.md",
        """
        ---
        name: bad-skill
        description: Bad skill fixture.
        ---

        # Bad Skill

        ## Output

        Free-form report text.
        """,
    )
    write(
        root / ".ai" / "templates" / "bad-report.md",
        """
        # Bad Report

        Approve when there are no high severity issues.
        Allow a justified exception for remaining gaps.
        """,
    )
    write(
        root / "docs" / "plans" / "human" / "bad-agent-plan.md",
        """
        # Bad Agent Plan

        This is a markdown plan only, without machine plan validation.
        """,
    )
    write(
        root / ".ai" / "skills" / "agent-dag-planner" / "SKILL.md",
        """
        ---
        name: agent-dag-planner
        description: Bad planner.
        ---

        # Bad Planner

        Only report gaps and defer plan gaps.
        """,
    )
    write(
        root / ".ai" / "skills" / "agent-plan-implementer" / "SKILL.md",
        """
        ---
        name: agent-plan-implementer
        description: Bad implementer.
        ---

        # Bad Implementer

        Audit loop optional; skip audit loop for speed.
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
          "testing"
          "time"
        )

        func TestBad(t *testing.T) {
          _, _ = os.MkdirTemp("", "bad")
          time.Sleep(time.Millisecond)
        }
        """,
    )
    write(
        root / "test" / "integration" / "adapter_real_test.go",
        """
        package integration

        func TestBadRealCoverageUsesFakeRunner() {
          _ = NewFakeRunner()
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
    write(
        root / "test" / "integration" / "adapter_real_test.go",
        """
        package integration

        func TestGoodRealCoverageUsesSubprocessHarness() {}
        """,
    )


def main() -> int:
    test_run_semgrep_reports_invalid_json_stderr()
    test_run_semgrep_reports_timeout()

    # Use a non-ignored temp base: semgrep's default ignore list skips /tmp,
    # which would silently drop every fixture file (scanned=[]). Manual cleanup
    # replaces the context manager because mkdtemp does not self-clean.
    tmp = fresh_temp_dir(prefix="mivia-semgrep-rules-")
    try:
        bad_root = Path(tmp) / "bad"
        create_bad_fixture(bad_root)
        bad_code, bad_rules, bad_stderr = run_semgrep_reliably(bad_root)
        if bad_code != 1:
            print(f"FAIL: bad fixture expected Semgrep exit 1, got {bad_code}", file=sys.stderr)
            print(bad_stderr, file=sys.stderr)
            return 1
        missing = EXPECTED_RULES - bad_rules
        if missing:
            print(f"FAIL: bad fixture missed rules: {sorted(missing)}", file=sys.stderr)
            print(f"found: {sorted(bad_rules)}", file=sys.stderr)
            if bad_stderr:
                print(bad_stderr, file=sys.stderr)
            return 1

        good_root = Path(tmp) / "good"
        create_good_fixture(good_root)
        good_code, good_rules, good_stderr = run_semgrep_reliably(good_root)
        if good_code != 0 or good_rules:
            print(f"FAIL: good fixture expected no findings, got {sorted(good_rules)}", file=sys.stderr)
            print(good_stderr, file=sys.stderr)
            return 1
    finally:
        shutil.rmtree(tmp, ignore_errors=True)

    print("semgrep rule tests passed")
    return 0


if __name__ == "__main__":
    sys.exit(main())
