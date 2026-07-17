# {{ .Project.Name }} Agent Control Surface

Profile: {{ .Profile }}
Managed by {{ .Binary }} {{ .Version }}.

Rules live in `.ai/rules/`, skills in `.ai/skills/`, workflows in `.ai/workflows/`.

Ordinary deep-bug-audit is report-only. Optional supervised campaigns use
`mivia-agent campaign run` and stay disabled by default in `mivia-agent.yaml`.
