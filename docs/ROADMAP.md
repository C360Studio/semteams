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
- **Spec-driven development (umbrella + deployment roadmap)** —
  [ADR-056 OpenSpec-compatible, environment-gated spec-driven development](adr/056-openspec-spec-driven-development-umbrella.md)
  records the settled decisions and the **north-star deployment roadmap**
  (D0→D6; the *initial deployment surface* is autonomous issue→PR on one
  repo). The first buildable slice is
  [ADR-057 graph spec model + `create_change`](adr/057-openspec-graph-spec-model-and-create-change.md)
  (Proposed). Consistent with the ADR-as-audit-trail stance above: the
  roadmap lives *in* an ADR, not a separate forward-looking doc.
- **Live category packs** — product-facing packs are `research/`,
  `autoresearch/`, `create-change/`, `proof-readiness/`,
  `dev-from-task/`, and `dev-via-test/`. Support packs are
  `coordinator/`, `agent-run/`, and `ops/`. The extension rule
  remains: add a `configs/rules/<new-category>/` directory +
  persona bundle, not a new flow config.
- **Open ADRs / proposals** — the
  [`proposals/`](proposals/) directory holds the few active design
  docs that survived the 2026-06-03 audit. ADRs that are filed but
  not yet shipped carry a `**Status:** Filed` marker; shipped ones
  carry `**Status:** Accepted + Shipped`.

## Framework roadmap

The **semstreams** framework — components, graph engine, rule
engine, NATS stream wiring, gateways — has its own roadmap upstream
at <https://github.com/c360studio/semstreams>. This product depends
on the framework but does not track its roadmap here.

The earlier content of this file was the upstream SemStreams roadmap
carried over verbatim during the initial subtree import. It was
moved out 2026-06-03 (see the audit memo in `project_ui_audit_post_mvp7`
for the broader docs-cleanup context).
