# Objective grounding — mandatory first step

Before any analysis, load the objective spec for the flow under
review. It lives at `docs/objectives/<flow_name>.md` in this
repository. If it does not exist, your analysis is complete: call
`emit_diagnosis` once with `finding="missing objective spec for
<flow_name>"`, `recommendation="author docs/objectives/<flow_name>.md
before further ops analysis"`, `confidence=0.9`, `evidence=[<flow
entity id>]`, then call `submit_work`. Do not infer an objective from
the telemetry.

The objective spec contains five sections you must consult
explicitly:

1. **Primary metric** — compute this value over the observation
   window. Cite the predicate names the spec references in your
   finding's `evidence` array.

2. **Secondary metrics** — compute each over the same window. If a
   proposed finding would recommend a change that regresses any
   secondary beyond its stated regression budget, suppress the
   finding — it violates the Pareto bound the spec set.

3. **Immutable guardrails** — before any `emit_diagnosis` call,
   check whether following the recommendation would touch a
   guardrail. If yes, suppress. This is not negotiable. A finding
   that implies a guardrail violation is a bug in your reasoning;
   do not rationalize it.

4. **Failure-mode predictions** — the spec pre-registers the likely
   reward-hacking moves. Apply each as an explicit reject heuristic
   against your draft findings. Each failure mode lists a signal
   pattern; if your draft finding's evidence matches the pattern,
   reject the finding.

5. **Evaluation window** — your task payload's `observation_window`
   should match the spec's window size. If it does not, call
   `emit_diagnosis` once with `finding="observation window mismatch:
   payload=<N> spec=<M>"`, `confidence=0.9`, evidence citing the
   window count, then `submit_work`.

## Citing the spec in evidence

Every `emit_diagnosis` call must include the objective spec path in
the `recommendation` text and at least one supporting graph entity
ID in the `evidence` array. A finding that cannot be tied to
specific graph entities is not grounded — emit nothing.

The `evidence` array contains **entity IDs only** (e.g.
`c360.semteams-research.agent.loop.completed.<uuid>`), not free
text. Use `query_entities` to find candidate evidence entities and
`query_relationships` to verify their predicate values support the
finding.
