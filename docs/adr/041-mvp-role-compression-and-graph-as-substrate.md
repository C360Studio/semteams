# ADR-041: MVP role compression and graph-as-substrate (not reasoning)

## Status

**Proposed (2026-05-12).** Re-frames the agentic team for MVP based on
two empirical observations that have hardened across smokes #8 / #19:

1. **Frontier models do not reason from the graph.** Gemini 3.1 Pro,
   Sonnet 4.6, and the same models running semspec arcs ignore graph
   triples while reasoning, even when their personas are instructed to
   query and even when `summarize_graph` / `search_graph` are in the
   tool surface. The graph is a **substrate** (audit trail, lineage,
   harness-side memory injection) — not a **reasoning aid** for agents.
2. **The model floor for hard scenarios (e.g. OSH-demo) is a frontier
   model.** Smaller models converge on toy slices but consistently
   fail or fabricate on the OSH-class scenario. We can no longer
   justify role splits whose only rationale was "small models can't
   hold two cognitive jobs at once."

Given (1) and (2), the 13-role roster is over-engineered for MVP. This
ADR compresses it to ~4 functional roles and re-scopes the graph from
"reasoning substrate" to "operator/harness substrate."

This ADR **partially supersedes ADR-040** (`source-curator`),
**re-scopes ADR-031 §dev-via-spec phasing** (planner / architect /
challenger / reviewer collapse), and **does not change** ADR-027
(ops), ADR-029 (product-shell wiring), ADR-037 (chain-pause), ADR-038
(chain entity), or ADR-039 (needs-clarification recovery). The
chain-pause and approval seams are independent of role count and
keep their full value.

## Why this exists

### The 13-role inventory (current state)

| Role | Persona dir | Status under MVP |
|---|---|---|
| coordinator | `coordinator/` | **Keep** — entry point, decomposition, chain-level decisions |
| researcher | `researcher/` | **Keep** — single cognitive contract (read corpus, emit artifact) |
| research-reviewer | `research-reviewer/` | **Collapse** into reviewer (persona-swap) |
| source-curator | `source-curator/` | **Drop** — revisit per ADR-040 supersession below |
| source-registrar | `source-registrar/` | **Drop** — handled by semsource watcher, not LLM |
| dev-via-spec-planner | `dev-via-spec-planner/` | **Collapse** into researcher (planning is a research phase) |
| dev-via-spec-architect | `dev-via-spec-architect/` | **Collapse** into researcher (architecture is a research phase) |
| dev-via-spec-challenger | `dev-via-spec-challenger/` | **Drop** for MVP — re-introduce only if reviewer alone misses ambiguity classes |
| dev-via-spec-reviewer | `dev-via-spec-reviewer/` | **Collapse** into reviewer |
| dev-via-spec-builder | `dev-via-spec-builder/` | **Keep** as `builder` — produces artifact |
| dev-via-spec-qa-reviewer | `dev-via-spec-qa-reviewer/` | **Collapse** into reviewer |
| ops | `ops/` | **Keep** — read-only diagnostic, out-of-band from chain |
| ops-chain-observer | `ops-chain-observer/` | **Keep** — ADR-038 milestone subscriber, not a chain role |

13 → 4 chain-participating roles (**coordinator, researcher, builder,
reviewer**) plus 2 ops roles that are off-chain.

### The graph-as-reasoning hypothesis has failed

R3.2's graph-emit tools (`emit_research_artifact`, `add_entity`, etc.)
and R3.7's `summarize_graph` / `search_graph` were built on the
hypothesis that agents would *read* the graph to ground subsequent
reasoning. Smokes #8 (real-LLM, OSH-demo) and #19 (recovery cap
validation) show this consistently fails:

- Researchers query the graph less than once per loop on average; when
  they do, they discard the results and fall back to training-data
  priors. Quoted trajectories in ADR-040 §"Why this exists" make this
  explicit ("I will assume a hypothetical prior artifact…", "I'll have
  to rely on my existing knowledge…").
- Architects in smoke #8 wedged on `needs_clarification` rather than
  walk graph lineage to find the research artifact — fixed in semstreams
  beta.51 by *injecting* `lineage.<role>` triples into the prompt
  payload, not by improving graph-walk tools. The fix proved
  injection-side, not query-side.
- Curators in smokes #18 / #19 fabricated `verified_entity_ids` under
  pressure rather than re-query. Anti-fabrication persona text
  partially landed (#136) but the failure mode persists.

The graph still earns its keep — but as substrate, not reasoning:

| Graph use case | Verdict |
|---|---|
| Audit trail / lineage for operators | **Keep** — invaluable for debugging |
| Harness-side memory injection (RelatedLoops, lineage.<role>) | **Keep** — proven in beta.51 |
| Chain-entity milestone surface (ADR-038) | **Keep** — product-shell + UI value |
| Storing data coming in (semsource indexing) | **Keep** — entity catalog for operators |
| **Agent-side `summarize_graph` / `search_graph` for reasoning** | **De-emphasize** — keep tools available, don't design around them |
| **Agent-side graph-emit tools as required terminals** | **Keep emit, retire required-emit** — agents emit because *we* need triples (operator-side), not because the *next agent* needs them |

The shift in framing: graph tools are for the harness/operator, not
for the agent's reasoning loop. The agent emits because the harness
reads, not because the next agent reads.

### The role-rigour rationale has weakened

ADR-035 D2 ("per-role rigour, no cross-arc reads") and ADR-040's
cognitive-load split both leaned on a small-model floor. The argument
was: smaller models can't hold multi-job context, so split jobs into
roles with single contracts. Under a frontier-model floor (Gemini 3.1
Pro, Sonnet 4.6), this argument is belt-and-suspenders:

- Frontier models hold the merged context cleanly. The OSH-demo
  evidence isn't "frontier models fail at multi-job loops"; it's
  "smaller models hallucinate, frontier models converge."
- The orchestration cost of multi-role chains (rule-engine fires,
  KV/Stream round-trips, persona-fragment loading, chain entity
  updates) is real and currently *exceeds* the cognitive benefit at
  frontier scale.
- Goodhart resistance from separate roles still has value, but
  reviewer-as-separate-role gives most of that resistance; we don't
  need planner-vs-architect-vs-challenger to get it.

We keep the per-role *contract* discipline (each loop has one explicit
output contract). We drop the per-role *staffing* discipline (each
contract gets its own persona, persona dir, rule set, transition
rule).

## Decision

### MVP roster: 4 chain roles + 2 ops roles

```
coordinator → researcher → builder → reviewer
                                       ↓
                             (approve | retry | escalate)
```

- **coordinator** (unchanged) — entry point. Reads the user's request.
  Decomposes if needed. Spawns the first researcher.
- **researcher** (expanded contract) — absorbs `planner` and
  `architect` via persona-swap on `decide(next_phase=...)`. One loop,
  one output contract per call, but the phase the researcher is in
  can flip mid-chain. Reads the corpus, plans the work, drafts the
  artifact. `research-reviewer` does **not** collapse here — it
  collapses into `reviewer` (see below); the researcher only spans
  the *production* phases, not the *review* phase.
- **builder** (renamed from `dev-via-spec-builder`) — produces the
  concrete artifact (code, spec doc, harness fixture, research report).
  Single contract: take the researcher's artifact and ship it.
- **reviewer** (expanded contract) — absorbs `research-reviewer`,
  `dev-via-spec-reviewer`, `dev-via-spec-qa-reviewer`. Verifies against
  a structural check + reviewer persona. Single contract:
  `decide(action="approved" | "insufficient" | "needs_clarification")`.

Off-chain:

- **ops** (unchanged) — read-only diagnostic per ADR-027.
- **ops-chain-observer** (unchanged) — milestone subscriber per
  ADR-038.

### What dies entirely (delete, not deprecate)

- `configs/personas/fragments/source-curator/` — supersedes ADR-040.
  See §"ADR-040 supersession" below.
- `configs/personas/fragments/source-registrar/` — semsource watcher
  handles this; never load-bearing as an LLM role.
- `configs/personas/fragments/dev-via-spec-challenger/` — challenger
  pass was Goodhart-defense; reviewer + structural checks cover the
  same surface for MVP.
- `configs/personas/fragments/dev-via-spec-planner/`,
  `dev-via-spec-architect/`, `research-reviewer/` — fragments
  re-homed into `researcher/` (planning/architecting fragments) or
  `reviewer/` (research-reviewer fragments).
- `configs/personas/fragments/dev-via-spec-qa-reviewer/`,
  `dev-via-spec-reviewer/` — fragments re-homed into `reviewer/`.

### Rule directories that collapse

| Current | Target | Notes |
|---|---|---|
| `configs/rules/research-mode-transition/02-reviewer-rejected-spawn-curator.json` | **Delete** | No curator to spawn |
| `configs/rules/research-mode-transition/02b-curator-indexed-to-researcher.json` | **Delete** | Curator gone |
| `configs/rules/research-mode-transition/02c-curator-needs-clarification-to-researcher.json` | **Delete** | Curator gone |
| `configs/rules/research-mode-transition/02-reviewer-rejected-retry-research.json` | **New** | Reviewer "insufficient" → spawn researcher with reviewer's reason |
| `configs/rules/dev-via-spec/01-planner-to-reviewer.json` | **Rewrite** as `01-researcher-to-reviewer.json` | Phase-swap inside researcher; rule fires on `agent.loop.role = researcher` AND `decide.action = emit` |
| `configs/rules/dev-via-spec/03-reviewer-approved-to-challenger.json` | **Delete** | No challenger; reviewer-approved → builder directly |
| `configs/rules/dev-via-spec/04-challenger-concerns-retry-planner.json` | **Delete** | No challenger |
| `configs/rules/dev-via-spec/05-challenger-accept-to-architect.json` | **Delete** | No challenger, no architect |
| `configs/rules/dev-via-spec/06-architect-emit-to-builder.json` | **Rewrite** as `02-reviewer-approved-to-builder.json` | Reviewer's approved-spec artifact → builder |
| `configs/rules/dev-via-spec/07-builder-decide-to-qa-reviewer.json` | **Rewrite** as `04-builder-decide-to-reviewer-qa.json` | Builder's `decide(action=tests_passing)` → spawn reviewer with qa-mode fragment selection; same rule shape, target role changes from `dev-via-spec-qa-reviewer` to `reviewer` (input-loop phase = `qa`) |
| `configs/rules/dev-via-spec/08-architect-needs-clarification-to-researcher.json` | **Rewrite** as `03-needs-clarification-to-researcher.json` | Any role's `needs_clarification` → researcher |
| `configs/rules/dev-via-spec/09-qa-reviewer-needs-clarification-to-architect.json` | **Delete** | No architect; routed through new rule 03 |

### Researcher phase-swap mechanism

The researcher's persona contract becomes: "You are in one of four
phases. The current phase is named in your loop input. On `decide`,
you may name the next phase (subject to the allowed-transitions
table below) or `emit` (which terminates this research arc and
hands to reviewer)."

Phases:

| Phase | Fragments loaded | Output contract |
|---|---|---|
| `plan` | researcher/plan/* | Loop scope, sources to consult, structural shape of artifact |
| `gather` | researcher/gather/* | Corpus reads, entity queries, evidence collection |
| `synthesize` | researcher/synthesize/* | Draft artifact from gathered evidence |
| `architect` | researcher/architect/* | Concrete shape (interfaces, contracts) once `plan` + `gather` complete |

#### Allowed transitions (declared phase graph)

```
                   ┌─────────────────────────┐
                   ▼                         │
plan ──► gather ──► synthesize ──► architect ──► emit ──► reviewer
  │        ▲           │              ▲
  │        └───────────┘              │
  │              (re-gather allowed)  │
  └──────────────────► emit (premature; reviewer will reject)
```

Edge set (the structural validator enforces exactly these):

| From | To | Notes |
|---|---|---|
| `plan` | `gather` | Normal forward progression |
| `plan` | `emit` | **Premature emit**: allowed structurally, expected to be rejected by reviewer. Counts toward chain recovery cap (ADR-039). |
| `gather` | `synthesize` | Forward |
| `synthesize` | `gather` | **Re-gather allowed**: legitimate when synthesis surfaces a corpus gap. Bounded by per-phase cap below. |
| `synthesize` | `architect` | Forward |
| `architect` | `gather` | **Re-gather from architect allowed**: legitimate when concrete-shape work uncovers a missing dep. Bounded by per-phase cap below. |
| `architect` | `emit` | Forward; normal terminal |
| any → any not listed | **Rejected by validator** | E.g. `synthesize → plan`, `architect → plan`, `gather → architect`. Failed validation = chain failure, not soft warning. |

The forward path is `plan → gather → synthesize → architect → emit`.
The two back-edges (`synthesize → gather`, `architect → gather`)
exist because empirically (smoke #8 trajectories) the model does
discover corpus gaps during draft/architecture. No other back-edges
are useful: re-planning mid-arc is a coordinator-level concern
(re-spawn the chain), not a researcher-self-transition concern.

#### Per-phase cap (analogous to ADR-039's chain-level cap)

Each phase has a max-fires-per-chain count. Exceeding any limit
fires a chain-failure event (same shape as ADR-039 cap exhaustion),
not a soft warning:

| Phase | Max fires per chain | Rationale |
|---|---|---|
| `plan` | 1 | Re-planning is a coordinator concern, not a researcher concern |
| `gather` | 3 | Initial gather + up to 2 back-edge re-gathers (from synthesize, from architect) |
| `synthesize` | 2 | Initial synthesis + one revision after back-edge re-gather |
| `architect` | 2 | Initial architect + one revision after back-edge re-gather |

Implementation: a new chain predicate `chain.researcher.phase_count.<phase>`
incremented in the spawn rule (parallel to ADR-039's
`chain.recovery.count`). The structural validator (new) reads these
counters in the same rule fire and rejects the transition if the
target phase is at cap. This is enforceable in the rule engine
without LLM judgment.

#### Premature emit from `plan` is allowed by design

The contract permits `plan → emit` (researcher emits an artifact
without gathering/synthesizing). This is **expected behavior**, not
a bug: small models or models that misread their scope will do this,
reviewer will reject with `decide(action="insufficient")`, the chain
rule re-spawns researcher with the reviewer's reason. This counts
against the ADR-039 chain recovery cap and the per-phase `plan` cap
(which is 1 — so a second `plan → emit` will be the chain-failure
trigger). Smoke runs should expect zero-to-one premature emits per
chain; more than that is the failure signal.

#### Structural validator (Go, framework-side rule pre-filter)

The transition allow-list above is enforced by a Go validator that
runs as a rule pre-filter (before the rule's LLM call, if any). It
reads:

- the current researcher loop's input phase (from `agent.loop.input.phase`)
- the researcher's `decide` output's `next_phase` value
- the chain's per-phase counters

and rejects with a structured error that triggers chain-failure
handling per ADR-037. This is the right primitive because phase
transitions are deterministic — there's no LLM judgment to defer.

Phase progression is researcher-decided within the allow-list,
not coordinator-decided. The coordinator spawns researcher(phase=`plan`);
subsequent phases are `decide(next_phase=...)` self-transitions
validated structurally. This keeps the
rule-engine-as-orchestrator pattern (each phase is a fresh loop with
its own input contract) while collapsing the rule count.

The cost paid here: researcher loops increase in number (4 phases
instead of 1 monolithic researcher) but the rules + personas to staff
them decrease.

### Graph tool posture

| Tool | MVP status |
|---|---|
| `add_entity` / `add_triple` (rule-side) | **Keep** — chain entity, lineage, milestones |
| `query_entity` / `query_entities` (agent-side) | **Keep available, not required** — agents may query if they want; not in any persona's structural contract |
| `summarize_graph` / `search_graph` (agent-side) | **Keep available, de-emphasize** — remove from default persona fragments; opt-in per-arc |
| `emit_research_artifact` / `emit_spec_artifact` / etc. (agent-side) | **Keep** — but framing is "the harness needs this triple," not "the next agent will read this triple" |
| `emit_curator_artifact` | **Delete** — supersedes ADR-040 |
| `graph-query` component | **Keep** in all configs that have it today (PR #107) — needed for harness milestone subscribers |

Operator-side graph reads (chain entity in the UI, lineage walks
in ops diagnoses, semsource entity catalog browse) all keep their
full value. Nothing changes on the substrate side.

### ADR-040 supersession

ADR-040 (`source-curator`) is **superseded for MVP** but **not
rolled back blindly**. Its empirical contribution holds: source
acquisition + indexing-wait is a different cognitive job from
research-substrate consumption. Under MVP:

- The researcher role does not regain `add_source_repo`. That
  tool stays narrow (operator-invoked or semsource-watcher-driven,
  not researcher-driven).
- Operators add sources via semsource directly (CLI, MCP, or
  semsource's own watcher reacting to `add_source_repo` events
  on a control plane). The LLM agent is not in this loop for MVP.
- If/when post-MVP we re-introduce curator, it should be on a
  smaller-model floor where the cognitive-load split returns to
  the foreground. For frontier-floor MVP, source acquisition is
  not an agent concern.

ADR-040's "curator persona contract" + `emit_curator_artifact`
typed payload remain useful design references if the role
returns. The ADR keeps its accepted status; this ADR notes the
**MVP scope** where curator does not run.

### ADR-031 §dev-via-spec re-scope

The dev-via-spec arc as written had 6 roles (planner → reviewer
→ challenger → architect → builder → qa-reviewer). Under MVP
that arc becomes:

```
researcher(phase=plan)
  → researcher(phase=gather)
    → researcher(phase=synthesize)
      → researcher(phase=architect)
        → reviewer(spec)
          → builder
            → reviewer(qa)
```

7 loops, 3 roles. Each phase has its own contract; each role has
its own persona; reviewer's reviewer-vs-qa-reviewer split is a
persona-fragment selection (`reviewer/spec/*` vs `reviewer/qa/*`)
based on the input loop's phase, not a separate role.

The empirical question this ADR resolves: does the phase-swap
researcher converge as well as the multi-role chain did on
OSH-demo? Test plan below.

## What we deliberately preserve

These were named in conversation as "do not throw out with the
curator." Spelling out so the implementation phase doesn't drop
them by mistake:

1. **Chain-pause + approval seam (ADR-037 + ADR-038 + ADR-030).**
   Independent of role count. The product-shell value of human
   approval gates, chain-failure handling, and the chain entity
   itself is unchanged.

2. **Rule engine as the orchestrator.** Even though roles collapse,
   transitions stay rule-driven (rule fires on `agent.loop.role +
   decide.action` shape). We do **not** introduce a mega-loop with
   internal phase tracking. Each phase remains a fresh loop with
   its own input contract, observable in the harness, interruptible
   by ops, and approval-gated where required.

3. **Per-role output contracts.** Each loop still has exactly one
   `decide` allowlist + one emit contract. The researcher's four
   phases are four contracts, not one fuzzy contract. The reviewer's
   spec-vs-qa fragments are two contracts, not one fuzzy contract.

4. **ADR-039 needs-clarification recovery + cap.** The whole-chain
   cap and `chain.recovery.count` semantics (PRs #120-123) keep their
   value: they apply per-chain regardless of how many roles staff
   the chain. If anything, the cap matters *more* under role
   compression because a single role going `needs_clarification`
   four times is now four researcher loops, not four different roles.

5. **Ops agent (ADR-027).** Read-only diagnostic role is unchanged.
   Ops is the right place for "did the chain converge sensibly?"
   judgements; we are not moving that into the chain itself.

## Consequences

### Wins

- **Persona / rule / config surface shrinks ~60%.** 11 chain
  persona dirs → 4 (coordinator, researcher, builder, reviewer).
  Rule files in `dev-via-spec/` + `research-mode-transition/`:
  14 → ~7 (post-#3 builder→reviewer(qa) addition).
- **Fewer cross-role contracts to maintain.** Persona fragments
  no longer need to anticipate "what the next role will read";
  emit shapes optimize for harness/operator readability instead
  of LLM-readability. (The latter never worked anyway.)
- **Cheaper smokes.** Fewer rule fires per chain. Fewer
  persona-fragment loads per loop. Fewer chain-entity milestone
  triples per arc.
- **The graph-as-substrate framing is defensible.** We're no
  longer pretending agents reason from the graph. The graph's
  audit/lineage/operator-substrate value is unchanged and
  load-bearing.

### Risks

- **Phase-swap researcher might not converge as well as
  multi-role chain.** OSH-demo on Gemini 3.1 Pro is the empirical
  test. If the researcher fragments-per-phase don't ground
  cleanly, we'd see degraded artifact quality vs smoke #8 run-5
  (which was the last GREEN multi-role baseline).
- **Reviewer doing both spec-review and qa-review with persona
  swap could blur the contracts.** Fragment swap alone is weak —
  same model, same biases, same rubber-stamp failure mode. The
  load-bearing mitigation is **structural pre-checks gating the
  LLM reviewer**, not persona content:
  - **qa-mode pre-check (cheap, framework-side):** the
    `04-builder-decide-to-reviewer-qa.json` rule pre-filter
    *rejects* the builder→reviewer transition unless the builder's
    `decide` payload carries `action="tests_passing"` AND
    `tests_run > 0` AND `tests_failed = 0`. This is enforceable
    in Go before the reviewer LLM call. Builder cannot deliver a
    qa-review-ready artifact without structurally-evidenced
    passing tests; reviewer LLM is then judging quality, not
    presence.
  - **spec-mode pre-check (cheap, framework-side):** the
    `02-reviewer-approved-to-builder.json` rule pre-filter
    requires the reviewer's `decide(action="approved")` to carry
    a non-empty `coordinator.evidence_loop_ids` referencing at
    least one researcher loop in the chain. Reviewer can't
    approve a spec without evidence of prior research.
  - Persona fragment selection (`reviewer/spec/*` vs
    `reviewer/qa/*`) is a *secondary* mitigation that shapes the
    LLM's prompt but is not what blocks the failure mode.
- **Dropping challenger removes one Goodhart-resistance layer.**
  If we see Goodhart-shaped failures (artifact passes reviewer
  but is actually wrong) we re-introduce challenger. The
  empirical test is whether reviewer-with-structural-check
  catches the same class.
- **Curator removal might mean researchers fabricate corpus
  entities again.** Mitigation: semsource-driven indexing
  (operator-side) plus the researcher's persona constraint
  "you may only cite entity IDs returned by query_entity in
  this loop." If fabrication returns, we re-introduce curator
  on smaller-model floor (where ADR-040's argument still holds).

### Cost paid

- **Re-test surface.** Every smoke fixture that exercised
  dev-via-spec needs to be re-run under the compressed roster.
  Smokes #5 / #7 (last GREEN runs at multi-role) are the
  baselines.
- **ADR-040 work doesn't fully ship.** Curator persona /
  `emit_curator_artifact` tool / rules 02/02b/02c built in PRs
  #126 + follow-on PRs delete from MVP. The design remains
  documented for post-MVP re-introduction. We pay the sunk-cost
  honestly: those PRs validated the cognitive-load split exists,
  which informs *why* we're keeping researcher's contract
  narrow even without curator.
- **R3.7.3-R3.7.5 catalog work (ADR-033) needs review.** Some
  of the architect/builder/qa-reviewer fragments built there
  presume the multi-role chain. The catalog itself stays;
  fragment homing changes.
- **Role-name string surface in Go code is non-trivial.** The
  reviewer rightly flagged "audit external consumers" — done
  pre-merge, results below. None of these are external to
  semteams, but several are in production code paths that need
  to track the rename, not just config:
  - `cmd/semteams/chainpause/decision_handler.go:190` — workspace-
    reset gate keyed on `role == "dev-via-spec-builder"`. Phase 1
    code change to track `role == "builder"`.
  - `cmd/semteams/chain/predicates.go:233` — ADR-039 Tier 3
    cluster predicate description naming `dev-via-spec-builder`.
    Update doc string + any literal use of the role name.
  - `cmd/semteams/devviaspec/artifact.go` — entire file is
    architect-emitted-artifact code. Re-home to researcher's
    `architect` phase output OR collapse into a single
    "research artifact" shape. Phase 1 decision required.
  - `cmd/semteams/chainpause/decision_handler_test.go` (literal
    `dev-via-spec-reviewer`, `dev-via-spec-challenger`),
    `decision_handler_integration_test.go` (literal
    `dev-via-spec-builder` as `chain.paused.role` object) —
    test fixtures need updates in lockstep with Phase 1.
  - `cmd/semteams/inject_integration_test.go`,
    `cmd/semteams/evidence/preprocessor_test.go`,
    `cmd/semteams/chainpause/pauser_test.go`,
    `cmd/semteams/recoverycounter/counter.go`,
    `cmd/semteams/chain/*_test.go` — string-match role names in
    tests; update with codebase rename.
  - UI surface is clean: `ui/src/lib/types/agent.test.ts`,
    `ui/src/lib/stores/agentStore.test.ts`,
    `ui/src/lib/components/chat/AgentLoopCard.test.ts`,
    `ui/src/lib/components/board/TaskDetailPanel.test.ts`,
    `ui/src/lib/components/agents/TrajectoryViewer.test.ts`
    already use unprefixed `researcher` / `builder` / `reviewer`.
    No UI rename burden.
  - No external semsource / semspec / semdragons consumer of
    `agent.loop.role` strings was found in the grep (those repos
    consume payloads + tools, not role names).

## Phasing

Implementation in 4 phases. Each phase passes a reviewer pass
per CLAUDE.md before the next phase starts. Gate-test type varies
by phase:

| Phase | Gate |
|---|---|
| 1 (persona consolidation) | Contract tests (Go) — no smoke |
| 2 (rule rewrite + structural validator) | Contract tests (Go) — no smoke |
| 3 (config wiring) | Mock-LLM Playwright journey — no real-LLM cost |
| 4 (real-LLM validation) | OSH-demo smoke + structural acceptance criteria 1–6 |

**Phase 1 — Persona consolidation (no rule changes).**

- Re-home fragments: `researcher/plan/*`, `researcher/gather/*`,
  `researcher/synthesize/*`, `researcher/architect/*`.
- Re-home reviewer fragments: `reviewer/spec/*`, `reviewer/qa/*`,
  `reviewer/research/*`.
- Add researcher's `decide(next_phase=...)` terminal +
  persona-fragment selection on input-loop phase.
- Test: contract test verifies all fragments load; persona
  manager picks correct fragment set for each phase.

**Phase 2 — Rule rewrite + structural validator + curator-tool teardown.**

- Rewrite `dev-via-spec/` rules per the table above (including
  the new `04-builder-decide-to-reviewer-qa.json`).
- Delete `research-mode-transition/` curator rules; add
  `02-reviewer-rejected-retry-research.json`.
- Implement Go structural validator for researcher phase
  transitions (rule pre-filter, see §"Structural validator").
- Implement Go structural pre-checks for spec-mode and qa-mode
  reviewer rules (see §Risks mitigation).
- **emit_curator_artifact teardown.** Audit + delete consumers:
  - `cmd/semteams/tools/emitcuratorartifact/` (executor)
  - `cmd/semteams/curator/artifact.go` (typed payload)
  - `cmd/semteams/product_tools.go` (registration call site)
  - `test/contract/rule_tools_allowed_test.go` (allowlist test —
    update expected set, don't just delete)
  - Config references: `configs/e2e-dev-via-spec.json`,
    `configs/e2e-research-mode-transition.json`,
    `configs/osh-demo.json` — strip curator tool from
    `allowed_tools` lists, strip curator-role references
  - Payload registry: re-check `cmd/semteams/main.go`
    `payloadbuiltins.Register` call for any curator-payload
    registration; deregister
  - Run `grep -rn "CuratorArtifact\|emit_curator_artifact\|curator_artifact" .` post-edit to confirm zero remaining
    references outside ADR-040 (which keeps its accepted status
    as historical record).
- Test: rule-engine contract test (rules load, validate, fire on
  expected event shapes); structural-validator unit tests cover
  all allow-list edges + a representative rejection set.

**Phase 3 — Config wiring + mock-LLM smoke + docs sweep.**

- Update `agentic.json`, `agentic-claude.json`, `deep-research.json`,
  `onboarding.json`, `e2e-*.json` configs to load only the 4 chain
  personas.
- Mock-LLM Playwright journey covers coordinator → researcher
  (all 4 phases) → reviewer → builder → reviewer.
- **Existing fixture re-run inventory** (every smoke-fixture +
  e2e-spec that exercised dev-via-spec or multi-role research
  must pass under compressed roster before Phase 4):
  - [ ] `test/fixtures/journeys/dev-via-spec.yaml` + `ui/e2e/agentic/dev-via-spec.spec.ts`
  - [ ] `test/fixtures/journeys/dev-via-spec-qa.yaml` + `ui/e2e/agentic/dev-via-spec-qa.spec.ts`
  - [ ] `test/fixtures/journeys/research-mode-transition.yaml` + `ui/e2e/agentic/research-mode-transition.spec.ts`
  - [ ] `test/fixtures/journeys/research-with-source.yaml` + `ui/e2e/agentic/research-with-source.spec.ts` (will exercise the "no curator role" path)
  - [ ] `test/fixtures/journeys/research-iterative.yaml` + `ui/e2e/agentic/research-iterative.spec.ts`
  - [ ] `test/fixtures/journeys/research-harness-hit.yaml` + `ui/e2e/agentic/research-harness-hit.spec.ts`
  - [ ] `test/fixtures/journeys/deep-research.yaml` + `ui/e2e/agentic/deep-research.spec.ts`
  - [ ] `test/fixtures/journeys/coordinator-researcher.yaml` + `ui/e2e/agentic/coordinator-researcher.spec.ts`
  - [ ] `test/fixtures/journeys/tool-approval-gate.yaml` + `ui/e2e/agentic/tool-approval-gate.spec.ts` (validates approval seam unchanged)
  - Out of scope for Phase 3 re-run (off-chain or non-role-sensitive):
    `ops-agent-baseline.yaml`, `real-time-activity-stream.yaml`,
    `action-chips-personas.spec.ts`, `admin-flows-inventory.spec.ts`,
    `task-story-trace.spec.ts`, `task-title-aliases.spec.ts`.
- **Docs sweep** (prevents mismatched documentation post-MVP):
  - Add tombstone note at top of `docs/adr/031-research-flow-and-semspec-handoff.md`
    pointing readers to ADR-041 for MVP-scope dev-via-spec
    phasing. ADR-031 stays accepted; the tombstone says "the
    planner / architect / challenger / qa-reviewer staffing is
    superseded by ADR-041 §MVP roster; the *contracts* in this
    ADR remain accurate."
  - Add tombstone note at top of `docs/adr/040-source-curator-role.md`
    pointing readers to ADR-041 §ADR-040 supersession (already
    in this ADR; just add the back-pointer at the top of 040).
  - Audit `docs/objectives/`, `docs/journeys/README.md`, and
    `docs/` for any walkthroughs that name the deleted roles;
    update with the compressed-roster path.
- Test: existing journey specs adapted; new chain-compression
  journey added.

**Phase 4 — Real-LLM smoke on OSH-demo.**

- Run smoke #21 on OSH-demo with Gemini 3.1 Pro coordinator +
  Sonnet 4.6 worker (per smoke #19 model pin doc). Smoke #20 is
  cancelled; #21 is the first real-LLM run under the compressed
  roster. Precede with a cheap Gemini-3.1-wire isolation probe
  (one researcher loop, ~$0.05) to confirm the model/backend
  variable before paying for a full chain.
- Compare against smoke #8 run-5 GREEN baseline.

**Acceptance criteria are structural, not LLM-judged.** Reviewer
is one of the things being collapsed, so reviewer-validates-reviewer
self-confirmation is forbidden as the acceptance signal:

| # | Criterion | Source |
|---|---|---|
| 1 | Builder's artifact passes `go build ./...` AND `task lint` | Deterministic — measurable without an LLM |
| 2 | Builder's `decide` payload carries `tests_passing` with `tests_run ≥ N` (where N = the count from smoke #8 run-5's GREEN builder loop) | Structural — read from `agent.loop.decide` event |
| 3 | All ADR-038 milestone triples land on the chain entity (`chain.researched_at`, `chain.planned_at`, `chain.spec_approved_at`, `chain.consensus_at`, `chain.built_at`) | Structural — counted from graph triples |
| 4 | Total loop count ≤ 1.5× smoke #8 run-5 baseline (which was 8) | Deterministic count |
| 5 | Zero structural-validator rejections during chain (a rejection means a buggy phase transition slipped through Phase 1+2 contract tests) | Deterministic count |
| 6 | Per-phase counters stay within caps (no `chain.researcher.phase_count.gather > 3` etc.) | Deterministic count |

If 1–6 all pass, the compression is empirically validated and we
ship.

If only 1+2+3 pass but 4 fails (loop count > 1.5×), the compression
works but is inefficient — file a follow-up to tune per-phase caps,
don't block ship.

If 1 or 2 fail, builder is shipping broken artifacts — Phase 4
fails, roll back to the 13-role chain, root-cause before retrying.

If 5 or 6 fail with 1+2+3 passing, the validator is mis-specified —
fix the allow-list before retry, don't roll back.

Explicitly **no LLM judge** in the acceptance path. The "is the
artifact good?" question is answered by "builds + tests pass +
milestones land," not by reviewer-as-judge. If a future reviewer
finds the artifact substantively wrong despite passing 1–6, that's
a follow-up Goodhart signal that motivates re-introducing
challenger, not a blocker for the MVP compression itself.

## What this ADR does NOT decide

- **Model pins per role.** Smoke #19's model pin doc (PRs
  #123-125, coordinator on Sonnet, workers on Gemini 3.1) stays.
  Whether to revise once researcher absorbs planner/architect
  phases is a Phase 4 question.
- **Whether to re-introduce challenger post-MVP.** Empirical
  question for smokes #20+.
- **Whether to re-introduce curator post-MVP.** Empirical
  question gated on smaller-model floor returning to scope.
- **Whether to drop graph-emit tools entirely.** This ADR keeps
  them as harness-substrate emitters. Post-MVP we may further
  simplify if operators don't read certain triples.
- **The exact name `researcher` for the merged role.** If
  `analyst` or `worker` reads better, name it then. The
  contract matters; the label is bikeshed.

## Relationship to other ADRs

- **Supersedes (MVP scope only):** ADR-040 (source-curator),
  ADR-031 §dev-via-spec phasing (planner/architect/challenger
  collapse into researcher/reviewer).
- **Re-scopes:** ADR-033 (catalog harness) — fragments re-home
  but the catalog itself stays.
- **Unchanged:** ADR-027 (ops), ADR-029 (product-shell wiring),
  ADR-030 (approval flow), ADR-035 §per-role-rigour-contracts
  (contracts stay; staffing compresses), ADR-037 (chain-pause),
  ADR-038 (chain entity + milestones), ADR-039 (needs-clarification
  cap).

## References

- Smoke #8 run-5 GREEN baseline (multi-role chain, 8m 11s):
  see [project_strategic_pivot_2026_05_06.md](../../memory/project_strategic_pivot_2026_05_06.md).
- Smoke #19 run-1 recovery cap validation (multi-role chain,
  11 loops, ~$0.50-$1):
  see [project_smoke19_run1.md](../../memory/project_smoke19_run1.md).
- Smoke #8 run-13 cognitive-load discovery that motivated
  ADR-040:
  see [project_smoke8_run13.md](../../memory/project_smoke8_run13.md).
- Graph-not-read evidence across smokes #8 run-13, run-18,
  smoke #19 run-1 (trajectory quotes in ADR-040 §"Why this
  exists").
- ADR-040 §"Curator failure modes (smoke #19 run-1)" — the
  honesty about curator's actual behavior under pressure
  contributed to dropping the role.
- Gemini 3.1 Pro + wire backend pivot (PR #147, branch
  `feat/gemini-3-1-wire-backend`):
  see [project_gemini_3_1_wire_pivot.md](../../memory/project_gemini_3_1_wire_pivot.md).
