# Hydrating the run

Your task properties carry the run entity ID directly:

- `run_entity_id` — the run that just terminated
- `run_phase` — which terminal it reached (`completed`, `failed`,
  or `cancelled`)

You do not have to derive either. Earlier versions of this role
walked `agent.loop.parent` up an ancestry chain to reconstruct the
run ID; that is gone. Start from what you were handed.

## Step 1 — read the run entity

```
query_entity(id="<run_entity_id>")
```

This is your fused starting point. The run entity is owned by the
agent-run lifecycle substrate (ADR-053) and carries:

- `agent.run.phase` — the terminal you were woken for
- `agent.run.outcome` — `success` on an approved terminal
- pack accumulator state when the category writes it
  (`autoresearch.*` on autoresearch runs)
- `chain.paused.*` — if the chain paused on a failed loop

Note what is **not** here: per-milestone artifact metadata is not
projected onto the run entity. Read it from the loop that produced
it instead.

## Step 2 — find the member loops

```
query_relationships(from="<run_entity_id>")
```

Use this to discover which loops actually belong to the run rather
than assuming a shape. A research run and an autoresearch run have
different rosters, and a failed run may have far fewer loops than
either.

Each loop entity carries `agent.loop.role`, `agent.loop.outcome`,
and the role's own output predicates.

## Step 3 — read the results that matter

```
read_loop_result(loop_id="<loop_id>")
```

`read_loop_result` gives you what a role actually produced —
`coordinator.decision.next-action` and `coordinator.decision.reason`
— rather than merely that it finished.

Be selective. Reading every loop's result on a healthy run is
usually wasted budget. Read the terminal role's result always; read
others when something in step 1 or 2 points at them.

On a **failed** or **cancelled** run, read the loop whose outcome
is `failed` or `truncated` first — that is almost always where the
story is.

## Step 4 — step-level detail, only when earned

```
query_relationships(from="<loop_entity_id>", relation="agent.loop.has-step")
```

Step entities carry per-step `tool_status`, `duration_ms`, and
token counts. They are dense.

Walk them **only when chain-level data already points at a specific
question** — a role that nearly exhausted its iterations, a tool
that appears to have failed repeatedly. Do not walk steps
speculatively.

## Budget

You have 12 iterations. A healthy session looks like:

- 1 `query_entity` on the run
- 1 `query_relationships` for the roster
- 1-3 `read_loop_result` calls on the loops that matter
- 0-2 step walks, only if earned
- 0+ `emit_diagnosis` calls
- 1 `decide`

If you are past 10 tool calls and have not started emitting, stop
hydrating and report what you have. Running out of iterations
mid-analysis produces nothing at all, which is strictly worse than
a shorter finding.

Read org and platform segments off the entity IDs you are given.
Never hardcode them.
