## MODIFIED Requirements

### Requirement: Structured builder claim, review parity, and decision/hold gate overlays

SemTeams SHALL derive structured coordination overlays on each work item from repository-protocol records without
introducing a separate agent message bus or mutating GitHub state:
- **Builder claim baton (`claim`):** SHALL identify an active claiming pull request (`agent-pr` when the branch prefix
  is `claude/`, `codex/`, or `gemini/` or `implemented-by` names an agent persona; `human-pr` otherwise) or issue
  assignee (`assignee`), including the pull request reference, draft status, `implemented-by` attribution, and declared
  stop-point when present.
- **Review baton (`review`):** SHALL compare the active pull request's `HEAD` commit SHA against its latest structured
  `Review request` and its effective `Review record` — accepting both ATeam's canonical `reviewed-by:` (or an
  `Independent Cross-Agent Review` markdown heading naming a single agent vendor `claude`, `codex`, or `gemini`) +
  `(Re-)Reviewed at <sha>` + `Verdict:` PR comment records (outside CommonMark `0..3`-space fenced code blocks and `>`
  blockquotes) and historical `<!-- semteams-review-request -->` / `<!-- semteams-review-record -->` markers — and
  report `not_requested`, `requested`, `current` (when the effective `Review record` passes at `HEAD`), `stale` (when
  commits landed after the latest `Review record` without a newer request at `HEAD`), or `changes_requested` (when the
  effective `Review record` at `HEAD` reports blocking findings or `REQUEST CHANGES`), along with `reviewed-by` and
  reviewer-lane attribution. Matching ATeam's merge gate (`C360Studio/ateam` `internal/review/gate.go` `gateVerdict()`,
  `36b999b`), an ATeam record SHALL map to `pass` only when it carries exactly one `Verdict:` line outside code fences
  and that line, trimmed, is `Verdict: APPROVE` (case-insensitive); a value led by `REQUEST CHANGES`,
  `CHANGES REQUESTED`, `BLOCKING`, or `FAIL` SHALL map to `blocking`; any other value — including conditional or
  decorated approvals such as `APPROVE pending H1`, `APPROVE, but REQUEST CHANGES on H1`, or `APPROVE with nits` — and
  more than one `Verdict:` line SHALL leave the comment unparsed. Historical `<!-- semteams-review-record -->` marker
  records keep their own `verdict:` field mapping (`pass` or `approved` to `pass`; `blocking`, `changes_requested`, or
  `fail` to `blocking`). The effective `Review record` SHALL be derived by replaying every valid record in comment
  order at any SHA (matching ATeam `record()`, which keeps the latest valid record whatever commit it names), per
  reviewer lane (`<name>-reviewer` tokens of the lane line; any other lane text is one opaque lane; a record without a
  lane line is an all-lanes record that resets every lane): each lane keeps its latest record. If any surviving lane
  record is blocking, the newest surviving blocking record SHALL be effective; otherwise the newest surviving record not
  at `HEAD` SHALL be effective (so the overlay reports `stale`); otherwise the latest record is. A blocking all-lanes
  record therefore remains effective until a newer all-lanes record supersedes it, even after every named lane
  approves. A comment carrying `<!-- semteams-review-record -->` or unquoted ATeam review-record signals that fails to
  parse unambiguously SHALL set `review_unparsed` unless surviving lane state newer than it covers it: every lane it
  names; or, when it names no lane but carries a reviewer identity (`reviewed-by:` or an `Independent Cross-Agent
  Review` heading), a newer all-lanes record; or, when it carries neither (for example an implementer reply), any newer
  surviving record. Open pull request comments SHALL be read sequentially page by page (`page=N`) up to
  `MAX_PR_COMMENT_PAGES`, so one pull request costs at most `MAX_PR_COMMENT_PAGES` x `READ_TIMEOUT_MS`; page-number
  pagination is not snapshot-consistent, so a comment deleted between page reads can cause one comment to be skipped
  (comments added between reads land on later pages and are still read). When the last page read is full, or when
  `review_unparsed` is set, the source SHALL mark `review_inspected: false` (`lookup: "partial"`) and render the
  `review` overlay as `unknown` rather than retaining an earlier `current` or `known` state.
- **Decision and hold gate (`gate`):** SHALL surface `status:needs-decision` owner-docket labels, `status:blocked`
  labels and `blocked-by` dependencies, repository-wide open `class:flake` merge holds on active pull requests, and
  unticked OpenSpec `tasks.md` caveats (`HALT`, `RED`, `BLOCKED`, `WONTDO`, `OPEN-Q`).

None of `claim`, `review`, or `gate` SHALL alter the work item's board column (`column`) or Project status (`pm_status`).

#### Scenario: Agent draft pull request claim with declared stop-point

- **GIVEN** a work item linked to an open draft pull request on branch `claude/gh312-mqtt-nats-research` declaring a
  stop-point
- **WHEN** the work item renders on the board, table, and overview panel
- **THEN** the builder claim baton SHALL render as `known` with vendor `claude`, the pull request reference, draft
  status, and the declared stop-point
- **AND** the work item's column SHALL remain determined solely by its GitHub Project status or GitHub field fallback

#### Scenario: Stale cross-agent review record after follow-up commit

- **GIVEN** a work item whose active pull request has `head_sha` `a1b2c3d` and whose latest `Review record` comment was
  recorded at commit `9988776`
- **WHEN** the work item renders on the board, table, and overview panel
- **THEN** the review baton SHALL render as `known` with status `stale`
- **AND** the overview panel SHALL display both the `HEAD` SHA and the stale `Review record` SHA

#### Scenario: ATeam upstream review record and historical SemTeams marker compatibility

- **GIVEN** a pull request carrying an ATeam review record comment with vendor-identifying `reviewed-by: codex (gpt-6-astra, high)`
  (or an `Independent Cross-Agent Review` heading naming `<vendor>`), `(Re-)Reviewed at <sha>`, and `Verdict: APPROVE` or
  `Verdict: REQUEST CHANGES` outside `0..3`-space code fences and `>` blockquotes
- **WHEN** `parseReviewComments` inspects the pull request's comments alongside historical `<!-- semteams-review-* -->`
  comments
- **THEN** ATeam's `APPROVE` and `REQUEST CHANGES` verdicts SHALL map to `pass` and `blocking` respectively at the
  single designated `<sha>` (ignoring fenced examples and quoted replies, and rejecting vendor-less headings,
  non-`Reviewed at` SHA lines, or ambiguous multi-commit comments)
- **AND** valid historical `<!-- semteams-review-request -->` and `<!-- semteams-review-record -->` comments outside
  code and blockquotes SHALL continue to parse to the same result

#### Scenario: Unparseable review record fails closed to unknown

- **GIVEN** an open pull request whose comments include a valid `APPROVE` record at `HEAD` followed by a comment
  carrying `<!-- semteams-review-record -->` or unquoted ATeam review-record signals whose reviewer identity,
  `Reviewed at <sha>`, or `Verdict:` cannot be parsed unambiguously
- **WHEN** the GitHub work source enriches the pull request and derives baton overlays
- **THEN** `parseReviewComments` SHALL set `review_unparsed: true`
- **AND** the work item lookup SHALL report `partial` with `review_inspected: false` and render the `review` overlay as
  `unknown` rather than `current`

#### Scenario: Later valid review record clears an earlier unparseable comment

- **GIVEN** an open pull request whose comments include an unparseable record-shaped comment (for example an
  implementer reply quoting a verdict and commit SHA) followed by a valid `Review record` at `HEAD`
- **WHEN** `parseReviewComments` inspects the comments
- **THEN** it SHALL NOT set `review_unparsed` when the earlier comment carries no reviewer identity and no lane, or
  names only lanes that newer surviving records cover
- **AND** it SHALL keep `review_unparsed: true` when the earlier comment names a reviewer lane (for example
  `svelte-reviewer`) that no newer record covers, or carries a reviewer identity but no lane and no newer all-lanes
  record follows it

#### Scenario: Conditional APPROVE verdict does not pass

- **GIVEN** an ATeam review record at `HEAD` whose only `Verdict:` line is `Verdict: APPROVE pending H1 fix`,
  `Verdict: APPROVE, but REQUEST CHANGES on H1`, or another value that is not exactly `APPROVE`
- **WHEN** the GitHub work source derives the `review` overlay
- **THEN** the record SHALL NOT map to `pass` and the `review` overlay SHALL render as `unknown`, not `current`

#### Scenario: Blocking reviewer lane survives another lane's approval

- **GIVEN** an open pull request with a `svelte-reviewer` `REQUEST CHANGES` record followed by a `go-reviewer`
  `APPROVE` record, both at `HEAD`
- **WHEN** the GitHub work source derives the `review` overlay
- **THEN** the `review` overlay SHALL render `changes_requested` with reviewer lane `svelte-reviewer`
- **AND** a later `svelte-reviewer` `APPROVE` or all-lanes `APPROVE` record at `HEAD` SHALL make it `current`
- **AND** a later record for the same lane (or an all-lanes record) at an older SHA SHALL make it `stale` or
  `changes_requested`, never `current`
- **AND** an all-lanes `REQUEST CHANGES` record SHALL stay `changes_requested` after both `go-reviewer` and
  `svelte-reviewer` approve at `HEAD`, until a newer all-lanes record supersedes it

#### Scenario: Review comments span more than one page

- **GIVEN** an open pull request whose first comment page holds `GITHUB_PAGE_LIMIT` comments including an `APPROVE` at
  `HEAD` and whose second page holds a `REQUEST CHANGES` at `HEAD` for the same lane
- **WHEN** the GitHub work source enriches the pull request
- **THEN** it SHALL read the second page and render the `review` overlay as `changes_requested`
- **AND** when every page up to `MAX_PR_COMMENT_PAGES` is full it SHALL mark `review_inspected: false` and render the
  `review` overlay as `unknown`

#### Scenario: Decision docket and OpenSpec hold gates

- **GIVEN** a work item carrying `status:needs-decision` or linked to an OpenSpec change with an unticked `Hold:` task
  line
- **WHEN** the work item renders on the board, table, and overview panel
- **THEN** the gate overlay SHALL render as `known` with the corresponding `needs-decision` or `openspec-hold` entry and
  reference
