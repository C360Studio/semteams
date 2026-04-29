# ADR-031: Research Flow Ownership and the Internal Dev-via-Spec Mode

## Status

Accepted — 2026-04-29 (reframed same day from the original
"SemTeams → SemSpec handoff" framing — see Q5 in the open-questions
memo for the reframe rationale).

## Context

The c360 ecosystem ships two products that look superficially similar
but solve different problems:

- **SemSpec** — issue-to-PR planner. Takes intent that is already
  shaped enough for an `ArchitectureDocument` to be assembled, runs
  a fixed pipeline (planner → reviewer → architect → challenger),
  produces structured output. Strong on bounded plans; explicitly
  scoped at MVP to assume "form is known."
- **SemTeams** — coordinator-as-classifier with runtime flow
  composition. Coordinator classifies intent, swaps personas, and
  triggers sub-flows via rules at decision points. The coordinator
  has agency at the coordination layer — it decides *when* to
  invoke a more structured sub-flow.

Two recent observations force a strategic decision:

1. **The OSH-class prompt** ("create a driver for OpenSensorHub
   using OGC Connected Systems for Meshtastic devices") is the
   north-star ask the team keeps trying to put through SemSpec.
   SemSpec chokes on it because the prompt is doing three jobs at
   once: domain research (understand OGC CS, OSH driver patterns,
   Meshtastic protocol), source acquisition (those three corpuses
   need to land in a SemSource instance before any planning can
   happen), and scope shaping (turn "build a driver" into the right
   set of epics with the right architectural boundaries). Items 1
   and 2 are explicitly out of scope for SemSpec by design.

2. **Graph-grows-during-execution.** SemSpec's planner runs against
   a stable substrate — the graph is mostly read during execution.
   A research flow's graph *grows during execution* (sources are
   added to SemSource as a side effect of the flow). That's not a
   phase difference; it's a structural difference. Retrofitting it
   into SemSpec would violate SemSpec's planner-stability
   assumption.

The question is not "should SemSpec add a research phase." It is
"what is the right home for the work that has to happen before
SemSpec can do its job, and is that home SemTeams?"

## Decision

**SemSpec stays narrow.** SemSpec's defensible position is that it
is the right shape for issue-to-PR work where the form of the
answer is known. Adding a research phase makes SemSpec worse at
that core job (more state, more stall points, more concepts in the
user's head) and only marginally better at the harder problem,
because the real research need is unbounded and iterative.

**SemTeams owns the entire arc — research AND dev-via-spec.** The
coordinator classifies "form not yet known, needs exploration,"
spawns a research flow that can iterate, modifies the SemSource
substrate (adds sources) as a side effect of execution, stabilises
on a research-shaped artifact, then **transitions into a
SemTeams-internal dev-via-spec mode** that mirrors SemSpec's
planner / reviewer / architect / challenger arc without runtime
dependency on SemSpec.

**SemSpec is a pattern source, not a runtime dependency.** SemTeams
ports the patterns from SemSpec that earn their keep — reviewer-as-
enumerator, failure-class taxonomy, `ActorDef` / `IntegrationPoint`
emission, adversarial Challenger at the issue level — and assembles
them into a thinner dev-via-spec flow inside the same coordinator.
We read SemSpec's code, port what works, leave what doesn't. No
cross-product API to version, deprecate, or co-design.

**The boundary between research and dev-via-spec is internal.**
When research stabilises, the coordinator records a stable
`ResearchArtifact` as a SemTeams-internal payload type and switches
into dev-via-spec mode via the existing persona-swap pattern. The
artifact gates the mode transition; the same coordinator owns both
sides. No wire contract with another product.

### Why the original cross-product framing was wrong

The original ADR draft (same day) proposed `ResearchArtifact` as a
cross-product event SemSpec would consume. We chose differently
after weighing:

- **Versioning cost.** A cross-product wire contract has to be
  versioned and deprecated across two release cadences. An
  internal payload type in SemTeams is one repo's concern.
- **Velocity coupling.** SemSpec's roadmap and SemTeams' roadmap
  are independent. Coupling them at runtime makes either system's
  delivery hostage to the other's priorities.
- **Boundary placement.** "SemTeams owns research, SemSpec owns
  planning" splits the natural arc of an OSH-class prompt across
  two products with different operational maturity, ownership,
  and demo lifecycles. Single-coordinator ownership is cleaner.
- **Agency posture.** The Lubin unbounded-agency warning surfaces
  most sharply at the coordinator's composition layer. Keeping
  the whole arc inside one coordinator keeps that surface
  inspectable in one place.

The reframe is not "we couldn't make the cross-product contract
work" — it is "we shouldn't have needed it in the first place."

## Reusable patterns from SemSpec

The following SemSpec patterns port cleanly into SemTeams. The
research flow uses the first three; the dev-via-spec flow uses
all four:

| Pattern | Where it earns its keep |
|---|---|
| Reviewer-as-enumerator | Research stop condition (Q1 / Q2): "did we actually find the things we needed to find." Same pattern reapplied in dev-via-spec for "is this plan actually decomposable into work." |
| Failure-class taxonomy + negative memory injection | Both flows. Research that hits the same dead end and dev-via-spec that revisits the same architectural choice are the same shape of thrash; same mitigation. |
| `ActorDef` / `IntegrationPoint` emission (was Architect role) | Research output. Stabilised research artifacts enumerate actors + integration points; dev-via-spec consumes the same shape internally as planning seed. |
| Adversarial Challenger at issue/epic level | Dev-via-spec. "Does this issue actually represent the work, or is it confidently wrong" guards the dev-via-spec flow's plan quality before execution. |

What we **do not** port from SemSpec is the **prescribed pipeline**.
The whole point of SemTeams is that the form is not known yet; the
coordinator composes the flow rather than running a fixed one. We
port roles and patterns; we assemble our own pipeline.

Concretely, the port lives at:

- `configs/personas/fragments/dev-via-spec/` — ported persona
  fragments (reviewer, challenger, architect-light)
- `configs/dev-via-spec.json` — flow config wiring the personas
- `configs/rules/dev-via-spec/` — rules that drive transitions
  within the dev-via-spec arc (plan → review → execute)

All three layered on the existing coordinator infrastructure.
No new components required from semstreams.

## What "research-shaped" means at the internal boundary

The internal contract that gates the coordinator's transition from
research mode to dev-via-spec mode is the same shape we'd have
demanded of any cross-product handoff — the difference is the
consumer is the same coordinator, not another product:

- **Sources are reachable.** The named SemSource instance contains
  every corpus the artifact references. No "we'll need OGC docs"
  TODOs left for the dev-via-spec flow to discover.
- **Actors are enumerated.** Every external system / framework /
  library the work touches is named, with a one-line role
  description. (Example for OSH: the OSH driver framework, the OGC
  CS endpoints, the Meshtastic radio interface.)
- **Integration points are enumerated.** Every actor-to-actor
  interaction is named, with the data flow direction specified.
- **Seed Requirements at decomposable granularity.** Not "build a
  driver" — closer to "implement OSH `IDriver` interface backed by
  Meshtastic radio events, exposing OGC CS observation endpoints."
  Dev-via-spec's planning persona can decompose from there.

If any of those are missing, the research flow has not stabilised
and must iterate. The reviewer-as-enumerator gate enforces this.

Although the contract is internal, treating it as a typed payload
(rather than ad-hoc state) buys us:

- A clear gating signal for the rule that fires the mode
  transition.
- A persistent audit trail of what each stable artifact contained,
  by `loop_id`.
- Forward-compatibility if a future external consumer (an
  observer, a UI inspector, a dashboard) wants to subscribe to
  research stabilisation events without becoming a runtime
  dependency.

## What SemTeams owns end-to-end

With the cross-product handoff dropped, SemTeams owns every
capability the OSH-class arc requires:

- **Source acquisition.** Tools like `add_source_repo` and
  `add_source_docs` publishing on `graph.ingest.add.{namespace}`
  per the SemSource team's contract (Q4). Approval-gated by
  default — adding to shared infra is a side effect that warrants
  the human-in-the-loop pattern landed in
  [ADR-030](030-approval-flow-ui-and-identity.md).
- **Iteration with stabilisation.** Researcher loop with reviewer
  gate. Re-iterates if gaps; records stable artifact when reviewer
  approves.
- **Coordinator action-space bounds.** The coordinator decides
  research is needed, spawns sub-flows, decides research has
  stabilised, transitions into dev-via-spec mode, drives the
  ported planner / reviewer / challenger arc. Each step is
  bounded; the *composition* is the unbounded-agency surface that
  needs guard rails. Bounding strategy: hybrid rules + persona
  tool-allowlist + approval gates on side effects (Q3 in the
  memo).
- **Dev-via-spec execution.** Light port of SemSpec's planner /
  reviewer / architect / challenger patterns running as a
  SemTeams-internal flow. Personas under
  `configs/personas/fragments/dev-via-spec/`; flow under
  `configs/dev-via-spec.json`; rules under
  `configs/rules/dev-via-spec/`.

## Consequences

Positive:

- SemSpec's defensible position holds. The narrow product stays
  narrow. We don't expand it under demo pressure.
- SemTeams gets the canonical use case it was architected for.
  Coordinator-as-classifier, runtime flow composition, agency at
  the coordination layer, persona-driven mode transitions all
  light up against a real problem.
- No cross-product API. Versioning, deprecation, schema drift,
  cross-team coordination — all dropped. SemTeams ships on its
  own cadence.
- Single coordinator owns the whole arc. The Lubin
  unbounded-agency surface is in one inspectable place; bounding
  strategy (rules + persona allowlist + approvals) applies
  uniformly.
- SemSource integration is a first-class SemTeams concern, with
  the contract pinned by the SemSource team's reply (Q4).

Negative:

- SemTeams now owns more code than originally scoped. The
  dev-via-spec flow port is real work, not a thin wrapper around
  a SemSpec call. Light port (Q5 option B) keeps it bounded but
  it is still meaningful effort in R3.
- We have to read SemSpec carefully to extract the patterns
  worth porting, while resisting the temptation to port the
  prescribed pipeline along with them. That requires discipline,
  not just engineering.
- The OSH-class prompt is not demoable end-to-end until R3
  completes. Until then, the answer to early adopters is "this
  whole arc lives in SemTeams; here's the timeline; here's the
  bounded R1 prompt we're starting with." That is a roadmap
  statement, not a capability concession — but it requires
  message discipline.
- If SemSpec eventually wants to consume some shape of stable
  research artifact for its own purposes, we will need to design
  a cross-product subscriber surface at that point. The internal
  `ResearchArtifact` payload is forward-compatible (typed
  payload, stable subject, persistent audit trail) but is not
  yet an external contract. That is the correct posture today.

## Demo discipline

When the OSH prompt comes up before R3 is ready, the answer is
**not** "we are adding research to SemSpec." That concedes an
architecture problem that does not exist. The answer is:

> "SemTeams runs the whole arc — research, then dev-via-spec —
> inside a single coordinator. SemSpec is a pattern source we
> ported from, not a runtime dependency. This prompt is exactly
> what SemTeams was architected for. Here is the R1 → R2 → R3
> timeline; here is the bounded prompt R1 is starting with."

That is a roadmap statement. Hold the line.

When the question "why doesn't SemSpec just do this" comes up:

> "SemSpec's MVP scope is intent that arrives already shaped enough
> for an ArchitectureDocument. Domain research, source acquisition,
> and scope shaping are upstream of that bound. Adding them to
> SemSpec makes it worse at its core job and only marginally
> better at the harder problem. SemTeams owns the upstream arc;
> SemSpec stays narrow."

## Out of scope for this ADR

- The specific bounding strategy for coordinator action space at
  scale. Hybrid (rules + persona allowlist + approvals) is the
  decision; concrete bounds per decision point will land as a
  follow-on ADR after R1 surfaces real examples.
- The internal `ResearchArtifact` payload's exact field set.
  Land it in R1 with the minimum the reviewer-as-enumerator
  needs to gate on; expand in R3 as the dev-via-spec consumer
  surfaces requirements.
- The dev-via-spec flow's exact persona / rule / role layout.
  Pinned in R3; the light-port commitment (Q5 option B) bounds
  the scope but does not pre-decide the layout.

## Phasing (summary; full plan in proposals)

| Phase | Decision answered | Ships |
|---|---|---|
| R1 | Stop condition for research iteration (Q1, Q2, Q3) | Iterative researcher with reviewer-as-enumerator gate. No SemSource yet. Bounded prompt against existing graph. Stable artifact recorded but not yet consumed. |
| R2 | Action-space bounds for side effects (Q4) | `add_source_*` tools publishing on `graph.ingest.add.{namespace}` per the SemSource contract, approval-gated. Researcher requests, human approves, corpus indexes. |
| R3 | Internal dev-via-spec mode (Q5 reframed) | Coordinator transitions on stable research artifact into ported dev-via-spec persona set. Personas + rules + flow templates layered on existing infrastructure. End-to-end OSH-class demo. |

OSH is the north-star forcing function for R3. R1 starts against
a bounded prompt against an already-indexed corpus.

## References

- Strategic conversation captured 2026-04-29 (Opus 4.7,
  transcript in session memory). The original cross-product
  framing and the same-day reframe both originated here.
- Open-questions memo:
  [`docs/proposals/research-flow-open-questions.md`](../proposals/research-flow-open-questions.md).
  Q5 captures the reframe rationale in detail.
- ADR-030: Approval-Flow UI — provides the human-in-the-loop
  primitive R2 builds on for source-acquisition gates.
- ADR-029: Product-Shell Wiring — the framework integration
  pattern R1/R2/R3 will mirror.
- Lubin, "Bounded vs unbounded agency in AI systems" — the
  bounding-the-action-space concern that informed Q3's hybrid
  recommendation.
- SemSpec patterns to port: reviewer-as-enumerator,
  failure-class taxonomy, `ActorDef` / `IntegrationPoint`
  emission, adversarial Challenger. Read SemSpec's code for the
  shape; port what earns its keep, leave the prescribed
  pipeline.
