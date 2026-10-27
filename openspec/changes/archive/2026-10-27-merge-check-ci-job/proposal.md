# Run `ateam review gate` as a `Merge check` CI job from the base commit

## Why

[Issue #372](https://github.com/C360Studio/semteams/issues/372): after #363, the merge gate (`task merge:check -- <pr>`, `scripts/merge-check.sh` → `ateam review gate`) runs only by hand. `Required` can be green on a code PR that has no `reviewed-by:` record, has a weaker reviewer, or has an open `class:flake`, so the gate is advisory. Owner ruling on #372 and #375 (2026-10-27): match ateam, whose own `ci.yml` runs a `Merge check` job from the base commit and makes `Required` depend on it.

## What changes

- Add a `merge-check` job (`name: Merge check`) to `.github/workflows/ci.yml`. It checks out the base commit (`github.event.pull_request.base.sha || github.sha`) without persisted credentials, installs `ateam@v0.3.2`, and runs `scripts/merge-check.sh "$PR_NUMBER"` with read-only `contents`, `issues` and `pull-requests` permissions.
- `Required` needs `merge-check` alongside `go`, `ui` and `governance`.
- Add a `repository-governance` requirement for the job.
- Trigger `pull_request` on `ready_for_review` too, and adopt ateam's `concurrency` block, so the run that marking a
  draft ready starts applies the review check and lands last.

Recorded divergences from ateam: ATeam is installed from the pinned `go install` rather than built from the base tree
(this is not the ATeam repository); `pull_request.branches: [main]` stays; exit 2 (a forge read was unavailable) gets no
special handling beyond the gate's own `unavailable` log line, as in ateam. Known limits: `ci.yml` is read from the
pull request, so editing the job is caught by review only; the shared concurrency group can cancel a *pending* push run
on `main` when several merges land quickly, which leaves that commit without `Required` until rerun. ateam's
`review-pr` § 5 rerun step is not in the v0.3.2 skill, so `.agents/protocol.md` carries it until #364.

## Non-goals

- Enabling ateam's dormant `routes` / `ateam review check` path.
- The `Gate files` workflow and `gate-change-approved` label.

## Impact

Every code/UI PR shows `Merge check`, and therefore `Required`, red until an approving cross-agent record exists. The reviewer then reruns the failed job (ateam `review-pr` § 5). Docs-only PRs pass without review lines. On pushes to `main` the gate checks only the up-to-date rule.
