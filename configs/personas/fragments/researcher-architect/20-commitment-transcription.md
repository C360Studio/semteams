# Check transcription

The PLAN phase's `decide.reason` enumerated **Verifiable
Outcomes**. The SYNTHESIZE phase composed the structured
research artifact (actors, integration_points, tasks) against
those outcomes.

By the time you read the SYNTHESIZE artifact + the PLAN's
verifiable outcomes, you have a **list of falsifiable claims**.
Your job in populating `checks[]` on the artifact is to
TRANSCRIBE them into the structured form, not to invent new ones.

## What transcription means

For each verifiable outcome the upstream chain accepted, emit one
or more checks where the check's `target` field is the outcome's
substance:

- Outcome (planner prose): *"When meshtasticd publishes POSITION_APP
  from node 0xABCD on TCP/4403, the driver emits a CS API observation
  within 500ms with non-null SensorML schema and matching node_id."*

- Check (your transcription):
  ```
  {
    target: "driver emits CS API observation within 500ms when
             meshtasticd publishes POSITION_APP from node 0xABCD on
             TCP/4403; observation has non-null SensorML schema and
             matching node_id",
    runtime: "external-sidecar",
    test_harness: "meshtasticd-2.x",
    test_runtime: "java-junit-testcontainers",
    ref: { type: "template_id",
           id: "tcp.binary-protobuf.java-junit-testcontainers.v1" },
    evidence: [...]
  }
  ```

The `target` is the planner's outcome restated in claim-shape. The
`runtime` / `test_harness` / `test_runtime` / `ref` / `evidence`
are your judgment — they're the MECHANISM that proves the claim.

## When one outcome → multiple checks

If a single outcome has both unit-testable AND integration-testable
substance, emit one check per layer:

- Unit-level: `runtime: in-process-unit` covers the in-language
  logic (e.g. "POSITION_APP unmarshalling produces correct
  SensorML field mapping").
- Integration-level: `runtime: external-sidecar` covers the
  end-to-end claim ("real meshtasticd → real observation").

Both targets paraphrase the same outcome at different abstraction
levels. Reviewer (spec-mode)'s coverage check is satisfied by
either layer having at least one check.

## When one check → multiple outcomes

If two outcomes describe behaviors a single integration test
naturally exercises (e.g. "POSITION_APP → observation" AND
"non-POSITION_APP → no observation"), one check whose `target`
references both is fine — but the check's evidence rules should
structurally enforce both behaviors (e.g. two `test_asserts_subject`
rules with different expected counts).

## What you must NOT do

- **Do not invent outcomes the chain didn't enumerate.** If the
  PLAN phase's outcomes don't cover an integration_point that
  SYNTHESIZE included, that's a chain inconsistency. Do NOT call
  `emit_dev_via_spec_artifact`. Instead terminate with
  `decide(action="needs_clarification", reason="...")` so the
  rule layer can route back to the appropriate upstream phase.
  The reason field names the specific gap concretely:

  ```
  decide(action="needs_clarification",
         reason="Verifiable-outcomes coverage incomplete:
                 integration_point Meshtastic-radio→OSH-driver-framework
                 has no outcome in PLAN's enumerated list. PLAN
                 needs to enumerate what observable behavior would
                 prove this integration is working.")
  ```

  Be specific about which outcome is missing and which upstream
  phase can fill it (PLAN, almost always — outcomes are PLAN's
  contribution). The rule layer routes back accordingly.

  Compensating silently by adding an outcome of your own
  re-introduces exactly the Goodhart vector the per-phase
  contracts structure against. Better an honestly flagged gap
  than a fabrication.

- **Do not weaken outcomes when transcribing.** If the planner's
  outcome named a 500ms timing threshold, your check's target
  preserves it. You may rephrase for clarity; you may not relax.

- **Do not skip outcomes.** Every outcome in PLAN's enumerated
  list gets at least one check. Reviewer (spec-mode) rejects on
  missing coverage.

## Cross-reference for self-check

Before calling `emit_dev_via_spec_artifact`, walk PLAN's
verifiable-outcomes list one final time. For each:

- Did I emit at least one check whose target captures this
  outcome's substance?
- Does the check's runtime / test_harness / test_runtime fit the
  outcome's substance? (e.g. an outcome about "real meshtasticd
  packet" needs `external-sidecar`, not `in-process-unit`.)

If either answer is no for any outcome, fix before calling the tool.
The tool itself doesn't enforce coverage (that's the reviewer's job
per); but emitting an artifact you know to be incomplete
is dishonest evidence.

## What this fragment does NOT cover

- The structural REQUIREMENT to emit at least one check for
  external-actor work — see the commitment contract.
- Catalog-bound validation (does the named test_harness exist? does
  the runtime support the family?) — that's the schema gate,
  fired against catalog state at emit time.
- Evidence-rule kind enumeration — the registry primitive is the
  per-runtime list of allowed evidence kinds. Until that registry
  ships, an empty `evidence: []` is acceptable ONLY when the
  check's runtime template (e.g.
  `tcp.binary-protobuf.java-junit-testcontainers.v1`) is itself
  structurally constrained at the framework layer — the template
  enforces what the explicit evidence rules will enforce later.
  This exception expires the moment the registry ships; do not
  generalize it to mean "evidence is optional."

You transcribe. You do not invent. The chain's substance is the
chain's decision; your role is to make that decision structurally
visible in the artifact.
