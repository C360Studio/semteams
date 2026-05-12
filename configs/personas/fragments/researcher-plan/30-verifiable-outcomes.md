# Verifiable outcomes

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
- *"`mvn test` exits 0."* — that's exactly the loophole closes.

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

## Why this is YOUR job, not the ARCHITECT phase's

The ARCHITECT phase downstream has to emit `checks[]` on the
structured artifact. Each check's `target` field is a verifiable
claim; ARCHITECT TRANSCRIBES from your outcomes, not INVENTS from
your epic prose. If you don't enumerate outcomes, ARCHITECT ends
up doing your job under emission pressure — and that's the
small-LLM capability wall the per-phase contracts explicitly
budget against.

PLAN enumerates outcomes. GATHER collects evidence against them.
SYNTHESIZE composes the structured research artifact. ARCHITECT
crystallizes the outcomes into `checks[]`. Reviewer (spec-mode)
enforces presence. Each phase contributes once.
