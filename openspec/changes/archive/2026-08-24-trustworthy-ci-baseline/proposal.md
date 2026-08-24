# Change: Establish A Trustworthy CI Baseline

## Why

SemTeams has separate path-filtered Go and UI workflows, so neither provides one dependable status for every pull
request. The copied `semspec-validation.yml` workflow is also not a valid SemTeams check. Before branch protection is
considered, the repository needs one small CI workflow that runs the commands maintainers already use and always reports
the same aggregate context.

Work authority is GitHub issue #254 and draft PR #262.

## What Changes

- Replace the fragmented merge checks with one unconditional `Repository CI` workflow and one stable
  `CI Status Check` aggregate.
- Run clear Go, UI, and repository-governance jobs using repository commands.
- Pin validation semantics to OpenSpec 1.7.0, Task 3.51.1, revive 1.15.0, Node 22.20.0, and the Go version in `go.mod`.
  Use official GitHub Action major tags, consistent with sibling repositories.
- Retire the path-filtered `ui.yml` and broken copied `semspec-validation.yml` workflows.
- Keep the new workflow name distinct from `CI`, so the existing container workflow's
  `workflow_run.workflows: ["CI"]` listener cannot publish from it.

## Non-goals

- Required mock E2E; this slice does not add an E2E merge gate.
- Ruleset activation or a multi-head hosted proof.
- An immutable action-SHA policy.
- Static integration-boundary ratchets or NATS sleep cleanup.
- Release or container redesign, which remains owned by issue #259.
