# ADR-038: Chain Entity for Cross-Arc Data Flow and Milestone Markdown Rendering

## Status

**Proposed (2026-05-07).** Establishes the canonical 6-part chain entity
that downstream personas query for cross-arc data, retires the slice 4c
loop_id-substitution pattern, and codifies milestone markdown rendering
as a healthy side-effect of chain progress (not a canonical store).

## Why this exists

Smoke #8 run-2 (2026-05-06 21:00Z) wedged at `dev-via-spec-architect`.
Verbatim from `/tmp/smoke8-run2/findings.md`:

> "Cannot emit artifact due to incomplete provenance chain. The task
> provided only the challenger_loop_id. To select the correct
> `test_harness` for the OGC SensorThings API integration check, I
> must read the research artifact, but its loop ID is missing from
> the inputs. Inventing a `test_harness` ID causes a tool error."

ADR-035 §D2 ("per-role rigour, not exhaustive backward reach") is the
right principle at the persona layer — each role reads ONE prior loop,
focuses attention, resists Goodhart from echo-forward gymnastics. But
the architect genuinely needs the research artifact's `harness` field
to populate `test_harness` references in `checks[]`. Per-role rigour
forecloses persona-prose echo-forward; it does NOT foreclose a
data-layer primitive that exposes cross-arc references by stable
predicate name.

Slice 4c (PR #91, 2026-05-07) shipped a tactical fix: rule actions
substitute `$entity.triple.lineage.researcher` literals into prompt
bodies so the architect's spawn rule writes the research_loop_id into
the architect's prompt. semstreams beta.51 auto-stamps a
`lineage.<role>` triple on every spawn; rules forward-thread the
lineage through the dev-via-spec arc; the architect substitutes the
literal into its prompt and calls `read_loop_result` on it.

Slice 4c works AS SCAFFOLDING. It has three problems as a load-bearing
mechanism:

1. **Per-arc lineage threading is per-rule bookkeeping.** Every spawn
   rule that wants research lineage to reach a downstream role needs
   `properties.related_loops.researcher: $entity.triple.lineage.researcher`.
   Adding a new arc means re-threading through every rule. The
   bidirectional contract test (`test/contract/lineage_threading_test.go`)
   enforces parity but the bookkeeping is real.
2. **Lineage substitution into prompt prose is coaching, not data.**
   The architect's spawn rule substitutes
   `$entity.triple.lineage.researcher` into the prompt body. The LLM
   sees a UUID embedded in prose and is asked to call `read_loop_result`
   on it. This is the rule prompt doing what the persona's tool-call
   contract should do; the rule prompt becomes a coaching surface
   (audit at `/tmp/prompt-coaching-audit.md` finds rule_05 at 58% P+F,
   rule_06 at 50% — partly because of these literal substitutions and
   partly because the iteration choreography around them).
3. **It only solves the architect's case.** Plan, consensus, evidence
   summary all need similar cross-arc reach in future slices. Each
   would need its own `lineage.<role>` triple, its own per-rule
   forwarding, its own prompt-body substitution. The complexity grows
   per-arc.

The right primitive is a stable cross-arc data structure that
downstream queries by name. **The chain entity.**

## Framing

### 6-part entity IDs are canon

Loop entities are addressed at `c360.<platform>.agent.agentic-loop.execution.<loop_id>`
per `agentic.LoopExecutionEntityID` in semstreams beta.51. Trajectory
steps are at `c360.<platform>.agent.agentic-loop.step.<loop_id>-<n>`.
Model endpoints are at `c360.<platform>.agent.model-registry.endpoint.<name>`.
Ops diagnoses are at `c360.<platform>.ops.diagnosis.finding.<uuid>`.

Chain entities slot into the same canonical shape:

```
c360.<platform>.agent.chain.execution.<chain_id>
```

Treat `chain` as a sibling component to `agentic-loop` within the
`agent` domain. NO ad-hoc entity ID shapes. The query surface is
`query_entity(id=<6-part>)` and `query_entities(predicate=<stable>)`
exactly like every other entity in the graph.

### Markdown is a side effect, not the canonical store

The graph is first-class memory. Personas query the graph; humans
read the markdown. The markdown gets emitted at critical chain
milestones for human consumption (review, audit, scanning) and git
use (diffs, blame, navigation). Both produced together at
milestones — never one without the other when the data warrants
both.

The existing `emit_dev_via_spec_artifact` already embodies the
pattern: it writes `docs/specs/<slug>.md` (markdown side-effect),
mints marker triples on the loop entity (graph data), and publishes
a typed payload (audit). ADR-038 generalizes the pattern to the
chain level.

## Decision

### D1. Chain entity primitive

A chain entity is created at chain start and lives for the chain's
duration. Canonical 6-part ID:

```
c360.<platform>.agent.chain.execution.<chain_id>
```

**chain_id derivation.** The dispatch loop's UUID becomes the chain
anchor. No new ID generation required at chain-start; the dispatch
loop IS the chain origin. ADR-037 §Open question 1 named "derive at
query time vs mint at dispatch" as a forward question; ADR-038
chooses **mint at dispatch** because the chain entity now has its
own subject-predicate cluster and queries against that cluster
benefit from a stable ID without walking `prior_loop_id`.

**Lifecycle.** The dispatch rule (or first-loop boot hook) mints the
entity at chain start by writing `chain.dispatched_at` triple with an
RFC3339 timestamp. The entity persists for the chain's active
duration plus retention. It does NOT get reaped per-loop — chain
entities outlive every loop in the chain.

**Constructor.** A new `agentic.ChainExecutionEntityID(org, platform,
chainID)` lands in semstreams (preferred) or in the product shell
under `cmd/semteams/chain/entity_ids.go` if upstream timing is wrong.
Callers that need to publish triples on the chain entity use the
constructor; no string concatenation. This mirrors
`LoopExecutionEntityID`'s validation discipline.

Component naming. `chain` is the sibling to `agentic-loop` within
the `agent` domain. Future expansions (e.g. ops aggregation entities
spanning multiple chains) introduce new component names — not
overloaded `chain`.

### D2. Predicate namespace on chain entity

Predicates on the chain entity follow `chain.<milestone>.<field>`.
Stable, queryable by name without loop_id indirection.

The schema below is illustrative; the final list is earned per arc
at implementation time. The shape is load-bearing.

```
# Chain start (set by dispatch rule)
chain.dispatched_at                       <rfc3339>

# Research arc milestone
chain.research_artifact_loop              <researcher_loop_id>
chain.research_artifact.harness           <catalog_test_harness_name>
chain.research_artifact.path              <docs/research/<slug>.md>
chain.research_artifact.actor_count       <int>
chain.research_artifact.task_count        <int>

# Dev-via-spec planning milestones
chain.plan_loop                           <planner_loop_id_at_approval>
chain.plan.path                           <docs/plans/<slug>.md>
chain.consensus_loop                      <challenger_loop_id_at_accept>
chain.consensus.path                      <docs/consensus/<slug>.md>

# Architect terminal artifact
chain.spec_artifact_loop                  <architect_loop_id>
chain.spec_artifact.path                  <docs/specs/<slug>.md>
chain.spec_artifact.check_count           <int>

# Build milestone
chain.build_loop                          <builder_loop_id>
chain.build.workspace_path                <workspace_root>
chain.build.tests_passing                 "true|false"

# Evidence summary
chain.evidence.summary_ready              "true"
chain.evidence.summary.path               <docs/evidence/<slug>.md>

# Failure handling (already in use per ADR-037)
chain.paused.cause                        <token>
chain.paused.classification               <fine_token>
chain.paused.role                         <failed_role>
# ... full ADR-037 §D5 schema
chain.decision.verb                       <retry|kill|defer>
chain.decision.actor                      <X-User-Id>
chain.resumed                             <new_loop_id>
chain.killed                              <rfc3339>
chain.deferred                            <rfc3339>
```

ADR-037 already established `chain.paused.*` and `chain.decision.*`
predicates. CURRENT IMPLEMENTATION stamps these on the FAILED LOOP'S
entity (see `cmd/semteams/chainpause/pauser.go:62`). ADR-038
**moves** them onto the canonical chain entity. This is the
re-pointing PR B carries; ADR-037's predicate naming convention is
preserved verbatim.

**Flat vs nested namespace.** ADR-038 chooses **flat**:
`chain.research_artifact.harness` not
`chain.dev_via_spec.research_artifact.harness`. Rationale: a single
chain spans multiple arcs. Nesting per-arc invites the question
"does this chain have a dev_via_spec arc?" at every read. Flat
predicates are unambiguous — the chain has at most one research
artifact, one approved plan, one consensus, one spec, one build.
Future need for parallel arcs (ADR-037 §Open question 5 named the
parallel-arc shape as deferred) re-opens this; until then, flat.

**Predicate write authority.** Most chain triples are written by
**rule actions** at milestone events (e.g. rule_03 stabilise-and-
transition writes `chain.research_artifact_loop` when the
research-reviewer approves). A few are written by **emit-tools** as
side-effects of milestone tool calls (e.g.
`emit_dev_via_spec_artifact` writes `chain.spec_artifact.path` and
`chain.spec_artifact_loop` directly when the architect calls it).
The split is mechanical: if a rule action's inputs are sufficient,
write from the rule. If the milestone is itself a tool call that
already has the data in hand, write from the tool. Both paths use
the same predicate namespace.

### D3. Markdown side-effect convention at critical milestones

The graph is the canonical store. Markdown is the rendered view at
critical milestones — emitted because humans review and git diffs.

**Path convention.**

```
docs/research/<slug>.md
docs/plans/<slug>.md
docs/consensus/<slug>.md
docs/specs/<slug>.md       # already exists; promoted to chain triple
docs/evidence/<slug>.md
```

**Slug derivation.** Reuse the existing `deriveSlug` helper from
`cmd/semteams/tools/emitspecartifact/executor.go:548`:
date-prefix (YYYY-MM-DD) + lower-kebab-case from a title-derived
string. Same shape across arcs; one helper, all milestones.

**Content shape.** Tool template renders headings; persona supplies
content via structured tool args. The persona never writes raw
markdown text — the persona supplies typed fields (goal, context,
actors, decisions, etc.) and the tool's deterministic template
renders them. This is exactly the pattern in
`emitspecartifact/executor.go:762` (the `artifactTemplateText`
template constant).

**Co-emission with chain triples.** Every emit-tool that writes
markdown ALSO writes the corresponding chain entity triples in the
SAME tool execution. Both atomic per slice 3 lessons (per-loop
emission ordering matters for downstream rule firing). If the
markdown write fails the triple write is skipped; if the triple
write fails the markdown is already on disk (idempotent on retry —
slug-deterministic path overwrites).

**Milestone selection criterion.** Would a human reviewer want to
read or diff this in git? The answer drives the decision:

| Milestone | Markdown? | Why |
|---|---|---|
| Research artifact | Yes | Reviewers audit research substance; sources, actors, harness ref are git-trackable. |
| Plan approved | Yes | Plan is the chain's terminal substance until architect ratifies. Reviewers audit. |
| Consensus accepted | Yes | Challenger's verdict reason is the load-bearing chain context. |
| Spec emitted | Yes | Already shipped; promoted to chain triple namespace. |
| Build complete | Maybe | Workspace tarball is the artifact; markdown summary may help operators. Defer to PR C. |
| Evidence summary | Yes | Operators audit which checks fired, with what evidence. Already exists as a triple-only store; ADR-038 promotes the markdown emission. |
| Reviewer rejection reason | No | Per-loop ephemeral context. Read via `read_loop_result`. Not a chain milestone. |
| Iteration trace | No | Trajectory data lives at `/teams-loop/trajectories/`. UI consumes; not git-relevant. |

The criterion is durable: ask "is this the artifact a human would
expect to find in the repo?" If yes, render. If it's per-loop
reasoning that the next role reads via `read_loop_result`, don't.

### D4. Loop entity stays transient (per-execution scoped)

Chain entity is for cross-arc CONTEXT data. Loop entity remains for
per-execution state.

**On the loop entity (unchanged):**

- `coordinator.next_action` — terminal action of the loop
- `coordinator.decision_reason` — terminal verdict reason
- `agent.loop.outcome` — success/failed/etc.
- Trajectory step subjects (per `TrajectoryStepEntityID`)
- Per-tool emission triples scoped to that loop's execution

**On the chain entity (new pattern):**

- Cross-arc references and milestone metadata (per D2)
- Markdown side-effect paths
- Failure-handling triples (re-pointed from ADR-037)

`read_loop_result(loop_id=X)` continues to work for trajectory and
per-loop state queries. **Cross-arc reads MOVE to chain entity
queries.** ADR-035 §D2 "per-role rigour" is reaffirmed at the
persona level — each persona is single-purpose, reads its immediate
prior loop. The chain entity adds a second query target (the chain)
that exposes cross-arc context the persona genuinely needs without
forcing the persona to walk loop chains.

The persona surface looks like:

```
# What the architect persona does (post-ADR-038):
1. read_loop_result(loop_id=<challenger>) — own's prior loop, per-role rigour
2. query_entity(id=<chain_entity_6_part>) — cross-arc references by predicate
3. emit_dev_via_spec_artifact(...) — terminal action, writes back chain triples + markdown
4. decide(action=tasks_emitted, ...)
```

No loop-id substitution into prompt prose. No echo-forward through
challenger's reason into reviewer's reason. The chain entity is the
seam.

### D5. Slice 4c lineage threading retires

Slice 4c (PR #91) was scaffolding for the architect's harness-reach
problem. ADR-038 supersedes it.

**What dies (in PR D):**

- `lineage.<role>` triples as a forward-threaded mechanism in
  product rules. The semstreams beta.51 auto-stamp at the framework
  layer stays — `Action.RelatedLoops` is a useful upstream primitive
  for OTHER consumers — but our rules stop using it.
- `$entity.triple.lineage.researcher` literal substitution in rule
  prompt bodies (rule_03 source-stamp, rules 01/02/03/04/05/06/07
  forward-thread).
- `properties.related_loops` blocks across the dev-via-spec rule
  set's spawn actions, where they exist solely to forward
  research lineage.
- The `test/contract/lineage_threading_test.go` contract test
  becomes obsolete. It is replaced by a chain-entity-coverage
  contract test that asserts every chain milestone writes its
  expected predicate set.
- The architect persona prose that walks the LLM into reading the
  literal `lineage.researcher` predicate. Replaced with persona
  prose that names the chain entity query.

**What stays:**

- `Action.RelatedLoops` field stays in our rule definitions for
  backwards compatibility where it serves OTHER lineage purposes
  (e.g. ADR-037 audit trail might want to record what loop spawned
  a paused chain). Where it carries research lineage forward, those
  property blocks delete.
- All `chain.paused.*` and `chain.decision.*` predicates stay
  exactly as ADR-037 §D5 specified — they re-point onto the chain
  entity.

**Memory rotation.** `feedback_architect_needs_research_lineage.md`
flips to RESOLVED-by-ADR-038. The lesson preserved: per-role rigour
at the PERSONA layer, chain entity at the DATA layer; do not
conflate.

### D6. Migration target — upstream `write_artifact` is roadmapped

Per `cmd/semteams/tools/README.md` and ADR-031's R3.2 framework-
alignment review: upstream's `write_artifact` (ADR-028 §What's not
built here, verified absent in semstreams beta.36) is the eventual
home for emit-tool patterns. SemTeams' `emit_*_artifact` tools are
product-shell prototypes of that slot.

ADR-038's new emit-tools (`emit_plan`, `emit_consensus`,
`emit_evidence_summary`) pattern-match on the same canonical shape:
typed args → markdown render → triple writes → typed payload
publish. When upstream's `write_artifact` ships:

1. Open a tracking issue evaluating migration of every
   `emit_*_artifact` tool onto the generic primitive.
2. The chain-entity triple writes likely move into a generic
   `write_artifact` execution hook (or stay as a thin product-shell
   wrapper around the upstream call).
3. The arc-specific markdown templates probably stay product-local
   (they encode SemTeams-specific arc shape that the upstream
   primitive shouldn't carry).

We ship the chain-entity-aware domain-specific tools because the
generic ones aren't ready, and contract to migrate when they are.
Mirror ADR-031 §addendum 2026-04-30's working template; document
the survey + alternatives in this ADR's references section.

### D7. Pairs with prompt cleanup

The 2026-05-07 prompt-coaching audit (`/tmp/prompt-coaching-audit.md`)
found 28% aggregate P+F across rule prompts; rule_05/06 at 50-58%.
A chunk of that coaching exists *because* personas were being told
how to thread loop_ids through prose. With chain entity queries the
rule prompt collapses to substance:

```
The chain has reached <state>. Your job: <terminal contract>.
Inputs: $entity.instance + chain entity (query for the predicates
you need). Terminal contract: <emit-tool> + <decide>.
```

PR D pairs the prompt rewrite with the data-layer fix because
shipping either alone fails: clean prompts without queryable chain
data recreate the architect wedge; chain data without prompt
cleanup leaves the audit P+F% in place.

## Migration path from slice 4c

Explicit retirement table. What stays, what dies, what's replaced.

| Slice 4c artifact | ADR-038 replacement | PR |
|---|---|---|
| `lineage.researcher` triple stamps in rule_01a, rule_01b, rule_03 | Chain entity `chain.research_artifact_loop` at rule_03 milestone | PR B |
| `properties.related_loops.researcher` in rules 01/02/03/04/05/06/07 (research forwarding) | Removed; chain entity is queryable by 6-part ID without forwarding | PR D |
| `$entity.triple.lineage.researcher` substitution in rule_05 prompt body | Removed; chain entity query in persona contract | PR D |
| `test/contract/lineage_threading_test.go` | New `test/contract/chain_entity_coverage_test.go`: every milestone writes its expected predicate set | PR B + PR D |
| `feedback_architect_needs_research_lineage.md` (memory) | Flips to RESOLVED-by-ADR-038; cited in this ADR's references | PR A |
| `dev-via-spec-architect/10-output-contract.md` lineage walk prose | Replaced with chain entity query pattern | PR D |
| `chain.paused.*` / `chain.decision.*` triples on failed loop's entity | Re-pointed to chain entity (ADR-037 schema preserved verbatim) | PR B |
| `evidence.summary` / `evidence.summary_ready` on builder loop's entity | Migrated to `chain.evidence.summary` / `chain.evidence.summary_ready` (rule_07's two-AND condition rewires accordingly) | PR B + PR C |
| `dev_via_spec.artifact.*` triples on architect loop's entity | Stay (per-loop scoped); ADD `chain.spec_artifact.*` on chain entity in same tool call | PR B |

The migration is incremental. PR B re-points the existing predicates
without changing their object values; rule conditions migrate to
match `subject` on the chain entity. PR C adds new emit-tools and
markdown rendering. PR D cleans the persona/prompt surface.

## What this ADR explicitly does NOT decide

1. **Implementation-time mechanics.** Long-term retention beyond
   active chain lifecycle, exact `deriveSlug` reuse-vs-fork per
   arc, and whether `agentic.ChainExecutionEntityID` lands upstream
   or in `cmd/semteams/chain/` initially are PR B/C decisions.
   Default posture: reuse existing helpers, ship the constructor
   wherever timing works, file an upstream PR if the surface is
   right.

2. **Cross-chain queries.** "Show me all chains where the architect
   selected `meshtasticd-3.x`" is ops-agent / polars territory.
   ADR-038's chain entities feed that without prescribing the query
   surface.
   **Earns its slot when:** ops-agent has the polars regime
   primitive (per upstream ADR-027 + ADR-037 §Open question 4) AND
   at least one operator surfaces a concrete cross-chain question.

3. **Tool-vs-rule split for chain entity write authority per
   predicate.** D2 named the heuristic ("rule action if inputs
   sufficient; emit-tool if milestone is itself a tool call") but
   the per-predicate split is mechanical. PR B's rule action
   refactor makes the call.
   **Earns its slot when:** PR B implementation surfaces ambiguity
   on a specific predicate.

4. **Parallel-arc chain shape.** ADR-037 §Open question 5 named the
   parallel-arc shape (two roles fail simultaneously, two arcs in
   flight) as deferred. ADR-038's flat predicate namespace assumes
   serial arcs. If parallel arcs land, predicates may need
   per-branch suffixes or a different namespace shape.
   **Earns its slot when:** ADR-033's multi-arc dependency block
   formalises a parallel arc shape AND smoke evidence shows it's
   real, not theoretical.

## Relationship to prior ADRs

- **ADR-029 (Product-Shell Wiring).** Chain entity wiring is
  product-shell concern. Chain ID minted at dispatch via product-
  shell rule, predicates written by product-shell rule actions and
  emit-tools. No new framework wiring.
- **ADR-031 (Research Flow).** Research arc retains its existing
  `emit_research_artifact` tool. Gains chain-entity triple writes
  (`chain.research_artifact_loop`, `chain.research_artifact.harness`,
  `chain.research_artifact.path`) at the same emission. No persona
  change in research arc until PR D's substance-over-process
  cleanup.
- **ADR-035 §D2 (Per-Role Rigour).** ADR-038 composes — per-role
  rigour at the PERSONA layer, chain entity at the DATA layer.
  Personas remain single-purpose, reading their immediate prior
  loop. Chain entity is for cross-arc CONTEXT data only. The
  audit's substance-over-process pivot (ADR-035 §D3) reaches its
  full expression with the chain entity in place — rule prompts
  collapse to substance, persona contracts query a stable surface.
- **ADR-036 (Test-Harness Lifecycle).** Chain entity makes the
  test_harness reference queryable without lineage indirection. The
  research-reviewer's harness-gate contract (configs/personas/
  fragments/research-reviewer/40-harness-gate.md) approves the
  research artifact; rule_03 stamps `chain.research_artifact.harness`
  to the chain entity at approval; the architect queries the chain
  entity at architect time. No more `lineage.researcher` walk.
- **ADR-037 (Chain Failure Handling).** Chain-pause already uses
  the `chain.paused.*` triple namespace; ADR-038 documents this as
  the canonical convention and **moves** the triples from the
  failed loop's entity to the chain entity. The schema in ADR-037
  §D5 is preserved verbatim; only the `Subject` changes. PR B
  carries the re-point; ADR-037 is updated with a forward reference
  in its addendum.
- **ADR-028 upstream (Orchestration Architecture).** `write_artifact`
  is the migration target for emit-tools per D6. ADR-038 builds the
  chain-entity-aware emit-tools as product-shell prototypes; when
  upstream's primitive ships, evaluate migration.

## Phasing — 4 PRs

Each PR has an explicit gate. No PR ships without its gate satisfied.

### PR A — This ADR (doc-only)

**Ships:**
- This ADR (ADR-038).
- Memory updates: `feedback_architect_needs_research_lineage.md`
  flips to RESOLVED-by-ADR-038 with a forward reference; the slice
  4c retirement note in `project_strategic_pivot_2026_05_06.md`.
- Slice 4c retirement note pinned in this ADR (D5 above).

**Gate:** Coby reviews and accepts. PR is doc-only; no code changes.

### PR B — Chain entity primitive

**Ships:**
- `cmd/semteams/chain/entity_ids.go` with
  `ChainExecutionEntityID(org, platform, chainID)` constructor
  (mirrors `LoopExecutionEntityID` validation discipline).
- Dispatch rule extension: write `chain.dispatched_at` on chain
  entity at dispatch.
- Rule_03 (research-mode-transition stabilise-and-transition):
  add `chain.research_artifact_loop` + `chain.research_artifact.harness`
  + `chain.research_artifact.path` writes to chain entity at the
  research-reviewer-approves transition.
- Chain entity re-point for ADR-037 triples: `cmd/semteams/chainpause/
  pauser.go` switches `Subject` from loop entity to chain entity for
  `chain.paused.*` triples; `chainpause/decision_handler.go` does the
  same for `chain.decision.*`.
- `emit_dev_via_spec_artifact` extension: in addition to existing
  loop-entity triples, write `chain.spec_artifact_loop`,
  `chain.spec_artifact.path`, `chain.spec_artifact.check_count` to
  chain entity.
- Evidence preprocessor extension: in addition to existing loop-
  entity `evidence.summary` / `evidence.summary_ready`, write
  `chain.evidence.summary` and `chain.evidence.summary_ready` to
  chain entity. Rule_07's two-AND condition migrates to match on
  chain entity.
- New contract test: `test/contract/chain_entity_coverage_test.go`
  asserts every milestone rule writes its expected predicate set
  to the chain entity.

**Gate:** smoke #8 mock-LLM journey converges with chain triples
landing on the chain entity at every milestone, OLD `lineage.*`
triples still emitted (drift-safe), real-LLM smoke deferred to PR D
or later. Lineage forwarding still active per slice 4c (PR D
removes it).

### PR C — Milestone emit-tools and markdown rendering

**Ships:**
- `cmd/semteams/tools/emitplan/` — `emit_plan` tool (typed args,
  markdown render to `docs/plans/<slug>.md`, chain triples).
  Called by planner persona at approved-pass terminal.
- `cmd/semteams/tools/emitconsensus/` — `emit_consensus` tool
  (markdown render to `docs/consensus/<slug>.md`, chain triples).
  Called by challenger persona at accept terminal.
- `cmd/semteams/tools/emitresearch/` — markdown side-effect added
  to existing `emit_research_artifact` (already writes triples;
  adds `docs/research/<slug>.md` rendering). Existing payload
  surface unchanged.
- `cmd/semteams/evidence/` — markdown summary file emission to
  `docs/evidence/<slug>.md` alongside existing graph-triple write.
- Persona contracts updated for plan/consensus/research/evidence
  emission (the affected fragments named in the audit's worst
  offenders).
- Chain entity coverage test extended for the new triples.

**Gate:** mock-LLM journey produces all five markdown artifacts on
disk after a successful chain pass; chain entity has the
corresponding triples. Lineage forwarding still active.

### PR D — Persona/prompt cleanup, lineage retirement, smoke #8 run-4

**Ships:**
- Rule prompt rewrite per audit's worst offenders (rule_05, rule_06,
  rule_07 prompt bodies; target P+F < 15%).
- Persona fragment rewrite for the audit's 🔴 list
  (`dev-via-spec-builder/10-bash-iteration-contract.md`,
  `dev-via-spec-architect/40-brownfield-discovery.md`,
  `dev-via-spec-builder/15-commitment-driven-authoring.md`,
  `dev-via-spec-architect/10-output-contract.md`,
  `dev-via-spec-builder/30-test-harness.md`,
  `dev-via-spec-architect/20-commitment-transcription.md`).
- Chain entity query pattern documented in
  `configs/personas/fragments/dev-via-spec-architect/00-identity.md`
  (or a new `05-chain-entity-queries.md` shared across roles, TBD
  at impl).
- Lineage forwarding retired: `properties.related_loops.researcher`
  blocks deleted from rule set; `$entity.triple.lineage.researcher`
  literal substitutions removed from rule prompt bodies.
- `test/contract/lineage_threading_test.go` removed.
- Fixture updates as needed.
- Real-LLM smoke #8 run-4: validates the architect reaches the
  chain entity successfully, the substance discipline holds, the
  builder slice consumes the chain entity for spec_artifact.path.

**Gate:** smoke #8 run-4 converges to qa-reviewer terminal on real
LLM, chain entity carries every D2 milestone predicate, no lineage
triples consulted in any persona prose, audit P+F% on the rewritten
files measures < 15%.

## Vocabulary

ADR-038 uses post-rename vocabulary per ADR-036 §Vocabulary:
`check`, `runtime`, `ref`, `tasks`, `test_harness`. New vocabulary
introduced here:

| Term | Meaning |
|---|---|
| chain entity | The 6-part `c360.<platform>.agent.chain.execution.<chain_id>` graph entity that carries cross-arc context. |
| chain milestone | A specific step in chain progress (research approved, plan approved, consensus accepted, spec emitted, build complete, evidence summary, paused, decided, resumed, killed, deferred) that writes a predicate cluster on the chain entity and optionally renders markdown. |
| markdown side-effect | The rendered `.md` file emitted alongside chain triples at a milestone. NOT a canonical store; canonical store is the graph. Rendered for human/git consumption. |
| chain_id | The dispatch loop's UUID, reused as the chain anchor identifier. |

## Consequences

### Positive

- **Cross-arc reach is a stable query surface.** Architect queries
  `query_entity(id=<chain_entity_6_part>)` for every cross-arc
  reference. Plan, consensus, evidence summary all use the same
  surface. New arcs add predicates; queries don't change shape.
- **Per-role rigour is preserved at the persona layer.** Each role
  still reads ONE prior loop. The chain entity adds a second query
  target that's stable, scoped, and queryable by predicate name —
  not an indirection through prose.
- **Markdown is exactly what it should be.** Rendered view at
  milestones for humans + git, not the canonical store. The
  product stays graph-first.
- **ADR-037 schema preserved.** Chain-pause / chain-decision triples
  re-point onto the canonical chain entity with no schema change.
  Operator surface and audit-trail consumers see the same data on
  a more durable subject.

### Negative

- **PR phasing is real work.** Four PRs to retire slice 4c is more
  than a single fix. Phasing exists because PR D's persona/prompt
  rewrite needs the chain entity queryable first; rushing the
  prompt cleanup without the data layer recreates the wedge.
- **Chain entity is a new query target.** Personas that didn't
  query the graph before now learn to. Persona fragment doc surface
  grows by one fragment per role at most (a shared "querying the
  chain entity" pattern fragment).
- **Two consumers for the same data during migration.** PR B leaves
  `lineage.*` triples in place (drift-safe); PR D removes them.
  Between PR B and PR D there is double-writing. Acceptable cost;
  one slice's worth of drift overhead.
- **Markdown-everywhere risk.** Future agents may be tempted to
  render markdown for every chain detail. The criterion in D3 is
  load-bearing: would a human reviewer want to read or diff this in
  git? If not, no markdown. ADR-038 names the discipline; future
  reviewers enforce.

### Neutral

- **No new payload-registry types.** Chain entity carries triples
  only; no typed payload analogous to `dev_via_spec.artifact.v1`.
- **No new framework primitives.** `agentic.ChainExecutionEntityID`
  is a 5-line constructor — preferred upstream landing site is
  semstreams (mirrors `LoopExecutionEntityID`), but the product
  shell can ship it locally with no consequence.
- **No new stream wiring.** Chain entity triples ride the existing
  `graph.ingest.*` subject pattern. Subject-predicate cluster on
  the chain entity is graph data exactly like loop entity triples.

## References

- `~/.claude/projects/-Users-coby-Code-c360-semteams/memory/project_strategic_pivot_2026_05_06.md`
  — strategic pivot 2026-05-06; Slice 4c context.
- `~/.claude/projects/-Users-coby-Code-c360-semteams/memory/feedback_architect_needs_research_lineage.md`
  — smoke #8 run-2 wedge that motivated slice 4c (flips to
  RESOLVED-by-ADR-038 in PR A).
- `~/.claude/projects/-Users-coby-Code-c360-semteams/memory/feedback_format_compliance_goodhart.md`
  — substance-over-format pivot from R3.4a smoke #4.
- `/tmp/prompt-coaching-audit.md` — 2026-05-07 audit showing 28%
  aggregate P+F coaching across rule prompts and persona fragments,
  rule_05/06 at 50-58%. Cited throughout D7 + appendix in PR D's
  rationale.
- `/tmp/smoke8-run2/findings.md` — the wedge.
- `/tmp/smoke8-run3/findings.md` — slice 4c verified on real LLM
  (8 lineage.researcher triples landed). Substance still untested.
- [ADR-029](029-product-shell-wiring.md) — product-shell wiring.
- [ADR-030](030-approval-flow-ui-and-identity.md) — approval surface
  composes with chain decision verbs.
- [ADR-031](031-research-flow-and-semspec-handoff.md) — research
  flow ownership; `emit_research_artifact` migration posture.
- [ADR-035](035-dev-via-spec-arc.md) §D2 — per-role rigour; ADR-038
  composes at the data layer.
- [ADR-036](036-test-harness-lifecycle.md) — test-harness lifecycle;
  chain entity makes test_harness reference queryable.
- [ADR-037](037-chain-failure-handling.md) — chain-pause primitive;
  predicate namespace already established, ADR-038 generalizes.
- [ADR-028 upstream](https://github.com/c360studio/semstreams/blob/main/docs/adr/028-orchestration-architecture.md)
  — `write_artifact` migration target for emit-tools.
- `cmd/semteams/tools/emitspecartifact/executor.go` — existing
  emit-tool pattern this ADR generalizes (markdown + triple emit
  in same call, deterministic template, slug derivation).
- `cmd/semteams/chainpause/pauser.go` — current ADR-037 implementation
  that writes to loop entity; PR B re-points to chain entity.
- `cmd/semteams/evidence/preprocessor.go` — current evidence
  summary writer; PR B/C extend to chain entity + markdown.
- `test/contract/lineage_threading_test.go` — slice 4c contract test
  retired in PR D.
- `cmd/semteams/tools/README.md` — framework-alignment review
  template.
