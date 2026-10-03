# SemTeams Project Context

## Purpose

SemTeams is an always-on program manager for a configurable portfolio. Its first target MVP is a read-only,
evidence-backed Program Pulse; current research and autoresearch packs demonstrate the agent foundation.

Today SemTeams composes a frozen SemStreams runtime through its product shell, Svelte UI, category rule packs,
persona corpus, product tools and journeys. ADR-061 assigns the target agent runtime to SemTeams and the shared
substrate and generic durability to SemEngine, with SemDev absorbed as a separately qualified development pack.
Human participation should become less exhausting through observable work, inspectable evidence and explicit
controls. This direction does not claim that extraction, pack activation or the future UI is implemented.

## Product Boundary

- **SemEngine target:** graph, transport, mutation, rule evaluation, lifecycle/projection and generic durable execution.
  Attempt/effect/replay mechanics are planned in SemEngine #24, not a shipped guarantee of this dependency.
- **SemTeams agent-runtime target:** loop, dispatch, model/tool orchestration, context/memory assembly, agent-specific
  approvals/governance, trajectories and agent rule extensions. Extract in reviewed slices with behavior, tests and
  source provenance preserved; public engine extension APIs remain unsettled.
- **SemTeams product and packs:** `cmd/semteams/` wiring, bootstrap/category rules, personas, domain tools, Svelte UI,
  program semantics and journeys. Development preserves issue → OpenSpec → implementation → verification → PR,
  harness-owned outcomes and clean-room verification on the shared runtime.
- All processor implementations currently remain in frozen SemStreams. `internal/runtimecatalog` makes registration
  explicit without narrowing the frozen surface or activating packs. Reuse existing primitives. New tools, action
  shapes, payloads, buckets or streams require the framework-alignment review in `CLAUDE.md` and an ADR or upstream
  contract discussion. Do not create a competing generic durability journal or workflow engine.
- Cross-repository contracts are shared boundaries. Record the durable reason in an ADR and the current behavioral
  mechanics in a living spec; do not silently fork an upstream contract in the product shell.

## Current Product State

- The Go module is pinned to SemStreams `v1.0.0-beta.162.0.20260930150212-8b99efe9c66a` at SHA
  `8b99efe9c66a4faa4fa509f9f62cc6bad8392128`. The qualified migration from beta.160 landed in PR #281.
  Fresh isolated NATS 2.14.4 and graph state are required; retained-state conversion is not implemented.
  No production storage wipe is authorized. See `docs/migrations/semstreams-8b99efe/README.md` for retained evidence
  and limitations: 24 active mock browser passes, five explicit skips, and the qualified autoresearch delivery repair.
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
  not shipped behavior. The absorbed development pack and new artifact/control UI are also future behavior.
- SemSource-backed dogfooding waits for SemSource readiness. A later SemEngine switch needs its own approved consumer
  contract; symbol and closure measurements are evidence, not permission to expand its first release.
- ADR-061's future observable-work contracts cover work identity across attempts, artifact revisions, revision-bound
  decisions and invalidation, evidence ownership/provenance, command acceptance versus outcome, and authoritative
  current-state/change views. Implement and qualify their contracts before describing them as live behavior.

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

- `architect` designs API, graph, data, and integration contracts and reviews substrate/runtime/pack boundaries.
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
