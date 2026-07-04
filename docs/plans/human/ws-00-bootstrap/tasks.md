# WS0 — Repo Bootstrap

- **Phase:** 0
- **Depends on:** —
- **PRD:** §1, §4, §9 (NFR-1, NFR-2)
- **Plan:** WS0, "Repository Architecture"
- **Exit gate (Phase 0):** `go test ./...` green; `mivia-agent --help` works.

Goal: a compilable Go module with a Cobra root command, version command, CI, and the foundational repo files. Nothing domain-specific yet.

## T1 — Go module + main entrypoint

Create:
- `go.mod` — module `github.com/MiviaLabs/mivia-agentkit`, Go 1.22+.
- `cmd/mivia-agent/main.go` — calls `cli.Execute()` (package `cli`), prints any error and exits non-zero.

Spec:
- Module path exactly `github.com/MiviaLabs/mivia-agentkit`.
- `main()` is thin: `if err := cli.Execute(); err != nil { fmt.Fprintln(os.Stderr, err); os.Exit(1) }`.
- No business logic in `main`.

Dependencies: stdlib only.

Notes:
- Pin Go version to the latest stable at implementation time (≥1.22 for `slices`, `maps`).

## T2 — Cobra root + version commands

Create:
- `internal/cli/root.go` — package `cli`; `rootCmd *cobra.Command`; `Execute() error`; `NewRootCommand()` constructor.
- `internal/cli/version.go` — `versionCmd`; prints `mivia-agent <version>` where version is injected via `internal/version`.
- `internal/version/version.go` — `var Version = "dev"` (overridable at link time via `-ldflags`).
- `internal/cli/cli_test.go` — root/version tests.

Spec:
- Root command use=`mivia-agent`, short description present.
- `--help` exits 0.
- Version command prints `mivia-agent <Version>` on a single line.
- Unknown command exits non-zero.

Tests that must pass:
- `TestRootCommandShowsHelp`
- `TestVersionCommandPrintsVersion`
- `TestUnknownCommandExitsNonZero`

Mutation proof:
- Change `Version` print to omit the version string; `TestVersionCommandPrintsVersion` must fail.

## T3 — Project metadata + .gitignore

Create:
- `README.md` — one-paragraph summary, install placeholder, link to `docs/prd/0001-mivia-agentkit.md`.
- `LICENSE` — already exists (Apache-2.0/MIT per org norm; confirm at impl time).
- `.gitignore` — Go defaults: `/dist/`, `/bin/`, `*.test`, `coverage.out`, `.ai/runs/` (run artifacts never committed).

Spec:
- README links to PRD and to `docs/plans/human/00-overview.md`.
- `.gitignore` excludes generated run artifacts.

Dependencies: none.

## T4 — Template source directory skeleton

Create:
- `templates/README.md` — explains: this is the source-controlled template directory. Files here are embedded into the binary at build time via `//go:embed` in `internal/templates/templates.go`. They are NOT `.ai/` files; they are the raw material that `init` renders into a target repo's `.ai/` and root-adapter files. See plan "Distribution Model" > "Where templates live".
- `templates/core/INDEX.md` — placeholder with a comment `<!-- mivia-agent: replace with canonical INDEX template in WS2 -->`.
- `templates/core/rules/` — empty directory with a `.gitkeep`.
- `templates/core/skills/` — empty directory with a `.gitkeep`.
- `templates/core/quality/contracts/` — empty directory with a `.gitkeep`.
- `templates/core/quality/review-policies/` — empty directory with a `.gitkeep`.
- `templates/adapters/codex/` — empty with `.gitkeep`.
- `templates/adapters/claude/` — empty with `.gitkeep`.
- `templates/adapters/copilot/` — empty with `.gitkeep`.
- `templates/adapters/gemini/` — empty with `.gitkeep`.
- `templates/adapters/crush/` — empty with `.gitkeep`.
- `templates/workflows/` — empty with `.gitkeep`.
- `templates/prompts/` — empty with `.gitkeep`.
- `templates/ci/github-actions/` — empty with `.gitkeep`.

Spec:
- The skeleton is committed as part of the initial repo. WS2 populates each directory with real template files and removes the placeholders.
- No template content yet — just the directory structure so that `go:embed` has something to reference (even if empty) and the `internal/templates/templates.go` embed call can compile.
- `templates/README.md` is the authoritative explanation of what lives here and why.

Tests that must pass:
- `TestTemplatesDirExists` — assert `templates/README.md` is readable.
- `TestTemplatesSubdirsExist` — assert each named subdirectory exists.

Mutation proof:
- Delete `templates/README.md`; `TestTemplatesDirExists` must fail.

## T5 — CI workflow

Create:
- `.github/workflows/ci.yml` — Go test + vet on push/PR.

Spec:
- Matrix: latest stable Go on ubuntu-latest (macOS/windows optional but recommended for the cross-platform binary claim).
- Steps: checkout, setup-go, `go vet ./...`, `go test ./... -count=1`.
- Caches the module directory.

Notes:
- Do not add release workflow here (WS8).

## Verification

```bash
go test ./... -count=1
go vet ./...
go run ./cmd/mivia-agent --help
go run ./cmd/mivia-agent version
test -f templates/README.md && echo "templates/README.md exists"
```

WS0 is ☑ when:
- [ ] all listed tests pass (including `TestTemplatesDirExists`, `TestTemplatesSubdirsExist`)
- [ ] `--help` and `version` work
- [ ] `templates/` skeleton committed (README.md + all subdirectories with .gitkeep)
- [ ] CI green on a feature branch
- [ ] mutation proof executed + reverted
- [ ] status updated in `00-overview.md`
