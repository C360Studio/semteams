# Autoresearch category rule pack

This live category pack measures a baseline, iterates propose → execute, compares numeric measurements in a Go tool,
and synthesizes a reviewed artifact when the run cap is exhausted. It uses the shared substrate in
`configs/flow-bootstrap.json`; it does not build a separate runtime flow. The design is recorded in
[ADR-042](../../../docs/adr/042-coordinator-instantiated-flows-via-templates.md) and
[ADR-053](../../../docs/adr/053-adoption-plan.md).

The migration to frozen SemStreams `8b99efe9c66a` preserves run-owned iteration state and returns final results through
the actual origin coordinator. [Migration evidence](../../../docs/migrations/semstreams-8b99efe/README.md) separates
mock orchestration proof from live-model quality, retained-state migration and evidence-body rendering claims.

## State and identity

The origin coordinator loop and run are distinct entities with the same instance UUID:

- Loop: `{org}.{platform}.agentic-loop.agent.execution.{uuid}`.
- Run: `{org}.{platform}.chain.agent.execution.{uuid}`.

Rule 01 publishes the baseline with `run_scope: new`. The framework records the actual origin on the run and assigns
native task ancestry. `emit_autoresearch_baseline` initializes parameters, baseline, running best, `run.status=active`
and `iteration.pending=initial` on the run. Its parameters are command, allowed surface, cap and metric parser;
the baseline persona extracts them from the coordinator's intent and the emitter validates the structured input.

The run owns the cap, current status, best value/experiment, pending marker and append-only experiment journal.
The coordinator loop is not the run entity. Run-triggered iteration tasks retain category metadata in
`related_loops`; that metadata is not a replacement for the framework's typed route ancestry.

## Iteration driver and empirical comparison

Rule 05 matches an active run with a positive cap and pending marker. It first clears the marker with the owned
`reconcile_predicates` action, allowing the next pending update to create a fresh match transition. The durable
rule-state iteration selects propose while `$state.iteration <= cap`; the next transition selects synthesis and
reconciles run status to stopped. Late pending updates cannot restart a stopped run.

The repeating clear/propose actions explicitly use `max_iterations: 0`, the framework's unlimited action setting.
The run cap therefore governs the number of experiments. Omitting this setting previously imposed an independent
three-firing default and stalled a cap-five run at iteration four; the migration's behavioral test captures that red
and proves five proposes followed by one synthesis. Synthesis/stop retain their existing terminal branch.

`emit_autoresearch_measurement` reads the current best, compares numerically, and records kept/reverted/crashed on
the execute loop. Lower is better; a failed pass gate cannot become the best even if its numeric metric improves.
Rule 04c promotes only kept measurements through owned reconciliation of best value and experiment ID. The final
artifact's LLM reviewer checks the rollup; it does not make the empirical keep/revert comparison.

Execute accounting remains explicit:

- Clean `measured` completion adds its execute entity to the journal and re-arms pending (04a).
- Involuntary execute failure also consumes a slot and records the failed execute (04b).
- Successful `needs_clarification` and failures outside execute return to the coordinator without silently
  consuming another execute slot. Budgeted execute failures do not terminate the whole run.

The journal is append-only evidence, not the cap condition. A prompt's `.length` suffix on one scalar journal value
is not the number of journal triples. Existing prompt-grounding limits are documented below.

## Rule map

| Rules | Responsibility |
|---|---|
| 01 | Coordinator `autoresearch` decision creates/asserts the run and spawns baseline |
| 03 | Propose `measure` decision spawns execute |
| 04a / 04b | Record clean / involuntarily failed execute attempts and re-arm the run marker |
| 04c | Promote kept best value and experiment reference through owned reconciliation |
| 05 | Run-local marker/iteration driver; propose until cap, then synthesize and stop |
| 07 | Synthesize `emit` decision spawns reviewer |
| 08 | Successful approved reviewer appends approved source fact and existing success outcome to the run |
| 09 | Insufficient reviewer re-rolls synthesis with the existing bounded retry |
| 10 | Baseline clarification retains its native-parent coordinator return |
| 10b | Successful descended clarification appends its source fact to the run |
| 11 | Non-execute loop failure records the existing pause diagnostic |
| 12 / 13 | Baseline / descended non-budgeted failure appends failure source and existing failed outcome |
| 14 / 15 | Completed run forwards approved fact to origin; origin spawns result coordinator |
| 16 / 17 | Executing run forwards clarification fact to origin; origin spawns clarification coordinator |
| 18 / 19 | Failed run forwards failure fact to origin; origin spawns failure-report coordinator |

Role personas are `autoresearch-baseline`, `autoresearch-propose`, `autoresearch-execute`,
`autoresearch-synthesize` and `reviewer-autoresearch`, under `configs/personas/fragments/`.
The file numbering preserves historical removals; there are no current rule 02 or 06 dispatchers.

## Origin return and lifecycle

The three append-only product facts are `autoresearch.reply.approved`, `autoresearch.reply.clarification` and
`autoresearch.reply.failed`. Values are full source-loop entity IDs. Each source writer requires one unambiguous
run anchor; each bridge requires one source, one actual origin and a canonical same-authority/same-instance origin.
Approved, clarification and failure bridges require completed, executing and failed run phases respectively.

The origin loop must be a coordinator whose decision is `autoresearch`. Its return action uses `run_scope: new`
to assert the existing origin-rooted run through the framework's idempotent Mint path. This preserves phase and
origin while assigning the return child's native parent and run IDs, even if the loop already carries an older
inherited anchor. `related_loops.terminal`, `.rejecting` or `.failed` names the full source entity for reading its
result; `autoresearch-run` names the root UUID. No conflicting old run anchor is threaded into the return task.

Success/failure reports permit `respond_direct`. Clarification permits `respond_direct` or `ask_user`, subject to
the existing deployment clarification policy. `ask_user` pauses the run; a limitation `respond_direct` deliberately
leaves it executing under the existing agent-run outcome policy. A delivered limitation is not completed optimization.

Persisted rule state suppresses tested duplicate/stale events and evaluator recreation. The rule engine does not
atomically commit task publication with state persistence; crash-window exactly-once delivery is not promised.
Likewise, the phase guard does not make cancellation after action selection atomic. The frozen action's existing
Mint-error fallback can publish with a parent and no run ID; direct Mint refusal is not a stronger publication fence.
See the [independent review](../../../docs/migrations/semstreams-8b99efe/autoresearch-repair-review.md).

## Sandbox and qualification limits

Autoresearch mutates and measures the per-run attested workspace provisioned by `request_sandbox`.
The existing sandbox manager, attestation runner, approval and command boundaries remain in force; the terminal
repair changes none of those permissions. Mock fixtures use isolated infrastructure and attested test workspaces;
they do not prove paid-model judgment or production measurement quality.

The five focused repair journeys prove actual emitter/graph facts where applicable, native ancestry, original-channel
results/prompts, run phase and inspection of the raw typed envelope in the existing UI. They include both clarification
decisions and a real loop-budget failure. The complete matrix and source identities are in the
[browser report](../../../docs/migrations/semstreams-8b99efe/browser-baseline.md).

Known boundaries remain:

- The cap is the hard stop. Plateau, wallclock and cost-budget stop policies are not implemented.
- Rule03's explanatory best-value token resolves against the wrong entity; rule05's journal `.length` prose uses a
  scalar object. Both defects occur at beta.160 and the frozen pin. The real emitter comparison and state-based cap
  are separately tested; mock orchestration does not qualify live-model interpretation of those prompts.
- Rich artifact/prose evidence-body rendering and context handoff remain limited by
  [#261](https://github.com/C360Studio/semteams/issues/261). Raw envelope inspection is a narrower capability.
- No retained beta.160 state conversion, SemSource dogfooding, SemEngine acceptance or Program Pulse shipment is
  established by this pack. Parked development packs remain unwired.

The pack has no `chain.mode`, phasevalidator or chainstall dependency. Those historical mechanisms remain retired.
The measurement/baseline/artifact emitters are existing product-shell tools with their own tests; they are not future
companion work or mock-only substitutions. Their alignment rationale remains in the existing ADR/tool documentation.
