# Design: GitHub Read Adapter (`github_read`)

## D1. Framework-Alignment Posture

- Upstream `semstreams` removed its builtin `github_*` executors in commit `a533306e` (`refactor(framework)!: enforce
  package composition boundaries`), classifying GitHub integrations as product-owned.
- `github_read` follows the `add_source_repo` pattern (`cmd/semteams/tools/README.md` step 4): an external-service tool
  whose scope is restricted by an operator-authored allowlist (`SEMTEAMS_PORTFOLIO_PATH`) rather than arbitrary
  LLM-supplied repositories.
- Registered on the shared `agentictools.ExecutorRegistry` from `cmd/semteams/product_tools.go` with tool category
  `agentictools.ToolCategory("github")` via `agentictools.RegisterToolCategory("github_read", "github")`.

## D2. Shared Evidence Record Contract (`cmd/semteams/research/evidence.go`)

- `research.EvidenceRecord` defines the channel-agnostic evidence shape shared between `#273` and `#271`:
  - `Channel`: `"github"` | `"web"` | `"semsource"`
  - `Locator`: original object locator (`github://<owner>/<repo>/issues/<n>`, `github://<owner>/<repo>/pulls/<n>`,
    `github://<owner>/<repo>/releases/<tag>`, `github://<owner>/<repo>/commits/<sha>`,
    `github://<owner>/<repo>/actions/runs/<id>`)
  - `OriginID`: canonical normalized locator (`NormalizeOriginID(channel, locator)` — lowercases scheme and
    `owner/repo` for `github`; lowercases scheme/host and strips tracking parameters and fragments for `web`)
  - `SourceClass`: `"primary"` | `"official"` | `"secondary"` | `"community"` (`"primary"` for GitHub API records)
  - `RetrievedAt`: UTC retrieval timestamp (`time.Time`)
  - `RawBody`: deterministic text/JSON representation of the retrieved object that `#271`'s emit executor can match
    cited `Quote` spans against without an LLM
  - Optional metadata/provenance fields: `Kind`, `Repository`, `HTMLURL`, `UpdatedAt`, `Quote`, `ToolResultRef`,
    `Revision`.

## D3. Portfolio Allowlist and Cursor Suppression (`cmd/semteams/tools/githubread`)

- `cmd/semteams/portfolio` parses and validates `SEMTEAMS_PORTFOLIO_PATH` against the rules in
  `configs/portfolio/portfolio.schema.json`.
- `github_read` accepts optional `program`, `project`, `repository`, `kinds` (`issues`, `pull_requests`, `releases`,
  `commits`, `ci_status`), `state` (`open`, `closed`, `all`), `limit`, and `include_unchanged`. Any request naming an
  unconfigured repository fails closed with `agentic.ToolErrorInvalidArgs`.
- A `CursorStore` (in-memory by default, or file-backed when `SEMTEAMS_GITHUB_CURSOR_PATH` is set) tracks per-`(repo,
  kind)` watermarks and per-`origin_id` version digests (`updated_at` + content SHA-256 prefix). On subsequent runs over
  unchanged repositories, already-observed object versions are suppressed (`records: []`, `unchanged_filtered > 0`),
  preventing duplicate reporting across ticks.
