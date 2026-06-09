# Walking the chain

Your task properties carry one or more loop IDs in
`agent.related_loops`:

- `qa_reviewer` — the loop that just reached terminal (your trigger)
- `researcher` — a completed-ancestor loop in the chain (always
  set by every dev-via-spec spawn rule; the chain entity's
  canonical anchor)

These are hydration handles. Use them to walk the chain.

## Step 1 — find the chain entity by walking parent ancestry

The chain entity ID is `c360.<platform>.agent.chain.execution.<root_loop_id>`,
but you don't know `root_loop_id` directly. Walk
`agent.loop.parent` from the `researcher` loop_id (in your task
properties) up to the root, then construct the chain entity ID
from the root loop_id and the platform/org segments.

The walk:

```
query_entity(id="c360.<platform>.agent.agentic-loop.execution.<researcher_loop_id>")
```

…returns the researcher's loop entity triples. Inspect the
`agent.loop.parent` predicate value — it's the parent loop's
full 6-part entity ID, or absent if the researcher IS the root
(rare in dev-via-spec; common in pure research arcs).

If `agent.loop.parent` is set, recurse: take the parent entity
ID's last segment as the parent loop_id, query that, repeat
until `agent.loop.parent` is absent. The terminal loop_id is
the chain root. Construct the chain entity ID by replacing
`agentic-loop` → `chain` and `execution.<loop_id>` →
`execution.<root_loop_id>` in the segment positions.

In dev-via-spec chains this walk is typically 1-3 hops from the
researcher (researcher → research-reviewer → dispatch root, or
researcher itself IS spawned from dispatch). Budget 3-5 query
calls for the walk.

The org/platform segments come from the entity IDs you read
during the walk — every loop entity carries the same prefix.
Don't hardcode platform values; read them.

## Step 2 — read the chain entity's full triple set

```
query_entity(id="<chain_entity_id_from_step_1>")
```

This is your fused starting point. The run entity
(`agent.chain.execution.<run_id>`) is owned by the agent-run lifecycle
substrate (ADR-053) and carries:

- `agent.run.phase` — the run's lifecycle phase (`dispatched` →
  `executing` → `completed` / `failed` / `cancelled`)
- `agent.run.outcome` — `success` on a reviewer/CBG-approved terminal
- pack accumulator state when present (`autoresearch.*`, `dev_via_test.*`)
- `chain.paused.*` — if the chain paused (failed-loop ancestry)

Per-milestone artifact metadata is NO LONGER projected onto the run
entity (the hand-rolled chain milestone stampers were retired in ADR-053).
Read it from the producing LOOP entities instead, reached via lineage:
the researcher/synthesize loop carries `research.artifact.{path,
test_harness, actors_count, tasks_count}`; reach it via `lineage.researcher`
on the reviewer loop (related_loops), not a `chain.research_artifact.*`
triple on the run entity.

## Step 3 — read each milestone's loop_result

Read the qa_reviewer's loop_result first (that's your trigger
context):

```
read_loop_result(loop_id="<qa_reviewer_from_related_loops>")
```

This gives you the qa-reviewer's verdict: `coordinator.next_action`
(accept | reject | needs_clarification) and `coordinator.decision_reason`.

Then read each milestone loop's result:

- `read_loop_result(loop_id=<lineage.researcher on the reviewer loop>)` — researcher's terminal
- `read_loop_result(loop_id=<chain.plan_loop>)` — planner's terminal (legacy; ADR-041 MVP collapses planner into researcher-plan)
- `read_loop_result(loop_id=<chain.plan_reviewer_loop>)` — reviewer's terminal (legacy; same reason)
- `read_loop_result(loop_id=<chain.spec_artifact_loop>)` — researcher-architect's terminal

You now have:

- The chain shape (which arcs ran, which milestones landed)
- Each role's terminal verdict + reason
- The qa-reviewer's final verdict + reason

## Step 4 — sample step triples for resource patterns

If a finding warrants it (slow loop, high token burn, repeated
tool failures), query the loop's step entities:

```
query_relationships(from="<loop_entity_id>", relation="agent.loop.has_step")
```

This returns the step entity IDs. Read them with
`query_entity` to inspect tool_status, duration_ms, tokens_in/out
per step.

**Don't always do step-level walking.** Step triples are dense.
Walk them only when the chain-level data points at a specific
pattern (e.g. builder iteration count is high → walk builder's
steps to see which tool calls dominated). Otherwise stay at the
chain + loop level.

## Hydration discipline

Each query is a hydration cost (tokens for the response). The
ideal session shape is:

- 1-3 query_entity calls for the parent walk to find chain root
- 1 query_entity for the chain entity triples
- 5-7 read_loop_result calls (one for qa_reviewer + one per
  `chain.*_loop` milestone predicate)
- 0-2 step-level walks if a specific signal warrants it
- 1+ `emit_diagnosis` calls
- 1 `submit_work` call

Roughly 10-15 tool calls total per session. If you find yourself
exceeding 30, stop and re-evaluate — you're probably walking
something that doesn't pay rent.
