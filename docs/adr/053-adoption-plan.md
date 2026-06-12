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

**C. The 2-step marker (the firing-entity escape hatch).** The loop-firing
rules fire on a **loop entity**, not the run entity. To reach the run: (i)
the loop-firing rule `add_triple`s a marker onto the run entity via the
subject override `$entity.triple.agent.run.entity_id`; (ii) a
run-entity-firing rule in the `agent-run` pack matches the marker (+ guards
current phase) → `lifecycle_transition`. The concrete markers (refined per
the Coby review — see §D/§E): **`agent.run.handoff`** (confirmed-handoff,
drives `dispatched→executing`) and **`agent.run.outcome=success`** (drives
`executing→completed`). Each stamping loop carries `agent.run.entity_id`
(the coordinator-dispatch loop from its own mint; the reviewer/CBG loop via
inherit). **GUARD:** a *non-delegating* coordinator answering a plain chat
has NO run (no `run_scope=new` fired) → no `agent.run.entity_id` → the
marker stamp MUST be conditional on the triple's presence so it no-ops for
run-less chats (rule-01-fence discipline). Grammar-collision audit the
`agent.run.*` tokens before stamping
(`feedback_grammar_collision_audit_on_new_tokens`).

**D. `dispatched→executing` + the D3 race — THE risk.** D3's *intended*
invariant is "root terminates while `dispatched` AND **no child handoff**
= zombie," but the framework code (`agentrun.go:572`) checks ONLY
`phase=="dispatched"` + `ev.LoopID==runID` — **no children check.**

**Trigger on CONFIRMED HANDOFF, not bare mint (Coby review P1).** Mint is
NOT atomic with handoff: `agentrun.Mint` runs at `actions.go:1193`, the
child `publisher.Publish` at `1330`, and `Publish` can **fail** — leaving a
run minted `dispatched` with NO child. If `dispatched→executing` fired off
the freshly-minted `agent.run.phase==dispatched`, a publish failure would
strand the run in `executing` with no child — a zombie WORSE than
`dispatched` (D3 can't catch `executing`). The correct handoff evidence is
`rule.spawned_task`, stamped on the firing (coordinator) loop entity **only
after publish success** (`actions.go:1353`). So the trigger is a **handoff
marker**: a rule on the coordinator loop matching `rule.spawned_task`
present + `agent.run.entity_id` present → `add_triple agent.run.handoff` on
the run entity (subject override); a run-entity rule matches
`agent.run.handoff` + `phase==dispatched` → `executing`. **Publish-failure
is now safe:** no `rule.spawned_task` → no handoff marker → run stays
`dispatched` → D3 correctly fails it as a genuine childless zombie.

**RACE:** the coordinator (root) hands off the child, then its own loop
terminates → `LoopCompletedEvent(coordinator)`. If `dispatched→executing`
has not landed when the subscriber processes that event, D3 wrongly
`failed`s a healthy run. The handoff marker is stamped right after the
durable publish-success, before the coordinator loop's own completion
event, so the transition *likely* lands first — but the window is real and
async. **Architect-corrected posture:**
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
| `dispatched→executing` | confirmed-handoff marker (`rule.spawned_task` → `agent.run.handoff` on run entity), §D | **Universal** (substrate) |
| `executing→completed` | explicit **success outcome** (`agent.run.outcome=success`) stamped by the reviewer/CBG-**approved** rules, NOT bare `respond_direct` (Coby review P1) | **Shared** (success rules + one run-entity rule) |
| `executing→failed` | per-pack `loop-failed-pause` markers **+** D3 | **NOT cleanly shared — coverage holes** (see below) |
| `executing⇄awaiting_approval` | NOT the CBG gate (it's an automated reviewer) — reserve for the real `approval_required` tool-gate | **Deferred / re-scoped** (see §H 4c) |
| `→cancelled` | `coordinator/03-ask-user` / cap-exhausted-to-human | **OPEN** (see Q2) |

**`executing→completed` must be success-discriminated (Coby review P1).**
`coordinator/03b-respond-direct` fires on EVERY `respond_direct` — including
the limitation path ("the request cannot be served": research/06,
autoresearch/10 plateau/environment-failure). Those coordinators DO carry
`agent.run.entity_id` (they're in the run), so the run-less guard does NOT
exclude them — marking all `respond_direct` as `completed` would mark a
failed/limited run as a success. The clean discriminator: the **success**
terminals are the reviewer/CBG-**approved** rules (`research/07`,
`autoresearch/08`, `dev-via-test/07a`), which carry
`wakeup_mode: chain_terminal_*` (vs the recovery path's
`recovery: needs_clarification` / `autoresearch_needs_clarification`).
**Chosen mechanism:** those three success rules stamp an explicit
`agent.run.outcome=success` on the run entity (subject override — they fire
on reviewer/CBG loops that carry `agent.run.entity_id`); a run-entity rule
matches `agent.run.outcome==success` + `phase==executing` → `completed`.
This **decouples the completed transition from `respond_direct` entirely** —
`03b` stays purely about delivering the reply, and the limitation/front-door
`respond_direct` paths never stamp success → never complete (their
failed/cancelled outcomes are 4a′/4b). (Alternative considered: gate a
completed marker on `respond_direct` + `wakeup_mode in [chain_terminal_*]`;
rejected — couples completion to reply-delivery and re-introduces the
respond_direct flavor discrimination the outcome-marker avoids.)

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
phase-guarded with the current `agent.run.phase` as a TOP-LEVEL rule
CONDITION** (e.g. `agent.run.outcome==success` AND `agent.run.phase==
executing`), **NOT an action-level `when`.** This is the load-bearing
distinction (Coby review): a top-level condition keeps the rule
*not-matching* until the edge is legal, so the invalid edge is never
*attempted* (no ERROR, no lost transition). A phase guard buried in an
action `when` would let the rule *enter* on `outcome=success` alone, skip
the action while `dispatched`, and — because the rule already entered —
never re-enter when the phase advances → the completed transition is
silently lost. Put phase in the conditions, never in `when`. State-machine
testing per `feedback_state_machine_testing`: restart mid-run, duplicate
terminal, out-of-order terminal.

**G2. RISK 4 — the early-marker race (RESOLVED by beta.102 semantics +
top-level phase guard).** The `executing→completed` flow is two async hops:
(i) a success rule stamps `agent.run.outcome=success` on the run entity;
(ii) the run-entity rule matches `outcome==success` AND `phase==executing`
→ transition. On a fast chain the success marker can land while the run is
still `dispatched`. **With the top-level phase guard (§G), this is benign,
not a lost transition:** the completed rule's condition is simply FALSE
while `dispatched` (it does NOT attempt an illegal `dispatched→completed`)
and the evaluator stores it as not-matching. beta.102 re-evaluates **every**
entity-state rule on **each** KV revision of the entity, not just rules
whose specific predicate changed (`message_handler.go:245`); the stateful
evaluator compares prior `IsMatching=false` → current true and fires
`on_enter` (`stateful_evaluator.go`), with a durable stale-replay guard for
exactly-once. So when `dispatched→executing` writes the later KV revision,
the completed rule re-evaluates, sees `outcome` still true + `phase` now
`executing`, and enters — exactly once. **Do NOT** fold the success-check
into the `dispatched→executing` rule; the marker is durable and the
re-evaluation is automatic. This stays an OPEN ITEM only as a **4a
production-wire contract test** ("success marker stamped before `executing`,
the later phase revision completes the run exactly once") — if that test
ever disproves the beta.102 re-eval behavior, THEN fold the check in.

**H. Slicing (architect-narrowed + Coby review — conditional Go on 4a).**
- **4a (subscriber + the two clean transitions ONLY):** subscriber wiring +
  the `agent-run` pack:
  - `dispatched→executing` — a handoff-marker rule (coordinator loop,
    `rule.spawned_task` + `agent.run.entity_id` → `agent.run.handoff` on the
    run entity) + a run-entity rule (`agent.run.handoff` + `phase==dispatched`
    → `executing`). Confirmed-handoff, publish-failure-safe (§D, P1).
  - `executing→completed` — the 3 reviewer/CBG-**approved** rules stamp
    `agent.run.outcome=success` on the run entity + a run-entity rule
    (`agent.run.outcome==success` + `phase==executing` → `completed`).
    Decoupled from `respond_direct` (§E, P1).
  - Both run-entity rules **phase-guarded** (§G); the marker stamps carry the
    run-less-chat guard (require `agent.run.entity_id`).
  - Deliberately EXCLUDES `executing→failed` (coverage holes, §E) and
    `awaiting_approval` (re-scoped, §E). Isolates the D3 race (§D) + the
    early-marker race (§G2) as the two things 4a proves.
  - **Gate:** go-reviewer + **all 3 pack mock journeys green with a DIRECT
    `agent.run.phase` assertion (`completed`, not `failed`/`dispatched`)** —
    mock is the positive race detector — + a **real-LLM research smoke** for
    the slow-path timing + **failure-injection / state-machine tests (Coby
    review P2):** (a) marker-before-`executing` (success/handoff marker lands
    while `dispatched` → must still reach the right terminal, not stick); (b)
    duplicate terminal marker (second `completed` is `ErrTerminalPhase` →
    must be a guarded no-op, NOT an ERROR line); (c) restart mid-run (durable
    consumer resume + transition replay); (d) publish-fail-after-mint (run
    stays `dispatched` → D3 fails it, NOT an `executing` zombie). Per
    `feedback_state_machine_testing` — mock journeys prove happy ordering,
    these prove state-machine safety.
- **4a′ (failure phase, its own slice) — SHIPPED 2026-06-10:** `executing→failed`.
  - **Run-entity transition:** `agent-run/04-executing-to-failed.json`
    (`agent.run.outcome==failed` + `phase==executing` → `failed`), the
    symmetric twin of rule 03.
  - **Non-coordinator role failures → run-failed** (the bigger zombie class):
    per-pack `…-loop-failed-run-outcome` rules (`research/09`,
    `autoresearch/12`+`13`, `dev-via-test/09`+`10`) stamp
    `agent.run.outcome=failed` on the run entity with the same per-pack anchor
    as the success path (research = `agent.run.entity_id`; autoresearch/dev-via-test
    split first-pass `run_scope=new` children onto `agent.run.entity_id` vs
    run-entity-descended roles onto `lineage.run-loop-entity-id`). The existing
    `…/08`/`…/11` `chain.paused.marker` rules (operator surface) are left
    intact, now widened to `[failed, truncated]` to match.
  - **Coordinator-role failures → run-failed:** `agent-run/05` (anchor
    `agent.run.entity_id`) + `06` (`lineage.run-loop-entity-id`), fenced on
    lineage presence (`length_eq 0` / `length_gt 0`, the proven 02-vs-02g
    split). These cover the executing-flow coordinators that carry a readable
    anchor.
  - **Audit conclusion on the role lists:** the budgeted execute roles
    (`autoresearch-execute`, `dev-via-test`/`ralph`) are correctly EXCLUDED —
    a budgeted loop-failure keeps the run `executing` (04b counter / coordinator
    →ask_user). Pinned by `TestAgentRunPack_BudgetedRolesExcludedFromFailedStamp`.
  - **Scope boundary → 4b (deliberate, not silent):** the run-entity-descended
    `ask_user`/`needs_clarification` recovery coordinators (`autoresearch/10`
    non-baseline; dev-via-test `02e`/`02f`/`07b`/`07e` re-plan branches) carry
    NO readable run anchor and would hang `executing` on involuntary failure.
    Their `action_allowlist ⊆ {respond_direct, ask_user}` — pure
    human-in-the-loop delivery whose run-phase semantics (and a *failed*
    delivery's terminal) are 4b's design (Q2). 4b threads their anchor as part
    of that work. The boundary is pinned structurally by
    `TestAgentRunPack_CoordinatorSpawnCoverage` — every coordinator-spawn rule
    is classified post-approval / anchor-covered / deferred-4b, and an
    unclassified new rule fails the test (the adversarial review's anti-silent-
    zombie guard). `cancelled` (deliberate abort) is also 4b, not 4a′.
- **4b — run-level CLARIFICATION POLICY (reframed 2026-06-10).** The product
  requirement is a per-run/per-deployment policy, not "full-auto":
  - **interactive (default):** the coordinator MAY pause and `ask_user` when
    user intent is the missing dependency.
  - **autonomous:** the coordinator MAY NOT block on the user — it resolves
    ambiguity by an explicit assumption, narrow/retry, or a limitation reply.
  Q2 RESOLVED (see §I.2): `ask_user`/`needs_clarification` stamp NO run-phase
  transition; the run stays `executing`. (A genuine "pause into
  `awaiting_approval` + resume" needs reply-correlation that does not exist
  today — deferred to 4b-2.) Phasing:
  - **4b-1 (proceedable now):** (a) thread a readable run anchor
    (`run-loop-entity-id` from `$entity.triple.lineage.run-loop-entity-id`)
    onto the deferred recovery coordinators (`02e`/`07b`/`07e` single-add;
    `autoresearch/10` + `dev-via-test/02f` length-fence SPLITS, mirroring
    12/13 + 02/02g) so their INVOLUNTARY failure routes to the shipped
    `agent-run/06` — after this `deferred_4b` is EMPTY (zombie hole closed),
    update `TestAgentRunPack_CoordinatorSpawnCoverage`. (b) the SemTeams
    clarification-policy config mode + a coordinator persona variant — BLOCKED
    on the upstream primitive **semstreams#239** (a config-level decide-action
    policy applied uniformly to ALL coordinator tasks incl. the front-door,
    which carries no `action_allowlist`). The product-shell overlay
    alternative (~12 `ask_user`-stripped rule variants) is rejected as
    ADR-029 accretion; `autonomous` maps to the framework primitive disabling
    the blocking `ask_user` decide action.
  - **4b-1 SHIPPED.** (b) the clarification-policy config landed as **4b-1b**
    (#208, 2026-06-11) — semstreams#239 shipped upstream as
    `agentic-tools.restricted_decide_actions`, wired + validated by the
    `clarification-autonomous` mock journey. (a) the anchor-threading landed as
    **4b-1a** (2026-06-11): `autoresearch/10` split → `10` (baseline,
    **anchor_inherit**→`agent-run/05`) + `10b` (propose/execute/synthesize/
    reviewer, **anchor_threaded**→`agent-run/06`); `dev-via-test/02f` split →
    `02f` (first-pass Lisa, **anchor_inherit**) + `02f-replan` (re-plan Lisa,
    **anchor_threaded**); `02e`/`07b`/`07e` single-add thread. **Correction to
    the plan sketch above:** first-pass Lisa (`02f`) and the `autoresearch`
    baseline (`10`) are `run_scope=new` roles that carry a bare `agent.run`, so
    their spawned coordinator INHERITS `agent.run.entity_id` → they are
    `anchor_inherit`→rule 05, NOT threaded→06 (verified in `actions.go`/
    `graph_writer.go`). The governing principle: **thread iff the firing role
    lacks a bare `agent.run`** (run-entity-descended roles); inherit otherwise.
    `deferred_4b` is now EMPTY; `TestAgentRunPack_CoordinatorSpawnCoverage`
    gained an `anchor_inherit` invariant + sibling-sync + reviewer-spawn-thread
    pins. Two mock journeys prove anchor resolution end-to-end on mock-LLM:
    `run-failed-coordinator` (threaded → rule 06) and
    `run-failed-coordinator-inherit` (inherit → rule 05). An adversarial
    anchor-reachability review (7 skeptics, default "zombie") confirmed every
    recovery coordinator resolves to the correct `chain.execution` run entity.
  - **4b-2:** the interactive PAUSE — `executing→awaiting_approval` on
    `ask_user` + reply-correlation (carry the asking-run-id, re-anchor the
    reply coordinator, resume `awaiting_approval→executing`). Its own slice.

    **DESIGN SPIKE (2026-06-11, architect-reviewed) — facts resolved:**
    - The lifecycle GRAPH needs no change: `executing⇄awaiting_approval` edges
      are already legal (`agentrun.go:45-52`). 4b-2 reuses `awaiting_approval`
      (human DECISION: not a new `awaiting_user` phase) — the UI disambiguates
      4b-2's clarification (`coordinator.user_question`, no `pending_approval`)
      from 4c's tool-gate (`pending_approval` present).
    - **`reply_to` does NOT re-anchor to the run** (corrects the "no
      reply-correlation today" framing): `reply_to` re-uses the loop-id STRING
      but `CreateLoopWithID` (`agentic-loop/state.go:132-156`) OVERWRITES the
      loop entity with a fresh one — no continue-path, RunID unset, run triples
      not re-stamped. So the reply lands run-orphaned; net-new run-anchor
      threading IS required (resolved below).
    - **Autonomous mode gates 4b-2 for free**: under
      `restricted_decide_actions:["ask_user"]` the framework rejects
      `decide(ask_user)` BEFORE the `coordinator.decision.next_action=ask_user`
      triple is stamped (`decide.go:307-322`), so every 4b-2 rule (all gated on
      that triple) is structurally inert in autonomous mode. No rule-level gate
      needed; pinned by a contract assertion.
    - **Scope** (human DECISION): backend-first. PR-1 = pause; PR-2 = resume +
      the upstream field; UI (surface question + reply affordance) is a deferred
      follow-up (operators verify via `nats sub`/run-entity polling meanwhile,
      per `03-ask-user.json`'s `open_followup`).

    **PR-1 (pause, product-shell only, no upstream):** the in-run `ask_user`
    pauses the run. Recovery coordinators that `ask_user` within a run carry the
    run anchor as EITHER `agent.run.entity_id` (inherit) OR
    `lineage.run-loop-entity-id` (threaded) — the 4b-1a split — so the pause
    marker is an anchor PAIR mirroring 05/06: `agent-run/07-ask-user-pause-run-anchor`
    (`agent.run.entity_id != ""` + lineage `length_eq 0`) +
    `08-ask-user-pause-lineage-anchor` (lineage `length_gt 0`), each stamping
    `agent.run.clarification_pending=$entity.instance` on the run entity.
    `09-executing-to-awaiting-on-clarification` is the run-entity transition
    (`clarification_pending != ""` + `phase==executing` top-level guard →
    `awaiting_approval`), modelled on rule 02. The front-door `ask_user` (no run
    minted) does NOT pause — there is no run; the run-anchor guard makes it a
    no-op. PR-1 is strictly more honest than today (was `executing`-forever on
    ask_user, now `awaiting_approval`-forever until PR-2's resume).

    **PR-2 (resume): ACTIVATED on semstreams beta.106 (2026-06-12); upstream U1
    semstreams#256 SHIPPED via PR #261.** Two halves, both now landed:

    - *Product-shell slice (rules `10`/`11`):* rule `10`
      (`10-clarification-reply-resume-marker`) fires on the reply coordinator loop
      — gated on `agent.run.entity_id != ""` (the run anchor #256 threads onto the
      reply) + `agent.loop.reply_to != ""` (the reply discriminator) — and
      stamps `agent.run.clarification_resumed` on the run. Rule `11`
      (`11-resume-awaiting-to-executing`) fires on the run entity
      (`clarification_resumed != ""` + `phase==awaiting_approval` top-level guard)
      and does, IN ORDER: `lifecycle_transition→executing`, remove
      `clarification_pending`, remove `clarification_resumed`. Single marker rule
      (no 07/08-style split) because #256 gives the reply loop `agent.run.entity_id`
      directly. **Marker-clear is bounce-proof WITHOUT an atomic-write primitive
      (improves on the original on_exit sketch):** rule `09` gained a
      `clarification_resumed length_eq 0` guard, and rule `11` removes
      `clarification_pending` BEFORE `clarification_resumed` — so at every
      intermediate KV revision rule `09` is blocked by either marker, no
      pause↔resume bounce regardless of re-eval granularity. Pinned by
      `TestAgentRunPack_ClarificationResume{Marker,Transition}` +
      `TestAgentRunPack_PauseResumeBounceGuard` + the transition/wiring tests.
      Both trigger triples are stamped at SPAWN by `buildSpawnIdentityTriples`
      whenever a reply carries `run_id` + `in_reply_to`, so the resume fires on loop
      creation (before any LLM call).
    - *Upstream U1 (semstreams#256 → PR #261, beta.106):* "make the HTTP reply path
      resumable". The pre-fix reply branch dropped `RunID` and carried no reply
      discriminator. #261 threads two reply-branch fields: **Thread 1** the run
      anchor (`HTTPMessageRequest.run_id → UserMessage.RunID → TaskMessage.RunID` →
      existing `SetRunID`/`buildSpawnIdentityTriples` stamps `agent.run.entity_id`)
      + **Thread 2** the reply identity (typed `in_reply_to → TaskMessage.InReplyTo`
      → new `agent.loop.reply_to` triple, a 6-part loop entity ref via
      `agvocab.LoopReplyTo`). The maintainer DELIBERATELY chose the typed predicate
      over a third `lineage.*` overload — so rule `10`'s discriminator was swapped
      `lineage.clarification-reply` → `agent.loop.reply_to` (the documented one-line
      change; the lineage-namespace caveat is gone). #261 also consolidated the two
      byte-identical task-build sites into `buildTaskMessage` (the drift source).
    - *Behavioral mock journey — LANDED with activation.* The `clarification-resume`
      journey (`ui/e2e/agentic/clarification-resume.spec.ts` +
      `test/fixtures/journeys/clarification-resume.yaml`) drives the resume FOR REAL:
      it reuses the pause flow, then POSTs the operator's answer to the dispatch
      `/message` endpoint with `run_id` + `in_reply_to` read from the paused run's
      triples (the same direct-API pattern `tool-approval-gate` uses; no seeding,
      no triple-write seam needed). DECISIVE assertion:
      `awaiting_approval→executing` + both markers cleared + no re-pause (the run
      stays `executing` — a plain coordinator `respond_direct` stamps no
      `agent.run.outcome`, so there is no completion; the journey proves the resume
      MECHANIC, not work completion). The former `..._BlockedOnUpstream` Skip gate
      was retired.
    - *Remaining (separate slice):* a production human-facing reply affordance —
      no UI surface yet renders `coordinator.user_question` with a free-text answer
      box that POSTs the anchors. Deferred to the UI thread (it pairs with surfacing
      `awaiting_approval`).
  - **cancel (deferred, narrow):** `executing→cancelled` fires ONLY on an
    explicit abandon, NOT on cap/budget exhaustion (those route to `ask_user`
    and stay `executing`). Likely home is an UPSTREAM widening of the D3 guard
    to the executing phase (it only covers `dispatched` today), not a
    product-shell cancel subscriber. Its own slice. NOTE (go-reviewer N2): a 4b-2
    run that pauses on `ask_user` and is NEVER replied to parks in
    `awaiting_approval` with no terminal-timeout — an abandoned-clarification run
    is this cancel/timeout slice's job (tracked here, not a 4b-2-PR-1/PR-2 blocker).
- **4c:** the real `approval_required` tool-gate → `awaiting_approval`
  (NOT the CBG automated reviewer, which stays `executing` — §E).
- Then Phase 5.

**I. Open questions.**
1. **D3 race** — empirical result; upstream ask ("D3 must not fire on
   `CategoryLoopCompleted`") if it bites (the watch-item).
2. **`ask_user` phase — RESOLVED (2026-06-10).** `ask_user` and
   `needs_clarification` stamp NO run-phase transition; the run stays
   `executing`. A genuine `awaiting_approval` pause was rejected for 4b-1:
   there is no reply-correlation today (the user's answer arrives as a fresh,
   unlinked `user.message` → brand-new coordinator loop — `agentApi.ts`
   sends only `{content}`, `user.response.*` is a fire-and-forget OUTPUT
   port), so a real `awaiting_approval→executing` resume is net-new plumbing,
   and `awaiting_approval` is reserved for the 4c tool-gate. The "waiting on
   you" affordance is delivered out-of-band from the existing
   `coordinator.user_question` audit triple + a UI badge, NOT a lifecycle
   transition. A true pause is deferred to 4b-2 (gated on first building
   reply-correlation). Whether `ask_user` is even AVAILABLE is the run-level
   clarification policy (interactive vs autonomous, §H 4b), enforced by the
   upstream decide-action policy primitive (semstreams#239).
3. **Marker vocabulary** — finalize the predicate names (`agent.run.handoff`,
   `agent.run.outcome`) + the grammar-collision audit on the `agent.run.*`
   tokens.
4. **Handlers in 4a?** — register a thin `MilestoneHandler` now, or wait
   for Phase 5's stamper re-platform. (Lean: none in 4a — the two
   transitions are pure rules; handlers arrive with the Phase-5 re-platform.)
5. **Phase-guard re-evaluation — RESOLVED (Coby review), retained as a 4a
   test only.** beta.102 evaluates every entity-state rule on each KV
   revision (`message_handler.go:245`), and the stateful evaluator fires
   `on_enter` on a prior-false→current-true flip (`stateful_evaluator.go`),
   so the durable `agent.run.outcome=success` marker DOES re-drive
   `completed` on the later `dispatched→executing` revision (exactly once,
   via the stale-replay guard). No design change; do NOT fold the success-
   check into `dispatched→executing`. Keep open ONLY as a 4a production-wire
   contract test ("success marker before `executing` → later phase revision
   completes exactly once"); revisit only if that test disproves beta.102.

**Architect review (2026-06-09):** spike reviewed against upstream
beta.102. Verdict: architecture sound (substrate `agent-run` pack,
firing-on-run-entity, 2-step marker, dual-write); **conditional Go on a
narrowed 4a** with the four fixes folded in above (phase-guards + §G
rewrite; `executing→failed` split to 4a′; direct `agent.run.phase`
journey assertions + mock-as-detector; upstream ask reframed). §B/§C/§F
and the `executing→completed` convergence confirmed sound; multi-iteration
runs verified to stay `executing` correctly.

**Coby review (2026-06-09):** read the spike + architect findings, raised
two P1s (gate condition) + a P2, now folded in: **(P1a)**
`dispatched→executing` triggers on confirmed handoff (`rule.spawned_task`,
post-publish-success), NOT bare mint — publish-failure now leaves the run
`dispatched` for D3 to fail, not an `executing` zombie (§D). **(P1b)**
`executing→completed` is driven by an explicit `agent.run.outcome=success`
stamped by the reviewer/CBG-approved rules, NOT bare `respond_direct` —
the limitation `respond_direct` paths (research/06, autoresearch/10) no
longer mis-complete a failed run (§E). **(P2)** the 4a gate adds
failure-injection / state-machine tests (marker-before-executing, duplicate
terminal, restart mid-run, publish-fail-after-mint) — mock journeys prove
ordering, these prove state-machine safety (§H). Verdict: **conditional Go
on 4a after the two P1s are folded in** — done.

**Coby review round 2 (2026-06-09):** two wording/semantics corrections on
the early-marker race, **no architecture blocker.** (1) The early success
marker does NOT cause an illegal `dispatched→completed` attempt — IF the
phase guard is a TOP-LEVEL rule CONDITION (not an action `when`), the rule
is simply not-matching while `dispatched`; §G/§G2 rewritten to make the
condition-vs-`when` distinction load-bearing (an action-`when` phase guard
WOULD lose the transition — the real footgun). (2) Q5 RESOLVED positively
by beta.102: the processor re-evaluates every entity-state rule per KV
revision (`message_handler.go:245`) + fires `on_enter` on a
false→true flip (`stateful_evaluator.go`), so the durable success marker
re-drives `completed` on the later `dispatched→executing` revision. Do NOT
fold the success-check into `dispatched→executing`; keep Q5 open only as a
4a production-wire contract test ("success marker before `executing` →
later phase revision completes exactly once"). **Net verdict: no new
blocker; 4a is Go once built with the top-level phase guard.**

### Phase 5 — Retire the hand-rolled layer
- The stampers + `subscriber.go` were already retired in Phase 4a (the
  `chain.*` milestone projections are gone; the agent-run substrate owns
  run-phase authority).
- **PARTIAL (2026-06-11, `chore/adr-053-phase5-dead-chain-cleanup`):** deleted
  the genuinely-dead survivors — `lineage_reader.go` (no production consumer
  since Phase 3c retired emit_plan's read; `AnchorFromMetadata`/`AnchorRoleKeys`/
  `ReadChainFor` all unreferenced) AND `predicates.go` in full (all 42 `Predicate*`
  constants were orphaned vocabulary for the RETIRED chainmode/phasevalidator/
  chainstall/needs-review/ADR-040/ADR-041 machinery — zero external references;
  the live `chain.paused.*`/`chain.decision.*` predicates live in `chainpause/`,
  unaffected; `task schema:generate` is a no-op after removal). So "keep
  predicates.go vocabulary" in the original sketch was wrong — it was dead.
- **DONE (2026-06-11, `feat/adr-053-phase5-retire-resolver`):** semstreams#250
  shipped in **beta.105** — the framework resolves the run/chain entity at
  dispatch and carries it on the execution context:
  `ToolCall.Metadata[agent.run_id]` (bare root loop-id) + `[agent.run_entity_id]`
  (6-part), `LoopFailedEvent.RunEntityID` / `LoopCompletedEvent.RunEntityID` on
  the wire, and `agentic.TryChainExecutionEntityID` as the non-panicking
  reconstructor. The ancestry-walk `Resolver` is retired:
  - NEW `cmd/semteams/runanchor/` — `Anchor(call, org, platform)` reads the
    metadata anchor (reconstructs the 6-part entity from the bare runID when only
    `run_id` is present); `ChainEntityID` resolves in precedence order
    related_loops["chain-entity-id"] pin → run anchor. Replaces the deleted
    `chain.ResolveChainEntityID` / `chain.IDResolver` / `chain.ChainEntityRoleKey`.
  - `chainbash` reads the **bare** runID and rewrites `Metadata["task_id"]` to it
    (NOT the 6-part entity — the sandbox uses task_id as a worktree dir name and
    `AttestationRunner` re-prefixes; a dotted id double-prefixes).
  - `requestsandbox` + `querysandboxattestation` use `runanchor.ChainEntityID`.
  - `chainpause.Pauser` reads `ev.RunEntityID` straight off the failure event;
    `chainpause.DecisionHandler` — the operator HTTP path with only a
    `failed_loop_id` (no event, no ToolCall) — does a SINGLE graph read of the
    failed loop entity's `agent.run.entity_id` via the surviving
    `chain.NATSEntityReader` (using `TryLoopExecutionEntityID` so untrusted dotted
    input returns HTTP 400, not a panic — the guard the deleted
    `chain.ValidateLoopID` used to own).
  - Deleted `chain/resolver.go` + `chain/entity_resolver.go` + their tests; what
    survives in `chain/` is the single-entity read (`NATSEntityReader` /
    `EntityTripleReader`, now in `entity_reader.go`). Net **−547 LOC**.
  - go-reviewer pass caught the dotted-`failed_loop_id` panic regression (now
    fixed + tested). Build, `-race`, lint, schema (zero diff), contract, and the
    chain integration test (live NATS) all green.

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
