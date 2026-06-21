# Governed-SKG Readiness (ADR-054/055/056/057)

Tracking doc for the semstreams "governed Semantic Knowledge Graph" pivot and
its impact on SemTeams rule packs. Predict-then-verify: this is the **hypothesis
map** of what our rule packs will do under the new contract. The live
observe-only metrics (named below) are the **verification gate** — a metric read
we did not predict is a broken filter, not a quiet system.

- Authored 2026-06-14. Status: **on `v1.0.0-beta.114`** — the BREAKING ADR-055
  must-exist flip (beta.112 #300) AND the ADR-056 owner-token write-lease (PR-1..5)
  are LANDED + ADOPTED with **FULL ownership-contract compliance**
  (`enforce_owner_lease` ON; see the **beta.113 section** below). Prior:
  observe-only substrate (beta.109, #218) + #278 rule-pack projection-producer
  adoption (beta.110, #219/#220) + beta.111 insulated bump (#221).
- **beta.113 → 114 (2026-06-21): trivial additive bump, ZERO rule-pack delta.**
  beta.114 ships #311 (HTTPClientPort declarative descriptor — component wiring),
  #307 (graph-query bounded `QueryPrefixAll` auto-pager — additive query capability,
  extends beta.113 #303 prefix-discovery), and #308 (docs refresh for the must-exist
  flip). None touch the rule-pack / governed-SKG write surface. Verified clean:
  build/vet/gofmt, `-race` (25 pkgs), lint (0 new), schema-no-drift; `go.mod` hash
  unchanged ⇒ no transitive dep change; the `autoresearch` mock-LLM journey re-passed
  green (born-first + `replace_owned` + owner-lease surfaces re-exercised). The only
  remaining rule-pack migration — the agent-run HITL pair (`clarification_*` +
  `approval_*` → `replace_owned`) — stays deferred: beta.114 does NOT bring the
  subscriber-side owned-write lane that `approval_*` needs. **That lane is now
  tracked upstream as semstreams#313** (filed 2026-06-21, residual to #278, on the
  semstreams backlog). The accurate ask: a reusable Go owned-write helper (a
  `TripleMutator.ReplaceOwned` analog factored out of the rule processor) — the
  binding half already exists (`projection.Bind`/`BindAndHeartbeat`, #280; used by
  the `agentic-loop-graph-writer`, a non-rule/non-Manager Go writer). Migrate the
  HITL pair as a unit once #313 ships.

## beta.113 — must-exist flip ADOPTED + owner-lease ENFORCED (full compliance)

The breaking governed-SKG batch landed (beta.112 #300 must-exist flip + ADR-056
owner-token write-lease PR-1..5 + the ADR-058 boot refactor; beta.113 #303 adds
graph-query discovery, additive). semteams adopted it with **full ADR-056
ownership-contract compliance** — not the "enforcement is off by default, skip it"
half-measure:

- **Producer compliance (token presentation).** `cmd/semteams/main.go` mirrors
  upstream's ADR-058 wiring (ADR-029): `service.WireOwnership` (buckets + Registry +
  loop-execution contract + Manager heartbeater) + Phase-B `NewOwnershipService`
  (static heartbeater) + Phase-B `NewMilestoneService`. Every owner — the
  loop-execution graph writer, the agent-run lifecycle Manager, and the
  `rule-pack.semteams` `replace_owned` producer (via `BindRulePackContracts`) — mints
  + presents its `<owner>#<incarnation>` OwnerToken from the SAME live Registry, so
  its writes match the live lease.
- **Clean shutdown (the fence's liveness dependency).** `service.WireOwnershipShutdown`
  gives the cancel+join (run()-deferred → LIFO before `natsClient.Close`), so a dying
  incarnation releases OWNER_PRESENCE promptly. This closes the previously-deferred
  clean-heartbeat-shutdown item — now load-bearing.
- **Consumer enforcement.** `enforce_owner_lease: true` on graph-ingest in BOTH flow
  configs. The fence is ENGAGED: a stale/superseded incarnation's cached token fails
  the lease check (`pkg/ownership/doc.go`: the reject "protects the rule-pack
  replace_owned producer"). Single-process semteams ⇒ one incarnation in steady
  state ⇒ no false-reject; the only reject window is a redeploy overlap, where
  rejecting the dying incarnation's late write is the DESIRED fence behavior.

**Verified GREEN (mock-LLM, beta.113, enforcement ON):** all four pack journeys
— `autoresearch`, `run-failed`, `research-mvp`, `dev-via-test-replan` — pass with
clean boot, **zero `entity_not_found`** (must-exist flip: born-first holds across
agent-run / autoresearch / research run+plan-loop / dev-via-test) and **zero
`owner_lease_stale`** (every owned write presents a matching token). go-reviewer:
full compliance verified against beta.113 source, 0 Critical/Major. This is the
post-break confirmation #222 asks for.
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

### beta.110 — #278 adoption: autoresearch scalar replaces → `replace_owned`

Upstream shipped our #278 ask in beta.110 (#281 docs / #282 main-side bind /
#283 `replace_owned` + envelope validation). The must-exist flip and the
owner-token lease are NOT in this tag, so Group-B origin-first work, the trap
metrics, and the deferred clean-heartbeat shutdown all stay deferred — beta.110
is purely the rule-pack projection-producer capability, additive / non-breaking
(pin-bump verified: build/vet/`-race`/lint/schema + `run-failed` mock journey).

**What we adopted (first slice — the autoresearch single-valued scalar replaces):**

- `pack_id: "semteams"` on the single substrate rule processor (both flow
  configs). semteams runs ONE rule processor for all category packs (ADR-042
  substrate-singleton), so the framework-level "pack" is the whole substrate
  corpus → ONE owner `rule-pack.semteams`, with per-category ownership carved by
  named `projection_contracts`. This is the upstream multi-contract-per-owner
  design, NOT a per-category processor split (which would fight ADR-042).
- A named `autoresearch.run-projection` contract: a `replace-owned` group owning
  `autoresearch.best.value`, `autoresearch.best.experiment_id`,
  `autoresearch.run.status` on `*.*.agent.chain.execution.*` (the run anchor,
  `agentrun.EntityIDPattern`).
- Rules `autoresearch/04c` (best.value + best.experiment_id) and `autoresearch/05`
  (run.status flip) flipped `update_triple` → `replace_owned` — the atomic owned
  replace-by-(subject, predicate) via `update_with_triples`, closing the
  non-atomic remove-then-add read-between-revisions gap the rules documented.
  (`replace_owned` honors subject-override + object substitution exactly as
  `update_triple` did; only the PREDICATE must be a literal — verified, both
  predicates are literal.)
- Wiring: `cmd/semteams/main.go` now calls `service.BindRulePackContracts` after
  the rule processors are constructed (mirrors upstream §11b, ADR-029);
  `wireAgentRunSubstrate` returns the ownership Registry + Heartbeater so the bind
  reuses the same pair. The executor `ownerID` is set independently by the rule
  processor factory when `pack_id` is present (`SetProjectionOwner`), so writes
  carry owner `rule-pack.semteams` regardless of the bind.

**Framework-alignment posture (CLAUDE.md mandatory review):** this ADOPTS the
upstream #278 primitive (`replace_owned` + `projection.Contract` binding) — the
"exists upstream → use it" case, no product-local fork. The primitive was
requested by us (semstreams#278) and shipped to spec.

**Owner-claim disjointness (verified by design; observe-only at runtime):**
`rule-pack.semteams` claims `{autoresearch.best.value, .best.experiment_id,
run.status}` (replace-owned) on `*.*.agent.chain.execution.*`. The lifecycle
`agent-run` owner claims `{agent.run.phase}` (cas-transition) + the `agent.run.*`
audit/struct fields (replace-owned) on the SAME pattern. The predicate sets are
disjoint → the blessed multi-owner-by-predicate-group pattern, no overlap. (An
overlap would WARN, not brick — `BindRulePackContracts` is observe-only this tag.)

**Known boundary — the executor-seeds-then-rule-owns split.**
`emit_autoresearch_baseline` / `emit_autoresearch_measurement` (Go tool
executors) SEED `best.value` / `run.status` via the bare `add_triple` lane (their
`TriplePublisher` has no owned lane), and the rule's first `replace_owned`
reconciles that seed away. On the observe-only tag this mixed-writer seed is
benign; an executor-side owned-write lane is a SEPARATE upstream gap — now
**filed as semstreams#313** (residual to #278: rule packs got a derivation entry
point in #278; subscribers/executors did not). Tracked, not blocking.

**Structural pin:** `test/contract/governed_skg_replace_owned_test.go` reproduces
the framework's boot-time envelope check (every `replace_owned` predicate ⊆ the
declared replace-owned contract; predicate must be a literal) + a regression pin
that the three migrated predicates stay on the owned lane (vs a silent revert to
`update_triple` / `add_triple`).

**Fast-follow (shipped on top of #219) — coordination gates → `replace_owned`.**
A second slice migrates the cleanly rule-pack-owned clearable coordination state:

- `autoresearch.iteration.pending` (set rules 04a/04b, cleared rule 05) — added to
  the `autoresearch.run-projection` contract. The rule-05 first-action clear stays
  ordered-first so the presence-marker re-entry (semstreams#204) is unchanged; the
  atomic empty-object clear is equivalent to the prior `remove_triple`.
- dev-via-test retry gates — `dev_via_test.{plan,cbg}.retry.{pending,finding}` +
  `cbg.retry.target_task` (set rules 02c/07c, cleared rules 02d/07d/07a) — a new
  `dev-via-test.retry-projection` contract. Validated by the `dev-via-test-replan`
  journey (asserts the retry-finding stamp on the run entity).

Note on the STAMP lane: `replace_owned` is must-exist (no auto-vivify — unlike the
`add_triple` / `update_triple` it replaces). The migrated stamps all target the run
anchor via `lineage.run-loop-entity-id`, which only resolves once the run entity
exists, so the anchor is always present mid-chain — a no-op in practice, and the
intended ADR-056 semantics (it pre-aligns these writes with the coming ADR-055
must-exist flip). The one consequence: a future refactor that moved a stamp earlier
than run-anchor birth would surface as a logged Error (`entity_not_found`) rather
than a silent auto-vivify — fail-fast, which is the desired direction.

**Still deferred — the agent-run HITL pair (clarification + approval), as a unit.**
`agent.run.clarification_{pending,resumed}` is fully rule-written (ownable today),
BUT its sibling `agent.run.approval_{pending,resumed}` is SET by the `approvalpause`
Go subscriber (`cmd/semteams/approvalpause/`), not a rule — so rule-pack ownership
is the wrong owner for it; the subscriber is the natural owner and needs the
executor/subscriber-side owned-write lane (**filed upstream as semstreams#313**,
residual to #278). The two HITL markers are a coherent, delicate ADR-053 resume
subsystem (rule 11's ordered clears + rule 09's `length_eq 0` bounce guard), so
migrating clarification alone would split the pair across two lanes for marginal
benefit (presence markers have no read-between-revisions gap). Migrate BOTH
together once #313 ships and `approval_*` is ownable.

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

## Per-pack test list (anchor-before-marker) — PROVEN (semteams#222, 2026-06-17)

Each pack's marker→anchor born-first ordering is now ASSERTED in its mock-LLM
journey via `ui/e2e/agentic/born_first.ts` (`assertAnchorBornFirst`). The
assertion derives the anchor from the MARKER's own subject, then asserts that
entity carries its born-first ENVELOPE — a lifecycle/spawn identity predicate the
framework stamps at birth: `agent.run.phase` for `*.*.agent.chain.execution.*`
run anchors (lifecycle Manager, at Create); `agent.loop.role` for
`*.*.agent.agentic-loop.execution.*` loop/plan anchors (agentic-loop graph writer,
at spawn). This FAILS under simulated auto-vivify — a marker-vivified bare stub
carries ONLY the marker, never the birth-stamped envelope. All green on
mock-LLM beta.111.

- [x] **agent-run** — run anchor born-first before `agent.run.outcome` (rule 09).
  `run-failed.spec.ts` Step 4b. GREEN.
- [x] **autoresearch** — run anchor born-first before `autoresearch.best.value`
  (rule 04c). `autoresearch.spec.ts`. GREEN.
- [x] **dev-via-test** — run anchor born-first before `dev_via_test.plan.retry.finding`
  (rule 02c). `dev-via-test-replan.spec.ts`. GREEN. All dev-via-test markers target
  the SAME `chain.execution` run anchor (via `lineage.run-loop-entity-id`), so the
  replan proof covers the pack. ⚠️ The dev-via-test HAPPY-PATH journey
  (`dev-via-test.spec.ts`) is currently red on a PRE-EXISTING, unrelated issue —
  `sandbox.attestation.ready` is not landing (deterministic across 3 builds
  2026-06-17, survives a 30s poll; was a documented intermittent flake, now
  consistent). NOT a born-first concern; flagged for separate investigation (env
  vs. a sandbox-provisioning/MockRunner regression).
- [x] **research** — TWO anchors. Run anchor born-first before `agent.run.outcome`
  (rule 07); plan-loop anchor born-first (`agent.loop.role`) before
  `research.gather.completed_subtopic` (rule 03a — the LOOP_ANCHOR case).
  `research-mvp.spec.ts`. GREEN.
- [x] **coordinator** — Group A (`subject: null`): rules `03`/`03b` write to the
  FIRING coordinator loop (always exists — it is the entity being processed). No
  foreign-anchor stamp → zero must-exist risk BY CONSTRUCTION. Exercised by the
  `clarification-*` journeys; no born-first assertion needed.
- [ ] **ops** — UPSTREAM watch-item (NOT our test): confirm semstreams'
  `emit_diagnosis` births its `…ops.diagnosis.finding.{uuid}` entities with a
  semantic envelope (ADR-055 §2; `content` profile). Our ops rules stamp no
  foreign-subject marker. Flag for upstream.
- [x] **Observe-only signals clean** — backend logs clean during live runs (fresh
  scrape 2026-06-17): zero ownership-overlap WARNs, zero foreign-edge /
  mutation-rejection / ERROR lines. Metric counters (`foreign_edge_unclaimed_total`
  = 0, `mutation_rejections_total` baseline) were read explicitly in beta.109
  (PR #218) and are unchanged by construction — our writes never hit the
  foreign-edge lane that beta.111's routing modified. (`/metrics` is not exposed on
  a mapped e2e port, so a counter re-scrape in e2e is unavailable; the log-side
  scrape + the beta.109 counter read cover it.)

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
5. **Reusable Go owned-write helper for subscribers/executors** — FILED as
   **semstreams#313** (2026-06-21, residual to #278, on the semstreams backlog).
   #278 gave *rule packs* the owned-write lane; the lifecycle `Manager` already had
   one. A Go *subscriber/executor* still cannot do an owned replace-by-(s,p): the
   only emitter is `TripleMutator.ReplaceOwned` inside `processor/rule`. NOTE the
   binding half is NOT the gap — `projection.Bind`/`BindAndHeartbeat` (#280, authored
   by us) is already general-purpose and used by the `agentic-loop-graph-writer` (a
   non-rule/non-Manager Go writer); `UpdateEntityWithTriplesRequest.OwnerToken` is
   public. Ask: factor a `ReplaceOwned` analog out of the rule processor so any Go
   component holding a bound token can emit `update_with_triples`. Beneficiaries:
   (1) the `approval_{pending,resumed}` HITL markers (set by our `approvalpause`
   subscriber — why the ADR-053 HITL pair can't migrate as a unit); (2) the product
   tool-executor seed-reconcile pattern (`emit_autoresearch_*` seeds
   `best.value`/`run.status` on the bare lane, reconciled by a rule's `replace_owned`).
   Backlog / non-blocking (presence markers don't need CAS; the seed-reconcile is
   benign).

## Posture

- **FULLY MIGRATED + COMPLIANT + ENFORCING (beta.113).** The must-exist flip
  (ADR-055) and the owner-token write-lease (ADR-056 PR-1..5) are LANDED and
  ADOPTED: all rule-pack owned writes register ownership (`rule-pack.semteams`) +
  write via `replace_owned` presenting a live-lease-matching OwnerToken;
  `enforce_owner_lease` is ON; clean cancel+join shutdown released. The Group-B
  origin-first invariant is no longer a prediction — it is verified by the four
  born-first pack journeys reading **zero `entity_not_found`** under the live flip.
  (See the beta.113 section above.) Only the agent-run HITL pair remains deferred
  — its `approval_*` half is subscriber-written, needing the executor/subscriber-
  side owned-write lane, NOT a rule-pack concern.
- **On beta.109 (the first integration pass):** confirm rule packs run unchanged
  (mock-LLM e2e + real-LLM smoke), watch the *real* observe-only signals
  (`foreign_edge_unclaimed_total` = 0; `mutation_rejections_total` baseline = no
  new rejections; no lifecycle overlap WARNs), and run the per-pack e2e for
  origin-first ordering. **Do not** read `entity_not_found`-on-`triple.add` or
  `unregistered_authoritative_write_total` as passes — they are pre-flip traps
  (see Verification).
- **Do raise the feedback now** — done: C360Studio/semstreams#278 (`enhancement`).
