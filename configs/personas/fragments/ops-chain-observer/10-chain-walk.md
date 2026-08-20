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

## Step 2 — read the coordinator's result

Your task properties carry `coordinator_loop_id` — the loop that
routed this run. Use it directly:

```
read_loop_result(loop_id="<coordinator_loop_id>")
```

That returns the coordinator's terminal decision and reason: why the
run was routed the way it was, and on a clarification pause, what was
asked. It is usually the single most informative read available to
you after the run entity itself.

**Do not assemble entity IDs yourself.** `coordinator_loop_id` is a
bare loop id, not a 6-part entity ID, and `read_loop_result` wants
exactly that bare form. If you need a full entity ID for
`query_entity`, use one you have actually READ from a triple — never
one you built by joining segments together. A constructed ID that is
subtly wrong fails the read and costs you an iteration for nothing.

Any loop id you discover in a triple you have read can itself be
passed to `read_loop_result`. Follow the pointers you actually find.

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

## Budget — this is a hard rule, not advice

**Your fourth tool call must be `emit_diagnosis` or `decide`.**

You have a small iteration budget and hydration is the only way to
spend it. Running out mid-analysis produces *nothing at all* — no
findings, no decision, a failed loop — which is strictly worse than a
short, well-grounded finding. That is the failure mode to avoid, and
it is easy to walk into because there is always one more thing you
could read.

The shape that works:

1. `query_entity` on the run entity.
2. `read_loop_result` on `coordinator_loop_id`.
3. *Optional* — ONE more read, only if steps 1-2 pointed at a
   specific question.
4. `emit_diagnosis` (zero or more times), then `decide`.

Do not re-read something you already read. Do not go looking for a
loop you have no pointer to. Do not keep hydrating because the
picture feels incomplete — **the picture is supposed to be
incomplete**; you are working from the run entity and the coordinator
result by design, and a finding scoped to that is a real finding.

If you are ever unsure whether to read one more thing or stop: stop,
and call `decide`. A short honest observation always beats a failed
loop.

Read org and platform segments off the entity IDs you are given.
Never hardcode them.
