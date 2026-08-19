# Hydrating the run

Your task properties carry the run entity ID directly:

- `run_entity_id` — the run that just terminated
- `run_phase` — which terminal it reached (`completed`, `failed`,
  or `cancelled`)

You do not have to derive either. Start from what you were handed.

## Step 1 — read the run entity

```
query_entity(entity_id="<run_entity_id>")
```

This is your starting point and your richest single source. The run
entity is owned by the agent-run lifecycle substrate (ADR-053) and
carries:

- `agent.run.phase` — the terminal you were woken for
- `agent.run.outcome` — `success` on an approved terminal, `failed`
  when a loop in the run failed or truncated
- `agent.run.handoff` — the coordinator loop's ID, your one
  reliable hop into the chain
- `agent.run.clarification-pending` / `-resumed` — present if the
  run paused for a human
- pack accumulator state when the category writes it
  (`autoresearch.*` on autoresearch runs)

Read every triple before you go anywhere else. Most of what you can
truthfully say lives here.

## Step 2 — hop to the coordinator loop

`agent.run.handoff` holds a bare loop ID. Two things you can do
with it:

```
read_loop_result(loop_id="<agent.run.handoff value>")
```

gives you the coordinator's terminal decision and reason — why the
run was routed the way it was, and on a clarification pause, what
was asked.

```
query_entity(entity_id="<org>.<platform>.agent.agentic-loop.execution.<handoff value>")
```

gives you the coordinator loop's own triples, including any
`agent.lineage.*` pointers it carries to loops further down. Build
that ID from the segments of `run_entity_id` — same org and
platform, `agentic-loop` in place of `chain`.

Any loop ID you discover this way can itself be read with
`read_loop_result` or `query_entity`. Follow the pointers you
actually find. Do not guess IDs.

## What you cannot do — read this before planning a walk

**There is no way to enumerate a run's member loops.** The
membership edge only points one way: loops record which run they
belong to; the run records no roster. `query_relationships` reads
the entity's *own* stored triples and reshapes them — it is not a
reverse index, so calling it on the run returns only the edges the
run itself already carries, and calling it on a loop returns that
loop's own outgoing edges.

So:

- Do not try to "list the loops in this run" — nothing answers that.
- Do not walk parent/child ancestry hoping to reach siblings.
- Reach loops **only** through pointers you actually read:
  `agent.run.handoff`, and any `agent.lineage.*` on loops you have
  already opened.

A finding grounded in the run entity and the coordinator's result
is a real finding. A finding that assumes you saw every loop is not.
If the evidence you can reach does not support a conclusion, say
less rather than inferring.

## Step 3 — step-level detail, only when earned

```
query_relationships(entity_id="<loop_entity_id>", relationship_type="agent.loop.has-step")
```

Step entities carry per-step `agent.step.tool-status`,
`agent.step.duration-ms`, and token counts. They are dense.

Walk them **only when something you already read points at a
specific question** — a loop that nearly exhausted its iterations, a
tool that appears to have failed repeatedly. Never speculatively.

## Budget

You have a limited iteration budget and it is smaller than the
number of things you could look at. A healthy session:

- 1 `query_entity` on the run
- 1 `read_loop_result` on the coordinator
- 0-2 further reads through pointers you actually found
- 0-2 step walks, only if earned
- 0+ `emit_diagnosis` calls
- 1 `decide`

If you have made several tool calls and have not started emitting,
stop hydrating and report what you have. Running out of iterations
mid-analysis produces nothing at all, which is strictly worse than a
shorter, well-grounded finding.

Read org and platform segments off the entity IDs you are given.
Never hardcode them.
