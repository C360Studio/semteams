# Synthesis rules — read the journal, roll up, emit

## Step 1 — Read the run state

Substitutions from the run entity:

- command: `$entity.triple.autoresearch.run.command`
- surface: `$entity.triple.autoresearch.run.surface`
- baseline.value: `$entity.triple.autoresearch.baseline.value`
- best.value: `$entity.triple.autoresearch.best.value`
- best.experiment_id: `$entity.triple.autoresearch.best.experiment-id`
- cap: `$entity.triple.autoresearch.run.cap`
- stop.reason: `$entity.triple.autoresearch.stop.reason`
  (cap | plateau | budget — v1 only stamps "cap")

## Step 2 — Read the per-iteration journal

The completed-iteration loop IDs are inlined as
`$entity.triple.autoresearch.experiment.completed.triples` (JSON
array). Parse the array; for each loop ID, call read_loop_result
to read that iteration's terminal reason.

Each iteration's reason carries:

- The propose hypothesis (verbatim)
- The measurement value (numeric)
- The pass bool
- The outcome (kept | reverted | crashed)

Also check `$entity.triple.autoresearch.experiment.loop-failed.triples`
for any execute loops that crashed mid-iteration (rule 04b
stamps this). Those iterations consumed budget but produced no
measurement; the journey should mark them "loop-failed (no
measurement data)."

Order the journey by iteration number (the order of the
experiment.completed array IS the chronological order).

## Step 3 — Read the best experiment's diff (if non-baseline)

If `best.experiment_id != "baseline"`:

```
bash git diff <baseline-tag>..HEAD -- <surface>
```

This gives you the cumulative kept diff (the running best is the
sum of all kept iterations). Summarise in scratchpad before
emitting the artifact.

If no baseline-tag exists OR best.experiment_id == "baseline"
(0 kept iterations), skip this step.

## Step 4 — Compose the artifact in scratchpad

Decide on:

- **title**: short descriptive (e.g. "task test:integration
  wallclock reduction", "ESLint warning-count reduction")
- **improvement_pct**: `(baseline - best) / baseline * 100`,
  rounded to one decimal place. 0% for negative-result runs.
- **iterations_completed**: length of the journal
- **iterations_kept / reverted / crashed**: counts per outcome
- **best_diff_summary**: one paragraph describing the cumulative
  kept changes (or "no kept iterations; tree at baseline" for
  negative results)
- **journey**: ordered list of iteration entries (iteration index,
  hypothesis verbatim, value, outcome)
- **open_opportunities**: one paragraph on what the journal
  suggests has headroom but this run did not explore. Honest
  observation, not a plan. Empty string is fine if the run
  exhaustively covered the surface.

## Step 5 — Emit the artifact

```
emit_autoresearch_artifact(
  title="<from step 4>",
  command="<from run entity>",
  baseline_value=<from run entity>,
  best_value=<from run entity>,
  improvement_pct=<computed>,
  cap=<from run entity>,
  iterations_completed=<count>,
  iterations_kept=<count>,
  iterations_reverted=<count>,
  iterations_crashed=<count>,
  best_experiment_id="<from run entity — autoresearch.best.experiment-id; use literal 'baseline' when iterations_kept == 0>",
  best_diff_summary="<from step 4>",
  journey=[<ordered list of {iteration, hypothesis, value, outcome}>],
  open_opportunities="<from step 4>"
)
```

The tool renders markdown at /artifacts/autoresearch/<slug>.md
and stamps `autoresearch.artifact.{title, path, revision, cap,
best_experiment_id, …}` on your loop entity.

## Step 6 — Terminal — fire decide IMMEDIATELY after emit

Your next action is `decide(action="emit", ...)`. Nothing else.

```
decide(action="emit", reason="autoresearch artifact rev <N>:
baseline=<n> best=<n> (<pct>% improvement); <count> iterations
(<kept> kept, <reverted> reverted, <crashed> crashed); slug=<slug>")
```

The moment `emit_autoresearch_artifact` returns success, the
artifact is on disk and the triples are stamped. Do not call
bash, scratchpad, read_loop_result, or re-emit; the tool returns
non-zero on render failure, and the reviewer is the verifier.
Your work is done — fire decide.

Rule 07 spawns reviewer-autoresearch next.

If reviewer rejects (action=insufficient), rule 09 re-spawns you
with a tightened reason citing structural gaps. Address the
gaps named in the rejection — NOT re-litigation of the underlying
run. The journal is immutable; only the rollup composition is
revisable.

## When to needs_clarification

Only for structural inconsistency in the run state:

- `experiment.completed.length` doesn't match the length of your
  parsed journey.
- `best.experiment_id` points to a loop ID not in the journey.
- `experiment.loop_failed.length > experiment.completed.length`
  (impossible — loop_failed is a subset).

Otherwise emit. Even degenerate cases (1 iteration, 0 kept,
plateau immediate) are emittable as honest negative-result
artifacts.

## Iteration budget

Synthesize is journal-heavy: parse N entries, read N loop
results, summarise, emit. For cap=10 runs, expect 12-18
iterations (one per journal entry + composition + emit + decide).
Stay focused on the rollup; don't re-litigate iteration decisions
that already happened.
