# Research category rule pack

**ADR-042 §Phase 2 redesign — MVP-2 first category.** Category-keyed
rule pack that drives the `research` task class: a plan → gather →
synthesize → reviewer-research arc, terminating on the reviewer's
`decide(action=approved)` (which wakes the coordinator to compose a
user-facing reply).

This pack runs through the substrate singletons configured by
`configs/flow-bootstrap.json`. It is one of two-to-three category
packs that ship in the MVP-2 inventory; see ADR-042 §"MVP scope" for
the full list.

## Naming convention

Per ADR-042 open question #2, role tokens follow
`<cognitive-role>-<category>-<phase?>`:

| Role token | Phase | Persona dir (MVP-3) |
|---|---|---|
| `researcher-research-plan` | plan | `configs/personas/fragments/researcher-research-plan/` |
| `researcher-research-gather` | gather | `configs/personas/fragments/researcher-research-gather/` |
| `researcher-research-synthesize` | synthesize | `configs/personas/fragments/researcher-research-synthesize/` |
| `reviewer-research` | (single-phase) | `configs/personas/fragments/reviewer-research/` (already exists) |

The `<cognitive-role>-<category>` prefix lets `ls
configs/personas/fragments/ | grep ^researcher-research-` enumerate
the pack's personas in phase order, and lets contract tests assert
three-way consistency (persona dirs ∪ role tokens emitted by rules ∪
role names referenced in coordinator delegations).

## No chain.mode / phasevalidator / chainstall

The pack deliberately omits the gating triples used by the legacy
research-mode-transition pack:

- `chain.mode.classification` — pack identity replaces chain-entity mode
- `chain.phase_transition.proceed.{gather,synthesize,architect}` —
  phase-validator sentinels; pack uses direct role+decision matches
- `chain.spec_mode_gate.proceed`, `chain.qa_mode_gate.proceed` —
  dev-via-spec only; not relevant for research
- `chain.recovery.proceed` — recoverycounter sentinel; pack uses rule
  `max_iterations` for the same bound
- `evidence.summary_ready` — builder-only gate; not relevant for research

A contract test
(`test/contract/research_rule_pack_test.go::TestResearchRulePack_NoChainMachinery`)
pins this — the pack must boot without any of the retired
`cmd/semteams/{chainmode,phasevalidator,chainstall,recoverycounter}/`
machinery.

## Rules

| File | Trigger | Spawn |
|---|---|---|
| `01-coordinator-research-spawn.json` | coordinator decide(research) | researcher-research-plan |
| `02-plan-to-gather.json` | researcher-research-plan decide(gather, subtopics=[…]) | **N parallel researcher-research-gather** (ADR-046 for_each) |
| `03a-gather-stamp-completion-on-plan.json` | researcher-research-gather decide(synthesize) | (stamp `research.gather.completed_subtopic` on PLAN loop — counter half of the JOIN) |
| `03b-synthesize-when-all-gathers-complete.json` | PLAN loop's stamp counter `length_eq` PLAN's subtopics.length | researcher-research-synthesize (aggregates N gather siblings) |
| `04-synthesize-to-reviewer.json` | researcher-research-synthesize decide(emit) | reviewer-research |
| `05-reviewer-rejected-retry.json` | reviewer-research decide(insufficient) | researcher-research-plan (max_iterations=3) |
| `06-needs-clarification-replan.json` | any pack role decide(needs_clarification) | coordinator (max_iterations=3) |
| `07-reviewer-approved-to-coordinator.json` | reviewer-research decide(approved) | coordinator (wake-up for respond_direct) |
| `08-loop-failed-pause.json` | any pack role outcome=failed | chain.paused.marker triple (operator surface) |

### Fan-out shape (rules 02, 03a, 03b)

The planner emits `decide(action="gather", subtopics=[…])`. The `subtopics` list is the planner's epic decomposition, one-to-one. Rule 02's `for_each` over `coordinator.decision.subtopics` spawns one researcher-research-gather per item in parallel, each carrying `$subtopic` as its scope.

Each gatherer's `decide(action="synthesize")` triggers rule 03a, which uses the beta.83 `subject` override to stamp `research.gather.completed_subtopic` on the PLAN loop entity (Object = the gather's own loop id, so the predicate-set deduplicates naturally).

Rule 03b fires on the PLAN loop entity whenever its triples change, matching when the counter's `length_eq` equals the planner's subtopics list length (resolved dynamically via beta.84's `.length` substitution — `$entity.triple.coordinator.decision.subtopics.length`). It spawns ONE researcher-research-synthesize, passing `gather_loop_ids` (the accumulated counter list) so synthesize can `read_loop_result` on each sibling without graph-query tools.

N=1 (non-decomposable prompts) is a first-class case: the planner emits `subtopics=["<the whole question>"]`, rule 02 spawns one gatherer, rule 03a stamps once, rule 03b's `length_eq` matches at count=1, synthesize aggregates one source. No special path.

**Partial failure caveat (Phase A):** if any of the N gatherers terminates `needs_clarification` or `outcome=failed`, the counter never reaches the subtopics length and rule 03b never fires — the chain wedges visibly via rule 08's `chain.paused.marker` (for failures) or routes through rule 06 (for `needs_clarification`). Phase B may add a partial-success recovery rule; today, operator intervention or coordinator re-plan is the recovery.

Approved is terminal apart from the coordinator wake-up. `respond_direct`
publishes the user-facing reply via the existing
`configs/rules/coordinator/03b-respond-direct.json`, which is loaded
alongside this pack from the bootstrap config.

## Migration posture

When MVP-7 retires the chain.mode machinery,
`configs/rules/research-mode-transition/` and
`configs/rules/research-iterative/` delete in the same PR; this pack
takes over the research arc entirely.
