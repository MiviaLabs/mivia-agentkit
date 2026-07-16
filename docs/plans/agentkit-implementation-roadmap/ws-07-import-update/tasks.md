# WS7 — `import` + `update`

- **Phase:** 5
- **Depends on:** WS2 (templates), WS3 (doctor)
- **PRD:** FR-1.4, FR-9.1, FR-9.2
- **Plan:** WS7
- **Exit gate (Phase 5):** `import` produces a read-only migration plan and writes only with `--write`; `update` updates managed blocks only and reports conflicts on user-edited managed blocks.

Goal: bring an existing agent setup under mivia-agent, and upgrade managed templates without clobbering user content.

## T1 — Importer: read sources

Create:
- `internal/importer/importer.go` — `type Finding struct{ Source, Kind, Path string; Reusable bool }`, `func Inspect(repo string) ([]Finding, error)`.
- `internal/importer/importer_test.go`

Spec — read (never write) from:
- `AGENTS.md`, `CLAUDE.md`, `GEMINI.md`
- `.codex/`, `.claude/`, `.agents/skills/`, `.github/copilot-instructions.md`, `.github/instructions/`, `.github/agents/`
- existing loop/workflow definitions in common shapes: GitHub Actions workflows named `*-agent*.yml`, `.claude/agents/*`, a `.dagger/` module (detect only), any `*.loop.yaml`/`workflow.yaml`.
- For each, classify `Kind` (`rules`, `skill`, `hook`, `instruction`, `loop`) and whether it's `Reusable` (maps cleanly into `.ai/`).

Tests that must pass (fixtures under `testdata/import/`):
- `TestImportReadsExistingAgentFiles`
- `TestImportClassifiesKinds`
- `TestImportDetectsExistingWorkflowDefinitions`
- `TestImportFlagsConflicts`

## T2 — Import plan + write

Create:
- `internal/importer/plan.go` — `type Plan struct{ Actions []Action; Conflicts []Conflict }`, `func BuildPlan(repo string, manifest config.Manifest) (Plan, error)`, `(p Plan) Apply(repo string, force bool) (Report, error)`.
- `internal/importer/plan_test.go`
- `internal/cli/import.go` — `importCmd`, flags `--repo`, `--write`, `--force`, `--json`.

Spec:
- `Plan`: map reusable findings into `.ai/` writes; list conflicts (e.g. an `AGENTS.md` with policy that contradicts `.ai/rules/`).
- Default behavior: read-only, print the plan. Write only with `--write`. Never delete existing files.
- `Apply` writes only the mapped `.ai/` files using WS2 render (managed blocks) + pathpolicy; user files are untouched.
- After `--write`, run doctor.

Tests that must pass:
- `TestImportPlanDoesNotWriteByDefault`
- `TestImportWriteCreatesAIMappedFiles`
- `TestImportWritePreservesExistingUserFiles`
- `TestImportReportsConflicts`

Mutation proof:
- Make `--write` the default; `TestImportPlanDoesNotWriteByDefault` must fail.

## T3 — Updater

Create:
- `internal/update/update.go` — `type Change struct{ Path, Kind string }`, `func Diff(repo string) ([]Change, error)`, `func Apply(repo string) (Report, error)`.
- `internal/update/update_test.go`
- `internal/cli/update.go` — `updateCmd`, flags `--repo`, `--write`, `--json`.

Spec:
- Read installed `template_version` from manifest; compare to embedded templates.
- `Diff`: list managed blocks that differ.
- `Apply`: update managed blocks only (WS2 `ReplaceManaged`); preserve user content.
- If a managed block was user-edited (content between markers changed from the known managed content and not by a template bump), report a conflict and skip that block unless `--force`.
- After `Apply`, run doctor.

Tests that must pass:
- `TestUpdateChangesManagedBlockOnly`
- `TestUpdatePreservesUserTextOutsideManagedBlocks`
- `TestUpdateReportsConflictForUserEditedManagedBlock`
- `TestUpdateForceOverwritesConflictedBlock`
- `TestUpdateNoOpWhenAlreadyCurrent`

Mutation proof:
- Let `Apply` overwrite a non-managed region; `TestUpdatePreservesUserTextOutsideManagedBlocks` must fail.
- Let `Apply` overwrite a conflicted block without `--force`; `TestUpdateReportsConflictForUserEditedManagedBlock` must fail.

## Verification

```bash
go test ./internal/importer/... ./internal/update/... ./internal/cli/... -count=1
go vet ./internal/importer/... ./internal/update/...
tmp=$(mktemp -d) && (cd "$tmp" && git init -q && git commit -q --allow-empty -m init)
# Seed a fake existing setup, then import:
mkdir -p "$tmp/.claude" && echo 'old policy' > "$tmp/CLAUDE.md"
go run ./cmd/mivia-agent import --repo "$tmp" --json
go run ./cmd/mivia-agent import --repo "$tmp" --write
go run ./cmd/mivia-agent doctor --repo "$tmp"
```

WS7 is ☑ when:
- [x] all listed tests pass
- [x] import read-only-by-default + overwrite-preserving mutation proofs
- [x] update managed-block-only + conflict-reporting mutation proofs
- [x] status updated in `00-overview.md`

## Completion — 2026-07-05

- Tests: 13 passing (`internal/importer`, `internal/update`, and scoped `internal/cli` verifiers).
- Mutation proofs: `--write` default enabled fail-then-revert ok; conflicted managed block overwrite without `--force` fail-then-revert ok; non-managed-region overwrite fail-then-revert ok.
- Files: 20 created.
- Residual risk: none.
- Follow-ups: none.
