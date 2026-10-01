# Frozen SemStreams consumer adaptation

## Context and decisions

The user selected frozen SHA `8b99efe9c66a4faa4fa509f9f62cc6bad8392128`, resolved by module
`v1.0.0-beta.162.0.20260930150212-8b99efe9c66a`. ADR-029 continues to assign composition-root wiring to SemTeams;
framework internals remain upstream. The [architect inventory](../../../docs/migrations/semstreams-8b99efe/contract-inventory.md)
approved scoped TDD adaptation before implementation. This document records that application, not a new framework design.

Remove retired flow managers, template seed loading and runtime author/deploy calls. Use public projection contracts,
strict composition validation, one rule manager key family, and caller-owned service lifecycle. Read effective platform
authority after config startup and update owned entity builders, rule patterns and UI consumers together. Preserve the
live research/autoresearch and support packs; do not enable parked donors.

Typed dispatch owns user responses and durable loop authority. User control follows owner lookup and exact approval
execution identity. Pending/answered graph receipts must correlate loop and execution across callback order and process
replacement. The reviewed initial receipt implementation fails that ordering contract; its focused repair and independent
re-review are required. No new bucket, daemon or framework shim is approved. Fresh NATS/graph state is required because
identity and pending-marker formats change. Retained beta.160 state has no conversion or compatibility reader.

## Qualification boundary

Compile and unit gates are necessary but do not prove browser delivery. Preserve baseline/config/fixture hashes,
record observed failures and skips, and separate graph/loop/terminal/trajectory observations. The autoresearch
run-triggered rule loses typed route ancestry; its completed final coordinator does not qualify final user delivery.
No flat response publisher or invented loop predicate may hide that gap.

UI evidence-body rendering remains limited by #261. Program Pulse is not delivered by this migration. SemSource
backed dogfooding waits for SemSource readiness, and SemEngine replacement needs a separate approved consumer contract.
Compiler references, registry closure and parked-source obligations inform that contract without becoming initial
release requirements. Final documentation must state measurement methodology and incomplete contexts honestly.

## Evidence and completion

The [migration index](../../../docs/migrations/semstreams-8b99efe/README.md) links baseline comparisons, Go/browser
validation, independent reviews, dependency measurements and the terminal blocker. Final reviewer corrections and gates
must be reflected there before checking the remaining tasks. Do not archive while terminal delivery remains unresolved.
