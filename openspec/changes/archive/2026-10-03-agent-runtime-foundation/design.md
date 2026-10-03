# Agent-runtime foundation design

## Context and scope

[Issue #282](https://github.com/C360Studio/semteams/issues/282) and
[PR #283](https://github.com/C360Studio/semteams/pull/283) establish the ownership boundary before implementation
moves out of SemStreams. The starting point is SemTeams `d5ee63252e987f75885b110b3ba02844ab04dedd`, including
PR #281, with SemStreams frozen at `8b99efe9c66a4faa4fa509f9f62cc6bad8392128`.

The code change replaces aggregate framework registration with a shared, explicit product catalog using the same
frozen implementations. It preserves all 27 component factories and the payload registrations from seven upstream
owners: `message`, `agentic`, `processor/gated-dag`, `storage/objectstore`, `governance`, `pkg/lifecycle`, and
`graph/inference`. Product payload owners `research`, `devviaspec`, and `semsource` remain registered as before.
Registration availability does not activate a pack or component. Production and mock bootstrap compositions, the
research/autoresearch routing boundary, public schemas, and parked development packs retain their current behavior.

No runtime implementation is extracted here. The catalog is a composition boundary, not a new workflow SDK, plugin
protocol, or interface abstraction. It makes a later extraction reviewable without claiming reduced dependencies.

## Ownership decisions

[ADR-061](../../../../docs/adr/061-agent-runtime-ownership-and-observable-work.md) records the successor decision:

| Layer | Target responsibility |
|---|---|
| SemEngine | Graph, transport, mutations, rules, lifecycle/projection, and generic durable-execution mechanics |
| SemTeams agent runtime | Agent loop, model/tool orchestration, context/memory assembly, dispatch, agent approvals and governance, trajectories, and agent-specific rule-action extensions |
| SemTeams packs and UI | Domain policy, personas, tools and evidence, the future SemDev development pack, program-management capabilities, and observable operator journeys |

SemEngine's [contract](https://github.com/C360Studio/semengine/blob/2ec3bcf08bc1d2794d6cb1081ebd1982e08cf2c5/docs/contract.md) describes its intended first
release. Generic attempts, settlement/effect guards, typed recovery and replay are planned in
[SemEngine #24](https://github.com/C360Studio/semengine/issues/24#issuecomment-5937829621), beyond the initial lifecycle phase contract. Their
planned foundation is lifecycle/projection; neither that plan nor graph persistence proves durable outside effects.
The public agent rule-action extension API is unsettled. This change invents neither its signatures nor substitutes
for missing mechanics. Existing agent-specific recovery remains in the frozen dependency until a separately
qualified migration maps it to implemented engine contracts.

## Explicit registration boundary

`internal/runtimecatalog.RegisterComponents` supplies component registration to the product binary, schema/OpenAPI
generators and relevant contract tests. `RegisterPayloads` supplies the frozen upstream payload catalog to runtime
and tests; bootstrap continues to layer the existing product-owned payloads separately. Callers continue to own registries and
initialization ordering. Direct upstream owner registration replaces broad aggregate registrar calls; the catalog
returns useful errors with the failing registration identified.

The accepted compatibility set is the full frozen catalog, including currently inactive factories. Narrowing to the
bootstrap alone would silently change advertised schemas and other supported registration consumers. Any later
removal needs a separate behavioral decision and interface qualification. The foundation must preserve names,
registration metadata, payload type/version pairs, lifecycle registration, and product-owned payload handling.
Generated schemas and OpenAPI outputs should remain unchanged; a difference is a defect to explain and repair, not
an incidental cleanup.

Focused tests compare the explicit catalog with the frozen baseline and prove the production and mock configs still
resolve their factories and payload contracts. They should also cover registration failure propagation where the
registry supports a meaningful failure case. Existing build, schema, race, integration and OpenSpec gates remain
applicable. Full browser repetition is selected by the actual change's risk; retained qualification is identified as
baseline evidence, never represented as a run against this branch.

## Future runtime and pack obligations

These are design commitments for subsequent changes. The foundation adds no work-item store, artifact editor,
approval protocol, status endpoint or control-command implementation.

| Contract | Required observable behavior | Qualification boundary |
|---|---|---|
| Work identity | A work item survives separate agent runs, attempts, retries and resumptions; each attempt remains distinguishable. | Reuse admitted identity/lifecycle contracts; an agent run alone is not proof of durable work identity. |
| Artifact revisions | A spec or other artifact has an authoritative location and immutable revision reference; edits validate and expose conflicts. | Repository artifacts and GitHub retain authority. No second mutable copy silently becomes the source of truth. |
| Human decisions | A decision names the artifact revision or execution it approves and the consequence of acting. Later edits expose invalidated approval/evidence. | Define stale-decision rejection and re-review policy before enabling mutations; a visual approval badge is insufficient. |
| Evidence | Results identify their producer, source, revision/attempt and recoverable evidence. Deterministic outcomes are stamped by their executing harness. | Distinguish observations, inferences and recommendations; respect access boundaries and the current evidence-body limit #261. |
| Operator commands | Commands have identities and distinguish accepted, applied and rejected outcomes; pending or unknown completion remains visible. | Retry and external-effect semantics must map to implemented SemEngine contracts, without a competing effect journal. |
| Status and changes | A returning operator can recover current state, relevant changes, outstanding decisions and expected next action, then inspect the underlying evidence. | Use authoritative projections with revisions/readiness; do not infer completion from event delivery or silence. |

The acceptance story is to return to an interrupted development task, understand its state, inspect and revise its
spec, and direct the next step without rebuilding context from chat. Detailed UI, API and data shapes remain open.
Complete inspectability does not require flooding the default view with every event. Checkpoints should ask for
judgment when uncertainty or consequence requires it, not merely increase the number of approvals.

The future development pack preserves issue → OpenSpec → implementation → verification → PR, harness-owned outcomes
and independent clean-room verification. It uses the same agent runtime as other packs. Existing parked packs are
donor material, not the activation path. HumanLayer and Pi are comparative references, not compatibility goals;
there is no parallel freeform coding workflow in this decision.

## Migration constraints and next boundary

Extraction proceeds by demonstrated package dependencies, carrying existing behavior, tests and source provenance.
SemEngine adoption, agent-runtime extraction and development-pack activation are independently qualified changes.
Each running binary uses one substrate. The eventual cutover must settle retained state, in-flight work, rollback and
recovery explicitly; fresh-state qualification does not establish retained-state migration.

[Issue #284](https://github.com/C360Studio/semteams/issues/284) owns design qualification of the architect's candidate
next slice: the agent contract family, root `agentic`, `vocabulary/agentic` and `internal/looptoken`. The existing
[framework footprint](../../../../docs/migrations/semstreams-8b99efe/framework-footprint.md) and narrow source inspection
motivate this candidate; they do not prove an import-closed extraction or authorize copying it. Before implementation,
verify production/test closure and all concrete-type consumers, then design an atomic transfer of payload authority.
Upstream processors cannot consume independently defined lookalike Go payloads. Engine-owned message, projection,
vocabulary and generic types remain dependencies rather than being copied with the family.

This foundation does not authorize bulk copying `agentic-*`, a new generic workflow engine, new durable
buckets/streams, or speculative interfaces. SemSource dogfooding, Program Pulse implementation and paid-model
qualification also remain outside scope. Follow-up issues own the extraction order and any unresolved holds.

## Risks and review

- Aggregate registrar removal can omit an inactive but advertised factory or payload. Preserve the complete frozen
  set, compare registry metadata, and check generated interfaces.
- Ownership documentation can be mistaken for shipped extraction. Label target contracts explicitly and retain the
  frozen dependency and current product limits in entry-point guidance.
- Missing engine APIs can tempt a competing implementation. Keep unresolved extension and durability mapping visible
  as the next design boundary; do not fill them with local generic machinery.
- SemDev absorption can accidentally weaken its guarantees. Treat its workflow, harness evidence, isolation and
  independent verification as required transfer contracts, separately qualified before activation.

Architecture review precedes catalog implementation; independent Go review follows local qualification. Any discovered
public UI contract change requires frontend review before proceeding. Review findings and command evidence belong in
the change's review/evidence records; future sequencing belongs in GitHub issues.
