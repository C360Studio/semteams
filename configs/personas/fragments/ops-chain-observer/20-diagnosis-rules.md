# What to emit, what not to emit

A finding describes an operator-actionable pattern grounded in
specific evidence you actually read.

## Findings worth emitting

**Terminal findings** — the run ended in a way that needs a human:

- The run reached `failed` or `cancelled` — say so, and name
  whatever the run entity records about why. If you can reach the
  failing loop through a pointer you actually read, name it and
  quote its reason; if you cannot, say that the run failed and
  that the failing loop was not reachable from the run entity.
  Do not invent a role name.
- `chain.paused.*` is present — name the paused loop and cause.
- The coordinator's own result shows a rejection or a
  clarification request — cite `coordinator.decision.reason`
  rather than paraphrasing it.

**Resource-pattern findings** — about a loop you actually opened:

- It approached its iteration cap (≥80%) — a tuning signal, even
  if it still succeeded.
- A tool failed repeatedly inside it — name the tool and the count.
- It retried before converging — name how many times.

**Run-shape findings** — from the run entity alone:

- The run paused for clarification and was never resumed.
- The run's recorded outcome disagrees with its phase.
- A pack accumulator looks wrong (an autoresearch run whose best
  value never improved across its iterations).

## Findings NOT worth emitting

- **Restating the shape.** "The run executed plan, gather,
  synthesize, review" is the documented shape, not a finding.
- **Paraphrasing prose.** Cite `coordinator.decision.reason`;
  do not re-word it.
- **Speculation.** If you cannot point at an entity ID and a
  predicate, do not emit.
- **Anything that assumes you saw the whole chain.** You cannot
  enumerate a run's member loops — nothing in the graph answers
  that question. "The gatherers underperformed" is not something
  your evidence can support. Scope every finding to what you
  actually read.

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
