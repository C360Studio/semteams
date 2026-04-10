# SemStreams UI

Visual graph explorer and flow builder for any SemStreams-compatible application.

## Overview

SemStreams UI is a standalone frontend that works with **any** application built on the SemStreams framework:

- **semstreams** - Core stream processing framework
- **semsource** - Semantic source analysis
- **semmem** - Semantic memory management
- **Your application** - Any SemStreams-based system

The UI discovers components, schemas, and graph data **at runtime** from the backend's APIs. No hardcoded component types or backend-specific logic.

## Features

- **Graph Explorer** - Interactive knowledge graph visualization (Sigma.js/WebGL) as the default homepage
- **Contextual Chat** - AI assistant with slash commands, context chips, and tool integration
- **Flow Builder** - Visual flow editor for creating and deploying processing pipelines
- **Runtime Monitoring** - Health, logs, metrics, and message tracing tabs
- **Backend-Agnostic** - Connects to any running SemStreams app via standard API endpoints
- **Runtime Discovery** - Dynamically loads component schemas from `/components/types`

## Tech Stack

- **SvelteKit 2** - Full-stack framework
- **Svelte 5** - Reactive UI with runes system (`$state`, `$derived`, `$effect`, `$props`)
- **TypeScript** - Strict types, generated from OpenAPI specs
- **Sigma.js** - WebGL graph visualization
- **Graphology** - Graph data structure
- **Vitest** + **@testing-library/svelte** - 3099+ unit/component tests
- **Playwright** - E2E testing

## Quick Start

### Prerequisites

- Node.js 22+ (see `.nvmrc`)
- A running SemStreams-compatible backend
- Caddy (`brew install caddy` on macOS)
- Docker (optional, for full-stack dev)

### Connect to a Running App (Recommended)

Point the UI at any running SemStreams application — no Docker needed for the backend:

```bash
npm install

# Connect to a local backend
task dev:connect

# Connect to a remote backend
BACKEND_HOST=myapp.example.com:8080 task dev:connect

# With separate GraphQL gateway
BACKEND_HOST=app:8080 GRAPHQL_HOST=app:8082 task dev:connect
```

Open `http://localhost:3001` — you'll land on the graph explorer with chat.

### Full Stack Development (Build from Source)

Start NATS + backend + Caddy + Vite from source:

```bash
# Start everything (requires ../semstreams sibling directory)
task dev:full
# Access at http://localhost:3001
```

Manage individually:

```bash
task dev:backend:start   # Start NATS + backend in background
task dev                 # Start Vite dev server
task dev:backend:logs    # View backend logs
task dev:backend:stop    # Stop backend
```

Custom ports:

```bash
DEV_UI_PORT=3002 DEV_VITE_PORT=5174 task dev:full
```

## Architecture

### Request Flow

```
Browser --> Caddy (:3001) --+--> /flowbuilder/*  --> Backend (:8080)
                            +--> /components/*   --> Backend (:8080)
                            +--> /health         --> Backend (:8080)
                            +--> /graphql        --> GraphQL Gateway
                            +--> /*              --> Vite (:5173)
```

Caddy eliminates CORS by serving everything from a single origin. The UI makes relative fetch calls; Caddy routes them to the right backend.

### Pages

| Route         | Purpose                                               |
| ------------- | ----------------------------------------------------- |
| `/`           | Graph explorer (DataView) — default homepage          |
| `/flows`      | Flow list — create and manage flows                   |
| `/flows/[id]` | Flow editor — visual canvas, chat, runtime monitoring |

### Key Directories

```
src/
├── lib/
│   ├── components/          # 51 Svelte 5 components
│   │   ├── chat/            # Chat system (ChatPanel, SlashCommandMenu, ContextChips, etc.)
│   │   ├── runtime/         # Runtime tabs (Health, Logs, Metrics, Messages, SigmaCanvas)
│   │   └── layout/          # Three-panel layout
│   ├── services/            # API clients (chatApi, graphApi, slashCommands)
│   ├── server/              # SvelteKit server (AI providers, tool registry, MCP)
│   ├── stores/              # Runes-based stores (graphStore, chatStore, runtimeStore)
│   └── types/               # TypeScript types (chat, graph, flow, slashCommand)
├── routes/
│   ├── +page.svelte         # Graph explorer homepage
│   ├── flows/+page.svelte   # Flow list
│   └── flows/[id]/          # Flow editor
└── hooks.server.ts          # SSR fetch transformation
e2e/                         # Playwright E2E tests
docs/                        # Architecture docs
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

`graphStore` uses `SvelteMap`/`SvelteSet` from `svelte/reactivity` for reactive collections.

## Chat System

The chat assistant supports:

- **Slash commands** — `/search`, `/flow`, `/explain`, `/debug`, `/health`, `/query`
- **Context chips** — Pin entities or components to the chat context via "+Chat" buttons
- **Page-aware tools** — Different tools available on flow-builder vs data-view pages
- **Attachment-based messages** — Rich responses with search results, entity details, health status, flow diffs
- **Multi-provider AI** — Anthropic (Claude) or OpenAI-compatible APIs

## Configuration

### Environment Variables

```bash
# Backend connection (used by Caddy and SvelteKit server)
BACKEND_HOST=localhost:8080        # Backend host:port
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

Generate types from any backend's OpenAPI spec:

```bash
task generate-types:semstreams       # From semstreams
task generate-types:semmem           # From semmem
OPENAPI_SPEC_PATH=/path/to/spec.yaml task generate-types  # Custom spec
BACKEND_URL=http://localhost:8080 task generate-types:from-url  # Running backend
```

## Testing

### Unit Tests (Vitest)

```bash
npm run test        # Run all tests (3099+ tests, 136 files)
npm run test:ui     # Watch mode
```

### E2E Tests (Playwright)

```bash
npm run test:e2e    # Auto-manages Docker stack
task test:e2e:semsource-graph  # With semsource graph integration
```

### Quality Checks

```bash
npm run check       # TypeScript type checking (0 errors)
npm run lint        # ESLint
npm run format      # Prettier
```

## Backend Requirements

Your SemStreams app must expose:

1. **Component Types**: `GET /components/types`
2. **Flow Management**: `GET|POST /flowbuilder/flows`, `GET|PUT|DELETE /flowbuilder/flows/:id`
3. **Health Check**: `GET /health`
4. **GraphQL** (optional): `POST /graphql` for graph explorer

## Task Commands

| Command                  | Description                                            |
| ------------------------ | ------------------------------------------------------ |
| `task dev:connect`       | Connect to any running backend (no Docker needed)      |
| `task dev:full`          | Full stack from source (NATS + backend + Caddy + Vite) |
| `task dev`               | Vite dev server only                                   |
| `task dev:backend:start` | Start backend infra in background                      |
| `task dev:backend:stop`  | Stop backend infra                                     |
| `task test`              | Unit tests                                             |
| `task test:e2e`          | E2E tests                                              |
| `task lint`              | ESLint                                                 |
| `task check`             | TypeScript checking                                    |
| `task clean`             | Clean Docker volumes                                   |
| `task generate-types`    | Generate TS types from OpenAPI                         |

## Documentation

- **[docs/architecture/CHAT_REDESIGN.md](docs/architecture/CHAT_REDESIGN.md)** - Chat system architecture
- **[docs/architecture/SEMSOURCE_E2E_INTEGRATION.md](docs/architecture/SEMSOURCE_E2E_INTEGRATION.md)** - SemsSource E2E integration
- **[docs/testing/](docs/testing/)** - E2E testing guide
- **[docs/auth/](docs/auth/)** - Authentication patterns
- **[INTEGRATION_EXAMPLE.md](INTEGRATION_EXAMPLE.md)** - Backend integration guide
- **[E2E_SETUP.md](E2E_SETUP.md)** - E2E test setup

## Troubleshooting

```bash
# Check backend is reachable
curl http://localhost:8080/health

# Check component discovery
curl http://localhost:8080/components/types | jq

# Check GraphQL
curl -X POST http://localhost:8080/graphql -H 'Content-Type: application/json' \
  -d '{"query":"{ entitiesByPrefix(prefix: \"\", limit: 5) { id } }"}' | jq

# Docker cleanup (clears NATS KV stale data)
docker compose -f docker-compose.dev.yml down -v
task clean
```
