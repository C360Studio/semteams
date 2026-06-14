# Governed-SKG Readiness (ADR-054/055/056/057)

Tracking doc for the semstreams "governed Semantic Knowledge Graph" pivot and
its impact on SemTeams rule packs. Predict-then-verify: this is the **hypothesis
map** of what our rule packs will do under the new contract. The live
observe-only metrics (named below) are the **verification gate** — a metric read
we did not predict is a broken filter, not a quiet system.

- Authored 2026-06-14. Status: pre-bump (we are on `v1.0.0-beta.108`, which
  carries only the observe-only substrate; the breaking flips are forthcoming).
- Upstream ADRs: [`055`](../../semstreams/docs/adr/055-graph-write-intent-taxonomy.md)
  (envelope-on-birth, must-exist flip), [`056`](../../semstreams/docs/adr/056-authoritative-semantic-state.md)
  (predicate-group ownership, **Accepted** 06-13),
  [`054`](../../semstreams/docs/adr/054-semantic-indexing-eligibility.md)
  (indexing profile), [`057`](../../semstreams/docs/adr/057-cryptographic-provenance.md)
  (signing — scope stub, blocks nothing).

## The load-bearing finding

**Every SemTeams rule write is on the legacy bare-triple mutation lane, and none
can declare ownership.** The rule engine implements all three triple actions on
top of the `tripleMutator`, whose subjects are `graph.mutation.triple.add` /
`.remove` (`processor/rule/triple_mutator.go:17-18`). They are **not** uniformly
append-evidence — that earlier shorthand was wrong:

| Rule action | Path | Graph-ingest semantics | Ownership-declarable? |
|---|---|---|---|
| `add_triple` | `triple.add` (`actions.go:624`) | append; auto-vivifies today, **must-exist after ADR-055** (`component.go:1826`, `Version:0` branch ~`:1850`) | No |
| `remove_triple` | `triple.remove` (`actions.go:709`) | **CAS destructive** — `UpdateWithRetry` removes all triples for the predicate (`component.go` `RemoveTriple`, ~`:1986`) | No |
| `update_triple` | `triple.remove` then `triple.add` (`actions.go:883`, `:895`) | **non-atomic remove-plus-add**, two revisions, no `ExpectedRevision` | No |

So `add_triple` is append/evidence-like; `remove_triple` and `update_triple` are
destructive-but-ownerless. There is no rule-side path to the `update_with_triples`
*replace-owned* / *cas-transition* lane — which is why we register no owners (and,
today, *cannot*).

Consequence — most of ADR-056 does not touch us:

| ADR-056 mechanism | Lane it gates | Applies to our rule packs? |
|---|---|---|
| Owner-token write lease (`ErrorCodeOwnerLeaseStale`) | `update_with_triples` / `create_with_triples` | **No** — bare-triple / unowned writes carry no token (Decision 2) |
| Cross-process overlap rejection (`OWNER_CLAIMS`) | registered owning claims | **No** — we register no owning claims |
| Decision-5 unregistered-write lint/metric | `update_with_triples` call sites | **No** — we never call that lane |
| `ForeignEdgeClaim` / T2-seam reject | multi-subject Graphable + mutation batches | **No** — bare `triple.add` is "a direct ownership-checked write to its subject, not a foreign-edge producer" (Decision 4 lane-independence note) |
| **ADR-055 must-exist flip** | `triple.add` / `add_batch` lose auto-vivify | **YES — this is our entire exposure** |

So the only question that matters for every rule write is: **does the target
entity exist when the write fires?**

## Verification — what each tag can actually prove

The signals split by tag. **Crucial:** two of the "predicted-zero" metrics are
**traps on the observe-only tag** (beta.109) — a zero there is a broken-filter
reading, not a confirmation. Verified against `adr-056-4c-pre-1` @ `15b1cd46`.

### beta.109 (observe-only) — the first integration pass

What beta.109 genuinely proves (real, readable signals):

1. **Rule packs run unchanged** — our mock-LLM e2e journeys + a real-LLM smoke
   pass on the new pin. The integration premise.
2. `semstreams_graph_ingest_foreign_edge_unclaimed_total{message_type,predicate}`
   (`component.go:144`, shipped #272) — predicted **zero for our rule writes**.
   The one *real* predicted-zero ownership metric on this tag. Non-zero ⇒ the
   projection seam is classifying our run-entity stamps as foreign edges ⇒ we'd
   need `ForeignEdgeClaim`s.
3. `semstreams_graph_ingest_mutation_rejections_total{subject,reason}`
   (`component.go:108`) — the **pre-flip baseline**: predicted **no new**
   validation / CAS / create-or-fail rejections attributable to our packs. This
   *does* cover our `remove_triple` / `update_triple` halves (already must-exist —
   they would reject pre-flip if a target were missing).
4. **Lifecycle ownership-overlap WARN logs** (`manager.go:173`) — overlap is
   *logged, not metered* (observe-only). Predicted: none referencing our entities
   (we register no owners; only the framework Manager does).
5. **Origin-first ordering** — the actual must-exist risk — is proven **only** by
   the per-pack **e2e anchor-before-marker assertions** below (flip-independent),
   NOT by a metric. See the trap note.

### beta.109 verification — RESULTS (2026-06-14, GREEN)

Ran with the observe-only substrate ACTIVATED (`cmd/semteams/main.go` now calls
`ownership.EnsureBuckets` + `Manager.AttachOwnership` + binds the loop-execution
`projection.Contract`). Vehicle: the `run-failed` mock-LLM journey (mints an
AgentRun, drives executing→failed, stamps `agent.run.outcome=failed` on the run
anchor). Evidence from the live stack:

- **Clean boot, substrate active.** `OWNER_CLAIMS` + `OWNER_PRESENCE` buckets
  created; `agent-run` workflow registered with `ownership_attached:true`; the
  `OWNER_CLAIMS` epoch holds BOTH owners with disjoint patterns (overlap check ran,
  passed): `agent-run` → `*.*.agent.chain.execution.*` (phase=cas-transition +
  audit=replace-owned); `agentic-loop-graph-writer` →
  `*.*.agent.agentic-loop.execution.*`. **Zero** ownership WARN/ERROR all boot.
- **Runs unchanged.** Journey passed (1 passed, 2.5s); executing→failed driven by
  rule 04 (`last_transition_from=executing`), not the D3 zombie guard.
- **Insulation proven by "ran and found none," not absence.** graph-ingest
  demonstrably processed entities (`semstreams_graph_ingest_indexing_profile_default_total`:
  `agentic.loop_execution.v1=2`, `lifecycle.harness.v1=1`, `unknown=5`); against
  that, `foreign_edge_unclaimed_total` and `mutation_rejections_total` are absent =
  zero foreign edges classified, zero rejections. Our rule writes tripped no
  ownership machinery.
- **Origin-first confirmed (direct graph read).** The run anchor
  `…agent.chain.execution.loop_089f29e3` is born enveloped
  (`entity.indexing.profile=control`, `lifecycle.harness.v1`), and carries the
  markers stamped onto it: `agent.run.outcome=failed`, `agent.run.handoff`,
  alongside the Manager-owned `agent.run.phase=failed` — the multi-owned-by-
  predicate-group pattern, live. The marker landed on an anchor that already
  existed; no auto-vivified stub.
- **The `unknown=5` envelope-less defaults** are the auto-vivified population the
  ADR-055 must-exist flip will convert; `agentic.loop_execution.v1` defaulting is a
  known upstream ADR-054 Phase-2 registry-seed gap, not ours.

Follow-up (breadth, not blocking): the same substrate-level findings generalize
across packs (all use bare-triple markers on the shared substrate); running the
autoresearch / dev-via-test journeys would confirm the Group-C `update_triple`
(`best.value`) and retry-marker paths empirically.

### The two traps (do NOT read a zero as a pass on beta.109)

- `entity_not_found` on `triple.add` (in `mutation_rejections_total`) is
  **structurally zero pre-flip.** The code is explicit (`component.go:108-110`): it
  surfaces "*when the must-exist flip lands*; until the flip [`triple.add`] silently
  auto-vivifies." So a zero proves nothing about origin-first ordering — that is
  what the e2e (item 5) is for. This metric becomes the real origin-first signal
  **only post-flip.**
- `unregistered_authoritative_write_total` is **not emitted on this tag** (Decision
  5's lint + metric are deferred). Nothing to read — predicting zero for an absent
  metric is a zero-on-healthy-logs reading. Revisit when the enforcement increment
  ships.

### Deferred cleanup (at the enforcement increment)

When the ADR-056 enforcement increment lands (owner-token write lease — presence
liveness becomes load-bearing), scope the observe-only heartbeaters to a
cancellable shutdown ctx cancelled before `natsClient.Close`. Two heartbeaters run
on the app-root `context.Background()` today (the static loop-execution projection
owner + the lifecycle Manager's own, started inside `AttachOwnership`) and stop
only at process exit; the only exposure on the observe-only tag is a benign WARN if
a heartbeat `Put` races shutdown. Bundle this with the pre-existing discarded
milestone-subscriber `stop func` (`wireAgentRunSubstrate` drops upstream's
`defer stopMilestoneSubscriber()`). Reason for deferral: `run()` is at revive's
50-statement function-length cap, so threading `hbCancel` back through it needs an
offsetting refactor not worth the churn while the exposure is benign. (go-reviewer,
2026-06-14, beta.109 wiring pass.)

### Post-flip (the must-exist tag, later)

`entity_not_found` on `triple.add` reading **zero over a bake window** becomes the
real origin-first confirmation; a non-zero names the offending pack + predicate via
labels. The e2e from item 5 is the pre-flip guarantee that this stays zero.

## Classification map

Our rule writes classify on **two orthogonal axes**, and conflating them is what
produced the earlier "all append-evidence" error:

- **Target axis (Group A / B) → must-exist risk.** Whether the write targets the
  firing loop (always exists) or a foreign run/plan/loop-execution anchor (must be
  born first). This is our *actual* exposure to the ADR-055 flip, today.
- **Lifecycle axis (Class 1 / 2) → ownership.** Whether the predicate group is
  write-once append-evidence (genuinely unowned, multi-writer-safe) or clearable
  coordination / current-state (single-writer pack, set→clear or replaced). This
  decides the *future* ownership question and is what the upstream ask is about.

The two are independent: a clearable-class predicate can target the firing loop
*or* a foreign anchor. The `Class` column below carries the lifecycle axis; the
Group heading carries the target axis.

### Group A — target = the firing loop (`subject: null` / trigger entity)

Always exists (it is what the engine is processing). **Zero must-exist risk.**

| Write | Class | Rules |
|---|---|---|
| `chain.paused.marker` / `chain.paused.role` | 1 write-once | `autoresearch/11`, `dev-via-test/08`, `research/08` |
| `coordinator.user_question` / `user_reply` | 1 write-once | `coordinator/03`, `03b` |
| `dev_via_test.execute.outcome` | 1 write-once | `dev-via-test/04a`, `04b` |
| `autoresearch.run.status` (`update_triple` replace) | **2 clearable** | `autoresearch/05` |
| `remove_triple` clears (the clear half of Class-2 groups) | **2 clearable** | `agent-run/11,13`; `dev-via-test/02d,07d`; `autoresearch/05` |

### Group B — target = the run / plan / loop-execution anchor (foreign subject)

Target must be born before the stamp. **The one must-exist invariant to verify.**

| Predicate group | Class | Rules |
|---|---|---|
| `agent.run.outcome` (success/failed) | 1 write-once (mutually-exclusive writers) | `agent-run/05,06`; `autoresearch/08,12,13`; `dev-via-test/07a,09,10`; `research/07,09` |
| `agent.run.handoff` | 1 write-once | `agent-run/01` |
| `autoresearch.experiment.{completed,loop_failed}` | 1 write-once (multi-valued per experiment) | `autoresearch/04a,04b` |
| `research.gather.completed_subtopic` | 1 write-once (multi-valued per subtopic) | `research/03a` |
| `dev_via_test.execute.task_{completed,failed}` | 1 write-once | `dev-via-test/04a,04b` |
| `agent.run.clarification_pending` / `clarification_resumed` (and `approval_pending` / `approval_resumed`) | **2 clearable** (set→clear, ADR-053 HITL) | `agent-run/07,08,10,12` |
| `autoresearch.iteration.pending` (set→clear) | **2 clearable** | `autoresearch/04a,04b` |
| `autoresearch.best.{value,experiment_id}` (`update_triple` replace) | **2 clearable** (single-valued running-max) | `autoresearch/04c` |
| `dev_via_test.{plan,cbg}.retry.pending`, `cbg.retry.{finding,target_task}` (set→clear) | **2 clearable** | `dev-via-test/02c,07c,07a` |

Predicted: must-exist reject stays **zero**, because the anchor is minted before
any terminal/pause marker can fire (the run entity via `Manager.Create`,
enveloped create-or-fail; loop-execution entities via the agentic-loop writer).
True by causal ordering today — but the flip makes it fail-fast, so it earns a
test (below) rather than an assumption.

### The Class-2 (clearable / current-state) set — the ownership axis

Class 2 is the set that motivates the upstream ask. These are **owned coordination
state**, not accumulating evidence: a *single* pack is the only writer, the
predicate group is current-state (presence toggled set→clear, or a value
replaced), and we manage it today with non-atomic `remove` / remove-plus-add and
**zero ownership registration**. Members, across both target groups:

- **ADR-053 HITL pending markers** — `agent.run.clarification_pending` /
  `approval_pending` and their `*_resumed` siblings (set on pause, cleared on
  reply). The most load-bearing Class-2 group.
- **Retry / iteration gates** — `autoresearch.iteration.pending`,
  `dev_via_test.{plan,cbg}.retry.pending`, `cbg.retry.{finding,target_task}`.
- **Replaced current-state** — `autoresearch.best.{value,experiment_id}` (the
  running-max upsert, validated end-to-end on real Gemini) and
  `autoresearch.run.status`. These are `update_triple` = non-atomic remove-then-add
  — the exact ADR-055 §4 smell ("a mutable single-valued predicate may not ride
  the fact-arrival stream").

None of this is *blocked* by the pivot (all stay on the bare-triple lane, all stay
exempt from the lease). The point is the opposite: Class 2 is real rule-pack-owned
state that has no ownership lane and no derivation entry point — which is exactly
the upstream feedback (`governed-skg-upstream-feedback-draft.md`, Findings A + B).

## Per-pack test list (anchor-before-marker)

Testable **today** on mock-LLM — assert the anchor entity is present in
`ENTITY_STATES` at the moment its marker rule fires. Independent of the flip;
the must-exist reject is the production confirmation post-flip.

- [ ] **agent-run** — run anchor (`agent.run.entity_id`) present before rules
  `01` (handoff), `05`/`07` stamp; lineage anchor present before `06`/`08`.
  Drive: dispatch a category (run_scope=new) → assert run entity exists → reach
  terminal/pause → assert `agent.run.outcome` / `clarification_pending` lands.
- [ ] **autoresearch** — run-loop anchor (`lineage.run-loop-entity-id`) present
  before `04a`/`04b` (experiment markers), `04c` (best.value), `08`/`12`/`13`
  (outcome). Drive: spawn autoresearch → first experiment completes → assert
  anchor exists → assert `experiment.completed` + `best.value` land.
- [ ] **dev-via-test** — run-loop / plan anchor present before `02c`, `04a`/`04b`,
  `07a`/`07c`, `09`/`10`. Drive: first-pass Lisa → execute → assert anchor →
  assert `execute.task_completed` / `agent.run.outcome` land.
- [ ] **research** — plan-loop anchor (`lineage.plan-loop-entity-id`) present
  before `03a` (`gather.completed_subtopic`); run anchor before `07`/`09`
  (outcome). Drive: research plan → gather a subtopic → assert anchors → assert
  markers land.
- [ ] **coordinator** — Group A only (`subject: null`); confirm `03`/`03b` write
  to the firing coordinator loop, no anchor dependency. (Smoke-level only.)
- [ ] **ops** — no foreign-subject rule stamp; ops findings are minted by the
  `emit_diagnosis` tool executor (upstream). **Watch-item**, not our test:
  confirm upstream `emit_diagnosis` births its `…ops.diagnosis.finding.{uuid}`
  entities with a semantic envelope (ADR-055 §2) — ops findings are `content`
  profile (ADR-054).

## Upstream feedback

**Canonical text: `governed-skg-upstream-feedback-draft.md`** (Codex-reviewed,
ready to post to semstreams). Summarized here so this doc stands alone; if the two
ever diverge, the draft wins.

1. **Rule packs can't bind `projection.Contract`s** — Decision 6's derivation
   (`pkg/projection/contract.go`) has no entry point for config-level producers.
   Ask: declare contracts beside rule definitions, `Bind` at load under a
   subject-safe owner id `rule-pack.<pack-id>` (`validOwnerID` charset, no hash).
2. **No declared owned-write rule action** — `update_triple` is non-atomic
   remove-plus-add on the bare-triple lane; the Class-2 set (above) has no
   `replace-owned` lane. Ask: one declared replace action emitting
   `update_with_triples` for contract-declared predicates only (not
   `cas-transition` — that stays with the lifecycle `Manager`; `OwnerToken`
   carried once that deferred write-lease field lands).
3. **Flip-gate framing for product rule packs** — gate the must-exist flip on
   `entity_not_found` rejects (zero over a bake window) + product e2e, **not**
   `foreign_edge_unclaimed_total`. Plus the B1 sequencing dependency: our
   `lineage.*` markers target loop-execution entities whose own birth is ADR-055
   Wave 2.
4. **Object-token semantics** — spec `$entity.instance` vs `$entity.id` vs
   `$entity.triple.*`; normalize `agent-run/07,08` (the pinned marker-object
   asymmetry that once broke run-resume).

## Posture

- **Do not migrate yet.** The substrate is observe-only; the must-exist flip is
  gated upstream on hatch-empty + a green crash-recovery test. Premature
  `RegisterOwner` calls would be churn — and we have nothing to register anyway
  (no rule write reaches the owned lane; the Class-2 set *would* register, but the
  lane to do so does not exist yet — that is feedback item 1/2).
- **On beta.109 (the first integration pass):** confirm rule packs run unchanged
  (mock-LLM e2e + real-LLM smoke), watch the *real* observe-only signals
  (`foreign_edge_unclaimed_total` = 0; `mutation_rejections_total` baseline = no
  new rejections; no lifecycle overlap WARNs), and run the per-pack e2e for
  origin-first ordering. **Do not** read `entity_not_found`-on-`triple.add` or
  `unregistered_authoritative_write_total` as passes — they are pre-flip traps
  (see Verification).
- **Do raise the feedback now** — done: C360Studio/semstreams#278 (`enhancement`).
