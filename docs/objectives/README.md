# Flow Objective Specs

Objective specs are the contract the ops agent (ADR-027 Phase 1) reads
before analyzing a flow. Each stock flow in semteams gets one markdown
file in this directory. Without an objective spec, any diagnosis the
ops agent produces is ungrounded — "just guessing" — and should be
treated as such.

## Why this exists

Stanford Meta-Harness (Lee et al., arXiv:2603.28052) and the
autoresearch lineage (Karpathy, Shopify, Autoimprove) converge on one
observation: the evaluation harness is harder than the optimization
loop. Autoimprove's author put it bluntly — "Building the golden set
and evaluation script took longer than running all 14 experiments."

Semteams inherits that bottleneck. The ops agent can observe ~113
graph predicates the semstreams runtime emits, but predicates are not
self-interpreting. What does "good" look like for a deep-research
flow? What does "good" look like for code-review? Different flows,
different answers. The objective spec is where that answer lives in
writing, authored once by a human, cited by the ops agent on every
diagnosis.

## When to write a new spec

When a flow is stable enough that you would accept a human operator
making tuning calls against it — and want an agent to do the same
analysis. New flows should ship without an objective spec and without
ops-agent coverage; promotion to spec'd status is a deliberate step,
not a default.

## Structure

Every spec follows the same five-section shape. Keep each section
concrete and predicate-backed; prose without predicates is unactionable.

### 1. Primary metric

The single scalar the ops agent is trying to move. State:

- **Name** — a human-readable label used in findings.
- **Formula** — how it is computed from graph predicates. Reference
  predicates by their exact names so the ops agent can query them.
- **Direction** — up or down.
- **Baseline** — the current value (or "not yet measured" with a
  target sample size to establish one).

A single scalar is preferred. If a flow genuinely needs a composite,
state the weights explicitly — `0.7 * accuracy + 0.3 * cost_efficiency`
is answerable; "balance quality and cost" is not.

### 2. Secondary metrics (Pareto axes)

Cost, latency, and any flow-specific signals that bound the primary.
Same shape as above — predicate, formula, direction. These prevent
the ops agent from trading primary gain for unbounded regression on
a secondary. Call out the allowable regression budget per secondary
(e.g., "p95 latency may rise by up to 15% if primary improves by
5 points").

### 3. Immutable guardrails

Configuration the ops agent must never propose changing. Examples:

- Approval filter on write tools (`manage_flow`, `create_rule`,
  `bash`).
- Governance filter enabled (`agentic-governance.enable_tool_governance`).
- Tool category rules that enforce role-based access.
- Model floor (e.g., "never downgrade researcher below Haiku").
- Safety prompt fragments that must remain in the assembled prompt.

Guardrails are the anti-reward-hack layer. Whatever dimension of the
system would degrade quality if weakened, name it here.

### 4. Failure-mode predictions

Given this objective, what is the likely reward-hacking move? What
"improvement" would game the metric without improving real quality?
Each entry becomes an explicit reject heuristic the ops agent applies
to its own draft findings before emitting them.

For example, if the primary is "task success rate" and success is
counted by `agent.loop.outcome = success`, a trivial hack is to
lower the iteration budget so loops fail fast and the denominator
shrinks. Name that up front.

### 5. Evaluation window

How many completed flow runs constitute a meaningful sample before
the ops agent is allowed to emit a finding. Prevents premature
diagnosis on small-sample noise. State the window size and the
rationale (e.g., "20 deep-research loops — typical weekly volume
at current adoption").

## What the ops agent does with this

1. Trigger rule fires after N completed loops of a given flow
   (N = evaluation window).
2. Ops agent task payload carries `{ flow_name, observation_window }`.
3. Ops agent loads `docs/objectives/<flow-name>.md` as prompt context.
4. Ops agent queries graph predicates named in the spec, computes the
   primary and secondaries over the window, and drafts findings.
5. Ops agent runs its own reject heuristics against each draft.
6. Surviving findings land as `ops.diagnosis.*` triples on the flow
   entity, each citing the objective spec by path.

## Authoring checklist

Before merging a new objective spec:

- [ ] Primary metric has an exact formula in terms of named predicates.
- [ ] Baseline is either measured or has an explicit "TBD after N
      loops" target.
- [ ] At least one secondary metric is named (cost or latency
      minimum).
- [ ] At least three immutable guardrails are listed.
- [ ] At least three failure-mode predictions are listed.
- [ ] Evaluation window is stated with a sample-size rationale.
- [ ] A reviewer who doesn't know the flow can read the spec and
      judge whether a given proposed change would violate it.
