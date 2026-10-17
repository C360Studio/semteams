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

### Requirement: Program Pulse execution via the research pack and `/pulse` operator command

SemTeams SHALL expose `/pulse` as a registered operator slash command and UI action chip alongside `/research`, rewriting `/pulse <request>` to `@pulse <request>` via `teamhint` and routing through the coordinator into the `research` pack (`decide(action="research")`) without introducing a duplicate `program-report` rule pack or role set. `configs/rules/research/02-plan-to-gather.json` SHALL include `github_read` in `researcher-research-gather`'s allowed `tools` list so gatherers can query configured portfolio repositories alongside `web_search` and `bash`.

#### Scenario: `/pulse` hint routes through dispatch and coordinator to the research pack

- **GIVEN** an operator submits `/pulse summarize portfolio activity across semteams and semengine` from the UI chat bar or dispatch API
- **WHEN** `teamhint` processes the command and the coordinator evaluates the `@pulse` hint
- **THEN** `teamhint` SHALL rewrite and republish the message as `@pulse summarize portfolio activity across semteams and semengine`
- **AND** the coordinator persona SHALL classify the portfolio pulse request as `decide(action="research")`, spawning `researcher-research-plan`

#### Scenario: Research gather rule permits `github_read`

- **GIVEN** `configs/rules/research/02-plan-to-gather.json` spawns `researcher-research-gather` for each planned subtopic
- **WHEN** the rule's `publish_agent` action is inspected or executed
- **THEN** its `tools` allowlist SHALL include `"github_read"` alongside `"read_loop_result"`, `"web_search"`, `"bash"`, `"scratchpad"`, and `"decide"`

### Requirement: Multi-project Program Pulse decomposition and evidence-backed synthesis

When an operator requests a Program Pulse or portfolio/project status synthesis across configured projects, `researcher-research-plan` SHALL decompose the inquiry into `1..4` project- or repository-scoped subtopics honoring the operator-authored portfolio mapping (`SEMTEAMS_PORTFOLIO_PATH`) without inferring unconfigured project membership. Each `researcher-research-gather` loop SHALL read configured GitHub repositories via `github_read`, and `researcher-research-synthesize` SHALL emit a `research.Artifact` whose factual `actors` and `integration_points` claims carry mechanically verified `channel: "github"` `EvidenceRecord` citations, while keeping attention recommendations (`tasks`) and incomplete-evidence disclosures (`open_gaps`) visibly separate.

#### Scenario: Multi-project Program Pulse journey passes mechanical derivation across configured projects

- **GIVEN** a portfolio configuration with at least two projects (`semteams` and `semengine` under program `c360`) and deterministic GitHub activity records served for each repository
- **WHEN** the `program-pulse.yaml` journey executes `researcher-research-plan` (`N=2` project subtopics) → `researcher-research-gather` (`×2` calling `github_read`) → `researcher-research-synthesize` (`emit_research_artifact`) → `reviewer-research` → `coordinator`
- **THEN** `emit_research_artifact` SHALL mechanically verify every `actors` and `integration_points` citation against the gather loops' `github_read` tool outputs (`channel: "github"`, `source_class: "primary"`, matching `locator`/`origin_id`, and verbatim `quote` from `raw_body`)
- **AND** the resulting artifact SHALL identify completed, active, waiting/at-risk work and labeled attention recommendations (`tasks`) across both configured projects without mutating any GitHub repository state

