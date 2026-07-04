# WS8 — CI, Release, Docs

- **Phase:** 5
- **Depends on:** all prior WS
- **PRD:** §9 (NFR-1), §11, §14 (Phase 5 gate)
- **Plan:** WS8
- **Exit gate (Phase 5):** release binaries build for linux/macOS/windows; docs cover install, init, doctor, preflight, hooks, run, review, loops; CI runs the full suite + a generated-fixture doctor smoke.

Goal: distribution, the fixture that proves the whole system composes, and the docs a new user needs.

## T1 — Version injection + release workflow

Create / extend:
- `internal/version/version.go` (exists from WS0) — ensure `Version`, `Commit`, `Date` vars exist, all defaulting to `dev`/`unknown`.
- `.github/workflows/release.yml` — on tag `v*`: build for `linux/amd64`, `darwin/amd64`, `darwin/arm64`, `windows/amd64`; inject `-ldflags "-X ...version.Version=... -X ...Commit=... -X ...Date=..."`; produce `checksums.txt`; attach to the release.

Spec:
- Version is always set at link time in release builds; `dev` only locally.
- Each binary is named `mivia-agent-<os>-<arch>[.exe]`.

Tests that must pass:
- `TestReleaseVersionInjected` (unit: build a test binary with ldflags, run `version`, assert non-`dev`)

Mutation proof:
- Drop the ldflags injection in the workflow; assert the test would catch it (locally simulate by linking without ldflags and checking the test fails).

## T2 — Generated-fixture smoke (the composition test)

Create:
- `test/fixture/fixture_test.go` — an integration test that, in a temp repo:
  1. `mivia-agent init --profile standard --adapter codex --adapter claude --adapter copilot --write`
  2. `mivia-agent doctor` → expect exit 0
  3. `mivia-agent audit` → expect no error-severity findings
  4. `mivia-agent preflight` (with stubbed verifiers) → expect stamp written
  5. `mivia-agent adapters` → expect codex+claude listed (presence reflects the test env; headless not asserted here)
  6. `mivia-agent run --workflow research --dry-run` → expect a non-empty plan

Spec:
- Uses the real built binary (subprocess) — this is the end-to-end composition check.
- No real CLIs invoked (only `--dry-run` and read-only commands).
- Asserts the full `find` output of the fixture repo matches the expected file set from the plan.

Tests that must pass:
- `TestGeneratedFixtureDoctorPasses`
- `TestGeneratedFixtureFileSetMatchesStandardProfile`
- `TestGeneratedFixtureDryRunPlanNonEmpty`

Mutation proof:
- Remove one expected file from the standard set; `TestGeneratedFixtureFileSetMatchesStandardProfile` must fail.

## T3 — CI workflow update

Extend `.github/workflows/ci.yml` (WS0):
- Add the fixture smoke as a separate job.
- Add a `mivia-agent adapters --json` smoke (will report whatever's present on the runner).
- Run on Linux + macOS + Windows.

## T4 — User docs

Create:
- `docs/user-guide.md` — install, `init`, `doctor`, `preflight`, `adapters`, `run`, `review`, hooks, exit codes. With copy-pasteable examples.
- `docs/adapter-authoring.md` — the `Adapter` interface, `Detect`/`Run`/`Review`, the `FakeRunner` pattern, how to add a CLI.
- `docs/loop-authoring.md` — loop YAML schema, `bound`/`exit_when`/`on_exhausted`, consensus modes, `iterate` semantics, a worked research-loop example.
- `docs/template-authoring.md` — managed blocks, template variables, the `List(profile, adapters)` contract.
- `docs/adr/0001-product-boundary.md` — records: no network, no service, no raw persistence, AGT optional, adapter-based.

Spec:
- Every documented command cross-links to the relevant PRD FR.
- `loop-authoring.md` includes the ASCII loop diagram from PRD §12.

## T5 — Optional distribution (post-MVP-flavor, but cheap to land)

Create (only if the org wants it now; otherwise defer):
- Homebrew tap formula (separate repo) — out of scope for this WS's test gate; track as follow-up.
- Codex plugin packaging — explicitly deferred per plan ("only after CLI proves stable").

## Verification

```bash
go test ./... -count=1
# Cross-compile smoke:
GOOS=linux   GOARCH=amd64 go build -o /tmp/ma-linux   ./cmd/mivia-agent
GOOS=darwin  GOARCH=arm64 go build -o /tmp/ma-darwin  ./cmd/mivia-agent
GOOS=windows GOARCH=amd64 go build -o /tmp/ma-windows ./cmd/mivia-agent
ls -la /tmp/ma-*
# Fixture smoke:
go test ./test/fixture/... -count=1 -v
```

WS8 is ☑ when:
- [x] release workflow builds 4 binaries + checksums
- [x] fixture composition test green
- [x] all four docs land and cross-link PRD FRs
- [x] CI matrix green on linux+macos+windows
- [x] status updated in `00-overview.md`

## Completion — 2026-07-05

- Tests: 5 passing (`TestReleaseVersionInjected`, `TestGeneratedFixtureDoctorPasses`, `TestGeneratedFixtureFileSetMatchesStandardProfile`, `TestGeneratedFixtureDryRunPlanNonEmpty`, plus `go test ./... -count=1`).
- Mutation proofs: remove one expected fixture file from the planned standard set fails `TestGeneratedFixtureFileSetMatchesStandardProfile`; remove ldflags from the release-version test build fails `TestReleaseVersionInjected`; both reverted.
- Files: 7 created.
- Residual risk: none.
- Follow-ups: T5 distribution extras remain explicitly deferred by plan.
