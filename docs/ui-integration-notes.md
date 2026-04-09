# UI Integration Notes: Agentic Superpowers

Implementation notes for the semstreams-ui team. The backend (Phases 1-4) is
complete in semteams. These notes describe the APIs, data structures, and
components the UI needs to integrate with.

## Backend API Reference

All endpoints are on the agentic-dispatch HTTP handler, typically prefixed
with `/agentic-dispatch/`.

### Existing Endpoints

| Method | Path | Description |
|--------|------|-------------|
| POST | `/message` | Submit a user message (sync response) |
| GET | `/commands` | List available slash commands |
| GET | `/health` | Component health |
| GET | `/loops` | List all tracked loops |
| GET | `/loops/{id}` | Get specific loop details |
| POST | `/loops/{id}/signal` | Send control signal to a loop |
| GET | `/activity` | SSE stream of real-time agent activity |
| GET | `/debug/state` | Internal state dump |

### Signal Endpoint

**POST `/loops/{id}/signal`**

Currently accepts `pause`, `resume`, `cancel`. **Needs extension** to also
accept `approve` and `reject` for the approval gate workflow. The backend
slash commands `/approve` and `/reject` work via NATS pub, but the HTTP
endpoint validation at `http.go:584-591` only allows 3 types. This is a
known gap — either extend the HTTP endpoint or have the UI use the message
endpoint with `/approve {loop_id}` command text.

```json
// Request
{ "type": "approve", "reason": "Looks good" }

// Response
{ "loop_id": "loop_abc12345", "signal": "approve", "status": "sent" }
```

### Activity SSE Stream

**GET `/activity`**

Streams from `AGENT_LOOPS` KV bucket. Each event is a JSON-encoded loop
state update. Event types emitted:

- `connected` — initial connection confirmation
- `sync_complete` — initial KV state delivered
- `loop_update` — loop state changed (the main event)
- `heartbeat` — SSE comment every 30s

The `loop_update` data contains the full loop entity serialized from KV:

```json
{
  "loop_id": "loop_abc12345",
  "task_id": "...",
  "state": "executing",
  "role": "editor",
  "iterations": 3,
  "max_iterations": 20,
  "user_id": "user-1",
  "channel_type": "web",
  "parent_loop_id": "",
  "outcome": "",
  "error": ""
}
```

**Key states to render differently:**
- `exploring`, `planning`, `architecting`, `executing`, `reviewing` — active, show spinner
- `paused` — show resume button
- `awaiting_approval` — show approve/reject buttons (this is the approval gate)
- `complete` — show result, green status
- `failed` — show error, red status
- `cancelled` — show cancelled status

## New UI Components Needed

### 1. AgentLoopCard (chat attachment)

Inline card in the chat message list showing an active agent loop. Rendered
when the activity SSE stream reports a `loop_update` for the current user.

**Data source:** Activity SSE → `loop_update` events
**Displays:** Loop ID (truncated), state badge, iteration progress bar,
current role, elapsed time
**Updates:** Real-time via SSE

### 2. ApprovalPrompt (chat attachment)

Rendered when a loop enters `awaiting_approval` state. This happens when an
agent calls a tool marked `RequiresApproval: true` (currently: `create_rule`,
`update_rule`, `delete_rule`).

**Displays:** What the agent wants to do (from loop metadata), approve button,
reject button with optional reason textarea
**Actions:**
- Approve → `POST /loops/{id}/signal` with `{"type": "approve"}`
- Reject → `POST /loops/{id}/signal` with `{"type": "reject", "reason": "..."}`

### 3. ToolCallCard (chat attachment)

Shows tool calls in progress during agent execution.

**Data source:** Activity SSE (tool execution events in loop metadata)
**Displays:** Tool name, arguments (collapsed/expandable), result (when
complete), duration, error state
**Tool names to expect:** `bash`, `web_search`, `http_request`, `graph_query`,
`create_rule`, `update_rule`, `delete_rule`, `list_rules`, `get_rule`,
`github_read`, `github_write`

### 4. RuleDiffCard (chat attachment)

Shown when an agent proposes a rule change via `create_rule` or `update_rule`.
Part of the approval flow — the user sees the rule JSON before approving.

**Displays:** Rule ID, name, conditions summary, actions summary, before/after
diff (for updates), approve/reject buttons
**Data:** Comes from the tool call arguments in the loop's pending tool results

### 5. Agent Monitoring Page (`/agents` route)

Dedicated page for monitoring all agent activity.

**Components:**
- Active loops table (loop_id, state, role, iterations, user, age)
- Agent tree view (parent/child hierarchy via `parent_loop_id`)
- Token usage summary (from loop metadata)
- Activity timeline (chronological SSE events)

**Data source:** `GET /loops` for initial state, `GET /activity` SSE for
real-time updates

### 6. TrajectoryViewer

Full replay of a completed agent's execution. Each iteration shows the LLM
request, response, tool calls, and results.

**Data source:** Loop trajectory data (stored in NATS KV, accessible via loop
detail endpoint once trajectory API is added)

## New Slash Commands

Add these to the client-side slash command registry (`slashCommands.ts`):

| Command | Maps to | Page |
|---------|---------|------|
| `/approve [loop_id]` | POST message with `/approve {loop_id}` | both |
| `/reject [loop_id] [reason]` | POST message with `/reject {loop_id} {reason}` | both |
| `/pause [loop_id]` | POST message with `/pause {loop_id}` | both |
| `/resume [loop_id]` | POST message with `/resume {loop_id}` | both |

These go through the normal message endpoint — the backend command registry
handles routing to the signal system.

## New Chat Attachment Types

Extend the `MessageAttachment` discriminated union in `chat.ts`:

```typescript
// New kinds to add alongside existing flow, search-result, entity-detail, etc.
type AgentLoopAttachment = {
  kind: 'agent-loop';
  loopId: string;
  state: string;
  role: string;
  iterations: number;
  maxIterations: number;
  parentLoopId?: string;
};

type ApprovalAttachment = {
  kind: 'approval';
  loopId: string;
  toolName: string;
  toolArgs: Record<string, unknown>;
  question: string;  // What the agent wants to do
};

type ToolCallAttachment = {
  kind: 'tool-call';
  toolName: string;
  args: Record<string, unknown>;
  result?: string;
  error?: string;
  status: 'pending' | 'running' | 'complete' | 'error';
  durationMs?: number;
};
```

## New Store: agentStore

Create `src/lib/stores/agentStore.svelte.ts` for centralized agent state:

```typescript
// Subscribes to activity SSE stream
// Tracks all loops, their hierarchies, and state transitions
// Used by: AgentLoopCard, ApprovalPrompt, Agent monitoring page

interface AgentState {
  loops: Map<string, LoopInfo>;
  connected: boolean;
  lastEvent: Date | null;
}
```

## Integration with Existing Chat System

The chat system already has:
- SSE streaming from `POST /api/ai/chat`
- Context chips for pinning domain objects
- Multiple attachment card types

The agent activity SSE stream is **separate** from the AI chat SSE. The UI
needs to maintain two SSE connections:
1. `POST /api/ai/chat` — for AI responses (existing)
2. `GET /agentic-dispatch/activity` — for agent loop status (new)

Agent events from stream #2 should inject attachment cards into the chat
message list when they relate to the current user's active conversation.

## MCP Tool Updates

Add these tools to the MCP tool registry (`toolRegistry.ts`) for the AI
chat assistant to use:

| Tool | Description |
|------|-------------|
| `agent_loops` | List active agent loops with status |
| `agent_signal` | Send signal (approve/reject/pause) to a loop |
| `list_rules` | List active rules in the engine |
| `get_rule` | Get full rule definition |

## Priority Order

1. Activity SSE client + agentStore (foundation for everything else)
2. AgentLoopCard (basic loop visibility)
3. ApprovalPrompt (human-in-the-loop gate)
4. Slash commands (/approve, /reject, /pause, /resume)
5. ToolCallCard (tool execution visibility)
6. Agent monitoring page (/agents)
7. RuleDiffCard (self-programming visibility)
8. TrajectoryViewer (execution replay)

## Known Backend Gaps

1. **HTTP signal endpoint needs approve/reject** — `http.go:584` only validates
   pause/resume/cancel. Either extend validation or use message endpoint as
   workaround.
2. **Trajectory API not yet exposed** — Loop trajectory data exists in KV but
   there's no HTTP endpoint to retrieve it. Needed for TrajectoryViewer.
3. **Activity SSE events lack tool-level detail** — Currently streams full loop
   state from KV. Tool call start/complete events would need to be added for
   real-time ToolCallCard updates.
