# SemTeams Roadmap

SemTeams is moving from a configurable agent-team demonstration toward an
always-on program manager for a configurable portfolio. The canonical product
direction and MVP boundary live in
[SemTeams Program Manager](product/program-manager.md).

This page describes the delivery sequence, not an exhaustive issue tracker.
GitHub issues own work sequencing, and milestones own release membership. The
current runnable truth remains the shipped configs, journeys, and
[demo evidence](demo-mvp-claims.md).

## Foundation available today

- **Coordinator front door** — conversational intake and explicit routing to
  live capabilities.
- **Research pack** — planning, parallel evidence gathering, synthesis,
  review, artifact emission, and coordinator wake-up.
- **Autoresearch pack (retained framework demonstration)** — metric optimization
  with empirical keep/revert in an attested per-tenant devcontainer. It remains
  wired on disk with contract/integration coverage and opt-in browser journeys
  as a framework demonstration, but it is not part of the Program Pulse MVP
  claim or the required `demo-mvp` / `test:e2e` browser gates.
- **Governed repository workflow** — issues, OpenSpec changes where behavior
  changes, draft PR claims, explicit closure authority, and a readable
  Repository CI baseline.
- **Frozen runtime baseline** — one SemStreams-backed substrate and agent runtime
  with bounded category packs. [ADR-061](adr/061-agent-runtime-ownership-and-observable-work.md)
  assigns future agent-runtime ownership to SemTeams and the shared substrate
  to SemEngine; neither transfer is shipped by this baseline.

## Parked beta.160 regression (ADR-059)

Artifact-card content and artifact-context handoff lost their evidence-body
source when the UI moved to the beta.160 GraphQL trajectory surface. The
pre-cutover OpenSpec change is retained in Git history, not the active queue.
Issue [#261](https://github.com/C360Studio/semteams/issues/261) owns an
authorized `StorageReference` evidence-fetch contract and the freshly
reconciled OpenSpec change required for resumption.

## Parked development packs (ADR-058)

The spec-authoring and software-implementation packs (create-change,
proof-readiness, dev-from-task, dev-via-test) are parked in place as donor
material: files stay in-repo, nothing is wired. SemDev currently owns the
issue-to-PR implementation journey. [ADR-061](adr/061-agent-runtime-ownership-and-observable-work.md)
and [ADR-063](adr/063-development-pack-absorption-boundary.md) assign its
separately qualified future absorption to a SemTeams development pack, with
harness-owned outcomes and independent verification preserved.

## Delivery sequence

### 1. Program pulse MVP

- Load an operator-authored program → project → repository mapping.
- Observe configured GitHub repositories through a deterministic,
  product-owned read adapter.
- Produce scheduled and on-demand, evidence-backed program reports.
- Classify completed, in-progress, waiting, and at-risk work.
- Recommend which projects need attention and support project drill-down.
- Preserve human-requested research as an available supporting capability.

This stage is read-only. It does not file issues or review pull requests.

### 2. Project intelligence

- Use research when repository facts do not fully explain a program finding.
- Enrich project analysis with SemSource code, documentation, and change
  evidence through supported service interfaces when indexing is needed.
  SemSource is recommended for that purpose and is not required for the first
  forge-observation journey.
- Add recommendation classes such as SOP-version drift (a project pinned
  behind the current org SOP release with an applicable change; documentation
  drift is one instance of it), dependency risk, and missing delivery evidence.
- Restore artifact evidence fetch and context handoff through the contract
  owned by [#261](https://github.com/C360Studio/semteams/issues/261).

### 3. Planning and governed action

- Add project planning and design-review journeys with program context.
- Propose issue-shaped work before requesting authority to file it.
- Add human-approved GitHub issue creation for the separately qualified
  development workflow to consume.
- Add independent pull-request review and operator recommendations while
  preserving the development pack's maker-side quality gates.

### 4. Standing program management

- Give configured projects durable project-manager context and cadence.
- Have the program manager coordinate project-manager reports, dependencies,
  and escalations rather than reprocess every repository directly.
- Use source-specific observations and category rule packs to start bounded
  work. [ADR-064](adr/064-rule-pack-coordination-and-source-observation.md)
  defines outbound GitHub polling as the first candidate for deployments with
  no public inbound route, with optional webhook notifications and later
  non-forge adapters. The first proof is one changed issue in one configured
  repository; inter-manager execution remains separately qualified.
- Monitor the outcomes of approved actions.
- Add operator-selected notification and cadence policies.
- Retain project-local lessons and push eligible ones to SemMem over
  SemStreams federation, as any SemStreams-based instance can; SemTeams is a
  lesson source as well as an observer of the SOP process. Institutional
  knowledge flows back to products only through SOP releases.
- Add generic MCP consumption when a governed client primitive is qualified;
  keep SemSource-specific integration product-local. MCP is
  runtime retrieval of active practices, never the trigger; SOP releases are.

## Release alignment

The current compatibility baseline is frozen SemStreams
`8b99efe9c66a4faa4fa509f9f62cc6bad8392128`. Agent-runtime extraction,
SemEngine adoption, and development-pack activation are independently
qualified changes. Dependency dates or a rolling-beta target do not override
consumer-contract, recovery, state, and product qualification gates
([ADR-059](adr/059-semstreams-beta160-graph-foundation-adoption.md),
[frozen migration evidence](migrations/semstreams-8b99efe/README.md),
[ADR-061](adr/061-agent-runtime-ownership-and-observable-work.md)).

## Framework boundary

The current binary composes frozen SemStreams components. The target boundary
in [ADR-061](adr/061-agent-runtime-ownership-and-observable-work.md) assigns
graph, transport, mutation, rule evaluation, lifecycle/projection, and generic
durability to SemEngine. SemTeams owns the agent loop, dispatch, model/tool
orchestration, context, agent-specific approvals, trajectories and rule
extensions, alongside its packs and product surface. [ADR-062](adr/062-agent-contract-extraction-boundary.md)
governs bounded extraction without copying shared contracts or policy.

New components, tools, payloads, rule actions, KV buckets and streams need a
framework-alignment review: assign generic substrate mechanics to SemEngine,
agent-specific machinery to the SemTeams runtime, and domain policy to a pack.
No pack gets a second journal or workflow engine.
