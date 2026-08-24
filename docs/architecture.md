# SemTeams Architecture: substrate + category packs

A new-user orientation to what happens when you send SemTeams a
prompt, and the moving parts that make it happen.

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
is internal harness state.

The coordinator can also decide that no team should run yet. In
that mode it answers directly, asks a clarifying question, or helps
the human shape the request before routing. Slash commands such as
`/research` and `/optimize` are team hints carried through the same
front door, not a bypass around the coordinator. Asks for the parked
spec/build teams (ADR-058) get an honest direct answer.

Rendered artifacts are reusable context. The UI can attach an
artifact to the next chat prompt, then the coordinator receives the
artifact title, source tool, and content along with the user's new
request. Any artifact can anchor a follow-up discussion before the
next team is chosen.

## The substrate-plus-overlays architecture

```
┌──────────────────────────────────────────────────────────────────┐
│ Substrate (one process, configs/flow-bootstrap.json)             │
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
	                     ┌────────────┴────────────┐
	                     ▼                         ▼
	          ┌──────────────────┐      ┌──────────────────┐
	          │ research/        │      │ autoresearch/    │
	          │ rule pack +      │      │ rule pack +      │
	          │ personas         │      │ personas         │
	          └──────────────────┘      └──────────────────┘

	  (parked, on disk but unwired — ADR-058: create-change,
	   proof-readiness, dev-from-task, dev-via-test)
```

Substrate = the singleton components wired by `cmd/semteams/main.go`.
Overlays = category-keyed rule packs (`configs/rules/<category>/`)
+ named persona bundles (`configs/personas/fragments/<role>/`).

Adding a new task class is a new overlay — drop a rule pack
directory, drop persona bundles for the new roles, teach the
coordinator's `decide(action=…)` contract one new token. **No new
components, no new flow configs.**

## Live category packs

The live product-facing packs are **research** (the inner-loop
fan-out arc) and **autoresearch** (metric optimization). Each
terminal chain either wakes the coordinator to reply to the user or
pauses behind a visible human decision. The demo claim boundary
lives in [`demo-mvp-claims.md`](demo-mvp-claims.md).

The dev-via-test description below is retained as reference for the
parked pack (ADR-058) — it is on disk but unwired.

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
  the per-tenant devcontainer. Calls
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
  `bash cat` resolves to the same path from both sides.
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

### dev-via-test (PARKED — ADR-058)

Decompose-and-dispatch software development. The coordinator
dispatches when the user asks to **build or change code against a
verifiable acceptance** ("add an endpoint that…", "implement a
parser with tests"). One planner decomposes the ask into tasks;
each task converges in the sandbox against its own test; **one
reviewer (CBG) gates twice** — the *plan* at chain-start and the
*work* at chain-end.

```
coordinator(dispatch) ──request_sandbox──▶ (per-tenant devcontainer; see §sandbox)
        │
        └──decide(dev_via_test)──▶ dev-via-test-plan (Lisa)
                                          │ emit_dev_via_test_plan
                                          ▼
                                 reviewer-dev-via-test  ◀── PLAN GATE (no tests run)
                                  decide(plan_approved | plan_rejected_retry | plan_rejected)
                                          │ approved
                                          ▼
                          ┌──── coordinator (between tasks) ◀────────┐
                          │  decide(dev_via_test,            │ (next ready task)
                          │         subtopics=[task])        │
                          ▼                                  │
                 dev-via-test-execute (Ralph)               │
                 (bash edit→test→emit_dev_via_test_          │
                  measurement; iterate until pass)          │
                          └──────────────────────────────────┘
                                          │ (all tasks done) decide(dev_via_test_finalize)
                                          ▼
                                 reviewer-dev-via-test  ◀── WORK GATE (runs integration test + diff)
                                  decide(approved | rejected_retry | rejected)
                                          │ approved
                                          ▼
                                 coordinator(wake-up) ──▶ user reply
```

Per-role contracts:

- **dev-via-test-plan (Lisa)** — reads the user's ask (preserved in
  the coordinator's `decide` reason) via `read_loop_result`. Emits
  a Karpathy-shaped plan via `emit_dev_via_test_plan`: per-task
  `goal`, `assumptions[]`, `non_goals[]`, `target_files[]`, and a
  `test_command` — the **required schema fields encode the planning
  discipline structurally**, so a plan can't ship without surfacing
  scope and a verifiable acceptance. Stamps `plan.*` triples on the
  run entity. Terminal `decide(planned)`.
- **reviewer-dev-via-test (CBG) — plan gate** — reads the ask +
  the emitted plan in one `query_entity` call (both live on the run
  entity) and checks **fidelity**: did every explicit user
  constraint (named libraries, "do not hand-roll", required test
  types) survive into the plan, and is each `test_command` a real
  test rather than a bare `go build`? Terminal `decide(plan_approved
  | plan_rejected_retry | plan_rejected)`. *Why this gate exists:*
  a planner can read a constraint and still drop it at emit; Ralph
  then builds to the soft spec and wastes a chain before the work
  gate catches it. Catching it here is cheap.
- **coordinator (between tasks)** — woken between tasks. Reads
  `plan.task.*` + the per-task completion markers, picks the
  lowest-position ready task, and dispatches it
  (`decide(dev_via_test, subtopics=["<task>"])`). When all tasks
  are done, `decide(dev_via_test_finalize)`. Plan state lives as
  triples on the run entity, so the walk is **resumable** — kill
  the process mid-chain, restart, and the coordinator reads where it
  left off.
- **dev-via-test-execute (Ralph ×N)** — one loop per task (and one
  more per retry). Reads its task spec via `query_entity`, edits
  files within `target_files` in the per-tenant devcontainer,
  runs the task's `test_command`, and calls
  `emit_dev_via_test_measurement(pass=<exit==0>)`. Iterates until
  the test passes; terminal `decide(measured)`.
- **reviewer-dev-via-test (CBG) — work gate** — at chain-end runs
  the plan's full `integration_test_command` in the devcontainer
  and reads the cumulative `git diff` against the chain-start tag —
  the deterministic cross-task-drift catcher. Terminal
  `decide(approved | rejected_retry | rejected)`.
- **coordinator (wake-up)** — reads CBG's verdict and replies to
  the user (`respond_direct` on approve, `ask_user` on a
  human-needed reject).

**Both gates are bounded reject/retry/approve**. A `*_rejected_retry`
routes back for a bounded fix — the work
gate re-dispatches **Ralph** with the finding (re-implement); the
plan gate re-dispatches **Lisa** with the finding (re-plan, which
*upserts* the plan in place). Each retry budget
(`plan.cbg_retry_budget`, `plan.lisa_retry_budget`) is plan-data
clamped to a small ceiling; exhaustion escalates to the user. A
plain `*_rejected` (plan/scope/human problem) escalates immediately
— the fail-safe default. The bound is enforced structurally by
`$state.iteration` on a run-entity-anchored driver rule, not by
persona prose.

## How a sandbox gets created

Both autoresearch and dev-via-test run their `bash` in a real,
isolated **per-tenant devcontainer** — autoresearch-execute edits +
measures there, and Lisa / Ralph / CBG all share one. That
container doesn't appear by magic; the coordinator provisions it
*before* dispatching the pack, and the chain runs on the
**attestation** of what it got.

The sequence, for a build/optimize ask:

1. **Coordinator calls `request_sandbox` first.** The
   decision-contract tells the coordinator that `autoresearch` and
   `dev_via_test` both require a prepared environment, so it calls
   `request_sandbox` (a product-shell tool) *before* it emits
   `decide(action=…)`, and routes on the result.
2. **The sandbox manager picks a canonical profile and provisions.**
   `request_sandbox` hands a `SandboxRequirements` to the
   `sandboxmanager` (`cmd/semteams/sandboxmanager/`), which selects
   one of the **canonical profiles** checked into the repo
   (`.devcontainer/go-backend/`, `svelte-ui/`, `full-stack-e2e/`) —
   the LLM never renders a Dockerfile; it requests a profile. The
   manager's runner then provisions:
   - `SEMTEAMS_SANDBOX_RUNNER=api` (real): the `SandboxAPIRunner`
     POSTs to the sandbox sidecar container's `/exec`, which runs
     `devcontainer up` via `@devcontainers/cli` (Docker-out-of-
     Docker) against the chosen profile → a per-tenant devcontainer.
   - `SEMTEAMS_SANDBOX_RUNNER=mock` (default / Playwright journeys):
     the `MockRunner` returns a fabricated `Ready` so mock-LLM e2e
     runs need no Docker.
3. **The manager attests and stamps.** It verifies the container
   (image digest, probes) and stamps
   `sandbox.attestation.{ready, verified, profile, image_digest,
   requirements_hash, host_workspace_folder, signature, outcome,
   terminal, ttl_seconds, …}` on the chain entity. The coordinator
   reads `ready` / `terminal` and either dispatches the pack or, on
   a `terminal` failure, surfaces it to the user via
   `respond_direct`.
4. **Every chain agent's `bash` routes into that one container.**
   A chain-scoped wrapper sends each chain role's `bash` into the
   per-tenant devcontainer, so Lisa, every Ralph, and CBG (and the
   autoresearch loops) operate on the **same filesystem** — that's
   how Ralph's edits are visible to CBG's integration test at
   chain-end.
5. **Attestation-aware routing keeps paths consistent.** When an
   executor writes an artifact *into* the workspace (e.g. an
   autoresearch rollup), it writes through the host side; the
   reviewer reads it with `bash cat` from the container side. The
   `host_workspace_folder` in the attestation maps the two so both
   resolve to the same path (semteams#194).

So "the sandbox" a chain agent uses is just: a profile the
coordinator requested, provisioned by `devcontainer up`, attested
on the chain entity, and addressed by every chain role through the
same wrapper. Nothing in the chain invents an environment — it
inherits an attested one.

## Ops role (parallel observability track)

One ops role operates *off* the chain on a parallel track. It
reads the graph (chain agents do not) and emits diagnoses for
human review:

- **ops-chain-observer** — wakes once per run, when the run entity
  reaches `completed`, `failed`, or `cancelled`. Triggering on the
  run rather than on a reviewer role makes it category-agnostic
  (research, autoresearch, and future packs are covered without
  re-authoring) and lets it see failed and cancelled runs, which a
  reviewer-completion trigger structurally misses.

It carries no run association (`run_scope: "none"`), so it never
becomes a member of the run it observes.

**Known limitation.** The observer cannot enumerate a run's member
loops: membership points one way (loops record their run; the run
records no roster) and `query_relationships` reshapes an entity's
own triples rather than serving a reverse index. It works from the
run entity's own facts plus the coordinator loop reached via
`agent.run.handoff`, following only pointers it actually reads.
Widening this needs either an upstream reverse-index read or a
member-roster predicate on the run entity.

A second role, `ops-progress-observer`, was retired when the pack
was re-wired: its `fire_every_n_events` throttle does not gate
`publish_agent` at all (semstreams#1007), and a completion-triggered
rule structurally cannot observe a *stalled* chain — the case it
existed for. That needs a cron primitive with an idle-cost gate.

Diagnoses are stamped as `ops.diagnosis.*` triples and rendered in
the chain-entity UI for the operator. Phase 2
(ops-proposes-changes) is config-only: `create_rule`
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
loop.

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
recover or pause the chain.

### Chain pause

A loop failing with `outcome=failed` stamps `chain.paused` on the
chain entity. The UI surfaces this as `awaiting_approval` on the
parent task; the operator approves a resume, restarts a phase, or
cancels the chain.

## The graph is internal harness state

**Chain agents do not read the graph. The graph is internal harness
state — audit, lineage, milestone stamping, evidence aggregation.
Only ops agents read it.**

What each reader class has access to:

| Reader | Reads |
|---|---|
| Chain agents (research, autoresearch, reviewers) | `read_loop_result` on prior loop terminals · `bash` on `/artifacts/<kind>/<slug>.md` · `web_search` external · per-tenant devcontainer filesystem (autoresearch) |
| Ops agents (`ops-*`) | All graph-query tools (`query_entity`, `summarize_graph`, `read_loop_result`, etc.) — observing harness state IS their job |
| Operators (you, partners, auditors) | Rendered markdown in `/artifacts/`, persona docs, chain-entity UI, ops diagnoses |

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

- **Current capability inventory:** `configs/README.md` — the
  production bootstrap, live category packs, personas, and task
  runner entry points.
- **Demo claim boundary:** [`demo-mvp-claims.md`](demo-mvp-claims.md)
  — what the MVP can honestly claim, what is still a non-claim,
  and which journeys prove the boundary.
- **Product vocabulary:** [`product/vocabulary-map.md`](product/vocabulary-map.md)
  — public names for runs, specs, tasks, gates, artifacts, and
  slash commands.
- **Per-pack rule packs:** `configs/rules/research/README.md`,
  `configs/rules/autoresearch/README.md`,
  `configs/rules/dev-via-test/README.md` — every rule has a
  description block explaining the trigger conditions, actions,
  and the why (often including the upstream-framework workarounds
  being deployed). Worth reading for any pack work.
- **Per-role personas:**
  `configs/personas/fragments/<role>/` — identity, output
  contract, iteration rules per role. Read like job descriptions.
- **Product shell wiring:**
  `cmd/semteams/main.go` — how the binary boots the substrate on top of
  upstream semstreams primitives.
- **Ops agent:** `configs/rules/ops/` + the `ops-chain-observer`
  persona dir.
- **Sponsor packs:** `sponsor-packages/` — annotated trajectory
  walkthroughs of completed chains. First entry:
  `research-pack-fan-out-2026-05-29`.
