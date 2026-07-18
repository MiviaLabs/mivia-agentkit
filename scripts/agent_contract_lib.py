#!/usr/bin/env python3
"""Shared fail-closed contract helpers for plan/telemetry gates."""

from __future__ import annotations

import ast
import json
import re
from pathlib import Path
from urllib.parse import unquote


ROOT = Path(__file__).resolve().parents[1]

FALSE_COMMIT_PATHS = [
    "docs/examples/README.md",
    "docs/examples/zai-glm-examples.md",
    "docs/config-examples.md",
    "README.md",
    ".ai/workflows/zai-smoke-patch.yaml",
]

FORBIDDEN_FALSE_COMMIT_PATTERNS = [
    re.compile(r"approval:\s*commit\b(?!-)", re.I),
    re.compile(
        r"(?:approval:\s*)?protect:commit.{0,220}(?<!not )(?<!not a )(?<!not itself a )"
        r"(?:stages? (?:and )?commits?|performs a [Gg]it commit|is a [Gg]it commit|"
        r"lands? in the repo|allows the commits?|will commit|runs git commit|"
        r"does a [Gg]it commit|makes a [Gg]it commit|triggers a [Gg]it commit|"
        r"is itself a [Gg]it commit|causes a commit to land)\b",
        re.I | re.S,
    ),
    re.compile(
        r"`approval:\s*protect:commit`\s+is a protected action.{0,120}allows the commits?",
        re.I | re.S,
    ),
    re.compile(r"protect:commit\s+(?:field\s+)?(?:stages?|commits?)\b", re.I | re.S),
    re.compile(r"the protect:commit approval stages\b", re.I),
    re.compile(r"\bcommitted artifacts?\b", re.I),
]

POSITIVE_FALSE_COMMIT_FIXTURES = [
    "approval: commit",
    "protect:commit performs a Git commit",
    "protect:commit performs the Git commit",
    "approval: protect:commit\nThis stages and commits the repo",
    "The protect:commit field stages files",
    "protect:commit will commit your changes",
    "protect:commit will git commit your work",
    "protect:commit does a Git commit",
    "protect:commit is itself a Git commit",
    "protect:commit is the Git commit step",
    "protect:commit equals a Git commit",
    "protect:commit causes a commit to land in the repository",
    "protect:commit runs git commit",
    "protect:commit runs `git commit`",
    "protect:commit executes git commit",
    "protect:commit allows the commits",
    "the protect:commit approval stages the tree",
    "`approval: protect:commit` is a protected action that allows the commit",
    "protect:commit " + ("x" * 230) + " stages and commits",
    "protect:commit creates a commit",
    "protect:commit actually commits to git",
    "protect:commit commits to git",
    "protect:commit enables a commit",
    "protect:commit automatically commits",
    "protect:commit lets you commit",
    "protect:commit lands commits",
    "committed artifact",
    "committed artifacts",
]


def read_text(rel: str) -> str:
    return (ROOT / rel).read_text(encoding="utf-8")


_CLAIM_TOKEN = re.compile(
    r"(?is)("
    r"`?git[\s-]*commit`?"
    r"|stages?\s+and\s+commits?"
    r"|lands?\s+(?:commits?|in\s+the\s+repo)"
    r"|allows\s+the\s+commits?"
    r"|performs\s+(?:a|the)\s+`?git\s+commit`?"
    r"|executes\s+`?git\s+commit`?"
    r"|is\s+the\s+`?git\s+commit`?\s+step"
    r"|equals\s+a\s+`?git\s+commit`?"
    r"|will\s+`?git\s+commit`?"
    r"|runs\s+`?git\s+commit`?"
    r"|is\s+itself\s+a\s+`?git\s+commit`?"
    r"|causes\s+a\s+commit\s+to\s+land"
    r"|creates?\s+a\s+commit"
    r"|actually\s+commits?(?:\s+to\s+git)?"
    r"|commits?\s+to\s+git"
    r"|enables?\s+(?:a\s+)?commit"
    r"|auto(?:matically)?\s+commits?"
    r"|lets\s+you\s+commit"
    r")"
)
_NEGATED_CLAIM_CHUNK = re.compile(
    r"(?is)("
    r"not\s+a\s+`?git\s+commit`?"
    r"|not\s+itself\s+a\s+`?git\s+(?:stage\s+or\s+)?commit`?"
    r"|(?:does|do)\s+not\s+(?:stage|perform|run|execute)[^\n.]{0,80}`?git\s*commit`?"
    r"|(?:does|do)\s+not\s+stage\s+or\s+`?git\s*commit`?"
    r"|never\s+[^\n.]{0,40}`?git\s*commit`?"
    r"|(?:not|never|does\s+not|do\s+not|without)[^\n.]{0,80}"
    r"(?:stages?\s+and\s+commits?|lands?\s+in\s+the\s+repo|allows\s+the\s+commits?)"
    r")"
)


def check_false_commit_surfaces() -> list[str]:
    failures: list[str] = []
    for rel in FALSE_COMMIT_PATHS:
        path = ROOT / rel
        if not path.is_file():
            failures.append(f"missing false-commit surface: {rel}")
            continue
        body = path.read_text(encoding="utf-8")
        for pattern in FORBIDDEN_FALSE_COMMIT_PATTERNS:
            if pattern.search(body):
                failures.append(f"{rel}: false commit claim matching {pattern.pattern}")
        # Windowed claim scan: claim tokens after protect:commit are banned unless negated.
        for match in re.finditer(r"protect:commit", body, re.I):
            window = body[match.start() : match.start() + 800]
            window = re.sub(r"[`*]", "", window)  # strip markdown emphasis/code ticks for matching
            cleaned = _NEGATED_CLAIM_CHUNK.sub(" ", window)
            if _CLAIM_TOKEN.search(cleaned):
                failures.append(f"{rel}: protect:commit window contains unnegated Git-commit claim")
        if "protect:commit" in body:
            has_negation = any(
                needle in body
                for needle in (
                    "not a Git commit",
                    "does not stage or git commit",
                    "does not stage or `git commit`",
                    "not itself a Git stage or commit",
                )
            )
            if not has_negation:
                failures.append(f"{rel}: protect:commit without stamp/policy-not-Git clarifier")
    return failures


def _fixture_blocked(fixture: str) -> bool:
    if any(p.search(fixture) for p in FORBIDDEN_FALSE_COMMIT_PATTERNS):
        return True
    if "protect:commit" in fixture:
        for match in re.finditer(r"protect:commit", fixture, re.I):
            window = fixture[match.start() : match.start() + 800]
            window = re.sub(r"[`*]", "", window)
            cleaned = _NEGATED_CLAIM_CHUNK.sub(" ", window)
            if _CLAIM_TOKEN.search(cleaned):
                return True
    return False


def positive_false_commit_fixtures_blocked() -> list[str]:
    failures: list[str] = []
    for fixture in POSITIVE_FALSE_COMMIT_FIXTURES:
        if not _fixture_blocked(fixture):
            failures.append(f"fixture not blocked by forbidden patterns: {fixture!r}")
    if len(FORBIDDEN_FALSE_COMMIT_PATTERNS) < 4:
        failures.append("FORBIDDEN_FALSE_COMMIT_PATTERNS too small")
    if len(FALSE_COMMIT_PATHS) < 5:
        failures.append("FALSE_COMMIT_PATHS too small")
    return failures


def _fully_unquote(value: str) -> str:
    current = value
    for _ in range(8):
        decoded = unquote(current)
        if decoded == current:
            return decoded
        current = decoded
    raise ValueError(f"plan path percent-decoding did not stabilize: {value!r}")


def normalize_plan_path(path: str) -> str:
    raw = path.replace("\\", "/").strip()
    if raw.startswith("/") or raw.startswith("~"):
        raise ValueError(f"absolute plan path not allowed: {path!r}")
    raw = _fully_unquote(raw)
    if "%" in raw:
        raise ValueError(f"plan path retains percent-encoding: {path!r}")
    parts: list[str] = []
    for part in raw.split("/"):
        if part in ("", "."):
            continue
        if part == "..":
            raise ValueError(f"plan path must not contain '..': {path!r}")
        parts.append(part)
    return "/".join(parts)


def _normalize_dashes(value: str) -> str:
    # Collapse unicode hyphens/dashes to ASCII '-' for historical WS matching.
    return re.sub(r"[\u2010\u2011\u2012\u2013\u2014\u2212]", "-", value)


def scope_tree_prefix(entry: str) -> str | None:
    raw = entry.strip()
    # Normalize common tree-marker aliases to a directory prefix.
    for suffix in ("/**/**", "/**/*", "/**", "/*", "/"):
        if raw.endswith(suffix) and raw != suffix:
            return raw[: -len(suffix)].rstrip("/")
    if raw.endswith("**") and raw != "**":
        return raw.rstrip("*").rstrip("/")
    return None


def _historical_ws_banned(folded: str) -> bool:
    try:
        decoded = _normalize_dashes(_fully_unquote(folded).casefold())
    except ValueError:
        return True
    return bool(
        re.search(
            r"docs/plans/agentkit-implementation-roadmap/ws-(0\d|1[0-4])(?:$|/|-)",
            decoded,
        )
    )


def _tree_subsumes_historical_ws(tree_prefix: str) -> bool:
    """True when a tree marker can include historical WS0–WS14 task paths."""
    folded = _normalize_dashes(tree_prefix.casefold().rstrip("/"))
    hist = "docs/plans/agentkit-implementation-roadmap"
    if folded in {"docs", "docs/plans", hist}:
        return True
    if folded.startswith(hist + "/ws-") and re.search(r"/ws-(0\d|1[0-4])($|-)", folded):
        return True
    return False


def path_allowed_by_scope(path: str, scope_in: list[str], scope_out: list[str]) -> bool:
    tree_prefix = scope_tree_prefix(path)
    is_tree = tree_prefix is not None
    raw_for_norm = tree_prefix if is_tree else path
    norm = normalize_plan_path(raw_for_norm)
    folded = _normalize_dashes(norm.casefold())
    if is_tree and _tree_subsumes_historical_ws(norm):
        return False
    if _historical_ws_banned(folded):
        return False
    if folded == ".ai/runs" or folded.startswith(".ai/runs/"):
        return False
    for banned in scope_out:
        banned_raw = banned.strip()
        if not banned_raw:
            continue
        out_prefix = scope_tree_prefix(banned_raw)
        if out_prefix is None and banned_raw.endswith("/"):
            out_prefix = banned_raw.rstrip("/")
        if out_prefix is None and "/" not in banned_raw.rstrip("/") and not banned_raw.endswith(
            (".md", ".go", ".py", ".json")
        ):
            # bare single-segment dir token is a tree ban
            out_prefix = banned_raw.rstrip("/")
        if out_prefix is not None:
            if norm == out_prefix or norm.startswith(out_prefix + "/"):
                return False
            if is_tree:
                if norm == out_prefix or norm.startswith(out_prefix + "/") or out_prefix.startswith(norm + "/"):
                    return False
            continue
        banned_norm = banned_raw.rstrip("/")
        if "/" in banned_norm or banned_norm.endswith((".md", ".go", ".py", ".json")):
            if norm == banned_norm or norm.startswith(banned_norm + "/"):
                return False
    if (not is_tree and (norm in scope_in or path in scope_in)) or (is_tree and path in scope_in):
        return True
    for entry in scope_in:
        if entry in (path, norm):
            return True
        prefix = scope_tree_prefix(entry)
        if prefix is None:
            continue
        if not is_tree and (norm == prefix or norm.startswith(prefix + "/")):
            return True
        if is_tree and (norm == prefix or norm.startswith(prefix + "/")):
            return True
    return False


def check_supervised_plan_allowlist(plan_path: Path | None = None) -> list[str]:
    failures: list[str] = []
    path = plan_path or (ROOT / ".ai" / "plans" / "supervised-deep-bug-audit-repair-campaign.plan.json")
    if not path.is_file():
        return [f"missing plan: {path}"]
    parsed = json.loads(path.read_text(encoding="utf-8"))
    scope_in = list(parsed.get("scope", {}).get("in") or [])
    scope_out = [str(x) for x in (parsed.get("scope", {}).get("out") or [])]
    if not scope_in:
        failures.append("supervised plan scope.in empty")
        return failures
    for bad in [
        "templates-evil/**",
        "scripts-malware/**",
        "internal/configX/**",
        "templates/../evil",
        "docs/plans/agentkit-implementation-roadmap/ws-00-bootstrap/tasks.md",
        "docs/plans/agentkit-implementation-roadmap/WS-00-bootstrap/tasks.md",
        "docs/plans/agentkit-implementation-roadmap/ws-%30%30-bootstrap/tasks.md",
        "docs/plans/agentkit-implementation-roadmap/ws-%2530%2530-bootstrap/tasks.md",
        "docs/plans/agentkit-implementation-roadmap/ws-15-supervised-audit-repair-campaign/%2e%2e/ws-00-bootstrap/tasks.md",
        "docs/plans/%2e%2e/evil",
        "docs/plans/agentkit-implementation-roadmap/ws-15%2f../ws-00-bootstrap/tasks.md",
        "docs/plans/agentkit-implementation-roadmap/**",
        "docs/plans/agentkit-implementation-roadmap/**/*",
        "docs/plans/agentkit-implementation-roadmap/**/**",
        "docs/plans/**",
        "docs/plans/**/*",
        "docs/**",
        "docs/**/*",
        "docs/plans/agentkit-implementation-roadmap/ws-00\u2010bootstrap/tasks.md",
        "docs/plans/agentkit-implementation-roadmap/ws-00/tasks.md",
        "docs/plans/agentkit-implementation-roadmap/ws-00%2Fbootstrap/tasks.md",
        ".ai/runs/x",
        ".ai/runs/**",
    ]:
        try:
            ok = path_allowed_by_scope(bad, scope_in, scope_out)
        except ValueError:
            ok = False
        if ok:
            failures.append(f"scope allowlist incorrectly accepts {bad!r}")
    for node in parsed.get("dag", {}).get("nodes", []):
        for edit in node.get("files_edit") or []:
            try:
                ok = path_allowed_by_scope(edit, scope_in, scope_out)
            except ValueError as exc:
                failures.append(f"node {node.get('id')} path {edit!r} invalid: {exc}")
                continue
            if not ok:
                failures.append(
                    f"node {node.get('id')} files_edit path {edit!r} outside scope.in or hits scope.out"
                )
    return failures


def _call_target(call: ast.Call) -> str | None:
    if isinstance(call.func, ast.Name):
        return call.func.id
    if isinstance(call.func, ast.Attribute) and isinstance(call.func.value, ast.Name):
        return f"{call.func.value.id}.{call.func.attr}"
    return None


def _last_function_def(tree: ast.Module, name: str) -> ast.FunctionDef | None:
    found = None
    for node in tree.body:
        if isinstance(node, ast.FunctionDef) and node.name == name:
            found = node
    return found


def main_calls_via_ast(source: str, *, straight_line: bool = True) -> set[str]:
    """Name calls in the last def main().

    When straight_line=True (test scripts), any branch/loop before return
    invalidates the dual-home proof (empty set). When False (verify main),
    branch/loop statements are skipped and only top-level call statements count;
    post-return statements are ignored.
    """
    tree = ast.parse(source)
    main_fn = _last_function_def(tree, "main")
    if main_fn is None:
        return set()
    calls: set[str] = set()
    for stmt in main_fn.body:
        if isinstance(stmt, (ast.Pass, ast.Import, ast.ImportFrom)):
            continue
        if isinstance(stmt, ast.Expr) and isinstance(stmt.value, ast.Constant):
            continue
        if isinstance(stmt, (ast.Return, ast.Raise)):
            break
        if isinstance(
            stmt,
            (
                ast.If,
                ast.For,
                ast.AsyncFor,
                ast.While,
                ast.With,
                ast.AsyncWith,
                ast.Try,
                ast.Match,
                ast.FunctionDef,
                ast.ClassDef,
            ),
        ):
            if straight_line:
                return set()
            continue
        target = None
        if isinstance(stmt, ast.Expr) and isinstance(stmt.value, ast.Call):
            target = _call_target(stmt.value)
        elif isinstance(stmt, ast.Assign) and isinstance(stmt.value, ast.Call):
            target = _call_target(stmt.value)
        if target is None:
            if straight_line:
                return set()
            continue
        calls.add(target)
    return calls


def function_body_calls(source: str, function_name: str) -> set[str]:
    """Straight-line call targets in last matching function; empty if non-linear or bare-pass for-loop."""
    tree = ast.parse(source)
    fn = _last_function_def(tree, function_name)
    if fn is None:
        return set()
    found: set[str] = set()
    for stmt in fn.body:
        if isinstance(stmt, (ast.Pass, ast.Import, ast.ImportFrom)):
            continue
        if isinstance(stmt, ast.Expr) and isinstance(stmt.value, ast.Constant):
            continue
        if isinstance(stmt, (ast.Return, ast.Raise)):
            break
        if isinstance(
            stmt,
            (ast.If, ast.While, ast.With, ast.AsyncWith, ast.Try, ast.Match, ast.FunctionDef, ast.ClassDef),
        ):
            # Skip branch bodies; do not count them as dual-home proof.
            continue
        if isinstance(stmt, (ast.For, ast.AsyncFor)):
            if not isinstance(stmt.iter, ast.Call):
                continue
            target = _call_target(stmt.iter)
            if target:
                found.add(target)
            body_ok = False
            for body_stmt in stmt.body:
                if isinstance(body_stmt, ast.Raise):
                    body_ok = True
                if isinstance(body_stmt, ast.Expr) and isinstance(body_stmt.value, ast.Call):
                    t = _call_target(body_stmt.value)
                    if t in {"require", "fail"}:
                        body_ok = True
            if not body_ok:
                # bare for ...: pass is not dual-home proof for this helper
                found.discard(target or "")
            continue
        if isinstance(stmt, ast.Expr) and isinstance(stmt.value, ast.Call):
            target = _call_target(stmt.value)
            # Discarded contracts.check_*() / positive_*() calls are not dual-home proof.
            if target and not (
                target.startswith("contracts.check_")
                or target.startswith("contracts.positive_")
            ):
                found.add(target)
            continue
        if isinstance(stmt, ast.Assign) and isinstance(stmt.value, ast.Call):
            target = _call_target(stmt.value)
            if not target:
                continue
            # Bare assign of contracts.check_*/positive_* counts only when a later
            # straight-line sibling enforces the result (if failures: raise/require).
            if target.startswith("contracts.check_") or target.startswith("contracts.positive_"):
                # Look ahead in remaining body for fail-closed use of the assigned name.
                names = {t.id for t in stmt.targets if isinstance(t, ast.Name)}
                enforced = False
                # deferred: scan remaining stmts after this one
                # handled below by tagging; for simplicity require for-loop form for dual-home
                # of verify paths, and allow assign only if next non-empty stmt is If that raises.
                pass
            else:
                found.add(target)
            continue
        # other statements ignored
        continue
    # Second pass: Assign contracts.check_*() followed by if <name>: raise/require
    for idx, stmt in enumerate(fn.body):
        if not (isinstance(stmt, ast.Assign) and isinstance(stmt.value, ast.Call)):
            continue
        target = _call_target(stmt.value)
        if not target or not (
            target.startswith("contracts.check_") or target.startswith("contracts.positive_")
        ):
            continue
        names = {t.id for t in stmt.targets if isinstance(t, ast.Name)}
        if not names:
            continue
        for later in fn.body[idx + 1 :]:
            if isinstance(later, (ast.Return, ast.Raise)):
                break
            if isinstance(later, ast.If):
                # if failures: raise / require(...)
                test = later.test
                uses_name = isinstance(test, ast.Name) and test.id in names
                if uses_name:
                    for body_stmt in later.body:
                        if isinstance(body_stmt, ast.Raise):
                            found.add(target)
                        if isinstance(body_stmt, ast.Expr) and isinstance(body_stmt.value, ast.Call):
                            t = _call_target(body_stmt.value)
                            if t in {"require", "fail"}:
                                found.add(target)
                break
            if isinstance(later, (ast.Expr, ast.Assign, ast.For, ast.If)):
                # other statements may intervene; keep scanning a bit
                continue
            break
    return found


def entrypoint_calls_main(source: str) -> bool:
    """True if if __name__ == '__main__' unconditionally calls main()/sys.exit(main())."""
    tree = ast.parse(source)
    for node in tree.body:
        if not isinstance(node, ast.If):
            continue
        test = node.test
        ok_test = False
        if isinstance(test, ast.Compare) and len(test.ops) == 1 and isinstance(test.ops[0], ast.Eq):
            left, right = test.left, test.comparators[0]

            def is_name(n: ast.AST, s: str) -> bool:
                return isinstance(n, ast.Name) and n.id == s

            def is_const(n: ast.AST, s: str) -> bool:
                return isinstance(n, ast.Constant) and n.value == s

            if (is_name(left, "__name__") and is_const(right, "__main__")) or (
                is_const(left, "__main__") and is_name(right, "__name__")
            ):
                ok_test = True
        if not ok_test:
            continue
        for stmt in node.body:
            if isinstance(stmt, (ast.If, ast.For, ast.While, ast.Try, ast.With)):
                return False
            if isinstance(stmt, ast.Expr) and isinstance(stmt.value, ast.Call):
                call = stmt.value
                if isinstance(call.func, ast.Name) and call.func.id == "main":
                    return True
                if isinstance(call.func, ast.Attribute) and call.func.attr == "exit":
                    if call.args and isinstance(call.args[0], ast.Call):
                        inner = call.args[0]
                        if isinstance(inner.func, ast.Name) and inner.func.id == "main":
                            return True
        return False
    return False


def test_function_names(source: str) -> list[str]:
    tree = ast.parse(source)
    return [
        node.name
        for node in tree.body
        if isinstance(node, ast.FunctionDef) and node.name.startswith("test_")
    ]


def missing_main_test_calls(source: str) -> list[str]:
    names = test_function_names(source)
    calls = main_calls_via_ast(source)
    return [name for name in names if name not in calls]
