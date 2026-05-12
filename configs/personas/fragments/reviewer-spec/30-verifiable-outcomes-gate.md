# `checks[]` gate

In addition to the completeness checklist, walk the artifact's
`checks[]` for verification adequacy. The architect's commitment
contract requires `checks[]` to be populated when
`integration_points[]` names an external actor; you enforce
presence, coverage, and substance.

## When `checks[]` is required

The wire schema marks `checks[]` optional so v1 consumers see no
schema drift. The architect's commitment contract makes it
required when `integration_points[]` names any external actor
(an actor outside the workspace being built — e.g. an external
service, framework, or runtime).

Walk the artifact:

- [ ] **If `integration_points[]` names any external actor, is
      `checks[]` non-empty?** Empty `checks[]` when the artifact
      describes external integration is a gap. The builder needs
      to know what evidence proves the integration works.
- [ ] **Does every external-actor `integration_points[]` entry
      get covered by at least one check?** Walk
      `integration_points[]`; for each entry whose `from` or `to`
      names an external actor, find a check whose `target`
      describes the same flow. Missing coverage is a gap.

If no `integration_points[]` entry names an external actor, the
artifact may legitimately have empty `checks[]`. That is not a
gap.

## Check substance

For each entry in `checks[]`:

- [ ] **Is `target` a concrete verifiable claim?** Each check's
      `target` field names a specific input, a specific output,
      and (when relevant) a timing/threshold. "The driver works"
      is not a target — flag it. "When a `POSITION_APP` packet
      arrives on the radio, the driver emits a position triple
      within 500ms" is.
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

- *"`integration_points[]` lists the Meshtastic radio as an
  external actor but `checks[]` is empty. Cannot proceed: builder
  has no verifiable target for the external integration."*
- *"Check #2 target is 'driver receives data and forwards it' —
  not concrete; names neither input message type nor output shape.
  Tighten to specific protobuf message types and observation
  schema."*
- *"Check #1 target is 'the driver successfully starts.' That's a
  lifecycle event, not a behavior. Replace with a target that
  describes what the driver DOES once started."*
- *"`integration_points[]` entry `meshtastic-radio → osh-driver`
  has no covering check. Builder has no proof surface for this
  external integration."*

## Examples of things that are NOT valid grounds for `insufficient`

- *"Checks don't use Given/When/Then format in `target`."* —
  substance is what `target` claims; format is grammar.
- *"`checks[]` doesn't enumerate every edge case."* — coarse
  targets that name the happy-path observable behavior are
  sufficient at this gate. (Under MVP there is no challenger
  pass; if edge-case coverage degrades over time, the qa-mode
  reviewer's structural pre-checks against builder evidence
  catches the regression.)
- *"`evidence[]` rule kinds aren't all populated."* — `evidence[]`
  registry-validated; substance of the rules themselves is the
  evidence gate's concern, not yours.

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
