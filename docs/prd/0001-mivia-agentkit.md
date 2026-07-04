# PRD-0001 — Mivia AgentKit

- **Status:** Draft v2 (validated against implementation plan revision 2)
- **Owner:** Mivia Labs
- **Date:** 2026-07-04
- **Implementation plan:** `docs/mivia-agentic-workflow-cli-proposal-2026-07-04.md`
- **Replaces:** —

## 1. Summary

Mivia AgentKit (`mivia-agent`) is a standalone, Mivia-branded CLI that prepares any Git repository for high-rigor agentic software workflows **and orchestrates those workflows across multiple agent CLIs**. It installs a generic agent-control surface (instructions, skills, hooks, quality gates, contract matrices, loop definitions, review policies) plus a thin, swappable adapter layer for agent tools. It then executes workflows by invoking those adapters headlessly, routing artifacts between them, running cross-CLI consensus review, and letting deterministic local gates decide whether risky work can finish.

In MVP, governance runs in a no-op provider that allows all actions but still records them; the Microsoft Agent Governance Toolkit (AGT) is wired as an opt-in, interface-backed provider with full enforcement targeted at the strict profile post-MVP.

## 2. Problem

Teams adopt coding-agent CLIs (Codex, Claude Code, Gemini CLI, Crush, Copilot) one at a time, and each brings its own configuration format, hook model, and approval semantics. The result is:

1. **No canonical control surface.** Policy is duplicated across `AGENTS.md`, `CLAUDE.md`, `.codex/`, `.claude/`, Copilot instructions, and CI, and drifts.
2. **No cross-CLI verification.** One agent's output is rarely checked by another before it lands. There is no primitive for "agent B reviews agent A's work."
3. **No composable loops.** Research, bug-audit, and fix-review are ad hoc and unbounded; they cannot run safely in CI.
4. **Advisory guidance is mistaken for enforcement.** Instructions are suggestions; teams ship assuming deterministic gates exist where they do not.
5. **Tool lock-in.** Workflows are wired to one CLI; swapping or adding a tool is a rewrite.

## 3. Vision

> Agent guidance is advisory; agent execution is orchestrated; deterministic local gates decide whether risky work can finish.

A single distributable binary that:

- Configures a repo for any mix of agent CLIs from one canonical source of truth (`.ai/`).
- Orchestrates loops across those CLIs — produce → consensus-review → iterate — headlessly and boundedly.
- Makes every CLI behind a swappable `Adapter` interface, so adding a tool is one adapter, not a rewrite.
- Gates risky work behind a local quality stamp and (opt-in) a real governance engine, with a no-op fallback so the binary still ships standalone.

## 4. Goals / Non-Goals

### Goals

- One command (`init`) installs the full control surface for any adapter mix.
- One command (`run`) executes named loops by orchestrating adapters headlessly, with bounded iterations and consensus review.
- One command (`review`) runs a one-off cross-CLI consensus review.
- Deterministic gates (quality stamp + policy decisions) block protected actions: commit, push, PR, deploy, release, live-smoke.
- Adapter-based: Codex, Claude Code, Gemini CLI, Crush (orchestrable, Crush pending headless verification); Copilot (guidance). Adding a CLI = writing one adapter.
- Configurable loops in `mivia-agent.yaml`: research, bug-audit, fix-review, release-audit — bounded by iterations (budget mode is post-MVP).
- Optional governance backbone via AGT; standalone by default.
- Single Go binary; minimal dependencies; no required service, database, container engine, or cloud account.

### Non-Goals

- Not a hosted service.
- No dependency on any existing application codebase.
- No live connectors (Jira/Slack/GitHub data) by default.
- No auto push/PR/deploy/remote changes.
- No Dagger, Temporal, Kubernetes, or external workflow runtime required.
- No storage of secrets, raw prompts, raw model output, or provider payloads.
- No "expert"/unbounded budget loops in MVP.
- No "pi agent" integration (explicitly dropped).

## 5. Target users

- **Solo dev / OSS maintainer:** wants a one-command setup that makes their agent honest about what it verified.
- **Platform/DevEx team:** wants one canonical agent config across many repos and the ability to add a new CLI without rewriting policy.
- **Automation-heavy team:** wants agent loops (research, bug-audit) that are safe to run in CI with deterministic gates.

## 6. Personas & primary jobs

| Persona | Primary job |
|---|---|
| Owner | `init` the repo for my adapter mix; trust `doctor` to tell me what's broken. |
| Reviewer | `review` an artifact across 2–3 CLIs and get a consensus verdict. |
| Workflow author | Define a loop in YAML (produce → review → iterate) that runs in CI. |
| CI pipeline | `doctor --json` gates the build; optionally `run --workflow <name>` executes a loop. |
| Agent session | Hook fires; protected action is denied until a fresh stamp + passing policy decision exist. |

## 7. Key concepts

- **`.ai/`** — canonical agent-control surface (rules, skills, workflows, quality contracts, review policies). Root/vendor files are thin adapters pointing here.
- **Adapter** — swappable interface (`Detect`/`Run`/`Review`) wrapping one CLI. `orchestrable` adapters can be invoked headlessly by `run`; `guidance` adapters (Copilot) only receive instruction files.
- **Loop** — named, bounded workflow of steps. `bound: iterations` (MVP) or `bound: budget` (post-MVP). Has `exit_when` gate and `on_exhausted` policy.
- **Routing** — how artifacts flow between steps: sequential handoff, parallel fan-out, or conditional edges on gate outcomes.
- **Consensus review** — a review step dispatches one artifact to N reviewer adapters in parallel; a voting policy (`majority`/`unanimous`/`weighted`/`first-pass`) plus a tie-breaker decides pass/fail/iterate.
- **Quality stamp** — local artifact under `.git/mivia-agent-quality-stamp.json` recording HEAD, diff hash, changed files, verifiers, mutation proofs, and policy-decision refs. **Stale if any of HEAD, diff hash, or changed-files set changes.**
- **Governance provider** — `noop` (default; allows all, records to `.ai/audit.jsonl`) or `agt` (Microsoft AGT; deterministic `Decide()` before protected actions and loop steps, tamper-evident audit).

## 8. Functional requirements

Traceability column: **WS** = implementation workstream, **Ph** = release phase (see §12 / plan).

### FR-1 Setup
| ID | Requirement | WS | Ph |
|---|---|---|---|
| FR-1.1 | `init` generates the full `.ai/` surface + thin adapters for the selected adapter mix and profile (`starter`/`standard`/`strict`). | WS2 | 1 |
| FR-1.2 | `init --write` is idempotent: same options → no diff (asserted by `TestInitWriteIsIdempotent`). | WS2 | 1 |
| FR-1.3 | `init` never overwrites user-owned files without `--force` (asserted by `TestInitRefusesToOverwriteUserOwnedFile`). | WS2 | 1 |
| FR-1.4 | `update` refreshes managed template blocks only, preserving user edits. | WS7 | 5 |

### FR-2 Validation
| ID | Requirement | WS | Ph |
|---|---|---|---|
| FR-2.1 | `doctor` validates generated wiring, hook entrypoints, skills, CI, loop definitions, consensus satisfiability, and governance provider. | WS3 | 1 |
| FR-2.2 | `doctor` fails if `governance.provider: agt` is set under the strict profile and the AGT dependency is unavailable. | WS3/WS12 | 4 |
| FR-2.3 | `audit` reports quality gaps (missing canonical `.ai/`, duplicated policy, weak consensus, missing review-before-protect, etc.) without writing. | WS3 | 1 |
| FR-2.4 | `preflight` validates the current diff and writes a stamp; stale on any change to HEAD, diff hash, or changed-files set. | WS4 | 2 |

### FR-3 Adapters
| ID | Requirement | WS | Ph |
|---|---|---|---|
| FR-3.1 | Each CLI is wrapped by an `Adapter` implementing `Detect`/`Run`/`Review`. | WS9 | 2 |
| FR-3.2 | `adapters` reports each adapter's presence, headless capability, approval mode, and whether it is approved for `run`. | WS9/WS13 | 2 |
| FR-3.3 | Adapters enforce non-interactive approval mode, max turns, and timeout on every `Run` (asserted by `Test*RunEnforcesNonInteractiveApproval`). | WS9 | 2 |
| FR-3.4 | An adapter that cannot be made non-interactive is flagged and excluded from `run` (asserted by `TestAdapterReturnsErrorWhenHeadlessNotCapable`). | WS9 | 2 |
| FR-3.5 | Adapter results never contain raw prompts or raw model output — only the declared artifact and scrubbed metadata. | WS9 | 2 |

### FR-4 Orchestration
| ID | Requirement | WS | Ph |
|---|---|---|---|
| FR-4.1 | `run --workflow <name>` resolves a loop to a DAG and executes it: producers sequentially, reviewers in parallel. | WS10/WS13 | 3 |
| FR-4.2 | Loops are bounded: `iterations` in MVP; both `bound: budget` **and** the `expert` profile are rejected with a clear error (asserted by `TestManifestRejectsBudgetBoundInMVP`, `TestManifestRejectsExpertProfileInMVP`). | WS1/WS10 | 1,3 |
| FR-4.3 | `exit_when.gate` short-circuits a loop on pass; `on_exhausted` decides fail/warn/proceed at bound exhaustion (asserted by `TestLoopExitsWhenGatePasses`, `TestLoopFailsOnExhaustionWithOnExhaustedFail`). | WS10 | 3 |
| FR-4.4 | `run --dry-run` prints the execution plan without invoking adapters (asserted by `TestRunDryRunPrintsPlanWithoutInvoking`). | WS13 | 3 |
| FR-4.5 | Every run writes a JSONL trace + summary under `.ai/runs/<run-id>/`. | WS10 | 3 |

### FR-5 Consensus review
| ID | Requirement | WS | Ph |
|---|---|---|---|
| FR-5.1 | A review step fans out one artifact to N reviewers concurrently and returns structured `Verdict`s. | WS10/WS11 | 3 |
| FR-5.2 | Consensus modes: `majority`, `unanimous`, `weighted`, `first-pass`. Tie-breakers: `strict`, `manual`, `prefer:<adapter>`. | WS11 | 3 |
| FR-5.3 | `review` command runs a one-off consensus review without a full loop. | WS13 | 3 |
| FR-5.4 | `min_reviewers` unsatisfiable by enabled headless adapters is a `doctor`/`audit` failure. | WS3 | 1 |

### FR-6 Loops (configurable)
| ID | Requirement | WS | Ph |
|---|---|---|---|
| FR-6.1 | Users declare loops in `mivia-agent.yaml` and/or `.ai/workflows/*.yaml`. | WS2 | 1 |
| FR-6.2 | Shipped loop templates: `research-loop`, `bug-audit-loop` (standard profile, disabled by default). | WS2 | 1 |
| FR-6.3 | Loops support `on_fail: iterate` (route reviewer notes back to the producer), `fail`, or `proceed`. | WS10 | 3 |
| FR-6.4 | Strict profile requires `majority` or `unanimous` consensus for any loop ending in a protected action; `first-pass` is forbidden for such loops. | WS3/WS11 | 4 |

### FR-7 Governance & safety
| ID | Requirement | WS | Ph |
|---|---|---|---|
| FR-7.1 | Protected actions (commit/push/PR/deploy/release/live-smoke) require a fresh stamp and a passing policy decision. | WS5/WS12 | 4 |
| FR-7.2 | Governance provider is `noop` (default) or `agt`; strict profile requires `agt`. | WS12 | 4 |
| FR-7.3 | mivia-agent itself makes no network calls; only invoked agent CLIs do, during `run`/`review`. | all | — |
| FR-7.4 | No raw secrets, prompts, outputs, or provider payloads are persisted anywhere. | WS9/WS10 | 2,3 |
| FR-7.5 | Path policy rejects traversal and secret paths; symlink escape is blocked. | WS1 | 1 |

### FR-8 Hooks
| ID | Requirement | WS | Ph |
|---|---|---|---|
| FR-8.1 | Codex (`user-prompt-submit`, `pre-tool-use`, `permission-request`, `stop`) and Claude (`pre-tool-use`, `stop`) hook entrypoints share one policy engine. | WS5 | 4 |
| FR-8.2 | Hooks deny protected actions on missing/stale stamp or policy denial. | WS5 | 4 |
| FR-8.3 | Hooks fail closed on malformed payloads that request protected actions. | WS5 | 4 |

### FR-9 Import
| ID | Requirement | WS | Ph |
|---|---|---|---|
| FR-9.1 | `import` detects existing agent configs and produces a read-only migration plan into `.ai/`. | WS7 | 5 |
| FR-9.2 | `import` detects existing loop/workflow definitions in common formats and proposes mappings into `.ai/workflows/`. | WS7 | 5 |

## 9. Non-functional requirements

- **NFR-1 Portability:** single static Go binary, Linux/macOS/Windows. No required service/container/cloud. All templates, loop definitions, and skill content are embedded in the binary at build time (`//go:embed`). The binary ships alone — no companion data directory, no `.ai/` bundle, no config file on disk. A user installs via Homebrew, downloads a release binary, or `go install`, then runs `init` in their repo to generate `.ai/` from the embedded templates. `update` refreshes from the newer binary's embedded templates, not from the internet. The binary does not create or require `.ai/` for its own runtime. See plan "Distribution Model" section.
- **NFR-2 Dependency-light:** `cobra`, `yaml.v3`, `go-cmd`, `oklog/run`; AGT optional and lazy. No durable-workflow runtime.
- **NFR-3 Performance:** `init`/`doctor`/`audit`/`preflight` complete in well under a second on a typical repo. `run` latency is bounded by adapter turn/time budgets.
- **NFR-4 Security:** secrets never persisted; non-interactive approval enforced on every adapter `Run`; protected actions gated.
- **NFR-5 Determinism:** file generation is idempotent; routing and decision traces are deterministic from manifest + Git state. **Model output is non-deterministic and is treated as evidence, not as state** — i.e. re-running the same step may yield different artifacts, but the routing graph, gate outcomes, and recorded decision refs are reproducible from the inputs.
- **NFR-6 Observability:** every run emits a JSONL trace and a structured summary; every policy decision is recorded to the audit log.
- **NFR-7 Extensibility:** adding a CLI = one `Adapter`; adding a loop = one YAML file; swapping the governance provider = one interface impl.

## 10. User stories (representative)

- **US-1:** As an OSS maintainer, I run `mivia-agent init --profile standard --write` so that Codex + Claude + Copilot are all configured consistently from one place.
- **US-2:** As a reviewer, I run `mivia-agent review --artifact src/auth.go --reviewers codex,claude --mode majority` so that two CLIs independently vet the change before I merge.
- **US-3:** As a workflow author, I define a `bug-audit` loop (Codex audits → Claude + Codex consensus-review → iterate to fix) and run it in CI with a 5-iteration bound.
- **US-4:** As a platform engineer, I add a new orchestrable adapter (one file) — e.g. Crush once it is verified headless — and immediately route research steps to it without touching policy. *(If the target CLI is not headless-capable, the adapter is accepted but excluded from `run` per FR-3.4.)*
- **US-5:** As CI, I deny a `git push` because the stamp is stale and the latest policy decision is `denied`.

## 11. Command surface

| Command | Purpose | MVP |
|---|---|---|
| `init` | Install agent workflow files. | ✅ |
| `doctor` | Validate installed setup. | ✅ |
| `audit` | Report workflow quality gaps (read-only). | ✅ |
| `preflight` | Validate diff and write quality stamp. | ✅ |
| `adapters` | List adapters: presence, headless, approval, approved-for-run. | ✅ |
| `run` | Execute a named loop by orchestrating adapters headlessly. | ✅ (faked adapters) |
| `review` | One-off cross-CLI consensus review. | ✅ (faked adapters) |
| `hook codex <event>` | Codex hook entrypoint. | ✅ |
| `hook claude <event>` | Claude Code hook entrypoint. | ✅ |
| `import` | Inspect existing setup, propose migration. | Post-MVP |
| `update` | Update managed template blocks. | Post-MVP |

`run`/`review` are in MVP from a contract standpoint (tested behind faked adapters); real-CLI execution depends on the adapter being headless-capable at runtime.

## 12. Loop data flow (canonical shape)

```
┌─────────┐   artifact    ┌────────────────────┐  verdicts   ┌────────────┐
│ produce │ ────────────▶ │ review (fan-out)   │ ──────────▶ │ consensus  │
│ (1 CLI) │               │ (N CLIs in parallel│             │ policy     │
└─────────┘               │  via oklog/run)    │             └─────┬──────┘
     ▲                     └────────────────────┘                   │
     │                                                              │
     │  iterate: reviewer notes fed back as input                   ▼
     └──────────────────────────  on_fail: iterate   ◀── pass?  ─── fail/warn/proceed (on_exhausted)

exit_when.gate = review-pass  ──▶ loop ends successfully
bound (iterations) hit        ──▶ on_exhausted: fail | warn | proceed
```

- Artifacts live under `.ai/runs/<run-id>/<step>/`.
- Each step's invocation produces a `policy.Decision` ref stored in the stamp (when governance is on).
- The trace (`trace.jsonl`) records every step, verdict, decision, and iteration.

## 13. Success metrics (MVP exit criteria)

Tracked qualitatively for v0.1; instrumented lightly via the JSONL trace.

1. `init` → `doctor` passes on a fresh empty repo for every supported adapter mix (mix = any subset of {codex, claude, copilot, gemini, crush}).
2. `init --write` is provably idempotent (`TestInitWriteIsIdempotent`).
3. `preflight` writes a valid stamp that goes stale on any change to HEAD, diff hash, or changed-files set (`TestCheckStampRejectsStaleDiffHash`, `TestCheckStampRejectsChangedFilesMismatch`).
4. Hooks deny protected actions without a fresh stamp (`TestProtectedActionRequiresFreshStamp`).
5. `adapters` correctly reports headless capability (`TestAdaptersReportsHeadlessCapability`).
6. `run --workflow research` completes a full pass/fail/iterate cycle with faked adapters and writes a trace whose shape matches the contract in §12 (`TestRunExecutesResearchLoopFixture`). "Complete" = the loop terminates via `exit_when.gate` pass or `on_exhausted`, with a correct exit code.
7. `review` returns a correct consensus verdict with faked adapters (`TestReviewOneOffConsensus`).
8. Mutation proofs exist for every guard: path policy, stale stamp, protected command, overwrite, non-interactive approval enforcement, loop bound, stamp-before-protect, consensus threshold.
9. No raw prompt/output/secret leakage in any run artifact (`TestRunArtifactContainsNoRawPromptsOrOutputs` + audit grep over `.ai/runs/`).
10. Every loop definition in the shipped templates satisfies `doctor` for its target profile.

## 14. Phase exit gates

Each phase ships only when its exit gate is green (tests + mutation proofs).

| Phase | Scope (from plan) | Exit gate |
|---|---|---|
| 0 | Repo bootstrap | `go test ./...`; `--help` works. |
| 1 | Init + Doctor (manifest incl. routing/loops/governance fields, templates, path policy, git state) | FR-1.1–1.3, FR-2.1/2.3/2.4(stamp schema), FR-5.4, FR-6.1/6.2, FR-7.5; idempotency + path-policy mutation proofs. |
| 2 | Adapter system (Codex, Claude headless) + Preflight + `adapters` | FR-2.4, FR-3.1–3.5, FR-7.4; approval-enforcement + scrubbing tests. |
| 3 | Orchestrator + Consensus + `run`/`review` (Gemini; Crush gated on headless) | FR-4.1–4.5, FR-5.1–5.3, FR-6.3; loop-bound + stamp-before-protect + consensus-threshold mutation proofs. |
| 4 | Governance (noop + AGT interface), Hooks, Strict profile, Copilot templates | FR-2.2, FR-6.4, FR-7.1/7.2, FR-8.1–8.3; strict-requires-agt doctor failure test. |
| 5 | Import, Update, Distribution | FR-1.4, FR-9.1/9.2; release binaries build for linux/macOS/windows. |
| 6 (post-MVP) | Budget loops + `expert` profile + production AGT wiring | Lifts the FR-4.2 rejection; full OWASP-Agentic-Top-10 coverage under AGT. |

## 15. Risks & mitigations

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Headless invocation flags drift across CLI versions | High | High | Each adapter cites its targeted doc URL + version; `adapters` probes at runtime; doctor fails on version mismatch. |
| Adapter version drift between probe-time and run-time | Medium | High | `adapters` is re-run at the start of each `run`; the run aborts if a previously-approved adapter is no longer headless-capable. |
| Crush lacks a true headless mode | Medium | Medium | Mark Crush interactive-only; exclude from `run` until verified; ship the adapter behind the interface so it activates if headless lands. |
| AGT Go SDK API churn | Medium | Medium | Wrap behind `policy.Provider`; `noop` keeps the binary standalone; lazy/build-tagged import. |
| Consensus gives false confidence (correlated reviewer errors) | Medium | High | Default to `majority` + `min_reviewers: 2` + `strict` tie-breaker; strict profile forbids `first-pass` for protect-bound loops (FR-6.4). |
| `first-pass` consensus yields a false pass under strict | Medium | High | `audit`/`doctor` reject `first-pass` on any loop ending in a protected action when profile ≥ strict. |
| Loops run unbounded in CI | Medium | High | MVP rejects `budget` and `expert` (FR-4.2); enforces `max_iterations`; `on_exhausted: fail` is the default for protect-bound loops. |
| Adapter leakage of secrets via stdout | Medium | High | Scrub captured stdout/stderr; persist only declared artifacts + scrubbed metadata; `TestRunArtifactContainsNoRawPromptsOrOutputs`. |
| Policy duplication across adapters | Low | Medium | `.ai/` canonical; adapters are thin pointers; `audit` flags duplication. |
| External runtime dependency creep | Low | High | No Dagger/Temporal/K8s; in-house DAG + `oklog/run` + `go-cmd`. |

## 16. Open questions

1. **Crush headless?** Confirm whether Crush supports a non-interactive mode. If not at MVP, ship the adapter as `interactive-only` and revisit per release. Blocks Phase 3 Crush enablement.
2. **AGT Go SDK stability** — pin a version when Phase 4 starts; decide lazy-import vs build tag. Blocks FR-7.2 production use.
3. **Consensus over non-text artifacts (diffs)** — define the `Verdict.evidence_ref` shape for diff artifacts before Phase 3 lands.
4. **Manual tie-breaker UX** — how `tie_breaker: manual` surfaces to a human in CI (likely: fail the run, emit a structured request, resume on next invocation).
5. **Loop template marketplace** — out of MVP, but fix the YAML schema now so community loops are forward-compatible.

## 17. Out of scope (explicitly rejected for MVP)

- `pi agent` integration — removed.
- `bound: budget` loops and the `expert` profile — rejected until Phase 6.
- Dagger / Temporal / Kubernetes / any external workflow runtime — not used.
- Live connectors (Jira, Slack, GitHub data) — not enabled by default.
- Hosted service / cloud account / database — none required.
- Raw prompt, raw model output, or provider payload persistence — forbidden.
- Auto push / PR / deploy / remote mutation — forbidden.

## 18. Glossary

- **Adapter** — swappable wrapper around one agent CLI (`Detect`/`Run`/`Review`).
- **Loop** — named, bounded workflow of steps with an exit gate and exhaustion policy.
- **Routing** — artifact flow between steps (sequential, parallel, conditional).
- **Consensus review** — parallel multi-reviewer verification with a voting policy.
- **Stamp** — local quality artifact under `.git/` proving what was verified for a given diff; stale on HEAD/diff-hash/changed-files change.
- **Governance provider** — policy engine (`noop` or `agt`) deciding protected actions.
- **Protected action** — commit, push, PR, deploy, release, or live-smoke.
- **AGT** — Microsoft Agent Governance Toolkit (MIT-licensed, Go SDK); deterministic policy + audit.
- **`.ai/`** — canonical agent-control surface; all other config files are thin adapters to it.
- **WS** — implementation workstream (see plan).
- **Ph** — release phase (see §14).
