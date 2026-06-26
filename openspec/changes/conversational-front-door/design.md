# Design: Conversational Front Door

## Decision: Coordinator remains the only front door
Do not add a chat agent above the coordinator. A second agent would duplicate classification, split audit ownership, and
create ambiguous authority over `respond_direct`, `ask_user`, and category dispatch. The existing ADR-042 shape already
has one front-door role plus category packs; this change makes that role more chat-capable.

## Decision: Team commands are intent hints
Power-user commands are sent through the same `/teams-dispatch/message` path as any other user prompt. The command prefix
does not call a team directly. It tells the coordinator what the user intends, and the coordinator still enforces the
target team's contract.

Examples:
- `/research MQTT vs NATS for IoT edge` is a strong hint toward `decide(action="research")`.
- `/optimize make this faster` is not enough for autoresearch because it lacks a metric, command, and bounded surface; the
  coordinator should ask for the missing facts.
- `/dev-via-test write a blog intro` is the wrong team; the coordinator should respond directly or suggest create-change
  or research, depending on the ask.

## Decision: Product-level commands only
Expose category commands, not internal phase roles. Users should learn workflows such as research, spec authoring,
optimization, and implementation-with-tests. They should not need to know Lisa, Ralph, CBG, reviewer role names, or rule
pack internals.

## Evidence
The routing matrix is the first proof point: command-hinted prompts still produce coordinator decisions, and vague or
meta prompts remain `ask_user` / `respond_direct` rather than spawning teams blindly. A later UI transcript slice should
make `coordinator.user_reply` feel like ordinary chat, but the dispatch contract in this change is independent of that UI
projection.
