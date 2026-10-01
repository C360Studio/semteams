# Frozen SemStreams consumer adaptation

## Context and decisions

The user selected frozen SHA `8b99efe9c66a4faa4fa509f9f62cc6bad8392128`, resolved by module
`v1.0.0-beta.162.0.20260930150212-8b99efe9c66a`. ADR-029 continues to assign composition-root wiring to SemTeams;
framework internals remain upstream. The [architect
inventory](../../../../docs/migrations/semstreams-8b99efe/contract-inventory.md)
approved scoped TDD adaptation before implementation. This document records that application, not a new framework
design.

Remove retired flow managers, template seed loading and runtime author/deploy calls. Use public projection contracts,
strict composition validation, one rule manager key family, and caller-owned service lifecycle. Read effective platform
authority after config startup and update owned entity builders, rule patterns and UI consumers together. Preserve the
live research/autoresearch and support packs; do not enable parked donors.

Typed dispatch owns user responses and durable loop authority. User control follows owner lookup and exact approval
execution identity. Pending/answered graph receipts must correlate loop and execution across callback order and process
replacement. The initial receipt implementation failed that ordering contract; the revision-fenced pair-set projection
repairs it and has independent code approval plus actual-owner browser evidence. No new bucket, daemon or framework
shim is approved. Fresh NATS/graph state is required because
identity and pending-marker formats change. Retained beta.160 state has no conversion or compatibility reader.

## Qualification boundary

Compile and unit gates are necessary but do not prove browser delivery. Preserve baseline/config/fixture hashes,
record observed failures and skips, and separate graph/loop/terminal/trajectory observations. The historical
autoresearch run-triggered return lost typed route ancestry. The approved origin-return contract below restores
final delivery with existing primitives and passes five focused browser cases. No flat response publisher or
invented loop predicate was introduced.

UI evidence-body rendering remains limited by #261. Program Pulse is not delivered by this migration. SemSource
backed dogfooding waits for SemSource readiness, and SemEngine replacement needs a separate approved consumer contract.
Compiler references, registry closure and parked-source obligations inform that contract without becoming initial
release requirements. Final documentation must state measurement methodology and incomplete contexts honestly.

## Evidence and completion

The [migration index](../../../../docs/migrations/semstreams-8b99efe/README.md) links baseline comparisons, Go/browser
validation, independent reviews, dependency measurements and the historical terminal failure with its repair.
The final full matrix passes 24 active scenarios with five explicit skips and no source changes; new-content
generated-output checks also pass. Historical failures and the invalidated source-mutation run remain separate.


## Approved autoresearch return contract

The owner authorized a SemTeams-only repair without future SemStreams work. The architect and independent Go
reviewer approved the existing-primitives design in
[ADR-053's origin return
addendum](../../../../docs/adr/053-adoption-plan.md#addendum-2026-10-01--autoresearch-returns-through-its-run-origin)
before backend implementation. Run-local iteration state remains authoritative. Approved, clarification and
non-budgeted failure source facts cross from the correctly phased run through its exact framework-owned origin
relation. That existing origin loop spawns the return coordinator with an idempotent origin-rooted run assertion,
which avoids inherited-first anchor ambiguity and supplies native typed ancestry.

The facts are `autoresearch.reply.approved`, `autoresearch.reply.clarification` and `autoresearch.reply.failed`,
with full source-loop entity IDs. Missing, ambiguous, foreign or different-instance origins cannot forward.
Only completed success, executing clarification, or failed run states qualify for their respective returns.
The repeating iteration actions explicitly disable their independent default-three action cap so the run cap
continues to govern. No task publication shim, store, subscriber, queue or framework change is part of this repair.
Persisted replay suppression and cancellation before admission are tested; transactional publication across crashes
or cancellation after an action is selected are not promised by the existing rule engine.
