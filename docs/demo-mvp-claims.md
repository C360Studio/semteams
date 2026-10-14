# Demo MVP Claims And Evidence

This document defines the claims SemTeams can make for the inner/outer-loop demo and the evidence that is allowed to
support them. The demo scope was reset by [ADR-058](adr/058-beta159-realignment-and-demo-lane-focus.md) and aligned
with the [Program Manager](product/program-manager.md) product direction: the MVP demo surface is the coordinator
front door (outer loop), the research arc (inner loop), and fail-closed sandbox readiness. The `autoresearch` pack
remains wired on disk as a retained framework demonstration, not an MVP product claim or member of the `demo-mvp` /
required `test:e2e` gates. The spec-driven-development claims that previously lived here are parked with their packs
and move to the Parked Claims section until those packs are re-authored and re-wired.

## Frozen migration qualification

The beta.160 baseline and frozen target are compared in the
[migration record](migrations/semstreams-8b99efe/README.md). The module is pinned to
`v1.0.0-beta.162.0.20260930150212-8b99efe9c66a` ([PR #281](https://github.com/C360Studio/semteams/pull/281)).
A loop count, terminal graph phase, or logger completion is insufficient evidence of user delivery; terminal delivery
through the origin coordinator and raw-evidence inspection are asserted directly in the active browser journeys.

## Supported and bounded MVP claims

SemTeams can route prompt classes from the coordinator front door: plain chat/direct response, clarification, or
research (plus routing to the retained `autoresearch` framework demonstration when explicitly requested). Product-level
slash commands are supported as coordinator-routed hints, not bypasses. Asks for parked teams (spec authoring,
implementation) get an honest direct response, never a silent dead-end.

SemTeams can run an evidence-gathering research arc end to end: plan, fan out parallel gatherer loops with run-owned
subtopic accounting, join, synthesize, emit a structured research artifact, review, and publish a typed final
coordinator response correlated to the origin run and inspectable in the UI's raw-evidence message log. Source facts
are recoverable from the graph; this does not claim full artifact-body rendering in the UI.

SemTeams can prove sandbox readiness fails closed: execution-bound routing waits on, or honestly surfaces, a
non-ready/denied sandbox attestation instead of dispatching work into an unprepared environment.

## Retained Framework Demonstration (Non-MVP)

The `autoresearch` pack (`configs/rules/autoresearch/`, `configs/personas/fragments/*autoresearch*`, and
`test/contract/autoresearch_*`) remains wired in `flow-bootstrap.json` as a framework demonstration of baseline
measurement, bounded proposal/execution in an attested sandbox, empirical keep/revert decisions, synthesis, review,
and origin-coordinator wake-up. It is not part of the Program Pulse MVP product claim or the required `demo-mvp` /
`test:e2e` browser aggregates; its browser journeys (`task ui:test:e2e:agentic:autoresearch` and
`task ui:test:e2e:agentic:autoresearch-guardrails`) remain runnable on demand.

## Parked Claims (ADR-058)

The following previously supported claims are parked with the dev-side packs. Their configs, journeys, and tests stay
in-repo but are unwired; the claims return when the packs are re-authored under the canonical predicate contract:

- Produce, review, edit, approve, and export an OpenSpec handoff (create-change pack).
- Prove readiness before execution via claims, proof dependencies, harness profiles, readiness records, evidence, and
  waivers (proof-readiness pack).
- Implement from an approved spec or a build-with-tests prompt through plan/execute/integration gates (dev-from-task +
  dev-via-test packs), including the MAVLink-hard goal.

## Parked Regression (ADR-059)

Artifact context handoff is not a live claim, including at the frozen migration target. ADR-059 decision 7 removed the trajectory evidence bodies that
fed `ArtifactCard`, the handoff panel, team buttons, and context chip. The
`artifact-context-handoff.spec.ts` journey is deliberately `describe.skip`. Git history preserves the removed OpenSpec
change. Issue [#261](https://github.com/C360Studio/semteams/issues/261) owns the authorized evidence-fetch UI contract
and freshly reconciled change required to dereference each fact's `StorageReference` and restore artifact rendering.

## Non-Claims

Program Pulse is the approved target MVP, not a delivered slice. SemSource-backed dogfooding is held until SemSource
is ready. A future SemEngine switch requires its own consumer contract; this migration does not expand its first release.

Runtime saved-flow authoring/deployment is retired. `/admin/flows` displays admitted composition, not current liveness.
Neither HTTP submission acknowledgement nor successful cancellation acknowledgement proves a terminal outcome.

SemTeams is not claiming it can build software in this deployment: the implementation teams are parked.

SemTeams is not counting fixture-seeded graph or NATS writes as black-box proof that the product can produce those facts
itself.

Mock-LLM journeys prove wiring and delivery, not model judgment: routing-quality claims (does the coordinator pick well
on real prompts?) require the real-LLM smoke.

## Evidence Rule

Black-box demo journeys may drive SemTeams only through public UI, dispatch HTTP, slash-command, and documented
task-runner surfaces. They may read graph facts, trajectories, logs, downloads, and HTTP status for assertions and
evidence.

Black-box demo journeys must not write directly to NATS, graph storage, lifecycle state, proof facts, or private KV
buckets to make the claimed behavior pass.

Fixture-seeded journeys must be labeled as fixture-seeded bridge proof. They can validate adapters or future bridges,
but they do not count as pure black-box MVP evidence.

## MVP Evidence Pack

The black-box demo evidence pack is:

- `task ui:test:e2e:agentic:demo-mvp` — the aggregate: routing matrix, research arc (single-branch and parallel fan-out), readiness gate
- `task ui:test:e2e:agentic:coordinator-routing-matrix`
- `task ui:test:e2e:agentic:research-mvp`
- `task ui:test:e2e:agentic:research-fanout`
- `task ui:test:e2e:agentic:coordinator-readiness-gate`

Other journeys (including the opt-in `task ui:test:e2e:agentic:autoresearch` and
`task ui:test:e2e:agentic:autoresearch-guardrails` framework demonstrations) exercise narrower or additional behavior,
but they are not members of the `demo-mvp` aggregate and are not evidence for MVP claims beyond their own assertions.

The paid optional smoke is the real-LLM research fan-out run (see ADR-058; the create-change Gemini smoke is parked with
its pack).

## OpenSpec Lifecycle

The spec-driven development MVP change is archived at
`openspec/changes/archive/spec-driven-dev-readiness-hitl/`. Its accepted requirements are baselined in
`openspec/specs/agentic-sdd/spec.md` so future changes build on the living spec instead of mutating the seed proposal.
The unimplemented post-MVP repo-readiness initializer was removed from the active OpenSpec queue without promoting its
delta into current truth. Git history preserves the proposal. Issue
[#258](https://github.com/C360Studio/semteams/issues/258) owns that retirement deliverable; issue
[#260](https://github.com/C360Studio/semteams/issues/260) owns any future reintroduction and requires a freshly
reconciled change after spec-driven development is deliberately re-authored and re-wired.
