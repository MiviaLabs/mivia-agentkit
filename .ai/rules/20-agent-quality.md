# Agent Quality

## Tests

- Write the listed tests before or alongside production code.
- Every exported function must have at least one success-path test and one relevant error-path test unless the task file explicitly narrows the contract.
- Use `t.TempDir()` for filesystem writes and real temp Git repos for Git behavior.
- Helpers must call `t.Helper()` and fail through `testing.TB`; they must not return opaque booleans that hide why a check failed.

## Mutation Proofs

- Every reject, deny, guard, stale-check, path-policy check, idempotency claim, and secret-scrubbing rule needs a mutation proof.
- Execute the mutation by making the described code change, confirm the named test fails, revert the mutation, and record the result in the completion report.
- Do not claim mutation proof from code inspection alone.

## Reviews

- Before merge-ready claims, run an adversarial review of the changed behavior and tests.
- Review tests for false positives: remove the implementation guard mentally and verify at least one test would fail.
- Any residual risk must name the missing test, missing fixture, or external behavior that remains unproven.

## Coverage

- Coverage percentage is secondary. Contract coverage is required.
- Each workstream must cover success paths, error paths, malformed inputs, idempotency, secret hygiene, and no-network constraints where applicable.
- For hook and adapter code, test the real payload shape and scrubbed output shape.
