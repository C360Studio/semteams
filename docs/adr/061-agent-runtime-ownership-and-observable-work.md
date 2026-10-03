# ADR-061: Agent-runtime ownership and observable work

**Status:** Accepted direction (2026-10-03), recorded from the owner's approval in
[issue #282](https://github.com/C360Studio/semteams/issues/282). This is a target ownership and product contract;
agent-runtime extraction, SemEngine adoption, development-pack activation and new UI behavior are not implemented
by this decision. The foundation change preserves the frozen SemStreams runtime.

**Supersedes in part:** [ADR-029](029-product-shell-wiring.md)'s upstream-only agentic implementation premise;
[ADR-058](058-beta159-realignment-and-demo-lane-focus.md)'s future dev-pack restoration approach in §Consequences;
and [ADR-060](060-program-manager-direction-and-research-design.md)'s statements that SemTeams never authors PRs
and SemDev must remain a separate product boundary. The development path is now separately qualified SemDev
absorption, with old packs retained as donor material. The historical reasons remain valid. ADR-029's explicit composition discipline, ADR-058's parked-pack fence, and ADR-060's program-manager goal,
read-only pulse MVP, research design, authority ladder and governed external-service boundaries remain in force.

## Context

SemTeams currently composes agentic and graph components from SemStreams frozen at
`8b99efe9c66a4faa4fa509f9f62cc6bad8392128`. Its
[qualified migration](../migrations/semstreams-8b99efe/README.md) establishes that baseline; it does not establish
SemEngine compatibility, retained-state migration or new product capabilities.

SemEngine extracts the shared substrate and explicitly leaves current agent features to its consumers. Migrating
SemTeams therefore includes ownership of the agent runtime itself. Keeping SemDev as another independent runtime
owner would duplicate the same agent machinery and make development behavior harder to integrate and observe.
SemDev brings more than prompts: its issue-to-PR flow depends on harness-owned outcomes, controlled execution and
independent verification. Those guarantees must travel with the workflow.

The product purpose remains making program and project work understandable and manageable for a human operator.
Visibility alone is insufficient: an event firehose or an endless series of approvals can increase exhaustion.
The operator needs a reliable account of what changed, what is established or uncertain, what needs judgment, and
what an action will do, with underlying evidence available for inspection.

## Decision

### 1. Separate substrate, agent runtime, and product packs

| Layer | Ownership |
|---|---|
| SemEngine | Graph, transport, mutation, rule evaluation, lifecycle/projection and generic durable-execution mechanics |
| SemTeams agent runtime | Agent loop, dispatch, model/tool orchestration, context/memory assembly, agent-specific approvals/governance, trajectories, and agent rule-action extensions |
| SemTeams packs and product | Domain workflows, personas, tools, evidence policy, portfolio/program semantics and the Svelte operator experience |

Application wiring remains small and explicit while the agent runtime becomes a deliberately bounded SemTeams
responsibility. A pack adds domain behavior to the shared runtime; it does not bring a second loop or workflow
engine. Shared persona guidance remains domain-neutral. Owning the runtime is not permission to reimplement the
substrate, add speculative primitives, or copy every dependency into SemTeams.

The [SemEngine contract](https://github.com/C360Studio/semengine/blob/2ec3bcf08bc1d2794d6cb1081ebd1982e08cf2c5/docs/contract.md) describes intended
first-release behavior, not a shipped SemTeams dependency. Lifecycle phases and revision-guarded transitions are
distinct from generic attempt ownership, effect settlement, retries and replay. The latter are planned in
[SemEngine #24](https://github.com/C360Studio/semengine/issues/24#issuecomment-5937829621), using lifecycle/projection as their foundation.
We must map agent recovery to those implemented contracts as they arrive. Until then, preserve frozen agent recovery
behavior and name its limits. Neither persisted graph facts nor accepted messages prove an outside effect occurred
once. Do not build a competing generic execution journal or effect ledger to bridge the wait.

Agent-specific rule actions, including `publish_agent`, move with the agent runtime. The engine's public extension
registration API remains an integration question; this decision does not invent package paths or signatures for it.
Generic rule evaluation and lifecycle mechanics remain engine responsibilities.

### 2. Absorb SemDev as a separately qualified development pack

The development journey remains **issue → OpenSpec → implementation → verification → PR**. Investigation and
iteration take place within that process. There is no parallel freeform coding path in this decision.

The transfer preserves SemDev's [constitution](https://github.com/C360Studio/semdev/blob/main/docs/constitution.md):
harness-owned outcome facts, declared fact writers, isolated clean-room verification before a terminal deliverable,
reviewable evidence and domain policy. Moving personas and rules alone is not sufficient qualification. Development
execution and sandbox tools remain pack responsibilities unless demonstrated shared use justifies runtime ownership.

The absorbed pack uses the same agent runtime as research and other packs. Its activation and authority are separate
from the substrate migration and from the read-only program-pulse MVP. Existing `create-change`, `proof-readiness`,
`dev-from-task` and `dev-via-test` packs remain parked donor material. This ADR does not revive them, alter the live
coordinator taxonomy, or bypass their predicate and graph-mutation migration requirements.

### 3. Make work inspectable and controls understandable

Future runtime and pack contracts must let a person return to interrupted work without reconstructing a chat log:

- **Work and attempts:** stable work identity spans retries and resumptions while each execution attempt remains
  separately identifiable.
- **Artifacts:** an artifact names its authoritative location and revision. Edits validate against that source and
  expose concurrent changes; a displayed copy does not silently become another source of truth.
- **Decisions:** approval or rejection identifies the revision or execution reviewed, the authority exercised and
  the consequence. Changed requirements expose which approvals or evidence need reconsideration.
- **Evidence:** facts retain producer, source, attempt/revision and accessible supporting material. Harness results
  are distinct from model claims; observations, inferences and recommendations remain distinguishable.
- **Controls:** a command has an identity and separately visible accepted, applied or rejected outcomes. Pending or
  unknown outcomes stay explicit. Delivery or acknowledgment is not completion.
- **Current state and changes:** an authoritative, readiness-aware snapshot connects progress, relevant changes,
  required decisions and the expected next action, with drill-down to the underlying evidence.

GitHub and repository artifacts remain authoritative for repository work. Future spec CRUD must edit the real
OpenSpec artifact, run its validation and identify the revision used by subsequent work. Approval invalidation and
external-effect semantics must be designed before those controls are enabled. Detailed screens, schemas and APIs are
unsettled; these commitments do not claim that an artifact editor or new control API exists today.

The representative acceptance story is: return to an interrupted issue-to-PR task, understand its state, inspect and
revise the spec, and give the next instruction without rebuilding context from the conversation. Default views
summarize the useful state; complete evidence remains inspectable. Human checkpoints follow uncertainty and
consequence rather than maximizing approval count.

HumanLayer and Pi are references to evaluate against this product purpose, not compatibility goals. SemTeams does
not adopt a new product identity or pursue feature parity with either.

### 4. Move behavior in independently qualified slices

Preparation begins on the frozen source by making the complete component/payload catalog explicit. Preserve
advertised registration and generated interfaces; registration availability is separate from live activation.
Subsequent extraction carries existing behavior, tests, source provenance and licensing with the smallest package
group its real dependencies permit. Introduce interfaces only for demonstrated boundaries.

Agent-runtime extraction, development-pack activation and SemEngine cutover each require their own qualification.
A running binary has one substrate. Before cutover, settle persisted-state compatibility, in-flight executions,
recovery and rollback explicitly. Fresh isolated-state evidence is not retained-state migration proof. Do not hide
SemEngine gaps by maintaining two substrates or two workflow engines inside one binary.

## Consequences

- Current processors remain in frozen SemStreams until reviewed extraction changes move them. Research and
  autoresearch remain the only live product-facing packs; Program Pulse and the development pack remain targets.
- Framework-alignment review now asks both whether a primitive belongs in SemEngine and whether agent-specific
  behavior belongs in the SemTeams runtime or a pack. It must still reject duplicate generic mechanics.
- Runtime ownership is substantial engineering work; it does not justify an unbounded migration branch. Preserve
  qualification and independent review at each behavior or dependency boundary.
- Existing evidence-body rendering and artifact-handoff limits (#261), continuation/recovery limits and explicit
  browser skips remain limits until separately repaired and qualified.
- The program manager retains observation-first authority. Absorbing the maker workflow does not grant every pack
  PR-authoring authority or bring issue creation, implementation or merging into the read-only pulse MVP.
- Issues and milestones own sequencing and holds. This ADR records the boundary and rationale, not a migration
  checklist or a second status system.

## Alternatives considered

- **Wait for SemEngine before owning the runtime boundary.** Defers the largest consumer migration uncertainty;
  explicit registration and ownership decisions can be qualified against the frozen source now.
- **Maintain separate agent runtimes in SemTeams and SemDev.** Duplicates integration/recovery work and fragments the
  operator experience. A development pack preserves its opinions within one runtime.
- **Build a backend-neutral loop and generic durability layer now.** Risks rewriting proven behavior against imagined
  engine APIs and duplicating SemEngine #24. Preserve behavior first and adapt to implemented contracts.
- **Combine extraction, UI redesign and dev activation into one cutover.** Makes regressions difficult to attribute
  and rollback difficult to bound. Qualify each separately.
- **Adopt another agent product's interaction model wholesale.** Popularity does not prove lower supervision effort
  for this product. Evaluate individual ideas against the observable-work acceptance story.

## Related

- [Product direction](../product/program-manager.md)
- [Foundation PR #283](https://github.com/C360Studio/semteams/pull/283)
- [Frozen contract inventory](../migrations/semstreams-8b99efe/contract-inventory.md) and
  [framework footprint](../migrations/semstreams-8b99efe/framework-footprint.md)
- [ADR-042](042-coordinator-instantiated-flows-via-templates.md): category packs and domain-neutral shared personas
- [ADR-043](043-devcontainer-as-sandbox-spec.md): sandbox attestation
- [ADR-060](060-program-manager-direction-and-research-design.md): preserved program-manager and research decisions
