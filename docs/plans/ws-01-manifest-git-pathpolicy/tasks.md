# WS1 — Manifest, Git State, Path Policy

- **Phase:** 1
- **Depends on:** WS0
- **PRD:** FR-1.1, FR-4.2, FR-7.5; §7 (concepts), §8 (manifest fields)
- **Plan:** WS1, "Manifest" section
- **Exit gate (Phase 1, partial):** manifest parse/validate, git state, diff hash, path policy all green; mutation proofs for traversal rejection and diff hashing.

Goal: parse `mivia-agent.yaml` (including the new routing/loops/governance fields), detect git state + stable diff hash, and enforce a path allow/deny policy. No file writing yet (that's WS2).

## T1 — Manifest types + defaults

Create:
- `internal/config/manifest.go` — `type Manifest struct{...}`, `Defaults() Manifest`, `Validate() error`, `Parse([]byte) (Manifest, error)`.
- `internal/config/manifest_test.go`

Spec:
- Struct fields mirror the manifest in plan "Manifest" section: `Version`, `Profile`, `TemplateVersion`, `Project`, `Adapters` (map of `name -> {Enabled, Role}`), `Routing` (default producer/reviewers, `Consensus{Mode,Weights,TieBreaker,MinReviewers}`, `OnReviewFail`, `MaxIterations`), `Loops` (map), `Commands`, `ProtectedActions`, `Quality`, `Paths`, `Governance{Provider,AuditLog,PolicyDecisions}`, `MCP`.
- `Defaults()` returns the `standard` profile defaults from the plan (Codex+Claude orchestrable, Copilot guidance, Gemini/Crush disabled, `routing.default_reviewers=[codex,claude]`, `mode=majority`, `min_reviewers=2`, `tie_breaker=strict`, `on_review_fail=iterate`, `max_iterations=3`, `governance.provider=noop`).
- `Validate()` returns errors for: unknown `Profile`, unknown adapter `Role` (must be `orchestrable|guidance`), `Loop` with no `bound`, `bound: budget` in MVP, `expert` profile in MVP, consensus `Mode` not in the allowed set.
- Unknown fields: fail closed (strict YAML decode; reject unknown keys) — surface a clear error.

Tests that must pass:
- `TestManifestDefaultsIncludeRoutingAndLoopDefaults`
- `TestManifestRejectsUnknownProfile`
- `TestManifestRejectsUnknownAdapterRole`
- `TestManifestRejectsBudgetBoundInMVP`
- `TestManifestRejectsExpertProfileInMVP`
- `TestManifestRejectsUnknownConsensusMode`
- `TestManifestRejectsUnknownYAMLField`

Mutation proof:
- Comment out the `expert` rejection; `TestManifestRejectsExpertProfileInMVP` must fail.
- Comment out the `budget` rejection; `TestManifestRejectsBudgetBoundInMVP` must fail.

## T2 — Loop definition validation

Create:
- `internal/config/loop.go` — `type Loop struct{...}`, `(l Loop) Validate(enabledAdapters map[string]AdapterRole) error`.
- `internal/config/loop_test.go`

Spec:
- A `Loop` has `Description`, `Bound`, `MaxIterations`, `Steps []Step`, `ExitWhen`, `OnExhausted`.
- A `Step` has `ID`, `Producer` (adapter name), `Reviewers []string`, `Artifact`, optional `Approval`, `MaxTurns`, `Timeout`, `Consensus` override, `OnFail`.
- `Validate` errors if: any step references an adapter not in `enabledAdapters`; a review step's reviewers include a non-orchestrable adapter; `MaxIterations <= 0`; `OnExhausted` not in `{fail,warn,proceed}`; two steps share an `ID`; a step has neither a producer nor reviewers.
- `OnExhausted` defaults to `fail` for any loop whose `exit_when` gate leads to a protected action; otherwise `warn`.

Tests that must pass:
- `TestLoopValidateRejectsUnknownAdapter`
- `TestLoopValidateRejectsGuidanceAdapterAsProducer`
- `TestLoopValidateRejectsNonPositiveMaxIterations`
- `TestLoopValidateRejectsDuplicateStepIDs`
- `TestLoopValidateDefaultsOnExhaustedToFailForProtectBoundLoop`

Mutation proof:
- Remove the "guidance adapter as producer" check; `TestLoopValidateRejectsGuidanceAdapterAsProducer` must fail.

## T3 — Git root detection

Create:
- `internal/gitstate/gitstate.go` — `DetectRoot(start string) (string, error)`, `Head(repo string) (string, error)`.
- `internal/gitstate/gitstate_test.go`

Spec:
- `DetectRoot` walks up from `start` until it finds a `.git` dir/file; returns abs path. Errors if none found up to filesystem root.
- `Head` runs `git -C <repo> rev-parse HEAD`; errors if no commits yet (return a sentinel error `ErrNoCommits`).
- No shell; use `os/exec` directly with explicit args.

Tests that must pass (use real temp repos via `git init`):
- `TestDetectRootFindsGitDir`
- `TestDetectRootErrorsOutsideRepo`
- `TestHeadReturnsCommitSha`
- `TestHeadErrorsOnRepoWithNoCommits`

Mutation proof:
- Make `DetectRoot` stop one level early; `TestDetectRootFindsGitDir` must fail.

Notes:
- Do not mock git. Use `t.TempDir()` + `git init` + `git commit`. See conventions §4.

## T4 — Changed files + stable diff hash

Create:
- `internal/gitstate/diff.go` — `ChangedFiles(repo string) ([]string, error)`, `DiffHash(repo string, files []string) (string, error)`.
- `internal/gitstate/diff_test.go`

Spec:
- `ChangedFiles` returns the union of tracked-modified, staged, and untracked files (via `git status --porcelain=v1 -z` parsed). Paths are repo-relative, forward-slash.
- `DiffHash` returns a SHA-256 hex string that is stable for identical content+status and changes when either file content or file status changes. Hash over: for each file, its path, its status byte, and the content of the working-tree blob (read directly, not via git plumbing that may not be portable). Deterministic ordering (sort by path).
- Empty change set → well-defined hash (hash of empty input).

Tests that must pass:
- `TestChangedFilesDetectsModifiedStagedUntracked`
- `TestDiffHashStableForIdenticalContent`
- `TestDiffHashChangesWhenFileChanges`
- `TestDiffHashChangesWhenStatusChanges` (same content, modified vs staged → different hash)
- `TestDiffHashEmpty`

Mutation proof:
- Hash over path only (drop content); `TestDiffHashChangesWhenFileChanges` must fail.
- Hash over content only (drop status byte); `TestDiffHashChangesWhenStatusChanges` must fail.

## T5 — Path policy

Create:
- `internal/pathpolicy/pathpolicy.go` — `type Policy struct{...}`, `NewDefault() Policy`, `(p Policy) Check(repoRoot, rel string) error`, `(p Policy) Abs(repoRoot, rel string) (string, error)`.
- `internal/pathpolicy/pathpolicy_test.go`

Spec:
- Default policy forbids: `.env`, `.env.*`, `secrets/**`, any path matching `**/*private*key*` (case-insensitive).
- `Check` rejects: any `..` traversal; any resolved absolute path outside `repoRoot` (after `filepath.Symlink` resolution); any forbidden-pattern match.
- `Abs` resolves `rel` under `repoRoot`, evaluates symlinks, and verifies the result is still under `repoRoot`; returns the abs path or an error.
- Repo-relative generated paths under `.ai/`, `.git/`, root adapters are allowed by default.

Tests that must pass:
- `TestPathPolicyRejectsTraversal`
- `TestPathPolicyRejectsSecretPaths` (`.env`, `.env.production`, `secrets/db.pem`, `id_rsa_private_key`)
- `TestPathPolicyRejectsSymlinkEscape` (create a symlink under repoRoot pointing outside; `Abs` must error)
- `TestPathPolicyAllowsRepoRelativeGeneratedPaths`
- `TestPathPolicyAbsRejectsAbsOutsideRepo`

Mutation proof:
- Disable traversal rejection; `TestPathPolicyRejectsTraversal` must fail.
- Skip symlink resolution; `TestPathPolicyRejectsSymlinkEscape` must fail.

## Verification

```bash
go test ./internal/config/... ./internal/gitstate/... ./internal/pathpolicy/... -count=1
go vet ./internal/config/... ./internal/gitstate/... ./internal/pathpolicy/...
grep -rnE 'http\.|net\.Dial' ./internal/config ./internal/gitstate ./internal/pathpolicy || echo "no network"
```

WS1 is ☑ when:
- [ ] all listed tests pass
- [ ] mutation proofs executed and reverted (5 total)
- [ ] `go vet` clean
- [ ] no network calls
- [ ] status updated in `00-overview.md`
