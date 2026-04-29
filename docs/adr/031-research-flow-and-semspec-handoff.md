# ADR-031: Research Flow Ownership and the SemTeams → SemSpec Handoff Contract

## Status

Proposed — 2026-04-29

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

**SemTeams owns the research capability.** The coordinator
classifies "form not yet known, needs exploration," spawns a
research flow that can iterate, modifies the SemSource substrate
(adds sources) as a side effect of execution, and stabilises on a
research-shaped artifact. The OSH-class prompt is the canonical
SemTeams use case, not a SemSpec deficit.

**The two products communicate via a `ResearchArtifact` contract.**
When SemTeams' research flow stabilises, it emits a
`ResearchArtifact` carrying:

- Pointer to the populated SemSource instance + indexed-corpus
  list.
- Seed `ArchitectureDocument` (actors and integration points
  enumerated; not full plan).
- Seed Requirements at SemSpec-decomposable granularity.

SemSpec subscribes to `agent.research.artifact.<id>`, runs its
normal pipeline against the seed, and produces issues + PRs. Each
system stays good at what it is good at; the contract is honest
about the boundary.

## Reusable patterns from SemSpec

The following SemSpec patterns port cleanly into SemTeams' research
flow:

| Pattern | Why it earns its keep in research |
|---|---|
| Reviewer-as-enumerator | Essential for "did we actually find the things we needed to find" — answers stop-condition for output quality |
| Failure-class taxonomy + negative memory injection | Research that hits the same dead end repeatedly is exactly where this prevents thrash |
| Architect role's `ActorDef` / `IntegrationPoint` emission | Precisely what the research flow needs to produce as its handoff artifact |
| Adversarial Challenger applied at the epic/issue level | "Does this issue actually represent the work, or is it confidently wrong" — guards the handoff quality |

What we **do not** port from SemSpec is the **prescribed pipeline**.
The whole point of SemTeams is that the form is not known yet; the
coordinator composes the flow rather than running a fixed one.

## What "research-shaped" means at the boundary

The contract between SemTeams and SemSpec for a stabilised
`ResearchArtifact`:

- **Sources are reachable.** The named SemSource instance contains
  every corpus the artifact references. No "we'll need OGC docs"
  TODOs left for SemSpec to discover.
- **Actors are enumerated.** Every external system / framework /
  library the work touches is named, with a one-line role
  description. (Example for OSH: the OSH driver framework, the OGC
  CS endpoints, the Meshtastic radio interface.)
- **Integration points are enumerated.** Every actor-to-actor
  interaction is named, with the data flow direction specified.
- **Seed Requirements at decomposable granularity.** Not "build a
  driver" — closer to "implement OSH `IDriver` interface backed by
  Meshtastic radio events, exposing OGC CS observation endpoints."
  SemSpec's planner can decompose; SemSpec is not expected to
  research.

If any of those are missing, the research flow has not stabilised
and must iterate. The reviewer-as-enumerator gate enforces this.

## What SemTeams owns that SemSpec cannot

- **Source acquisition.** Tools like `add_source_repo` and
  `add_source_docs` that talk to a SemSource instance, request
  indexing, return when ingestion is complete enough to query.
  Approval-gated by default — adding to shared infra is a
  side effect that warrants the human-in-the-loop pattern landed
  in [ADR-030](030-approval-flow-ui-and-identity.md).
- **Iteration with stabilisation.** Researcher loop with reviewer
  gate. Re-iterates if gaps; emits handoff when reviewer approves.
- **Coordinator action-space bounds.** The coordinator decides
  research is needed, spawns sub-flows, decides research has
  stabilised, emits handoff. Each is bounded; the *composition*
  is the unbounded-agency surface that needs guard rails. See
  the open-questions memo for the bounding strategy.

## Consequences

Positive:

- SemSpec's defensible position holds. The narrow product stays
  narrow.
- SemTeams gets the canonical use case it was architected for.
  Coordinator-as-classifier and runtime flow composition both
  light up against a real problem.
- The cross-product contract via `ResearchArtifact` is testable
  in isolation. Either system can be exercised end-to-end without
  the other.
- SemSource integration becomes a first-class concern of SemTeams
  rather than an awkward bolt-on to SemSpec.

Negative:

- The OSH-class prompt is no longer demoable end-to-end against
  SemSpec alone. Until SemTeams' research flow ships, the answer
  to early adopters is "this prompt is upstream of where SemSpec
  starts; here's how SemTeams will handle it." That is a roadmap
  statement, not a capability concession — but it requires
  message discipline.
- SemTeams now owns the unbounded-agency surface. The coordinator
  composing flows at runtime is exactly the case where governance
  questions surface — what bounds the action space, what is the
  equivalent of `submit_work` between coordinator and sub-flows,
  how do we know research has stabilised. We do not have to solve
  all of that to start, but the OSH prompt will force every
  question simultaneously.
- The `ResearchArtifact` schema is a cross-product API; it must be
  versioned and managed as such. Lives upstream in
  `semstreams/agentic/research/` so both products consume from one
  source.

## Demo discipline

When the OSH prompt comes up before SemTeams is ready, the answer
is **not** "we are adding research to SemSpec." That concedes the
architecture problem when there is not one. The answer is:

> "This prompt is upstream of where SemSpec starts. SemTeams handles
> the research and source acquisition. Once the research flow
> stabilises, SemTeams emits a ResearchArtifact that SemSpec
> consumes. Here is the timeline for SemTeams' research flow."

That is a roadmap statement. Hold the line.

## Out of scope for this ADR

- The specific bounding strategy for coordinator action space.
  Tracked in the open-questions memo
  ([`docs/proposals/research-flow-open-questions.md`](../proposals/research-flow-open-questions.md)).
  Will land as a follow-on ADR after R1 surfaces concrete examples.
- The wire-level shape of `ResearchArtifact`. Co-designed with the
  SemSpec team during phase R3.
- SemSource API surface. Co-designed with the SemSource team
  during phase R2; pinned as a stable seam before R2 code lands.
- The specific stop condition for research iteration. Tracked in
  the open-questions memo.

## Phasing (summary; full plan in proposals)

| Phase | Decision answered | Ships |
|---|---|---|
| R1 | Stop condition for research iteration | Iterative researcher with reviewer-as-enumerator gate. No SemSource yet. Bounded prompt against existing graph. |
| R2 | Action-space bounds for side effects | `add_source_*` tools, approval-gated. Researcher requests, human approves, corpus indexes. |
| R3 | Cross-product contract | `ResearchArtifact` payload + `emit_research_artifact` tool + SemSpec subscriber stub. End-to-end OSH-class demo. |

OSH is the north-star forcing function for R3. R1 starts against
a bounded prompt against an already-indexed corpus.

## References

- Strategic conversation captured 2026-04-29 (Opus 4.7, transcript
  in session memory).
- ADR-030: Approval-Flow UI — provides the human-in-the-loop
  primitive R2 builds on.
- ADR-029: Product-Shell Wiring — the framework integration
  pattern R1/R2/R3 will mirror.
- Lubin, "Bounded vs unbounded agency in AI systems" — the
  bounding-the-action-space concern this ADR forwards to the
  open-questions memo.
- SemSpec ArchitectureDocument shape — the seed format the
  `ResearchArtifact` carries.
