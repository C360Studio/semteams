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

- The Go module is pinned to SemStreams `v1.0.0-beta.161`. Crossing either the beta.159-to-beta.160 or
  beta.160-to-beta.161 boundary requires fresh NATS storage; there is no compatibility or in-place data migration.
- The live product-facing categories are `research` and `autoresearch`. `coordinator`, `agent-run`, and `ops` are
  support packs in the bootstrap.
- `create-change`, `proof-readiness`, `dev-from-task`, and `dev-via-test` remain on disk but are unwired under ADR-058.
  They must be re-authored for the canonical predicate and beta.160 graph-mutation contracts before re-wiring. Their
  journeys and relevant tests remain parked and are not live demo evidence.
- The coordinator's live taxonomy is `research | autoresearch | respond_direct | ask_user`; parked-team requests receive
  an honest direct response.
- `Repository CI` runs Go, UI, and Governance/OpenSpec jobs for every pull request to `main`; the jobs feed one stable
  `CI Status Check` aggregate. Required mock E2E and a main-branch ruleset remain future work.

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
