# SemTeams Architecture: substrate + category packs

A new-user orientation to what happens when you send SemTeams a
prompt, and the moving parts that make it happen. For *why* it is
built this way read the ADRs — especially
[ADR-042](adr/042-coordinator-instantiated-flows-via-templates.md)
(substrate-plus-overlays) and
[ADR-043](adr/043-devcontainer-as-sandbox-spec.md)
(devcontainer-as-sandbox).

Framework background: SemTeams is a reference/demo product on top
of [semstreams](https://github.com/c360studio/semstreams). The
framework provides the agentic loop, the rule engine, NATS streams,
and the graph. SemTeams contributes a thin product shell
(`cmd/semteams/`), the personas + rules + tool executors in
`configs/` and `cmd/semteams/tools/`, and the UI. This doc covers
what the *runtime* does — the sequence of agent loops that
processes one user prompt end-to-end.

## What semteams does when you send it a prompt

A user prompt enters via the dispatch endpoint (HTTP `POST
/teams-dispatch/message` or the chat UI). The dispatch default role
is **coordinator** — the chat front-door. The coordinator reads
the prompt, decides which **task class** the user is asking about
via `decide(action=<class>)`, and a rule fires that spawns the
first loop of that class's category pack.

The chain then runs as a sequence of loops, each spawned by a rule
firing on the previous loop's terminal decision. Eventually a
terminal loop fires a rule that spawns a coordinator wake-up loop;
the wake-up coordinator reads the chain's rendered artifact and
replies to the user via `decide(action="respond_direct")`.

The product shell holds no orchestrator. Every loop is a fresh
spawn carrying just (a) its persona fragments, (b) its tool
allowlist, (c) its `decide`-action allowlist, and (d) lineage
triples threaded through `related_loops` at spawn time. Cross-loop
state lives in the graph (chain entity + attached triples) and in
rendered artifacts on disk. **Chain agents do not read the graph
directly** — they read the previous loop's terminal via
`read_loop_result` and on-disk artifacts via `bash cat`. The graph
is internal harness state. (ADR-041 §"graph as substrate"
addendum 2026-05-15.)

## The substrate-plus-overlays architecture

```
┌──────────────────────────────────────────────────────────────────┐
│ Substrate (one process, configs/flow-bootstrap.json, ADR-042)    │
│                                                                  │
│   agentic-dispatch  ── intake + default role + chat front-door   │
│   agentic-loop      ── think/act state machine per loop          │
│   agentic-model     ── LLM client (OpenAI / Anthropic / Gemini)  │
│   agentic-tools     ── tool executor registry                    │
│   graph-ingest      ── triple ingest (chain entity, milestones)  │
│   graph-query       ── ops-only triple read API                  │
│   rule-processor    ── conditions → on_enter actions             │
│                                                                  │
│   coordinator persona  + decide-action contract                  │
└──────────────────────────────────────────────────────────────────┘
                                  │
                ┌─────────────────┼─────────────────┐
                ▼                 ▼                 ▼
     ┌──────────────────┐  ┌──────────────────┐  ┌──────────────────┐
     │ research/        │  │ autoresearch/    │  │  (future pack)   │
     │ rule pack +      │  │ rule pack +      │  │                  │
     │ personas         │  │ personas         │  │                  │
     └──────────────────┘  └──────────────────┘  └──────────────────┘
```

Substrate = the singleton components wired by `cmd/semteams/main.go`.
Overlays = category-keyed rule packs (`configs/rules/<category>/`)
+ named persona bundles (`configs/personas/fragments/<role>/`).

Adding a new task class is a new overlay — drop a rule pack
directory, drop persona bundles for the new roles, teach the
coordinator's `decide(action=…)` contract one new token. **No new
components, no new flow configs.**

## Live category packs

Two packs ship today. Each terminates with a coordinator wake-up
that reads the chain's artifact and replies to the user.

### research

Prose research arc. Coordinator dispatches when the user is asking
for a comparison, a synthesis, or a how-does-X-work question.

```
coordinator(dispatch) ──decide(research)──▶ researcher-research-plan
                                                    │
                                                    ▼
                                          (for_each subtopic)
                                                    │
                          ┌─────────────────────────┼─────────────────────────┐
                          ▼                         ▼                         ▼
            researcher-research-gather   researcher-research-gather   ...  (N gatherers,
            (subtopic_1)                 (subtopic_2)                       one per subtopic)
                          └─────────────────────────┼─────────────────────────┘
                                                    ▼
                                       researcher-research-synthesize
                                                    │
                                                    ▼
                                              reviewer-research
                                                    │
                                                    ▼
                                       coordinator(wake-up) ──▶ user reply
```

Per-role contracts:

- **researcher-research-plan** — reads the coordinator's prompt
  via `read_loop_result`. Emits a plan via `emit_plan` (goal,
  scope_in, scope_out, subtopics[]). Terminal `decide(gather)`.
  The `subtopics` array drives the fan-out cardinality at the
  next rule.
- **researcher-research-gather (×N)** — one loop per subtopic,
  spawned by `for_each` fan-out. Each is scoped to **its own
  subtopic only**. Uses `web_search` + `bash curl` for external
  evidence. Terminal `decide(synthesize, reason="Subtopic: <…>\n\n<findings>")`.
- **researcher-research-synthesize** — joins after all gatherers
  stamp completion. Reads the plan + each sibling gatherer's
  reason via `read_loop_result`. Emits the aggregate research
  artifact via `emit_research_artifact` (actors,
  integration_points, tasks, addressed_gaps, open_gaps). Terminal
  `decide(emit)`.
- **reviewer-research** — reads the rendered artifact via
  `bash cat $entity.triple.research.artifact.path`. Grades for
  structural completeness + evidence grounding. Terminal
  `decide(approved)` or `decide(insufficient, reason=<gaps>)`.
- **coordinator (wake-up)** — reads the reviewer's terminal +
  reads the artifact. Composes a user-facing prose reply via
  `decide(respond_direct, reason=<reply>)`.

### autoresearch

Karpathy-style propose/execute iteration loop. Coordinator
dispatches when the user is asking to **optimize a measurable
metric** (latency, footprint, error count). Each iteration proposes
a single-axis change, runs the measurement command, and KEEPS or
REVERTS based on an empirical comparison.

```
coordinator(dispatch) ──decide(autoresearch)──▶ autoresearch-baseline
                                                       │
                                                       ▼
                                       ┌── (iteration N≤cap) ──┐
                                       │                       │
                              autoresearch-propose             │
                                       │                       │
                                       ▼                       │
                              autoresearch-execute             │
                              (bash + emit_autoresearch_       │
                                  measurement,                 │
                                  outcome: kept / reverted)    │
                                       │                       │
                                       └─── stamp marker ──────┘
                                                       │ (cap exhausted)
                                                       ▼
                                       autoresearch-synthesize
                                                       │
                                                       ▼
                                              reviewer-autoresearch
                                                       │
                                                       ▼
                                       coordinator(wake-up) ──▶ user reply
```

Per-role contracts:

- **autoresearch-baseline** — parses the four autoresearch
  parameters (command, surface, cap, metric_parser) from the
  coordinator's reason. Runs the measurement command verbatim,
  parses a baseline number. Emits via `emit_autoresearch_baseline`
  — stamps `autoresearch.{command, surface, cap, metric_parser,
  baseline.value, best.value, best.experiment_id}` on the
  coordinator's loop entity (the run entity).
- **autoresearch-propose (×N)** — one loop per iteration. Reads
  the journal (every prior iteration's outcome via
  `read_loop_result`) + the running best. Forms a single-axis
  hypothesis, edits files within `surface`, terminal
  `decide(measure)` carrying the hypothesis in `reason`.
- **autoresearch-execute (×N)** — runs the measurement command in
  the per-tenant devcontainer (ADR-043). Calls
  `emit_autoresearch_measurement`: the executor reads prior best,
  compares this iteration's value, stamps `kept` or `reverted` on
  the execute loop entity. If kept, the executor also upserts
  `best.value` and `best.experiment_id` on the run entity.
- **autoresearch-synthesize** — joins at cap-exhaust. Walks every
  iteration's terminal via `read_loop_result`. Emits the run
  artifact via `emit_autoresearch_artifact` (baseline,
  per-iteration journey, running best, recommended action). The
  artifact is written **into the per-tenant devcontainer's
  workspace** via the sandbox `/exec` endpoint, so the reviewer's
  `bash cat` resolves to the same path from both sides (ADR-043
  attestation-aware routing, semteams#194).
- **reviewer-autoresearch** — reads the rendered artifact via
  `bash cat $entity.triple.autoresearch.artifact.path`. Grades
  the improvement narrative + checks that kept iterations actually
  moved the metric. Terminal `decide(approved)` or
  `decide(insufficient)`.
- **coordinator (wake-up)** — reads the reviewer's terminal +
  reads the artifact. Replies to the user with the
  best.value / baseline ratio + the kept-journey summary.

The iteration loop's `propose → execute` cycle is driven by a
single rule (`05-iteration-dispatch.json`) using the
**presence-marker pattern** (semstreams#204): on each execute
completion, rule 04 stamps
`autoresearch.iteration.pending = <execute-entity-id>`; rule 05
fires on the marker, clears it (`remove_triple` so the next stamp
re-fires the rule), and spawns the next propose. At iteration > cap
the propose-spawn gate flips false and the synthesize-spawn action
fires instead. See the rule's prose description for the full
mechanic.

## Ops roles (parallel observability track)

Two ops roles operate *off* the chain on a parallel track. They
read the graph (chain agents do not) and emit diagnoses for human
review:

- **ops-chain-observer** — wakes once at the chain's terminal
  (reviewer's `approved` or coordinator's `respond_direct`),
  walks the completed chain end-to-end, emits diagnoses about
  role coverage, evidence completeness, and recovery cycles.
- **ops-progress-observer** — wakes every 5 non-terminal
  completions, checks whether the in-flight chain is making
  forward progress or spinning.

Diagnoses are stamped as `ops.diagnosis.*` triples and rendered in
the chain-entity UI for the operator. Phase 2
(ops-proposes-changes) is config-only per ADR-027 — `create_rule`
/ `manage_flow` would gate through the approval filter for human
review.

## Failure modes

### needs_clarification → coordinator recovery

Any chain role can terminate
`decide(action="needs_clarification", reason="<blocking question>")`
when it genuinely cannot proceed. A rule fires on
`needs_clarification` → spawns a coordinator wake-up with the
blocking question as context. The coordinator can ask the user a
follow-up (`decide(ask_user)`) or terminate the chain. This routes
chain-internal ambiguity to the human, not to a self-recovery
loop. (ADR-039.)

### Reviewer rejection → re-synthesize / re-propose

`reviewer-research` rejecting an artifact (`decide(insufficient)`)
routes back to `researcher-research-synthesize` with the reviewer's
gap list. `reviewer-autoresearch` rejection routes back to
`autoresearch-synthesize`. Bounded by the per-pack recovery cap.

### Iteration / time caps

Each agentic loop has a `max_iterations` bound (default 8 for
research roles, 20 for coordinator, configurable per role via
persona). Hitting the cap without a terminal `decide` fails the
loop with `outcome=failed`; the parent rule decides whether to
recover or pause the chain (ADR-037 chain-pause).

### Chain pause

A loop failing with `outcome=failed` stamps `chain.paused` on the
chain entity. The UI surfaces this as `awaiting_approval` on the
parent task; the operator approves a resume, restarts a phase, or
cancels the chain. (ADR-037 + ADR-039.)

## The graph is internal harness state

Per ADR-041 addendum 2026-05-15: **chain agents do not read the
graph. The graph is internal harness state — audit, lineage,
milestone stamping, evidence aggregation. Only ops agents read it.**

What each reader class has access to:

| Reader | Reads |
|---|---|
| Chain agents (research, autoresearch, reviewers) | `read_loop_result` on prior loop terminals · `bash` on `/artifacts/<kind>/<slug>.md` · `web_search` external · per-tenant devcontainer filesystem (autoresearch) |
| Ops agents (`ops-*`) | All graph-query tools (`query_entity`, `summarize_graph`, `read_loop_result`, etc.) — observing harness state IS their job per ADR-027 |
| Operators (you, partners, auditors) | Rendered markdown in `/artifacts/`, ADRs, persona docs, chain-entity UI, ops diagnoses |

The intuition: chain agents are *authors*. They write artifacts
and chain-state triples. Ops agents are *auditors*. They read both
and emit diagnoses (which are themselves human-readable, for
operators). Operators read everything that's been rendered for
human consumption.

This split is what keeps the chain honest — chain agents cannot
introspect their own harness state to optimize for it. They reason
from external facts (web), filesystem artifacts (their previous
loops' output), and their own loop terminal. Nothing else.

## Where to read deeper

- **Substrate-plus-overlays decision:**
  [`adr/042-coordinator-instantiated-flows-via-templates.md`](adr/042-coordinator-instantiated-flows-via-templates.md)
  — the load-bearing ADR for the current architecture. §Phase 2
  redesign is what actually shipped.
- **Sandbox + attestation:**
  [`adr/043-devcontainer-as-sandbox-spec.md`](adr/043-devcontainer-as-sandbox-spec.md)
  — per-tenant devcontainer attestation + attestation-aware
  routing. The killer feature autoresearch needed.
- **Per-pack rule packs:** `configs/rules/research/README.md` +
  `configs/rules/autoresearch/README.md` — every rule has a
  description block explaining the trigger conditions, actions,
  and the why (often including the upstream-framework workarounds
  being deployed). Worth reading for any pack work.
- **Per-role personas:**
  `configs/personas/fragments/<role>/` — identity, output
  contract, iteration rules per role. Read like job descriptions.
- **Product shell wiring:**
  [`adr/029-product-shell-wiring.md`](adr/029-product-shell-wiring.md)
  — how `cmd/semteams/main.go` boots the substrate on top of
  upstream semstreams primitives.
- **Ops agent:** upstream
  [`semstreams adr/027`](https://github.com/c360studio/semstreams/blob/main/docs/adr/027-ops-agent-meta-harness.md)
  + this repo's `objectives/` directory + `ops-chain-observer` /
  `ops-progress-observer` persona dirs.
- **Sponsor packs:** `sponsor-packages/` — annotated trajectory
  walkthroughs of completed chains. First entry:
  `research-pack-fan-out-2026-05-29`.
