# Design: Program Pulse as a Portfolio Profile of the Research Pack (#267)

## Context

When [ADR-060](../../../docs/adr/060-program-manager-direction-and-research-design.md) was authored on 2026-09-03, SemTeams had no `/work` board; an LLM-driven `program-report` flow was envisioned as the primary way an operator would see completed, active, waiting, and at-risk work across repositories. Between 2026-10-08 and 2026-10-17, two major tracks converged on `main`:

1. **Deterministic Observable Work (`/work`):** `#296`, `#314`, `#315`, `#319`, `#320`, and `#324` shipped a live GitHub-backed work board (`ui/src/lib/server/work/githubSource.ts`, `baton.ts`) over `configs/portfolio-c360.json` that deterministically computes columns, Projects v2 status/priority, builder claim batons, PR review SHA freshness, and decision/hold gates (`status:needs-decision`, `status:blocked`, `class:flake`, OpenSpec `Hold:` caveats), plus an inline agentic drawer (`[Work item: ...]`).
2. **Verified Research Engine (`research`):** `#273` landed the portfolio-scoped `github_read` tool returning `channel: "github"` `EvidenceRecord` envelopes; `#271` landed mechanical citation derivation in `emit_research_artifact` (including native `evidenceRecordsEnvelope` verification for `github_read`); and `#272` landed per-spawn iteration budgets and the `1..4` planner fan-out ceiling.

## Decisions

### D1: Reuse the `research` pack topology; expose `/pulse` at the UX and command layer

- **Decision:** Keep the internal rule pack (`configs/rules/research/`), role identifiers (`researcher-research-plan`, `researcher-research-gather`, `researcher-research-synthesize`, `reviewer-research`), coordinator action token (`decide(action="research")`), and canonical 3-segment predicates (`research.plan.*`, `research.gather.*`, `research.artifact.*`) unchanged. Expose `/pulse` alongside `/research` in `cmd/semteams/product_commands.go`, `ui/src/lib/services/slashCommands.ts`, and `ui/src/lib/components/layout/ChatBar.svelte` (mirroring how `/optimize` fronts `autoresearch`).
- **Rationale:** The internal `research` identifiers have 291 references across 40+ rule, contract, E2E, and UI test files, and the 10-rule `for_each` fan-out + join graph in `configs/rules/research/` is already the exact topology ADR-060 §5 specified for Program Pulse. Adding `"github_read"` to `02-plan-to-gather.json` and registering `/pulse` as a `teamhint` command gives operators a first-class Program Pulse entrypoint with zero duplicate rule files or role proliferation.

### D2: Clear boundary between `/work` (structural state) and `/pulse` / `/research` (semantic synthesis)

- **Decision:** `/work` is the authoritative surface for deterministic structural portfolio state (issue columns, Projects v2 status, claim/review batons, and decision/hold gates). `/pulse` (and `/research` over portfolio projects) is used for semantic cross-repository synthesis, change/risk impact narratives, and scoped work-item investigations that require reading and correlating issue/PR/release/commit/CI bodies via `github_read`.
- **Decomposition & evidence contract:**
  - **Plan (`researcher-research-plan`):** For portfolio/program pulse asks, decomposes by configured portfolio project or repository into `1..4` subtopics (e.g. one subtopic per project in `configs/portfolio-c360.json`), never inferring unconfigured project membership.
  - **Gather (`researcher-research-gather`):** Calls `github_read` scoped by `program`, `project`, or `repository` and records `tool_result_ref`, `channel: "github"`, `locator`, `origin_id`, `source_class: "primary"`, and verbatim `quote` substrings from `records[].raw_body`.
  - **Synthesize (`researcher-research-synthesize`):** Emits a `research.Artifact` where observed project/repository facts live in `actors` and `integration_points` (every entry backed by mechanically verified `channel: "github"` evidence records), labeled attention recommendations live in `tasks` (citing the project/repo and underlying evidence), and incomplete or unavailable repository disclosures live in `open_gaps`.

### D3: Deterministic mock GitHub REST endpoints for contract and E2E verification

- **Decision:** Serve deterministic read-only GitHub REST v3 endpoints (`/repos/{owner}/{repo}/issues`, `/pulls`, `/releases`, `/commits`, `/actions/runs`) directly from `test/e2e/mock/openai_server.go` and point `SEMTEAMS_GITHUB_API_URL=http://mock-llm:8080` and `SEMTEAMS_PORTFOLIO_PATH=/app/configs/portfolio-c360.json` in `ui/docker-compose.agentic-e2e.yml`.
- **Rationale:** `mock-llm` is already a required healthy dependency of `backend` in `ui/docker-compose.agentic-e2e.yml`. Serving deterministic repository fixtures from `mock-llm` requires no extra container, keeps E2E and contract tests offline and zero-cost, and exercises the real `githubread.HTTPClient` wire path end-to-end.
