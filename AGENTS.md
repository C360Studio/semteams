# AGENTS.md

Guidance for coding agents. Codex and Antigravity (Gemini) load this file as `AGENTS.md`; `CLAUDE.md` imports it, so
there is one copy (kept strictly under 24,000 bytes so rule loaders with a 24 KB per-file cap never truncate it). Role
contracts, model/effort routing, and shared skills live in [`.agents/README.md`](.agents/README.md); the full shared
work protocol lives in [`.agents/protocol.md`](.agents/protocol.md); the enforcement audit matrix lives in
[`.agents/rules-index.md`](.agents/rules-index.md).

## What this is for

SemTeams is an **always-on program manager for a configurable portfolio** ([`docs/product/program-manager.md`](docs/product/program-manager.md)).
The current runtime composes agentic, graph, rule and I/O components from the frozen
[SemStreams](https://github.com/c360studio/semstreams) dependency (`v1.0.0-beta.162.0.20260930150212-8b99efe9c66a`,
`8b99efe9c66a4faa4fa509f9f62cc6bad8392128`). SemTeams owns product wiring (`cmd/semteams/`), category packs
(`configs/`), shared personas, product tools, Svelte 5 / SvelteKit 2 UI (`ui/`), and documentation.

[ADR-061](docs/adr/061-agent-runtime-ownership-and-observable-work.md) records the approved ownership boundary:
SemEngine owns the shared substrate (graph, transport, mutation, rule engine, lifecycle/projection, generic durability
including [SemEngine #24](https://github.com/C360Studio/semengine/issues/24)); SemTeams owns the agent runtime (loop,
dispatch, model/tool orchestration, context, approval/governance, trajectories, agent rule extensions) and absorbs
SemDev as a separately qualified development pack. Extraction, SemEngine adoption, and dev-pack activation are future
changes; preserve existing recovery and never create a competing journal or workflow engine.

Deterministic structural portfolio visibility lives on `/work` (`#309`), while the read-only, evidence-backed **Program
Pulse** (`#267`, `#268`) runs as an operator profile (`/pulse` and opt-in cron `10-scheduled-program-pulse.json`) of
the `research` pack using replay-safe incremental `github_read` cursors (`#273`, `#268`) under the `1..4` fan-out and
loop containment budget (`#272`). Custom work-classification buckets remain deferred to `horizon:post-mvp`.

## Architecture, Config Layering, and Product-Shell Wiring

The product shell runs **one** flow config — `configs/flow-bootstrap.json` (`gemini-flash` default; `e2e-flow-bootstrap.json`
is its `mock-llm` clone with disabled compaction) — wiring substrate singletons (`graph-ingest`, `graph-query`,
`rule-processor`, `agentic-loop`, `agentic-dispatch`, `agentic-tools`, `agentic-model`). Task classes are added as
**category-keyed rule packs** (`configs/rules/<category>/`) + persona bundles (`configs/personas/fragments/`) +
coordinator `decide(action=<category>)` ([ADR-042](docs/adr/042-coordinator-instantiated-flows-via-templates.md)).

- **Live categories ([ADR-058](docs/adr/058-beta159-realignment-and-demo-lane-focus.md)):** `research` (`/pulse` and
  `/research`: `researcher-research-plan → researcher-research-gather ×1..4 → researcher-research-synthesize →
  reviewer-research → coordinator`) and `autoresearch` (`/optimize`).
- **Parked packs (ADR-058):** `create-change`, `proof-readiness`, `dev-from-task`, `dev-via-test` are on disk but
  UNWIRED (`parked_packs` build tag; `test/contract/predicate_contract_test.go` CI fence). Coordinator taxonomy is
  `research | autoresearch | respond_direct | ask_user`. Shared coordinator personas stay domain-neutral.
- **Ops Agent Phase 1 ([ADR-027](docs/adr/027-ops-agent-self-sufficient-operations.md), wired):**
  `configs/rules/ops/01-run-terminal-observe.json` fires once per run on `agent.run.phase in [completed, failed,
  cancelled]` (`role: ops-chain-observer`), terminates via `decide(action="observed")`, and emits `emit_diagnosis`
  (`{org}.{platform}.diagnosis.ops.finding.{uuid}`). Cadence is set by trigger scope, never `fire_every_n_events` or
  `cooldown`.
- **Product-Shell Wiring ([ADR-029](docs/adr/029-product-shell-wiring.md)):** `cmd/semteams/main.go` orders
  `runtimecatalog.RegisterComponents` (Pattern C, shared with `cmd/openapi-generator`) → `persona.NewManager` +
  `LoadFromDirectory` (Pattern B) → `rule.NewConfigManager` + `config.WithKeyFamily` (Pattern B, before config startup)
  → `registerRuleConfigService` (starts after components in `StartAll`, stops before them; initial rule reconciliation
  inside readiness barrier) → `agentic.LoopExecutionContract()` / `LessonContract()` (Pattern C) →
  `payloadregistry.New` + `runtimecatalog.RegisterPayloads` (Pattern A) → `agentictools.NewExecutorRegistry` +
  `executors.RegisterBuiltins` (Pattern A+B) → `chainpause.Subscriber` and `approvalpause.Subscriber` (reading
  effective platform ID after config starts; `StopAll` uses a fresh bounded shutdown context).
- **IDs & Toggles:** Canonical IDs are `org.platform.system.domain.type.instance` (loops: `agentic-loop.agent.execution`;
  runs: `chain.agent.execution`); loop tokens are canonical UUIDs; typed `agentic.user_response.v1` uses
  `user.response.*`. Instance names `teams-dispatch` and `teams-loop` point `"name"` at `agentic-dispatch` and
  `agentic-loop`. Personalization toggles default `false`; `restricted_decide_actions` is `[]` (interactive) or
  `["ask_user"]` (autonomous with `-persona-overlay configs/personas/fragments-autonomous`).

## Commands

```bash
task doctor               # Report tool versions and Docker reachability against pins
task build                # Build bin/semteams
task fmt                  # Format Go sources (worktree-safe)
task fmt:check            # Check Go source formatting without writes
task tidy:check           # Fail if go.mod or go.sum is not tidy
task cleanup-roots:check  # Fail on unbounded context.Background/TODO test cleanup
task lint                 # Go vet, fmt:check, tidy:check, revive, cleanup-roots, lint-test-ports
task test                 # Run Go tests (fast)
task test:unit            # Go unit/contract tests with -race at -cpu 1
task test:race            # Go unit/contract tests with -race
task test:repeat          # Run unit tests 5x shuffled at -cpu 1 without -race
task test:integration     # Integration tests (testcontainers; sequential w/ -p 1 on macOS)
task schema:generate      # Regenerate schemas/ + specs/openapi.v3.yaml
task spec:check           # Strict OpenSpec validation + stray Hold: check (alias: openspec:validate)
task spec:queue           # In-flight OpenSpec changes and Hold: lines (alias: openspec:queue)
task openspec:queue-test  # Fixture test for OpenSpec queue reporter
task publish:verify-test  # Fixture test for release publication gate
task merge:check -- <pr>  # Check main ruleset, cross-agent review lines, and open class:flake issues
task ui:lint              # ESLint
task ui:check             # svelte-check (TypeScript)
task ui:test              # Vitest unit/component tests
task ui:build             # Production UI build
task ui:test:e2e          # Playwright E2E tests (auto-manages Docker stack)
task check                # Go lint + test
task check:all            # Go + UI lint + type-check + test + build
task validate             # Read-only local CI parity gate (Go, UI, Governance) (alias: task verify)
```

Before pushing:

```bash
task validate
```

Gate selection per diff and evidence rules: [`.agents/skills/semteams-preflight/SKILL.md`](.agents/skills/semteams-preflight/SKILL.md).

## Rules and what enforces them

Domain-grouped working reference. [`.agents/rules-index.md`](.agents/rules-index.md) carries the full audit matrix
(what enforces each rule and what stays review only). Short names in "Where": `arch`, `go-dev`, `go-rev`, `svelte-dev`,
`svelte-rev`, `writer` → `.agents/contracts/<role>.md`; `protocol` → `.agents/protocol.md`. In "Gate": `review` means
review only; `verify` means enforced by `task validate` (`verify+review` when structural checks cover the core rule and
edge cases remain review only). A new repo-wide or agent-conduct rule updates both this file and
`.agents/rules-index.md`.

### 1. Go, composition root, and runtime ownership

| Gate | Rule | Where |
| --- | --- | --- |
| `verify+review` | `cmd/semteams/` independently wires Pattern-A/B/C primitives per ADR-029; no imports from `cmd/semstreams/` | `go-dev`, `go-rev`; ADR-029 |
| `verify+review` | Rule key family registers before config startup; rule-config service starts after components in `StartAll` and stops before them; initial rule reconciliation runs inside the readiness barrier; `StopAll` uses a fresh bounded shutdown context | `go-dev`, `go-rev`; ADR-029; `cmd/semteams/main.go` |
| `review` | Canonical IDs are 6 parts (`org.platform.system.domain.type.instance`); never reconstruct the platform segment from configured base alone — read effective platform ID after config starts | `go-dev`, `go-rev`; ADR-029 |
| `verify+review` | `internal/runtimecatalog` explicitly registers the frozen component and payload surface; importing a package registers nothing, and `task schema:generate` keeps `schemas/` and `specs/openapi.v3.yaml` drift-free | `go-dev`, `go-rev`; `task validate` |
| `review` | Before adding a tool in `cmd/semteams/tools/`, rule action shape, local payload type, KV bucket, or stream, complete a framework-alignment review (`cmd/semteams/tools/README.md`) and classify as substrate (SemEngine), agent runtime (SemTeams), or pack | `arch`, `go-dev`, `go-rev`; ADR-061 |
| `review` | Never build a competing generic execution journal, effect ledger, or workflow engine while SemEngine #24 durability contracts are pending | `arch`, `go-dev`, `go-rev`; ADR-061 |
| `review` | Production structs never retain `context.Context`; failure paths fail closed and any skip, drop, or degrade is declared and tested | `go-dev`, `go-rev` |
| `verify` | Pinned `revive` (`revive.toml`), worktree-safe `gofmt`, `go mod tidy -diff`, and `go vet` pass with zero errors | `task lint` (`task validate`) |

### 2. Packs, rules, predicates, and personas

| Gate | Rule | Where |
| --- | --- | --- |
| `verify+review` | All predicates in live configs and rules follow the 3-segment lower-kebab canonical contract (`domain.category.property`); fail closed with no alias mode | `go-dev`, `go-rev`; `TestPredicateContract` |
| `verify+review` | Parked dev-side packs (`create-change`, `proof-readiness`, `dev-from-task`, `dev-via-test`) remain unwired in `configs/flow-bootstrap.json` and `configs/e2e-flow-bootstrap.json` until qualified under ADR-061 | `arch`, `go-dev`, `go-rev`; ADR-058; `TestPredicateContract` |
| `verify+review` | New prompt classes are added as category rule packs (`configs/rules/<category>/`) + persona bundles (`configs/personas/fragments/<role>-<category>-<phase?>/`) + coordinator `decide(action=<category>)`, not new runtime components; shared coordinator personas stay domain-neutral | `arch`, `go-dev`, `go-rev`; ADR-042 |
| `verify+review` | Every `publish_agent` rule action sets an explicit `loop_max_iterations` ceiling and respects the `research` `1..4` fan-out / loop containment budget (#272) | `go-dev`, `go-rev`; ADR-060; `TestResearchContainment` |
| `verify+review` | Rule cadence is set by trigger scope, never expression-rule `fire_every_n_events` (semstreams#1007) or `cooldown` | `go-dev`, `go-rev`; ADR-027 |
| `verify+review` | `configs/e2e-flow-bootstrap.json` stays a structural clone of `configs/flow-bootstrap.json` with `mock-llm` and disabled compaction | `go-dev`, `go-rev`; `TestConfigDispatch` |

### 3. Svelte UI and observable work

| Gate | Rule | Where |
| --- | --- | --- |
| `verify+review` | Browser reads of portfolio and work state go through scoped SvelteKit server routes (`/api/work/*`), never direct browser NATS connections or admin credentials | `svelte-dev`, `svelte-rev`; `observable-work` spec |
| `verify+review` | Graph-derived overlays carry explicit `known | none | unknown` states; never collapse a failed, partial, or unsupported lookup into `none` or simulate unshipped pulse/attention state | `svelte-dev`, `svelte-rev`; ADR-061 §3; `observable-work` spec |
| `verify+review` | Svelte 5 runes (`$state`, `$derived`, `$effect`, `$props`) avoid subscription/SSE reconnect loops and preserve keyboard focus order, `aria-busy`/`aria-disabled`, and WCAG 2.5.3 compliance | `svelte-dev`, `svelte-rev`; `task ui:check`, `task ui:test` |
| `verify` | Frontend `npm run lint`, `npm run check`, `npm run test:unit`, `npm run generate-types:check`, and `npm run build` pass without overwriting tracked generated types | `task validate`; `TestValidateTaskMirrorsRepositoryCI`, `TestValidateTaskAndDocsContract` |

### 4. Testing, E2E, and verification

| Gate | Rule | Where |
| --- | --- | --- |
| `verify+review` | `task validate` (and `task verify`) runs the read-only local parity gate (Go, UI, Governance) without mutating tracked, staged, unstaged, or untracked worktree files | `go-dev`, `svelte-dev`, `go-rev`, `svelte-rev`; `TestValidateScriptCommandCompositionAndWorktreePreservation` |
| `verify+review` | Tests use an independent oracle and are shown able to fail; structural guards carry paired sensitivity tests | `go-dev`, `svelte-dev`, `go-rev`, `svelte-rev`; `TestAgentSetupParitySensitivity` |
| `verify` | Tests bind ephemeral TCP ports (`net.Listen("tcp", "127.0.0.1:0")`) and bound `.Stop`/`.Close`/`.Terminate` cleanup with `context.WithTimeout`, never `context.Background()`/`context.TODO()` | `task lint`; `scripts/lint-test-ports.sh`, `scripts/cleanup-roots-check.sh`; `TestCleanupRootsCheckSensitivity` |
| `review` | Integration tests run sequentially (`-p 1`) under `task test:integration`; serialize heavy Docker/testcontainers runs across concurrent sessions on a shared host | `go-dev`, `go-rev`; `semteams-preflight` skill |
| `review` | Playwright E2E journeys (`task ui:test:e2e`) run in background and are monitored every 20–30s via test output, backend Docker logs, and `/message-logger/entries`, aborting early on loops with quoted log evidence | `svelte-dev`, `svelte-rev` |
| `review` | Never run a git command that can discard working-tree state (`checkout --`, `restore`, `stash`, `clean`, `reset --hard`); temporary in-tree fault checks use `cp` backup + verified `shasum -a 256` restore | `go-dev`, `svelte-dev`, `go-rev`, `svelte-rev`, `writer` |
| `review` | Added or edited statements on how frozen SemStreams, SemEngine, NATS, or SvelteKit behaves cite `file:line` at the pinned commit or a named test/probe, or state it is not promised | `arch`, `go-dev`, `svelte-dev`, `go-rev`, `svelte-rev`, `writer` |

### 5. Workflow, review, and merge gates

| Gate | Rule | Where |
| --- | --- | --- |
| `spec:check+review` | OpenSpec changes and specs are well formed (`task openspec:validate` / `task spec:check`) and truthful against code | `writer`, `go-rev`, `svelte-rev`; `task validate` |
| `spec:check+review` | Every OpenSpec task can be ticked in or before the archive commit; holds are written as `Hold:` inside the unticked task they stop (`scripts/openspec-queue.sh --check`, `TestSpecQueueCheck`) | `protocol`; `writer`, `go-rev`, `svelte-rev`; `task validate` |
| `verify+review` | Steps end when `task validate` passes locally on a clean tree; reviewer `APPROVE` cites commit SHA + local verification run, never CI (#171) | `go-dev`, `svelte-dev`, `go-rev`, `svelte-rev`; `protocol` |
| `CI required` | A merge needs `Repository CI` (`CI Status Check`) green on a head up to date with `main` (`task merge:check -- <pr>`) | `protocol`; `repository-governance` spec; `TestMergeCheckUpToDateRule` |
| `verify+review` | No merge while a known flake (`class:flake` issue) is open in a required job unless the PR closes every open one with before/after reproduction evidence (`task merge:check -- <pr>`) | `protocol`; `scripts/merge-check.sh`; `TestMergeCheckKnownFlake` |
| `verify+review` | Code/UI PRs require review by the appropriate SemTeams reviewer lane (`go-reviewer`, `svelte-reviewer`, or both) and cross-agent review (`implemented-by:` and `reviewed-by:` naming model family `claude`, `codex`, or `gemini`; `task merge:check -- <pr>`); docs-only PRs take any agent's reviewer | `protocol`; `go-rev`, `svelte-rev`; `TestMergeCheckReview` |
| `review` | Any defect found in repo code not fixed in the active change is filed as a GitHub issue (`status:blocked` while its code is only on a PR branch), never left in comments | `protocol`; `go-rev`, `svelte-rev` |
| `review` | A finding needing an unchosen condition (library timing/ordering, OS state, mid-op fault, test-only call sequence) is `BLOCKING`/`HIGH` only with a named routine real-world trigger; else `MEDIUM` at most (#185) | `go-rev`, `svelte-rev`, `go-dev`, `svelte-dev`; `protocol` |
| `verify+review` | A new repo-wide or agent-conduct rule names what enforces it and adds its row to both `AGENTS.md` and `.agents/rules-index.md` | `go-rev`, `svelte-rev`; `TestAgentSetupParity` |
| `review` | Before claiming work, mutating state, or spawning role subagents on an underspecified prompt, check named issues/PRs/files first, then confirm Deliverable, Scope/non-goals, Definition of Done/verification, and Stop boundary | `protocol`; `.agents/README.md` |
| `review` | Role-agent briefs carry the owner's intent and the artifact itself, not a paraphrase; resume for continuity, start fresh for mechanics; designs cap at 3 review rounds before escalating to the owner | `.agents/README.md`; `arch` |
| `verify+review` | `architect`, `go-developer`, `go-reviewer`, `svelte-developer`, and `svelte-reviewer` run on the frontier model tier (`opus` / `gpt-6-astra` or `gpt-6-sol` / Gemini `inherit` or `pro`); `technical-writer` runs on `sonnet` / `gpt-6-sol` / Gemini `inherit` (or `flash` when requested); effort defaults match `.agents/README.md` | `.agents/README.md`; `TestAgentSetupParity` |
| `verify+review` | `CLAUDE.md` only imports `@AGENTS.md`, `AGENTS.md` stays strictly `< 24,000` bytes and matches `.agents/rules-index.md` in numbered domains and row counts, and all role/skill adapters match their canonical contracts | `.agents/README.md`; `TestAgentSetupParity`, `TestAgentSetupParitySensitivity` |
| `review` | Pushed claim branches live in dedicated worktrees (`../semteams-wt/<branch>` or `.worktrees/<branch>`) and update by merging `origin/main`, never by rebase or force-push | `protocol` |
| `review` | Evidence cited in a PR or task can be opened on another machine; git-ignored paths are named as local only, and task choices not in the design go in `openspec/changes/<id>/notes/<task>.md` | `semteams-handoff`, `semteams-preflight` skills; `protocol` |
| `review` | Docs are written for a working developer: shipped vs. target behavior kept distinct (`docs/demo-mvp-claims.md`), coined terms defined at first use, no marketing register | `writer`, `go-rev`, `svelte-rev` |

## Shared work protocol (Claude, Codex, and Gemini)

State that all agents must see lives in the repository's tools, never in private memory or a separate status file.
The canonical protocol is [`.agents/protocol.md`](.agents/protocol.md); role contracts and model/effort routing live in
[`.agents/README.md`](.agents/README.md). Four gates never become a pointer:

- **Clarify:** before claiming work, mutating state, or spawning role subagents, read any named issue, PR, or file so
  you never ask what the repository already answers, then verify four boundaries are clear: **Deliverable** (answer,
  issue, OpenSpec design, docs-only PR, or code/UI PR), **Scope/non-goals**, **Definition of Done/verification**, and
  **Stop boundary**. When a prompt leaves any of the four ambiguous, stop first, propose a concise 3–4 bullet
  recommended Definition of Done, and ask at most 1–3 targeted questions; once confirmed, run autonomously to that
  stop boundary without pausing on routine steps.
- **Claim:** a draft PR opened before the work, in its own worktree on an agent-prefixed branch (`claude/<topic>`,
  `codex/<topic>`, `gemini/<topic>`; CLI uses `../semteams-wt/<branch>`, Enjoy desktop uses `move_to_worktree` under
  `.worktrees/<branch>`) with `Closes #n` (or `Addresses #n` for an epic). No draft PR, no claim. Primary checkout is
  discovery-only. Verify worktree path and branch before every edit, commit, push, or PR mutation.
- **Merge:** appropriate SemTeams reviewer lane (`go-reviewer`, `svelte-reviewer`, or both) + cross-agent review on
  code/UI PRs (`Review request` / `Review record` comments; `implemented-by:` and `reviewed-by:` in the PR body naming
  model family `claude`, `codex`, or `gemini`) → archive/spec sync (`openspec archive <id>`) as the final content
  commit → `task validate` and `Repository CI` (`CI Status Check`) green on a head up to date with `main` with no open
  unfixed `class:flake` → squash merge.
- **Close:** no issue closes without the owner's explicit `CONFIRM-CLOSE` or the squash merge of an approved PR that
  declared `Closes #n`.

## Roles and Related Repos

Role agents are the default path for nontrivial work ([`.agents/contracts/`](.agents/contracts/)): `architect`
(read-only inventories + contract/OpenSpec/ADR drafts), `go-developer` (backend/runtime/packs/contract tests),
`go-reviewer` (independent read-only backend/pack audit), `svelte-developer` (`ui/` Svelte 5 / SvelteKit 2 + tests),
`svelte-reviewer` (independent read-only UI/accessibility/contract audit), and `technical-writer` (durable docs +
conservative OpenSpec task truth).

Related repos (read-only from here): [semstreams](https://github.com/c360studio/semstreams) (`8b99efe9`),
[semengine](https://github.com/c360studio/semengine), [semdev](https://github.com/c360studio/semdev),
[semmem](https://github.com/c360studio/semmem), [semsource](https://github.com/c360studio/semsource),
[semdragons](https://github.com/c360studio/semdragons).
