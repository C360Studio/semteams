# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

# SemTeams Project Context

SemTeams is an **always-on program manager for a configurable portfolio** built on the
[semstreams](https://github.com/c360studio/semstreams) framework
— the infrastructure that wraps an LLM with tools, memory,
triggers, context, and channels so the model can *operate* rather
than answer one-shot prompts. SemStreams owns the components
(`agentic-dispatch`, `agentic-loop`, `agentic-memory`,
`agentic-tools`, `agentic-governance`, plus graph/I/O processors,
gateways, NATS clients). SemTeams owns the product-shell wiring,
the chain-template library, the shared persona corpus, the Svelte
UI, and the docs.

The owner-approved product direction lives in
[`docs/product/program-manager.md`](docs/product/program-manager.md). Research,
planning, and design are capabilities used in support of programs and projects;
the first target MVP is a read-only, evidence-backed program pulse. The live
runtime still exposes only the research and autoresearch product-facing packs,
so do not describe the target MVP as shipped behavior.

**There are no custom Go components in SemTeams.** Every processor
comes from semstreams via the `github.com/c360studio/semstreams`
Go module dependency. The product shell in `cmd/semteams/` (~600
LoC) independently wires every framework primitive per ADR-029.

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
`research | autoresearch | respond_direct | ask_user`. SemDev now owns the
issue-to-PR implementation journey; do not rewire these packs as a shortcut to
program-manager action. Any separately approved reuse would first require
predicate re-authoring and would fail the current CI fence otherwise
(`test/contract/predicate_contract_test.go`). Read
[ADR-058](docs/adr/058-beta159-realignment-and-demo-lane-focus.md)
before touching any of this.

Adding a new prompt class (e.g. program-report or project-plan) is a **new category pack**: rule files under
`configs/rules/<category>/`, persona bundles under
`configs/personas/fragments/<role>-<category>-<phase?>/`, plus a
coordinator-persona entry teaching the new `decide(action=<category>)`
token. NO new components, NO runtime flow construction. See
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
- Go module: `github.com/c360studio/semstreams` (currently `v1.0.0-beta.160`; every bump is a first-class change — see ADR-058 for the beta.115→159 flag-day and ADR-059 for the beta.160 graph-foundation cutover; fresh NATS storage + NATS server 2.14.4 mandatory across the 159→160 boundary)
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
| `configs/` | Flow-template library. Loadable at runtime via UI |
| `docs/` | Product and integration documentation |
| `schemas/`, `specs/` | Generated (via `task schema:generate`) — do not hand-edit |
| `test/contract/` | Contract tests: payload registry consistency, config sanity checks |
| `test/e2e/mock/` | Mock OpenAI / AGNTCY server for UI Playwright journeys |
| `test/fixtures/journeys/` | Playwright journey fixtures (YAML) |
| `ui/` | Svelte 5 + SvelteKit 2 frontend (graph explorer, flow builder, agentic UI) |
| `docker/` | Production Dockerfile + optional services compose (observability) |

## What does NOT live here

- Framework code (components, gateways, NATS clients, the graph engine) —
  all upstream in semstreams.
- Backend e2e scaffolding — deliberately removed; will be rebuilt from
  scratch when coordinator/ops-agent work lands.
- Custom `agentic-*` processors — upstreamed to semstreams as of beta.8.

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
executor mints `{org}.{platform}.ops.diagnosis.finding.{uuid}`
entities with `ops.diagnosis.{finding,recommendation,confidence,
evidence,observed_role,severity}` predicates.

Phase 2 (ops proposes changes) is **config-only** per upstream's
`_phase2_note`: add `create_rule`/`manage_flow`/etc. to
`allowed_tools` and mirror into `approval_required`. The existing
`ApprovalFilter` transitions the loop to `LoopStateAwaitingApproval`
for human review. No framework blocker remaining.

### Product-Shell Wiring (ADR-029)

`cmd/semteams/main.go` independently implements every framework-wiring
pattern the product relies on — it does **not** import from
`cmd/semstreams/`. Upstream's `main.go` is reference, not library.
Mirroring (~50 lines of boot code) is the cost of admission. Live
wirings:

| Surface | Pattern | Call site |
|---|---|---|
| `componentregistry.Register` | C | `setupRegistriesAndManager` |
| `persona.NewManager` + `LoadFromDirectory` | B | `loadPersonaFragments` |
| `rule.NewConfigManager` | B | `buildRuleManager` → `executors.RegisterBuiltins` |
| `flowstore.NewManager` | B | `buildFlowManager` → `executors.RegisterBuiltins` |
| `flowtemplate.NewManager` | B | `buildFlowTemplateManager` → `executors.RegisterBuiltins` |
| `payloadregistry.New` + `payloadbuiltins.Register` | A | before tool registry; plumbed via `Dependencies.PayloadRegistry` (beta.18) |
| `agentictools.NewExecutorRegistry` + `executors.RegisterBuiltins` | A + B tool executors | after persona load; plumbed via `Dependencies.ToolRegistry` (beta.16) |

When a journey breaks because a tool executor isn't firing or persona
fragments aren't grounding, suspect drift here first.

Product-local subscribers (not tools, not rules) also live here:
`evidence.NATSSubscriber` (agent.complete.> → evidence triples) and
`chainpause.Subscriber` (agent.failed.> → §D5 chain.paused triples).
Both follow the same start-after-tools boot order enforced by
`setupToolsAndPreprocessor`.

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

SemTeams is a thin program-manager product shell on top of semstreams (ADR-029).
The product shell intentionally stays thin. The trap pattern is
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

1. Survey upstream `~/go/pkg/mod/github.com/c360studio/semstreams@<current>`
   for an existing or planned-and-roadmapped equivalent.
2. If exists → use it. If "near" → port to it; do not fork.
3. If planned but not shipped → land a domain-specific instance,
   document the migration target in the relevant ADR addendum.
4. If not in scope upstream by intent → document why the SemTeams
   case justifies a product-local primitive, in an ADR.

The evidence trail (the ADR addendum recording the survey + the
alternatives ruled out + the migration posture) is what protects
future agents from re-litigating the decision in a vacuum or
silently extending a pattern they don't understand the *why* of.

Reference: `cmd/semteams/tools/README.md` lists the existing
product-shell tools with their migration posture and links the
working-template addendum (ADR-031 §addendum 2026-04-30
"Framework-alignment review for R3.2 emission shape").

If you cannot point at an upstream pattern your design implements
or a planned one in the upstream roadmap — **that is a stop signal**.
Either the design is wrong, or the framework is missing a primitive
that should be raised upstream rather than worked around in product
code.

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

- [semstreams](https://github.com/c360studio/semstreams) — framework.
  Owns all `agentic-*`, `graph-*`, `rule`, I/O, and gateway components.
  The place to make framework-level changes.
- [semdragons](https://github.com/c360studio/semdragons),
  [semspec](https://github.com/c360studio/semspec) — sibling products
  that also import semstreams.
