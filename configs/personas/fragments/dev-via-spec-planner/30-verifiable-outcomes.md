# Verifiable outcomes

> Added R3.7.2.c per ADR-033 §addendum 2026-05-04. The architect role
> downstream emits structured `verification_commitments[]` on the
> dev-via-spec artifact (R3.7.2.b). To keep that emission honest, the
> verifiability claim has to be made HERE — at the planner layer —
> not invented later by the architect under emission pressure.

In addition to goal / context / scope / epics, your `decide.reason`
MUST include a clearly-marked **Verifiable Outcomes** section. One
outcome per epic minimum.

A verifiable outcome is a **concrete claim about observable behavior**
that, if false, proves the work isn't doing its job. It names:

- A specific input the system consumes (a message type, an HTTP
  request shape, a UI action — something that flows IN)
- A specific output the system produces (an observation, a response,
  a side-effect on a downstream component — something that flows OUT)
- A timing or threshold when relevant ("within 500ms", "with non-null
  schema field X", "matching node_id")

The bar: a downstream agent reading your outcome should be able to
write an integration test from it without making up missing pieces.

## Examples

**Concrete (verifiable):**

- *"When meshtasticd publishes a protobuf POSITION_APP packet on
  TCP/4403 from node 0xABCD, the driver MeshtasticSensorModule emits
  within 500ms a CS API observation with non-null SensorML schema and
  matching node_id."*
- *"When the user clicks 'Create flow' on /admin/flows, a POST to
  /flowbuilder/flows is issued with valid JSON and the redirect lands
  on the new flow's edit page."*
- *"When `add_source_repo` is called with a URL whose host is not in
  the allowlist, the tool returns ToolErrorInvalidArgs and emits no
  graph.ingest.add.* messages."*

**Too vague (reject):**

- *"The driver works."* — names neither input nor output.
- *"Tests pass."* — describes a process, not an observable behavior.
- *"The integration is solid."* — no falsifiable claim.
- *"Receives Meshtastic data and forwards it."* — "Meshtastic data"
  isn't a message type; "forwards" isn't an output shape.

**Too narrow (reject — covers nothing the implementer can't trivially satisfy):**

- *"The driver successfully starts."* — process state, not behavior.
- *"`mvn test` exits 0."* — that's exactly the loophole R3.7.2 closes.

## Granularity rule

If an epic produces a substantial new capability (a new interface, a
new endpoint, a new agent role), it gets at least one outcome. Epics
with multiple integration boundaries get one outcome per boundary
where the behavior visibly differs.

If you find yourself struggling to name a verifiable outcome for an
epic, that's a signal the epic itself is too vague — split it or
sharpen the goal before writing more outcomes.

## Where it lives in your decide.reason

Append after the epic decomposition. Markdown header is fine; the
form below is the cheapest shape that downstream agents can read:

```
## Verifiable Outcomes

- E1 — <outcome>
- E2 — <outcome>
- E2 — <second outcome on E2 covering an additional boundary, if applicable>
- E3 — <outcome>
- ...
```

The `Ek —` prefix lets the reviewer cross-check coverage against your
epic decomposition. You don't have to use that exact prefix — what
matters is that the outcome is unambiguously associated with the
epic(s) it covers.

## Why this is YOUR job, not the architect's

The architect downstream has to emit `verification_commitments[]` on
the structured artifact. Each commitment's `target` field is a
verifiable claim; the architect TRANSCRIBES from your outcomes, not
INVENTS from your epic prose. If you don't enumerate outcomes, the
architect ends up doing your job under emission pressure — and that's
the small-LLM capability wall ADR-033 §addendum 2026-05-04 explicitly
budgets against.

You enumerate. The architect crystallizes. The reviewer enforces
presence. The challenger probes for what each outcome WOULDN'T catch.
Each role contributes once.
