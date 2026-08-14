# Design: Artifact Context Handoff

## Decisions

### Decision: Keep handoff UI-generic
Artifact cards should not know the semantic next hop. They may offer public team commands as convenience starters, but
the outbound prompt remains editable and the coordinator validates the route.

### Decision: Avoid new backend envelope for MVP
For this slice, the UI appends rendered artifact context to the next coordinator message. That gives humans immediate
artifact reuse without changing dispatch wire format or adding product-local artifact storage. A later change can add
durable `artifact_refs` once the backend has a framework-level artifact lookup contract.

### Decision: Show context before send
The chat bar displays an artifact context chip. Sending clears the context; dismissing the chip drops the artifact without
changing the typed prompt.

### Decision: Surface descendant artifacts from the parent task
The board still presents one card per top-level coordinator run, but the detail panel must expose descendant loops because
team artifacts often live below the direct child tier. Flattening descendants in parent-before-child order keeps emitted
artifacts reachable without introducing a new backend task entity for MVP.

## Risks
- Large artifacts can make prompts long. The MVP uses rendered markdown and leaves future summarization or durable refs to
  a backend-backed slice.
- A user may click the wrong team. This is acceptable because team commands are coordinator-routed hints, not bypasses.
