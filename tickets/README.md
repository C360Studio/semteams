# SemTeams Work Tickets

Tickets in this directory are YAML planning records. They track approved
work and dependencies; they are not an execution engine or a second product
authority.

## Schema

Each `*.yaml` file uses:

- `ticket`: stable identifier.
- `title`: concise outcome.
- `owner`: one role from the repository sub-agent roster.
- `reviewer`: one or more roster roles that enforce the quality gate.
- `status`: `ready`, `blocked`, `in_progress`, or `done`.
- `depends_on` and `blocks`: ticket identifiers; use `[]` when empty.
- `summary`: why the work exists and its boundary.
- `acceptance_criteria`: observable conditions required for completion.
- `non_goals`: scope explicitly excluded from the ticket.
- `source_references`: repository or pinned-upstream evidence for the work.
- `blockers`: genuine external impediments not represented by a ticket
  dependency; use `[]` when none. Do not restate ticket work or acceptance
  criteria here.

A ticket with unmet `depends_on` entries has `status: blocked` even when its
`blockers` list is empty. It becomes ready only after every dependency and
external blocker is resolved. Reviewers sign off after the listed acceptance
criteria are verified.
