# SemTeams Roadmap

SemTeams is a reference/demo product on top of the
[semstreams](https://github.com/c360studio/semstreams) framework.
The product-level "roadmap" is captured in ADRs as discrete decisions
land — there is no single forward-looking roadmap document, by
design: the ADR sequence is the audit trail.

## Where to look for current direction

- **Current architecture** —
  [ADR-042 substrate-plus-overlays](adr/042-coordinator-instantiated-flows-via-templates.md)
  is the load-bearing design as of MVP-7. One product-shell flow,
  category-keyed rule packs + persona bundles add task classes
  without new components.
- **Sandbox + attestation** —
  [ADR-043 devcontainer-as-sandbox spec](adr/043-devcontainer-as-sandbox-spec.md)
  defines per-tenant attestation + attestation-aware artifact
  routing. Shipped 2026-06-03 alongside the autoresearch pack.
- **Live category packs** — `research/` and `autoresearch/`. The
  next pack is dropping in as a `configs/rules/<new-category>/`
  directory + persona bundle, not a new flow config.
- **Open ADRs / proposals** — the
  [`proposals/`](proposals/) directory holds active design docs
  (e.g. `agentic-superpowers.md`, `ui-redesign.md`). ADRs that are
  filed but not yet shipped carry a `**Status:** Filed` marker.

## Framework roadmap

The **semstreams** framework — components, graph engine, rule
engine, NATS stream wiring, gateways — has its own roadmap upstream
at <https://github.com/c360studio/semstreams>. This product depends
on the framework but does not track its roadmap here.

The earlier content of this file was the upstream SemStreams roadmap
carried over verbatim during the initial subtree import. It was
moved out 2026-06-03 (see the audit memo in `project_ui_audit_post_mvp7`
for the broader docs-cleanup context).
