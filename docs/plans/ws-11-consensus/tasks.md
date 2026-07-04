# WS11 — Consensus

- **Phase:** 3
- **Depends on:** WS9 (for the `Verdict` type)
- **PRD:** FR-5.2, FR-6.4
- **Plan:** WS11, "Consensus" / "Review (consensus)"
- **Exit gate (Phase 3, partial):** all four modes + tie-breakers correct; `min_reviewers` unsatisfiable → fail.

Goal: a pure, deterministic policy engine over a set of `Verdict`s. No I/O. This is the easiest WS to mutation-proof exhaustively.

## T1 — Policy types

Create:
- `internal/consensus/consensus.go` — `type Mode string` (`Majority`, `Unanimous`, `Weighted`, `FirstPass`), `type TieBreaker string` (`Strict`, `Manual`, `PreferPrefix`), `type Policy struct{ Mode; MinReviewers int; Weights map[string]float64; TieBreaker; Threshold float64 }`, `type Outcome struct{ Pass bool; Reason string; For, Against []string; Tied bool }`.
- `internal/consensus/consensus_test.go`

Spec:
- `Weights` default to `1.0` per adapter if unset.
- `Threshold` default: for `Weighted`, `0.5 * sum(weights)`; for `Majority`, implicit `>50%`.

## T2 — Evaluate

Create:
- `internal/consensus/evaluate.go` — `func Evaluate(p Policy, verdicts []adapter.Verdict) (Outcome, error)`.
- `internal/consensus/evaluate_test.go`

Spec:
- Error if `len(verdicts) < p.MinReviewers` → `ErrMinReviewersUnsatisfied`.
- `Majority`: count of `Pass==true` adapters (one per unique adapter name; duplicates dedup by name, last-write-wins with a warning in `Reason`). Pass if `passers > total/2`.
- `Unanimous`: pass iff all pass.
- `Weighted`: `sum(weights of passers) >= Threshold`.
- `FirstPass`: pass iff ≥1 passer.
- Tie-breaker applies when the mode yields a tie (e.g. Majority with 2 pass / 2 fail):
  - `Strict`: `Pass=false`, `Reason="tie: strict fails"`.
  - `Manual`: `Pass=false`, `Reason="tie: requires manual resolution"` (WS10 surfaces this to the caller).
  - `PreferPrefix` (e.g. `prefer:codex`): `Pass` = verdict of the preferred adapter if it voted, else fall back to strict.
- `Outcome.For`/`Against` list adapter names.

Tests that must pass:
- `TestMajorityPassesAboveThreshold`
- `TestMajorityFailsAtOrBelowHalf`
- `TestMajorityDedupsByAdapterName`
- `TestUnanimousFailsOnOneReject`
- `TestUnanimousPassesAllPass`
- `TestWeightedRespectsWeights`
- `TestWeightedThresholdDefaultsToHalf`
- `TestFirstPassPassesOnOne`
- `TestFirstPassFailsOnNone`
- `TestMinReviewersUnsatisfiedErrors`
- `TestTieBreakerStrictFailsOnTie`
- `TestTieBreakerManualFailsOnTie`
- `TestTieBreakerPrefersNamedAdapter`
- `TestTieBreakerPreferFallsBackToStrictWhenAbsent`

Mutation proof:
- Flip the majority comparison to `>=`; `TestMajorityFailsAtOrBelowHalf` (a 2-of-4 case) must fail.
- Make `Unanimous` ignore a single reject; `TestUnanimousFailsOnOneReject` must fail.

## T3 — Policy validation against profile

Create:
- `internal/consensus/profile.go` — `func ValidateForProfile(p Policy, profile string, protectBound bool) error`.
- `internal/consensus/profile_test.go`

Spec:
- For `profile == "strict"` and `protectBound == true`: `Mode` must be `Majority` or `Unanimous` (not `FirstPass`); `MinReviewers >= 2`; `TieBreaker != PreferPrefix` unless the preferred adapter is named (i.e. bare `prefer:` is invalid).
- Otherwise no constraint beyond `MinReviewers >= 1`.

Tests that must pass:
- `TestStrictProtectBoundRejectsFirstPass`
- `TestStrictProtectBoundRejectsMinReviewersOne`
- `TestStrictProtectBoundAcceptsMajorityMinReviewersTwo`
- `TestNonStrictAcceptsFirstPass`

Mutation proof:
- Allow `FirstPass` under strict protect-bound; `TestStrictProtectBoundRejectsFirstPass` must fail.

## Verification

```bash
go test ./internal/consensus/... -count=1
go vet ./internal/consensus/...
```

WS11 is ☑ when:
- [ ] all listed tests pass (≈18)
- [ ] mutation proofs executed and reverted (≥3)
- [ ] `go vet` clean
- [ ] status updated in `00-overview.md`
