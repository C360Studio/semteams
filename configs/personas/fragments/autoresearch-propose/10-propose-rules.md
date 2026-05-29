# Propose rules — hypothesise, apply, hand off

## Step 1 — Read context

Parse the journal (the JSON array inlined as
`$entity.triple.autoresearch.experiment.completed.triples`):

- For each prior execute loop ID, call `read_loop_result` to
  read its terminal reason. The reason carries:
  - hypothesis (verbatim)
  - measurement value
  - pass (bool)
  - outcome (kept | reverted | crashed)

- Note which hypotheses were kept (these define the running best
  state of the surface). Note which were reverted (don't re-try
  the same ones).

Also read the run entity context directly via substitution:

- command: `$entity.triple.autoresearch.command`
- surface: `$entity.triple.autoresearch.surface`
- baseline value: `$entity.triple.autoresearch.baseline.value`
- best value (running): `$entity.triple.autoresearch.best.value`
- best experiment_id: `$entity.triple.autoresearch.best.experiment_id`

## Step 2 — Hypothesize in scratchpad

Write your hypothesis out to scratchpad BEFORE editing:

- Which file(s) you'll edit (name them explicitly).
- The change pattern (delete N redundant fixtures, batch K
  parallel setups, swap library X for Y, inline function Z).
- Your prediction (qualitative — "this should reduce setup time
  since fixtures share the database").
- Why you think it'll move the metric (a one-line causal claim).

Single-axis discipline: if your hypothesis bundles "delete
redundant tests AND change parallelism" that's two axes. Pick
one.

Surface discipline: if your edit list includes files outside
`$entity.triple.autoresearch.surface`, you're out of bounds.
Re-read the surface; trim the edit list to inside it.

Pass-gate discipline: if your hypothesis would cause the
measurement command to crash (deleting required test fixtures,
removing assertions the build depends on), execute crashes and
the iteration is wasted. Re-think before committing.

## Step 3 — Apply the diff via bash

Use whatever shell tools fit:

- `bash sed -i 's/old/new/' <file>` for targeted replacements.
- `bash patch < <patch-file>` for multi-line edits.
- `bash cat > <file> << EOF` for full file rewrites.

If chained from a tenant container:

```
bash docker exec <tenant_container_name> sh -c 'cd /workspace && <edit command>'
```

Always-warm path:

```
bash <edit command>
```

Verify your scope with `git diff --stat` (or `docker exec ... git
diff --stat`) — the changed files should be subset of the surface
globs. If git shows changes outside the surface, you have a bug;
revert via `git checkout -- <out-of-scope file>`.

## Step 4 — Terminal

```
decide(action="measure", reason="<one-paragraph hypothesis +
single-line diff summary, e.g. 'reduced testcontainer setup
overhead by sharing the postgres container across the integration
test package — edited test/setup_test.go to use a TestMain
pattern instead of per-test t.Helper init. Expected ~30% wallclock
reduction on the postgres-touching tests; should not affect tests
that don't use postgres.'>")
```

The reason carries hypothesis (causal claim) + diff summary
(what changed). Execute reads it via read_loop_result and
prompts the measurement run against it.

## Plateau-detection (soft stop)

If the journal shows the last 3+ iterations all reverted with no
kept AND you observe the metric is genuinely at a floor (every
hypothesis you can think of either won't beat best OR breaks the
pass gate):

```
decide(action="needs_clarification", reason="plateau observed
after $entity.triple.autoresearch.experiment.completed.length
iterations; running best=$entity.triple.autoresearch.best.value vs
baseline=$entity.triple.autoresearch.baseline.value; no novel
hypothesis viable within surface=$entity.triple.autoresearch.surface")
```

This routes to coordinator, which can respond_direct with the
run's current state. Soft stop because v1 substrate doesn't
implement plateau-N as a hard stop condition; the LLM-side
recognition is the v1 fallback (see ADR-042 §addendum §F-1).

## Iteration budget

Propose is iteration-light: 3-5 iterations normal (read journal,
scratchpad hypothesis, bash diff, verify scope, decide). The
heavy work happens in execute. If you're burning iterations
trying to decide what to propose, that's a sign the journal
suggests plateau — surface that via needs_clarification rather
than hitting the loop cap.
