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
  `architect` via persona-swap. The phase the researcher is in can
  flip mid-chain: each researcher loop emits `decide(action="<next-phase>",
  reason=...)` and the rule layer spawns a fresh loop into the
  named phase's persona dir. The `action` token IS the next-phase
  name (see §"Wire shape" for why `action` rather than a separate
  `next_phase` arg). Reads the corpus, plans the work, drafts the
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

#### Wire shape: phase rides on `decide.action`

The upstream `decide` tool (`processor/agentic-tools/decide.go`)
accepts exactly four args: `action`, `reason`, `subtopics`,
`retry_hint`. There is no `next_phase` arg, and adding one would
be a framework change ADR-041 explicitly avoids. The next-phase
name therefore IS the `action` token — `decide(action="gather", reason=...)`,
`decide(action="synthesize", reason=...)`, etc. Implementation
references in this ADR that say "next_phase" mean "the action
token, which encodes the target phase." The structural validator
+ persona allow-lists both read the action token directly.

Back-edges (synthesize → gather, architect → gather) emit the
same `action="gather"` token as the forward `plan → gather`
transition. The rule layer disambiguates by reading the spawning
loop's input phase, not by introducing a separate vocabulary
(e.g. `regather`). This keeps the persona vocabulary one-token-
per-target-phase and aligns with the single
`chain.researcher.phase_count.gather` counter.

#### Structural validator (Go, framework-side rule pre-filter)

The transition allow-list above is enforced by a Go validator that
runs as a rule pre-filter (before the rule's LLM call, if any). It
reads:

- the current researcher loop's input phase (from `agent.loop.input.phase`)
- the researcher's `decide` output's `action` value (the next-phase
  name; see §"Wire shape" above)
- the chain's per-phase counters

and rejects with a structured error that triggers chain-failure
handling per ADR-037. This is the right primitive because phase
transitions are deterministic — there's no LLM judgment to defer.

Phase progression is researcher-decided within the allow-list,
not coordinator-decided. The coordinator spawns researcher(phase=`plan`);
subsequent phases are `decide(action="<target-phase>")` self-transitions
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

- Re-home fragments into the phase-as-sub-role dirs:
  `researcher-plan/*`, `researcher-gather/*`,
  `researcher-synthesize/*`, `researcher-architect/*`. (Upstream
  `persona.LoadFromDirectory` is depth-2 only; phase-keyed
  fragment selection within a single role dir would require an
  upstream change, so MVP uses dashes-as-sub-roles. Documented
  fully in §"Implementation notes" addendum below.)
- Re-home reviewer fragments: `reviewer-spec/*`, `reviewer-qa/*`,
  `reviewer-research/*`.
- Declare each researcher phase's `decide.action` allow-list in
  the phase's identity fragment (the next-phase name IS the
  action token; see §"Wire shape").
- Test: contract tests verify (a) all phase dirs load,
  (b) phase identities reference their phase name,
  (c) phase identities declare the expected decide-action set per
  the phase graph, (d) no stale dev-via-spec vocabulary
  (`action="planned"` etc.) leaks into the new identities.

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

## Addendum 2026-05-12 — reviewer-research collapse-vs-keep decision

Phase 2 Slice C resolves an ambiguity left open by §"What dies
entirely" + §"ADR-031 §dev-via-spec re-scope" + the rule-collapse
table.

**Ambiguity.** §141-143 says "reviewer (expanded contract)
absorbs `research-reviewer`, `dev-via-spec-reviewer`,
`dev-via-spec-qa-reviewer`" — implying one role with three modes.
§"ADR-031 §dev-via-spec re-scope" diagram (lines 357-364) names
only two reviewer spawn points: `reviewer(spec)` after
researcher-architect-emit and `reviewer(qa)` after builder. The
phase-graph (line 207) shows `architect → emit → reviewer` with
no `reviewer(research)` label.

The structural question: a pure-research arc (e.g. `deep-research.json`
or any non-dev-via-spec deployment that runs only
`researcher-plan → gather → synthesize → emit`) terminates with a
reviewer. That reviewer reads a research artifact (not a spec
artifact). If reviewer is one role with one prompt, it has to
disambiguate at the LLM layer which artifact shape applies on
every spawn. Three persona-fragment dirs avoid that ungrounded
persona work and align with §141-143's "absorbs three ancestor
roles" framing.

**Decision: keep reviewer-research as a third mode.**

Three reviewer modes, fragment-selected by the spawning rule
based on the spawning loop's `agent.loop.input.phase`:

| Mode | Persona dir | Evaluates | Spawned by |
|---|---|---|---|
| research | `reviewer-research/` | research artifact (typed `emit_research_artifact` payload: actors[]/integration_points[]/tasks[]/addressed_gaps[]/open_gaps[]/test_harness/substrate_mutations[]/revision) | researcher emits from `synthesize` phase (pure research arc) |
| spec | `reviewer-spec/` | spec artifact (typed `emit_dev_via_spec_artifact` payload: goal/context/actors[]/integration_points[]/tasks[]/checks[]/provenance) | researcher emits from `architect` phase (dev-via-spec arc) |
| qa | `reviewer-qa/` | builder output (build green, tests passing, evidence rules pass) | builder emits `decide(action="tests_passing")` |

**Why keep, not collapse.**

The three modes have distinct output contracts. Both research and
spec artifacts carry `actors[]`, `integration_points[]`, and
`tasks[]` — but those shared fields have different shapes
(research `tasks` is `[]string`; spec `tasks[]` is `[]Task` with
`grounds_actors[]` + `grounds_integration_points[]`) and the
artifacts diverge in their non-shared fields:

- **Research-only fields**: `open_gaps`, `addressed_gaps`,
  `substrate_mutations`, `revision`, `test_harness`. These exist
  because the research artifact is a snapshot of an iterating
  research arc: gaps known and addressed, substrate changes
  recorded, revision counter monotonic.
- **Spec-only fields**: `goal`, `context`, `checks[]`, `provenance`.
  These exist because the spec artifact is a one-shot terminal
  output of the dev-via-spec arc: builder needs explicit goal
  + context + verification commitments + audit lineage.
- **QA mode** evaluates a builder commit (not a structured
  artifact) — `decide.action="tests_passing"` carries `tests_run`
  + structural evidence pre-checks that the rule pre-filter
  enforces before the LLM call.

Collapsing into one reviewer prompt forces the LLM to
disambiguate which contract applies, on every spawn. That's
ungrounded persona work — a persona-fragment selection driven by
the spawning rule's known input is the cheaper and more reliable
mechanism.

**Mode-selection mechanism (deferred to Slice D rule wiring).**

The ADR's rule table currently shows a single `01-researcher-to-reviewer.json`
rule. Under three modes, the rule layer needs to route to the
right persona dir per spawn. Two implementable options:

1. **One rule, two conditions.** The single rule fires on
   `agent.loop.role = researcher` AND `decide.action = emit` and
   sets `agent.task.persona_dir` based on `agent.loop.input.phase`
   (synthesize → reviewer-research; architect → reviewer-spec).
   Cleanest if the rule engine can write a conditional spawn arg.
2. **Two rules, one each.**
   `01a-researcher-synthesize-to-reviewer-research.json` (input.phase=synthesize)
   and `01b-researcher-architect-to-reviewer-spec.json`
   (input.phase=architect). Same condition matching as option 1
   but spreads across two files.

Slice D picks the option that matches the rule engine's actual
expressiveness. Option 1 depends on the rule engine supporting
conditional substitution in `agent.task.persona_dir` based on the
spawning loop's input phase; if that capability is absent, option
2 is forced — equivalent semantics, two files instead of one.
Either works; the persona-dir-per-mode discipline is the
load-bearing thing.

**Implementation status as of Slice C.**

- `reviewer-research/` (5 files) — research-artifact-shape
  evaluation contract verified. Read-channel paragraph in
  `00-identity.md` updated to the two-channel pattern
  (parallel to Slice 2A B1 fix for reviewer-spec) +
  techsplain ADR refs stripped.
- `reviewer-spec/` (4 files) — spec-artifact-shape evaluation
  (Slice 2A).
- `reviewer-qa/` (3 files) — content from old
  dev-via-spec-qa-reviewer; vocabulary review deferred to
  Slice D alongside the rule-wiring contract test (Phase 2
  todo item 4 from Phase 1 reviewer-pass).

**Contract test added.** `TestADR041_ReviewerResearchArtifactShape`
parallel to `TestADR041_ReviewerSpecArtifactShape`. Catches
drift-back regressions where someone re-edits reviewer-research
fragments toward spec-artifact vocabulary (the wrong artifact
shape).

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

## Addendum 2026-05-15 — Graph is internal harness state ONLY for chain agents

The original ADR-041 §"Graph posture shift" said: "Agent-side
`summarize_graph` / `search_graph` stay available but leave default
persona fragments." That phrasing was too permissive — it left the
tools available for personas to opt back into, which is what
**PR #152** did when [[project_synthesize_thinness]] surfaced a
"GATHER produces nothing" failure mode and the diagnostic instinct
was to add discovery tools.

PR #152 was rolled back in this addendum's sweep. The correct
policy, made explicit:

> **Chain agents do not read the graph. The graph is internal
> harness state — audit, lineage, milestone stamping, evidence
> aggregation. Only ops agents (whose named job IS observing
> harness state) read it.**

### Why the line is in this exact place

Three classes of "graph read" exist in our system, and only one is
agent-reasoning surface:

| Read | Purpose | Who reads | Verdict |
|---|---|---|---|
| `read_loop_result` (loops KV) | Chain's own progress state | Chain agents | ✅ Internal chain state, legitimate read |
| `bash` on `/artifacts/<kind>/<slug>.md` | Rendered emit-tool output | Chain agents | ✅ Filesystem, not graph |
| `web_search` | External-world facts | Chain agents | ✅ External, not graph |
| `summarize_graph` / `query_by_type` / `query_entity` / `query_entities` / `search_graph` | Substrate + corpus reads | **Forbidden for chain** / Allowed for ops | ❌ for chain |

`read_loop_result` reads from the loops bucket, which IS technically
graph state, but it's specifically *the chain's own previous-loop
terminal* — a single legitimate channel that the framework already
mediates. It's "internal state" in the same way a function reads its
own local variables.

`bash cat /artifacts/<...>.md` reads files written by emit-tools —
the file IS the inter-phase channel under ADR-029 §
shared-agentic-artifacts. Not graph reading.

`web_search` is external; the graph isn't in scope.

All graph-query tools (`summarize_graph`, `query_by_type`,
`query_entity`, `query_entities`, `search_graph`,
`query_relationships`, `query_neighbors`) are forbidden for chain
agents. They remain allowed for ops agents (`ops-chain-observer`,
`ops-progress-observer`, `ops-analyst`) — observing harness state IS
the ops role's named job per ADR-027.

### Why corpus reading is also gone (the [[project_synthesize_thinness]] re-frame)

The original instinct that motivated PR #152 was: "researcher needs
to discover facts about the prompt domain." That was correct. The
wrong inference was: "discovery should come from the local graph."

Under MVP, the local graph is not a corpus. ADR-040's source-curator
role (which would have indexed external corpora for researcher
consumption) was dropped from the MVP roster. The semsource watcher
indexes only what an operator pre-configures. For arbitrary demo
prompts (OSH-Meshtastic), the local graph has no relevant entities
— it carries chain substrate (chain.*, agent.*) and at most a
placeholder seed.

So:

- **For corpus discovery**, researcher uses `web_search`. External
  facts (OSH driver framework API, Meshtastic protobuf shapes) live
  on the web, not in our graph.
- **For chain-internal context** (what did the previous loop
  emit?), researcher uses `read_loop_result`.
- **For grounded artifacts** (the markdown emitted by upstream
  emit-tools), researcher uses `bash cat`.

No graph queries needed. No graph queries allowed.

### Sweep performed in this addendum's slice

Chain-agent spawn rules updated to drop graph-query tools:

- `configs/rules/research-mode-transition/04-phase-transition-to-gather.json`
  — drops `summarize_graph`, `query_by_type`, `query_entity`,
  `query_entities` (the last two PRE-DATED PR #152; pulled in this
  sweep). Keeps `web_search` (external grounding) +
  `read_loop_result` + `scratchpad` + `decide`.
- `configs/rules/research-mode-transition/06-phase-transition-to-architect.json`
  — drops `query_entity`. Keeps `read_loop_result` + `bash` +
  `emit_dev_via_spec_artifact` + `scratchpad` + `decide`.
- `configs/rules/research-mode-transition/01a-researcher-synthesize-to-reviewer-research.json`
  — drops `query_entity`.
- `configs/rules/research-mode-transition/01b-researcher-architect-to-reviewer-spec.json`
  — drops `query_entity`.
- `configs/rules/dev-via-spec/04-builder-decide-to-reviewer-qa.json`
  — drops `query_entity`.
- `configs/rules/research-iterative/02-reviewer-rejected-retry.json`
  — drops `query_entity`, `query_entities`. The legacy
  research-iterative arc spawns a chain-role `researcher` on
  reviewer-rejected retries; same chain-membership rule applies.
  Caught by reviewer-pass extending the contract test scope from
  the MVP-roster directories to all rule directories that spawn
  chain agents.

Chain-agent personas (researcher-gather, researcher-architect,
reviewer-research, reviewer-spec, reviewer-qa) updated to remove
references to graph-query tools and restate the chain-agent
toolchain (`read_loop_result` + `bash` + `web_search` + `scratchpad`
+ `decide` + role-specific emit tool).

Ops rules untouched:
- `configs/rules/ops/chain-terminal-observe.json` (ops-chain-observer)
- `configs/rules/ops/observe-chain-progress.json` (ops-progress-observer)

Both keep `query_entity`, `query_entities`, `query_relationships`,
etc. — observing harness state is the ops role's named job per
ADR-027.

### Contract test

`TestADR041_ChainAgentsCannotReadGraph` walks every chain-agent
spawn rule and asserts the rule's allowed tools do NOT include any
of the graph-query tools. Ops rules are excluded by role name from
the walk — they're whitelisted as legitimate graph readers.

Drift surfaces structurally: any future PR adding a graph-query
tool to a chain role's spawn rule fails this test with an explicit
message naming this addendum.

### What this leaves open

- **Pre-seeding corpus for operators.** If a deployment wants
  researcher to consume a curated corpus, semsource indexes it at
  boot and the corpus content shows up via web-search-equivalent
  external surfaces, NOT via graph queries from chain agents.
- **Re-introducing curator post-MVP.** ADR-040 captured the design
  for a source-curator role that COULD legitimately read the graph
  to assess corpus state before deciding what to add. **Curator
  would not be a chain agent.** Chain membership is structural: a
  chain agent is one spawned by a rule in
  `configs/rules/{research-mode-transition,dev-via-spec,research-iterative,coordinator}/`
  (rules subscribed to `agent.task.*` for in-chain dispatch).
  Curator is invoked off-chain — at indexing time, before the
  chain begins, on operator action. Its graph reads happen outside
  any chain agent's loop. The structural distinction (rule-spawn
  membership) is what keeps the policy clean under future role
  additions; "chain agents do not read the graph" stays true
  because the chain-vs-not-chain question is decided by
  rule-membership, not nominal labels.
- **Smoke #27 framing.** The empirical question is no longer "does
  GATHER use the discovery tools?" — there are no discovery tools.
  It's: "does GATHER ground synthesize via `web_search` to a degree
  that produces a substantive artifact?" If the answer is no, the
  next investigation is into web_search results / persona prompting
  for GATHER — NOT a reintroduction of graph reads.

## Addendum 2026-05-16 — Synthesize action_allowlist must be mode-aware

**Status:** Filed (Coordinator Slice 2 Piece 3 real-LLM smoke evidence).

Empirical finding: under `chain.mode = research_only`, the synthesize
spawn rule's static `action_allowlist` still includes `architect`.
Persona reframing alone — including explicit "do not transition to
architect" directives injected by a coordinator wake-up — does NOT
reliably prevent synthesize from choosing `decide(action="architect")`.

Evidence: Slice 2 Piece 3 smoke (2026-05-16,
`/tmp/slice2-piece3-20260516-144412/`). The phasevalidator's
`chain.mode` gate (Slice 1b Phase B) correctly rejects the
synthesize→architect transition with `reject_reason=mode_mismatch`.
The chainstall subscriber (Slice 2 Piece 1) routes to a coordinator
wake-up that re-frames the retry with `delegate_research` plus an
explicit anti-drift `reason`. The retry synthesize **drifts again**.
Two consecutive recovery cycles reproduced the drift; the third
gather→synthesize transition then hits the per-phase cap
(`synthesize=2`) and the chain wedges fail-safe.

### Root cause

`configs/rules/research-mode-transition/05-phase-transition-to-synthesize.json`
declares:

```json
"action_allowlist": ["architect", "gather", "emit", "needs_clarification"]
```

This is correct for `dev_via_spec` mode (architect is the canonical
forward edge) but structurally wrong for `research_only` mode (the
forward terminal is `emit`; `architect` is illegal). The
phasevalidator catches the drift at the transition layer, but only
after the synthesize loop has burned a phase-count slot — and the
mode-mismatch path then routes through chainstall recovery, which
itself burns budget.

Persona prose cannot reliably constrain an LLM's `decide` action
when the action is structurally available in the allowlist. Structural
gates beat LLM judgment ([[feedback_structural_over_llm_judgment]]).
The action_allowlist IS the structural gate; today's wiring just
doesn't vary it by mode.

### Fix

Split `05-phase-transition-to-synthesize.json` into two rules
mirroring the existing `01a` / `01b` synthesize→reviewer convention:

| Rule | Condition | `action_allowlist` |
|---|---|---|
| `05a-phase-transition-to-synthesize-research-only.json` | `chain.phase_transition.proceed.synthesize == "true"` AND `chain.mode == "research_only"` | `["gather", "emit", "needs_clarification"]` (no `architect`) |
| `05b-phase-transition-to-synthesize-dev-via-spec.json` | `chain.phase_transition.proceed.synthesize == "true"` AND `chain.mode == "dev_via_spec"` | `["architect", "gather", "emit", "needs_clarification"]` (existing) |

The spawn prompt's "Terminal options" section also narrows per mode
— omit the `decide(action="architect")` bullet in the `05a` prompt
so the persona's documented options match the structural allowlist
exactly.

### Why this is structural, not persona work

The action_allowlist is enforced by the framework's `decide` tool:
out-of-allowlist actions return a tool error and force the LLM to
retry. That's the same shape as the mode-gate already running in
phasevalidator, just at decide-time rather than transition-time
(which is too late under research_only — by the time
phasevalidator rejects, the synthesize loop has emitted its
terminal and burned a phase-count slot).

The persona's reframing (chain_mode preamble, anti-drift directive
from coordinator wake-up) becomes belt-and-suspenders rather than
the load-bearing constraint. The structural gate carries the
contract; the persona prose explains it.

### Migration posture

- The split is rule config only; no framework changes.
- Contract test `TestADR041_Synthesize_AllowlistMatchesMode` (new)
  walks both rules and asserts the union of allowlists equals the
  current `05` allowlist, and that `architect` is present in `05b`
  only. Prevents an operator from accidentally re-merging the rules.
- If the rule engine supports conditional substitution on
  `action_allowlist` based on a runtime triple value, the long-term
  cleanup is a single rule with `$entity.chain.mode`-keyed allowlist.
  Not in scope today — beta.64 doesn't ship this expressiveness,
  and the two-rule pattern is already established by `01a` / `01b`.

### What this doesn't fix

This addendum only handles `synthesize`. The same pattern applies
to any phase where `action_allowlist` varies by mode. Today's
catalog has one such phase (synthesize). If future modes introduce
divergent allowlists for `gather`, `architect`, or `emit`, the
same split convention applies. The contract test should grow to
cover all such phases as they're added.

### Relationship to chainstall

The chainstall recovery substrate (Slice 2 Piece 1) is the
safety net for cases where structural prevention fails — operator
mis-configuration, new modes added without allowlist updates,
framework bugs. It is NOT the primary remedy. Closing this
addendum reduces recovery-substrate firing to genuinely
exceptional cases, where its coordinator wake-up has a real chance
of converging the chain.

### Companion findings deferred

- Phase-cap detection in chainstall (Piece 1's scope was
  mode_mismatch only — `reject_reason=phase_cap` does not stamp the
  `chain.stall.*` audit cluster today). Filed as a GitHub issue;
  motivated by the same smoke memo. Becomes lower-urgency once this
  addendum lands because mode-aware allowlists prevent the
  cap-burn-through pattern that motivated phase_cap detection.
- Chainstall × phase-cap composition (cap=2 + drift = effectively
  one retry). Captured in `project_coordinator_slice2_design.md`
  memory addendum; defer the cap-resize / reset-on-recovery design
  decision until post-allowlist-split smoke evidence shows whether
  it's still needed.

### Evidence

`/tmp/slice2-piece3-20260516-144412/` (loops.json, 20 triple
snapshots, watcher.log, backend-chainstall.log) shows the two
drift cycles end-to-end. Memory:
`project_coordinator_slice2_piece3_smoke.md` §"Finding 1".
