SHELL := /usr/bin/env bash
.SHELLFLAGS := -euo pipefail -c

.PHONY: help install-hooks hooks verify verify-agent semgrep-validate semgrep-test hook-test agent-hook-test audit-loop-test plan-contract-test skill-contract-test telemetry-contract-test semgrep pre-commit pre-push go-check test vet build

help:
	@printf '%s\n' \
		'Targets:' \
		'  make install-hooks     Install repo Git hooks for this clone' \
		'  make verify            Run all local quality gates' \
		'  make pre-commit        Run the committed pre-commit hook' \
		'  make pre-push          Run the committed pre-push hook' \
		'  make semgrep           Run repo Semgrep policy scan' \
		'  make semgrep-test      Run Semgrep rule contract tests' \
		'  make hook-test         Run Git hook contract tests' \
		'  make agent-hook-test   Run agent hook guard contract tests' \
		'  make audit-loop-test   Run audit loop Stop-hook contract tests' \
		'  make plan-contract-test Run agent plan contract and hook tests' \
		'  make skill-contract-test Run skill report contract tests' \
		'  make telemetry-contract-test Run report telemetry contract tests' \
		'  make go-check          Run Go format/test/vet/build when go.mod exists'

install-hooks hooks:
	@scripts/install_git_hooks.sh

verify: verify-agent semgrep-validate semgrep-test hook-test agent-hook-test audit-loop-test plan-contract-test skill-contract-test telemetry-contract-test semgrep go-check

verify-agent:
	@python3 scripts/verify_agent_config.py

semgrep-validate:
	@semgrep --validate --config semgrep/agent-standards.yml

semgrep-test:
	@python3 scripts/test_semgrep_rules.py

hook-test:
	@python3 scripts/test_git_hooks.py

agent-hook-test:
	@python3 scripts/test_agent_hook_guard.py

audit-loop-test:
	@python3 scripts/test_audit_loop_guard.py

plan-contract-test:
	@python3 scripts/test_agent_plan_contracts.py
	@python3 scripts/test_plan_hook_guard.py

skill-contract-test:
	@python3 scripts/test_skill_contracts.py

telemetry-contract-test:
	@python3 scripts/test_report_telemetry_contracts.py

semgrep:
	@semgrep --config semgrep/agent-standards.yml --error --skip-unknown-extensions --metrics off --disable-nosem .

pre-commit:
	@.githooks/pre-commit

pre-push:
	@.githooks/pre-push

go-check:
	@if [[ ! -f go.mod ]]; then \
		printf 'go.mod not present; skipping Go checks\n'; \
		exit 0; \
	fi; \
	mapfile -t files < <(git ls-files '*.go'); \
	if ((${#files[@]})); then \
		unformatted="$$(gofmt -l "$${files[@]}")"; \
		if [[ -n "$$unformatted" ]]; then \
			printf 'gofmt required for:\n%s\n' "$$unformatted" >&2; \
			exit 1; \
		fi; \
	fi; \
	go test ./...; \
	go vet ./...; \
	if [[ -d cmd/mivia-agent ]]; then go build ./cmd/mivia-agent; fi

test:
	@if [[ -f go.mod ]]; then go test ./...; else printf 'go.mod not present; skipping go test\n'; fi

vet:
	@if [[ -f go.mod ]]; then go vet ./...; else printf 'go.mod not present; skipping go vet\n'; fi

build:
	@if [[ -d cmd/mivia-agent ]]; then go build ./cmd/mivia-agent; else printf 'cmd/mivia-agent not present; skipping build\n'; fi
