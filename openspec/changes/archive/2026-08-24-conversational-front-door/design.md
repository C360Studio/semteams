# Design: Conversational Front Door

## Context

ADR-042 gives the coordinator sole front-door authority. ADR-058 narrows the live category taxonomy to research and
autoresearch while leaving the spec/dev packs on disk but unwired. The living spec already owns shortcut behavior; this
change adds the conversational decision that occurs before any live category dispatch.

## Decisions

### Decision: The coordinator remains the only front door

Do not add a chat agent above the coordinator. A second classifier would split audit ownership and duplicate
`respond_direct`, `ask_user`, and category dispatch. Plain chat and team-hinted chat use the same coordinator path.

### Decision: Conversation may resolve or refine before dispatch

The coordinator may answer product questions through `respond_direct` or ask one concrete question through `ask_user`.
It dispatches only after the message satisfies the selected live category's contract: research asks need evidence, and
autoresearch asks name a scalar metric, measurement command, direction, bounded surface, and sandbox target.

Examples:

- `/research MQTT vs NATS for IoT edge` may route to `decide(action="research")`.
- `/optimize make this faster` lacks a measurable contract, so the coordinator asks one concrete question.

### Decision: Parked-team asks fail honestly at the front door

Spec-authoring and implementation tokens are not visible team shortcuts. If a user types `/create-change`, `/spec`,
`/dev-via-test`, `/build`, `/dev`, or `/implement-spec`, the message still reaches the coordinator as ordinary intent.
The coordinator emits `respond_direct`, names the unavailable capability, and offers a live alternative where useful.
It never emits a parked category token.

## Evidence

The coordinator routing matrix proves direct answers, one-question clarification, valid research routing, guarded
autoresearch routing, and honest parked-team responses.
