# ADR-064: Authoritative planner fan-out admission

**Status:** Proposed design (2026-10-10) for [PR #312](https://github.com/C360Studio/semteams/pull/312), addressing the
fan-out portion of [#272](https://github.com/C360Studio/semteams/issues/272). No enforcement, source extraction or
runtime activation is implemented. This refines ADR-060's schema requirement within ADR-061/062's ownership boundary.

## Context

The research planner emits an optional audit artifact through `emit_plan`, then calls `decide(action="gather")`.
Only the latter writes `coordinator.decision.subtopics`, which rule 02 consumes directly. A measured invocation of
frozen `decide` followed by the actual rule action emitted 15 gather tasks without calling `emit_plan`. Limiting audit
`epics` cannot close that path. The [design](../../openspec/changes/archive/2026-10-10-research-fanout-admission/design.md) records source
positions, measurement limits and the proving matrix.

The frozen tool registry has no task context for `ListTools`. Rule spawning resolves tool names through that global
registry; loop intake accepts a task-specific tools list and caches it. The frozen executor has no pre-mutation
validation hook. These are separate execution and schema integration boundaries, not missing generic durability.

## Decision

### 1. Admit the terminal list, using one pack policy

The proposed research policy admits **one through four subtopics**, inclusive, for the authoritative
`researcher-research-plan` role and the allowlist-resolved canonical `gather` action. Four is a conservative initial
product limit, not an empirical quality optimum. It keeps decomposition useful while bounding each planner pass to
four investigators under the existing role budgets. This choice remains a proposal in this design PR.

The pack owns that immutable policy; the agent runtime consumes it for both schema projection and execution.
The implementation must provide one policy instance/snapshot to both consumers, not separate matching literals.
No runtime-editable override, new storage, action field or policy service is proposed. Any later configurable policy
requires its own lifetime/replay contract. Other roles and other planner actions retain their existing semantics.

Validate after the existing parser, allowlist resolution and deployment restrictions, but before SAP success signals,
decision fact construction or graph mutation. Missing, null, empty, malformed and over-limit lists yield the existing
invalid-arguments result, without terminal success or writes. The planner may correct its call within its current
loop budget. Never truncate, deduplicate, split, or silently reinterpret the list: fan-out and join must see the same
admitted list. Duplicate and blank strings retain the frozen parser's behavior and count as items; evidence quality
and semantic decomposition are separate contracts.

`emit_plan` remains additive audit. Skipping it or disagreeing with its epics cannot bypass admission, but this design
does not claim artifact/decision identity. Evidence, revisions and derivation remain separate work under #271.

### 2. Own a bounded executor implementation, retaining frozen shared authority

Select a future narrow transfer of the `decide` implementation and its private parsing/action-resolution helpers,
with meaningful source tests and MIT provenance. The public `SkipBuiltins` seam can omit only `decide` alongside the
existing `bash` omission; register the selected executor once under the canonical `agentic.DecideToolName`.
Do not copy the entire tool component or build a wrapper that reparses action normalization before delegation.

Keep frozen `agentic` concrete types, vocabulary, sentinel identities, platform types and the public
`agentictools.TriplePublisher` contract. Obtain its mutation implementation through the existing public
`NewNATSTriplePublisher`; the private graph-mutation adapter stays upstream. This is agent execution ownership,
not a substrate adaptation or contract-family transfer. It does not reopen #287's shared rollback-policy hold.

Upstream `agentic-tools` is still imported and initializes its Prometheus collectors. A copied collector with the
same name would panic. The selected executor will own `semteams_decide_tool_*` collectors with the existing SAP and
restricted-action signal meanings; upstream `semstreams_decide_tool_*` series cease receiving those live events.
The implementation must record dashboard migration and prove startup with both packages imported. This is an
explicit telemetry migration, not unchanged metrics or a second active decision executor.

### 3. Require task-scoped schema projection before activation

The planner's tool definition must structurally express the `gather` list requirement and `minItems: 1`, `maxItems: 4`
from the same policy as enforcement. Preserve the base definition for other roles and the existing argument contract
for other actions. A global `subtopics.maxItems` changes unrelated roles; an action-only global condition cannot
express role scope. Persona prose and tool descriptions are not substitutes for a schema.

The selected insertion boundary is loop intake, after the effective role/tool set is established and before the
per-loop tool cache is populated. Reuse those scoped definitions on initial, retry, continuation and recovered
requests. This is a required runtime behavior, **not an existing public hook**. The implementation must establish
an accessible, qualified integration through the agent-runtime ownership lane before activation. This design does
not authorize a whole-loop copy or invent a public API signature to conceal that missing seam.

Provider adapters must preserve a supported conditional representation of the action-scoped constraint. Unsupported
schema dialects are an activation failure for that deployment; silently dropping structural constraints is not an
admitted fallback. The executor remains authoritative even when a model returns a schema-violating call.

Both boundaries ship together in a later qualified change. An executor-only or audit-only change cannot claim the
schema-enforced contract. Until the intake integration exists, keep the frozen runtime active with its known limit.

### 4. Preserve authority and the extent of the guarantee

Select policy using the role bound by the runtime to the effective loop identity, not a model argument, category
string or unqualified caller metadata. Qualify this binding across ordinary, queued, approval and recovery paths.
Unresolved or inconsistent identity must fail before decision effects; it is an infrastructure/authority error,
not a model-correctable subtopic error. Existing internal transport trust remains the boundary; no new public raw
message ingress or cryptographic identity mechanism is part of this design.

The ceiling is per admitted planner decision. It bounds neither concurrent recovery passes nor total-run attempts,
provider calls, cost, replay duplicates or exactly-once dispatch. Generic attempt/effect/replay remains SemEngine's
responsibility. No #24 dependency is implied solely by list validation. An implementation moving rule actions must
separately qualify the existing engine extension/type boundaries (#25–#27), rather than add local substitutes.

## Alternatives

| Alternative | Reason not selected |
|---|---|
| Cap `emit_plan.epics` only | Audit is optional to dispatch; the terminal list remains bypassable. |
| Add only a rule length condition | Prevents a route but can strand an already-terminal planner/run; no corrective tool result. |
| Make the audit plan authoritative | Changes artifact revisions, fact ownership and activation; unnecessary for list admission. |
| Global schema cap | Applies research policy to unrelated roles and decisions. |
| Wrapper around frozen `decide` | Must reproduce its private canonical-action logic or validate after effects. |
| Intercept mutation or task subjects | Introduces indirect policy/control machinery and error translation instead of owning admission. |
| Copy rule/loop components to add a cap | Expands the source and private-policy boundary beyond the demonstrated executor cut. |
| Generic `for_each` ceiling | A separate substrate/action contract, not research planner schema admission. |

## Consequences

The contract has a small policy surface, preserves existing mutation and terminal semantics for valid calls, and
allows an honest correction path. It also exposes a real remaining integration dependency: the task-scoped schema
projection is absent from the frozen runtime. This is design progress, not permission to force early extraction.
Implementation admission and current work status live in #272 and the draft PR, not in this ADR as a backlog.

## Related

- [ADR-060](060-program-manager-direction-and-research-design.md): research and containment direction.
- [ADR-061](061-agent-runtime-ownership-and-observable-work.md) and
  [ADR-062](062-agent-contract-extraction-boundary.md): runtime ownership and shared authority.
- [#272 assessment](https://github.com/C360Studio/semteams/issues/272#issuecomment-6101060504): reproducible bypass.
- [SemEngine #25](https://github.com/C360Studio/semengine/issues/25),
  [#26](https://github.com/C360Studio/semengine/issues/26),
  [#27](https://github.com/C360Studio/semengine/issues/27): existing rule extension/type work; not a list-validation API.
