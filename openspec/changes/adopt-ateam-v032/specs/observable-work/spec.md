## MODIFIED Requirements

### Requirement: Structured builder claim, review parity, and decision/hold gate overlays

SemTeams SHALL derive structured coordination overlays on each work item from repository-protocol records without
introducing a separate agent message bus or mutating GitHub state:
- **Builder claim baton (`claim`):** SHALL identify an active claiming pull request (`agent-pr` when the branch prefix
  is `claude/`, `codex/`, or `gemini/` or `implemented-by` names an agent persona; `human-pr` otherwise) or issue
  assignee (`assignee`), including the pull request reference, draft status, `implemented-by` attribution, and declared
  stop-point when present.
- **Review baton (`review`):** SHALL compare the active pull request's `HEAD` commit SHA against its latest structured
  `Review request` and `Review record` comments — accepting both ATeam's canonical `reviewed-by:` / `Reviewed at <sha>`
  / `Verdict: APPROVE | REQUEST CHANGES` (and `### Independent Cross-Agent Review`) PR comment records and historical
  `<!-- semteams-review-request -->` / `<!-- semteams-review-record -->` markers — and report `not_requested`,
  `requested`, `current` (when the latest `Review record` passes at `HEAD`), `stale` (when commits landed after the
  latest `Review record` without a newer request at `HEAD`), or `changes_requested` (when the latest `Review record` at
  `HEAD` reports blocking findings or `REQUEST CHANGES`), along with `reviewed-by` and reviewer-lane attribution.
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

- **GIVEN** a pull request carrying an ATeam review record comment with `reviewed-by: codex (gpt-6-astra, high)`,
  `Reviewed at <sha>`, and `Verdict: APPROVE` or `Verdict: REQUEST CHANGES` outside code fences
- **WHEN** `parseReviewComments` inspects the pull request's comments alongside historical `<!-- semteams-review-* -->`
  comments
- **THEN** ATeam's `APPROVE` and `REQUEST CHANGES` verdicts SHALL map to `pass` and `blocking` respectively at the
  single designated `<sha>` (ignoring fenced examples and rejecting ambiguous multi-commit comments)
- **AND** historical `<!-- semteams-review-request -->` and `<!-- semteams-review-record -->` comments SHALL continue to
  parse identically

#### Scenario: Decision docket and OpenSpec hold gates

- **GIVEN** a work item carrying `status:needs-decision` or linked to an OpenSpec change with an unticked `Hold:` task
  line
- **WHEN** the work item renders on the board, table, and overview panel
- **THEN** the gate overlay SHALL render as `known` with the corresponding `needs-decision` or `openspec-hold` entry and
  reference
