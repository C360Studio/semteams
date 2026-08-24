# SemTeams Roadmap

SemTeams is moving toward a governed agentic-development harness: a
coordinator front door, reusable capability packs, visible artifacts, proof
gates, sandboxed execution, and auditable runs.

This page is product-facing direction, not an exhaustive issue tracker. The
current runnable truth is still the shipped configs, journeys, and demo evidence
pack.

## Ready for MVP demo

The demo scope is the inner and outer loops
([ADR-058](adr/058-beta159-realignment-and-demo-lane-focus.md)):

- **Coordinator front door (outer loop)** — humans can chat first, refine an
  idea, ask which team fits, or send a ready task. Team slash commands are
  coordinator-routed hints. Asks for parked teams get an honest direct
  response.
- **Research pack (inner loop)** — evidence-gathering research flow with
  planning, parallel gather fan-out, join, synthesis, review, artifact
  emission with recoverable sources, and coordinator wake-up.
- **Autoresearch pack** — metric optimization with empirical keep/revert
  decisions in an attested per-tenant devcontainer.
- **Black-box demo evidence** — Playwright journeys prove the claim boundary
  through public UI, dispatch, task-runner, and documented
  observation surfaces.

## Parked beta.160 regression (ADR-059)

Artifact-card content and artifact-context handoff lost their evidence-body
source when the UI moved to the beta.160 GraphQL trajectory surface. The
pre-cutover OpenSpec change is retained in Git history, not the active queue.
Issue [#261](https://github.com/C360Studio/semteams/issues/261) owns an
authorized `StorageReference` evidence-fetch contract and the freshly
reconciled OpenSpec change required for resumption.

## Parked pending predicate re-authoring (ADR-058)

The spec-authoring and software-implementation packs (create-change,
proof-readiness, dev-from-task, dev-via-test) are parked in place: files stay
in-repo, nothing is wired. Restoring one is a deliberate migration — re-author
its predicates under the upstream canonical contract, re-wire the bootstrap,
restore the taxonomy token, and un-park its tests and journeys.

## Near-term product direction

- Prove the inner and outer loops on real LLMs as the standing demo, and keep
  the mock journeys as the merge gate.
- Make the run UI better at answering "what is happening now?" without asking
  users to inspect graph facts or loop internals.
- Restore evidence-backed artifact rendering and context handoff only after the
  #261 authorized evidence-fetch contract is approved.
- Re-author and re-wire the parked packs, starting with the smallest
  (create-change) once the demo lanes are proven on the new framework.
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
