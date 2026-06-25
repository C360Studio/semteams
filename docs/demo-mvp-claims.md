# Demo MVP Claims And Evidence

This document defines the claim SemTeams can make for the spec-driven development demo and the evidence that is allowed
to support it.

## Supported Claims

SemTeams can produce, review, edit, approve, and export an OpenSpec handoff.

SemTeams can prove readiness before execution by modeling claims, proof dependencies, harness profiles, readiness
records, evidence, and waivers before implementation work is released.

SemTeams can route prompt classes to the right team: research, OpenSpec authoring, autoresearch, dev-via-test, direct
response, or clarification.

## Non-Claims

SemTeams is not yet claiming sponsor-ready full brownfield implementation from an arbitrary approved OpenSpec change.

SemTeams is not claiming it can infer and build missing infrastructure from prose without explicit proof dependencies,
harness profiles, readiness records, or human waivers.

SemTeams is not counting fixture-seeded graph or NATS writes as black-box proof that the product can produce those facts
itself.

The spec-to-dev bridge remains useful, but it is not a pure black-box demo claim until proof inventory is produced
through governed SemTeams surfaces.

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

- `task ui:test:e2e:agentic:coordinator-routing-matrix`
- `task ui:test:e2e:agentic:create-change`
- `task ui:test:e2e:agentic:coordinator-readiness-gate`
- `task ui:test:e2e:agentic:mavlink-hard-spec`
- `task ui:test:e2e:agentic:demo-mvp`

The fixture-seeded bridge proof is:

- `task ui:test:e2e:agentic:spec-to-dev-demo`

The paid optional smoke is:

- `task ui:test:e2e:agentic:create-change:gemini-smoke`

## MAVLink-Hard Goal

The MVP goal for the SemSpec `mavlink-hard` prompt is to produce, review, and export a solid OpenSpec handoff. The spec
must name the OSH/MAVSDK/Connected Systems API scope, PX4 SITL or equivalent proof dependencies, harness readiness
requirements, implementation tasks, and acceptance commands.

Developing from that spec is a stretch goal. It should not release code work until the required harness profile and fresh
readiness records exist, or an operator records a bounded waiver with visible residual risk.

## OpenSpec Lifecycle

The active OpenSpec change remains open while the demo evidence pack is still being added. After the evidence pack lands
green on `main`, the change should be archived into baseline OpenSpec specs so future changes can build on it instead of
continuing to mutate the seed proposal.
