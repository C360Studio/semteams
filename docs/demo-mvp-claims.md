# Demo MVP Claims And Evidence

This document defines the claims SemTeams can make for the inner/outer-loop demo and the evidence that is allowed to
support them. The demo scope was reset by [ADR-058](adr/058-beta159-realignment-and-demo-lane-focus.md): the live
surface is the coordinator front door (outer loop) plus the research and autoresearch arcs (inner loop). The
spec-driven-development claims that previously lived here are parked with their packs and move to the Parked Claims
section until those packs are re-authored and re-wired.

## Supported Claims

SemTeams can route prompt classes from the coordinator front door: plain chat/direct response, clarification, research,
or autoresearch. Product-level slash commands are supported as coordinator-routed hints, not bypasses. Asks for parked
teams (spec authoring, implementation) get an honest direct response, never a silent dead-end.

SemTeams can run an evidence-gathering research arc end to end: plan, fan out parallel gatherer loops, join, synthesize,
review, and deliver an artifact whose sources are recoverable from the graph.

SemTeams can run an autoresearch optimization arc end to end: baseline a measurement, propose and execute changes in an
attested sandbox, keep only empirically better results, and deliver a reviewed rollup.

SemTeams can prove sandbox readiness fails closed: execution-bound routing waits on, or honestly surfaces, a
non-ready/denied sandbox attestation instead of dispatching work into an unprepared environment.

SemTeams can pause and resume runs across human boundaries: clarification questions and gated tool approvals park the
run with visible markers and resume it when the human replies or approves.

## Parked Claims (ADR-058)

The following previously supported claims are parked with the dev-side packs. Their configs, journeys, and tests stay
in-repo but are unwired; the claims return when the packs are re-authored under the canonical predicate contract:

- Produce, review, edit, approve, and export an OpenSpec handoff (create-change pack).
- Prove readiness before execution via claims, proof dependencies, harness profiles, readiness records, evidence, and
  waivers (proof-readiness pack).
- Implement from an approved spec or a build-with-tests prompt through plan/execute/integration gates (dev-from-task +
  dev-via-test packs), including the MAVLink-hard goal.

## Parked Regression (ADR-059)

Artifact context handoff is not a live beta.160 claim. ADR-059 decision 7 removed the trajectory evidence bodies that
fed `ArtifactCard`, the handoff panel, team buttons, and context chip. The
`artifact-context-handoff.spec.ts` journey is deliberately `describe.skip`; it returns when the evidence-fetch UI pass
dereferences each fact's `StorageReference` and restores artifact rendering.

## Non-Claims

SemTeams is not claiming it can build software in this deployment: the implementation teams are parked.

SemTeams is not counting fixture-seeded graph or NATS writes as black-box proof that the product can produce those facts
itself.

Mock-LLM journeys prove wiring and delivery, not model judgment: routing-quality claims (does the coordinator pick well
on real prompts?) require the real-LLM smoke.

## Evidence Rule

Black-box demo journeys may drive SemTeams only through public UI, dispatch HTTP, slash-command, approval, export, and
documented task-runner surfaces. They may read graph facts, trajectories, logs, downloads, and HTTP status for
assertions and evidence.

Black-box demo journeys must not write directly to NATS, graph storage, lifecycle state, proof facts, or private KV
buckets to make the claimed behavior pass.

Fixture-seeded journeys must be labeled as fixture-seeded bridge proof. They can validate adapters or future bridges,
but they do not count as pure black-box MVP evidence.

## MVP Evidence Pack

The black-box demo evidence pack is:

- `task ui:test:e2e:agentic:demo-mvp` — the aggregate: routing matrix, research arc, readiness gate, autoresearch arc
- `task ui:test:e2e:agentic:coordinator-routing-matrix`
- `task ui:test:e2e:agentic:research-mvp`
- `task ui:test:e2e:agentic:autoresearch`
- `task ui:test:e2e:agentic:coordinator-readiness-gate`

Other journeys exercise narrower or additional behavior, but they are not members of the `demo-mvp` aggregate and are
not evidence for claims beyond their own assertions.

The paid optional smoke is the real-LLM research fan-out run (see ADR-058; the create-change Gemini smoke is parked with
its pack).

## OpenSpec Lifecycle

The spec-driven development MVP change is archived at
`openspec/changes/archive/spec-driven-dev-readiness-hitl/`. Its accepted requirements are baselined in
`openspec/specs/agentic-sdd/spec.md` so future changes build on the living spec instead of mutating the seed proposal.
The unimplemented post-MVP repo-readiness initializer was removed from the active OpenSpec queue without promoting its
delta into current truth. Git history preserves the proposal; issue
[#258](https://github.com/C360Studio/semteams/issues/258) owns its resume gates and requires a freshly reconciled change
after spec-driven development is deliberately re-authored and re-wired.
