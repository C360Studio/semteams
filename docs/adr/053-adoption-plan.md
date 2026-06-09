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

2. **research's plan-as-run-entity vs ADR-053's coordinator-rooted run.
   — RESOLVED (Phase 3c, 2026-06-09): the gather-join STAYS on the plan
   loop; do NOT re-root it to the run entity.**
   `run_scope=new` mints the run at the **firing loop = coordinator**.
   - **autoresearch + dev-via-test** already anchor run-state on the
     coordinator loop → mint root aligns; re-anchoring is mechanical.
   - **research** anchors gather-join state on the **plan** loop
     (`plan-loop-entity-id = plan $entity.id`, `rule 02`). Its
     `research.gather.completed_subtopic` counters live on the plan entity.
     The original premise — "re-root to the coordinator run entity" — was
     **rejected** in implementation. The gather-join is **per-plan-iteration**
     join state, not run-wide: rule 05 (reviewer-rejected retry) spawns a
     *fresh* plan loop that fans out its own gatherers and joins on its own
     accumulator. The run entity is **shared across plan iterations**, so
     re-rooting the accumulator there would *introduce* a retry over-counting
     bug the plan-loop anchor naturally avoids. The join is also threaded
     **100% via framework lineage** (`related_loops` → `buildLineageTriples`
     → `lineage.plan-loop-entity-id`), with **zero `cmd/semteams/chain/*`
     coupling — so re-homing it does nothing for Phase 3's actual goal
     (retire the hand-rolled chain layer). There is no framework substitute
     for "the plan loop" anyway (`agent.run.entity_id` is the *coordinator*
     root, the wrong entity). The survey's "`plan-loop-entity-id` = run-anchor
     to retire" was a `<x>-loop-entity-id` naming heuristic, not a coupling
     analysis. **Net: rules 02/03a/03b unchanged; the `length_eq` join, the
     for_each fan-out cardinality (D5), and the marker semantics are all
     preserved exactly as-is.** research's only genuine `cmd/semteams/chain/*`
     coupling was `emit_plan`'s fail-soft chain read (slug-stem / depends-on
     override, a dev-via-spec affordance dead in the plan-first research pack);
     3c retires that read, dropping the last production user of
     `chain.LineageReader`.

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
- **3c research** (plan-rooted) — **SHIPPED 2026-06-09.** Reframed per
  decision #2's resolution: the gather-join is per-plan-iteration framework
  lineage, not run-anchor state, so rules 02/03a/03b are **unchanged** (the
  feared "highest-risk rewrite" was unnecessary and would have introduced a
  retry over-counting bug). The slice retired `emit_plan`'s fail-soft
  `chain.LineageReader` read (dead in the plan-first research pack) + its
  `buildChainLineageReader` wiring + the planner persona's stale
  chain-derived-slug prose. `emit_research_artifact` already wrote its own
  loop entity. Net: research no longer uses `chain.LineageReader` (last
  production user) → Phase 5 can delete `lineage_reader.go`.
- Migrate the remaining shared chain-resolver users — `chainbash` (bash
  wrapper, all packs) + `requestsandbox`/`querysandboxattestation` (sandbox
  tools, dev-via-test/autoresearch) — off `ResolveChainEntityID` onto typed
  `RunID`. **Deferred to a separate shared-infra slice** (cross-cutting, not
  research-specific; bundling it into a pack PR would violate the per-pack
  blast-radius discipline that bounded 3a/3b).

### Phase 4 — Terminal authority
- Add `lifecycle_transition` rules at coordinator terminals
  (approved→completed, failure→failed, ask_user/cancel paths). Verify D3
  zombie guard handles dispatch-root early termination.

#### Phase 4 design spike (2026-06-09) — REVIEW BEFORE WRITING ANY RULES

Phase 4 is the first phase that makes the run *advance* past `dispatched`.
It is materially more intricate than 3a–3c (the firing-entity constraint
on transitions + a framework-vs-product division of labor + a real race),
so this spike pins the design before implementation.

**A. Model (ADR-053 §D3).** Framework **mints** (Phase 2, done) +
**observes** (the `MilestoneSubscriber`, wired here). The
**product/coordinator** owns the terminal decision and fires a
`lifecycle_transition` rule action to `completed`/`failed`/`cancelled`.
The subscriber's ONLY framework-initiated terminal is the **zombie
fallback**: the dispatch-root loop terminates (fail/cancel) while the run
is still `dispatched` → it transitions the run to `failed`/`cancelled`.
The phase machine is **no-skip**: `dispatched → executing →
{awaiting_approval, completed, failed, cancelled}`, `awaiting_approval ⇄
executing`. A run CANNOT go `dispatched → completed` directly.

**B. Load-bearing constraint — transitions fire on the run entity.**
`executeLifecycleTransition` transitions `ec.EntityID`, the **firing
entity**, with NO entity override (verified upstream
`processor/rule/actions_lifecycle.go:40`; the reference rule
`configs/rules/lifecycle/01-mission-launch.json` transitions
`$entity.id`). So every run-phase transition rule MUST **fire on the run
entity** (`agent.chain.execution.<runID>`). The rule processor already
watches `ENTITY_STATES` `c360.>`, which matches run entities (proven by
3a's rule 05). → A **new substrate-level rule pack
`configs/rules/agent-run/`** (loaded by the bootstrap alongside
`coordinator`/`ops`, NOT a category pack) holds the run-entity-firing
transition rules. Wire it into `flow-bootstrap.json` +
`e2e-flow-bootstrap.json`.

**C. The 2-step marker for terminals.** The coordinator/loop terminal
rules fire on the **coordinator loop**, not the run entity. To reach the
run: (i) the loop-firing terminal rule `add_triple`s a marker
(`agent.run.terminal.<state>`) onto the run entity via the subject
override `$entity.triple.agent.run.entity_id`; (ii) a run-entity-firing
rule in the `agent-run` pack matches the marker (+ guards current phase)
→ `lifecycle_transition`. The wake-up coordinator carries
`agent.run.entity_id` (inherited from the reviewer/CBG loop, which is in
the run). **GUARD:** a *non-delegating* coordinator answering a plain
chat via `respond_direct` has NO run (no `run_scope=new` fired) → no
`agent.run.entity_id` → the marker stamp MUST be conditional on the
triple's presence so it no-ops for run-less chats (rule-01-fence
discipline). Grammar-collision audit the `agent.run.terminal.*` tokens
before stamping (`feedback_grammar_collision_audit_on_new_tokens`).

**D. `dispatched→executing` + the D3 race — THE risk.** D3's *intended*
invariant is "root terminates while `dispatched` AND **no child handoff**
= zombie," but the framework code (`agentrun.go:572`) checks ONLY
`phase=="dispatched"` + `ev.LoopID==runID` — **no children check.** Mint
⟺ child handoff (both atomic in rule 01's `run_scope=new`), so a
run-entity rule matching the freshly-minted `agent.run.phase=="dispatched"`
→ `executing` is the correct "transition-at-handoff" semantic. **RACE:**
the coordinator (root) dispatches the child, then its own loop terminates
→ `LoopCompletedEvent(coordinator)`. If `dispatched→executing` has not
landed when the subscriber processes that event, D3 wrongly `failed`s a
healthy run. The mint is durable BEFORE the child is even dispatched
(`actions.go:1193` mint precedes `1330` publish), so the transition
*likely* lands first — but the window is real and async.
**Architect-corrected posture:**
- The D3 bug is **worse than "fails on a timing edge":** D3 maps
  `CategoryLoopCompleted → "failed"` *unconditionally* (`agentrun.go:574`;
  only `cancelled` is special-cased). A coordinator that **completes
  successfully** while the run is still `dispatched` gets the run marked
  `failed`. The handoff *did* happen.
- **No deterministic product-config closure exists** (confirmed): every
  lever — co-firing in rule 01, driving off the child's start, keeping the
  coordinator alive — fails the firing-entity constraint or isn't
  configurable. So implement → validate empirically → upstream-ask IS the
  right posture.
- **Mock is a POSITIVE race detector, not a blind spot.** Mock-LLM
  finishes the coordinator's `decide` near-instantly → *shrinks* the window
  for `dispatched→executing` to land first → mock is MORE likely to lose
  the race. A green mock run across N runs ⇒ the fast-path ordering is
  robust; real-LLM then confirms the slow path. **The mock journeys MUST
  assert `agent.run.phase` directly** (graph-query / KV read == `completed`,
  NOT `failed`) — current specs assert via UI only and would miss a run
  silently marked `failed`.
- **Upstream ask (if it bites), correctly framed:** "D3 must NOT fire on
  `CategoryLoopCompleted` — a successfully-completed root is by definition
  not a zombie; only a root that *failed/cancelled* while `dispatched` is."
  NOT "check `run.Children`" — agent-run declares no `ChildWorkflows`, so
  `Manager.Children` is empty and the check is unimplementable without a
  child-link predicate. The Phase-4 watch-item.

**E. Per-pack terminal mapping — the terminals CONVERGE (key finding).**

| Transition | Trigger | Shared? |
|---|---|---|
| `dispatched→executing` | run-entity rule on freshly-minted `agent.run.phase=dispatched` | **Universal** (substrate) |
| `executing→completed` | ALL packs wake coordinator → `decide(respond_direct)` → `coordinator/03b-respond-direct` | **Shared** (one marker + one run-entity rule; guard run-less chats) |
| `executing→failed` | per-pack `loop-failed-pause` markers **+** D3 | **NOT cleanly shared — coverage holes** (see below) |
| `executing⇄awaiting_approval` | NOT the CBG gate (it's an automated reviewer) — reserve for the real `approval_required` tool-gate | **Deferred / re-scoped** (see §H 4c) |
| `→cancelled` | `coordinator/03-ask-user` / cap-exhausted-to-human | **OPEN** (see Q2) |

Only `dispatched→executing` and `executing→completed` are **cleanly
substrate-shared** (validated across all three journeys). The other rows
have problems the architect review surfaced:

- **`executing→failed` has un-covered failure paths (HIGH).** No
  `loop-failed-pause` rule lists the `coordinator` role — so a coordinator
  (incl. wake-up coordinator) that fails while the run is `executing`
  stamps no marker, and D3 only fires while `dispatched` → **permanent
  `executing` zombie** (exactly the class D3 was meant to kill, uncovered
  past `dispatched`). Also `dev-via-test/08` lists only `dev-via-test-plan`
  + `reviewer-dev-via-test`, NOT `dev-via-test-execute` (ralph) → a ralph
  failure stamps no marker. So `executing→failed` needs a `coordinator`-role
  failed rule in the shared pack **+** a per-pack failed-pause role-list
  audit → **split into its own slice (4a′)**, not bundled into 4a.
- **`awaiting_approval` ≠ the CBG gate (MEDIUM).** `06-coordinator-dispatch-cbg`
  dispatches a `reviewer-dev-via-test` — an **automated** reviewer, not a
  human pause. The real human-approval surface is
  `agentic-tools.approval_required` → `ApprovalFilter` →
  `LoopStateAwaitingApproval` (OFF in flow-bootstrap). So CBG-in-flight
  stays `executing`; `awaiting_approval` is reserved for the
  `approval_required` tool-gate if/when enabled.

So **only two** transitions are load-bearing-and-clean for the first slice
(`dispatched→executing`, `executing→completed`); failure-phase modeling is
its own slice.

**F. Subscriber wiring (`main.go`).** Construct
`agentrun.NewMilestoneSubscriber(mgr, loopReader, triplePublisher, org,
platform, logger)` after the lifecycle manager (today
`attachLifecycleManager` drops the handle into
`svcDeps.LifecycleManager` and returns — expose/return it for the
subscriber). `AddHandler` for product `MilestoneHandler`s — Phase 5
re-platforms the chain stampers as handlers, so Phase 4 registers none
(or a thin one). `Start(ctx, natsClient, StartConfig{StreamName})` with
the stream from config (default `AGENT`), creating 2 durable consumers
(`agent.complete.*`, `agent.failed.*`). `loopReader` is a
`LoopTripleReader` for the fallback `ResolveRun` walk (un-threaded
loops); reuse a NATS entity reader. Boot order: alongside
`startChainMilestoneSubscribers` / `chainpause`, mirroring the existing
subscriber boot (ADR-029). The chain stampers KEEP running in parallel
(dual-write) until Phase 5.

**G. Restart recovery + the phase-guard requirement (architect-corrected).**
Durable consumers resume from last-ack. The manager has **no same-phase
short-circuit**: `TransitionWith` on an already-applied edge returns a
**hard error** (`ErrInvalidTransition` — there is no `executing→executing`
self-edge; `manager.go:445`) or `ErrTerminalPhase` if already terminal.
On the rule path that error is **logged at ERROR and dropped**
(`stateful_evaluator.go:405`) — the KV-watch entity path always Acks (no
NAK-forever loop), and the subscriber's D3 failure also logs-and-Acks. So
there is no redelivery wedge, BUT every benign duplicate/re-fire emits an
ERROR line — which would page a real-LLM smoke monitor armed on
`"level":"ERROR"`. **REQUIREMENT: every transition rule must be
phase-guarded** (a condition on the current `agent.run.phase` before the
`lifecycle_transition`) so the invalid edge is never *attempted*. This
also closes RISK 4 (below). State-machine testing per
`feedback_state_machine_testing`: restart mid-run, duplicate terminal,
out-of-order terminal.

**G2. RISK 4 — the early-marker second-order race (architect-found).** The
`executing→completed` flow is two async hops: (i) the loop rule stamps
`agent.run.terminal.completed` on the run entity; (ii) the run-entity rule
matches it → transition. On a fast chain the marker can land while the run
is still `dispatched` (if `dispatched→executing` hasn't fired yet) → the
rule attempts the illegal `dispatched→completed` (no-skip machine) →
dropped → **the completed transition is permanently lost; the run sticks**
(markers don't redeliver; no new KV revision re-fires the rule). Mitigation:
the completed-marker rule's phase-guard must make it **re-evaluate when the
phase advances** (not fire-once), OR `dispatched→executing` must be proven
to always land first. This is the strongest argument for landing a
rock-solid, fast `dispatched→executing` BEFORE any terminal marker — hence
the narrowed 4a (§H).

**H. Slicing (architect-narrowed — conditional Go on 4a).**
- **4a (subscriber + the two clean transitions ONLY):** subscriber wiring +
  the `agent-run` pack with `dispatched→executing` and `executing→completed`
  (respond_direct marker) — **both phase-guarded** (§G) — + the run-less-chat
  guard. Deliberately EXCLUDES `executing→failed` (coverage holes, §E) and
  `awaiting_approval` (re-scoped, §E). This isolates the D3 race (§D) and the
  early-marker race (§G2) as the two things 4a proves. Gate: go-reviewer +
  **all 3 pack mock journeys green with a DIRECT `agent.run.phase` assertion
  (`completed`, not `failed`/`dispatched`)** — mock is the positive race
  detector — + a **real-LLM research smoke** for the slow-path timing.
- **4a′ (failure phase, its own slice):** `executing→failed` — add a
  `coordinator`-role failed rule in the shared pack (fail-while-`executing`
  → `executing→failed`) + audit each pack's `loop-failed-pause` role list
  for completeness (dev-via-test missing `ralph`, etc.). Separate because
  the coverage holes need deliberate work, not a one-liner.
- **4b:** `ask_user` / `needs_clarification` / cancel run-phase semantics
  (Q2).
- **4c:** the real `approval_required` tool-gate → `awaiting_approval`
  (NOT the CBG automated reviewer, which stays `executing` — §E).
- Then Phase 5.

**I. Open questions.**
1. **D3 race** — empirical result; upstream ask ("D3 must not fire on
   `CategoryLoopCompleted`") if it bites (the watch-item).
2. **`ask_user` phase** — `awaiting_approval` (waiting on the user), or
   leave `executing` (the user reply re-dispatches and the run never
   actually paused), or `cancelled`? Lean: `ask_user` is NOT run-terminal;
   `needs_clarification`→coordinator recovery stays `executing`; only a
   genuine dead-end (cap-exhausted-to-human, no recovery) → `cancelled`.
3. **Marker vocabulary** — finalize `agent.run.terminal.*` predicate names
   + the grammar-collision audit.
4. **Handlers in 4a?** — register a thin `MilestoneHandler` now, or wait
   for Phase 5's stamper re-platform. (Lean: none in 4a — the two
   transitions are pure rules; handlers arrive with the Phase-5 re-platform.)
5. **Phase-guard re-evaluation** — confirm a phase-guarded marker rule
   re-evaluates when the phase advances (closes G2), and that an
   already-applied transition never *attempts* the invalid edge (closes the
   ERROR-noise of §G).

**Architect review (2026-06-09):** spike reviewed against upstream
beta.102. Verdict: architecture sound (substrate `agent-run` pack,
firing-on-run-entity, 2-step marker, dual-write); **conditional Go on a
narrowed 4a** with the four fixes folded in above (phase-guards + §G
rewrite; `executing→failed` split to 4a′; direct `agent.run.phase`
journey assertions + mock-as-detector; upstream ask reframed). §B/§C/§F
and the `executing→completed` convergence confirmed sound; multi-iteration
runs verified to stay `executing` correctly.

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
2. ~~Cardinality-many run facts (research gather fan-out) — confirm
   loop-qualified predicate shape on the run entity (D5).~~ — **RESOLVED
   (Phase 3c)**: the gather fan-out facts STAY on the plan loop (naturally
   loop-qualified by Object=`$entity.id`), not the run entity. Per-iteration
   isolation is a feature, not a constraint to migrate. See decision #2.
3. Whether the dispatcher-direct / MCP spawn site needs `RunID` (ADR-053
   "Open at implementation time").
4. Grammar-collision audit for `agent.run*` tokens before stamping
   (`feedback_grammar_collision_audit_on_new_tokens`).
