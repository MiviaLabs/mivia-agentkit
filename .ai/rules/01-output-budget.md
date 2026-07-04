# Output Budget

## During Work

- Send only status updates that change the user's understanding: current scope, discovered blocker, file edits about to happen, verification progress, or failed command.
- Keep each status update to one or two short sentences.
- Do not paste raw logs unless the user asks. Report the command and the key failing line or reason.

## Final Responses

- Include outcome, changed files, verification, and residual risk or blockers.
- Prefer concise bullets for multi-file work.
- Do not include broad rationale, repeated context, or external research summaries unless they are needed to understand the result.

## Task Slicing

- For implementation work, finish one task file plus its test before starting the next task.
- For audits, report concrete findings with file references and tests that would catch each issue.
- For handoffs, include read-first files, exact scope, verification commands, and mutation-proof targets.
