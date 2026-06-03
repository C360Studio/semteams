# SemTeams Documentation

SemTeams is a reference/demo product on top of the
[semstreams](https://github.com/c360studio/semstreams) framework.
Anything *framework-shaped* (components, graph, rule engine, NATS
streams, payload registry) is documented upstream. Anything
*product-shaped* (UI, configs, personas, rules, product-shell
tools, journeys) is documented here.

## If you are…

- **new to SemTeams (what does it DO?)** →
  [`mvp-chain.md`](mvp-chain.md) — the MVP chain in plain English
- **new to SemTeams (how do I run it?)** →
  [`getting-started.md`](getting-started.md)
- **extending the product shell** →
  [`adr/029-product-shell-wiring.md`](adr/029-product-shell-wiring.md)
  + [`../cmd/semteams/tools/README.md`](../cmd/semteams/tools/README.md)
- **writing a flow / persona / rule** →
  [`../configs/README.md`](../configs/README.md)
  (note: that README is still upstream-shaped — see "Imports" below)
- **writing a Playwright journey** →
  [`journeys/README.md`](journeys/README.md)
- **looking for framework concepts** → upstream
  [semstreams docs](https://github.com/c360studio/semstreams/tree/main/docs)

## What lives in this folder

### SemTeams-native

These are written for this product and contain decisions and shapes
that don't exist upstream.

| Path | Purpose |
|---|---|
| [`mvp-chain.md`](mvp-chain.md) | What semteams DOES — the 7-role MVP chain in plain English |
| [`getting-started.md`](getting-started.md) | New-dev quickstart + debugging recipes |
| [`adr/`](adr/) | Architectural Decision Records — every load-bearing product-shell decision |
| [`proposals/`](proposals/) | Larger design docs that precede / accompany an ADR (agentic-superpowers, ui-redesign, research-flow-open-questions) |
| [`specs/`](specs/) | Domain specs the dev-via-spec chain consumes (e.g. OSH-Meshtastic bridge) |
| [`objectives/`](objectives/) | Per-flow objective specs the ops agent grounds against (ADR-027) |
| [`journeys/`](journeys/) | Pointer doc — the journey *specs* are the Playwright tests under `ui/e2e/agentic/` |
| [`smoke7-osh-meshtastic.md`](smoke7-osh-meshtastic.md) | Smoke run findings — preserved for the integration plumbing they expose |
| [`ui-integration-notes.md`](ui-integration-notes.md) | UI ↔ backend integration notes |

### ADR index (read these first)

The ADRs are the most useful single read for understanding this
product. They're listed roughly in build order; the **bold** ones
are the load-bearing reads for understanding the current
architecture.

| ADR | What it decides |
|---|---|
| [023](adr/023-provider-adapters-and-tool-choice.md) | LLM provider adapters and tool-choice handling |
| [**029**](adr/029-product-shell-wiring.md) | How `cmd/semteams/main.go` wires framework primitives (the load-bearing reference for any new wiring) |
| [030](adr/030-approval-flow-ui-and-identity.md) | Approval-flow UI + the `X-User-Id` identity seam |
| [031](adr/031-research-flow-and-semspec-handoff.md) | Research-flow ownership + dev-via-spec internal mode. *Largely superseded by ADR-042 — dev-via-spec arc retired in MVP-7.* |
| [032](adr/032-r36-sandbox-design.md) | R3.6 builder sandbox design (precursor to ADR-043) |
| [033](adr/033-harness-anchored-verification-and-coordinator-authority.md) | Harness-anchored verification + coordinator-as-decision-authority |
| [034](adr/034-qa-runner-pattern-adoption.md) | QA-runner pattern (verification-runner pivot) |
| [035](adr/035-dev-via-spec-arc.md) | Dev-via-spec arc. *Superseded by ADR-042; retained for archeology.* |
| [036](adr/036-test-harness-lifecycle.md) | Test-harness lifecycle |
| [037](adr/037-chain-failure-handling.md) | Chain failure handling + chain-pause semantics |
| [038](adr/038-chain-entity-and-milestone-rendering.md) | Chain entity + milestone rendering |
| [039](adr/039-needs-clarification-recovery.md) | `needs_clarification` recovery routing |
| [040](adr/040-source-curator-role.md) | Source-curator role split |
| [041](adr/041-mvp-role-compression-and-graph-as-substrate.md) | MVP role compression + graph-as-substrate. Direct precursor to ADR-042. |
| [**042**](adr/042-coordinator-instantiated-flows-via-templates.md) | **Substrate-plus-overlays.** One product-shell flow, category-keyed rule packs + persona bundles. Current architecture as of MVP-7. |
| [**043**](adr/043-devcontainer-as-sandbox-spec.md) | **Devcontainer-as-sandbox spec.** Per-tenant attestation + attestation-aware artifact routing. The killer feature autoresearch needed. |

### Imports from upstream — keep, replace, or skip?

The following directories were carried over verbatim from
semstreams when SemTeams was forked into a product shell. They
explain the **framework**, not the product. Treat them as a
fallback while you're learning, but the canonical version lives
upstream and may have moved on:

- [`basics/`](basics/) — framework getting-started, Graphable
  interface, configuration tiers.
- [`concepts/`](concepts/) — knowledge graphs, embeddings, payload
  registry, agentic-systems, orchestration layers.
- [`advanced/`](advanced/) — agentic components, workflow
  configuration, JetStream tuning.
- [`operations/`](operations/) — local monitoring, troubleshooting,
  distributed tracing.
- [`contributing/`](contributing/) — testing, schema generation,
  contract testing, CI integration.
- [`ROADMAP.md`](ROADMAP.md) — *upstream* roadmap; the SemTeams
  active arc lives in ADR-031.
- [`../configs/README.md`](../configs/README.md) — describes the
  framework's structural / statistical / semantic tiers (graph
  components), not the product's flow library.

When upstream content drifts or gets in the way, prefer the
upstream source of truth at
<https://github.com/c360studio/semstreams/tree/main/docs> over
patching here. A targeted cleanup of these imports is on the
backlog (`project_docs_audit_needed`).

## Conventions

- Single `#` H1 per file; no skipping heading levels.
- Lines under 120 characters where practical.
- Fenced code blocks specify a language.
- Comments and docs explain the *why*, not the *what*.
- ADRs follow the standard format (Status / Context / Decision /
  Consequences / Alternatives / Related). Addenda land in-place
  with a dated heading rather than a new ADR when they refine
  scope without overturning the decision.
