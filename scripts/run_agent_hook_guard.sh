#!/usr/bin/env bash
set -euo pipefail

AGENT="${1:?agent surface required}"
EVENT="${2:?hook event required}"

ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
PAYLOAD_FILE="$(mktemp)"
trap 'rm -f "$PAYLOAD_FILE"' EXIT
cat >"$PAYLOAD_FILE"

set +e
GUARD_OUTPUT="$(python3 "$ROOT/scripts/agent_hook_guard.py" "$AGENT" "$EVENT" <"$PAYLOAD_FILE")"
GUARD_STATUS=$?
set -e

if [[ "$GUARD_STATUS" -ne 0 ]]; then
  if [[ -n "$GUARD_OUTPUT" ]]; then
    printf '%s\n' "$GUARD_OUTPUT"
  fi
  exit "$GUARD_STATUS"
fi

if [[ -n "$GUARD_OUTPUT" ]]; then
  printf '%s\n' "$GUARD_OUTPUT"
  exit 0
fi

set +e
AUDIT_LOOP_OUTPUT="$(python3 "$ROOT/scripts/audit_loop_guard.py" "$AGENT" "$EVENT" <"$PAYLOAD_FILE")"
AUDIT_LOOP_STATUS=$?
set -e

if [[ "$AUDIT_LOOP_STATUS" -ne 0 ]]; then
  if [[ -n "$AUDIT_LOOP_OUTPUT" ]]; then
    printf '%s\n' "$AUDIT_LOOP_OUTPUT"
  fi
  exit "$AUDIT_LOOP_STATUS"
fi

if [[ -n "$AUDIT_LOOP_OUTPUT" ]]; then
  printf '%s\n' "$AUDIT_LOOP_OUTPUT"
  exit 0
fi

set +e
PLAN_OUTPUT="$(python3 "$ROOT/scripts/plan_hook_guard.py" "$AGENT" "$EVENT" <"$PAYLOAD_FILE")"
PLAN_STATUS=$?
set -e

if [[ "$PLAN_STATUS" -ne 0 ]]; then
  if [[ -n "$PLAN_OUTPUT" ]]; then
    printf '%s\n' "$PLAN_OUTPUT"
  fi
  exit "$PLAN_STATUS"
fi

if [[ -n "$PLAN_OUTPUT" ]]; then
  printf '%s\n' "$PLAN_OUTPUT"
  exit 0
fi

if command -v mivia-agent >/dev/null 2>&1; then
  exec mivia-agent hook "$AGENT" "$EVENT" <"$PAYLOAD_FILE"
fi
