# Design: `task validate` local-parity Requirement

## Context

- `scripts/validate.sh` runs the Go, UI, and Governance lanes in a fixed order. Before it runs anything, it checks for
  each tool and for `ui/node_modules`, and it fails if Docker is unreachable. It leaves no trace in the worktree:
  `TestValidateScriptCommandCompositionAndWorktreePreservation` (`test/contract/validate_task_contract_test.go:407-632`)
  snapshots the worktree and checks that transient directories are cleaned up.
- `TestValidateTaskMirrorsRepositoryCI` (`:129`) maps each `ci.yml` step to the `validate.sh` fragment that runs it.
  It legitimately cites "Repository CI runs the obvious repository checks".
- `TestValidateTaskAndDocsContract` (`:634`) checks three things: that the Taskfile carries the `validate` and
  `verify` entries, that `scripts/verify.sh` delegates to `validate.sh`, and that `AGENTS.md` collapses "Before
  pushing" to `task validate`.

## Goals / Non-Goals

The goal is a Requirement that these two tests actually verify, with nothing added beyond what the code already does.
Non-goal: changing any behavior.

## Decisions

### D1. Where the Requirement lives

- **A (recommended): `repository-governance`.** That spec already owns Repository CI, and the new Requirement is
  defined as parity with CI. This touches one capability.
- B: a new `local-verification` capability. That adds another spec file for a single Requirement.

### D2. How much it promises

- **A (recommended): only what the two tests assert.** That covers lane parity (by reference to the step mapping),
  worktree preservation, failing closed on missing prerequisites and drift, the `task verify` alias, and the
  `AGENTS.md` pre-push block.
- B: also promise timing or lane-isolation properties. No test covers those, so the spec would claim more than the
  code proves.

## Risks / Trade-offs

Re-citing the two tests changes their attached comments, so `test:spec --base` treats them as changed. The new
citations must resolve, which they do once the delta is archived, because in-flight `openspec/changes/<id>/specs`
citations also resolve.

## Migration Plan

One squash merge. There is no runtime change.

## Open Questions

1. **Owner ruling: D1 A and D2 A**, or name a different option.
