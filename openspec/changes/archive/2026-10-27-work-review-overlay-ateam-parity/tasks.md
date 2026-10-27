## 1. Review verdict and unparsed-record parity with ATeam (#365)

- [x] 1.1 Map a `Review record` to `pass` only on exactly one unfenced `Verdict: APPROVE` line read from the original comment body with ATeam's `unfencedLines()` fence rule and Go `strings.TrimSpace` trimming (ATeam `gateVerdict()`), leaving conditional, inline-code-formatted, or decorated approvals unparsed, in `ui/src/lib/server/work/githubSource.ts` (see `notes/1.1.md`).
- [x] 1.2 Clear an earlier unparseable comment when a later valid `Review record` covering its lane follows it (ATeam `record()`), keeping an unparseable comment after the latest valid record fail-closed.

## 2. Reviewer-lane aggregation and paginated comment reads (#366)

- [x] 2.1 Replay every valid `Review record` per reviewer lane at any SHA in `parseReviewComments(comments, headSha?)` so a blocking lane survives another lane's `APPROVE` until a newer same-lane or all-lanes record supersedes it, and a later record at an older SHA is never skipped.
- [x] 2.2 Paginate open-PR comment reads up to `MAX_PR_COMMENT_PAGES`, failing closed to `review_inspected: false` when the last page read is full (see `notes/1.1.md`).

## 3. Tests, spec, and verification

- [x] 3.1 Add Vitest coverage in `ui/src/lib/server/work/githubSource.test.ts` for each rule above, each shown able to fail by a temporary mutation with a SHA-256-verified restore.
- [x] 3.2 Update the `observable-work` spec delta to match the code and pass `task spec:check` and `task validate` on a clean tree.
