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

- An ATeam `Review record` maps to `pass` only when its original comment body (not the inline-code-filtered text
  used for identity and SHA recognition) carries exactly one `Verdict:` line outside code fences and that line,
  trimmed of Go `strings.TrimSpace` whitespace, is `Verdict: APPROVE` (case-insensitive), matching `gateVerdict()`.
  Conditional, decorated, or multi-line approvals (`APPROVE pending H1`, ``APPROVE `pending verdict tests` ``,
  `APPROVE with nits`, `**Verdict**: APPROVE`, `` Verdict: `APPROVE` ``, `APPROVED`, a second `Verdict:` line indented
  with U+00A0) do not pass; they leave the comment unparsed so the overlay fails closed to `unknown`. Leading
  `REQUEST CHANGES` / `CHANGES REQUESTED` / `BLOCKING` / `FAIL` values still map to `blocking`. Historical
  `<!-- semteams-review-record -->` marker records keep their `verdict:` mapping (`pass` / `approved` to `pass`).
- `review_unparsed` is no longer sticky: a later valid `Review record` clears an earlier unparseable comment, matching
  `record()`. An unparseable comment that names a reviewer lane is cleared only by newer surviving records covering that
  lane; one that carries a reviewer identity but no lane is cleared only by a newer all-lanes record; one with neither
  (an implementer reply) is cleared by any newer record. One posted after the latest valid record still fails closed.
- `parseReviewComments(comments, headSha?)` replays every valid record in comment order at any SHA, per reviewer lane
  (a record without a lane line resets every lane). The newest surviving blocking record is effective; else the newest
  surviving record not at `HEAD` (stale); else the latest record. A blocking lane therefore stays effective until a newer
  record for that lane or an all-lanes record supersedes it, and a later record at an older SHA is never skipped. ATeam has no counterpart; it keys on one
  `reviewed-by:` reviewer, while SemTeams's `go-reviewer` / `svelte-reviewer` lanes are product-specific.
- Open-PR comment reads paginate (`&page=N`) up to `MAX_PR_COMMENT_PAGES` (10) pages of `GITHUB_PAGE_LIMIT` comments;
  when the last page read is still full, the PR is marked `review_inspected: false` and the overlay reads `unknown`.
  Reads are sequential (worst case `MAX_PR_COMMENT_PAGES` x `READ_TIMEOUT_MS` per PR), and page-number pagination can
  skip one comment if a comment is deleted between page reads.

## Non-goals

- No change to ATeam, the merge gate (`scripts/merge-check.sh`), historical `<!-- semteams-review-* -->` marker parsing,
  reviewer-identity or `Reviewed at <sha>` parsing, the `WorkReviewBaton` wire shape, or the UI components.
- No per-lane breakdown on the overlay; it reports the single effective record.

## Impact

`ui/src/lib/server/work/githubSource.ts`, `ui/src/lib/server/work/githubSource.test.ts`, and the `observable-work`
spec. Some review records that previously read as `current` (decorated `APPROVE` values) now read `unknown` until a
gate-conformant `Verdict: APPROVE` record is posted; the merge gate already rejected those records.
