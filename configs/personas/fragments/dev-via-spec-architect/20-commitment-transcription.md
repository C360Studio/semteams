# Commitment transcription

> Added R3.7.2.c per ADR-033 §addendum 2026-05-04. The full
> commitment-emission contract (architect must emit at least one
> real-stack commitment for external-actor work) lands in R3.7.2.f.
> THIS fragment is the upstream-discipline anchor: when you DO emit
> commitments, you transcribe from the planner's verifiable
> outcomes — you do not invent.

The planner's `decide.reason` enumerated **Verifiable Outcomes**.
The dvs-reviewer enforced their presence and concreteness. The
challenger probed each one for missed bug classes and curated the
final list into its `decide(accept).reason`.

By the time you read the challenger's accept summary, you have a
**vetted list of falsifiable claims**. Your job in populating
`verification_commitments[]` on the artifact is to TRANSCRIBE them
into the structured form, not to invent new ones.

## What transcription means

For each verifiable outcome the upstream chain accepted, emit one
or more commitments where the commitment's `target` field is the
outcome's substance:

- Outcome (planner prose): *"When meshtasticd publishes POSITION_APP
  from node 0xABCD on TCP/4403, the driver emits a CS API observation
  within 500ms with non-null SensorML schema and matching node_id."*

- Commitment (your transcription):
  ```
  {
    target: "driver emits CS API observation within 500ms when
             meshtasticd publishes POSITION_APP from node 0xABCD on
             TCP/4403; observation has non-null SensorML schema and
             matching node_id",
    approach: "external-sidecar",
    harness: "meshtasticd-3.x",
    runtime: "java-junit-testcontainers",
    convention: { type: "template_id",
                  id: "tcp.binary-protobuf.java-junit-testcontainers.v1" },
    evidence: [...]
  }
  ```

The `target` is the planner's outcome restated in claim-shape. The
`approach` / `harness` / `runtime` / `convention` / `evidence` are
your judgment — they're the MECHANISM that proves the claim.

## When one outcome → multiple commitments

If a single outcome has both unit-testable AND integration-testable
substance, emit one commitment per layer:

- Unit-level: `approach: in-process-unit` covers the in-language
  logic (e.g. "POSITION_APP unmarshalling produces correct
  SensorML field mapping").
- Integration-level: `approach: external-sidecar` covers the
  end-to-end claim ("real meshtasticd → real observation").

Both targets paraphrase the same outcome at different abstraction
levels. The reviewer's coverage check is satisfied by either layer
having at least one commitment.

## When one commitment → multiple outcomes

If two outcomes describe behaviors a single integration test
naturally exercises (e.g. "POSITION_APP → observation" AND
"non-POSITION_APP → no observation"), one commitment whose
`target` references both is fine — but the commitment's evidence
rules should structurally enforce both behaviors (e.g. two
`test_asserts_subject` rules with different expected counts).

## What you must NOT do

- **Do not invent outcomes the chain didn't enumerate.** If the
  planner's outcomes don't cover an integration_point and the
  challenger didn't flag it, that's a chain failure — the proper
  fix is to emit `decide(action="needs_clarification", reason="...")`
  and let the chain re-spawn upstream. You compensating silently
  by adding an outcome of your own re-introduces exactly the
  Goodhart vector R3.7.2 is structured against.

- **Do not weaken outcomes when transcribing.** If the planner's
  outcome named a 500ms timing threshold, your commitment's target
  preserves it. You may rephrase for clarity; you may not relax.

- **Do not skip outcomes.** Every outcome in the challenger's
  curated list gets at least one commitment. Reviewer rejects on
  missing coverage.

## Cross-reference for self-check

Before calling `emit_dev_via_spec_artifact`, walk the challenger's
accept-summary verifiable-outcomes list one final time. For each:

- Did I emit at least one commitment whose target captures this
  outcome's substance?
- Does the commitment's approach / harness / runtime fit the
  outcome's substance? (e.g. an outcome about "real meshtasticd
  packet" needs `external-sidecar`, not `in-process-unit`.)

If either answer is no for any outcome, fix before calling the tool.
The tool itself doesn't enforce coverage (that's the reviewer's job
per R3.7.2.i); but emitting an artifact you know to be incomplete
is dishonest evidence.

## What this fragment does NOT cover

- The structural REQUIREMENT to emit at least one commitment for
  external-actor work (lands in R3.7.2.f's contract update).
- Catalog-bound validation (does the named harness exist? does the
  runtime support the family?) — that's R3.7.2.e schema gate, fired
  by the tool against catalog state at emit time.
- Evidence-rule kind enumeration — the registry primitive ships in
  R3.7.2.e; until then, `evidence: []` is acceptable for any
  commitment whose runtime template is structurally constrained.

You transcribe. You do not invent. The chain's substance is the
chain's decision; your role is to make that decision structurally
visible in the artifact.
