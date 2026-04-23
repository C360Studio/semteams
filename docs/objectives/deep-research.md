# Objective: deep-research

Flow config: `configs/deep-research.json`.

## 1. Primary metric

**Name**: `deep_research_task_success_rate`

**Formula**: Over a window of completed deep-research loops, fraction
that satisfy **all three** of the following:

- `agent.loop.outcome = success` on the root research loop.
- At least 3 `research.has_evidence` triples linked to the loop entity.
- A non-empty `research.synthesis.present = true` triple on the loop
  entity.

Expressed as a graph query:

```
count(loops where outcome=success AND has_evidence_count >= 3 AND synthesis.present=true)
/
count(loops where flow_name=deep-research AND state=completed)
```

**Direction**: up.

**Baseline**: TBD. Establish from the first 20 completed production
deep-research loops after ADR-028 Layer 1 + the beta.8 consolidation
landed (f91c3d6, 2026-04-17). The spec is emitting findings against
an unmeasured baseline is itself a finding the ops agent should
surface.

## 2. Secondary metrics (Pareto axes)

### `deep_research_cost_usd_p95`

**Formula**: p95 of `LoopCostUSD` across completed deep-research
loops in the window.

**Direction**: down.

**Regression budget**: the ops agent may propose changes that raise
p95 cost by up to 20% if the primary success rate improves by ≥5
absolute points. Larger cost rises must be flagged explicitly in the
finding and surface to human review before any Phase 2 action.

### `deep_research_iterations_p95`

**Formula**: p95 of the iteration count per loop (from
`LoopIterationCount` or equivalent predicate).

**Direction**: down.

**Regression budget**: p95 may rise by up to 2 iterations if the
primary improves. A large iteration increase without a primary
improvement is the signal of a broken tuning, not a Pareto tradeoff.

### `deep_research_tokens_p95`

**Formula**: p95 of `LoopTokensTotal`.

**Direction**: down.

**Regression budget**: up to 25% rise is tolerable if primary
improves. Above that, the ops agent should propose a context
compaction change rather than accept the regression.

## 3. Immutable guardrails

The ops agent must **never** propose a change that touches any of
these. A finding that implies such a change is a bug in the ops
agent's reasoning and should be self-rejected before emission.

- `agentic-governance.enable_tool_governance` — must remain `true`.
- `agentic-tools.approval_required` list — the default entries
  (`manage_flow`, `create_rule`, `bash`, `http_request`) stay gated.
  The ops agent may propose *adding* names to this list; it may not
  propose removing any.
- `agentic-tools.enable_categories` — must remain `true` where the
  deep-research config sets it.
- Model floor — the researcher role must never be proposed to a model
  smaller than Haiku-class. Downgrading below Haiku reliably collapses
  synthesis quality without a visible metric signal.
- Safety prompt fragments — any fragment in the assembled prompt
  whose filename starts with `safety/` or `guardrail/` stays in. The
  ops agent may propose reordering non-safety fragments; it may not
  propose removing safety ones.
- Evidence floor — the primary metric's `>= 3` evidence-triple
  threshold is itself a guardrail. Proposals that rework the metric
  formula to require fewer evidence triples count as weakening the
  objective, not tuning toward it.

## 4. Failure-mode predictions

Pre-registered reward-hacking moves the ops agent should reject
even if they score well on the primary.

1. **Model downgrade masks quality loss.** Swapping the researcher
   to a smaller model can raise apparent success rate because small
   models shorter-circuit to "done" with weaker evidence. Signal:
   primary up, `LoopTokensTotal` sharply down, evidence triple count
   per successful loop also down. If evidence-triples-per-success
   drops while primary rises, reject.

2. **Prompt trimming collapses synthesis.** Trimming research prompt
   fragments ("be thorough," "cite sources," etc.) reduces token
   cost and can look like a free secondary-axis win. Signal: cost p95
   down ≥ 15%, primary flat, synthesis text length distribution
   shifting toward shorter outputs. If synthesis length median
   drops by > 20%, reject.

3. **Iteration cap drops the denominator.** Lowering `max_iterations`
   causes stuck loops to time out and not count as `state=completed`,
   artificially inflating success rate among those that do complete.
   Signal: primary up, absolute count of completed loops per window
   down, timeout outcomes up. If completed-loops-per-window drops by
   > 10%, reject.

4. **Cooldown/retry tweaks hide tool failures.** Tightening retry
   policy on flaky tools can make tool-failure signals disappear from
   the graph rather than resolving them. Signal: `StepToolStatus =
   failure` count drops without a corresponding change in
   `StepToolName` distribution — failures were silenced, not fixed.
   Reject unless the ops agent can point to the *root cause* fix in
   the finding's evidence.

5. **Evidence threshold drift.** Proposals that quietly reinterpret
   what counts as a `research.has_evidence` triple (e.g., allowing
   low-confidence extractions). If the predicate's emission conditions
   were changed by a proposed rule or prompt edit, reject the proposal
   as guardrail violation (see §3 evidence floor).

## 5. Evaluation window

**Window size**: 20 completed deep-research loops per flow.

**Rationale**: Small enough to be achievable at current adoption
(roughly a week of production usage in early deployments, less once
the UI drives volume). Large enough that the primary metric's noise
floor is tolerable — with 20 loops and a baseline success rate of
50%, the 95% confidence interval on the rate is roughly ±22 points,
wide but serviceable for Phase 1 diagnostic purposes. Phase 3
Pareto evaluation will need larger windows.

**Trigger**: the `configs/rules/ops/observe-complete-loops.json`
rule fires once every 20 completed deep-research loops (tracked via
rule cooldown), publishing a task to the ops agent flow.
