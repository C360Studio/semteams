# ADR-035: Dev-via-Spec Arc — Chain Shape, Persona Contracts, Substance Discipline

## Status

**Proposed (2026-05-06).** Pulls the dev-via-spec arc decisions
(R3.3 → R3.5) out of ADR-031, which was growing into a diary across
seven addenda spanning two products of decisions. ADR-031 keeps the
research-flow ownership decision; ADR-035 owns the post-research
chain.

When this ADR is accepted:

- ADR-031 retains R1–R3.2 (research flow + semspec handoff +
  emission-shape framework-alignment review).
- ADR-035 inherits R3.3 (dev-via-spec port from semspec), R3.4
  (smokes #4 and #5), and R3.5 (coordinator-as-meta-reviewer,
  designed but deferred).
- Forward dev-via-spec decisions (post-builder reviewer plumbing,
  the substance-vs-stubs problem) land here unless they grow large
  enough to earn their own ADR.

This ADR uses the post-rename vocabulary (per ADR-036 §Vocabulary
table). Historical references inside quoted persona content and
smoke findings preserve the old names for fidelity.

## Context

R3.1–R3.2 (ADR-031) shipped the research arc: dispatch researcher
→ research-reviewer → researcher-with-source → mode-transition. The
output is a stable `research.Artifact` with `actors[]`,
`integration_points[]`, and `tasks[]` (was `seed_requirements[]`).
This is the input to the next arc.

R3.3 ports a dev-via-spec chain from semspec — concepts and prompt
content, not pipeline. The chain is configs only (zero Go changes
in R3.3). Five rules wire role-to-role transitions; four persona
fragment dirs carry the role contracts.

The chain shape is **per-role rigour, not per-role exhaustive
backward reach.** Each role reads only its prior loop. The
reviewer-as-enumerator pattern works because each role has one
concrete input to walk a checklist against; cross-grounding
across hops would split attention and degrade reasoning quality
on small LLMs.

R3.4 ran two smokes against real LLM. Smoke #4 (R3.4a) wedged at
22 loops with format-compliance Goodhart. Smoke #5 (R3.4b) converged
in 6 loops with substance-only persona contracts. The delta is the
substance-over-format pivot — a coefficient that came from running
the wind tunnel, not from theory.

R3.5 (coordinator-as-meta-reviewer) is designed but deferred. Smoke
#5 converged without it; smoke #7 (R3.7.2.l′) will tell us if the
builder slice surfaces ambiguity that needs escalation.

## Decision

### D1. Chain shape — five rules, four roles, one terminal

The dev-via-spec chain runs as five event-driven rules under
`configs/rules/dev-via-spec/`, each firing on the prior agent's
`decide` action:

```
research-reviewer.decide(approved)            → research-mode-transition rule_03 → planner
                                                  ↓
planner.decide(planned)                       → rule_01 → reviewer
                                                  ↓
reviewer.decide(approved)                     → rule_03 → challenger
reviewer.decide(insufficient)                 → rule_02 → planner (retry)
                                                  ↓
challenger.decide(accept)                     → rule_05 → architect
challenger.decide(concerns)                   → rule_04 → planner (retry)
                                                  ↓
architect.decide(tasks_emitted)               → rule_06 → builder (R3.6)
```

Roles: planner, reviewer, challenger, architect, builder, qa-reviewer.
The first four exist as of R3.4b; builder + qa-reviewer land in
R3.6 + R3.7 (covered by ADR-036).

### D2. Per-role rigour, not exhaustive backward reach

Each role reads **only its prior loop**. Downstream cross-grounding
against the original research artifact is not wired and is
deliberately not planned. The `decide` reason field is the
contract; rigour at the prior role is what the next role builds
on.

semstreams' rule engine does not forward arbitrary upstream entity
IDs across hops (`processor/rule/execution_context.go`); $entity.*
substitution targets the *triggering* entity only. We resist the
upstream "forward-prop" feature ask on the same logic — the
persona layer does not need it, and adding it would invite the
multi-hop cross-grounding this section argues against.

**Echo-forward is a Goodhart loader.** The tempting alternative —
instruct each persona to echo upstream IDs through `decide` reasons
so the next role can `read_loop_result` deeper into the chain —
fails on four counts: splits the LLM's attention between primary
task and bookkeeping; creates a structural proxy ("did the LLM
include the IDs?" replaces "did the LLM actually reason about
upstream constraints?"); encourages performative `read_loop_result`
calls that satisfy the contract without informing reasoning;
extra upstream content in context does not equal better grounding
when effective attention is bounded.

If chain drift surfaces in future smokes, the right mitigation is
a **structural terminal validator** (rule or thin Go checker
asserting the architect's emitted tasks cite actors that exist in
the original research artifact), not echo-forward. Structural,
deterministic, Goodhart-resistant, deferrable.

### D3. Substance over format

R3.4a's first OSH smoke ran 22 loops without reaching architect
terminal. Root cause: the dev-via-spec reviewer's checklist (ported
from semspec verbatim) demanded literal markdown sections —
`### Goal`, `### Context`, `### Scope` with bullet-list scope
declarations. The planner produced substantively complete plans as
prose; the reviewer rejected on format. Each retry was a format
chase that didn't change the plan's substance.

R3.4b ripped format compliance out of the persona contracts.
Reviewer/challenger checklists ask substance questions only —
"does the plan name actors that match the research artifact" /
"does the decomposition cover the research's integration points" /
"do the tasks have testable success criteria." The architect's
terminal emit shifted from "reformat into a numbered task list"
(ceremony) to "call `emit_dev_via_spec_artifact`" (substantive —
the tool's deterministic template is the format authority, the
persona supplies content).

**Why this matters at the design layer:** semspec's checklist
works because semspec's downstream consumers can be parsers.
Format compliance is load-bearing for them. **Our chain's
downstream is always another LLM.** Reviewer's reason is read by
the planner on retry; planner's plan is read by the next reviewer;
challenger's concerns are read by the planner on retry; architect's
terminal artifact is the only place a deterministic format earns
rent (it lands in repo as markdown, gets diffed, gets read by
humans). Until the artifact, every consumer is an LLM that extracts
substance from prose fine.

The reusable principle: **don't tell the LLM what format to follow;
give it substance to reason about.** The same shape ships in
`researcher/15-source-acquisition.md` — fragment names good reasons
to call `add_source_repo` (training data potentially stale; prompt
names a specific commit/version; substrate empty AND domain has
public canonical sources) rather than mandating the call.

### D4. Smoke #5 as the empirical anchor

| | Smoke #4 (R3.4a) | Smoke #5 (R3.4b) |
|---|---|---|
| Personas | format-compliance Goodhart | substance-only |
| Architect role | reformat into numbered task list | curator → `emit_dev_via_spec_artifact` |
| Total loops | 22 (aborted, never reached architect) | 6 (terminal artifact written) |
| Reviewer rejections | 3 of 6 | 0 of 1 |
| Challenger concerns | 3 of 3 | 0 of 1 |
| Wallclock | aborted at ~7m | 6m 21s |
| Cost (sonnet at low effort) | ~$8 | ~$1.50 |
| Final artifact | none | typed payload + markdown spec in repo |

The smoke #4 → #5 delta is the empirical confirmation. Format
compliance was a Goodhart loader the chain optimised against
without earning anything for any consumer; removing it from the
persona contracts and moving structural responsibility to the
deterministic tool-side template (where the consumer is human /
git diff / file grep) was the right shape.

### D5. Demo-line: R3.4 produces the spec; R3.6 produces the driver; R3.7 produces verification

Smoke #5's spec is intermediate output, not terminal. The OSH-class
arc this ADR series names as the north-star demo is "working driver
in repo," not "structured spec in repo."

Phasing:

- **R3.4** (this ADR): chain converges on substance; structured
  spec artifact lands in repo via `emit_dev_via_spec_artifact`.
- **R3.5** (this ADR, deferred): coordinator-as-meta-reviewer.
  `decide(action="needs_clarification", ...)` terminal routes to
  coordinator with most-capable model + workspace context. Defers
  until builder slice surfaces real ambiguity.
- **R3.6** (ADR-032 sandbox + ADR-036 test-harness): builder writes
  code, runs tests, iterates until passing.
- **R3.7** (ADR-036): qa-reviewer grades the build deliverable
  against the architect's checks. **Substance evidence requires
  the test-harness lifecycle to actually run** — addressed in
  ADR-036.

Smoke #7 (R3.7.2.l′, 2026-05-05) confirmed the full chain converges
to qa-reviewer terminal on real LLM, but the builder produced
stubs-against-stubs. ADR-036 names the missing primitive.

## R3.5 design target — coordinator as meta-reviewer

Smoke #5 converged in 6 loops without R3.5. Smoke #7 reached
qa-reviewer with `decide(needs_clarification)` correctly identifying
a chain plumbing gap (the evidence summary stub). The
needs-clarification escape valve has earned its slot.

When R3.5 lands:

- A `decide(action="needs_clarification", reason=<concrete>,
  blocking_question=<…>)` terminal routes to the coordinator role
  rather than dead-ending the chain.
- Coordinator runs on the most-capable model with workspace context
  loaded (per ADR-031 §Coordinator framing).
- Coordinator either resolves the ambiguity (issues a directive that
  re-spawns the prior role with clarifying input) or escalates to
  human approval (per ADR-030 approval-flow).
- The route is rule-driven: a new rule fires on
  `coordinator.next_action == needs_clarification` from any
  dev-via-spec role.

R3.5 ships as configs + rule additions; no framework changes. The
coordinator role already exists.

## Migration posture

- **When upstream ships `write_artifact`** (ADR-028 follow-up):
  evaluate migrating the architect's terminal output onto the
  typed artifact-store path. Replacing
  `decide(action="tasks_emitted")` with a structured `write_artifact`
  call is mechanical; persona content adapts; rules unchanged.
- **If R3.6/R3.7 surface coverage gaps**: port more from semspec —
  the failure-class taxonomy (`error_categories.json`) maps to
  negative-memory-injection in reviewer prompts; the second-round
  review (R2) maps to a dual reviewer fragment.
- **If a future external consumer wants the dev-via-spec output**
  (UI dashboard rendering plans, audit observer): add an
  `output/file` or `output/httppost` component subscribed to
  whatever the architect terminal emits; additive.

## What this ADR explicitly does NOT decide

- **Test-harness lifecycle.** Owned by ADR-036.
- **Sandbox primitive.** Owned by ADR-032.
- **Verification-runner / browser-flow.** Owned by ADR-034.
- **Coordinator-as-decision-authority cross-arc.** Owned by ADR-033
  (post-slim).
- **Naming refactor surface.** Slice 2 of the strategic pivot;
  this ADR uses post-rename vocabulary but the mechanical refactor
  across personas, configs, code, triples is its own slice.

## What ports from semspec, what doesn't

Light port — concepts and prompt content only. semspec source at
`~/Code/c360/semspec/prompt/`:

| semspec source | maps to | What ports across | What changes |
|---|---|---|---|
| `domain/software.go:336` (planner) | `dev-via-spec-planner/` | "Decompose intent into a development plan; revision path on reviewer rejection." | Input is a stable `research.Artifact`, not a freshly-shaped intent. |
| `domain/software.go:426` (plan-reviewer) | `dev-via-spec-reviewer/` | Reviewer-as-enumerator: walk an explicit checklist, decide approved/insufficient with bullet-list gaps. | One-round review (not semspec's R1+R2 split); checklist is dev-via-spec-shaped. |
| `domain/software.go:575` + `:1445` (adversarial QA) | `dev-via-spec-challenger/` | "Adversarial: find what could go wrong, not approve." Standalone role. | Operates on a planner output. Probes for decomposition coarseness, scope creep, missing integration concerns. |
| `domain/software.go:832` (architect) | `dev-via-spec-architect/` | "Map plan to actors / integration points / decisions with rationale." | Lighter — the research artifact already enumerates actors and integration points; architect ratifies into final tasks. |

What we explicitly do NOT port:

- semspec's fixed sequential processor chain (planner →
  plan-reviewer → req-generator → scenario-generator →
  scenario-reviewer). We use the coordinator + rule-driven role
  swaps pattern.
- Per-Plan KV bucket + plan-manager state machine.
- semspec's `StatusRejected` / `ReviewIteration` /
  `MaxReviewIterations` Plan struct fields. Equivalent is the rule's
  `max_iterations` field + reviewer's `decide` terminal.
- `plan.mutation.revision` event surface. Equivalent is
  `agent.complete.>` + rule-driven retry pattern.
- Two-round R1/R2 review structure. One reviewer pass per role
  transition; R3.4's smoke would have surfaced if a second round
  was needed and didn't.

## Demo discipline (load-bearing)

For "why don't you just call semspec":

> semspec's MVP scope is intent that arrives already shaped enough
> for an `ArchitectureDocument`. The whole reason SemTeams owns
> this arc is that domain research, source acquisition, and scope
> shaping are upstream of semspec's bound. The dev-via-spec mode
> ports semspec's *patterns* into a coordinator that sees the
> upstream context too. semspec is a pattern source, not a runtime
> dependency.

For "isn't this just rebuilding semspec":

> We port four prompts and one taxonomy. semspec is sixteen
> components, a per-Plan KV bucket, a state machine, and a fixed
> processor chain — all of which we explicitly do not port. The
> dev-via-spec mode is configs only.

## Consequences

### Positive

- **Chain quality emerges from per-role rigour.** Smoke #5 confirms
  the per-role pattern works without forward-prop or echo-forward.
  Each role's persona is single-purpose, easy to evolve.
- **Substance discipline transfers.** The same principle (don't
  mandate format; give the LLM substance to reason about) ships
  in `researcher/15-source-acquisition.md` and is the durable
  contribution to future personas.
- **Configs only.** R3.3+R3.4 land zero Go changes. The product
  shell stays thin per ADR-029.

### Negative

- **No cross-loop grounding.** Future smokes may surface drift
  cases where each role does its job individually but the cumulative
  output has lost coherence. Mitigation is a structural terminal
  validator, not echo-forward. Deferrable until empirically
  needed.
- **R3.5 needs_clarification escape valve still deferred.** Smoke
  #7 has earned it; lands when the builder slice consumes
  ambiguity-routing more than the current synchronous hand-off.
- **Substance evidence still requires real test-harness lifecycle.**
  Smoke #7 converged but produced stubs-against-stubs. ADR-036 ships
  the missing primitive; until then, this chain produces verifiable
  artifacts only when the builder is non-naïve.

### Neutral

- **No new payload-registry types.** `dev_via_spec.artifact.v1` is
  the architect's terminal artifact; no other arc-level types.
- **No new stream wiring.** Existing AGENT/TOOL streams carry the
  full chain.

## References

- [ADR-031](031-research-flow-and-semspec-handoff.md) — research
  flow ownership (post-slim).
- [ADR-032](032-r36-sandbox-design.md) — sandbox primitive.
- [ADR-033](033-harness-anchored-verification-and-coordinator-authority.md)
  — coordinator-as-decision-authority + multi-arc dependency
  (post-slim).
- [ADR-034](034-qa-runner-pattern-adoption.md) — verification-runner
  pattern.
- [ADR-036](036-test-harness-lifecycle.md) — test-harness
  lifecycle and verification machinery.
- `~/.claude/projects/-Users-coby-Code-c360-semteams/memory/project_strategic_pivot_2026_05_06.md`
  — strategic pivot 2026-05-06.
- `docs/specs/2026-05-02-meshtastic-mqtt-protobuf-ogc-connected-systems-bridge.md`
  — smoke #5 terminal artifact.
- `/tmp/smoke7-run3/findings.md` — smoke #7 chain-converged + stubs-against-stubs evidence.
