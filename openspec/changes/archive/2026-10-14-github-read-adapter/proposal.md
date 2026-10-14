# GitHub read adapter as a product-local tool under framework-alignment review

## Why

[Issue #273](https://github.com/C360Studio/semteams/issues/273) requires a read-only, product-local GitHub tool so the
`program-report` gather phase (#267) can observe issues, pull requests, releases, commits, and CI status across an
operator-configured portfolio (`SEMTEAMS_PORTFOLIO_PATH`) with cursors and stable identities, emitting channel-typed
evidence records (`channel: "github"`, `locator`, `origin_id`, `source_class: "primary"`, `retrieved_at`, `raw_body`)
for mechanical derivation (#271). Upstream removed its `github_*` tools (`semstreams` `a533306e`), strips `GITHUB_TOKEN`
from `bash`, and restricts `http_request` to unauthenticated fetches ([ADR-060](../../../docs/adr/060-program-manager-direction-and-research-design.md) §5).

## What changes

- Record the mandatory framework-alignment review in an [ADR-060](../../../docs/adr/060-program-manager-direction-and-research-design.md)
  addendum and register the migration posture in [`cmd/semteams/tools/README.md`](../../../cmd/semteams/tools/README.md).
- Add the shared `cmd/semteams/portfolio` reader/validator enforcing `configs/portfolio/portfolio.schema.json` and the
  uniqueness invariants shared with `ui/src/lib/server/work/portfolio.ts`.
- Add the shared channel-typed `EvidenceRecord` contract (`channel`, `locator`, `origin_id`, `source_class`,
  `retrieved_at`, `raw_body`, optional `quote` / `tool_result_ref` / `revision`, plus `NormalizeOriginID` and
  `Validate()`) in `cmd/semteams/research/evidence.go` so `#273` and `#271` share one schema.
- Implement the read-only `github_read` executor in `cmd/semteams/tools/githubread/` with:
  - portfolio repository allowlist enforcement (`repository`, `project`, or `program` scope);
  - support for `issues`, `pull_requests`, `releases`, `commits`, and `ci_status` activity kinds;
  - per-`(repository, kind)` cursor watermarks and content digests so a second run over unchanged repositories yields no
    new activity;
  - server-side token configuration (`SEMTEAMS_GITHUB_TOKEN` / `GITHUB_TOKEN` on `githubread.Config`), never exposed to
    `bash`;
  - registration under `agentic-tools` with its own tool category (`github`).
- Add unit and contract tests proving evidence-record validation, portfolio allowlist enforcement, and cursor
  suppression on unchanged repositories.

## Non-goals

- GitHub write operations of any kind (issue creation, PR creation/review/merge, comments, labels).
- Wiring the `program-report` rule pack or pulse synthesis journey (owned by #267).
- Full artifact claim-level derivation enforcement inside `emit_research_artifact` (owned by #271, which consumes the
  `research.EvidenceRecord` contract introduced here).
- Durable cross-cluster cursor synchronization (owned by #268).

## Acceptance and risks

- ADR-060 addendum documents the upstream survey at frozen `8b99efe9`, the ruled-out alternatives (`bash` + `gh`,
  `http_request`, reviving beta.113's unscoped `github_read`), and the migration posture.
- Contract tests in `test/contract/` verify that `github_read` results validate as `research.EvidenceRecord` values and
  that a second run over unchanged repositories yields zero new activity records.
- `cmd/semteams/tools/README.md` lists `github_read` with its ADR reference and migration target.
