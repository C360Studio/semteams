# Adopt ATeam v0.3.4: separate Merge check workflow, draft deferral, and retire openspec-queue.sh

Approved-by:

Track: design (edits `.github/workflows/`, a gate file, and modifies `repository-governance` Requirements).

## Why

[ATeam v0.3.4](https://github.com/C360Studio/ateam/releases/tag/v0.3.4) shipped on 2026-10-29. It fixes the last
upstream blocker on epic #375 (`C360Studio/ateam#205`: `ateam state spec-check` now runs strict validation with
`openspec` from PATH), so #369 can retire `scripts/openspec-queue.sh`. It also changes ateam's own merge-gate
layout. `Merge check` moves into its own workflow, which also runs on `edited`, so the reviewer's `reviewed-by:` edit reruns it and an
open `class:flake` no longer turns `Required` red (`ateam#255`). Draft PRs also skip and rename every CI job, so a draft head carries no
passing `Required` (`ateam#250`). Our `Merge check` still runs inside `Repository CI` under `Required`
(`.github/workflows/ci.yml:135-187`). Owner ruling on #375: match ateam unless there is a very good reason not to.
Owner scope ruling (Enjoy, 2026-10-29): one change covering the pin bump, #369, the workflow split, and draft deferral.

## What changes

- Pin `ateam@v0.3.4` in `ci.yml` and `scripts/doctor.sh`. Sync `.agents/contracts/{developer,reviewer}.md` and the
  `review-pr` skill to the v0.3.4 templates that `ateam doctor --check` holds.
- New `.github/workflows/merge-check.yml` carries the `Merge check` job, with ateam's triggers (adds `edited`),
  queued per-PR concurrency, the draft guard and rename, and `ATEAM_REQUIRED_CHECKS: Required,Merge check`. In
  `ci.yml`, `required` then needs only `go`, `ui`, and `governance`.
- Every `Repository CI` job skips on a draft PR and renames itself to `<name> (draft, skipped)`. Marking the PR ready
  runs the full checks.
- Retire `scripts/openspec-queue.sh`, its fixture test, and the `queue-strict` and `queue-test` tasks.
  `spec:check` becomes `openspec validate --all --strict` followed by `ateam state spec-check` (design D3).
- `repository-governance` spec, `AGENTS.md`, `.agents/rules-index.md`, `.agents/protocol.md`, the preflight skill,
  the technical-writer contract, `README.md`, and the contract tests follow.

## Non-goals

- ATeam's `Gate files` workflow (declined on #372) and `ateam state prune` (usable now, but adoption is a separate
  protocol edit).
- Test-to-spec attribution (#386, blocked on `C360Studio/ateam#237`).

## Impact

- **Owner action before merge:** add `Merge check` to the `main` ruleset's required checks. With
  `ATEAM_REQUIRED_CHECKS` set, the gate fails any PR whose ruleset lacks it, including this one (design D2).
- Code PRs no longer need `gh run rerun --failed` after review. Editing the description reruns `Merge check`.
- An open `class:flake` fails `Merge check` only, never `Required`. `Publish` still reads `Required` alone.
- Draft PRs use no CI minutes and show only skipped checks.
