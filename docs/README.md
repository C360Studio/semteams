# SemTeams Documentation

SemTeams is an early program-manager product shell on top of the
[semstreams](https://github.com/c360studio/semstreams) framework.
Anything *framework-shaped* (components, graph, rule engine, NATS
streams, payload registry) is documented upstream. Anything
*product-shaped* (UI, configs, personas, rules, product-shell
tools, journeys) is documented here.

## If you are…

- **new to SemTeams (what does it DO?)** →
  [`product/program-manager.md`](product/program-manager.md) — the target product and MVP boundary
- **understanding the runtime that exists today** →
  [`architecture.md`](architecture.md) — the substrate-plus-overlays runtime in plain English
- **new to SemTeams (how do I run it?)** →
  [`getting-started.md`](getting-started.md)
- **extending the product shell** →
  [`product-shell wiring record`](adr/029-product-shell-wiring.md)
  + [`../cmd/semteams/tools/README.md`](../cmd/semteams/tools/README.md)
- **writing a flow / persona / rule** →
  [`../configs/README.md`](../configs/README.md)
- **writing a Playwright journey** →
  [`journeys/README.md`](journeys/README.md)
- **checking the demo claim boundary** →
  [`demo-mvp-claims.md`](demo-mvp-claims.md)
- **proposing or implementing a governed change** →
  [`../openspec/README.md`](../openspec/README.md)
- **looking for framework concepts** → upstream
  [semstreams docs](https://github.com/c360studio/semstreams/tree/main/docs)

## What lives in this folder

### SemTeams-native

These are written for this product and contain decisions and shapes
that don't exist upstream.

| Path | Purpose |
|---|---|
| [`product/program-manager.md`](product/program-manager.md) | Product direction, operating model, capability boundaries, and MVP |
| [`architecture.md`](architecture.md) | What semteams DOES — the substrate-plus-overlays runtime, live category packs, and how a sandbox gets created |
| [`getting-started.md`](getting-started.md) | New-dev quickstart + debugging recipes |
| [`demo-mvp-claims.md`](demo-mvp-claims.md) | Supported demo claims, non-claims, black-box evidence rules, and MAVLink-hard scope |
| [`product/vocabulary-map.md`](product/vocabulary-map.md) | Product vocabulary and UI label guidance |
| [`architecture archive`](adr/) | Maintainer decision records for load-bearing product-shell choices |
| [`proposals/`](proposals/) | Research and design notes that inform decisions but are not themselves product commitments. |
| [`journeys/`](journeys/) | Pointer doc — the journey *specs* are the Playwright tests under `ui/e2e/agentic/` |
| [`ui-integration-notes.md`](ui-integration-notes.md) | UI ↔ backend integration notes (historical reference; predates the 2026-06-03 UI slice). |

### Framework concepts — go upstream

This repo previously carried five upstream-semstreams documentation
directories (`basics/`, `concepts/`, `advanced/`, `operations/`,
`contributing/`) imported verbatim during the initial subtree
fork. They explained the framework, not the product, drifted
independently of upstream, and were removed 2026-06-03 in favour
of a single pointer at the canonical source:

- **<https://github.com/c360studio/semstreams/tree/main/docs>** —
  Graphable interface, knowledge-graph model, NATS-stream wiring,
  payload registry, component reference, agentic primitives.

`docs/ROADMAP.md` was a similar carry-over (the upstream SemStreams
roadmap) and now holds a SemTeams pointer instead.

`../configs/README.md` documents the product flow config and live
category-pack inventory. The legacy upstream-shaped graph-tier
examples still present under `configs/` are called out there as
non-product configs.

## Conventions

- Single `#` H1 per file; no skipping heading levels.
- Lines under 120 characters where practical.
- Fenced code blocks specify a language.
- Comments and docs explain the *why*, not the *what*.
- Maintainer decision records live in the architecture archive and
  follow the standard format documented there.

Work status does not live in `docs/`. GitHub issues own wanted work,
labels and decisions; milestones own release membership; draft pull
requests own claims and stop-points; OpenSpec changes own behavioral
target state and task truth; ADRs own durable architectural reasons.
See [`../openspec/README.md`](../openspec/README.md) and the shared
protocol in [`../CLAUDE.md`](../CLAUDE.md).
