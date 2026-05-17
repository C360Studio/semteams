# `checks[]` gate

In addition to the completeness checklist, walk the artifact's
`checks[]` for verification adequacy. The architect's commitment
contract requires `checks[]` to be populated when
`integration_points[]` names an external actor; you enforce
presence, coverage, and substance.

## When `checks[]` is required

The wire schema marks `checks[]` optional so v1 consumers see no
schema drift. The architect's commitment contract makes it
required when `integration_points[]` names any **external actor**:
an actor outside this codebase's process — another service, a
real protocol daemon, a database, a browser session, an upstream
LLM, a hardware device. Internal-only actors (libraries that run
in-process, framework hooks that are exercised by unit tests
alone) do not by themselves require `checks[]`.

Walk the artifact:

- [ ] **If `integration_points[]` names any external actor, is
      `checks[]` non-empty?** Empty `checks[]` when the artifact
      describes external integration is a gap. The builder needs
      to know what evidence proves the integration works.
- [ ] **Does every external-actor `integration_points[]` entry
      get covered by at least one check whose `target`
      substantively describes that specific flow?** Walk every
      external-actor entry one at a time. For each, find a check
      whose `target` names that flow's data shape and direction
      — not just any check on the list. A 3-external-boundary
      artifact with 1 check has 2 uncovered boundaries; that's
      `insufficient` with the specific boundaries named in the
      reason.

If no `integration_points[]` entry names an external actor, the
artifact may legitimately have empty `checks[]`. That is not a
gap.

## Check substance

For each entry in `checks[]`:

- [ ] **Is `target` a concrete verifiable claim?** Each check's
      `target` field names a specific input, a specific output,
      and (when relevant) a timing/threshold. "The consumer works"
      is not a target — flag it. "When a record with key
      `order_created` arrives on the Kafka topic, the consumer
      commits an offset and emits a downstream `order.ack` within
      500ms" is.
- [ ] **Is `target` observable, not internal?** "Tests pass" is a
      process state, not a behavior. "Successfully starts" is a
      lifecycle event, not a behavior. Real targets describe
      something that crosses a system boundary — a message
      flowing, a response shape, a UI element appearing.
- [ ] **Does `target` ground against `actors[]` or
      `integration_points[]`?** Targets about "the system" without
      naming WHICH actor or integration_point are too coarse.
      Tighten to specific actors and data shapes.

## Structural alignment (light pass)

The wire schema enforces most structural rules (runtime enum,
test_harness required for testcontainer/sidecar/browser-flow,
ref discriminator). Your job is substance, not re-validation.
Still, a quick walk:

- [ ] **Does `runtime` match what `target` describes?** A target
      about "an external service responds with X" pairs naturally
      with `process-local-testcontainer` or `external-sidecar`,
      not `in-process-unit`. A `runtime` choice that contradicts
      `target`'s boundary description is a gap — the architect
      picked the wrong verification surface.
- [ ] **When `test_harness` is named, does it actually relate to
      the target?** A harness from `configs/harnesses.json`
      brings up a real dependency stack; pick one whose stack
      hosts what the target observes. Mismatch is a gap.

## Granularity check

The architect is supposed to flag integration_points that resist
check enumeration as too vague. If you see an integration_point
covering an external actor AND `checks[]` doesn't reach it AND
the artifact doesn't acknowledge the difficulty, that's a gap —
push back upstream.

If you see one check covering three integration_points, that's
also a gap. The builder cannot differentiate three failure
modes from one check; either split the check or split the
integration_points.

## Examples of substance gaps that warrant `insufficient`

- *"`integration_points[]` lists the Kafka broker as an
  external actor but `checks[]` is empty. Cannot proceed: builder
  has no verifiable target for the external integration."*
- *"Check #2 target is 'consumer receives data and forwards it' —
  not concrete; names neither input record key nor output shape.
  Tighten to specific Kafka record keys and downstream message
  schema."*
- *"Check #1 target is 'the consumer successfully starts.' That's a
  lifecycle event, not a behavior. Replace with a target that
  describes what the consumer DOES once started."*
- *"`integration_points[]` entry `kafka-broker → order-consumer`
  has no covering check. Builder has no proof surface for this
  external integration."*

## Examples of things that are NOT valid grounds for `insufficient`

- *"Checks don't use Given/When/Then format in `target`."* —
  substance is what `target` claims; format is grammar.
- *"`checks[]` doesn't enumerate every edge case."* — coarse
  targets that name the happy-path observable behavior are
  sufficient at this gate. The qa-mode reviewer's structural
  pre-checks against builder evidence catches edge-case
  regressions later.
- *"`evidence[]` rule semantics look wrong."* — whether each
  evidence rule passes at build time is the qa-mode reviewer's
  concern, not yours. You grade target substance + coverage.

## When to send back upstream

If the artifact's `tasks[]` are decomposable but the architect
just didn't enumerate `checks[]` → `insufficient` with bullet
list naming missing checks per external `integration_points[]`
entry.

If the artifact's `integration_points[]` themselves are too vague
to support check enumeration → `insufficient` with a note that
the underlying integration_point is the problem; the architect
needs to sharpen the data flow before check work begins.

You evaluate. You do not author checks for the architect. If a
gap requires a specific check to be named, name the boundary that
needs covering and let the architect choose the wording.

ARCHITECT enumerates `checks[]`. You enforce presence and
substance. BUILDER's evidence gate evaluates the `evidence[]`
rules within each check at build time. Each phase contributes
once.
