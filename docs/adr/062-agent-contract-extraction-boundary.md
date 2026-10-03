# ADR-062: Agent-contract extraction boundary

**Status:** Accepted design (2026-10-03), following independent architecture review for
[PR #286](https://github.com/C360Studio/semteams/pull/286), addressing
[issue #284](https://github.com/C360Studio/semteams/issues/284). This decision admits an extraction design, not runtime
copying, a module switch or shipped ownership transfer. It refines [ADR-061](061-agent-runtime-ownership-and-observable-work.md).

## Context

The foundation makes SemTeams' frozen registration surface explicit while retaining SemStreams implementations.
The next candidate was root `agentic`, `vocabulary/agentic` and private helper `internal/looptoken` at SemStreams
`8b99efe9c66a4faa4fa509f9f62cc6bad8392128`. Its own forward dependencies alone cannot establish a safe cut:
processors, tools, rules and product code consume the original concrete Go types.

A Go type defined in a new package is distinct from an identically shaped type in the old package. Equal JSON fields,
domain/category/version names or method sets do not make concrete assertions and typed function parameters accept
both. The current root `agentic` registrar owns 20 payload registrations, including two projection contracts. Moving
the definitions independently would leave the registry and consumers disagreeing about the runtime value they share.
The [measurement record](../migrations/agent-contract-extraction/README.md) separates forward production/test closure,
direct reverse consumers and the concrete boundaries that make the difference.

The complete advertised catalog remains 27 factories. The nine factories named by production/mock configuration are
a different inventory, not permission to disregard inactive but advertised concrete consumers. This decision narrows
neither catalog nor product behavior. Research and autoresearch stay live; development packs remain parked.

## Decision

### 1. Hold independent transfer of the contract family

Keep `github.com/c360studio/semstreams/agentic` authoritative for concrete agent types while frozen upstream
consumers still require them. Do not extract the three-package family in isolation, register a competing set of
lookalike types, or represent a type-alias facade as completed ownership transfer.

An alias can preserve the old type's identity, but its definition and authority still live upstream. A second defined
type creates a boundary to migrate; registering it under the same payload identity does not migrate its consumers.
The eventual transfer must account for every participating consumer and move payload construction, decoding,
projections and concrete use to one authority in a single compatible composition. Package-level sentinels and shared
values are authority too: recreating `ErrToolNotFound` or `ErrMaxIterationsReached` with equal error text breaks
`errors.Is` identity; re-exporting the old error preserves upstream object authority. Preserve the policy ownership
of shared values such as `DispatchEnforcedMetadataKeys` rather than silently creating a second copy.

This does not require all agent implementation to wait. A bounded implementation package may move first while
continuing to import the frozen contract types, provided its real production and test dependencies permit the move.
That is an implementation-ownership change, not a contract-family transfer. Whole `agentic-model` and
`agentic-governance` candidates both depend on private helpers whose access and ownership must be resolved. Their
private imports are not blanket evidence that those helpers belong to the engine. The exact cut and proving tests
require a separate implementation claim after this design is reviewed.

### 2. Keep engine dependencies outside the extraction

| Boundary | Authority retained during preparation | Requirement for later transfer |
|---|---|---|
| Agent message definitions and payload factories | Frozen `agentic`, including its registered type/version identities | One concrete type authority for decoding, construction and all consumers |
| Loop/lesson projection contracts | Frozen agent contracts over shared projection interfaces | Preserve owner/group/predicate semantics and register each contract once |
| Sentinel errors and shared policy values | Frozen root `agentic` objects | Preserve `errors.Is` through wrapping and keep one declared authority for shared policy |
| Agent vocabulary and loop token rules | Their frozen definitions and actual consumers | Move with demonstrated private-import and contract closure; preserve semantics |
| Message envelopes, payload registry, generic vocabulary, projection and lifecycle | Shared substrate dependencies | Import admitted engine contracts; do not copy them into an agent-only package |
| Agent rule actions and authoring validation | Frozen rule implementation until the engine extension seam is agreed | Public action-family registration/admission contract with tests; no invented API |
| Generic attempt ownership, effect settlement and replay | Existing frozen behavior; future engine contract | Map to implemented SemEngine durability rather than add a competing journal |

SemEngine's [pinned contract](https://github.com/C360Studio/semengine/blob/2ec3bcf08bc1d2794d6cb1081ebd1982e08cf2c5/docs/contract.md)
and [latest #24 ruling](https://github.com/C360Studio/semengine/issues/24#issuecomment-5937829621) establish the intended
boundary. Lifecycle and projection form the durability foundation after 04A change 2; generic attempt/effect/replay
work remains planned. The first-floor work in [PR #48](https://github.com/C360Studio/semengine/pull/48) is draft at this
design's review baseline. None of those facts makes an engine package a shipped SemTeams dependency.

Two private-helper cases must remain distinct. SemEngine's
[pinned admission decision](https://github.com/C360Studio/semengine/blob/2ec3bcf08bc1d2794d6cb1081ebd1982e08cf2c5/openspec/changes/archive/2026-10-01-setup-03b-contract-boundary/design.md#d4-port-set-admission-or-separation-of-each-questioned-package)
separates `internal/deliverylane` with the agentic domain, while admitting `internal/lifecyclecleanup` under the
engine's private namespace. The delivery lane reacts to transport results as an owner: close admission, record the
reason, drain the exact handle and join. Its
[frozen migration contract](https://github.com/C360Studio/semstreams/blob/8b99efe9c66a4faa4fa509f9f62cc6bad8392128/docs/operations/migration-restart-safe-nats-client.md#L82-L101)
permits an adopter to implement that owner reaction and promises no public helper API. Moving it with meaningful
tests and frozen `natsclient` can fit agent-runtime ownership; generic delivery settlement stays a substrate concern.

The unresolved whole-component cut is access to the shared failed-start rollback policy currently implemented by
`internal/lifecyclecleanup`. Resolve it through an accessible shared policy or an expressly approved narrow owner
adaptation; no equivalent reusable public policy was identified. A manager's outer rollback does not discharge a
component's own acquired obligations. Do not silently copy the generic helper or create a second generic policy. Do not
mislabel the delivery lane as a promised engine export, or treat future #24 as a prerequisite for an implementation
move that leaves existing recovery semantics unchanged.

Phase persistence and revision checks do not establish exactly-once outside effects, recovery of unfinished attempts
or replay semantics. Preserve current behavior and its limitations until each replacement contract exists and has
consumer proof. The unresolved public rule-action extension API remains a real integration hold.

### 3. Carry behavior and provenance before redesign

Every later extraction must identify its source files and test fixtures at the frozen SHA, retain their algorithms
and meaningful tests, and record intentional adaptations. Preserve the upstream MIT copyright and permission notice
with copied substantial portions, together with a source-to-destination manifest. A package rename or local test pass
alone is not evidence of behavioral equivalence.

Qualification must cover the package's concrete boundaries, advertised registration, error/cancellation behavior and
relevant live journeys. Production and test import closure must both be checked, including private imports and build
tags. Failed or partial tagged contexts remain failed or partial evidence. Do not silently repair unrelated donor
code to report a green closure.

No retained-state conversion, durability improvement, new UI control, SemDev activation or paid-model behavior is
claimed by this design. Existing evidence-body, continuation, recovery and parked-test limits remain explicit.

## Consequences

- The proposed small contract-family cut is not independently admissible. Its single-authority consumer transfer
  remains held; a small forward dependency set cannot waive that hold.
- `agentic-model` with its agent-side delivery lane is a conditional next implementation candidate, retaining frozen
  contract types and public `natsclient`. Resolve failed-start rollback first and preserve the old model dependency
  still used by dispatch. No exact implementation cut is admitted by this design.
- Implementation ownership can advance on frozen shared types where measured closure and proving tests support it.
  This avoids a speculative module fork, copied substrate or bulk agent-runtime move.
- The engine action-family API and generic durability contracts need concrete consumer agreement before their
  dependent migrations. They do not block a proven package move that leaves those boundaries unchanged.
- [Issue #287](https://github.com/C360Studio/semteams/issues/287) owns the failed-start rollback agreement and
  conditional model/delivery-lane cut. This ADR records the decision; evidence preserves how it was reached without
  creating a separate migration status system.

## Alternatives considered

- **Copy the contract family and keep the same JSON.** Rejected: concrete Go type consumers do not become compatible
  because their wire representations match.
- **Alias everything under a SemTeams path and call the transfer complete.** Rejected as an ownership claim: aliases
  retain upstream identity and implementation. An alias is not a second authoritative contract.
- **Register both concrete families.** Rejected: one payload identity must select one concrete factory; duplicate
  registration cannot make incompatible consumers agree.
- **Fork or replace the SemStreams module, or copy shared engine packages to close imports.** Outside authorization
  and contrary to the shared-substrate boundary.
- **Copy the entire reverse consumer group immediately.** Not justified by the candidate's size. Measure a bounded
  implementation-first cut and qualify the eventual contract transfer separately.

## Related

- [ADR-061](061-agent-runtime-ownership-and-observable-work.md): runtime ownership and observable-work direction
- [Foundation qualification](../migrations/agent-runtime-foundation/README.md)
- [Extraction measurement and limitations](../migrations/agent-contract-extraction/README.md)
- [Issue #284](https://github.com/C360Studio/semteams/issues/284): design scope and authority
