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
spec.<cap>.purpose                       = "<## Purpose prose>"
spec.<cap>.requirement.<rid>.name        = "<### Requirement: Name>"
spec.<cap>.requirement.<rid>.statement   = "The system SHALL <behavior>"    // RFC-2119: SHALL/MUST/SHOULD/MAY
spec.<cap>.requirement.<rid>.scenarios   = [{name, steps:[{kw,text}]}, ...]  // JSON; Given/When/Then steps
spec.<cap>.requirement.<rid>.status      = "active"
```

Here `<cap>` = the OpenSpec `specs/<domain>/` folder name; `<rid>` = the
slugified `### Requirement: <Name>` title (OpenSpec has no formal IDs).
Scenarios are stored as structured Given/When/Then steps (§D3), not EARS.
These are written via `replace_owned` so the living spec has exactly one
authoritative current value per requirement and evolves only by archive
(§D4). The owner is the spec-layer writer; writes comply with the
governed-SKG owner-lease (born-first; see §Risks).

**Change (append evidence — immutable once written):**

```
change.<slug>.proposal.intent           = "<## Intent — why this change>"
change.<slug>.proposal.scope_in         = ["<in-scope>", ...]      // JSON-encoded
change.<slug>.proposal.scope_out        = ["<out-of-scope>", ...]  // JSON-encoded
change.<slug>.proposal.approach         = "<## Approach — high-level direction>"
change.<slug>.design.*                  = optional design.md (technical_approach, decisions[], data_flow, file_changes[])
change.<slug>.delta.<cap>.<rid>.op       = "added" | "modified" | "removed"   // OpenSpec ## ADDED/MODIFIED/REMOVED
change.<slug>.delta.<cap>.<rid>.statement= "The system SHALL <behavior>"      // added/modified
change.<slug>.delta.<cap>.<rid>.scenarios= [{name, steps:[…]}, ...]            // added/modified — Given/When/Then (JSON)
change.<slug>.delta.<cap>.<rid>.previously="<old statement>"                   // modified only — OpenSpec "(Previously: …)"
change.<slug>.delta.<cap>.<rid>.rationale= "<why deprecated>"                  // removed only — OpenSpec "(Rationale: …)"
change.<slug>.task.<n>.*                 = (see §D6; n = OpenSpec dotted number "1.1", + done-state)
change.<slug>.acceptance_command         = "<full-suite command CBG re-runs>"  // → plan.integration_test_command
change.<slug>.status                     = "draft" | "reviewed" | "archived"
change.<slug>.archive_date               = "YYYY-MM-DD"   // set on archive (folder prefix)
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

- **Render** (`graph → OpenSpec markdown`): hydrate `specs/<cap>/spec.md`
  (`## Purpose` + `### Requirement:` SHALL statements + `#### Scenario:`
  Given/When/Then blocks), `changes/<slug>/{proposal,design,tasks}.md`, and
  the delta `changes/<slug>/specs/<cap>/spec.md` (`## ADDED/MODIFIED/REMOVED
  Requirements`, with `(Previously: …)` / `(Rationale: …)`) from the triples
  above. Hydration is a projection — lossy by design (richer graph-only
  fields are not rendered).
- **Ingest** (`OpenSpec markdown → graph`): parse an existing `openspec/`
  tree (living specs, change folders, and `archive/`) into the triples
  above, preserving the Given/When/Then scenario steps. This is the
  brownfield "pick up current OpenSpec specs" capability (UC-1 step 2). Note
  OpenSpec `tasks.md` is **thin** (checkboxes only) — ingest recovers task
  text + done-state, **not** the execution-rich fields (§D6).

**Round-trip acceptance is semantic stability, not byte-stability.**
`ingest(render(G))` must equal `G` on the modeled predicates; `render`
∘ `ingest` of a hand-authored OpenSpec fixture must re-render
semantically (modulo formatting/ordering). The graph is canonical, so
markdown formatting drift is irrelevant — only the modeled facts must
survive the loop. P1's gate is this round-trip on one real OpenSpec
fixture.

### D3. Requirement format — RFC-2119 statement + Given/When/Then scenarios

OpenSpec is **not** EARS (verified against the upstream format 2026-06-22 —
see §Grounding addendum). A requirement is an **RFC-2119 statement** ("The
system **SHALL** / MUST / SHOULD / MAY <behavior>") plus one or more
**Given/When/Then scenarios** — the testable acceptance criteria. The
*scenarios* (not an EARS clause) map 1:1 to a dev-via-test step and are the
**claims** the env-readiness layer (ADR-054/055) later proves. The
`emit_change` tool (§Framework-alignment) **structurally validates** that
every requirement carries a SHALL-class statement and ≥1 scenario with at
least a `WHEN` and a `THEN` — the discipline lives in the schema that
rejects non-compliant payloads, not persona prose
([[encode-principles-structurally]]). Storing the canonical form as
OpenSpec's own shape (per the 2026-06-22 decision — native, not
EARS-with-conversion) keeps render/ingest a faithful round-trip; an
EARS / Spec-Kit converter is added at *those* ingest boundaries only if and
when we support them.

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
change.<slug>.task.<n>.goal             // == plan.task.<id>.goal           ┐ execution-rich
change.<slug>.task.<n>.target_files     // == plan.task.<id>.target_files   │ (project to
change.<slug>.task.<n>.test_command     // == plan.task.<id>.test_command   │  plan.task.*;
change.<slug>.task.<n>.assumptions      // == plan.task.<id>.assumptions    │  graph-only,
change.<slug>.task.<n>.non_goals        // == plan.task.<id>.non_goals      │  NOT in
change.<slug>.task.<n>.expected_outcome // == plan.task.<id>.expected_outcome ┘ tasks.md)
change.<slug>.task.<n>.text             // OpenSpec checkbox prose — renders "- [ ] <n> <text>" ┐ native
change.<slug>.task.<n>.done             // OpenSpec checkbox state (- [ ] / - [x])              │ (render
change.<slug>.task.<n>.section          // OpenSpec "## <section>" grouping                     ┘ tasks.md)
change.<slug>.task.<n>.requirement_ref  // -> delta.<cap>.<rid> the task implements — spec-layer-only
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
OpenSpec-native + linkage fields (`text`/`done`/`section`/`requirement_ref`)
are dropped at the seam (they render the spec / ground ADR-054/055 claims,
not execution).

**Ingest asymmetry.** This superset is the *render* direction (graph →
OpenSpec drops the execution-rich six). The *ingest* direction cannot invent
them: a brownfield OpenSpec `tasks.md` is thin checkboxes, so an ingested
change's tasks carry only `text`/`done`/`section`/`<n>` and must be
**enriched** (`create_change` re-planning, or a planner hop) before
`dev-from-task` can dispatch them to Ralph. Ingest-then-execute is therefore
not automatic for brownfield tasks — it routes through enrichment first.

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
OpenSpec requirement/delta shapes. `replace_owned` (governed-SKG) *is*
shipped and is the
owned-write substrate for §D1/§D4.

**2. Net-new product-shell surface.** Three product-local pieces, joining
the existing `emit*` family in `cmd/semteams/tools/` (`emitplan`,
`emitdevviatestplan`, `emitartifact`, …):

- `emit_change` — stamps `change.<slug>.*` triples; **validates the
  SHALL+scenario requirement shape + delta-op + the task superset (§D6)
  structurally** (rejects payloads missing scenarios or required task
  fields). Domain-specific, exactly like `emit_dev_via_test_plan`'s
  Karpathy validator.
- OpenSpec **render** + **ingest** — graph↔markdown transforms (§D2).
  Likely a product-shell tool pair or a subscriber; resolved at P1 build.

**3. Case for product-shell-local.** The requirement (SHALL+scenario) /
delta / task-superset schema enforcement is the load-bearing primitive
(§D3, §D6) — a generic
freeform-JSON `write_artifact` could not enforce it. This is the same
ruling as ADR-044 §addendum (Slice 1 §3).

**4. Alternatives ruled out.** (a) *Reuse `emit_plan`/`emitartifact`* —
they render markdown + stamp pointer triples; the spec layer needs
queryable `change.*` triples (substitutable via `$entity.triple.X`), not
a blob behind a path. (b) *Index-exploded array predicates* — rejected
per §D1 (rule engine can't iterate indexed predicates). (c) *requirement/
scenario validation in persona prose* — rejected hard; persona prose is
hopeful, schema is load-bearing.

**5. Migration target.** When upstream ships the ADR-028 generic
`write_artifact` suite, evaluate migrating `emit_change` alongside
`emit_plan` / `emit_dev_via_test_plan` / `emit_autoresearch_*`. The
requirement/delta schema stays product-local regardless (domain-specific
to the OpenSpec contract); if the generic primitive exposes a schema-validation
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
| Requirement (SHALL+scenario) + delta + task schema | | | `emit_change` validator |
| `create-change` persona bundle + rule pack | | | net-new |

**No new components. No framework changes.** Config + product-shell tools
on the existing substrate.

## MVP

- **P1 gate** — round-trip (§D2) on one real OpenSpec fixture:
  `ingest → render → ingest` is stable on the modeled predicates.
- **P2 gate** — `create_change` turns a prompt into a reviewer-passed
  change (proposal + delta + ≥1 scenario-validated requirement + tasks in the
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

1. **Render/ingest as tool vs subscriber?** — **RESOLVED (2026-06-22):
   a product-shell tool pair**, joining the existing `emit_*` / `query_*`
   family, not a subscriber. Rationale: (a) neither direction has a clean
   reactive trigger — ingest fires on a flow moment (repo cloned / sandbox
   ready with an `openspec/` dir), and render is "on demand" (§D5); a
   subscriber fits event→side-effect, these are imperative journey steps;
   (b) both touch the sandbox workspace filesystem (ingest reads
   `openspec/`, render writes it back for a PR), the established
   product-shell-tool-called-by-a-sandbox-role pattern (chainbash /
   request_sandbox precedent); (c) family coherence — the whole
   product-shell surface is tools, and `emit_change` (the `change.*`
   producer) is already one. Render is additionally usable as a plain
   internal function for the on-demand deliverable; it is exposed as an
   LLM tool only if a role needs to preview. Slice 5's subject-less
   mapping is wiring-agnostic, so this choice cost no rework. (ADR-055
   OQ1 — the analyzer's tool-vs-subscriber question — is independent and
   stays open under that ADR.)
2. **Capability + requirement identity** — RESOLVED by OpenSpec convention:
   `<cap>` = the `specs/<domain>/` folder name; `<rid>` = the slugified
   `### Requirement: <Name>` title (no formal IDs). Residue: slug-collision
   policy when two titles slugify equal, and how `<cap>` is chosen when
   *authoring* a new capability (ties to ADR-056 OQ2 brownfield detection).
3. **Change `<slug>` minting + collisions** — OpenSpec names changes
   descriptively (e.g. `add-dark-mode`); coordinator-authored vs
   prompt-derived + collision policy on re-runs stays open.
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
   risk as Lisa. Mitigated by the reviewer gate (ADR-039) + the
   SHALL+scenario structural enforcement (§D3).
3. **Round-trip lossiness mistaken for a bug.** Markdown is a lossy
   projection by design; the gate is semantic (graph) stability, not byte
   equality (§D2) — document this so a formatting diff is not chased.
4. **Predicate sprawl.** Held down by JSON-encoded arrays in single
   triples (§D1) and the OpenSpec-visible-subset rule (richer fields stay
   graph-only, not every field becomes a triple).

## Grounding addendum 2026-06-22 — OpenSpec format research

This ADR's first draft (2026-06-21) assumed **EARS** acceptance criteria. A
web-research pass against the canonical OpenSpec source corrected the model
*before* any code (the point of P1 step 1). Corrections folded into §D1/§D2/
§D3/§D6 above:

- **OpenSpec does not use EARS.** It uses **RFC-2119 `SHALL` requirement
  statements + `GIVEN/WHEN/THEN` scenarios** (verified across the README,
  `docs/concepts.md`, and `docs/getting-started.md`). Canonical-form
  decision (2026-06-22): store OpenSpec's native shape, *not*
  EARS-with-conversion (§D3).
- **`design.md` is a first-class change artifact** (Technical Approach /
  Architecture Decisions / Data Flow / File Changes) — added as
  `change.<slug>.design.*`.
- **`proposal.md`** sections are Intent / Scope (in/out) / Approach — the
  §D1 proposal predicates were aligned (from the invented why/what/impact).
- **`tasks.md` is thin** (`- [ ] 1.1` checkboxes, grouped by `##` section,
  no structured fields) — confirms the §D6 superset is render-direction
  only and adds the ingest-enrichment note.
- **No formal requirement IDs** — capability = `specs/<domain>/` folder,
  requirement key = slugified `### Requirement: <Name>` (OQ2/OQ3 narrowed).
- **MODIFIED** carries `(Previously: …)`, **REMOVED** carries
  `(Rationale: …)` — added to the delta model.

Lifecycle (`/opsx:propose → /opsx:apply → /opsx:archive`; CLI `openspec
init|list|show|validate|archive`) and `archive/<YYYY-MM-DD-name>/` match
§D4's staged compat. Package `@fission-ai/openspec` (Node ≥20.19).

Sources (Fission-AI/OpenSpec, `main`): <https://github.com/Fission-AI/OpenSpec>
· `docs/concepts.md` · `docs/getting-started.md`.

## Build addendum 2026-06-22 — slice 5 (the P1→P2 bridge: model ↔ graph facts)

P1 shipped the markdown↔model format layer (#229). Slice 5 adds the
model↔graph mapping in the same `cmd/semteams/openspec` package
(`facts.go`, `facts_change.go`, `facts_test.go`), keeping it a **pure**
layer (no graph/NATS import). It firms up the §D1 sketch into a built
contract; the deltas below are refinements made during the build, not
departures from the model:

- **Subject-less `Fact{Predicate, Object string}`.** The mapping emits a
  message.Triple *minus* the fields a writer owns (Subject / Source /
  Timestamp / Confidence). The graph-writing caller supplies those. This
  is what makes slice 5 **independent of OQ1** (tool vs subscriber) and of
  OQ2/OQ3 (entity identity): the same `Spec.Facts()` / `Change.Facts()` /
  `*FromFacts` transforms serve either wiring. It mirrors the precedent
  `plan.triples(runEntityID, now)` (emit_dev_via_test_plan), which is
  likewise subject-parameterised.
- **Object is always a string** — prose verbatim, scalars via `strconv`,
  arrays/scenarios JSON-encoded (§D1). The wire JSON shapes
  (`{name,steps:[{kw,text}]}` for scenarios; `[{name,body}]` decisions;
  `[{path,kind}]` file-changes) are pinned via DTOs decoupled from the Go
  model field names, so the model can evolve without breaking stored facts.
- **Ordering is order-of-fact-list-independent** (graph reads are
  unordered): spec/delta requirements carry an explicit
  `…​.<rid>.position` fact; **tasks are keyed by an integer index `<i>`**
  (`change.<slug>.task.<i>.{number,text,done,section}`), reconstructed by
  numeric sort. **Clarifies §D1/§D6:** the `<n>` in the task schema blocks
  is that integer index `<i>`, **not** the OpenSpec dotted label — the
  dotted "1.1" is preserved as `.number` (it may be absent or duplicated
  in a thin brownfield `tasks.md`, so it cannot be the key).
- **`<rid>` = `slug.Slugify(name)`**, collisions disambiguated `-2`/`-3`
  deterministically (§D1/OQ2). The original name is always stored in
  `.name` (for delta requirements too, not only living-spec ones — §D1's
  delta block omitted it), so the inverse recovers the name from the fact
  and never un-slugifies a (possibly suffixed) rid.
- **Heading text (the model `Title` fields) is not modeled** — §D1 carries
  no title predicate; a `# heading` is a render-synthesised projection
  (§D2 blesses formatting drift). The round-trip ignores `Title` (and the
  diagnostic `Warnings`), nothing else.
- **Writer-only graph-state is excluded from the pure mapping** — the
  lifecycle `…​.status` / `change.<slug>.{status,archive_date}`, the
  chain-level `acceptance_command`, and the §D6 execution-rich task fields
  (goal/target_files/test_command/…) are stamped by `emit_change` (P2),
  not derived from the OpenSpec model. The two predicate-shape tests
  actively assert they are *not* emitted here. This is what keeps the
  round-trip honest: it covers exactly the markdown-representable surface
  (§D2).

**P1 gate extended to facts:** `Facts∘FromFacts` is semantically stable
(ADR-057 §D2) on the real `add-mfa` change folder and the `auth` living
spec, plus rid-collision / dotted-capability / >9-task-ordering /
nil-vs-empty edge cases. go-reviewer pass: 0 Critical / 0 Major.

**OQ1 (render/ingest tool vs subscriber) resolved → tool pair** (see
Open Questions §1). It is the *wiring* of these transforms (P2), which
slice 5 was deliberately built not to depend on — so the resolution cost
no rework.

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
  the Given/When/Then scenarios (§D3) as claims.
- Governed-SKG (beta.113 `replace_owned` + owner-lease) — owned-write
  substrate for §D1/§D4.
