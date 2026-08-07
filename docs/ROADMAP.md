# SemTeams Roadmap

SemTeams is moving toward a governed agentic-development harness: a
coordinator front door, reusable capability packs, visible artifacts, proof
gates, sandboxed execution, and auditable runs.

This page is product-facing direction, not an exhaustive issue tracker. The
current runnable truth is still the shipped configs, journeys, and demo evidence
pack.

## Ready for MVP demo

- **Coordinator front door** — humans can chat first, refine an idea, ask which
  team fits, or send a ready task. Team slash commands are supported as
  coordinator-routed hints.
- **Research pack** — evidence-gathering research flow with planning, gather,
  synthesis, review, artifact emission, and coordinator wake-up.
- **Spec creation path** — OpenSpec-compatible authoring, review, export, and
  readiness gating for governed handoff.
- **Artifact context handoff** — emitted artifacts can be copied or attached to
  a new prompt so research, specs, optimization summaries, and implementation
  evidence can inform the next team.
- **Sandboxed build and optimization paths** — autoresearch and dev-via-test use
  attested per-tenant devcontainers when run against the real sandbox runner.
- **Black-box demo evidence** — Playwright journeys prove the claim boundary
  through public UI, dispatch, task-runner, approval, export, and documented
  observation surfaces.

## Near-term product direction

- Make the run UI better at answering "what is happening now?" without asking
  users to inspect graph facts or loop internals.
- Continue smoothing artifact reuse across teams, including clearer labels for
  what artifact is attached and how it will influence the next prompt.
- Expand proof-readiness UX so blocked work explains the missing dependency,
  available waiver path, and residual risk in plain language.
- Tighten the spec-to-dev bridge until approved specs can move into governed
  implementation with less fixture seeding and more live evidence.
- Keep capability packs as the extension unit: new job class means new rules,
  persona bundles, tool permissions, tests, and docs, not a new runtime.

## Framework boundary

The semstreams framework owns components, graph engine, rule engine, NATS stream
wiring, gateways, and reusable agentic primitives. SemTeams should stay a thin
product shell: UI, product configs, personas, rules, product tools, journeys,
and docs that prove how to assemble those primitives into governed agentic
workflows.

When a new feature looks framework-shaped, first check whether it belongs
upstream. Product-local additions should stay narrow, documented, and tied to a
SemTeams user journey.
