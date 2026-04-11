# SemTeams UI

The Svelte 5 + SvelteKit 2 frontend for [semteams](../README.md) — a graph
explorer, flow builder, chat-driven assistant, and agentic operations console
for the semteams backend.

> **History:** this tree was forked from
> [`semstreams-ui`](https://github.com/C360Studio/semstreams-ui) on 2026-04-10 and
> imported into `semteams/ui/` as a subtree. The upstream `semstreams-ui` repo
> continues to live as generic operational glass for the semstreams framework
> (used by semsource). This fork adds agentic superpowers — `AgentLoopCard`,
> `ApprovalPrompt`, `ToolCallCard`, `RuleDiffCard`, `/agents` monitoring page,
> real-time activity SSE — and drops the aspirational multi-backend framing.

## Features

- **Graph Explorer** — Interactive knowledge graph visualization (Sigma.js/WebGL)
  as the default homepage
- **Contextual Chat** — AI assistant with slash commands, context chips, and
  tool integration, including agentic slash commands (`/approve`, `/reject`,
  `/pause`, `/resume`)
- **Flow Builder** — Visual flow editor for creating and deploying processing
  pipelines
- **Agentic Operations** — Real-time agent loop visibility, approval gates for
  high-risk tool calls (`create_rule`, `update_rule`, `delete_rule`, etc.),
  multi-agent hierarchy rendering, trajectory replay
- **Runtime Monitoring** — Health, logs, metrics, and message tracing tabs
- **Runtime Discovery** — Dynamically loads component schemas from
  `/components/types` on the semteams backend

## Tech Stack

- **SvelteKit 2** — Full-stack framework
- **Svelte 5** — Reactive UI with runes (`$state`, `$derived`, `$effect`,
  `$props`)
- **TypeScript** — Strict types, generated from the semteams OpenAPI spec
- **Sigma.js** + **Graphology** — WebGL graph visualization
- **Vitest** + **@testing-library/svelte** — 3300+ unit/component tests
- **Playwright** — E2E tests

## Quick Start

### Prerequisites

- Node.js 22+
- A running semteams backend on `localhost:8080` (or configurable)
- Caddy (`brew install caddy` on macOS) — for the dev reverse proxy
- Docker (optional, for full-stack dev)

### Connect to a Running Backend

Point the UI at any running semteams backend — no Docker needed for the backend
itself:

```bash
npm install

# Connect to a local backend
task ui:dev:connect

# Connect to a remote backend
BACKEND_HOST=semteams.example.com:8080 task ui:dev:connect
```

Open `http://localhost:3001` — you'll land on the graph explorer with chat.

### Full Stack from Source

From the semteams repo root:

```bash
task build                    # Build the semteams binary
./bin/semstreams serve        # Start the backend on :8080 (see configs/)
cd ui && task dev:connect     # Start the UI pointing at :8080
```

Or use the dev composite (requires Docker):

```bash
cd ui && task dev:full
```

## Architecture

### Request Flow

```
Browser --> Caddy (:3001) --+--> /agentic-dispatch/* --> semteams (:8080)
                            +--> /agentic-loop/*     --> semteams (:8080)
                            +--> /flowbuilder/*      --> semteams (:8080)
                            +--> /components/*       --> semteams (:8080)
                            +--> /health             --> semteams (:8080)
                            +--> /graphql            --> GraphQL gateway
                            +--> /*                  --> Vite (:5173)
```

Caddy serves everything from a single origin to eliminate CORS. The UI makes
relative fetch calls; Caddy routes them to the right endpoint.

### Pages

| Route         | Purpose                                               |
| ------------- | ----------------------------------------------------- |
| `/`           | Graph explorer (DataView) — default homepage          |
| `/flows`      | Flow list — create and manage flows                   |
| `/flows/[id]` | Flow editor — visual canvas, chat, runtime monitoring |
| `/agents`     | Agent monitoring — live loop state, approvals, replay |

### Key Directories

```
src/
├── lib/
│   ├── components/
│   │   ├── agents/             # TrajectoryViewer for execution replay
│   │   ├── chat/               # ChatPanel + attachment cards:
│   │   │                       #   AgentLoopCard, ApprovalPrompt,
│   │   │                       #   ToolCallCard, RuleDiffCard, ...
│   │   ├── runtime/            # Runtime tabs (Health, Logs, Metrics, Graph)
│   │   └── layout/             # Three-panel layout
│   ├── services/               # API clients (chatApi, graphApi, agentApi)
│   ├── server/                 # SvelteKit server (AI providers, MCP, GraphQL)
│   ├── stores/                 # Runes-based stores:
│   │                           #   graphStore, chatStore, runtimeStore,
│   │                           #   agentStore (SSE-backed loop tracker)
│   └── types/                  # TypeScript types (chat, graph, flow, agent)
├── routes/
│   ├── +page.svelte            # Graph explorer homepage
│   ├── flows/                  # Flow list + editor
│   └── agents/                 # Agent monitoring page
└── hooks.server.ts
e2e/                            # Playwright E2E tests
docs/                           # In-tree architecture docs
```

### Store Pattern

Stores use the runes-based factory function pattern:

```typescript
function createMyStore() {
  let value = $state<Type>(initial);
  return {
    get value() {
      return value;
    },
    setValue(v: Type) {
      value = v;
    },
  };
}
export const myStore = createMyStore();
```

`graphStore` and `agentStore` use `SvelteMap`/`SvelteSet` from
`svelte/reactivity` for reactive collections.

## Chat System

The chat assistant supports:

- **Slash commands** — `/search`, `/flow`, `/explain`, `/debug`, `/health`,
  `/query`, `/approve`, `/reject`, `/pause`, `/resume`
- **Context chips** — Pin entities or components to the chat context via
  "+Chat" buttons
- **Page-aware tools** — Different tools available on flow-builder vs
  graph-explorer vs agent-monitoring pages
- **Attachment-based messages** — Rich responses: search results, entity
  details, health status, flow diffs, agent loop cards, approval prompts,
  tool call cards, rule diffs
- **Multi-provider AI** — Anthropic (Claude) or OpenAI-compatible APIs

## Agentic Integration

This UI renders semteams' agentic superpowers in real time. Key integration
points:

| UI element           | Backend source                                  |
| -------------------- | ----------------------------------------------- |
| `AgentLoopCard`      | SSE `GET /agentic-dispatch/activity`            |
| `ApprovalPrompt`     | Loop state `awaiting_approval`; signals via POST |
| `ToolCallCard`       | Loop trajectory events                          |
| `RuleDiffCard`       | `create_rule` / `update_rule` tool args         |
| `/agents` page       | `GET /agentic-dispatch/loops` + activity SSE    |
| `TrajectoryViewer`   | `GET /agentic-loop/trajectories/{loopId}`       |

See `docs/ui-integration-notes.md` at the semteams repo root for the full
backend API reference, signal types, and attachment shape definitions.

## Configuration

### Environment Variables

```bash
# Backend connection (used by Caddy and SvelteKit server)
BACKEND_HOST=localhost:8080        # semteams backend host:port
GRAPHQL_HOST=localhost:8082        # GraphQL gateway (defaults to BACKEND_HOST)
BACKEND_URL=http://localhost:8080  # Full URL for server-side AI/MCP calls

# AI provider
AI_PROVIDER=anthropic              # "anthropic" or "openai"
ANTHROPIC_API_KEY=sk-ant-...
OPENAI_API_KEY=sk-...
OPENAI_MODEL=gpt-4o
OPENAI_BASE_URL=https://api.openai.com/v1

# Development ports
DEV_UI_PORT=3001                   # Caddy listen port
DEV_VITE_PORT=5173                 # Vite dev server port
```

### TypeScript Type Generation

Generate types from the semteams OpenAPI spec:

```bash
# From the semteams backend's generated spec
OPENAPI_SPEC_PATH=../specs/openapi.v3.yaml task generate-types

# From a running backend
BACKEND_URL=http://localhost:8080 task generate-types:from-url
```

## Testing

### Unit Tests (Vitest)

```bash
npm run test        # Run all tests
npm run test:ui     # Watch mode
```

### E2E Tests (Playwright)

```bash
npm run test:e2e    # Auto-manages Docker stack
```

### Quality Checks

```bash
npm run check       # TypeScript type checking
npm run lint        # ESLint
npm run format      # Prettier
```

## Task Commands

All `task` commands work both from `semteams/ui/` directly and from the
semteams root via the `ui:` namespace (e.g. `task ui:dev`, `task ui:test`,
`task ui:test:e2e`).

| Command                  | Description                                            |
| ------------------------ | ------------------------------------------------------ |
| `task dev:connect`       | Connect to any running semteams backend                |
| `task dev:full`          | Full stack from source (NATS + semteams + Caddy + Vite) |
| `task dev`               | Vite dev server only                                   |
| `task dev:backend:start` | Start semteams backend infra in background            |
| `task dev:backend:stop`  | Stop semteams backend infra                           |
| `task test`              | Unit tests                                             |
| `task test:e2e`          | E2E tests                                              |
| `task lint`              | ESLint                                                 |
| `task check`             | TypeScript checking                                    |
| `task clean`             | Clean Docker volumes                                   |
| `task generate-types`    | Generate TS types from OpenAPI                         |

## Troubleshooting

```bash
# Check backend is reachable
curl http://localhost:8080/health

# Check component discovery
curl http://localhost:8080/components/types | jq

# Check GraphQL
curl -X POST http://localhost:8080/graphql -H 'Content-Type: application/json' \
  -d '{"query":"{ entitiesByPrefix(prefix: \"\", limit: 5) { id } }"}' | jq

# Check agentic activity stream
curl -N http://localhost:8080/agentic-dispatch/activity

# Docker cleanup (clears NATS KV stale data)
docker compose -f docker-compose.dev.yml down -v
task clean
```
