# WS2 — Templates + `init` + Global Layer

- **Phase:** 1
- **Depends on:** WS1
- **PRD:** FR-1.1, FR-1.2, FR-1.3, FR-6.1, FR-6.2, FR-10.1, FR-10.6
- **Plan:** WS2, "Generated Target Repo Files", "Manifest", "Config Hierarchy"
- **Exit gate (Phase 1, partial):** `init --dry-run` writes nothing; `init --write` creates the expected file set; idempotent; refuses to overwrite user files; `.agents/skills.json` includes both global and project skills.

Goal: populate the template source directory (skeleton created in WS0 T4), embed templates into the binary, render the full target-repo file set for a profile+adapter mix, support dry-run and write, preserve user content, keep init idempotent.

**Important:** the `templates/` directory is source-controlled in the agentkit repo (see plan "Distribution Model" > "Where templates live"). WS0 T4 creates the skeleton; this WS populates it with real template content. The templates are NOT `.ai/` files — they are the raw material that `init` renders into a target repo's `.ai/` and root-adapter locations. After build, they live only inside the binary (via `//go:embed`); the user never sees `templates/`.

## T1 — Template embedding + manifest

Create:
- `internal/templates/templates.go` — `//go:embed templates/**/*`, `FS() fs.FS`, `List(profile, adapters) ([]string, error)`.
- `internal/templates/templates_test.go`
- Populate `templates/core/INDEX.md` (replace the WS0 placeholder)
- Populate `templates/core/rules/*.md` (00-operating-doctrine.md, 01-output-budget.md, 10-security-privacy.md, 20-agent-quality.md)
- Populate `templates/core/skills/*/SKILL.md` (4 skill dirs: airtight-feature-delivery, test-coverage-audit, deep-bug-audit, adversarial-test-review)
- Populate `templates/core/quality/contracts/project-runtime.yaml`
- Populate `templates/core/quality/review-policies/default.yaml`
- Populate `templates/adapters/codex/*` (hook config, skills adapter)
- Populate `templates/adapters/claude/*` (settings, skills adapter)
- Populate `templates/adapters/copilot/*` (instructions)
- Remove the WS0 `.gitkeep` placeholders from all populated subdirectories

Spec:
- Templates are embedded via `embed.FS`; no disk reads at runtime.
- `List(profile, adapters)` returns the repo-relative output paths for the given profile+adapter mix (see plan "Generated Target Repo Files" for the standard set).
- Each template has a stable set of variables: `.Project.Name`, `.Profile`, `.Adapters.*`, `.Binary` (always `mivia-agent`), `.Version`.
- Two managed-block files (`AGENTS.md`, `CLAUDE.md`) use sentinel markers:
  ```
  <!-- mivia-agent:managed:start -->
  ...managed content...
  <!-- mivia-agent:managed:end -->
  ```

Tests that must pass:
- `TestEmbeddedFSContainsCoreFiles`
- `TestListStandardProfileReturnsExpectedFiles` (assert the exact standard set from the plan)
- `TestListRespectsAdapterSelection` (e.g. no `.codex/hooks.json` when codex disabled)
- `TestListIncludesWorkflowTemplatesForStandard` (`research-loop.yaml`, `bug-audit-loop.yaml`)

Mutation proof:
- Remove a core file from `templates/`; `TestEmbeddedFSContainsCoreFiles` must fail.

## T2 — Renderer

Create:
- `internal/render/render.go` — `type Renderer struct{...}`, `New() Renderer`, `(r Renderer) Render(tplName string, vars Vars) ([]byte, error)`, `(r Renderer) RenderAll(plan RenderPlan) (map[string][]byte, error)`.
- `internal/render/render_test.go`

Spec:
- Use `text/template` (no HTML escaping — these are markdown/yaml). Define a custom `funcs` map (`title`, `lower`, `join`).
- `Render` parses one template by name from the embedded FS and renders with `vars`.
- Unknown template variables are not silent: a referenced-but-undefined var errors; an undefined-but-unreferenced var is fine.
- `RenderAll` renders a `RenderPlan` (list of `{template, outPath}`) into a map keyed by `outPath`.

Tests that must pass:
- `TestRenderFillsProjectName`
- `TestRenderErrorsOnUndefinedReferencedVar`
- `TestRenderAllProducesAllExpectedOutputs`

Mutation proof:
- Remove the undefined-var check; `TestRenderErrorsOnUndefinedReferencedVar` must fail.

## T3 — Init command (dry-run + write + idempotency + overwrite guard)

Create:
- `internal/cli/init.go` — `initCmd *cobra.Command`, flags `--repo`, `--profile`, `--adapter` (repeated), `--with-loop` (repeated), `--dry-run`, `--write`, `--force`, `--json`.
- `internal/render/init.go` — `PlanInit(cfg InitConfig) (RenderPlan, error)`; the orchestration: read global config (WS1 `globalconfig.Read()`), load/merge manifest (`globalconfig.Layer()`), detect root, build plan.
- `internal/render/init_integration_test.go` — end-to-end over temp repos, including with and without `~/.agents/` present.

Spec:
- `--dry-run`: compute the plan, print intended writes (path + would-create/would-skip/would-conflict), write nothing. Exit 0 if no conflicts, non-zero on conflict.
- `--write`: render all, write each file. Before writing an existing user-owned file without the managed block, refuse unless `--force`. For managed-block files, only update the block (see T4).
- Global config from `~/.agents/` is read and layered under the project manifest before rendering. Global rules/skills are included in the effective config (project wins on conflict).
- `.agents/skills.json` in the target repo lists both global and project skills (merged, project wins on name conflict).
- After `--write`, call `doctor` (WS3) to validate. (For now, doctor is a stub the test skips if WS3 not present; wire fully in WS3.)
- Idempotent: `init --write` then `init --write` (same options) → no diff in the repo.
- `--adapter` accepts `codex|claude|copilot|gemini|crush`. Validate against manifest; unknown → error.
- `--json`: emit a structured report `{files_created, files_skipped, conflicts}`.

Tests that must pass:
- `TestInitDryRunWritesNothing` (assert no file touched; capture mtime or use a write-tracking FS wrapper)
- `TestInitWriteCreatesExpectedFiles`
- `TestInitWriteIsIdempotent` (run twice, `git diff --exit-code` clean in temp repo)
- `TestInitRefusesToOverwriteUserOwnedFile` (pre-create `AGENTS.md` without managed markers; expect conflict, file unchanged)
- `TestInitForceOverwritesUserOwnedFile`
- `TestInitRejectsUnknownAdapter`
- `TestInitReportJSONShape`
- `TestInitIncludesGlobalSkillsInSkillsJson` (global skill from `~/.agents/skills/` appears in `.agents/skills.json`)
- `TestInitProjectSkillOverridesGlobalSkill` (same name in both → project version wins in `.agents/skills.json`)
- `TestInitGlobalConfigAbsentNoError` (`~/.agents/` missing → init succeeds normally)

Mutation proof:
- Remove the overwrite guard; `TestInitRefusesToOverwriteUserOwnedFile` must fail (it will overwrite).
- Make `--dry-run` actually write; `TestInitDryRunWritesNothing` must fail.

## T4 — Managed-block update

Create:
- `internal/render/managedblock.go` — `ExtractManaged(content []byte) (pre, managed, post []byte, ok bool)`, `ReplaceManaged(original, newManaged []byte) ([]byte, error)`, `HasManaged(content []byte) bool`.
- `internal/render/managedblock_test.go`

Spec:
- Sentinel markers: `<!-- mivia-agent:managed:start -->` / `<!-- mivia-agent:managed:end -->` (markdown) and `# mivia-agent:managed:start` / `# mivia-agent:managed:end` (yaml).
- `ExtractManaged` returns the three sections; `ok=false` if no markers.
- `ReplaceManaged` preserves `pre` and `post` exactly; replaces only `managed`.
- If a file has managed markers but the new managed content differs, the file is rewritten; if identical, no write (idempotency contributor).

Tests that must pass:
- `TestManagedBlockExtractRoundTrip`
- `TestManagedBlockUpdatePreservesUserText` (user text in `pre`/`post` survives)
- `TestManagedBlockNoChangeProducesNoDiff`
- `TestManagedBlockRejectsMalformedMarkers` (start without end → error)

Mutation proof:
- Have `ReplaceManaged` overwrite `pre` too; `TestManagedBlockUpdatePreservesUserText` must fail.

## Verification

```bash
go test ./internal/templates/... ./internal/render/... ./internal/cli/... -count=1
go vet ./internal/templates/... ./internal/render/... ./internal/cli/...
# Smoke:
tmp=$(mktemp -d) && (cd "$tmp" && git init -q && git commit -q --allow-empty -m init)
go run ./cmd/mivia-agent init --repo "$tmp" --profile standard \
  --adapter codex --adapter claude --adapter copilot --dry-run
go run ./cmd/mivia-agent init --repo "$tmp" --profile standard \
  --adapter codex --adapter claude --adapter copilot --write
( cd "$tmp" && find . -type f -not -path './.git/*' | sort )
```

WS2 is ☑ when:
- [ ] all listed tests pass
- [ ] dry-run truly writes nothing
- [ ] idempotency proven on a temp repo
- [ ] overwrite guard + managed-block preservation proven
- [ ] mutation proofs executed and reverted
- [ ] status updated in `00-overview.md`
