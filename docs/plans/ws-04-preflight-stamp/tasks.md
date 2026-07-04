# WS4 — Preflight + Quality Stamp

- **Phase:** 2
- **Depends on:** WS1
- **PRD:** FR-2.4, FR-7.1 (partial)
- **Plan:** WS4, "preflight stamp" section
- **Exit gate (Phase 2, partial):** `preflight` writes a valid stamp; stale on HEAD/diff-hash/changed-files change.

Goal: classify the current diff, require the right proofs for high-risk changes, write the stamp, and reject stale stamps. Hooks (WS5) and governance (WS12) consume this stamp.

## T1 — Stamp types + (de)serialization

Create:
- `internal/preflight/stamp.go` — `type Stamp struct{ Head, DiffSHA256 string; ChangedFiles []string; ContractRows, FocusedVerifiers, BroadVerifiers, MutationProofs, NotRun []string; PolicyDecisionRefs []string; CreatedAt string }`, `(s Stamp) Marshal() ([]byte, error)`, `ParseStamp(b []byte) (Stamp, error)`.
- `internal/preflight/stamp_test.go`

Spec:
- `Marshal` produces stable, sorted-key JSON, terminated by a newline.
- `CreatedAt` is RFC3339 UTC.
- `PolicyDecisionRefs` is empty in WS4 (populated by WS12); the field exists now to avoid a format bump later.
- `ParseStamp` rejects malformed JSON and missing required fields (`Head`, `DiffSHA256`).

Tests that must pass:
- `TestStampMarshalRoundTrip`
- `TestStampMarshalStableOrder`
- `TestParseStampRejectsMissingHead`
- `TestParseStampRejectsMalformedJSON`

Mutation proof:
- Randomize key order in `Marshal`; `TestStampMarshalStableOrder` (byte-equal across two runs with same input) must fail.

## T2 — Risk classification

Create:
- `internal/preflight/risk.go` — `type Risk int` (`Low`, `Medium`, `High`), `Classify(files []string, contractMatrix ContractMatrix) Risk`.
- `internal/preflight/risk_test.go`

Spec:
- A change is `High` if any changed file touches a high-risk surface: `.github/workflows/`, scripts that run in CI, hook configs, deploy/runner/auth/security contract rows, or anything matched by `quality.require_contract_rows_for`.
- `Medium` if it touches code paths with a verifier but no security surface.
- `Low` otherwise (docs, comments-only diffs).

Tests that must pass:
- `TestClassifyHighForCIChange`
- `TestClassifyHighForHookConfigChange`
- `TestClassifyLowForDocsOnly`
- `TestClassifyMediumForCodeWithVerifier`

Mutation proof:
- Treat all changes as Low; `TestClassifyHighForCIChange` must fail.

## T3 — Preflight command + stamp write

Create:
- `internal/preflight/preflight.go` — `Run(ctx) (Stamp, error)`, enforces risk→required-proofs rules, writes the stamp.
- `internal/cli/preflight.go` — `preflightCmd`, flags `--repo`, `--contract-row` (repeated), `--focused-verifier` (repeated), `--broad-verifier` (repeated), `--mutation-proof` (repeated), `--not-run` (repeated), `--pipeline-preflight`, `--json`.

Spec:
- Compute `Head` (WS1), `ChangedFiles` (WS1), `DiffSHA256` (WS1).
- For `High` risk: require ≥1 contract row, ≥1 focused verifier, and ≥1 mutation proof. Missing → error listing what's missing.
- `--not-run <reason>` records an explicit not-run reason (allowed only if a broad verifier is otherwise missing and the reason is non-empty).
- `--pipeline-preflight` relaxes broad-verifier requirement (CI context where broad verifiers run separately).
- Write to `<repo>/.git/mivia-agent-quality-stamp.json` using WS1 pathpolicy for `.git/` allow.
- Never write outside `.git/`.

Tests that must pass (real temp repos):
- `TestPreflightWritesStampForLowRiskChange`
- `TestPreflightRequiresContractRowsForHighRiskChange`
- `TestPreflightRequiresMutationProofForHighRiskChange`
- `TestPreflightRequiresFocusedVerifierForHighRiskChange`
- `TestPreflightAcceptsNotRunReasonForMissingBroad`
- `TestPreflightRejectsNotRunWithoutReason`
- `TestPreflightStampWrittenUnderDotGit`

Mutation proof:
- Drop the high-risk contract-row requirement; `TestPreflightRequiresContractRowsForHighRiskChange` must fail.
- Drop the mutation-proof requirement; `TestPreflightRequiresMutationProofForHighRiskChange` must fail.

## T4 — Stamp check (staleness)

Create:
- `internal/preflight/check.go` — `CheckStamp(repo string) (Stamp, error)`.
- `internal/preflight/check_test.go`

Spec:
- Read the stamp; if missing → `ErrNoStamp`.
- Recompute current `Head`, `DiffSHA256`, `ChangedFiles`; compare to stamp.
- Stale if `Head != stamp.Head` OR `DiffSHA256 != stamp.DiffSHA256` OR set-difference on `ChangedFiles` is non-empty → `ErrStaleStamp{Reason}`.
- Otherwise return the fresh stamp.

Tests that must pass:
- `TestCheckStampRejectsMissingStamp`
- `TestCheckStampRejectsStaleHead`
- `TestCheckStampRejectsStaleDiffHash`
- `TestCheckStampRejectsChangedFilesMismatch`
- `TestCheckStampAcceptsFreshStamp`

Mutation proof:
- Disable the diff-hash comparison; `TestCheckStampRejectsStaleDiffHash` must fail.
- Disable the changed-files comparison; `TestCheckStampRejectsChangedFilesMismatch` must fail.

## Verification

```bash
go test ./internal/preflight/... -count=1
go vet ./internal/preflight/...
tmp=$(mktemp -d) && (cd "$tmp" && git init -q && git commit -q --allow-empty -m init && echo hi > a.go && git add a.go)
go run ./cmd/mivia-agent preflight --repo "$tmp" --focused-verifier "go test ./..." --mutation-proof "x" --contract-row hooks --contract-row ci --json
cat "$tmp/.git/mivia-agent-quality-stamp.json"
```

WS4 is ☑ when:
- [ ] all listed tests pass
- [ ] stamp written exactly under `.git/`
- [ ] staleness mutation proofs executed (2)
- [ ] risk→proof mutation proofs executed (≥2)
- [ ] status updated in `00-overview.md`
