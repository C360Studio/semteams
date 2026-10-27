# Align the `/work` review overlay with ATeam's review gate, reviewer lanes, and paginated comments

## Why

Cross-agent review of [PR #363](https://github.com/C360Studio/semteams/pull/363#issuecomment-6438822755) left three
non-blocking findings on the `/work` `review` overlay in `ui/src/lib/server/work/githubSource.ts`:

- [Issue #365](https://github.com/C360Studio/semteams/issues/365) `M2`: `review_unparsed` was sticky, so an older
  unparseable comment (an implementer reply quoting a verdict and SHA, or pre-ATeam prose) held the overlay at `unknown`
  after a newer valid `Review record` at `HEAD`.
- [Issue #365](https://github.com/C360Studio/semteams/issues/365) `M3`: conditional verdicts such as
  `Verdict: APPROVE pending H1 fix` or `Verdict: APPROVE, but REQUEST CHANGES on H1` mapped to `pass`.
- [Issue #366](https://github.com/C360Studio/semteams/issues/366): only the latest `Review record` counted, so a
  `svelte-reviewer` `REQUEST CHANGES` followed by a `go-reviewer` `APPROVE` at `HEAD` read as `current`; and only the
  first `GITHUB_PAGE_LIMIT` (100) comments were read, so an `APPROVE` on page 1 could hide a `REQUEST CHANGES` on page 2.

The owner ruling on epic [#375](https://github.com/C360Studio/semteams/issues/375) is to match ATeam unless there is a
very good reason not to. The reference is `C360Studio/ateam` `internal/review/gate.go` at `36b999b`: `record()`
(lines 500-527) skips comments that are not a valid record and keeps the latest valid one; `gateVerdict()` (lines
823-840) approves only on exactly one unfenced `Verdict:` line whose whole value is `APPROVE`.

## What changes

- A `Review record` maps to `pass` only when it carries exactly one `Verdict:` line outside code fences and that line,
  trimmed, is `Verdict: APPROVE` (case-insensitive), matching `gateVerdict()`. Conditional, decorated, or
  multi-line approvals (`APPROVE pending H1`, `APPROVE with nits`, `**Verdict**: APPROVE`, `` Verdict: `APPROVE` ``,
  `APPROVED`) do not pass; they leave the comment unparsed so the overlay fails closed to `unknown`. Leading
  `REQUEST CHANGES` / `CHANGES REQUESTED` / `BLOCKING` / `FAIL` values still map to `blocking`.
- `review_unparsed` is no longer sticky: a later valid `Review record` clears an earlier unparseable comment, matching
  `record()`. An unparseable comment that names a reviewer lane is cleared only by a later record covering that lane or
  an all-lanes record. An unparseable record-shaped comment posted after the latest valid record still fails closed.
- `parseReviewComments(comments, headSha?)` aggregates valid records at `HEAD` across distinct reviewer lanes: a record
  without a lane (or naming every lane it covers) resets the lanes it covers, and a blocking lane stays effective until a
  newer record for that lane or an all-lanes record supersedes it. ATeam has no counterpart; it keys on one
  `reviewed-by:` reviewer, while SemTeams's `go-reviewer` / `svelte-reviewer` lanes are product-specific.
- Open-PR comment reads paginate (`&page=N`) up to `MAX_PR_COMMENT_PAGES` (10) pages of `GITHUB_PAGE_LIMIT` comments;
  when the last page read is still full, the PR is marked `review_inspected: false` and the overlay reads `unknown`.

## Non-goals

- No change to ATeam, the merge gate (`scripts/merge-check.sh`), historical `<!-- semteams-review-* -->` marker parsing,
  reviewer-identity or `Reviewed at <sha>` parsing, the `WorkReviewBaton` wire shape, or the UI components.
- No per-lane breakdown on the overlay; it reports the single effective record.

## Impact

`ui/src/lib/server/work/githubSource.ts`, `ui/src/lib/server/work/githubSource.test.ts`, and the `observable-work`
spec. Some review records that previously read as `current` (decorated `APPROVE` values) now read `unknown` until a
gate-conformant `Verdict: APPROVE` record is posted; the merge gate already rejected those records.
