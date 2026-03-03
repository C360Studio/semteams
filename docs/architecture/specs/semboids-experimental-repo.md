# SemBoids — Experimental Coordination Playground

**Status**: Proposal
**Date**: 2026-02-27

## Summary

A separate repo (`github.com/c360studio/semboids`) providing a web UI where you type a task, watch agents self-organize via Boids signals, and see emergent coordination happen in real-time. SemSpec without the structure — just to see what happens.

## Motivation

Open-ended tasks are exactly where centralized orchestration breaks down. Strip away SemSpec's rigid phases (plan → review → task-gen → execute), give agents a goal, and let Boids coordinate. If emergent coordination works anywhere, it works here.

| Aspect | SemSpec | SemBoids |
|--------|---------|----------|
| Structure | Rigid phases, fixed roles | None — just agents and a goal |
| Coordination | Explicit reactive workflows | Boids steering signals only |
| Agent roles | architect, developer, reviewer | Undifferentiated (or minimal) |
| Task decomposition | Explicit (plan → tasks → subtasks) | Emergent |
| Value prop | Reliable, repeatable pipeline | Exploratory, adaptive, surprising |

## Architecture

```
┌──────────────────────────────────────────────────────┐
│                    Browser (Svelte)                    │
│  ┌──────────┐  ┌──────────────┐  ┌────────────────┐  │
│  │ Chat Box  │  │ Agent Feed   │  │ Boid Dashboard │  │
│  │ (input)   │  │ (activity)   │  │ (positions,    │  │
│  │           │  │              │  │  signals, graph)│  │
│  └──────────┘  └──────────────┘  └────────────────┘  │
└────────────────────────┬─────────────────────────────┘
                         │ WebSocket
┌────────────────────────┴─────────────────────────────┐
│                   Go Backend (semboids)                │
│                                                       │
│  HTTP/WS Server                                       │
│  ├── POST /api/task → spawn agents                    │
│  ├── WS /api/stream → real-time events                │
│  └── GET /api/status → agent/boid state               │
│                                                       │
│  Imports from semstreams:                              │
│  ├── processor/agentic-loop  (agent execution)        │
│  ├── processor/rule/boid     (rules, types, signals)  │
│  ├── processor/rule          (rule processor engine)   │
│  ├── natsclient              (NATS connection)        │
│  ├── component               (registry, deps)         │
│  └── message                 (BaseMessage, types)     │
│                                                       │
│  Subscribes to NATS subjects:                         │
│  ├── agent.boid.>     → steering signals to UI        │
│  ├── AGENT_POSITIONS  → position updates to UI        │
│  └── agent.response.> → agent output to UI            │
└────────────────────────┬─────────────────────────────┘
                         │
┌────────────────────────┴─────────────────────────────┐
│              NATS JetStream (Docker)                  │
│  ├── AGENT_POSITIONS KV bucket                        │
│  ├── AGENT stream                                     │
│  └── agent.boid.* subjects                            │
└──────────────────────────────────────────────────────┘
```

## Tech Stack

- **Backend**: Go, importing `github.com/c360studio/semstreams` (public repo)
- **Frontend**: SvelteKit + TypeScript
- **Infra**: Docker Compose (NATS JetStream)
- **One-command start**: `docker compose up` or `task dev`

## Repo Structure

```
semboids/
├── cmd/
│   └── semboids/
│       └── main.go              # Entry point: HTTP server + component bootstrap
├── internal/
│   ├── server/
│   │   ├── server.go            # HTTP/WebSocket server
│   │   ├── handlers.go          # API handlers (submit task, status)
│   │   └── stream.go            # WebSocket event streaming
│   ├── orchestrator/
│   │   ├── orchestrator.go      # Spawn/manage agent loops + rule processor
│   │   └── config.go            # Default boid-enabled config generation
│   └── observer/
│       ├── observer.go          # Subscribe to NATS, relay to WebSocket
│       └── events.go            # Event types for UI consumption
├── web/                         # SvelteKit frontend
│   ├── src/
│   │   ├── routes/
│   │   │   └── +page.svelte     # Main page: chat + dashboard
│   │   ├── lib/
│   │   │   ├── components/
│   │   │   │   ├── ChatInput.svelte
│   │   │   │   ├── AgentFeed.svelte
│   │   │   │   ├── SignalFeed.svelte
│   │   │   │   ├── PositionMap.svelte
│   │   │   │   └── MetricsPanel.svelte
│   │   │   ├── stores/
│   │   │   │   └── websocket.ts  # WebSocket store (Svelte 5 runes)
│   │   │   └── types/
│   │   │       └── events.ts     # TypeScript types matching Go events
│   │   └── app.html
│   ├── package.json
│   └── svelte.config.js
├── configs/
│   ├── boid-rules/              # Adapted from semstreams
│   │   ├── separation.json
│   │   ├── cohesion.json
│   │   └── alignment.json
│   └── default.yaml             # Default flow config
├── docker-compose.yml           # NATS JetStream
├── Taskfile.yml                 # task dev, task build, etc.
├── go.mod                       # module github.com/c360studio/semboids
├── go.sum
└── README.md
```

## Implementation Phases

### Phase 1: Repo Scaffolding + Go Backend

**`cmd/semboids/main.go`** — Entry point:
1. Parse flags (port, NATS URL, agent count, model)
2. Connect to NATS JetStream
3. Create KV buckets (AGENT_POSITIONS)
4. Boot rule processor with boid rules
5. Start HTTP/WebSocket server
6. Serve static frontend assets

**`internal/orchestrator/orchestrator.go`** — Agent lifecycle:
- `SubmitTask(task string, agentCount int)` → spawns N agentic-loop instances
- Each agent gets the same task, no role assignment
- Registers component factories from semstreams: `agenticloop.Register(registry)`
- Creates components with `boid_enabled: true`
- Tracks active agents, handles completion/failure

**`internal/observer/observer.go`** — NATS → WebSocket bridge:
- Subscribes to `agent.boid.>` (steering signals)
- Watches `AGENT_POSITIONS` KV (position updates)
- Subscribes to `agent.response.>` (agent output)
- Formats events and pushes to WebSocket clients

**`internal/server/server.go`** — HTTP + WS:
- `POST /api/task` — submit task, returns session ID
- `GET /api/ws` — WebSocket for real-time event stream
- `GET /api/status` — current agent/boid state snapshot
- Static file serving for frontend build

### Phase 2: Svelte Frontend

Main layout:

```
┌──────────────────────────────────────────────────────┐
│  SemBoids                              [3 agents] ▼  │
├──────────┬───────────────────────────────────────────┤
│          │  Agent-1: Exploring auth patterns...       │
│  Chat    │  Agent-2: Analyzing API structure...       │
│  Input   │  Agent-3: Reviewing data models...         │
│          ├───────────────────────────────────────────┤
│  [Send]  │  ● SEP Agent-2 ← avoid [auth.session]    │
│          │  ○ COH Agent-3 → focus [api.middleware]   │
│          │  △ ALN All → align [error_handling]       │
│          ├───────────────────────────────────────────┤
│          │  Positions       Metrics                   │
│          │  A1: ●●●○○       Overlap: 12%             │
│          │  A2: ○●●●○       Signals: 7               │
│          │  A3: ○○●●●       Velocity: 0.4 avg        │
└──────────┴───────────────────────────────────────────┘
```

| Component | Purpose |
|-----------|---------|
| `ChatInput.svelte` | Task input + agent count selector |
| `AgentFeed.svelte` | Real-time agent activity stream |
| `SignalFeed.svelte` | Boid signals as they fire (sep/coh/aln) |
| `PositionMap.svelte` | Visual of agent focus entities |
| `MetricsPanel.svelte` | Entity overlap %, signal count, velocity, tokens |

### Phase 3: Boid Toggle + Comparison Mode

- Toggle: "Boids ON / OFF" in the UI
- Run same task twice, compare results side-by-side
- Metrics per run: entity overlap, coverage breadth, tokens, time, signal count

### Phase 4: Polish + README

- One-command quickstart: `docker compose up && open http://localhost:8080`
- README with screenshots/GIF of agents coordinating
- Example tasks that demonstrate coordination well

## Prerequisites (semstreams)

The sister agent is fixing wiring gaps. SemBoids Phase 1 can proceed in parallel — gaps only matter when running agents with Boids enabled.

| Gap | Status |
|-----|--------|
| PositionProvider injection in rule factory | In progress |
| NATS subscription for `agent.boid.*` in agentic-loop | In progress |
| Signal consumption modifying context | Partially done (SignalStore built) |
| Entity state format resolution | Pending |

## Model Registry Integration

Uses the unified model registry (merged to semstreams main):

```yaml
model_registry:
  endpoints:
    default:
      provider: ollama
      url: "http://localhost:11434/v1"
      model: "qwen2.5-coder:32b"
      max_tokens: 32768
      supports_tools: true
  defaults:
    model: default
```

## Docker Compose

```yaml
services:
  nats:
    image: nats:latest
    command: ["-js", "-m", "8222"]
    ports:
      - "4222:4222"
      - "8222:8222"

  semboids:
    build: .
    ports:
      - "8080:8080"
    environment:
      - NATS_URL=nats://nats:4222
      - AGENT_COUNT=3
      - MODEL=qwen2.5-coder:32b
    depends_on:
      - nats
```

## Verification

```bash
# Phase 1: Backend
docker compose up -d nats
go run ./cmd/semboids/ --nats nats://localhost:4222
curl -X POST http://localhost:8080/api/task \
  -d '{"task": "Design a REST API", "agents": 3}'

# Phase 2: Frontend
cd web && npm run dev
# Open http://localhost:5173, submit a task

# Full stack
docker compose up
open http://localhost:8080
```

## Risks

| Risk | Mitigation |
|------|------------|
| semstreams API changes break semboids | Pin version in go.mod, update deliberately |
| Heavy transitive dependency tree | Accept it — semstreams is the platform |
| Agents produce chaos without structure | Start with planning tasks, add light structure if needed |
| Token costs with 3+ concurrent agents | Max iteration cap, model selector in UI |

## Related Documents

- [Boids Assessment](boids-assessment.md) — Honest evaluation of the Boids concept
- [Boids SemSpec Integration](boids-semspec-integration.md) — Integration proposal for SemSpec
