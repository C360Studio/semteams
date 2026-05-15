# The SemTeams MVP Chain

A new-user orientation to what SemTeams does when you send it a
prompt. If you've just cloned the repo and want to understand the
moving parts before reading ADRs, start here.

Framework background: SemTeams is a reference/demo product on top
of [semstreams](https://github.com/c360studio/semstreams). The
framework provides the agentic loop, the rule engine, NATS streams,
the graph. SemTeams contributes a thin product shell (`cmd/semteams/`),
the personas + rules in `configs/`, the UI, and the e2e + smoke
test harnesses. This doc covers what the *chain* does — the
sequence of agent roles that processes one user prompt end-to-end.

## What semteams does when you send it a prompt

A user prompt enters via the dispatch endpoint (HTTP `POST
/teams-dispatch/message` or the chat UI). The dispatch loop's
default role under the MVP roster is `researcher-plan`. That loop
processes the prompt, terminates with a decision, and a rule fires
that spawns the next role. Roles hand off through a sequence of
loops until either the chain reaches its terminal (reviewer-qa
accepts the builder's output) OR a recovery cap fires (the chain
got stuck) OR the chain runs out of iterations / time.

The chain doesn't centralize state in a single orchestrator — each
role is a fresh loop spawned by a rule, with its inputs threaded
through the rule's prompt template + the loop result of the prior
role. The framework's KV / streams carry chain-level state (the
"graph" — see §"The graph is internal harness state" below) but
agents don't read it directly; they read the previous loop's
terminal and on-disk rendered artifacts.

## The chain at a glance

```
                            ┌─────────────┐
                user prompt │  dispatch   │ (spawns researcher-plan)
                ───────────▶│             │
                            └──────┬──────┘
                                   │ decide(action="gather")
                                   ▼
                            ┌─────────────┐
                            │  researcher │  Plan + structure
                            │    -PLAN    │  (epics, verifiable
                            └──────┬──────┘   outcomes)
                                   │ decide(action="gather")
                                   ▼
                            ┌─────────────┐
                            │  researcher │  Ground actors / facts
                            │   -GATHER   │  via web_search
                            └──────┬──────┘
                                   │ decide(action="synthesize")
                                   ▼
                            ┌─────────────┐
                            │  researcher │  Compose structured
                            │ -SYNTHESIZE │  research artifact
                            └──────┬──────┘   (actors, integration
                                   │           points, tasks)
                                   │ decide(action="architect")
                                   ▼
                            ┌─────────────┐
                            │  researcher │  Spec artifact with
                            │ -ARCHITECT  │  verification checks[]
                            └──────┬──────┘
                                   │ decide(action="emit")
                                   ▼
                            ┌─────────────┐
                            │  reviewer   │  Grade spec for
                            │   -SPEC     │  structural completeness
                            └──────┬──────┘
                                   │ decide(action="approved")
                                   ▼
                            ┌─────────────┐
                            │   builder   │  Implement + test
                            │             │  in sandbox via bash
                            └──────┬──────┘
                                   │ builder_decide(tests_passing)
                                   ▼
                            ┌─────────────┐
                            │  reviewer   │  Grade build evidence
                            │    -QA      │
                            └──────┬──────┘
                                   │ decide(action="accept")
                                   ▼
                                TERMINAL
```

Each box is one loop. Solid lines are rule-fired transitions. The
dotted recovery path (not drawn) routes any role's
`needs_clarification` or reviewer's `insufficient` back through
`researcher-plan` for re-planning, bounded by the recovery cap.

Two ops roles operate *off* the chain on a parallel observability
track:

- `ops-chain-observer` — wakes once at reviewer-qa terminal, walks
  the completed chain end-to-end, emits diagnoses for human review.
- `ops-progress-observer` — wakes every 5 non-terminal completions,
  checks whether the in-flight chain is spinning or stalled.

## What each role does

### researcher-PLAN

Reads the user prompt (via `read_loop_result` on dispatch). Produces
the *plan* — a structured artifact naming scope, epics, and
verifiable outcomes. Renders to `/artifacts/plans/<slug>.md` via
`emit_plan`. Terminal: `decide(action="gather")`.

No corpus exploration — that's gather's job. Plan is pure scoping.

### researcher-GATHER

Reads the plan. Grounds the plan's actor names and integration
points in external facts via `web_search`. Accumulates findings in
`scratchpad` (loop-private). Terminal:
`decide(action="synthesize", reason="<summary>")` carrying the
evidence summary forward in the reason text.

Per ADR-041 addendum 2026-05-15, gather does NOT have graph-query
tools — chain agents do not read the internal graph state. If
`web_search` cannot ground the plan, gather terminates
`decide(action="needs_clarification")` honestly rather than
fabricating.

### researcher-SYNTHESIZE

Reads the plan + gather's narrative summary. Composes the structured
research artifact — `actors[]`, `integration_points[]`, `tasks[]`,
`addressed_gaps[]`, `open_gaps[]` — via `emit_research_artifact`.
Renders to `/artifacts/research/<slug>.md`. Terminal:
`decide(action="architect", reason="<artifact slug + revision>")`.

### researcher-ARCHITECT

Reads the research artifact via
`bash cat $entity.triple.research.artifact.path` (the spawn rule
substitutes the literal path). Adds verification `checks[]` binding
each task to an executable evidence rule. Produces the dev-via-spec
artifact via `emit_dev_via_spec_artifact`. Renders to
`/artifacts/specs/<slug>.md`. Terminal: `decide(action="emit")`.

This is the architect for one chain pass — not a separate role.
"Architect phase" of researcher.

### reviewer-SPEC

Reads the spec artifact via
`bash cat $entity.triple.dev_via_spec.artifact.path`. Grades against
the completeness checklist (every check binds to evidence; every
external actor has at least one task; every task grounds against
actors and integration_points). Terminal:
`decide(action="approved")` to proceed, or
`decide(action="insufficient", reason="<gaps>")` to bounce back
through the recovery cap.

### builder

Reads the approved spec via `bootstrap_workspace` (creates a
chain-scoped sandbox worktree and seeds SPEC.md) then iterates
with `bash` to implement and test. The sandbox is shared across
the whole chain — see ADR-041 Phase 4 (chain-scoped bash). Terminal:
`builder_decide(action="tests_passing", tests_run=N, tests_passed=M,
tests_failed=K)` carrying structured test evidence.

### reviewer-QA

Reads the builder's structured terminal + the evidence-summary
preprocessor's rendered check-by-check report. Grades whether the
builder's evidence actually proves the architect's checks (not
just that tests ran green). Terminal: `decide(action="accept")` to
terminate the chain successfully, or
`decide(action="needs_clarification")` / `reject` to route back.

This is the chain's terminal role. Reviewer-qa's accept fires the
`ops-chain-observer` rule for post-hoc diagnosis.

## Failure modes

### needs_clarification → recovery

Any role can terminate `decide(action="needs_clarification",
reason="<blocking question>")` when it genuinely can't proceed.
Rule 03 (`dev-via-spec/03-needs-clarification-to-researcher.json`)
fires on any role's needs_clarification → spawns `researcher-plan`
to re-plan with the gap addressed.

Bounded by the chain-level recovery cap (ADR-039 Phase 1 / ADR-040
§"chain-level recovery cap", default 3 cycles). After 3 recoveries
the chain pauses for human review.

### Reviewer rejection → re-plan

Reviewer terminates `decide(action="insufficient",
reason="<gaps>")`. Rule 02 in research-mode-transition fires →
researcher-plan recovery. Same cap.

### Per-phase iteration caps

Each researcher phase has its own per-chain cap (plan=1, gather=3,
synthesize=2, architect=2 per ADR-041 §"Per-phase cap"). The
`phasevalidator` package enforces these — a phase that would
exceed cap fails the transition validation and the chain pauses.

### Builder iteration cap

`agentic-loop.max_iterations` (typically 100 for osh-demo) bounds
the builder's `bash` iteration count. Hitting the cap without a
`builder_decide` terminal pauses the chain.

## The graph is internal harness state

Per ADR-041 addendum 2026-05-15: **chain agents do not read the
graph. The graph is internal harness state — audit, lineage,
milestone stamping, evidence aggregation. Only ops agents read it.**

What each reader class has access to:

| Reader | Reads |
|---|---|
| Chain agents (researcher, reviewer, builder) | `read_loop_result` on prior loop terminal · `bash` on `/artifacts/<kind>/<slug>.md` filesystem · `web_search` external |
| Ops agents (`ops-*`) | All graph-query tools (`query_entity`, `summarize_graph`, etc.) — observing harness state IS their job per ADR-027 |
| Operators (you, partners, auditors) | Rendered markdown in `/artifacts/`, ADRs, persona docs, chain-entity UI |

The intuition: chain agents are *authors*. They write artifacts
(SDD-zone) and chain-state triples (fact-set). Ops agents are
*auditors*. They read both, emit diagnoses (which are SDD-zone for
operators). Operators read everything that's been rendered for
human consumption.

This split is what keeps the chain honest — chain agents can't
introspect their own harness state to optimize for it. They reason
from external facts (web), filesystem artifacts, and their own loop
terminal. Nothing else.

## Where to read deeper

- **The compression decision and policy:**
  [`adr/041-mvp-role-compression-and-graph-as-substrate.md`](adr/041-mvp-role-compression-and-graph-as-substrate.md)
  — the ADR that landed the 4-role MVP chain. The 2026-05-15
  addendum at the bottom is the graph-as-internal-state policy.
- **Per-role personas:**
  `configs/personas/fragments/<role>/` — each chain role has
  identity, output-contract, and (where applicable)
  iteration-rules fragments. Read like job descriptions.
- **The spawn rules:**
  `configs/rules/research-mode-transition/` and
  `configs/rules/dev-via-spec/` — rule JSON files defining
  which role spawns which next, on what condition.
- **The product shell wiring:**
  [`adr/029-product-shell-wiring.md`](adr/029-product-shell-wiring.md)
  — how `cmd/semteams/main.go` boots the chain on top of upstream
  semstreams primitives.
- **Sandbox + chain-scoped bash:**
  [`adr/032-r36-sandbox-design.md`](adr/032-r36-sandbox-design.md) +
  ADR-041 Phase 4 (chain-scoped bash wrapper) — the
  chain-shared worktree the architect/reviewers/builder all
  bash into.
- **Smoke playbook:**
  [`smoke7-osh-meshtastic.md`](smoke7-osh-meshtastic.md) — how
  to run a real-LLM smoke + capture evidence.

## What a successful chain looks like

(This section gets filled in from smoke #27 evidence — annotated
trajectory walkthrough of a chain that reached reviewer-qa accept
terminal. Until smoke #27 lands, the closest reference is
[`smoke7-osh-meshtastic.md`](smoke7-osh-meshtastic.md) §"Expected
chain shape".)
