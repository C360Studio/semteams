# Proposal: Artifact Context Handoff

## Intent
Humans need to reuse reviewed artifacts as context for whatever they do next. A research artifact may feed spec
authoring, more research, optimization, implementation, or a front-door clarification turn. The UI should make that
handoff easy without assuming the next team.

## Scope
In scope:
- A generic artifact context affordance for emitted artifacts such as research and autoresearch outputs.
- Copying rendered artifact context for use outside SemTeams.
- Seeding the coordinator chat with any public team command while attaching the artifact as prompt context.
- A visible context chip so the human can see and clear the artifact before sending.

Out of scope:
- A backend artifact-reference schema or durable artifact library.
- Auto-dispatching a follow-up team when an artifact is selected.
- Assuming research always feeds OpenSpec authoring.
- Changing OpenSpec artifact review/export behavior.

## Approach
Render emitted artifacts as reusable context, not terminal-only trace details. Generic artifact cards provide "Copy" plus
"Use with" actions for the public team commands. Selecting a team fills the chat input with an editable prompt starter and
attaches the rendered artifact context to the next outbound coordinator message. The coordinator still decides whether the
prompt is shaped enough for the selected team or needs clarification.
