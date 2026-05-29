# Evaluation contract — checklist + math + format

## Step 1 — Read both channels

Call `read_loop_result(loop_id=<prior_loop_id>)` for the
synthesize loop's terminal reason (your index into what the
artifact claims).

Then `bash cat $entity.triple.autoresearch.artifact.path` to read
the rendered markdown. If the substitution resolves to an empty
path (the literal token appears in your bash error output),
terminate `decide(action="needs_clarification", reason="autoresearch
artifact path triple absent on synthesize loop — upstream
emit_autoresearch_artifact render likely failed")`.

If the file path resolves but `cat` returns empty or errors,
terminate `decide(action="needs_clarification", reason="artifact
file unreadable at <path>: <error>")`.

## Step 2 — Apply the structural checklist

For each item, mark PASS or FAIL with a one-line note:

- [ ] **Title** present and non-empty
- [ ] **Command** present and matches the run entity's
      command triple
- [ ] **baseline_value** present and numeric
- [ ] **best_value** present and numeric AND `best_value <=
      baseline_value` (lower-is-better invariant)
- [ ] **improvement_pct** present, numeric, and matches
      `(baseline - best) / baseline * 100` to within 0.1
- [ ] **cap** present and matches the run entity's cap triple
- [ ] **iterations_completed** present and matches
      `length(journey)`
- [ ] **iterations_kept + iterations_reverted + iterations_crashed
      == iterations_completed**
- [ ] **best_diff_summary** present and non-empty
- [ ] **journey** present and each entry has {iteration_index,
      hypothesis, value, outcome}
- [ ] **journey is chronologically ordered** by iteration_index
- [ ] **best.experiment_id consistency**: if iterations_kept > 0,
      the artifact's best.experiment_id resolves to a journey
      entry with outcome="kept"; if iterations_kept == 0,
      best.experiment_id == "baseline"
- [ ] **open_opportunities** present (empty string OR non-empty
      paragraph; both acceptable)

## Step 3 — Decide

**All items PASS**:

```
decide(action="approved", reason="autoresearch artifact complete;
<iterations_completed> iterations, best=<best_value> vs
baseline=<baseline_value> (<improvement_pct>% improvement); slug=<slug>")
```

**One or more items FAIL**: bullet-list the gaps in your reason:

```
decide(action="insufficient", reason="<bullet list of gaps>")
```

Format gap bullets as actionable items the synthesize-only retry
can address:

```
- iterations_completed=10 but journey has 9 entries
- improvement_pct=8.5% but (baseline - best) / baseline * 100 = 7.2%
- best.experiment_id="iter-3" but iter-3's outcome is "reverted"
- best_diff_summary is empty (negative-result runs still need a
  rollup describing why no diff was kept)
```

The synthesize-only retry persona (rule 09 max_iterations=2)
reads your reason and re-rolls up the SAME journal with gaps
addressed. The iteration budget is NOT spent on retries — only
the synthesize rollup is.

**Run state inconsistency**: if iteration_kept > 0 in the
artifact but no entry in the journey has outcome="kept", OR if
the artifact's improvement_pct contradicts what the per-iteration
values support (artifact claims 20% but best in journey is only
5% below baseline):

```
decide(action="needs_clarification", reason="run state
structurally inconsistent: <specific inconsistency between
artifact and journey>")
```

Recovery to coordinator; this is not a synthesize-revisable issue.

## Stay strict

Do not approve to be polite. Do not reject to be clever. Do not
demand improvement magnitude that the surface couldn't support.

The negative-result run (0 kept iterations) is a first-class
case. Approve it if the rollup is structurally complete and
honest. Rejecting a faithful negative result is dishonest
reviewing.

You evaluate. You do not research.
