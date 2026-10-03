# Agent-contract extraction design

## Scope and evidence

[Issue #284](https://github.com/C360Studio/semteams/issues/284) and
[draft PR #286](https://github.com/C360Studio/semteams/pull/286) own this design, stacked on foundation PR #283.
The source is SemStreams `8b99efe9c66a4faa4fa509f9f62cc6bad8392128`, selected by the existing module pin. No runtime
source, dependency, config, schema or UI change is part of this design.

[ADR-062](../../../../docs/adr/062-agent-contract-extraction-boundary.md) records the ownership decision. The
[measurement record](../../../../docs/migrations/agent-contract-extraction/README.md) owns commands, source positions,
production/test closure, direct reverse references, provenance and limitations. Counts summarize those sets; they
cannot establish concrete Go compatibility or an approved copying boundary on their own.

## Selected authority boundary

The proposed root `agentic` + `vocabulary/agentic` + `internal/looptoken` family cannot transfer independently while
upstream consumers require the original concrete types. Keep the frozen root agent contract authoritative during
preparation. The factory registered for a payload identity, its decoder result and every concrete consumer must
agree on the same Go type. Structurally identical JSON or methods do not satisfy a concrete type assertion.

Contract authority also includes exported sentinels and shared policy values. `ErrToolNotFound` and
`ErrMaxIterationsReached` must preserve `errors.Is` identity through wrapping; equal text from a newly allocated error
is insufficient. `DispatchEnforcedMetadataKeys` needs one declared policy authority, not silently duplicated values.

Aliases preserve upstream type identity and may expose another import name, but do not transfer ownership. Defining
new types under that name creates the very consumer migration this design must solve. Dual registration is not a
transition strategy. Do not fork/replace the upstream module or copy engine-owned dependencies to hide the boundary.

An implementation package can instead move first while retaining frozen upstream contract imports. This changes its
implementation owner without creating a second payload authority. The exact implementation candidate, package/test
closure and proving checks must be settled from the measured comparisons and reviewed before an implementation claim.
Contract-family transfer stays held until all participating concrete consumers can move in one compatible composition.

The whole model and governance component candidates share two private dependencies with different ownership:
`internal/deliverylane` is agent-side owner reaction, explicitly separated from the engine; it can move with its source
tests while using frozen `natsclient` settlement. `internal/lifecyclecleanup` is admitted engine code but remains
private, so access to its failed-start rollback policy is unresolved. Resolve that through an accessible shared
policy or an expressly approved narrow owner adaptation; no equivalent reusable public policy was identified.
Manager-level rollback does not discharge component-owned acquired resources. Do not silently duplicate generic
policy or infer a promised public delivery-lane API. The held boundary is accessible rollback policy, not the future #24 durability work.
Retained dispatch also constructs the frozen model client; a configured-component move alone would not prove removal
of the upstream model dependency or complete package ownership transfer.

The full product registration catalog remains 27 factories; the nine factories in bootstrap configuration are a
separate declared-configuration inventory. Keep advertised metadata, payload registrations, factory names and schema
outputs unchanged. A factory that is not configured is still part of the current advertised compatibility surface.

## Engine integration requirements

| Existing boundary | Intended engine/runtime split | Integration requirement |
|---|---|---|
| Generic graph, envelopes, registries, vocabulary, projections and lifecycle | Engine dependency; no copies in the agent family | Preserve admitted generic contracts and use their actual exported APIs |
| Agent action registration | Engine action-family extension point, agent handlers in SemTeams | [E1 #25](https://github.com/C360Studio/semengine/issues/25): public registration shape and implementation remain open |
| Agent fields in authored rules | Agent-owned wire/schema validation through the extension point | [E2 #26](https://github.com/C360Studio/semengine/issues/26): raw JSON preservation versus an extensions field is unsettled |
| `publish_agent` and agent stamp/validation/toolregistry plumbing | Agent implementation outside core rule evaluation | [E3 #27](https://github.com/C360Studio/semengine/issues/27) and [#29](https://github.com/C360Studio/semengine/issues/29): migrate handlers/validation, then remove shared `ToolRegistryReader` wiring without breaking rule admission |
| `deny`/`approve` and `VerdictAuditor` | Agent governance extension, preserving the core `DenyVerdict` terminal | [E4 #28](https://github.com/C360Studio/semengine/issues/28): separate agent handlers from generic terminal semantics |
| Generic attempts, effect settlement, retries and replay | Engine durability, based on lifecycle/projection | [Latest #24 ruling](https://github.com/C360Studio/semengine/issues/24#issuecomment-5937829621); no competing SemTeams journal |

The [pinned engine contract](https://github.com/C360Studio/semengine/blob/2ec3bcf08bc1d2794d6cb1081ebd1982e08cf2c5/docs/contract.md#rules-and-entity-workflows)
requires an unknown pack action to fail at startup. That behavior is settled; the extension API's shape and
implementation are not. Existing source fields and handlers are the requirements to preserve, not a proposed new SDK.
The engine first-floor [PR #48](https://github.com/C360Studio/semengine/pull/48) is draft at the reviewed baseline.
Lifecycle phases and revision checks must not be represented as shipped beyond-phase execution/effect guarantees.

## Proving an implementation move

Before a later code change claims extraction:

1. Pin exact upstream and SemTeams source identities and source-to-destination file/test/fixture mappings. Record
   copied code's MIT copyright and permission notice, and enumerate adaptations rather than silently rewriting it.
2. Compile the production and test dependency sets under the declared platform and tags. Audit every private import
   and reverse concrete consumer; retain errors in failed or partial contexts instead of counting them as closure.
3. Prove one concrete payload authority across registration, wire decoding, constructors, assertions, projections,
   published messages and rule actions. Prove `errors.Is` against sentinel errors across the moved/retained boundary
   and preserve shared policy-value ownership. Keep the advertised catalog and generated contracts unchanged.
4. Carry the source's meaningful tests and add tests at the changed ownership boundary. Show behavior with actual
   decoded payloads and outcomes; compile success and round-trip JSON alone cannot prove compatibility.
5. Exercise lifecycle, cancellation, errors and the relevant mock journey paths for the selected component. Use fresh
   isolated infrastructure where needed and report source-state and test limits. A package move does not qualify
   retained-state migration, live-provider quality or recovery semantics it did not test.
6. Obtain independent architecture and Go review of the exact package cut, adaptations and evidence before landing.

The conditional next candidate is `agentic-model` with the agent-side delivery lane, retaining frozen contracts and
public `natsclient`. In addition to the checks above, its rollback/access decision must preserve and prove:

- canceled or deadlined parent contexts preserve values while cleanup gets a fresh five-second context budget;
  cleanup runs synchronously and must cooperate with cancellation, with no callback preemption or wall-clock bound;
- callback and cleanup-expiry errors are joined, and failed rollback retains `cleanupPending`;
- controlled stop and abort retain their distinct cleanup semantics;
- the first fatal result closes lane admission once and records health before the observer drains the exact handle;
- the handle drains once, and closed admission refuses work/terminal settlement; ordinary concurrent `Stop` retains
  its separately documented ordering limit rather than gaining an unproved health-before-drain guarantee.

No component may rely on the manager's outer rollback for its own acquired obligations. This is a proving contract
for the later, separately claimed implementation, not code authorization or proof that all cases already pass here.

This design's own qualification is reproducible analysis, source inspection, independent architecture review and
OpenSpec validation. Existing source-test passes are baseline evidence; they are not tests of an extracted package.

## Preserved limits and non-goals

- `PublishedRequestID` is a request-settlement fact, not proof that a worker is still processing it. Deferred
  continuation replay can duplicate a turn under the documented frozen recovery window (#1365).
- Approval matching depends on execution identity; replayed approvals must not authorize a later tool execution.
- Trajectory pages carry immutable fact metadata and evidence references, not full evidence bodies; #261 remains.
- Parked-tag compilation, inactive registrations and optional runtime surfaces retain their actual qualification
  status. Neither a census nor unchanged code makes an untested context pass.
- SemDev activation, Program Pulse, artifact editing, SemEngine cutover, paid runs and persisted-state conversion are
  outside this design. No algorithm rewrite, new generic journal or speculative extension API is authorized.

## Holds and follow-up

Contract authority transfer is held on a compatible transfer of its concrete consumers. Whole model/governance
implementation moves are held on an accessible shared failed-start rollback policy or an expressly approved narrow
owner adaptation; the agent-side delivery lane
needs a reviewed port and tests, not an engine API promise. Eventual engine integration is held on implemented E1–E4
and applicable #24 contracts, with unknown-action startup refusal already fixed by the target contract. These engine
holds do not automatically block implementation-only moves that retain the existing dependencies.
[Issue #287](https://github.com/C360Studio/semteams/issues/287) owns the rollback-policy agreement and conditional
model/delivery-lane cut. It does not authorize runtime copying; this design is not a parallel backlog.
