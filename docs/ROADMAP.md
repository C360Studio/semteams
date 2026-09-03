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
- **Autoresearch pack** — metric optimization with empirical keep/revert in an
  attested per-tenant devcontainer. It remains a useful framework proof, but
  it is not the initial program-manager story.
- **Governed repository workflow** — issues, OpenSpec changes where behavior
  changes, draft PR claims, explicit closure authority, and a readable
  Repository CI baseline.
- **Substrate-plus-overlays runtime** — one SemStreams-backed substrate with
  bounded category packs rather than a bespoke runtime per job.

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
material: files stay in-repo, nothing is wired. SemDev owns the issue-to-PR
implementation journey. SemTeams should integrate through GitHub issues and
pull requests rather than restore a competing maker workflow.

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
  evidence through supported service interfaces.
- Add recommendation classes such as SOP-version drift (a project pinned
  behind the current org SOP release with an applicable change; documentation
  drift is one instance of it), dependency risk, and missing delivery evidence.
- Restore artifact evidence fetch and context handoff through the contract
  owned by [#261](https://github.com/C360Studio/semteams/issues/261).

### 3. Planning and governed action

- Add project planning and design-review journeys with program context.
- Propose issue-shaped work before requesting authority to file it.
- Add human-approved GitHub issue creation for work that SemDev can consume.
- Add independent pull-request review and operator recommendations without
  duplicating SemDev's maker-side quality gates.

### 4. Standing program management

- Give configured projects durable project-manager context and cadence.
- Have the program manager coordinate project-manager reports, dependencies,
  and escalations rather than reprocess every repository directly.
- Monitor the outcomes of approved actions.
- Add operator-selected notification and cadence policies.
- Retain project-local lessons and push eligible ones to SemMem over
  SemStreams federation, as any SemStreams-based instance can; SemTeams is a
  lesson source as well as an observer of the SOP process. Institutional
  knowledge flows back to products only through SOP releases.
- Add generic MCP consumption when the governed client primitive exists in
  SemStreams; keep SemSource-specific integration product-local. MCP is
  runtime retrieval of active practices, never the trigger; SOP releases are.

## Release alignment

SemTeams intends to match SemStreams' pace and reach product v1 alongside
SemStreams v1. During beta, SemTeams should normally stay no more than one
released SemStreams beta behind. Each bump remains a first-class compatibility
change, especially across storage or lifecycle boundaries.

## Framework boundary

The semstreams framework owns components, graph engine, rule engine, NATS stream
wiring, gateways, and reusable agentic primitives. SemTeams should stay a thin
product shell: UI, product configs, personas, rules, product tools, journeys,
and docs that prove how to assemble those primitives into governed agentic
workflows.

When a new feature looks framework-shaped, first check whether it belongs
upstream. Product-local additions should stay narrow, documented, and tied to a
SemTeams program-management journey.
