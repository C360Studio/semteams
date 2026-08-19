# What to emit, what not to emit

A finding describes an operator-actionable pattern grounded in
specific evidence you actually read.

## Findings worth emitting

**Terminal findings** — the run ended in a way that needs a human:

- The run reached `failed` or `cancelled` — name the loop whose
  outcome was `failed`/`truncated` and what its reason said.
- `chain.paused.*` is present — name the paused loop's role and
  the recorded cause.
- A reviewer rejected, or a role terminated
  `needs_clarification` — cite the specific gap from
  `coordinator.decision.reason` rather than paraphrasing it.

**Resource-pattern findings** — the run succeeded but something is
worth tuning:

- A role approached its iteration cap (≥80%) — a tuning signal,
  especially if it still succeeded.
- One role dominated total token spend — a cost-attribution
  signal.
- A phase retried repeatedly before converging — name how many
  times and what changed between attempts.
- A tool failed repeatedly inside one loop — name the tool, the
  count, and the loop.

**Inconsistency findings** — the run's own facts disagree:

- A role reported success but the next role's reason contradicts
  it — cite both.
- A terminal was reached with output thinner than the phase
  implies (a synthesize step with almost nothing in it, a gather
  step that cited no sources).
- The run's phase and outcome disagree with the roster you found
  in step 2.

## Findings NOT worth emitting

- **Restating the shape.** "The run executed plan, gather,
  synthesize, review" is the documented shape, not a finding.
- **Paraphrasing prose.** Cite `coordinator.decision.reason`;
  do not re-word it.
- **Speculation.** If you cannot point at an entity ID and a
  predicate, do not emit.
- **Expected outputs restated as observations.** "The gatherer
  produced 4 findings" is data. "The planner named 6 subtopics
  and only 4 gatherers reported" is a finding.

## emit_diagnosis contract

One call, one finding. Required fields:

- `finding` — what is wrong or worth attention (one sentence)
- `recommendation` — what an operator should do (one sentence)
- `confidence` — 0.0 to 1.0
- `evidence` — at least one graph entity ID you actually read;
  the run entity qualifies, plus any loop entities your finding
  cites
- `severity` — **exactly one of `info`, `warn`, `critical`**

The tool mints `{org}.{platform}.ops.diagnosis.finding.{uuid}`
entities. Those are the audit trail an operator reads.

### Severity is a closed set

`info`, `warn`, `critical` — nothing else. **Any other value is
silently clamped to `info`**, so writing `error` or `high` does
not fail loudly, it quietly downgrades your most serious finding
to the least serious level. Use the three words exactly.

- `info` — a noteworthy data point implying no action
- `warn` — a pattern an operator should investigate or tune
- `critical` — a structural gap or regression needing a fix

A failed or paused run is normally `critical`. A tuning signal on
a healthy run is `warn`. Use `info` sparingly — a finding nobody
needs to act on is usually a finding not worth emitting.

## Confidence calibration

- **0.9+** — structural: a triple says it and you cite the triple
- **0.7-0.9** — pattern evidence across several loops
- **0.5-0.7** — indirect, requiring inference
- **Below 0.5** — do not emit

Confidence is also how downstream consumers rank urgency, since
severity is only three levels. An honest 0.6 is more useful than
a reflexive 0.9.
