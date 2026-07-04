# Conventions — read before any workstream

Every `tasks.md` in this directory follows the format below. Do not free-form it.

## Task format

```markdown
## T<n> — <short title>

Create:
- `path/to/file.go` — <one-line purpose> (<key symbols>)
- `path/to/file_test.go` — <test suite>

Spec:
- <numbered behavioral bullets — what the code MUST do>

Tests that must pass:
- `TestXxxYyy`
- `TestZzz`

Dependencies:
- <packages, internal packages, or "stdlib only">

Mutation proof:
- <exact code change>; `<test>` must fail.

Notes:
- <optional: rationale, gotchas, links to plan/PRD sections>
```

A task is **one production file + its test file**. If a task needs two production files, split it.

## Invariants for every workstream

1. **Test-first where feasible.** Write the listed tests before or alongside the production code. Tests are not optional and not deferred.
2. **Every guard has a mutation proof.** For any "must reject / must deny / must fail closed" behavior, there is a `Mutation proof:` line. To prove it: make the described code change, confirm the named test fails, then revert. Record the result in the WS completion report.
3. **No network.** mivia-agent makes zero network calls itself. Tests must not hit the network. Real integration coverage may execute local binaries and local agent CLIs, but it must stay offline.
4. **No mocking of the thing under test.** If the risk is Git behavior, use a real temp Git repo (`git init` in `t.TempDir()`). If the risk is the hook output shape, assert against the real shape. Mock only collaborators. Fake runners are allowed for unit tests, but every shipped command and approved adapter also needs a real subprocess or built-binary integration path.
5. **Idempotency.** Anything that writes must be re-runnable with no diff under the same inputs. There is an idempotency test for every writer.
6. **Secret hygiene.** No test fixture, sample payload, or persisted artifact contains plausible secrets. Adapter result types never carry raw prompts or raw model output.
7. **File headers.** Every `.go` file starts with a package doc comment naming the WS that owns it and the plan/PRD sections it implements, e.g.:
   ```go
   // Package config implements mivia-agent.yaml parsing.
   // Plan: WS1. PRD: FR-1.1, FR-4.2.
   package config
   ```

## Verification block (end of every tasks.md)

```markdown
## Verification

```bash
go test ./<this-ws-pkgs>/... -count=1
go vet ./<this-ws-pkgs>/...
```

WS <id> is ☑ when:
- [ ] all listed tests pass
- [ ] all mutation proofs executed and reverted (results in completion report)
- [ ] `go vet` clean for this WS's packages
- [ ] no network calls added (grep for `http.`, `net.Dial`, `os/exec` outside adapter fakes)
- [ ] status updated in `00-overview.md`
```

## Completion report

When a WS is done, append to its `tasks.md`:

```markdown
## Completion — <date>

- Tests: <count> passing.
- Mutation proofs: <list, each "pass/fail-then-revert ok">.
- Files: <count> created.
- Residual risk: <none / describe>.
- Follow-ups: <none / issues>.
```

## Glossary used in tasks

- **Manifest** — `mivia-agent.yaml` at the repo root.
- **Global manifest** — `~/.agents/mivia.yaml` (optional, user-managed, layered under project manifest).
- **Global rules/skills** — `~/.agents/rules/` and `~/.agents/skills/` (optional, layered under `.ai/` equivalents).
- **Config hierarchy** — two-layer: global (`~/.agents/`, per-user, lowest priority) → project (`.ai/`, per-repo, highest priority).
- **Stamp** — `.git/mivia-agent-quality-stamp.json`.
- **Adapter** — `internal/adapter.Adapter` implementation (WS9).
- **Loop** — a named workflow in the manifest or `.ai/workflows/*.yaml` (WS10).
- **Verdict** — a reviewer's structured pass/fail on an artifact (WS9/WS11).
- **Decision** — a governance `policy.Decision` on a protected action (WS12).
- **Protected action** — commit, push, PR, deploy, release, live-smoke.

## Package map (canonical)

```
cmd/mivia-agent/         main entry
internal/cli/            cobra commands
internal/config/         manifest
internal/globalconfig/   ~/.agents/ reading + layering under project config
internal/detect/         language/tooling detection
internal/gitstate/       git root, HEAD, changed files, diff hash
internal/pathpolicy/     path allow/deny
internal/render/         template rendering
internal/templates/      embedded templates
internal/doctor/         doctor checks
internal/audit/          audit findings
internal/preflight/      stamp write/check
internal/hooks/          hook engine + codex/claude emitters
internal/adapter/        Adapter interface + per-CLI impls
internal/orchestrator/   DAG + loop engine
internal/consensus/      voting + tie-breakers
internal/policy/         governance provider (noop + agt)
internal/runstore/       .ai/runs/<id>/ storage
internal/importer/       import (WS7)
internal/update/         update (WS7)
internal/report/         text + json reporting
```

When a task creates a file outside this map, the task notes it explicitly and the overview's map is updated when the WS lands.
