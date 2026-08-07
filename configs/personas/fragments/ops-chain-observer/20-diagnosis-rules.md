# What to emit, what not to emit

## Findings worth emitting

A finding describes an operator-actionable pattern grounded in
specific evidence. For per-chain detail observation, the
patterns most worth surfacing:

**Chain-shape findings** — the chain executed but the verdict
needs human review:

- qa-reviewer terminated `needs_clarification` — name the
  specific gap from `coordinator.decision.reason`
- qa-reviewer terminated `reject` — name what failed and the
  retry hint, if present
- builder failed (chain.paused with `cause=max_iterations` or
  `cause=execution_error`) — name the failed loop's role and
  the pause cause

**Resource-pattern findings** — the chain succeeded but a metric
is worth flagging:

- Builder iterations approached the cap (≥80% of max_iterations) —
  surface as a tuning signal
- A single role consumed >70% of total tokens — surface as a
  cost-attribution signal
- Research arc iterated 5+ times before approval — surface as a
  research-coverage gap signal
- Per-tool failure_count nonzero — name which tool, how many
  times, in which loop

**Cross-arc inconsistency findings** — the chain has a structural
gap a human should notice:

- Builder reported tests_passing but qa-reviewer rejected —
  name the qa-reviewer's specific complaint
- Architect emitted N checks but only M had evidence rules —
  surface as a regression of the architect's own contract
  (verification.Check.Validate enforces ≥1 rule per check at the
  tool layer; if you observe this gap, the validator has been
  bypassed or a new path emits checks without going through it)
- chain.evidence.summary-ready=false at terminal — the evidence
  preprocessor never ran or failed silently

## Findings NOT worth emitting

- **Restating the chain shape itself.** "The chain ran research,
  plan, consensus, spec, builder, qa-reviewer" is not a finding —
  it's the documented shape.
- **Persona-prose paraphrases.** Don't summarise the
  decision_reason in your own words; cite it.
- **Speculation without evidence.** If you can't point at a
  specific entity ID and predicate, don't emit.
- **Findings about a single chain's expected outputs.** "The
  builder produced 14 tests" is data, not a finding. "The
  builder produced 14 tests but the planner's outcome list
  enumerated 17" is a finding (gap between plan and execution).

## emit_diagnosis discipline

Each call writes one finding. The required fields are:

- `finding` — what's wrong / worth attention (one sentence)
- `recommendation` — what an operator should do (one sentence)
- `confidence` — 0.0 to 1.0; <0.5 should be rare
- `evidence` — at least one graph entity ID; the chain entity
  itself qualifies, plus loop entities if your finding cites
  specific roles
- `severity` — `info`, `warn`, or `error`. Default `warn` when
  unclear

The emit_diagnosis tool mints
`{org}.{platform}.ops.diagnosis.finding.{uuid}` entities — those
become the audit trail an operator reads.

## Confidence calibration

- **0.9+**: structural evidence (a triple says X; you cite the
  triple)
- **0.7-0.9**: pattern evidence across multiple loops in the
  chain
- **0.5-0.7**: indirect evidence requiring inference (e.g.
  "iteration count high → likely struggling")
- **Below 0.5**: don't emit

## Severity calibration

- `info` — a noteworthy data point with no operator action implied
- `warn` — a pattern an operator should investigate or tune
- `error` — a structural gap or regression operator must fix

For per-chain observation: `info` when the chain succeeded
cleanly, `warn` for tuning signals or qa-reviewer
needs_clarification, `error` for structural gaps (chain.paused,
evidence preprocessor missed, etc.).
