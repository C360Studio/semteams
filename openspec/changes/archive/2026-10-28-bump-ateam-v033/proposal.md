# Bump the ATeam pin to v0.3.3

## Why

[Issue #364](https://github.com/C360Studio/semteams/issues/364): SemTeams pins `ateam@v0.3.2`, whose `ateam review gate` does not read `Verdict:` (`C360Studio/ateam#201`). [`v0.3.3`](https://github.com/C360Studio/ateam/releases/tag/v0.3.3) (`115a828`) ships the fix (`#211`): the latest review record must carry exactly one `Verdict: APPROVE` line outside code fences (`internal/review/gate.go:534-536`, `:825-839` at `v0.3.3`). Owner ruling on epic #375 (2026-10-27): match ateam unless there is a very good reason not to, and record any divergence.

## What changes

- Pin `ateam@v0.3.3` in the `Go`, `Governance` and `Merge check` jobs of `.github/workflows/ci.yml` and in `scripts/doctor.sh`.
- `repository-governance`: the governance and `Merge check` scenarios name ATeam v0.3.3, and the gate's verdict check is mechanical rather than review only.
- Sync the harness to the v0.3.3 templates that `ateam doctor --check` holds: the `design` skill and adapters (`ateam#178`), Gemini's nested `context.fileName` (`ateam#183`), and `.agents/contracts/{developer,reviewer}.md`; re-sync the unmodified `handoff`, `ponytail` and `review-pr` skills (`review-pr` § 5 now reruns the failed `Merge check`, `ateam#192`).
- `.agents/contracts/architect.md` carries ateam's new base architect contract verbatim, followed by the SemTeams specialist content, so the `architect` adapters keep one contract path.

## Non-goals

- Adopting or declining ateam's proposal `Approved-by:` / `Hold: OPEN-Q` default (`ateam#142`), the design-track protocol rows (`ateam#178`), or the `Approved-by` merge-check note (`ateam#182`): owner ruling #374.
- Retiring workarounds for upstream issues still open at v0.3.3 (`ateam#204`, `#205`, `#206`, `#207`, `#209`), including `scripts/openspec-queue.sh` (#369).

## Impact

A code/UI PR's `Merge check` now fails on a latest record with `Verdict: REQUEST CHANGES`, no `Verdict:` line, or more than one. `ateam review gate` prints a note for an OpenSpec change whose `proposal.md` has no filled `Approved-by:`; it is a note, not a failure.
