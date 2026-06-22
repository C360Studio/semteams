# ADR-057: OpenSpec-Compatible Graph Spec Model and the `create_change` Journey

## Status

**Proposed (2026-06-21).** First buildable slice (P1/P2) of the
[ADR-056](056-openspec-spec-driven-development-umbrella.md) umbrella. P1
is the graph spec model + OpenSpec render/ingest round-trip; P2 is the
`create_change` journey that produces a reviewed change from a prompt.

Builds on, and does not change:
[ADR-042](042-coordinator-instantiated-flows-via-templates.md)
(substrate-plus-overlays — this is a rule pack + persona bundle + a
`decide()` token, no new components),
[ADR-039](039-needs-clarification-recovery.md) (the reviewer gate's
`[NEEDS CLARIFICATION]` recovery), and the governed-SKG ownership model
(beta.113 `replace_owned` + owner-lease) for living-spec current-state.
The execution side (`dev-from-task`) is **out of scope here** — it is P4
([ADR-056](056-openspec-spec-driven-development-umbrella.md) §D5); this
ADR only specifies the **task shape** that makes the P4 seam clean (§D6).

## Context

ADR-056 §D1 settled *adopt OpenSpec, keep the graph canonical*. This ADR
specifies the model and the first journey. The load-bearing observation
is that **OpenSpec's structure already *is* a graph model** — its
artifacts map almost 1:1 onto triples, so the spec layer is predicate
design + render/ingest, not a new subsystem:

| OpenSpec artifact | Graph (authoritative) | Markdown (projection / interchange) |
|---|---|---|
| `specs/<cap>/spec.md` (living) | `spec.<cap>.requirement.<id>` — **owned current-state** (`replace_owned`) | rendered on demand |
| `changes/<slug>/proposal.md` | `change.<slug>.proposal.*` — append | rendered |
| `changes/<slug>/specs/<cap>/spec.md` (delta) | `change.<slug>.delta.<cap>.<id>.{op,acceptance}` — append | rendered |
| `changes/<slug>/tasks.md` | `change.<slug>.task.<i>.*` — append | rendered (OpenSpec-visible subset) |
| archive (merge delta → living) | a `replace_owned` mutation on the living-spec predicates | re-render both |

The spec layer must be **graph-primary**: markdown hydrates *from* the
graph (render) and ingests *into* it (parse), but never holds authority.
A typical brownfield target with an existing `openspec/` directory is
picked up via the ingest path; a fresh change is authored via
`create_change`.

## Decision

### D1. Predicate namespaces — `spec.*` owned, `change.*` append

The split follows the governed-SKG model: *current state that evolves*
is **owned** (single authoritative value, `replace_owned`, owner-lease);
*historical evidence that accretes* is **append**.

**Living spec (owned current-state):**

```
spec.<cap>.requirement.<id>.text        = "<requirement prose>"
spec.<cap>.requirement.<id>.acceptance  = ["<EARS clause>", ...]   // JSON-encoded
spec.<cap>.requirement.<id>.status      = "active"
spec.<cap>.title                        = "<capability title>"
```

These are written via `replace_owned` so the living spec has exactly one
authoritative current value per requirement and evolves only by archive
(§D4). The owner is the spec-layer writer; writes comply with the
governed-SKG owner-lease (born-first; see §Risks).

**Change (append evidence — immutable once written):**

```
change.<slug>.proposal.why              = "<why this change>"
change.<slug>.proposal.what_changes     = "<summary>"
change.<slug>.proposal.impact           = "<affected caps / breaking?>"
change.<slug>.delta.<cap>.<id>.op        = "add" | "modify" | "remove"
change.<slug>.delta.<cap>.<id>.text      = "<new/changed requirement text>"
change.<slug>.delta.<cap>.<id>.acceptance= ["<EARS clause>", ...]   // JSON-encoded
change.<slug>.task.<i>.*                 = (see §D6)
change.<slug>.acceptance_command         = "<full-suite command CBG re-runs>"  // → plan.integration_test_command
change.<slug>.status                     = "draft" | "reviewed" | "archived"
```

`acceptance_command` is **chain-level** (one per change, not per task) —
it is the full acceptance suite the chain-end reviewer (CBG) re-runs, the
direct analogue of Lisa's `plan.integration_test_command`. It is the spec
layer's equivalent of the per-task `test_command`s rolled up to a single
gate; §D6 explains why the execution seam needs it.

A change is a **historical record** — once reviewed it is not mutated in
place; superseding it means a new change. Archiving it (§D4) is a
`replace_owned` write onto the *living* predicates, not a mutation of the
change predicates. Arrays are **JSON-encoded strings in a single triple**,
never index-exploded predicates — the same discipline ADR-044 §addendum
(Slice 1, alternative "per-element triples ... Rejected") established,
because the rule engine substitutes triple *objects* but cannot iterate
indexed predicates.

### D2. Render-from-graph and parse-into-graph; prove the round-trip

P1 ships two pure, isolated transforms:

- **Render** (`graph → OpenSpec markdown`): hydrate `specs/<cap>/spec.md`,
  `changes/<slug>/{proposal,tasks}.md`, and the delta
  `changes/<slug>/specs/<cap>/spec.md` from the triples above. Hydration
  is a projection — lossy by design (richer graph-only fields are not
  rendered).
- **Ingest** (`OpenSpec markdown → graph`): parse an existing
  `openspec/` tree (living specs and/or change folders) into the triples
  above. This is the brownfield "pick up current OpenSpec specs"
  capability (UC-1 step 2).

**Round-trip acceptance is semantic stability, not byte-stability.**
`ingest(render(G))` must equal `G` on the modeled predicates; `render`
∘ `ingest` of a hand-authored OpenSpec fixture must re-render
semantically (modulo formatting/ordering). The graph is canonical, so
markdown formatting drift is irrelevant — only the modeled facts must
survive the loop. P1's gate is this round-trip on one real OpenSpec
fixture.

### D3. EARS acceptance criteria

Acceptance is stored as **EARS** clauses (`When <trigger>, the system
shall <response>`) on both living requirements and change deltas. EARS is
machine-checkable, maps 1:1 to a future test command, and is the
de-facto SDD standard. The emit tool (§Framework-alignment) **structurally
validates** that every requirement/delta carries ≥1 EARS-shaped clause —
the discipline lives in the schema that rejects non-compliant payloads,
not in persona prose ([[encode-principles-structurally]]). EARS clauses
are the **claims** the env-readiness layer (ADR-054/055) later proves.

### D4. OpenSpec compat depth — staged

Per ADR-056 Open Question 1, stage the compat surface:

- **v1 — change-folder read/write.** `create_change` produces
  `proposal + delta + tasks`; ingest reads change folders. The living
  spec is read-only context (ingested for gap analysis) and is **not**
  mutated by v1. This delivers the standalone "prompt → reviewed change"
  value with no owned-write lifecycle risk.
- **v2 — living-spec lifecycle.** Archive (merge an approved delta into
  the living spec via `replace_owned`) + the full living-spec round-trip.
  This is where the owner-lease write path is exercised in anger.

Staging keeps P1/P2 on the append-only `change.*` namespace (lowest risk)
and defers the owned-write `spec.*` evolution to v2, after the round-trip
and the journey are proven.

### D5. `create_change` journey — substrate-plus-overlays

P2 adds the journey as a category pack (ADR-042) — **no new components**:

- **`decide(action="create_change")`** — a new token in the coordinator's
  closed taxonomy. (`create_spec` for a full greenfield living spec is
  UC-3/P6; v1 ships only the delta-producing `create_change`.)
- **Rule pack** `configs/rules/create-change/` — spawn → draft → reviewer
  gate, mirroring the research/dev-via-test pack shapes.
- **Persona bundle** `configs/personas/fragments/<role>-create-change-<phase>/`
  — the harness-level job description; domain flavor stays in the pack
  ([[personas-describe-job-not-plumbing]]).

Journey shape:

```
prompt (front-door coordinator) → decide(action="create_change")
   ↓
author (one-shot): ingest living-spec context (if any) → gap analysis →
   emit change.<slug>.{proposal, delta, task}.* + [NEEDS CLARIFICATION]
   markers for unresolved questions
   ↓
reviewer (gate): gap/clarity review; unresolved markers → ADR-039
   needs-clarification recovery (back to author or ask_user), never a
   silent pass
   ↓
coordinator wake-up → respond_direct (the change is the deliverable;
   render to OpenSpec markdown on demand)
```

The **standalone deliverable is the reviewed change** — renderable,
hand-off-able, independent of execution.

### D6. The task shape is the P4 integration seam

`dev-from-task` (P4) reuses Ralph, which consumes `plan.task.<id>.*` on
the run entity, **and CBG**, which re-runs `plan.integration_test_command`
and diffs against `plan.chain_start_git_tag` (ADR-044 §Plan state as
triples). To keep that seam a **reprojection, not a transform**,
`create_change` emits each task as a **superset** of the planner-authored
`plan.task.*` fields:

```
change.<slug>.task.<i>.goal             // == plan.task.<id>.goal
change.<slug>.task.<i>.target_files     // == plan.task.<id>.target_files (JSON)
change.<slug>.task.<i>.test_command     // == plan.task.<id>.test_command
change.<slug>.task.<i>.assumptions      // == plan.task.<id>.assumptions (JSON)
change.<slug>.task.<i>.non_goals        // == plan.task.<id>.non_goals (JSON)
change.<slug>.task.<i>.expected_outcome // == plan.task.<id>.expected_outcome
change.<slug>.task.<i>.acceptance       // EARS — spec-layer-only, NOT projected
change.<slug>.task.<i>.requirement_ref  // -> delta.<cap>.<id> — spec-layer-only
```

Those six `==` predicates are exactly Lisa's planner-authored
`plan.task.*` fields. **Three categories of `plan.*` predicate are NOT
emitted by `create_change`, by design:**

1. **Coordinator-walk state** (`plan.task.<id>.status` / `.position` /
   `.depends_on`) — minted by the dispatcher at walk time, not spec data.
2. **Chain-level acceptance** — Lisa's `plan.integration_test_command`
   maps from the chain-level `change.<slug>.acceptance_command` (§D1), a
   one-to-one reprojection. **CBG needs this**; without it the chain-end
   gate has no suite to run. It is the reason §D1 carries a chain-level
   acceptance predicate at all.
3. **Runtime state** — `plan.chain_start_git_tag` is minted at sandbox
   entry (a git tag created when the workspace is provisioned), not
   projected from the spec.

So the P4 dispatch step is: reproject the six per-task fields
(`change.<slug>.task.<i>` → `plan.task.<id>`) **plus** the one chain-level
`acceptance_command → integration_test_command`; the dispatcher mints the
walk-state and the git tag. **Given those are populated, Ralph and CBG run
unchanged** — no change to the execute loop or the reviewer contract. The
spec-only fields (`acceptance` EARS, `requirement_ref`) are dropped at the
seam (they ground the spec layer / ADR-054/055 claims, not execution).

**This ADR fixes the schema; P4 specifies the projection mechanism** (a
dispatch-time rule vs. a `dev-from-task` entry tool — deferred per ADR-056
§D5). Recording the superset + the chain-level `acceptance_command`
constraint *now* is what prevents P2 from emitting a task shape P4 then
has to transform.

## Framework-alignment review (mandatory — CLAUDE.md §Product-Shell-Tool Discipline)

**1. Upstream survey** (`semstreams@v1.0.0-beta.113`, verified
2026-06-21). The emit-shaped executors upstream are `register_emit_diagnosis`,
`register_read_loop_result`, `register_write_todos`, `web_emit`,
`github_read`, `github_write`. The generic `write_artifact` / `read_artifact`
/ `list_artifacts` suite anticipated by semstreams ADR-028 §"What's not
built here" **remains unshipped** (same posture ADR-044 §addendum recorded
at beta.96). No upstream primitive renders/parses OpenSpec or validates
EARS/delta shapes. `replace_owned` (governed-SKG) *is* shipped and is the
owned-write substrate for §D1/§D4.

**2. Net-new product-shell surface.** Three product-local pieces, joining
the existing `emit*` family in `cmd/semteams/tools/` (`emitplan`,
`emitdevviatestplan`, `emitartifact`, …):

- `emit_change` — stamps `change.<slug>.*` triples; **validates EARS +
  delta-op + the task superset (§D6) structurally** (rejects payloads
  missing acceptance clauses or required task fields). Domain-specific,
  exactly like `emit_dev_via_test_plan`'s Karpathy validator.
- OpenSpec **render** + **ingest** — graph↔markdown transforms (§D2).
  Likely a product-shell tool pair or a subscriber; resolved at P1 build.

**3. Case for product-shell-local.** The EARS/delta/task-superset schema
enforcement is the load-bearing primitive (§D3, §D6) — a generic
freeform-JSON `write_artifact` could not enforce it. This is the same
ruling as ADR-044 §addendum (Slice 1 §3).

**4. Alternatives ruled out.** (a) *Reuse `emit_plan`/`emitartifact`* —
they render markdown + stamp pointer triples; the spec layer needs
queryable `change.*` triples (substitutable via `$entity.triple.X`), not
a blob behind a path. (b) *Index-exploded array predicates* — rejected
per §D1 (rule engine can't iterate indexed predicates). (c) *EARS
validation in persona prose* — rejected hard; persona prose is hopeful,
schema is load-bearing.

**5. Migration target.** When upstream ships the ADR-028 generic
`write_artifact` suite, evaluate migrating `emit_change` alongside
`emit_plan` / `emit_dev_via_test_plan` / `emit_autoresearch_*`. The
EARS/delta schema stays product-local regardless (domain-specific to the
OpenSpec contract); if the generic primitive exposes a schema-validation
hook, the JSON-Schema fragment lifts into config. The render/ingest pair
is a stronger upstream candidate (other products may want OpenSpec
interchange) — flag it for upstream after a second product needs it, per
the two-scenarios-before-substrate rule.

**6. Evidence trail.** `cmd/semteams/tools/README.md` gains rows for the
new tools with migration targets at P1 build; this §is the ADR-side
evidence trail.

## Reuse vs deltas

| Surface | Reused | Adapted | New |
|---|---|---|---|
| `replace_owned` + owner-lease (beta.113) | ✓ (living spec, v2) | | |
| Substrate-plus-overlays (rule pack + persona bundle) | ✓ | | |
| `decide` taxonomy | | extend | token `create_change` |
| ADR-039 needs-clarification recovery | ✓ (reviewer gate) | | |
| JSON-encoded-array-in-one-triple discipline | ✓ | | |
| Spec/change predicate model | | | `spec.*` / `change.*` |
| OpenSpec render + ingest | | | net-new transforms |
| EARS + delta + task-superset schema | | | `emit_change` validator |
| `create-change` persona bundle + rule pack | | | net-new |

**No new components. No framework changes.** Config + product-shell tools
on the existing substrate.

## MVP

- **P1 gate** — round-trip (§D2) on one real OpenSpec fixture:
  `ingest → render → ingest` is stable on the modeled predicates.
- **P2 gate** — `create_change` turns a prompt into a reviewer-passed
  change (proposal + delta + ≥1 EARS-validated requirement + tasks in the
  §D6 superset shape) on a brownfield fixture with an existing
  `openspec/` dir, validated on real-LLM smoke. The change renders to
  valid OpenSpec markdown. dev-via-test/Lisa journeys stay green
  (regression check).

Cost/iteration are **observations**, not gates (the ADR-044 posture).

## Consequences

### Positive

- Standalone "prompt → reviewed spec" value with zero execution risk.
- Brownfield OpenSpec pickup (ingest) and clean PR-body rendering fall out
  of the same render/ingest pair.
- The §D6 superset constraint makes the P4 execution seam a projection,
  not a transform — Ralph/CBG untouched.

### Negative

- A new product-local schema (`change.*`) to maintain until/unless the
  upstream generic artifact suite subsumes the emit half.
- Round-trip fidelity is a correctness surface (markdown ↔ graph).
- OpenSpec format drift upstream (their spec evolves) touches render/ingest
  — isolated, but real.

### Neutral

- Living-spec owned-write (`replace_owned`) lifecycle is deferred to v2;
  v1 is append-only `change.*`.
- Specs remain projections of graph truth, not authority (ADR-056 §D1).

## Alternatives Considered

- **Markdown-primary (OpenSpec files are the truth, graph mirrors).**
  Rejected — contradicts ADR-056 §D1 and the whole graph-primary harness;
  reintroduces the file-as-truth coupling SDD tools struggle with.
- **One big `spec_artifact` JSON blob behind a path pointer.** Rejected —
  not queryable/substitutable; defeats rule-engine routing on spec facts.
- **Skip ingest; author-only.** Rejected — ingest is the UC-1 brownfield
  "pick up existing OpenSpec" capability and is half of the round-trip
  that proves the model.
- **Full living-spec lifecycle in v1.** Rejected — exercises the
  owner-lease owned-write path before the model and journey are proven;
  staged to v2 (§D4).

## Open Questions

1. **Render/ingest as tool vs subscriber?** A coordinator-invoked tool
   pair, or a subscriber reacting to `change.*` writes? Resolve at P1
   (cf. ADR-055 OQ1 — same tool-vs-subscriber question for the analyzer).
2. **Capability (`<cap>`) identity** — how are capabilities named/keyed
   for brownfield ingest (derive from `openspec/specs/<dir>` vs a topology
   fact)? Ties to ADR-056 OQ2 brownfield detection.
3. **`<slug>` minting + collisions** — coordinator-authored vs derived
   from the prompt; collision policy on re-runs.
4. **v2 archive ordering** — when a delta archives into a living
   requirement under owner-lease, what is the born-first sequence
   (entity must exist before the owned write; see §Risks)?

## Risks

1. **Owned-write coordination (v2).** Living-spec `replace_owned` writes
   must comply with the governed-SKG owner-lease and the born-first
   constraint (the entity must exist before a foreign-anchor owned write).
   v1 sidesteps this (append-only `change.*`); v2 must thread it. The
   deferred HITL owned-write gap (semstreams#313 — approval-gated owned
   writes lack a subscriber-side owned-write lane) is a watch-item if a
   spec write ever sits behind an approval gate.
2. **Spec quality from the LLM.** Vague deltas diverge downstream — same
   risk as Lisa. Mitigated by the reviewer gate (ADR-039) + EARS
   structural enforcement (§D3).
3. **Round-trip lossiness mistaken for a bug.** Markdown is a lossy
   projection by design; the gate is semantic (graph) stability, not byte
   equality (§D2) — document this so a formatting diff is not chased.
4. **Predicate sprawl.** Held down by JSON-encoded arrays in single
   triples (§D1) and the OpenSpec-visible-subset rule (richer fields stay
   graph-only, not every field becomes a triple).

## Related

- [ADR-056](056-openspec-spec-driven-development-umbrella.md) — umbrella;
  this is its P1/P2 slice.
- [ADR-042](042-coordinator-instantiated-flows-via-templates.md) —
  substrate-plus-overlays (how the journey ships).
- [ADR-039](039-needs-clarification-recovery.md) — reviewer-gate recovery.
- [ADR-044](044-dev-via-test-pack.md) — the `plan.task.*` shape §D6 mirrors
  for the P4 seam.
- [ADR-054](054-test-harness-team-proof-environments-before-code.md) /
  [ADR-055](055-formal-claim-analysis-for-verification-gates.md) — consume
  the EARS clauses (§D3) as claims.
- Governed-SKG (beta.113 `replace_owned` + owner-lease) — owned-write
  substrate for §D1/§D4.
