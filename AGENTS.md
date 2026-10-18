# AGENTS.md

This file provides guidance to Codex (codex.ai) and Antigravity (Gemini) when working with code in this repository. Content is intentionally kept in sync with `CLAUDE.md` (`cp CLAUDE.md AGENTS.md` with the agent-specific opener) and kept under 24,000 bytes so rule loaders with a 24 KB per-file cap never truncate it. Role contracts, model/effort routing, and shared skills live in [`.agents/README.md`](.agents/README.md); the full shared work protocol lives in [`.agents/protocol.md`](.agents/protocol.md).

# SemTeams Project Context

SemTeams is an **always-on program manager for a configurable portfolio**. The current runtime composes agentic,
graph, rule and I/O components from the frozen [SemStreams](https://github.com/c360studio/semstreams) dependency.
SemTeams owns the product wiring, category packs, shared persona corpus, product tools, Svelte UI and documentation.

[ADR-061](docs/adr/061-agent-runtime-ownership-and-observable-work.md) records the approved next boundary: SemEngine
owns the shared substrate and generic durability; SemTeams owns the agent runtime and absorbs SemDev as a separately
qualified development pack. Extraction, SemEngine adoption and pack activation are future changes. Do not mistake
the ownership decision for implemented runtime migration.

The owner-approved product direction lives in [`docs/product/program-manager.md`](docs/product/program-manager.md).
Research, planning, and design are capabilities used in support of programs and projects. Deterministic structural
portfolio visibility lives on `/work` (`#309`), while the read-only, evidence-backed **Program Pulse** (`#267`) runs
as an operator profile (`/pulse`) of the `research` pack using `github_read` (`#273`) under the `1..4` fan-out and loop
containment budget (`#272`). Scheduled cadence and custom work classification (`#268`) are deferred to
`horizon:post-mvp`. The long-term topology is one program manager coordinating durable project managers; the MVP proves
that view through one program-manager journey with project drill-down.

**Current implementation:** every processor comes from the frozen `github.com/c360studio/semstreams` dependency.
`cmd/semteams/` independently wires the primitives per ADR-029; `internal/runtimecatalog` explicitly registers the
complete frozen component/payload surface for runtime and generators. Registration does not activate a pack.

**Target ownership:** agent loop, dispatch, model/tool orchestration, context, agent-specific approval/governance,
trajectories and agent rule extensions belong in the SemTeams runtime. Generic graph, transport, mutation, rule,
lifecycle/projection and durable-execution mechanics belong in SemEngine. Its generic attempt/effect/replay work
([SemEngine #24](https://github.com/C360Studio/semengine/issues/24)) is planned, not a shipped dependency guarantee.
Preserve existing recovery while the integration contract is resolved; do not create a competing journal or engine.

## Substrate-plus-overlays architecture (ADR-042; demo scope reset by ADR-058)

The product shell runs **one** flow config — `configs/flow-bootstrap.json` — wiring substrate singletons
(`graph-ingest`, `graph-query`, `rule-processor`, `agentic-loop`, `agentic-dispatch`, `agentic-tools`, `agentic-model`).
Task classes are added as **category-keyed rule packs** + named persona bundles loaded by the substrate, not separate
flow configs ([ADR-042](docs/adr/042-coordinator-instantiated-flows-via-templates.md) §Phase 2 redesign).

Live task categories ([ADR-058](docs/adr/058-beta159-realignment-and-demo-lane-focus.md)):

- **`research`** (`/pulse` and `/research`) — coordinator routes both portfolio/project Program Pulse asks (`/pulse`)
  and open-ended investigation asks (`/research`) via `decide(action="research")`. `configs/rules/research/` drives
  `researcher-research-plan → researcher-research-gather (×1..4 fan-out with github_read + web_search + bash) →
  researcher-research-synthesize → reviewer-research → coordinator wake-up`.
- **`autoresearch`** (`/optimize`) — metric optimization with empirical keep/revert in an attested devcontainer sandbox
  (`configs/rules/autoresearch/`).

**Parked donor material (ADR-058):** the dev-side packs (`create-change`, `proof-readiness`, `dev-from-task`,
`dev-via-test`) are on disk but UNWIRED — they predate the upstream canonical predicate contract (3-segment
lower-kebab, fail-closed at persistence, NO alias mode). Their contract tests carry a `parked_packs` build tag; their
journeys are `describe.skip`; the coordinator taxonomy is `research | autoresearch | respond_direct | ask_user`.
ADR-061 authorizes future SemDev absorption as a separately qualified pack preserving
`issue → OpenSpec → implementation → verification → PR`, harness-owned outcomes and clean-room verification. Do not
rewire parked packs as a shortcut (`test/contract/predicate_contract_test.go` enforces the CI fence).

Adding a new prompt class (e.g. `program-report` or `project-plan`) is a **new category pack**: rule files under
`configs/rules/<category>/`, persona bundles under `configs/personas/fragments/<role>-<category>-<phase?>/`, and a
coordinator-persona `decide(action=<category>)` token. Adding a category does not justify a new runtime component.
**Shared persona corpus (`configs/personas/fragments/coordinator/`) stays domain-neutral**; domain flavor lives in
category packs.

## Shared work protocol (Claude, Codex, and Gemini)

State that all agents must see lives in the repository's tools, never in private memory or a separate status file.
The full protocol is [`.agents/protocol.md`](.agents/protocol.md); role contracts and model/effort routing live in
[`.agents/README.md`](.agents/README.md).

SemTeams routes design and implementation through its project roles: `architect` owns API and integration contracts;
`go-developer` implements backend work and `go-reviewer` reviews it; `svelte-developer` implements UI work and
`svelte-reviewer` reviews it; cross-stack changes use both reviewer lanes; and `technical-writer` owns durable docs and
conservative OpenSpec task truth.

| Question | Home | Rule |
|---|---|---|
| What is wanted / decided | GitHub issue + labels (`type:`, `area:`, `class:`, `status:`, `horizon:`) | `status:needs-decision` is the owner's docket (ruling comment removes label). `status:blocked` names its blocker. |
| What gates the next tag | GitHub milestone named for the intended version | Membership is the gate: in or out. `horizon:pre-v1` means before v1.0.0, not before the next tag. |
| An epic | Tracking issue labeled `type:epic` with a `#n` task list | GitHub renders progress; no separate epic document. |
| Who has claimed what | **Draft PR** opened at work start with `Closes #n`; branch prefix names the agent (`claude/...`, `codex/...`, `gemini/...`) | No draft PR, no claim. Design claims commit the OpenSpec proposal first. Record the current stop-point in the PR body. |
| Target state & task truth | OpenSpec change inside that PR; `task openspec:queue` reads `Hold:` lines | Archive (`openspec archive <id>` + spec sync) as the final content commit. Never assert post-merge facts ("CI green"). |
| Why | ADR or owner ruling comment on the issue | ADRs record durable decisions, not status or checklists. |

Rituals:

- **Start:** query open issues, draft PR claims, `task openspec:queue`, recent `main` runs, and `status:needs-decision`
  issues before choosing work.
- **Clarify:** before claiming work, mutating state, or spawning role subagents, read any named issue, PR, or file
  first, then confirm four boundaries: **Deliverable**, **Scope/non-goals**, **Definition of Done/verification**, and
  **Stop boundary**. When ambiguous, propose a 3–4 bullet Definition of Done and ask 1–3 targeted questions; once
  confirmed, run autonomously to that stop boundary.
- **Take work:** unclaimed issue → dedicated worktree on an agent-prefixed branch (`claude/<topic>`, `codex/<topic>`,
  `gemini/<topic>`; CLI uses `../semteams-wt/<branch>`, Enjoy desktop uses `move_to_worktree` under
  `.worktrees/<branch>`) → push → draft PR with `Closes #n` → implementation. Primary checkout is discovery-only.
  Verify worktree path and branch before every edit, commit, push, or PR mutation. Update branches by merging
  `origin/main`, never by rebase or force-push.
- **Land:** undraft → appropriate reviewer lane (`go-reviewer`, `svelte-reviewer`, or both) + cross-agent review round
  when requested (`Review request` / `Review record` PR comments per [`.agents/protocol.md`](.agents/protocol.md)) →
  all local and hosted gates green (`Repository CI` / `CI Status Check`) with no open unfixed `class:flake` → squash
  merge. Include `implemented-by: <agent> (<persona/model>)` (and `reviewed-by:` when reviewed) in the PR body
  (`claude`, `codex` / `Sol`, or `gemini`).
- **Close:** no issue closes without the owner's explicit `CONFIRM-CLOSE` or the squash merge of an approved PR that
  declared `Closes #n`.

## Tech Stack & Repository Layout

- **Go 1.26.3** — `cmd/semteams/` (~600 LoC across `main.go`, `flags.go`, `banner.go`, `logging.go`). Independently
  wires framework primitives per [ADR-029](docs/adr/029-product-shell-wiring.md) — no imports from `cmd/semstreams/`.
- **Frozen SemStreams** — `github.com/c360studio/semstreams` pinned to
  `v1.0.0-beta.162.0.20260930150212-8b99efe9c66a` (`8b99efe9c66a4faa4fa509f9f62cc6bad8392128`). Every bump is a
  first-class change ([migration evidence](docs/migrations/semstreams-8b99efe/README.md)). Use fresh isolated NATS
  2.14.4 and graph state; retained-state conversion is not supported.
- **Frontend (`ui/`)** — Svelte 5 + SvelteKit 2 + TypeScript.

| Path | Purpose |
|------|---------|
| `cmd/semteams/` | Product-shell binary wiring Pattern-A/B/C primitives per ADR-029 |
| `cmd/openapi-generator/` | Generate OpenAPI spec from component registry |
| `configs/` | Bootstrap composition, category rule packs, personas |
| `docs/` | Product, ADR, and migration documentation |
| `internal/runtimecatalog/` | Explicit frozen component and payload registration surface shared by runtime and generators |
| `schemas/`, `specs/` | Generated via `task schema:generate` — do not hand-edit |
| `test/contract/`, `test/e2e/mock/`, `test/fixtures/journeys/` | Contract tests, mock LLM/AGNTCY server, Playwright YAML fixtures |
| `ui/` | Svelte 5 + SvelteKit 2 frontend (work board, runs board, graph explorer, composition inventory) |

## Runtime ownership boundary

- Current component implementations, gateways, NATS clients and graph engine remain in frozen SemStreams.
- Future agent-runtime extraction belongs here under ADR-061 in reviewed slices preserving behavior, tests, and
  provenance.
- Generic graph, transport, lifecycle/projection and durability belong in SemEngine; do not duplicate them locally.
- Development-pack activation and the SemEngine switch require separate qualification. The read-only Program Pulse
  remains the first product MVP.

## Common Tasks

```bash
task build              # Build bin/semteams
task test               # Run Go tests (fast)
task test:race          # Go tests with -race
task test:integration   # Integration tests (testcontainers; sequential w/ -p 1 on macOS)
task check              # Go lint + test
task check:all          # Go + UI lint + type-check + test + build
task validate           # Read-only local CI parity gate (Go, UI, Governance)
task schema:generate    # Regenerate schemas/ + specs/openapi.v3.yaml
go test ./test/contract/... -run TestConfigDispatch -v

# UI
task ui:dev             # Start Vite dev server
task ui:test            # Vitest unit/component tests
task ui:test:e2e        # Playwright E2E tests (auto-manages Docker stack)
task ui:lint            # ESLint
task ui:check           # svelte-check (TypeScript)
task ui:build           # Production build
```

## Config Layering & Ops Agent

| Config | Purpose | Model |
|--------|---------|-------|
| `flow-bootstrap.json` | Production substrate (research + autoresearch + coordinator + agent-run + ops packs). | `gemini-flash` default |
| `e2e-flow-bootstrap.json` | Mock-LLM clone with the same packs and disabled compaction. | `mock-llm` |

### Ops Agent Phase 1 (ADR-027, accepted) — WIRED

Read-only diagnostic agent in the same backend process: `configs/rules/ops/01-run-terminal-observe.json` fires once per
run on `agent.run.phase in [completed, failed, cancelled]` with `role: ops-chain-observer`
(`configs/personas/fragments/ops-chain-observer/`).

- **Cadence is set by trigger scope, not a throttle.** Never use `fire_every_n_events` (does not gate `publish_agent`
  on expression rules — semstreams#1007) or `cooldown` (per rule instance; suppression fires `on_exit`).
- Terminates via `decide(action="observed")`. Emits findings via `emit_diagnosis` (`finding`, `recommendation`,
  `confidence` 0.0–1.0, `evidence` ≥1 entity ID), minting `{org}.{platform}.diagnosis.ops.finding.{uuid}`. Phase 2
  (proposing changes) is not deployed; approval requires exact pending execution identity (default 12h timeout).

### Product-Shell Wiring (ADR-029)

`cmd/semteams/main.go` owns composition-root lifecycle ordering:

| Surface | Pattern | Call site |
|---|---|---|
| `runtimecatalog.RegisterComponents` | C | `setupRegistriesAndManager`; shared with schema/OpenAPI generators |
| `persona.NewManager` + `LoadFromDirectory` | B | `loadPersonaFragments` |
| `rule.NewConfigManager` + `config.WithKeyFamily` | B | `run` → `setupRemainingInfrastructure` → shared tool dependencies |
| rule-config lifecycle adapter | B | `registerRuleConfigService`; starts after components in `StartAll`, stops before them |
| public loop/lesson projection contracts | C | `agentic.LoopExecutionContract()` / `agentic.LessonContract()` |
| `payloadregistry.New` + `runtimecatalog.RegisterPayloads` | A | before tool registry; plumbed via `Dependencies.PayloadRegistry` |
| `agentictools.NewExecutorRegistry` + `executors.RegisterBuiltins` | A + B | after persona load; plumbed via `Dependencies.ToolRegistry` |

Product-local `chainpause.Subscriber` and `approvalpause.Subscriber` reflect failed-loop and approval events onto run
state via graph mutation, starting after tool registration. Rule key family registers before config startup; initial
rule reconciliation runs inside the readiness barrier; `StopAll` uses a fresh bounded shutdown context; local writers
read the effective platform ID after config starts.

Canonical IDs are `org.platform.system.domain.type.instance` (loops: `agentic-loop.agent.execution`; runs:
`chain.agent.execution`). Never reconstruct the platform segment from configured base alone. Loop tokens crossing
control boundaries are canonical UUIDs. Composition inspection uses `/components/flowgraph` and `/components/validate`.
Typed `agentic.user_response.v1` belongs on `user.response.*`.

### Component Instance vs Factory & Personalization Toggles

Configs use instance names `teams-dispatch` and `teams-loop` (`/teams-dispatch/*`, `/teams-loop/*`) pointing `"name"`
at upstream factories (`agentic-dispatch`, `agentic-loop`).

Personalization toggles (default `false`):
- `agentic-dispatch.enable_intent_classification` & `enable_onboarding` — off in `flow-bootstrap.json`.
- `agentic-memory.enable_profile_context` — off in `flow-bootstrap.json`.
- `agentic-tools.approval_required` & `enable_categories` — tool approval gates and category filtering.
- `agentic-tools.restricted_decide_actions` — run-level clarification policy (ADR-053 Phase 4b). `[]` = interactive;
  `["ask_user"]` = autonomous (threaded via `extractRestrictedDecideActions` → `RegisterBuiltins`). Autonomous
  deployments also set `-persona-overlay` / `SEMSTREAMS_PERSONA_OVERLAY_PATH` to
  `configs/personas/fragments-autonomous` (`coordinator/12-autonomous-clarification-policy.md`).
- `agentic-governance.enable_tool_governance` — pre-execution governance filtering.

## Mandatory Protocols

### Reviewer-Pass Protocol
Every multi-phase or nontrivial change runs `go-reviewer` (backend), `svelte-reviewer` (`ui/`), or both (cross-stack) at
each critical boundary (see [`.agents/contracts/`](.agents/contracts/)): verify locally → invoke reviewer with explicit
scope → fix criticals/nits or document explicit disagreement → re-verify.

### Product-Shell-Tool Discipline
Before adding a tool in `cmd/semteams/tools/`, rule action shape, local payload type, KV bucket, or stream, complete a
**framework-alignment review** (`cmd/semteams/tools/README.md`):
1. Survey frozen SemStreams and SemEngine's admitted contracts/roadmap; classify as substrate, agent runtime, or pack.
2. Reuse existing primitives; do not fork unreviewed.
3. Preserve agent-specific behavior during extraction; never build a competing journal, effect ledger, or workflow engine.
4. Missing generic mechanisms require upstream contract discussion; domain-local additions require an ADR first.

### E2E Active Monitoring Protocol
UI Playwright journeys run in background and must be monitored every 20–30s via test output, backend logs
(`docker compose -f ui/docker-compose.agentic-e2e.yml logs --since=30s`), and message logger
(`curl -s http://localhost:3100/message-logger/entries?limit=10 | jq '.[].subject'`). Dump evidence to `/tmp/`, abort
early on loops, and quote log lines.

### CI Baseline
`.github/workflows/ci.yml` runs Go, UI, and Governance/OpenSpec jobs feeding `CI Status Check`. Tag-only publication
runs via `.github/workflows/publish.yml` (`docs/release.md`). Before pushing:

```bash
task validate
```

## Related Repos

- [semstreams](https://github.com/c360studio/semstreams) — frozen runtime and extraction baseline (`8b99efe9`).
- [semengine](https://github.com/c360studio/semengine) — target shared substrate (graph, transport, rules, lifecycle, durability).
- [semdev](https://github.com/c360studio/semdev) — maker-side issue-to-reviewed-PR workflow (absorbed under ADR-061).
- [semmem](https://github.com/c360studio/semmem) — cross-product knowledge curator and SOP release publisher.
- [semsource](https://github.com/c360studio/semsource) — code, documentation, and change evidence service.
- [semdragons](https://github.com/c360studio/semdragons) — sibling product on semstreams (`semspec` retired).
