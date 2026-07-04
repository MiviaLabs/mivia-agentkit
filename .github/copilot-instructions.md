# GitHub Copilot Instructions

Root `AGENTS.md` is canonical. Use it for project overview, Go standards, testing, security, documentation workflow, and git workflow.

Copilot-specific rules:

- Read `.ai/INDEX.md` and relevant `.ai/rules/*.md` before suggesting implementation.
- Do not create Go source code unless a specific `docs/plans/human/ws-XX/tasks.md` task is in scope.
- Prefer patches that include tests, mutation-proof notes, and documentation references.
- Do not suggest network access, credential persistence, raw prompt/output storage, or CI workflows unless a later workstream explicitly asks for them.
