# Adopt the frozen SemStreams migration baseline

## Why

The starting SemTeams baseline imports SemStreams `v1.0.0-beta.160`. The owner requested independent migration to
`v1.0.0-beta.162.0.20260930150212-8b99efe9c66a`, frozen SHA
`8b99efe9c66a4faa4fa509f9f62cc6bad8392128`, alongside SemEngine development.
Issue [#280](https://github.com/C360Studio/semteams/issues/280) owns the work; draft PR
[#281](https://github.com/C360Studio/semteams/pull/281) on `codex/semstreams-frozen-8b99efe` owns the claim. Earlier draft #270 is historical
beta.161 evidence, including the final-coordinator delivery failure, not proof for this target.

## What Changes

- Capture the beta.160 baseline and inventory contracts before dependency adaptation.
- Pin the exact frozen module and adapt product composition using behavioral tests.
- Qualify routing, research fan-out/join/review, autoresearch, approval/sandbox boundaries, terminal states,
  cancellation, trajectories, and graph/UI behavior against fresh isolated infrastructure.
- Record exact configuration, command, source, and fixture identities and classify observed differences as
  intended upstream change, existing limitation, or reproduced defect.
- Measure required framework symbols and dependency closure as SemEngine consumer evidence.
- Obtain independent Go and Svelte/TypeScript reviews of compatibility changes and qualification limits.

## Non-goals

No SemEngine replacement, automatic expansion of its first release, SemSource-backed dogfooding, paid-model
qualification, production-state migration/wipe, parked dev-pack revival, or Program Pulse feature delivery.
The live product packs remain research and autoresearch. Evidence bodies and rich artifact/context handoff
remain limited by #261 unless a separately approved evidence-fetch contract is implemented.
SemTeams can complete this SemStreams migration independently. A later SemEngine switch needs its own
approved consumer contract; SemSource-backed dogfooding waits for SemSource readiness.

## Impact

Product-shell lifecycle/bootstrap, framework registries and tool wiring, strict configs, live rules, graph
and trajectory consumers, generated schemas, and UI/browser qualification. No new framework primitives
are authorized by this migration. Fresh NATS storage is used for qualification; retained production state
requires a separate recovery or migration decision.

## Current stop-point

The branch pins the exact frozen module and records beta.160 baseline evidence, compatibility changes, generated
schemas and independent reviews. [The migration record](../../../docs/migrations/semstreams-8b99efe/README.md)
indexes the source/configuration hashes, gate snapshots and observed differences. Autoresearch final typed delivery
remains blocked. Approval ordering is repaired and independently approved; final local Go/frontend gates pass.
The isolated browser matrix completes with 20 passes, one autoresearch delivery failure and five explicit skips.
Post-commit generated-output checks pass; hosted PR checks are recorded on the draft PR. Do not archive this change or
assert adoption complete while final typed autoresearch delivery remains unresolved.
