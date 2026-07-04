#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
cd "$ROOT"

chmod +x .githooks/pre-commit .githooks/pre-push
chmod +x scripts/git-hooks/pre-commit scripts/git-hooks/pre-push

git config core.hooksPath .githooks

printf 'Installed repo Git hooks via core.hooksPath=.githooks\n'
printf 'Required local commands: python3, semgrep, go/gofmt once Go code exists\n'
