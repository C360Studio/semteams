# Proposal: Conversational Front Door

## Intent
Humans see a chat box and expect to chat before they commit work to a team. SemTeams should let the coordinator handle
that front-door conversation: explain capabilities, refine a vague idea, suggest the right team, and only dispatch when the
request is shaped enough for the selected category.

## Scope
In scope:
- Coordinator policy for conversational intake before team dispatch.
- Product-level team slash commands as intent hints, not routing bypasses.
- UI command chips and slash hints for research, OpenSpec authoring, optimization, and dev-via-test.
- Routing-matrix evidence for direct chat and command-hinted team selection.

Out of scope:
- A second chat agent above the coordinator.
- Direct human access to internal phase roles such as Lisa, Ralph, CBG, or reviewers.
- Bypassing sandbox, proof-readiness, approval, or clarification gates because a slash command was used.
- Long-running coaching sessions that need their own artifact or memory model; those can become a future category pack.

## Approach
Keep the coordinator as the single human front door. Plain chat remains a normal coordinator turn, with `respond_direct`
used for capability guidance, product questions, and idea refinement. `ask_user` remains the one-question clarification
path. Team slash commands such as `/research`, `/create-change`, `/optimize`, and `/dev-via-test` are strong hints carried
in the user's message; the coordinator validates the prompt shape and either dispatches the matching `decide()` action,
asks for the missing fact, or explains the better route.

The UI exposes only product-level categories. Internal implementation roles stay hidden so the public contract remains
stable while category packs evolve.
