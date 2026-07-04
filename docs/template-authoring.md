# Template Authoring

PRD references: `FR-1.1` to `FR-1.4`, `FR-10.6`, `NFR-1`

## Template Source

Committed templates live under `templates/` and are embedded through [internal/templates/templates.go](../internal/templates/templates.go). The binary is the source of truth at runtime; `update` refreshes managed files from embedded templates, not from the network.

## Managed Blocks

Managed files preserve user-owned text outside:

```text
mivia-agent:managed:start
...
mivia-agent:managed:end
```

Use managed blocks when a file is partly repo-owned and partly user-owned. Whole-file managed outputs, such as `mivia-agent.yaml`, should stay deterministic and idempotent.

## Template Variables

Current render paths consume stable data such as:

- selected profile
- selected adapters
- project name
- generated workflow defaults

Keep variable inputs deterministic. Sort adapter names, file lists, and generated sections before rendering.

## `List(profile, adapters)` Contract

[internal/templates/templates.go](../internal/templates/templates.go) exposes `List(profile, adapters)` and defines the expected generated file set for a profile and adapter mix.

That contract must stay:

- deterministic in ordering
- adapter-sensitive
- profile-sensitive
- aligned with the embedded template tree

If `List` changes, update its tests and any generated-fixture expectations that depend on the standard profile file set.

## Authoring Checklist

- Add the committed template under `templates/`.
- Make sure the embedded path matches the committed source exactly.
- Keep adapter shims thin; point back to `.ai/INDEX.md` instead of duplicating policy.
- Add or update idempotency coverage when a rendered file changes shape.
