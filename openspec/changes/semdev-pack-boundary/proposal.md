# Define development-pack absorption admission

## Why

[Issue #290](https://github.com/C360Studio/semteams/issues/290) owns the design for absorbing SemDev as the development
pack under ADR-061. The workflow remains issue → OpenSpec → approval → implementation → measurement/review →
clean-room verification → PR. Before implementation, its actual authorities, guarantees, rejection paths and shared
runtime dependencies need a source-backed transfer contract.

This change starts from foundation #283 at SemTeams `042193463e3b27fb1996ed4e99bf2b0cd4024a06`, with SemStreams
frozen at `8b99efe9c66a4faa4fa509f9f62cc6bad8392128`. Accepted SemDev source is pinned to
`7a22bc7422c978992977be6e168bca8c3dba138b` on SemStreams beta.160. The donor's local draft sealed-run and uncommitted
verification/UI work are separate, unaccepted evidence; neither its guarantees nor its working tree become the baseline.
The draft PR on `codex/semdev-pack-boundary` will own this claim; its link is recorded after creation.

## What changes

- Trace the committed donor workflow through rules, tools, fact writers, authoritative artifacts and meaningful tests.
- Define development-pack, shared-agent-runtime and generic-engine ownership while preserving harness-owned outcomes,
  revision-bound human decisions, independent verification, bounded execution and governed external effects.
- Record the beta.160-to-frozen-runtime gap and distinguish implemented donor behavior, pending work and target
  admission requirements. Preserve one authority for shared types, sentinels and policy.
- Map actual engine needs to existing work. Prepare concise PM questions for unresolved choices or evidence-backed
  upstream issues for uncovered gaps after checking for duplicates; invent no API or local workaround.
- Obtain independent architecture/source review and define acceptance and rejection evidence for a later implementation.

## Non-goals

No runtime copying, dependency switch, shared-policy duplication, SemEngine API invention, pack activation, parked-pack
revival, UI implementation, paid run, external developer workflow execution or retained-state conversion. Do not edit
the donor checkout or present pending donor work as shipped behavior. Research and autoresearch remain SemTeams' only
live product-facing packs; Program Pulse and development-pack activation remain targets.

## Impact

A bounded architecture/admission design, source-pinned evidence and a `development-pack-admission` specification.
This is not an implementation backlog or a second migration-status system. GitHub issues own sequencing and holds;
no release milestone membership, merge or issue closure is implied. `CONFIRM-CLOSE` remains required.
