# Specify `task validate` as the read-only local mirror of Repository CI

Approved-by: Coby Leuschke (2026-10-31)

Track: design, because it adds a `repository-governance` Requirement.

## Why

Issue #393, found by go-reviewer on #391. Two tests in `test/contract/validate_task_contract_test.go`, at `:407` and
`:634`, cite "Repository CI runs the obvious repository checks":

- `TestValidateScriptCommandCompositionAndWorktreePreservation`
- `TestValidateTaskAndDocsContract`

They prove a different thing: `scripts/validate.sh` runs the CI lanes in order, fails closed on missing prerequisites
and drift, and leaves the worktree unchanged; `task verify` delegates to it; and `AGENTS.md` tells agents to run it
before pushing. No Requirement in `openspec/specs/` covers that today. `AGENTS.md` § 4 states the rule, but nothing in
the specs does.

## What changes

- `repository-governance` gains the Requirement "`task validate` is the read-only local mirror of Repository CI", with
  scenarios covering:
  - lane parity
  - worktree preservation
  - failing closed on a missing prerequisite or on drift
  - the `task verify` alias
- The two tests are re-cited to the new Requirement.
- `TestValidateTaskMirrorsRepositoryCI` keeps its current citation. It does prove what CI runs.

## Non-goals

- Any change to `scripts/validate.sh`, `Taskfile.yml`, or CI.

## Impact

None at runtime. The only change is that the two tests cite a Requirement that matches what they verify.

Closes #393.
