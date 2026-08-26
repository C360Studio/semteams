# ADR-060: Adopt SemStreams v1.0.0-beta.161 lifecycle reliability

**Status:** Accepted (2026-08-26)

**Implementation:** Blocked on
[SemStreams #1094](https://github.com/C360Studio/semstreams/issues/1094) (2026-08-26)

## Context

SemStreams published `v1.0.0-beta.161` on 2026-08-16 as the post-beta.160 reliability and lifecycle-control release.
SemTeams is beginning an always-on Program Pulse, so hidden lifecycle ownership, ambiguous terminal settlement, and
untyped response subjects are unacceptable foundations.

The release is intentionally breaking and provides no compatibility shims. Its downstream adoption contract requires
newly provisioned NATS storage. If retained deployed state is discovered, that deployment must stop for a separate
owner-reviewed migration or recovery design; the product must not wipe or silently reseed it.

The compiler and upstream migration inventory identify four direct SemTeams changes:

- `service.Manager.StopAll` now accepts a caller-owned `context.Context`, not a duration.
- `user.response.>` is reserved for typed `agentic.user_response/v1` messages. The flat `publish` actions in
  coordinator rules `03-ask-user` and `03b-respond-direct` are explicitly named downstream blockers.
- Every explicit dispatch `user.response` port override must preserve the typed interface declaration.
- Generated schemas remove `agentic-governance.violations.notify_user` and add `websocket.path` with default `/ws`.

Beta.161 also supplies the framework behavior SemTeams wants for unattended operation: normalized agent terminal
settlement, durable tool outcomes, bounded lifecycle joins, configured `MaxAckPending`, max-delivery observation,
slow-consumer attribution, caller-composed NATS observability, restored WebSocket paths, and complete ObjectStore write
binding.

## Decision

Adopt `github.com/c360studio/semstreams v1.0.0-beta.161` as one clean cut before implementing Program Pulse.

1. The process composition root creates an independent bounded shutdown context and passes that exact context to
   `manager.StopAll`. The canceled run context is never reused for teardown.
2. Coordinator rules `03-ask-user` and `03b-respond-direct` retain their clarification/reply audit triples but delete
   their generic `publish` actions. `agentic-dispatch` remains the sole response producer for those journeys.
3. Both bootstraps declare `agentic.user_response/v1` on the explicit `user.response` output port. The USER stream and
   message-logger observation remain; diagnostic observation is not claimed as end-user delivery.
4. Contract tests reject any flat rule writer under `user.response.>`, require both audit triples, require the typed
   dispatch port interface, and prove the production payload registry decodes a concrete `*agentic.UserResponse`.
5. Regenerate component schemas from beta.161. SemTeams already omits the retired governance field and accepts the
   upstream WebSocket default, so no product config override is added.
6. Start local and deployed verification on fresh NATS state. No legacy reader, union decoder, alias, bridge, retained
   state conversion, or downstream compatibility shim is permitted.

SemTeams does not select the upstream graph-research capability; its live `research` category is a product pack.
Beta.161's removal of phantom graph-research registration therefore requires no replacement wiring here.

Black-box validation found a second, blocking framework gap. Beta.161 publishes the directly submitted root
coordinator's intermediate handoff (`research` or `autoresearch`) because that loop owns the user route. The actual
terminal `respond_direct` coordinator is rule-spawned, owns no channel route, and is settled without publication.
[SemStreams #1094](https://github.com/C360Studio/semstreams/issues/1094) owns workflow-terminal selection and durable
reply correlation. SemTeams will not restore a flat rule writer or add a product-shell routing adapter while it waits.

After the correct workflow terminal is routable, beta.161 still copies the successful loop result verbatim into
`UserResponse.Content`. Because `decide` terminates with its full argument object, the response carries structured JSON
such as `{"action":"respond_direct","reason":"..."}` rather than bare `reason` prose.
[SemStreams #1090](https://github.com/C360Studio/semstreams/issues/1090) owns channel-ready decision rendering.

## Consequences

- Every beta.160-to-beta.161 stack cycle uses newly provisioned NATS storage. Long-lived retained state requires a new
  owner decision rather than an automated adoption step.
- User-response journeys correlate the typed response to the final coordinator loop. Counting the submission status or
  root handoff response is a false positive. Those journeys remain red until semstreams#1094 supplies workflow reply
  correlation.
- The coordinator audit facts remain queryable even though the unconsumed wake-up envelopes are gone.
- WebSocket deployments may now select a path explicitly; SemTeams retains the upstream `/ws` default.
- Program Pulse report generation may proceed independently, but human-facing delivery is gated first by
  semstreams#1094 (correct terminal routing), then semstreams#1090 (channel-ready rendering).

## Alternatives

- **Remain on beta.160 for the first Program Pulse.** Rejected because it puts scheduled operation on lifecycle and
  terminal semantics already corrected upstream and immediately creates migration debt.
- **Keep the flat coordinator writers on another subject.** Rejected because they have no consumer or delivery owner.
- **Add a flat/typed compatibility lane.** Rejected by the upstream clean-cut contract and unnecessary for unconsumed
  messages.
- **Reuse retained NATS state.** Rejected because beta.161 provides no migration or mixed-version contract.

## Related

- [SemStreams beta.161 release](https://github.com/C360Studio/semstreams/releases/tag/v1.0.0-beta.161)
- [ADR-059](059-semstreams-beta160-graph-foundation-adoption.md)
- [Program Pulse MVP epic](https://github.com/C360Studio/semteams/issues/269)
- [Adoption issue #266](https://github.com/C360Studio/semteams/issues/266)
- [SemStreams workflow terminal routing #1094](https://github.com/C360Studio/semstreams/issues/1094)
- [SemStreams typed decision rendering #1090](https://github.com/C360Studio/semstreams/issues/1090)
