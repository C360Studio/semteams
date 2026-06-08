# ADR-053 Adoption Plan — semteams migration to the agent-run substrate

**Status:** Draft (planning) — 2026-06-07. Authored against semstreams
`v1.0.0-beta.102` on branch `chore/bump-semstreams-beta102`. Not a new ADR;
this is the downstream-consumer migration plan ADR-053 §Consequences calls
for.

> **beta.102 update:** both upstream asks landed. (a) The dispatch `Loop`
> wire now carries `run_id` + `run_entity_id`, populated on `/activity` via
> `loopFromEntity(&e, deps.Platform.Org, deps.Platform.Platform)`
> (`loop_wire.go:70`, `http.go:1004`) → **thread 3's UI tail is unblocked**.
> (b) `$entity.triple.agent.run.entity_id` resolves to the full 6-part run
> entity ID (`cron_substitution_test.go:232`) → **decision #1 below is
> RESOLVED**; Phase 3 is a rule subject-swap, not a Go-handler rewrite.
> Bump is build-clean; only the pre-existing claude-haiku test (#199) fails.

## Why

beta.100 ships **ADR-053 "agent-run-substrate"** — the framework's answer
to semteams **#225**. The *run* (a coordinator + its nested child loops)
becomes a first-class Lifecycle `Participant` (`AgentRun`, entity
`{org}.{platform}.agent.chain.execution.<runID>`), with:

- a `run_scope` field on `publish_agent` rule actions (`new` | `inherit` | `none`),
- typed `RunID` on `agentic.TaskMessage` + `agentic.LoopEntity`,
- `RunID` + `RunEntityID` on all four loop events,
- a `lifecycle.Manager` + `agentrun.Register` + `agentrun.MilestoneSubscriber`.

Adopting it lets semteams **retire its hand-rolled run layer** (~600 LOC of
`cmd/semteams/chain/*`) and inherit operator API, audit history, restart
recovery, and rule integration for free. Net product LOC negative.

## Current state is NOT broken on beta.100

- `go build ./...` clean. Full `go test ./...` green **except**:
  - `TestModelCapabilitiesResolveInRules` — pre-existing (PR #199 fixes; not this bump).
  - `TestCommittedSchemasMatchCode/rule-processor` — the new `run_scope`
    field; fixed by regenerating `schemas/` (done on the bump branch).
- Existing `related_loops`/lineage-threading rules still function — ADR-053
  is additive; `run_scope` defaults to inherit/none.

So beta.100 is adoptable as a plain bump **today**; full ADR-053 adoption is
the project below. Per the ADR, adoption is lockstep with the tag and gated
on `task e2e:agentic` green.

## Ground truth (from the three migration surveys)

### The hand-rolled run layer to retire (`cmd/semteams/chain/`)

| File | LOC | Role | ADR-053 replacement |
|---|---|---|---|
| `resolver.go` | 288 | ancestry walk (`ChainID`), entity read (`NATSEntityReader`, `RequestClassified`) | framework run resolve + lifted fallback walk (WARN) |
| `entity_resolver.go` | 83 | 3-source `ResolveChainEntityID` | typed `RunID`/`RunEntityID` on wire |
| `lineage_reader.go` | 144 | `LineageReader` + `AnchorFromMetadata` | typed `run_id` on TaskMessage |
| `subscriber.go` | 191 | `agent.complete.>` demux | `agentrun.MilestoneSubscriber` |
| `dispatched.go` | 147 | `chain.dispatched.at` | `AgentRun` AuditPredicates |
| `research.go` | 269 | `chain.research_artifact.*` | `MilestoneHandler` |
| `needs_review.go` | 171 | `chain.needs_review.*` | `MilestoneHandler` |
| `terminal.go` | 191 | `chain.terminal.*` | `MilestoneHandler` |
| `predicates.go` | 822 | chain predicate vocabulary | **STAYS** (product milestone vocab) |

Touch-points: `requestsandbox`, `querysandboxattestation`, `chainbash`, and
every `emit_*` tool call `ResolveChainEntityID` / `LineageReader`
(`product_tools.go`); `main.go` wires the subscriber + stampers
(`startChainMilestoneSubscribers`, `startChainPauseSubscriber`).
Product-local subscribers `chainpause.Subscriber` (agent.failed.>) and
the planned `evidence.NATSSubscriber` re-platform onto the framework subscriber.

### Rule audit — 42 files, 3 mint points, zero ambiguous sites

- **`run_scope:"new"` (3 root spawns):** `research/01`, `autoresearch/01`,
  `dev-via-test/01` (the only rules with no inbound lineage).
- **`run_scope:"inherit"` (~23 downstream `publish_agent` rules).**
- **No `run_scope` (~16 stamp-only / coordinator / ops rules).**
- **RUN-ANCHOR keys** (retire as run-anchor): `run-loop-entity-id`,
  `plan-loop-entity-id`, `autoresearch-run`, `dev-via-test-run`,
  `parent_loop_id`/`run_loop_id` (properties), autoresearch lineal config
  (`command`/`surface`/`metric_parser`).
- **SIBLING-LINEAGE keys** (KEEP): `researcher-plan`, `researcher`,
  `terminal`, `autoresearch-propose`, `autoresearch-synthesize`,
  `previous-pack-loop-id/role`, `coordinator-loop-id`, recovery refs,
  ops `qa_reviewer`/`trigger_loop`.

### Upstream API to wire (beta.100)

```go
// main.go startup, after NATS client:
lm := lifecycle.NewManager(natsClient, logger)   // pkg/lifecycle/manager.go:76
agentrun.Register(lm)                             // agentic/agentrun/agentrun.go:187 — registers "agent-run" workflow
ruleProcessor.SetLifecycleManager(lm)             // processor/rule/processor.go:298 (→ actions.go:465)
```

- `run_scope=new` → `agentrun.Mint(ctx, lm, org, platform, firingLoopID)`,
  idempotent, stamps `agent.run` on the firing entity; sets `task.RunID`.
- Terminal: product emits `{"type":"lifecycle_transition","workflow":"agent-run","phase":"completed|failed|cancelled","reason":...}` (D3). Never `Manager.Complete` (non-deterministic with multiple terminal edges).
- `agentrun.MilestoneSubscriber` (`NewMilestoneSubscriber(mgr, reader, pub, org, platform, logger)`) decodes terminal events, demuxes by **payload category** (cancellation rides `agent.complete`), resolves the run, applies the D3 zombie guard, fans to product `MilestoneHandler`s (`OnLoopTerminal(ctx, ev, run, pub)`).

## Two decisions that shape the migration (resolve in Phase 1 design spike)

1. **How rules address the run entity as a subject. — RESOLVED (beta.102).**
   Cross-loop state today upserts via
   `subject: $entity.triple.lineage.run-loop-entity-id` (a *loop* entity).
   ADR-053's run entity is a *distinct* chain entity
   (`agent.chain.execution.<runID>`). beta.102 ships the substitution
   `$entity.triple.agent.run.entity_id` → the full 6-part run entity ID
   (proven by `cron_substitution_test.go:232`). So **Phase 3 is a per-site
   rule subject-swap** (`lineage.run-loop-entity-id` → `agent.run.entity_id`),
   NOT a Go-handler rewrite. The `agent.run.entity_id` triple is present once
   `run_scope` propagation (Phase 2) stamps the run on each loop entity.

2. **research's plan-as-run-entity vs ADR-053's coordinator-rooted run.**
   `run_scope=new` mints the run at the **firing loop = coordinator**.
   - **autoresearch + dev-via-test** already anchor run-state on the
     coordinator loop → mint root aligns; re-anchoring is mechanical.
   - **research** anchors gather-join state on the **plan** loop
     (`run-loop-entity-id = plan $entity.id`, `rule 02`). Its
     `research.gather.completed_subtopic` counters live on the plan entity.
     Re-rooting to the coordinator run entity is the **highest-risk** rewrite
     and must preserve: the `length_eq` join (`03b`), the for_each fan-out
     cardinality (D5 — per-gatherer facts MUST stay loop-qualified or
     parallel gatherers erase each other), and the marker semantics.

Also preserve the **autoresearch iteration-driver presence-marker pattern**
(rule `05`, semstreams#204) when re-homing its `iteration.pending` /
`experiment.completed` upserts.

## Phased plan (each phase: go-reviewer gate + `task e2e:agentic` green)

### Phase 0 — Land the bump (de-risk, non-adoption)
- `go.mod` → beta.100, `go mod tidy`, regenerate `schemas/` + `specs/`.
- Confirm existing packs still run end-to-end on beta.100 via a mock-LLM
  journey (research + autoresearch + dev-via-test fixtures). **No rule
  changes.** This proves the additive framework change is safe and unblocks
  staying current while the rest is planned. Ships as its own PR.
  *(Depends on PR #199 for green CI, or rebase onto it.)*

### Phase 1 — Wire the substrate (additive, no behavior change)
- `main.go`: construct `lifecycle.NewManager`, `agentrun.Register`,
  `ruleProcessor.SetLifecycleManager` (ADR-029 Pattern-B; mirror existing
  manager wiring + boot order).
- Resolve the two decisions above (design spike + upstream confirmation on
  the run-entity subject substitution).
- Validate: manager boots, recovery path clean, **no** rule yet uses
  `run_scope` → zero behavior change. Contract test for registration.

### Phase 2 — Mint + propagate (belt-and-suspenders)
- Add `run_scope:"new"` to the 3 root spawns; `inherit` to downstream
  `publish_agent` rules. **Keep the old `run-loop-entity-id` threading in
  parallel** this phase (dual-write) so nothing breaks.
- Validate: `agent.run` stamped at spawn; `RunID`/`RunEntityID` flow on
  events; run entity minted once (idempotent); restart recovery.

### Phase 3 — Re-home cross-loop state (the big rewrite, per-pack PRs)
Slice by pack, each its own PR + e2e gate, **easiest first**:
- **3a autoresearch** (coordinator-rooted; mechanical): move
  `run-loop-entity-id` upserts → run entity; preserve marker pattern + lineal
  config threading.
- **3b dev-via-test** (coordinator-rooted): same, plus the plan/CBG retry
  drivers.
- **3c research** (plan-rooted; highest risk): reconcile per decision #2;
  keep gather fan-out loop-qualified (D5 cardinality).
- Migrate `emit_*` tools + `requestsandbox`/`querysandboxattestation`/
  `chainbash` off `ResolveChainEntityID`/`LineageReader` onto typed `RunID`.

### Phase 4 — Terminal authority
- Add `lifecycle_transition` rules at coordinator terminals
  (approved→completed, failure→failed, ask_user/cancel paths). Verify D3
  zombie guard handles dispatch-root early termination.

### Phase 5 — Retire the hand-rolled layer
- Re-platform stampers (`dispatched`/`research`/`needs_review`/`terminal`)
  onto `MilestoneHandler`; `chainpause` + `evidence` onto
  `MilestoneSubscriber`. Delete `resolver.go`/`entity_resolver.go`/
  `lineage_reader.go`/`subscriber.go` (or reduce to fallback-only with WARN).
  Keep `predicates.go` vocabulary. Net LOC negative.

### Phase 6 — Tests, contract, docs
- Author `chain_entity_coverage` contract test (D7/D8 wire fields, D4 mint
  idempotence, RunID inheritance at every spawn path) modeled on upstream
  `reference_configs_test.go`.
- Update ADR-038/042/044 addenda; mark this plan executed.

## UI tail (thread 3) — depends on an upstream follow-up

**DELIVERED in beta.102.** The `Loop` wire now carries `run_id` +
`run_entity_id`, populated on `/activity` (`loopFromEntity` with
`deps.Platform.Org/Platform`). The `/loops` summary path leaves them empty
(same posture as `parent_loop_id`); the SSE stream the UI consumes has them.
So once Phase 3 lands (chain triples on the run entity), thread 3 reads them
via `graphApi.getEntity(loop.run_entity_id)` — and the UI's `normalizeWireLoop`
just needs to carry the two new fields through. The conversational-coordinator
gap ("B") follows thread 3.

## Risk / rollback

- Phases 0–2 are additive and individually revertible.
- Phase 3 is the breaking core — per-pack PRs + e2e gate bound blast radius;
  the dual-write in Phase 2 means a Phase-3 slice can revert without losing
  run minting.
- Whole project is lockstep with the beta.100 tag; do not ship a half-migrated
  `main`. Keep adoption on the branch until Phase 3 packs are e2e-green.

## Open questions (carry into Phase 1)

1. ~~Rule substitution for the 6-part run entity ID as an upsert subject
   (decision #1)~~ — **RESOLVED beta.102**: `$entity.triple.agent.run.entity_id`.
2. Cardinality-many run facts (research gather fan-out) — confirm
   loop-qualified predicate shape on the run entity (D5).
3. Whether the dispatcher-direct / MCP spawn site needs `RunID` (ADR-053
   "Open at implementation time").
4. Grammar-collision audit for `agent.run*` tokens before stamping
   (`feedback_grammar_collision_audit_on_new_tokens`).
