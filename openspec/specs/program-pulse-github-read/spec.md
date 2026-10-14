# program-pulse-github-read Specification

## Purpose
TBD - created by archiving change github-read-adapter. Update Purpose after archive.
## Requirements
### Requirement: Product-local read-only GitHub evidence adapter

SemTeams SHALL provide a read-only product-local `github_read` tool registered under `agentic-tools` with its own
`github` tool category and server-configured credentials (`SEMTEAMS_GITHUB_TOKEN` or `GITHUB_TOKEN`), never exposing the
token to `bash`. The tool SHALL restrict reads to repositories declared in the operator's portfolio configuration
(`SEMTEAMS_PORTFOLIO_PATH`) and SHALL return channel-typed evidence records (`channel: "github"`, stable `locator`,
canonical `origin_id`, `source_class: "primary"`, `retrieved_at`, and `raw_body`) covering issues, pull requests,
releases, commits, and CI status.

#### Scenario: Configured portfolio repository emits valid evidence records

- **GIVEN** a valid portfolio configuration and a GitHub API source serving issues, pull requests, releases, commits,
  and CI workflow runs for a configured repository
- **WHEN** `github_read` is invoked for that repository or its containing project/program
- **THEN** the tool SHALL return structured `research.EvidenceRecord` entries with `channel: "github"`,
  `source_class: "primary"`, a stable `github://` locator, canonical `origin_id`, non-zero `retrieved_at`, and
  non-empty `raw_body`
- **AND** every returned record SHALL pass `research.EvidenceRecord.Validate()`

#### Scenario: Unconfigured repository is rejected

- **GIVEN** a portfolio configuration that does not include `other/unconfigured`
- **WHEN** `github_read` is invoked with `repository: "other/unconfigured"`
- **THEN** the tool SHALL reject the call with `ToolErrorInvalidArgs`
- **AND** it SHALL NOT issue any GitHub API request for that repository

#### Scenario: Second run over unchanged repositories yields no new activity

- **GIVEN** `github_read` has already observed a repository's issues, pull requests, releases, commits, and CI status
- **WHEN** `github_read` is invoked a second time over the same unchanged repository state
- **THEN** the tool SHALL return zero new evidence records (`records` empty) and report the filtered unchanged count and
  active cursors
- **AND** if a single object updates on a subsequent run, only that updated object SHALL be emitted as a new evidence
  record

