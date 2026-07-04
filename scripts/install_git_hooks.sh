#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
cd "$ROOT"

chmod +x .githooks/pre-commit .githooks/pre-push .githooks/prepare-commit-msg
chmod +x scripts/git-hooks/pre-commit scripts/git-hooks/pre-push scripts/git-hooks/prepare-commit-msg

git config core.hooksPath .githooks

printf 'Installed repo Git hooks via core.hooksPath=.githooks\n'
printf 'Required local commands: python3, semgrep, go/gofmt once Go code exists\n'
