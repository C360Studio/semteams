# semstreams ADR-056 feedback (POSTED)

> **Posted 2026-06-14 as C360Studio/semstreams#278 (`enhancement`):**
> <https://github.com/C360Studio/semstreams/issues/278>. This is the source text;
> the posted body strips this preamble + the appendix's private-repo path. Kept as
> the record. Constructive feedback from the heaviest rule-driven consumer; 056 is
> well-built — this is about one producer class (config-level rule packs) that its
> derivation model doesn't yet reach. Codex-reviewed over two rounds (corrected the
> "all append-evidence" overstatement, the marker taxonomy, the
> `pkg/projection.Contract` ask, owned-action scope, flip-gate metric, owner-id
> charset, `OwnerToken`-is-deferred phrasing, and stale line numbers).

---

**Title:** ADR-056 feedback from semteams — rule packs need projection contracts
and a declared owned-write action

## Who and what

Feedback from **semteams** — the repo your ADR-056 Q2 acceptance fixture probed on
2026-06-13 (the 22 agent-run rules stamping the run entity). We mapped our full
rule-pack exposure to ADR-055 (must-exist flip) + ADR-056 (predicate-group
ownership). We are almost entirely insulated, and the reason we are insulated is
the gap: **rule-engine triple actions live on the legacy bare-triple mutation lane
and cannot declare ownership at all** — so a rule pack that legitimately owns a
coordination predicate group (and we have several) has no place in Decision 6's
derivation model and no atomic owned-write lane.

We believe this generalizes to any rule-driven product on semstreams.

## TL;DR — the asks (opinionated and narrow)

1. **Let rule-pack config declare and `Bind` normal `projection.Contract`s**
   (`pkg/projection/contract.go`) at rule-pack load, under a stable subject-safe
   owner id such as `rule-pack.<pack-id>` (`<pack-id>` constrained to
   `validOwnerID`'s charset `[A-Za-z0-9._=-]`, `glob.go:21` — **not** `:` and
   **not** a `RuleToken` hash; the owner id is compared and keyed as the canonical
   string). No parallel registry — the existing Decision-6 layer already supports a
   logical `Name` and an owner bound at `Bind` time.
2. **Add a single declared `replace-owned` rule action** that emits
   `update_with_triples` (atomic replace-by-(s,p)) **only** for predicates a bound
   contract declares the pack owns. Leave `cas-transition` to the lifecycle
   `Manager` — rules should not become a state-machine substrate.
3. **Frame the ADR-055 flip gate for product rule packs correctly**: the relevant
   signal is `entity_not_found` rejects on `graph.mutation.triple.add` plus
   targeted product e2e proving anchors are born before marker writes — **not**
   `foreign_edge_unclaimed_total` (bare `triple.add` never enters the foreign-edge
   classifier).
4. *(minor)* Spec the `$entity.instance` vs `$entity.id` object-token semantics.

## The framing fact (corrected — verified against code)

Rule-engine triple actions are **all on the legacy bare-triple mutation lane**
(`graph.mutation.triple.add` / `.remove`, `processor/rule/triple_mutator.go:17-18`),
and **none of them can declare ownership**. But they are not uniformly
append-evidence — that earlier shorthand was wrong:

| Rule action | Path | Graph-ingest semantics | Ownership-declarable? |
|---|---|---|---|
| `add_triple` | `triple.add` (`actions.go:624` → mutator `AddTriple`) | append; auto-vivifies today, **must-exist after ADR-055** (`component.go:1826`, the `Version:0` branch ~`:1850`) | No |
| `remove_triple` | `triple.remove` (`actions.go:709`) | **CAS destructive** — `UpdateWithRetry` removes all triples for the predicate (`component.go` `RemoveTriple`, ~`:1986`) | No |
| `update_triple` | `triple.remove` then `triple.add` (`actions.go:883`, `:895`) | **non-atomic remove-plus-add**, two revisions, no `ExpectedRevision` | No |

So `add_triple` is append/evidence-like; `remove_triple` and `update_triple` are
**destructive but ownerless**. The correct unifying statement is "rule actions are
on the bare-triple lane and cannot register as an owner," not "all append-evidence."

Consequence — most of ADR-056 correctly does not touch us today (confirming we
read it right): we **register no owning claims** (so the cross-process overlap
check and the Decision-5 unregistered-write lint have nothing of ours to gate),
and we **never call the owned mutation lane** (`update_with_triples` /
`create_with_triples`), so the owner-token write lease does not apply; bare
`triple.add` never enters the `ForeignEdgeClaim` / T2-seam classifier either. Our
only exposure is the ADR-055 must-exist flip. Good — but the same fact is the gap.

## Our markers are TWO classes, not one (this is what strengthens the ask)

Splitting our rule-written predicate groups by actual lifecycle:

- **Write-once append-evidence** (set once, never cleared; genuinely unowned,
  multi-writer-safe — Decision 2 already exempts these, no change wanted):
  `agent.run.outcome` (terminal scalar, mutually-exclusive writers),
  `agent.run.handoff`, `autoresearch.experiment.{completed,loop_failed}`,
  `research.gather.completed_subtopic`,
  `dev_via_test.execute.task_{completed,failed}`, `chain.paused.{marker,role}`,
  `coordinator.user_{question,reply}`.

- **Clearable coordination / current-state** (set then removed, or replaced — a
  *single* writer pack, presence/value is current-state not accumulating
  evidence): `agent.run.clarification_pending`/`clarification_resumed` and
  `approval_pending`/`approval_resumed` (the ADR-053 HITL machinery —
  set then cleared on reply, `agent-run/07,08,10,11,12,13`),
  `autoresearch.iteration.pending` (set→cleared, `04a/04b`→`05`),
  `autoresearch.run.status` and `best.{value,experiment_id}` (`update_triple`
  replace, `05`/`04c`), `dev_via_test.{plan,cbg}.retry.pending` and
  `cbg.retry.{finding,target_task}` (set→cleared).

The second class is the case for the ask. These are **owned coordination state**:
one pack is the only writer, the predicate group is current-state, and we manage
it today with non-atomic remove/remove-plus-add and zero ownership registration.
`autoresearch.best.value` (a running-max, `autoresearch/04c-execute-promote-best-on-kept.json`)
is the cleanest example, but the HITL `*_pending` markers are the same shape and
more load-bearing.

## Finding A — rule packs can't bind `projection.Contract`s

Decision 6 makes ownership claims **derive** from a registered `projection.Contract`
(`pkg/projection/contract.go:46`) — owner-less shape, owner bound at `Derive`/`Bind`
(`:142`, `:170`), `Groups []PredicateGroup` by `WriteMode`. Manual `RegisterOwner`
is the reserved escape hatch (lifecycle `Manager`; migration scaffolding).

A rule pack is neither, and yet it is a **permanent config-level producer class**
your own Decision-4 note names ("semteams rules → `triple.add`"). The `Contract`
already supports exactly what a rule pack needs — a **logical `Name`** (not only a
payload `MessageType`, `contract.go:47-50`) and an owner bound at `Bind`. What is
missing is the *wiring*: a way for **rule-pack config to declare contracts beside
the rule definitions and `Bind` them at load** under a stable owner id.

**Ask:** support `projection.Contract` declaration in rule-pack config, bound at
rule-pack load via
`projection.Bind(ctx, reg, "rule-pack.<pack-id>", contracts...)` (subject-safe
owner id per `validOwnerID`, above). Two packs claiming the same cell then collide
via `ownership.RegisterOwner` against the live epoch exactly like any other owners
— no parallel registry, no drift.

## Finding B — no declared owned-write rule action

Even with a bound contract, a rule has no runtime action that performs a
`replace-owned` write. `update_triple` is non-atomic remove-plus-add on the
bare-triple lane (above): a reader between the two revisions sees no value, and
two writers race with no conflict detection. That is the ADR-055 §4 smell, and the
clearable-marker class lives on it.

**Ask:** one declared `replace-owned` rule action that emits `update_with_triples`
(atomic replace-by-(subject,predicate)), valid **only for predicates a bound
contract declares the pack owns**. It should write under the bound owner identity,
and carry the `OwnerToken` once that write-lease field lands — `OwnerToken` is
**not** a mutation-request field today (ADR-056 lists it as a deferred
write-gating increment, `pkg/ownership/doc.go:62`), so the action ships with the
bound owner identity and gains the token when graph-ingest's lease check does.
Deliberately scoped:

- **Not** `cas-transition`. A rule has no real target revision to condition on;
  phase transitions stay with the lifecycle `Manager` via `lifecycle_transition`.
  We do not want rules to become a parallel state-machine substrate.
- **`add_triple` is unchanged** — append/evidence-style, must-exist after ADR-055.
  The write-once class above keeps using it.

## Finding C — flip-gate framing for product rule packs (corrected)

Our `lineage.*`-targeted markers stamp loop-execution / plan-loop entities whose
own birth lane is itself part of the ADR-055 migration (Wave 2 / B1). So the
must-exist flip cannot be safe for our markers until those targets are born-first
via the migrated lane.

But the gate for *product rule packs* is **not** `foreign_edge_unclaimed_total` —
bare `triple.add` never enters the foreign-edge classifier (your Decision-4
lane-independence note). The relevant signals are:

- `entity_not_found` rejects on `graph.mutation.triple.add` reading **zero over a
  bake window**, and
- targeted product e2e (agentic / semteams) proving every marker's anchor entity
  exists before the marker write fires.

**Ask:** the closing-move ordering should name "product rule packs that stamp
framework-born entities" as a flip precondition, gated on the two signals above —
so the flip isn't declared safe purely from the framework producers' view.

## Finding D — `$entity.instance` vs `$entity.id` object-token semantics (minor)

Two of our pause-marker rules stamp the object as `$entity.instance`
(`agent-run/07,08`) where a sibling marker used a full 6-part entity ref —
consumers had to normalize, and the mismatch once broke run-resume. Mostly ours to
fix, but a one-paragraph spec of `$entity.instance` vs `$entity.id` vs
`$entity.triple.*` object semantics would keep producers from diverging. Flagging
since Q2 opens the run-marker classification anyway.

## What ADR-056 gets right (we read it)

- **Predicate-group granularity** is load-bearing for us: it is the only thing
  that makes "22 rules + the lifecycle Manager on one run entity" legal-by-
  construction. v1's "one owner per entity" would have flagged our whole pack.
- **The Q2 classification was right** — multi-owned-by-predicate-group, phase via
  the Manager, markers disjoint. We confirm it against the full pack.
- **`projection.Contract` + `ownership` are the right substrate.** We are not
  asking for a new model — we are asking for the config-producer wiring into the
  one you shipped.

## Core ask (single paragraph)

> Rule packs are a config-level producer class. They should be able to declare
> `projection.Contract` entries beside rule definitions, bound at rule-pack load
> under a stable subject-safe owner id such as `rule-pack.<pack-id>` (`validOwnerID`
> charset, no hashing). Only predicates declared in those contracts may use a new
> declared owned **replace** action (emitting `update_with_triples`, carrying the
> bound owner identity — and the `OwnerToken` once that deferred write-lease field
> lands). Existing `add_triple` remains append/evidence-style and must-exist after
> ADR-055; lifecycle phase still moves only through `lifecycle_transition`. No
> parallel registry; rules don't own anything they
> haven't declared; delete/update are not pretended to be append-evidence.

---

### Appendix — our readiness map

Full per-pack classification + the verification metrics we'll run when we bump our
pin: `semteams/docs/governed-skg-readiness.md`.
