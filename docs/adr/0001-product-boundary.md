# ADR 0001: Product Boundary

- Status: accepted
- Date: 2026-07-05
- PRD: §1, §3, §4, `FR-7.3`, `FR-7.4`, `NFR-1`

## Decision

`mivia-agent` stays a single local binary with no hosted control plane.

The product boundary is:

- no required service, database, or container runtime
- no network calls from `mivia-agent` itself
- no persistence of raw prompts, raw model output, provider payloads, or secrets
- adapter-based CLI integration instead of provider-specific product code
- optional AGT-backed governance behind the same local decision interface

## Consequences

- Distribution happens through release binaries or `go install`.
- Runtime templates are embedded in the binary.
- Repo-local `.ai/` remains the canonical control surface.
- Governance can harden over time without changing the public CLI shape.
- New agent CLIs land as adapters plus templates, not as parallel product surfaces.
