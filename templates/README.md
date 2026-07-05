# Mivia AgentKit Templates

This is the source-controlled template directory. Files here are embedded into the binary at build time via `//go:embed` in `internal/templates/templates.go`. They are not `.ai/` files; they are the raw material that `init` renders into a target repo's `.ai/` and root-adapter files. See the product plan's "Distribution Model" section for the template packaging contract.
