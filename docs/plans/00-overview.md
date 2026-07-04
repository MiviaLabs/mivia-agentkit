# Mivia AgentKit — Implementation Roadmap

- **PRD:** [`docs/prd/0001-mivia-agentkit.md`](../prd/0001-mivia-agentkit.md)
- **Product plan:** [`docs/mivia-agentic-workflow-cli-proposal-2026-07-04.md`](../mivia-agentic-workflow-cli-proposal-2026-07-04.md)
- **Conventions (read before any workstream):** [`_conventions.md`](_conventions.md)

This directory decomposes the product plan into **workstreams** (WS). Each WS has its own folder with a `tasks.md` that is file-by-file, test-by-test executable. Work in dependency order; do not start a WS whose dependencies are not green.

## Dependency graph

```
Phase 0                  Phase 1                       Phase 2
┌────────────┐   ┌─────────────────────────┐   ┌────────────────────────┐
│ WS0 boot   │──▶│ WS1 manifest/git/path   │──▶│ WS9 adapters           │
│            │   │ WS2 templates/init      │   │ WS4 preflight stamp    │
│            │   │ WS3 doctor/audit        │   └───────────┬────────────┘
└────────────┘   └────────────┬────────────┘               │
                              │                            ▼
                              │                   Phase 3
                              │          ┌────────────────────────┐
                              │          │ WS10 orchestrator       │
                              │          │ WS11 consensus          │
                              │          └───────────┬────────────┘
                              │                      │
                              │                      ▼
                              │              Phase 3/4
                              │          ┌────────────────────────┐
                              └─────────▶│ WS13 run/review/adapters│
                                         │ WS12 governance         │
                                         │ WS5 hooks               │
                                         └───────────┬────────────┘
                                                     │
                                                     ▼
                                             Phase 4/5
                                         ┌────────────────────────┐
                                         │ WS6 adapter templates   │
                                         │ WS7 import/update       │
                                         │ WS8 CI/release/docs     │
                                         └────────────────────────┘
```

## Workstream list

| WS | Folder | Title | Phase | Depends on | Status |
|---|---|---|---|---|---|
| 0 | [`ws-00-bootstrap/`](ws-00-bootstrap/tasks.md) | Repo bootstrap | 0 | — | ☐ |
| 1 | [`ws-01-manifest-git-pathpolicy/`](ws-01-manifest-git-pathpolicy/tasks.md) | Manifest, Git state, path policy, global config | 1 | WS0 | ☐ |
| 2 | [`ws-02-templates-init/`](ws-02-templates-init/tasks.md) | Templates + `init` | 1 | WS1 | ☐ |
| 3 | [`ws-03-doctor-audit/`](ws-03-doctor-audit/tasks.md) | `doctor` + `audit` | 1 | WS2 | ☐ |
| 4 | [`ws-04-preflight-stamp/`](ws-04-preflight-stamp/tasks.md) | Preflight + quality stamp | 2 | WS1 | ☐ |
| 9 | [`ws-09-adapters/`](ws-09-adapters/tasks.md) | Adapter system (Codex, Claude headless) | 2 | WS1 | ☐ |
| 10 | [`ws-10-orchestrator/`](ws-10-orchestrator/tasks.md) | Orchestrator (DAG + loops) | 3 | WS4, WS9 | ☐ |
| 11 | [`ws-11-consensus/`](ws-11-consensus/tasks.md) | Consensus voting | 3 | WS9 | ☐ |
| 12 | [`ws-12-governance/`](ws-12-governance/tasks.md) | Governance provider (noop + AGT) | 2,4 | WS1 | ☐ |
| 13 | [`ws-13-run-review-adapters/`](ws-13-run-review-adapters/tasks.md) | `run`, `review`, `adapters` commands | 2,3 | WS9, WS10, WS11, WS12 | ☐ |
| 5 | [`ws-05-hooks/`](ws-05-hooks/tasks.md) | Hook engine (Codex + Claude) | 4 | WS4, WS12 | ☐ |
| 6 | [`ws-06-adapter-templates/`](ws-06-adapter-templates/tasks.md) | Adapter templates (incl. Crush) | 4 | WS2, WS9 | ☐ |
| 7 | [`ws-07-import-update/`](ws-07-import-update/tasks.md) | `import` + `update` | 5 | WS2, WS3 | ☐ |
| 8 | [`ws-08-ci-release-docs/`](ws-08-ci-release-docs/tasks.md) | CI, release, docs | 5 | all prior | ☐ |

Numbering follows the product plan, not execution order. **Execute by phase, not by number.**

## How to use this

1. Read [`_conventions.md`](_conventions.md) — it defines the task format every WS uses.
2. Pick the lowest-phase WS whose dependencies are all ☑.
3. Open its `tasks.md`, work top-to-bottom, every task is one file + its test.
4. Run the WS's verification block at the end before marking the WS ☑.
5. Update the status column above when a WS completes.

## Phase exit gates (from PRD §14)

- **Phase 0:** `go test ./...` green; `--help` works.
- **Phase 1:** FR-1.1–1.3, 2.1, 2.3, 5.4, 6.1, 6.2, 7.5, 10.1–10.3, 10.6 green; idempotency + path-policy mutation proofs.
- **Phase 2:** FR-2.4, 3.1–3.5, 7.4 green; approval-enforcement + scrubbing tests.
- **Phase 3:** FR-4.1–4.5, 5.1–5.3, 6.3 green; loop-bound + stamp-before-protect + consensus-threshold mutation proofs.
- **Phase 4:** FR-2.2, 6.4, 7.1, 7.2, 8.1–8.3 green; strict-requires-AGT doctor failure.
- **Phase 5:** FR-1.4, 9.1, 9.2; release binaries build for linux/macOS/windows.
