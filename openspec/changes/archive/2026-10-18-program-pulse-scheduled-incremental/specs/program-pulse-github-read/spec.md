# program-pulse-github-read Specification Delta

## MODIFIED Requirements

### Requirement: Product-local read-only GitHub evidence adapter

SemTeams SHALL provide a read-only product-local `github_read` tool registered under `agentic-tools` with its own
`github` tool category and server-configured credentials (`SEMTEAMS_GITHUB_TOKEN` or `GITHUB_TOKEN`), never exposing the
token to `bash`. The tool SHALL restrict reads to repositories declared in the operator's portfolio configuration
(`SEMTEAMS_PORTFOLIO_PATH`) and SHALL return channel-typed evidence records (`channel: "github"`, stable `locator`,
canonical `origin_id`, `source_class: "primary"`, `retrieved_at`, and `raw_body`) covering issues, pull requests,
releases, commits, and CI status. The tool SHALL distinguish initial baseline onboarding (`baseline: true`) from
incremental delta reads (`baseline: false`), SHALL track `last_checked_at` separately from `last_persisted_at` so
no-change checks (`no_change: true`) do not overwrite `watermark` or `last_persisted_at`, SHALL stage cursor updates per
run until `emit_research_artifact` succeeds so failed runs or same-run retries are replay-safe, and SHALL isolate
per-repository/per-kind fetch failures into `failures` without aborting healthy repository reads or advancing failed
cursors.

#### Scenario: Configured portfolio repository emits valid evidence records

- **GIVEN** a valid portfolio configuration and a GitHub API source serving issues, pull requests, releases, commits,
  and CI workflow runs for a configured repository
- **WHEN** `github_read` is invoked for that repository or its containing project/program on an initial baseline run
- **THEN** the tool SHALL return structured `research.EvidenceRecord` entries with `channel: "github"`,
  `source_class: "primary"`, a stable `github://` locator, canonical `origin_id`, non-zero `retrieved_at`, and
  non-empty `raw_body`, with `baseline: true` and `no_change: false`
- **AND** every returned record SHALL pass `research.EvidenceRecord.Validate()`

#### Scenario: Unconfigured repository is rejected

- **GIVEN** a portfolio configuration that does not include `other/unconfigured`
- **WHEN** `github_read` is invoked with `repository: "other/unconfigured"`
- **THEN** the tool SHALL reject the call with `ToolErrorInvalidArgs`
- **AND** it SHALL NOT issue any GitHub API request for that repository

#### Scenario: Second run over unchanged repositories yields no new activity

- **GIVEN** `github_read` has already observed and committed a repository's baseline issues, pull requests, releases,
  commits, and CI status
- **WHEN** `github_read` is invoked a second time over the same unchanged repository state
- **THEN** the tool SHALL return zero new evidence records (`records` empty), `baseline: false`, and `no_change: true`,
  and SHALL advance `last_checked_at` while preserving the prior `watermark` and `last_persisted_at`
- **AND** if a single object updates on a subsequent run, only that updated object SHALL be emitted as a new evidence
  record with `no_change: false`

#### Scenario: Run-scoped cursor staging replays idempotently on retry and does not advance on failed runs

- **GIVEN** a `github_read` call executed within a research run (`agent.run_id` present on tool call metadata)
- **WHEN** `github_read` is retried within the same run before `emit_research_artifact` commits the run, or a subsequent
  run executes after the prior run failed before `emit_research_artifact`
- **THEN** both calls SHALL evaluate against the pre-run committed cursor and return the identical delta records without
  dropping activity
- **AND** once `emit_research_artifact` succeeds and commits the run cursor, a subsequent run over unchanged state SHALL
  observe the committed cursor and return zero new records (`no_change: true`)

#### Scenario: Partial repository failure returns healthy repository evidence and requires open_gaps disclosure

- **GIVEN** a multi-repository `github_read` call where one configured repository returns an HTTP or rate-limit failure
  and another configured repository succeeds
- **WHEN** `github_read` executes and `emit_research_artifact` verifies the synthesized Program Pulse artifact
- **THEN** `github_read` SHALL return the healthy repository's `EvidenceRecord` entries alongside a structured
  `failures` entry for the failed repository without advancing the failed repository's cursor
- **AND** `emit_research_artifact` (`VerifyDerivation`) SHALL reject the artifact with `ToolErrorInvalidArgs` unless
  every failed repository in the cited `github_read` result is explicitly named in `artifact.open_gaps`
