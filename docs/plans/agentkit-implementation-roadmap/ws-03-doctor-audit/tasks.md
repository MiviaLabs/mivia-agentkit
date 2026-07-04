# WS3 — `doctor` + `audit`

- **Phase:** 1
- **Depends on:** WS2
- **PRD:** FR-2.1, FR-2.3, FR-5.4, FR-6.1, FR-6.2, FR-10.5; §11 (command surface)
- **Plan:** WS3, "doctor" and "audit" check lists (the augmented version with loop/consensus/governance/global checks)
- **Exit gate (Phase 1, partial):** `doctor` passes on a fresh `init`; fails on each broken-wiring class; `audit` reports the listed finding categories.

Goal: a read-only validator that knows the full expected file set + wiring invariants, and a quality-gap reporter.

## T1 — Report types + renderer

Create:
- `internal/report/report.go` — `type Finding struct{ Severity, Code, Message, Path string }`, `type Report struct{ Findings []Finding; ExitCode int }`, `(r Report) Text() string`, `(r Report) JSON() ([]byte, error)`.
- `internal/report/report_test.go`

Spec:
- Severities: `error`, `warn`, `info`. Exit codes: `0` (no error-severity findings), `1` (errors), `2` (warn-only under `--strict`).
- `Text()` is concise, sorted by severity then path. `JSON()` is stable-ordered.
- Codes are stable strings (e.g. `manifest.missing`, `loop.unknown-adapter`) — they are part of the contract for CI parsing.

Tests that must pass:
- `TestReportTextSortedBySeverity`
- `TestReportExitCodeFromFindings`
- `TestReportJSONStableOrder`

Mutation proof:
- Sort by path instead of severity; `TestReportTextSortedBySeverity` must fail.

## T2 — Doctor checks

Create:
- `internal/doctor/doctor.go` — `type Check struct{ ID, Run func(ctx) Finding }`, `Run(ctx) Report`, default check registry.
- `internal/doctor/checks_core.go` — file-existence + wiring checks.
- `internal/doctor/checks_loops.go` — loop/consensus checks (depend on WS1 `config.Loop.Validate`).
- `internal/doctor/doctor_test.go`

Spec — each check is a `Finding` (or nil when ok):
- `manifest.exists_parses` — `mivia-agent.yaml` exists and parses via WS1.
- `ai.index_exists` — `.ai/INDEX.md` exists.
- `adapters.point_to_index` — `AGENTS.md`/`CLAUDE.md` reference `.ai/INDEX.md`.
- `adapter_files_present` — each enabled adapter's files exist.
- `hooks.call_mivia_agent` — `.codex/hooks.json` and `.claude/settings.json` (when present) invoke `mivia-agent hook ...`.
- `skills.valid_frontmatter` — every `SKILL.md` under `.ai/skills/` and `.claude/skills/` has valid YAML frontmatter (`name`, `description`).
- `generated_markers.valid` — managed-block markers balanced (uses WS2 `HasManaged`).
- `ci.calls_doctor_json` — `.github/workflows/agent-control.yml` contains `mivia-agent doctor --json`.
- `no_generated_artifacts_staged` — nothing under `.ai/runs/` is staged.
- `no_secret_paths_generated` — none of the generated files match forbidden patterns (WS1 pathpolicy).
- `loops.bound` — every loop has `bound: iterations` (reject `budget`).
- `loops.known_adapters` — every step references an enabled adapter of valid role (uses WS1).
- `consensus.satisfiable` — every review step's `min_reviewers` ≤ count of enabled headless-capable adapters (note: headless-capability itself is WS9; here we approximate "enabled orchestrable adapters" and tighten in WS9).
- `governance.provider_known` — `governance.provider` ∈ `{noop, agt}`.
- `global.readable` — if `~/.agents/` exists, it is readable and parses (warn on parse errors).
- `global.no_rule_conflict` — if a rule file exists in both `~/.agents/rules/` and `.ai/rules/` with the same name, content divergence is a warning (not an error — project wins, but the user should know).

Tests that must pass (over temp repos initialized by WS2):
- `TestDoctorPassesFreshInit`
- `TestDoctorFailsMissingAIIndex`
- `TestDoctorFailsMissingAdapterFile`
- `TestDoctorFailsHookNotCallingMiviaAgent`
- `TestDoctorFailsLoopWithNoBound`
- `TestDoctorFailsLoopReferencingUnknownAdapter`
- `TestDoctorFailsConsensusMinReviewersUnsatisfiable`
- `TestDoctorFailsUnknownGovernanceProvider`
- `TestDoctorWarnsGlobalRuleConflict` (same rule name in global and project with different content → warn severity)
- `TestDoctorPassesWithNoGlobalConfig` (no `~/.agents/` → no error, no warning)

Mutation proof:
- Remove the hook-target check; `TestDoctorFailsHookNotCallingMiviaAgent` must fail.
- Remove the loop-bound check; `TestDoctorFailsLoopWithNoBound` must fail.

## T3 — Audit findings

Create:
- `internal/audit/audit.go` — `Run(ctx) Report`.
- `internal/audit/audit_test.go`

Spec — audit is advisory (always exits 0 unless `--strict`); reports:
- `canonical.missing_ai`
- `policy.duplicated_in_adapters` (long policy text appearing verbatim in both `.ai/` and an adapter file)
- `hooks.missing_for_adapter`
- `ci.missing_control_check`
- `quality.missing_stamp_gate`
- `contracts.missing_matrix`
- `commands.empty_verifier_matrix`
- `mcp.unsafe_config`
- `generated.edited_outside_managed_blocks` (managed file modified in the non-managed region)
- `loop.no_review_before_protect`
- `consensus.weaker_than_profile_requires` (e.g. `first-pass` under strict, or majority under strict for a protect-bound loop)
- `consensus.min_reviewers_exceeds_enabled`
- `governance.noop_under_strict`
- `global.rule_conflict_with_project` (same rule file name in `~/.agents/rules/` and `.ai/rules/` with divergent content)

Tests that must pass:
- `TestAuditReportsDuplicatedAdapterPolicy`
- `TestAuditReportsMissingCIForStrictProfile`
- `TestAuditReportsNoReviewBeforeProtect`
- `TestAuditReportsWeakConsensusUnderStrict`
- `TestAuditReportsEditedManagedFileOutsideBlocks`
- `TestAuditReportsGlobalRuleConflictWithProject`

Mutation proof:
- Make `duplicated_in_adapters` a substring match (instead of verbatim block); `TestAuditReportsDuplicatedAdapterPolicy` must still catch the fixture but a near-duplicate should slip past — adjust the assertion to prove the strictness.

## T4 — CLI wiring

Create:
- `internal/cli/doctor.go` — `doctorCmd`, flags `--repo`, `--json`, `--strict`.
- `internal/cli/audit.go` — `auditCmd`, flags `--repo`, `--json`, `--strict`.

Spec:
- `doctor` exit code from `Report.ExitCode`; default behavior is non-`--strict`.
- `audit` default exit 0 (advisory); `--strict` promotes warns.

Tests that must pass:
- `TestDoctorCmdExitsOneOnFinding`
- `TestAuditCmdExitsZeroUnlessStrict`

## Verification

```bash
go test ./internal/doctor/... ./internal/audit/... ./internal/report/... ./internal/cli/... -count=1
go vet ./internal/doctor/... ./internal/audit/... ./internal/report/... ./internal/cli/...
tmp=$(mktemp -d) && (cd "$tmp" && git init -q && git commit -q --allow-empty -m init)
go run ./cmd/mivia-agent init   --repo "$tmp" --profile standard --adapter codex --adapter claude --adapter copilot --write
go run ./cmd/mivia-agent doctor --repo "$tmp" --json
go run ./cmd/mivia-agent audit  --repo "$tmp" --json
```

WS3 is ☑ when:
- [ ] all listed tests pass
- [ ] `doctor` passes on a fresh init for codex+claude+copilot
- [ ] mutation proofs executed and reverted (≥3)
- [ ] `go vet` clean
- [ ] status updated in `00-overview.md`

## Completion — 2026-07-04

- Tests: `go test ./internal/doctor/... ./internal/audit/... ./internal/report/... ./internal/cli/... -count=1` passing.
- Audit fixes: Codex adapter instructions are required; managed-block drift checks compare rendered pre/post content; audit uses doctor check IDs instead of registry indexes.
- Mutation proofs: hook target check fail-then-revert ok; loop-bound check fail-then-revert ok; report severity sort fail-then-revert ok; Codex adapter instructions fail-then-revert ok; managed-block drift fail-then-revert ok.
- Verification: `go vet ./...`, focused WS3 tests, fresh `init` + `doctor --json` + `audit --json`, `git diff --check`, and plan validation passed.
- Audit loop: deep-bug-audit completed with two consecutive clean reports.
- Files: 11 created.
- Residual risk: none.
- Follow-ups: none.
