# SemTeams Program Manager

**Status:** Owner-approved product direction, 2026-08-26; amended 2026-09-03 per the owner ruling on
[#264](https://github.com/C360Studio/semteams/issues/264). This document describes the target product and MVP. For the
behavior available today, see the [README](../../README.md) and [demo claim boundary](../demo-mvp-claims.md).

## Product promise

SemTeams is an always-on program manager for a configurable portfolio of programs and projects. It observes work,
explains what changed, identifies where attention is needed, and helps the operator carry approved actions through.

Research, planning, and design are not separate product identities. They are capabilities the program manager applies
in support of a program or project.

The initial customer is an operator responsible for several related projects who needs a trustworthy view across them
without manually reconstructing status from repositories, issues, pull requests, releases, and documentation.

## Operating model

SemTeams uses four distinct levels:

| Level | Meaning |
|---|---|
| Program | A portfolio with shared outcomes, dependencies, and an operator-defined boundary. |
| Project | A bounded effort or product within a program. A project can span multiple repositories. |
| Repository | An evidence and delivery surface associated with one or more projects. It is not the project itself. |
| Work item | An issue, pull request, release, decision, risk, or other unit of observable work. |

The operator owns this mapping. SemTeams may recommend changes to it later, but it must not silently infer program or
project membership from repository activity.

## Target team topology

The long-term SemTeams organization is a governed team of managers: one program manager coordinates a project manager
for each configured project.

- The **program manager** owns portfolio outcomes, cross-project dependencies, priorities, risks, and the operator's
  program-level report.
- Each **project manager** owns project context, status, investigation, plans, recommendations, and the preparation of
  project actions for approval.
- **Research, planning, design, and review specialists** are shared capabilities invoked by the program or project
  managers when the work requires them.
- The **human operator** retains authority over consequential actions and can drill from the program view into any
  project manager's evidence and recommendations.

These are logical responsibilities, not a requirement that every manager immediately be a separate long-lived process.
The MVP uses one program-manager journey to construct project views from the configured portfolio. Later, project
managers may gain durable context, cadence, and delegated work; the program manager then coordinates their reports
instead of reprocessing every repository directly. A project manager is assigned to a project, not automatically to
each repository.

## The program-management loop

SemTeams grows through one coherent loop:

1. **Observe** activity across the configured portfolio.
2. **Explain** relevant changes, dependencies, risks, and unanswered questions.
3. **Recommend** where human attention or project action is warranted.
4. **Plan** sequencing, milestones, issues, and coordination steps.
5. **Design** product or technical proposals when a recommendation needs deeper treatment.
6. **Act** under explicit authority by filing issues, reviewing pull requests, and monitoring outcomes.
7. **Learn** from outcomes locally and push eligible lessons to SemMem over SemStreams federation, as every
   SemStreams-based instance can. SemTeams is a lesson source as well as an observer of the SOP process.

Every report separates observed facts from recommendations. A factual claim carries recoverable evidence: repository,
object identity, state, timestamp, link, and relevant issue/PR/release relationships. Recommendations state the
reasoning that connects the evidence to the proposed attention or action.

## Capability packs

The user experiences one program manager; the runtime stays understandable as bounded category packs on the shared
SemStreams substrate.

| Capability | Purpose | Product posture |
|---|---|---|
| `program-report` | Evidence-backed program pulse and project drill-down. | First MVP pack; not yet shipped. |
| `research` | Configurable-depth evidence gathering, synthesis, and review. | Shipped foundation; retained. |
| `project-plan` | Propose sequencing, milestones, risks, and issue-shaped work. | Valid post-MVP capability. |
| `design-review` | Develop or assess product and technical designs in project context. | Valid post-MVP capability. |
| `project-action` | Draft or file approved project work and monitor disposition. | Later authority stage. |
| `pr-review` | Independent review using issue, evidence, and program impact. | Later authority stage. |

General and deep research should be depth or budget profiles of the research capability unless experience proves they
have meaningfully different terminal contracts. Packs may reuse tools, evidence contracts, and persona fragments. The
first MVP must not depend on opaque pack-to-pack composition: each user-visible journey has an explicit entry point,
terminal result, and authority boundary.

Evidence-verified research is the program manager's core competence, and `program-report` is built as the first depth
profile of the `research` capability, not beside it: the planner decomposes the operator's portfolio, gatherers read
GitHub through the product-owned adapter, synthesis emits a pulse artifact whose every factual claim carries an evidence
record, and review is a mechanical derivation check (every cited object appears in a tool result in the run's own
trajectory) plus plan conformance. The evidence record is channel-typed so that `pr-review`, docs-drift findings, and
open-web research later share the same reviewer and the same check. GitHub is the first domain because it is the only
candidate domain with ground truth; the open web and SemSource are later channels of the same engine.
That reduced review is the verifiable-domain shortcut: the GitHub API is ground truth. Research over channels without
ground truth adds the falsification phase and origin-counted redundant width recorded in
[ADR-060](../adr/060-program-manager-direction-and-research-design.md) decision 6.

`autoresearch` remains a useful shipped framework demonstration. It is not part of the initial program-manager MVP
story unless a concrete program-management journey requires metric optimization.

## MVP boundary

The MVP proves the program-manager view before introducing durable delegated project managers or autonomous project
action.

It must:

- load an operator-authored program, project, and repository configuration;
- observe activity from configured GitHub repositories through a deterministic product-owned adapter;
- maintain cursors and stable identities so repeated runs do not duplicate old activity;
- produce scheduled and on-demand program pulses;
- classify work as completed, in progress, waiting, or at risk;
- recommend projects that warrant attention and explain why;
- drill from a program finding into the supporting project and work-item evidence;
- keep facts, inferences, and recommendations visibly distinct; and
- preserve the existing human-requested research journey.

The first implementation slice is the smallest useful version of that promise:

> Produce an evidence-backed daily program pulse across configured projects and repositories.

Deeper targeted research over open-web or SemSource channels may enrich a pulse when the observed evidence is
insufficient; it is not a prerequisite for the first end-to-end proof. What is a prerequisite is the shared evidence
contract and the containment budget: the first proof must not ship a fetch-and-summarize pipeline with its own evidence
shape or an unbounded loop.

The MVP does not:

- create or modify issues;
- approve, merge, or implement pull requests;
- replace GitHub as the work system of record;
- treat docs drift as the primary product;
- make architecture or product decisions without the operator; or
- curate cross-product knowledge inside SemTeams.

Docs drift is one future project-insight finding. Its general form is SOP-version drift: a project whose manifest pins
an org SOP bundle version behind the current SOP release, where at least one change since the pin applies to that
project. The trigger is a versioned SOP release, never an individual lesson, and the recommendation is one adoption
issue per project per version. Trending-topic reports are a standing research use case. Neither needs its own product
identity.

## Authority ladder

Authority expands only after the preceding stage is trustworthy:

1. Observe the program.
2. Recommend where attention is needed.
3. Explain the affected project and evidence.
4. Propose an action.
5. Perform a human-approved action.
6. Monitor the action's outcome.
7. Retain project-local lessons and export eligible lessons to SemMem.

The MVP stops at stage 3. Issue filing and pull-request review arrive through explicit, auditable approval policy; they
are not implied by always-on operation.

## Sem ecosystem boundaries

- **SemStreams** owns the reusable agentic, graph, rule, governance, memory, and integration primitives.
- **SemTeams** owns portfolio configuration, program observation, synthesis, recommendations, and operator-facing
  program-management journeys.
- **SemDev** owns the maker-side issue-to-reviewed-PR workflow and clean-room verification.
- **SemSource** supplies code, documentation, and change evidence through its supported service interfaces.
- **SemMem** ingests lessons pushed from any SemStreams-based instance, curates them into ecosystem practices, and owns
  the org SOP repository's content policy. It files SOP items as structured issues; it does not author PRs. Practices
  reach consumers as versioned SOP releases, which are the trigger, and over MCP for runtime retrieval, which is never
  the trigger.
- **GitHub** remains the human-visible work bus and authority for repository work state.

SemTeams can later file an issue for SemDev to consume and independently review the resulting pull request for the
operator. That review complements SemDev's internal quality gates; it does not duplicate them or require a private
SemTeams-to-SemDev API. The same rule holds across the ecosystem: managers and curators file issues, and SemDev
instances are the only PR authors, including on the SOP repository. Every edge between products is either a visible
artifact on GitHub or a governed SemStreams contract that carries provenance and is idempotent; never a shared graph or
an ad-hoc private API. Lessons flow in by push; institutional knowledge flows back out only as SOP releases.

## Release posture

SemTeams intends to match SemStreams' pace and reach its product v1 alongside SemStreams v1. During the beta period,
SemTeams should normally remain no more than one released SemStreams beta behind, while treating every dependency bump
as a verified compatibility change rather than a calendar-only upgrade.

GitHub issues own sequencing and release membership. This document defines the destination and boundaries; it is not a
second backlog.
