# SemTeams Project Context

## Purpose

SemTeams is a configurable multi-agent product harness built on the SemStreams framework. It owns the product shell,
Svelte UI, category rule packs, persona corpus, product-level tool composition, journeys, and documentation that turn
SemStreams' agentic and graph primitives into reviewable team workflows.

The product demonstrates auditable agent coordination rather than one-shot prompting: the coordinator classifies work,
category packs drive bounded roles and gates, artifacts carry evidence between teams, and operator-visible state is
recorded through the shared graph substrate.

## Product Boundary

- **SemStreams owns framework primitives and contracts:** processors, graph ingestion/query/mutation, NATS clients and
  storage patterns, rule execution, agentic loop/dispatch/model/tools/governance components, payload and vocabulary
  registries, lifecycle, health, and metrics.
- **SemTeams owns product composition and semantics:** `cmd/semteams/` wiring, bootstrap configs, product category
  rules, personas, user-facing workflows, product-only tool executors, Svelte surfaces, and product journeys.
- SemTeams has no custom Go processors. Reuse an upstream primitive when one exists. A new product-shell tool, rule
  action shape, payload, KV bucket, or long-lived stream requires the framework-alignment review in `CLAUDE.md` and an
  ADR addendum or upstream issue recording the result.
- Cross-repository contracts are shared boundaries. Record the durable reason in an ADR and the current behavioral
  mechanics in a living spec; do not silently fork an upstream contract in the product shell.

## Current Product State

- The Go module is pinned to SemStreams `v1.0.0-beta.162.0.20260930150212-8b99efe9c66a` at SHA
  `8b99efe9c66a4faa4fa509f9f62cc6bad8392128`. The migration from beta.160 is under qualification in PR #281.
  Fresh isolated NATS 2.14.4 and graph state are required; retained-state conversion is not implemented.
  No production storage wipe is authorized. See `docs/migrations/semstreams-8b99efe/README.md` for blockers.
- The live product-facing categories are `research` and `autoresearch`. `coordinator`, `agent-run`, and `ops` are
  support packs in the bootstrap.
- `create-change`, `proof-readiness`, `dev-from-task`, and `dev-via-test` remain on disk but are unwired under ADR-058.
  They must be re-authored for the canonical predicate and current graph-mutation contracts before re-wiring. Their
  journeys and relevant tests remain parked and are not live demo evidence.
- The coordinator's live taxonomy is `research | autoresearch | respond_direct | ask_user`; parked-team requests receive
  an honest direct response.
- `Repository CI` runs Go, UI, and Governance/OpenSpec jobs for every pull request to `main`; the jobs feed one stable
  `CI Status Check` aggregate. Required mock E2E and a main-branch ruleset remain future work.

- Runtime saved-flow authoring, managers and template seeds are retired. The UI exposes admitted composition and
  graph exploration. The rule manager joins configuration through a registered key family and readiness barrier.
- Evidence-body rendering and artifact handoff remain limited by #261. Program Pulse remains the target product,
  not shipped behavior. Autoresearch final user delivery remains a migration blocker.
- SemSource-backed dogfooding waits for SemSource readiness. A later SemEngine switch needs its own approved consumer
  contract; symbol and closure measurements are evidence, not permission to expand its first release.

## How We Spec

- `openspec/specs/<capability>/spec.md` is living accepted behavior. Seed or amend it only from code and reviewed
  durable requirements; distinguish implemented-but-parked capability from current live routing.
- `openspec/changes/<id>/` is the target-state delta and task truth for one claimed pull request, not a backlog or
  program plan.
- Archive a completed change in the landing pull request's final content commit so the change move and living-spec sync
  are reviewed with the implementation.
- Remove abandoned or parked changes from the active queue without archive or spec promotion. Preserve the resume gate
  in a GitHub issue. Issue #258 owns the `repo-readiness-init` retirement in this baseline; #260 owns any future
  reintroduction and freshly reconciled change.
- `docs/adr/` records durable reasons and cross-repository decisions. GitHub issues own wanted work, decisions,
  blockers, and holds; draft pull requests own claims and stop-points.

## Role Split

- `architect` designs API, graph, data, and integration contracts and reviews product/framework boundaries.
- `go-developer` uses TDD for backend or product-shell implementation; `go-reviewer` owns its quality gate.
- `svelte-developer` uses Svelte 5 and TypeScript for UI implementation; `svelte-reviewer` owns accessibility, UX, and
  frontend quality review.
- Cross-stack changes require both reviewer lanes.
- `technical-writer` owns durable documentation and conservative OpenSpec task truth after implementation evidence.

## Standing Conventions

- Go 1.26.3; Svelte 5, SvelteKit 2, and strict TypeScript; NATS JetStream KV/ObjectStore; Prometheus and `slog`; Task as
  the task runner.
- SemTeams-local persisted predicates follow the canonical three-segment lower-kebab grammar. Parked legacy dialect is
  not precedent.
- Rules trigger and components or tools execute; do not create a second workflow control plane.
- Test behavior and outcomes, use explicit synchronization, and prioritize critical-path and edge-case proof.
- Long-running paid or Playwright operations follow the active-monitoring protocol in `CLAUDE.md`; silence is not proof.
- Documentation uses one H1, consistent heading levels, language-tagged fences, and lines under 120 characters where
  practical.
