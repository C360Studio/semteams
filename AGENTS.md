# AGENTS.md

This file provides guidance to Codex (codex.ai) when working with code in this repository. Content is intentionally a copy of `CLAUDE.md` — if you edit one, keep the other in sync with `cp CLAUDE.md AGENTS.md` and re-apply the agent-specific opener.

# SemTeams Project Context

SemTeams is an **always-on program manager for a configurable portfolio**. The current runtime composes agentic,
graph, rule and I/O components from the frozen [SemStreams](https://github.com/c360studio/semstreams) dependency.
SemTeams owns the product wiring, category packs, shared persona corpus, product tools, Svelte UI and documentation.

[ADR-061](docs/adr/061-agent-runtime-ownership-and-observable-work.md) records the approved next boundary: SemEngine
owns the shared substrate and generic durability; SemTeams owns the agent runtime and absorbs SemDev as a separately
qualified development pack. Extraction, SemEngine adoption and pack activation are future changes. Do not mistake
the ownership decision for implemented runtime migration.

The owner-approved product direction lives in
[`docs/product/program-manager.md`](docs/product/program-manager.md). Research,
planning, and design are capabilities used in support of programs and projects;
the first target MVP is a read-only, evidence-backed program pulse. The live
runtime still exposes only the research and autoresearch product-facing packs,
so do not describe the target MVP as shipped behavior. The long-term topology
is one program manager coordinating durable project managers; the MVP proves
that view through one program-manager journey with project drill-down.

**Current implementation:** every processor comes from the frozen `github.com/c360studio/semstreams` dependency.
`cmd/semteams/` independently wires the primitives per ADR-029; `internal/runtimecatalog` explicitly registers the
complete frozen component/payload surface for runtime and generators. Registration does not activate a pack.

**Target ownership:** agent loop, dispatch, model/tool orchestration, context, agent-specific approval/governance,
trajectories and agent rule extensions belong in the SemTeams runtime. Generic graph, transport, mutation, rule,
lifecycle/projection and durable-execution mechanics belong in SemEngine. Its generic attempt/effect/replay work
([SemEngine #24](https://github.com/C360Studio/semengine/issues/24)) is planned, not a shipped dependency guarantee.
Preserve existing recovery while the integration contract is resolved; do not create a competing journal or engine.

## Bundled chains are illustrative configurations, not the product

## Substrate-plus-overlays architecture (ADR-042; demo scope reset by ADR-058)

The product shell runs **one** flow config — `configs/flow-bootstrap.json`
— wiring substrate singletons (graph-ingest, graph-query, rule-processor,
agentic-loop, agentic-dispatch, agentic-tools, agentic-model). Task
classes are added as **category-keyed rule packs** + named persona
bundles loaded by the substrate, NOT as separate flow configs.

Live task categories (ADR-058, 2026-08-07):

- **`research`** — coordinator routes prose research asks via
  `decide(action="research")`. The pack at `configs/rules/research/`
  drives `researcher-research-plan → researcher-research-gather (×N
  fan-out) → researcher-research-synthesize → reviewer-research →
  coordinator wake-up`.
- **`autoresearch`** — metric optimization with empirical keep/revert
  in an attested devcontainer sandbox
  (`configs/rules/autoresearch/`).

**Parked donor material (ADR-058):** the dev-side packs (`create-change`,
`proof-readiness`, `dev-from-task`, `dev-via-test`) are on disk but
UNWIRED — they predate the upstream canonical predicate contract
(3-segment lower-kebab, fail-closed at persistence, NO alias mode).
Their contract tests carry a `parked_packs` build tag; their journeys
are `describe.skip`; the coordinator taxonomy is
`research | autoresearch | respond_direct | ask_user`. SemDev currently implements the
issue-to-PR journey. ADR-061 authorizes its future absorption as a separately qualified pack preserving
issue → OpenSpec → implementation → verification → PR, harness-owned outcomes and clean-room verification.
Do not rewire the parked packs as a shortcut to that transfer or to program-manager action. Any separately approved reuse would first require
predicate re-authoring and would fail the current CI fence otherwise
(`test/contract/predicate_contract_test.go`). Read
[ADR-058](docs/adr/058-beta159-realignment-and-demo-lane-focus.md)
before touching any of this.

Adding a new prompt class (e.g. program-report or project-plan) is a **new category pack**: rule files under
`configs/rules/<category>/`, persona bundles under
`configs/personas/fragments/<role>-<category>-<phase?>/`, plus a
coordinator-persona entry teaching the new `decide(action=<category>)`
token. Adding a category does not justify a new runtime component or runtime flow construction. Agent-runtime
extraction is a separate contract change under ADR-061. See
[ADR-042](docs/adr/042-coordinator-instantiated-flows-via-templates.md)
§Phase 2 redesign for the substrate-plus-overlays rationale.

**Shared persona corpus stays domain-neutral.** The persona corpus at
`configs/personas/fragments/coordinator/` (plus the role-specific
dirs above) carries harness-level guidance only (decide contracts,
output structure, tool discipline). Domain flavor (software-domain,
information-domain, decision-domain, …) lives in category packs.

The `personas-describe-job-not-plumbing` memory captures the rule.

> **Note**: The repo's `README.md` was rewritten as SemTeams-specific
> and is the canonical entry point for new readers. This CLAUDE.md
> is the deeper project context (config layering, product-shell
> wiring map, mandatory protocols); `docs/adr/029-product-shell-wiring.md`
> (wiring) and `docs/adr/042-coordinator-instantiated-flows-via-templates.md`
> (substrate-plus-overlays) are the load-bearing ADRs.

## Shared work protocol (Claude and Codex)

State that both agents must see lives in the repository's tools, never in an agent's private memory or a separate
program-status document. Each question has one authoritative home, and each home is discoverable with `gh`, `task`, or
`openspec`.

SemTeams routes design and implementation through its project roles: `architect` owns API and integration contracts;
`go-developer` implements backend work and `go-reviewer` reviews it; `svelte-developer` implements UI work and
`svelte-reviewer` reviews it; cross-stack changes use both reviewer lanes; and `technical-writer` owns durable docs and
conservative OpenSpec task truth.

| Question | Home | Rule |
|---|---|---|
| What is wanted, what kind, is it decided | GitHub issue + labels (`type:` / `area:` / `class:` / `status:` / `horizon:`) | `status:needs-decision` is the owner's docket; a ruling is posted as an issue comment and the label removed. `status:blocked` names its blocker in a comment. |
| What gates the next tag | GitHub milestone named for the intended version | Membership is the gate: in or out; an unruled item is out. `horizon:pre-v1` means before v1.0.0, not before the next tag. |
| An epic | A tracking issue labeled `type:epic` whose body carries a task list of `#n` children | GitHub renders the progress; there is no separate epic document. |
| Who has claimed what | A **draft PR** opened at the start of the work, with `Closes #n` in its body; the branch prefix names the agent (`claude/...`, `codex/...`) | No draft PR, no claim. Design work claims the same way: its OpenSpec proposal is the first content commit. Put the current stop-point in the PR description. |
| Target state and task truth | The OpenSpec change inside that PR; `task openspec:queue` reads its holds | Archive (`openspec archive <id>` plus spec sync) as the landing PR's final content commit, reviewed with the implementation. No task may assert a post-merge fact such as "CI green" or "merge-ready". |
| Why | An ADR, or the owner's ruling comment on the issue | ADRs record durable architectural decisions, not work status or implementation checklists. |

Rituals:

- **Start:** query the milestone's open issues, draft PR claims, `task openspec:queue`, recent `main` runs, and
  `status:needs-decision` issues before choosing work.
- **Take work:** an unclaimed milestone issue → a dedicated worktree and agent-prefixed branch → push → draft PR with
  `Closes #n` → then implementation. While concurrent agents have claimed work, each claimed PR has exactly one
  dedicated worktree and the primary checkout is discovery-only. Verify the worktree path and branch before every edit,
  commit, push, or PR mutation; never share a worktree between claimed PRs.
- **Land:** undraft → the appropriate SemTeams reviewer lane (`go-reviewer`, `svelte-reviewer`, or both), plus the
  owner-run cross-agent round when requested → all applicable local and hosted gates green, with no known unfixed flake
  in a required job → squash merge closes the issue. A fresh green over a known flake is rerun-to-green: fix it, or file
  it and obtain an explicit owner waiver in a PR comment. State `implemented-by: <persona>` in the PR body (Codex uses
  `Sol`).
- **Close:** no issue closes without the owner's explicit `CONFIRM-CLOSE`.
- **CI baseline:** `Repository CI` runs the Go, UI, and Governance/OpenSpec jobs for every pull request to `main`; all
  three feed the stable `CI Status Check` aggregate. Required mock E2E and a main-branch ruleset remain future work.

OpenSpec changes are contract deltas, not backlogs. Sequencing, discovery, holds, and future work belong in GitHub
issues. There is no separate program baton document, and `/tickets` is legacy state pending issue-by-issue
reconciliation.

## Tech Stack

- Go 1.26.3 — `cmd/semteams/` binary (~600 LoC across `main.go`, `flags.go`,
  `banner.go`, `logging.go`). Independently implements every
  framework-wiring pattern per ADR-029 — no imports from upstream
  `cmd/semstreams/`. See [ADR-029](docs/adr/029-product-shell-wiring.md).
- Go module: `github.com/c360studio/semstreams`, pinned to
  `v1.0.0-beta.162.0.20260930150212-8b99efe9c66a` at frozen SHA
  `8b99efe9c66a4faa4fa509f9f62cc6bad8392128`. Every bump is a first-class change.
  See the [migration evidence](docs/migrations/semstreams-8b99efe/README.md) for the beta.160 comparison, qualified compatibility repairs and remaining
  qualification limits. Use fresh isolated NATS 2.14.4 and graph state; retained-state conversion is not supported.
  No production wipe is authorized. Historical ADR-058/059 describe prior transitions.
- NATS JetStream (KV, ObjectStore), Prometheus, slog — via semstreams
- Task (task runner) — run `task --list` for all commands
- `ui/` — Svelte 5 + SvelteKit 2 + TypeScript frontend (subtree-imported
  from semstreams-ui on 2026-04-10, see `ui/.claude/CLAUDE.md` for UI
  conventions)

## What lives here

| Path | Purpose |
|------|---------|
| `cmd/semteams/` | Product-shell binary. Wires Pattern-A/B/C framework primitives per ADR-029 — no custom components, but non-trivial wiring (payload registry, persona loader, Pattern-B managers, `executors.RegisterBuiltins`) |
| `cmd/openapi-generator/` | Dev tool: generate OpenAPI spec from component registry |
| `configs/` | Bootstrap composition, category rule packs, personas; retired flow templates are donor material |
| `docs/` | Product and integration documentation |
| `schemas/`, `specs/` | Generated (via `task schema:generate`) — do not hand-edit |
| `test/contract/` | Contract tests: payload registry consistency, config sanity checks |
| `test/e2e/mock/` | Mock OpenAI / AGNTCY server for UI Playwright journeys |
| `test/fixtures/journeys/` | Playwright journey fixtures (YAML) |
| `ui/` | Svelte 5 + SvelteKit 2 frontend (graph explorer, read-only composition inventory, agentic UI) |
| `docker/` | Production Dockerfile + optional services compose (observability) |

## Runtime ownership boundary

- Current component implementations, gateways, NATS clients and graph engine remain in frozen SemStreams.
- Future agent-runtime extraction belongs here under ADR-061, with existing behavior, tests and provenance carried
  through reviewed slices. No agentic implementation has moved in the foundation change.
- Generic graph, transport, lifecycle/projection and durability belong in SemEngine; do not duplicate them locally.
- Development-pack activation and the SemEngine switch need separate qualification, including recovery, state and
  authority boundaries. The read-only Program Pulse remains the first product MVP.

## Common Tasks

```bash
task build              # Build bin/semteams
task test               # Run Go tests (fast)
task test:race          # Go tests with -race
task test:integration   # Integration tests (testcontainers; sequential w/ -p 1 on macOS)
task check              # Go lint + test
task check:all          # Go + UI lint + type-check + test + build
task schema:generate    # Regenerate schemas/ + specs/openapi.v3.yaml

# Single Go test (use raw go test — no task wrapper)
go test ./test/contract/... -run TestConfigDispatch -v

# UI
task ui:dev             # Start Vite dev server
task ui:test            # Vitest unit/component tests
task ui:test:e2e        # Playwright E2E tests (auto-manages Docker stack)
task ui:lint            # ESLint
task ui:check           # svelte-check (TypeScript)
task ui:build           # Production build
```

## Config Layering

| Config | Purpose | Model |
|--------|---------|-------|
| `flow-bootstrap.json` | Production substrate. Wires research + autoresearch product packs and coordinator + agent-run + ops support packs. | `gemini-flash` default; registry fallbacks remain configurable |
| `e2e-flow-bootstrap.json` | Mock-LLM clone with the same live/support packs and disabled compaction. | `mock-llm` |

UI Playwright journey tasks (in `ui/Taskfile.yml`) manage the Docker stack
lifecycle — Playwright does NOT auto-start the stack. Each task: start →
health-check → test → cleanup.

The legacy concrete configs (`osh-demo.json`, `dev-research.json`,
`agentic.json`, `agentic-claude.json`, `onboarding.json`, all
`e2e-*` except `e2e-flow-bootstrap.json`) retired in ADR-042 MVP-7
(PR #178) alongside the `chain.mode` / `phasevalidator` / `chainstall`
machinery they depended on.

### Ops Agent Phase 1 (ADR-027, accepted) — WIRED

Read-only diagnostic agent. One role, one rule, triggered on the
**run entity** reaching a terminal phase. ADR lives at
`../semstreams/docs/adr/027-ops-agent-meta-harness.md`.

Single-process deployment: the ops agent runs in the same backend as
the chains it observes. The rule fires `publish_agent` with
`role: ops-chain-observer`, and the existing `agentic-loop` consumes
it without a second dispatch. (Upstream ships
`../semstreams/configs/flows/ops-agent.json` as a reference for
operators who prefer a standalone ops binary; SemTeams does not
deploy it.)

- `configs/rules/ops/01-run-terminal-observe.json` — the only ops
  rule. Fires once per run on `agent.run.phase in [completed, failed,
  cancelled]`. Triggering on the run entity rather than on a reviewer
  role is what makes it category-agnostic: it covers research,
  autoresearch, and every future pack without re-authoring, and it
  covers failed/cancelled runs, which a reviewer-completion trigger
  structurally misses.
- `configs/personas/fragments/ops-chain-observer/` — the only ops
  persona corpus.

**Cadence is set by trigger scope, not by a throttle.** Exactly one
ops loop per run. Do not reach for `fire_every_n_events`: it does NOT
gate `publish_agent` (upstream's `shouldFireAction` is only reached
from `fireRuleActions`, while `on_enter` runs through the stateful
evaluator, which never reads the counter — semstreams#1007). Do not
reach for `cooldown` either: it is per rule *instance*, not per
entity, and its suppression path fires `on_exit`.

**Retired in the same pass** (all dead, none re-wireable as written):
the `ops-analyst` role and its `configs/personas/fragments/ops/`
corpus (no rule spawned it); `observe-chain-progress.json` and
`ops-progress-observer/` (its throttle was inert, and a
completion-triggered rule structurally cannot observe a *stalled*
chain — that needs a cron primitive with an idle-cost gate that does
not exist yet); the flat `configs/personas/*.json` files (never read
— `LoadFromDirectory` keys personas by fragment directory); and
`docs/objectives/` (consumed only by the deleted corpus).

`submit_work` does not exist upstream — no executor, no registration,
only a category-map entry and comments (semstreams#1007). Unknown
tool names are logged and dropped, so the old personas instructed a
tool the model never received. Ops terminates with
`decide(action="observed")`, gated by `action_allowlist`.

The ops agent emits findings via the `emit_diagnosis` tool (not raw
triples). Each call requires `finding`, `recommendation`, `confidence`
(0.0–1.0), and `evidence` (≥1 graph entity ID). The framework's
executor mints `{org}.{platform}.diagnosis.ops.finding.{uuid}`
entities with `ops.diagnosis.{finding,recommendation,confidence,
evidence,observed_role,severity}` predicates.

Phase 2 (ops proposes changes) is not part of this deployment. It needs a separately approved tool and approval
contract; retired flow-management APIs are not available. Approval requires the exact pending execution identity,
and omitted approval timeout now means 12 hours. The frozen migration's approval qualification must pass before
broader approval claims are made.

### Product-Shell Wiring (ADR-029)

`cmd/semteams/main.go` independently implements every framework-wiring
pattern the product relies on — it does **not** import from
`cmd/semstreams/`. Upstream's `main.go` is reference, not library.
Composition-root lifecycle ordering is owned here under ADR-029. Live wirings:

| Surface | Pattern | Call site |
|---|---|---|
| `runtimecatalog.RegisterComponents` | C | `setupRegistriesAndManager`; shared catalog with schema/OpenAPI generators |
| `persona.NewManager` + `LoadFromDirectory` | B | `loadPersonaFragments` |
| `rule.NewConfigManager` + `config.WithKeyFamily` | B | `run` → `setupRemainingInfrastructure` → shared tool dependencies |
| rule-config lifecycle adapter | B | `registerRuleConfigService`; starts after components inside `StartAll`, stops before them |
| public loop/lesson projection contracts | C | `agentic.LoopExecutionContract()` / `agentic.LessonContract()` |
| `payloadregistry.New` + `runtimecatalog.RegisterPayloads` | A | before tool registry; existing product payloads layered separately; plumbed via `Dependencies.PayloadRegistry` |
| `agentictools.NewExecutorRegistry` + `executors.RegisterBuiltins` | A + B tool executors | after persona load; plumbed via `Dependencies.ToolRegistry` (beta.16) |

When a journey breaks because a tool executor isn't firing or persona
fragments aren't grounding, suspect drift here first.

Product-local `chainpause.Subscriber` and `approvalpause.Subscriber` also live here. They reflect failed-loop and
approval events onto product run state through the graph mutation contract. They start after tool registration.
The old evidence-body subscriber is retired; trajectory facts/references do not restore UI evidence rendering (#261).

The rule key family is registered before config startup; the same rule manager serves executors and live reload.
Initial rule reconciliation is inside the service readiness barrier. Runtime authority remains live while StopAll
uses a fresh bounded shutdown context. Local writers read the effective platform ID after configuration starts.

Canonical IDs are `org.platform.system.domain.type.instance`: loops use `agentic-loop.agent.execution`, runs use
`chain.agent.execution`. The platform segment includes the framework-minted authority suffix; never reconstruct it
from the configured base alone. Loop tokens crossing control boundaries are canonical UUIDs. `/implement-spec`
uses durable `LookupLoopOwner` and remains subject to the parked-pack boundary.

Saved-flow managers, seed loader, `--flow-templates`, and `/flowbuilder/*` are removed. Composition inspection uses
`/components/flowgraph` and `/components/validate`; admitted composition is not a runtime health signal. GraphQL entity
exploration remains separate. Typed `agentic.user_response.v1` belongs on `user.response.*`; flat rule publishers
must not be reintroduced to bypass a routing gap.

SemSource-backed dogfooding remains held until SemSource is ready. A later SemEngine switch needs an approved
consumer contract; measured compiler dependencies are evidence for that decision, not automatic release scope.

### Component Instance vs Factory

Configs use instance names `teams-dispatch` and `teams-loop` (so HTTP
endpoints at `/teams-dispatch/*` and `/teams-loop/*` match the UI's
hardcoded URL paths). The `name` field points at the upstream factory
(`agentic-dispatch`, `agentic-loop`):

```json
"components": {
  "teams-dispatch": {         // instance name → HTTP prefix
    "type": "processor",
    "name": "agentic-dispatch", // factory lookup
    ...
  }
}
```

### Personalization Toggles (agentic-dispatch, agentic-memory, agentic-tools)

These upstream config fields default `false`; enable per config as needed:

- `agentic-dispatch.enable_intent_classification` — LLM-assisted intent
  classifier. Off in flow-bootstrap.json (the coordinator persona does
  classification via `decide(action=...)`).
- `agentic-dispatch.enable_onboarding` — `/onboard` command + interview
  state machine. Off in flow-bootstrap.json.
- `agentic-memory.enable_profile_context` — assemble operating-model
  profile context on loop creation. Off in flow-bootstrap.json.
- `agentic-tools.approval_required` — list of tool names requiring human
  approval. Off in flow-bootstrap.json (the research-pack arc is
  autonomous; future category packs may add per-tool gates).
- `agentic-tools.enable_categories` — tool category filtering for
  role-based access
- `agentic-tools.restricted_decide_actions` — the run-level **clarification
  policy** (ADR-053 Phase 4b / semstreams#239, beta.104). A list of `decide`
  action names barred for EVERY coordinator task — front-door AND rule-spawned
  — taking precedence over per-task `action_allowlist`. `[]` (default) =
  **interactive** (`ask_user` available); `["ask_user"]` = **autonomous** (the
  coordinator must resolve without deferring to a human; an off-policy
  `decide(ask_user)` is rejected → the loop re-picks `respond_direct`/
  re-dispatch, no dead-end). Threaded via `extractRestrictedDecideActions` →
  `RegisterBuiltins` in `cmd/semteams/main.go` (ADR-029).
  - **Autonomous persona overlay (ADR-053 §4b polish).** An autonomous
    deployment SHOULD also load the autonomous coordinator persona overlay so
    the coordinator skips the otherwise-rejected `ask_user` attempt entirely
    (the LLM resolves ambiguity via `respond_direct` upfront — upstream
    `decide.go:312` says "fix the persona prompt rather than loosening the
    policy"). Set `-persona-overlay`/`SEMSTREAMS_PERSONA_OVERLAY_PATH` to
    `configs/personas/fragments-autonomous`; `loadPersonaFragments` loads it
    AFTER the base tree, and `LoadFromDirectory`'s `<role>/<id>` upsert
    overwrites/adds same-id fragments (here it ADDS
    `coordinator/12-autonomous-clarification-policy.md`). The base
    `coordinator/10-decision-contract.md` stays the interactive default and
    handles a stray rejection gracefully even WITHOUT the overlay. The e2e
    `clarification-autonomous` journey wires the overlay via the Taskfile
    `PERSONA_OVERLAY` var; the behavioral skip is a real-LLM-smoke concern
    (the mock serves fixtures regardless of persona). The gate
    (`restricted_decide_actions`) and the overlay are **intentionally
    decoupled**: the gate is ENFORCEMENT (barred actions are rejected
    regardless of persona); the overlay is an OPTIMIZATION (skip the wasted
    iteration). A deployment that sets the gate but forgets the overlay still
    recovers — the base `10-decision-contract.md` tells the coordinator to
    `respond_direct` with an assumption on an off-policy rejection rather than
    wedge. `loadPersonaFragments` guards a non-resolving overlay path with a
    loud WARN (a typo'd path boots base-only, not silently).
- `agentic-governance.enable_tool_governance` — pre-execution governance
  filtering

## Reviewer-Pass Protocol (MANDATORY)

Every multi-phase implementation runs a reviewer-pass at every
critical step. Critical step = phase boundary or commit boundary;
the agent picks the granularity based on the work's shape.

- `go-reviewer` for backend Go work (`cmd/semteams/`, `test/`,
  upstream coordination).
- `svelte-reviewer` for Svelte / TypeScript frontend (`ui/`).
- Both for cross-stack PRs.

Workflow per critical step:

1. Land the work (commit or phase complete).
2. Verify locally — build, lint, tests must be green.
3. Invoke the appropriate reviewer with explicit scope: which
   files, which contracts, which migration guides if upstream
   beta is involved.
4. Apply the reviewer's findings:
   - **Critical / blocker**: fix before proceeding to next phase.
   - **Nit / recommendation**: fix in the same cycle if
     scope-appropriate; defer with a tracking comment if not.
   - **Disagreement**: explicit, not silent. Document "reviewer
     flagged X; declining because Y" if the recommendation is
     not applied.
5. Verify again post-fix.

Trivial dep bumps / docs-only / single-line edits can skip the
reviewer pass. Anything touching business logic, security
surfaces, API contracts, or accessibility runs the reviewer.

This caught a wire-format bug (`time.Duration` typed as `string`
instead of `number`) and a WCAG 2.5.3 violation in PR #32 that
would have shipped otherwise.

## Product-Shell-Tool Discipline (MANDATORY)

SemTeams keeps application wiring thin (ADR-029) while taking bounded agent-runtime ownership (ADR-061).
Generic substrate mechanics remain SemEngine's responsibility; domain policy belongs in packs. The trap pattern is
**accretion** — each individual product-shell tool, rule, or payload
is defensible; the cumulative drift turns the product shell into a
bespoke monster (the semspec lesson).

Before adding any of these, do a **framework-alignment review**:

- A new tool in `cmd/semteams/tools/`
- A new rule action type or rule action shape
- A new SemTeams-local payload type
- A new KV bucket
- A new long-lived stream

The review:

1. Survey the frozen SemStreams source and SemEngine's admitted contract/roadmap for an existing or planned
   equivalent. Classify the responsibility as substrate, agent runtime or pack before choosing an implementation.
2. Reuse existing primitives. A near equivalent calls for alignment, not an unreviewed fork.
3. Preserve existing agent-specific behavior during extraction and map it to implemented SemEngine contracts.
   Planned generic durability is not permission for a competing SemTeams journal, effect ledger or workflow engine.
4. A missing generic mechanism requires an upstream contract discussion. A genuinely agent-specific or domain-local
   addition requires an ADR explaining ownership, alternatives and any migration target before implementation.

The evidence trail (the ADR addendum recording the survey + the
alternatives ruled out + the migration posture) is what protects
future agents from re-litigating the decision in a vacuum or
silently extending a pattern they don't understand the *why* of.

Reference: `cmd/semteams/tools/README.md` lists the existing
product-shell tools with their migration posture and links the
working-template addendum (ADR-031 §addendum 2026-04-30
"Framework-alignment review for R3.2 emission shape").

If ownership or the required engine contract is unresolved, stop that implementation slice and resolve the boundary.
ADR-061 permits reviewed agent-runtime extraction; it does not permit speculative abstractions, bulk copying or
working around a missing generic primitive in product code. Future observable-work contracts (stable work identity,
artifact revisions, revision-bound decisions, evidence provenance, explicit command outcomes and current-state views)
are design obligations, not claims that those APIs or UI controls are already implemented.

## E2E Active Monitoring Protocol (MANDATORY)

UI Playwright journeys are long-running. MUST monitor actively — never
block in foreground.

1. Launch via `run_in_background: true`
2. Monitor three sources every 20–30s:
   - Test output: non-blocking `TaskOutput` read
   - Backend logs: `docker compose -f ui/docker-compose.agentic-e2e.yml logs --since=30s`
   - Message logger: `curl -s http://localhost:3100/message-logger/entries?limit=10 | jq '.[].subject'`
3. Dump evidence to `/tmp/` for post-mortem
4. Abort early if stuck in loops or burning tokens on retries
5. Report with evidence — quote log lines, never guess at root cause

## CI Baseline

`.github/workflows/ci.yml` defines one unconditional `Repository CI`
workflow for pull requests to `main`. Its Go, UI, and
Governance/OpenSpec jobs feed the always-reported `CI Status Check`
aggregate. Validation tools and runtimes are pinned where their
versions define semantics; official GitHub Actions use reviewed major
tags. Required mock E2E and a main-branch ruleset remain future work.
Publication is tag-only through `.github/workflows/publish.yml`; see
`docs/release.md`.

Before pushing:

```bash
task lint
task test:race
task test:integration
go build ./...
task schema:generate
task schema:check-changes
task openspec:validate
task openspec:queue-test
```

## Related Repos

- [semstreams](https://github.com/c360studio/semstreams) — frozen source of the current runtime and extraction baseline.
- [semengine](https://github.com/c360studio/semengine) — target shared substrate: graph, transport, rules,
  lifecycle/projection and generic durability. Its admitted contracts define the migration boundary.
- [semdev](https://github.com/c360studio/semdev) — current maker-side issue-to-reviewed-PR implementation, to be absorbed
  as a separately qualified SemTeams pack under ADR-061. Preserve its harness and clean-room verification guarantees.
- [semmem](https://github.com/c360studio/semmem) — cross-product knowledge
  curator. Ingests lessons pushed from any SemStreams-based instance, owns the
  SOP repository's content policy, files SOP items as issues, publishes
  practices as versioned SOP releases; serves them over MCP for runtime
  retrieval. Federation back to products happens only via SOP releases.
- [semsource](https://github.com/c360studio/semsource) — code, documentation,
  and change evidence over its own service interfaces; does not share our
  graph.
- [semdragons](https://github.com/c360studio/semdragons) — sibling product
  that also imports semstreams. `semspec` was mined for design and retired
  (semstreams ADR-080).
