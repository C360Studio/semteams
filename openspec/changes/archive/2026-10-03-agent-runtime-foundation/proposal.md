# Establish the SemTeams agent-runtime foundation

## Why

The owner approved SemTeams taking responsibility for the agent runtime while moving to SemEngine, and absorbing
SemDev as a development pack. Human participation should become less exhausting through observable work,
inspectable evidence, and explicit controls. The development workflow remains issue → OpenSpec → implementation →
verification → PR. SemEngine owns the shared substrate and generic durability mechanics; this change must not
create competing primitives.

[Issue #282](https://github.com/C360Studio/semteams/issues/282) owns this bounded foundation change. Draft [PR #283](https://github.com/C360Studio/semteams/pull/283) on
`codex/agent-runtime-foundation` owns its claim. The starting point is the
qualified frozen SemStreams adoption, SemTeams `d5ee63252e987f75885b110b3ba02844ab04dedd` / SemStreams
`8b99efe9c66a4faa4fa509f9f62cc6bad8392128` (#281).

## What Changes

- Record the successor architecture decision and align repository guidance with the agreed ownership boundary.
- Define future runtime/pack obligations for durable work identity, versioned artifacts and decisions, evidence
  ownership, and observable controls. Separate these design commitments from implemented capabilities.
- Replace broad framework component and payload registration with explicit SemTeams composition registration,
  preserving all 27 advertised factories and seven framework payload owner families plus existing product
  payloads. Generated interfaces and runtime behavior remain unchanged; this is not catalog pruning.
- Prove admitted production/mock compositions and retained behavior with focused tests, existing qualification,
  required local checks, and independent architecture/Go review; add frontend review if its contracts change.
- Identify the next extraction slice from concrete dependencies without copying implementation in this change.

## Non-goals

No SemEngine module switch, generic execution journal, bulk agentic extraction, SemDev activation, parked dev-pack
revival, UI implementation, paid-model run, SemSource dogfood, or production-state conversion. Research and
autoresearch remain the only live product-facing packs; Program Pulse and the absorbed development pack remain
future behavior. Existing evidence-body rendering limitation #261 and qualification limits remain explicit.

## Impact

Repository guidance and architecture/product documentation, a shared composition registration boundary, product
bootstrap and its schema generator, focused contract tests, and verification of unchanged generated interfaces.
The exact registration surface is decided in the reviewed design; no new runtime primitive is authorized.
