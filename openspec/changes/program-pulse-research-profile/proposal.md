# Change: Fold Program Pulse (#267) into the research pack with `github_read` and `/pulse`

## Why

[Issue #267](https://github.com/C360Studio/semteams/issues/267) is the final item in milestone `v0.2.0` (`Program Pulse walking skeleton`, epic [#269](https://github.com/C360Studio/semteams/issues/269)). All four engineering prerequisites are now complete on `main`:
- [#280](https://github.com/C360Studio/semteams/issues/280) / [#270](https://github.com/C360Studio/semteams/issues/270) — frozen SemStreams `8b99efe` lifecycle baseline.
- [#273](https://github.com/C360Studio/semteams/issues/273) — portfolio-scoped `github_read` tool (`cmd/semteams/tools/githubread/`) and `configs/portfolio-c360.json`.
- [#271](https://github.com/C360Studio/semteams/issues/271) — channel-typed `EvidenceRecord` contract and mechanical derivation check in `emit_research_artifact`.
- [#272](https://github.com/C360Studio/semteams/issues/272) — research containment budget (per-spawn loop iteration caps, `1..4` planner fan-out ceiling, cron cooldown/inflight guard, and Pattern-B kill switch).

Meanwhile, the live GitHub-backed Observable Work Board (`/work`, `#296`, `#314`, `#315`, `#319`, `#320`, `#324`) now deterministically computes and renders structural portfolio state (Kanban columns, Projects v2 status/priority, builder claim batons, review SHA freshness, and `status:needs-decision` / `status:blocked` / `class:flake` gates). Building a separate `configs/rules/program-report/` category pack would duplicate the 10-rule `research` pack (`plan → gather (1..4) → synthesize → review → coordinator`) solely to add `github_read` to the gather tool allowlist.

Instead, per [ADR-060 §5](../../../docs/adr/060-program-manager-direction-and-research-design.md) ("`program-report` is built as the first depth profile of the research capability, not beside it") and the owner's 2026-10-17 roadmap decision:
1. `/work` owns deterministic structural portfolio state.
2. The `research` pack (equipped with `github_read` and exposed via both `/pulse` and `/research` operator commands, matching how `/optimize` fronts `autoresearch`) owns on-demand Program Pulse synthesis, cross-repository change/risk analysis, and work-item deep-dives.
3. Scheduled cron execution and cross-restart cursor synchronization ([#268](https://github.com/C360Studio/semteams/issues/268)) are deferred to `horizon:post-mvp`.

## What Changes

- **Operator-facing `/pulse` command and UI action chip:**
  - Register `/pulse` in `cmd/semteams/product_commands.go` (`teamhint.New("pulse", "pulse", ...)`) and in `ui/src/lib/services/slashCommands.ts` + `ui/src/lib/components/layout/ChatBar.svelte` alongside `/research` and `/optimize`, routing `@pulse` hints through the coordinator into `decide(action="research")`.
- **Wire `github_read` into the `research` pack and personas:**
  - Add `"github_read"` to `configs/rules/research/02-plan-to-gather.json`'s `tools` allowlist for `researcher-research-gather`.
  - Update `coordinator`, `researcher-research-plan`, `researcher-research-gather`, and `researcher-research-synthesize` persona fragments so portfolio/program pulse and `[Work item: ...]` inquiries decompose across configured portfolio projects/repositories (`1..4` subtopics), gather normalized GitHub activity via `github_read` (`channel: "github"`, `source_class: "primary"`), and separate observed facts (`actors` / `integration_points` with mechanically verified `channel: "github"` citations), labeled attention recommendations (`tasks`), and incomplete/unavailable-repo disclosures (`open_gaps`).
- **Deterministic mock GitHub REST endpoints and `program-pulse.yaml` journey:**
  - Extend `test/e2e/mock/openai_server.go` with read-only mock GitHub REST v3 endpoints (`/repos/{owner}/{repo}/issues`, `/pulls`, `/releases`, `/commits`, `/actions/runs`) for the configured C360 portfolio (`C360Studio/semteams`, `C360Studio/semengine`, `C360Studio/semstreams`).
  - Wire `SEMTEAMS_PORTFOLIO_PATH=/app/configs/portfolio-c360.json` and `SEMTEAMS_GITHUB_API_URL=http://mock-llm:8080` in `ui/docker-compose.agentic-e2e.yml`.
  - Add `test/fixtures/journeys/program-pulse.yaml` (and extend `test/contract/journey_emit_args_contract_test.go` to execute and resolve `github_read` calls) proving a multi-project (`semteams` + `semengine`, `N=2` fan-out) Program Pulse end-to-end with verified `channel: "github"` derivation.
- **Product and ADR documentation alignment:**
  - Record an ADR-060 addendum and update `docs/product/program-manager.md`, `AGENTS.md`, and `CLAUDE.md` to reflect `/work` (deterministic structural state) + `research` with `github_read` and `/pulse` (semantic Program Pulse synthesis).

## Impact

- Affected specs: `program-pulse-github-read`
- Affected code and configs:
  - `cmd/semteams/product_commands.go`
  - `configs/rules/research/02-plan-to-gather.json`, `configs/rules/research/README.md`
  - `configs/personas/fragments/coordinator/10-decision-contract.md`, `20-delegation-rules.md`
  - `configs/personas/fragments/researcher-research-plan/00-identity.md`, `10-plan-rules.md`
  - `configs/personas/fragments/researcher-research-gather/00-identity.md`, `20-iteration-rules.md`
  - `configs/personas/fragments/researcher-research-synthesize/00-identity.md`, `10-output-contract.md`
  - `ui/src/lib/services/slashCommands.ts`, `ui/src/lib/services/slashCommands.test.ts`
  - `ui/src/lib/components/layout/ChatBar.svelte`, `ui/src/lib/components/layout/ChatBar.test.ts`
  - `ui/docker-compose.agentic-e2e.yml`
  - `test/e2e/mock/openai_server.go`, `test/e2e/mock/openai_server_test.go`
  - `test/fixtures/journeys/program-pulse.yaml`
  - `test/contract/journey_emit_args_contract_test.go`, `test/contract/research_rule_pack_test.go`
  - `docs/adr/060-program-manager-direction-and-research-design.md`, `docs/product/program-manager.md`, `AGENTS.md`, `CLAUDE.md`
- Issues: Closes #267, Closes #269
