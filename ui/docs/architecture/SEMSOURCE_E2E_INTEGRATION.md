# SemSource E2E Integration Architecture

Semsource is a semstreams application. It runs the full graph pipeline internally — no separate semstreams backend is needed for graph data. The UI queries semsource's graph-gateway directly via GraphQL.

## Data Flow

```
┌─────────────────────────────────────────────────────────────────────┐
│ Host Machine (Playwright)                                           │
│                                                                     │
│  Playwright Tests ──► http://localhost:3000                         │
│                                                                     │
├─────────────────────────────────────────────────────────────────────┤
│ Docker Network (e2e-net)                                            │
│                                                                     │
│  ┌───────────────── semsource ─────────────────────────┐            │
│  │                                                     │            │
│  │  fixture/ ──► ast-source ──► NATS (graph.ingest.>)  │            │
│  │               doc-source         │                  │            │
│  │               cfgfile-source     ▼                  │            │
│  │                            graph-ingest             │            │
│  │                                  │                  │            │
│  │                         ENTITY_STATES KV            │            │
│  │                                  │                  │            │
│  │                            graph-index              │            │
│  │                                  │                  │            │
│  │                            graph-query              │            │
│  │                                  │                  │            │
│  │                          graph-gateway (:8080)      │            │
│  │                                  │                  │            │
│  │  + websocket-output (:7890)      │                  │            │
│  │    (for SemSpec/SemDragon)       │                  │            │
│  └──────────────────────────────────┼──────────────────┘            │
│                                     │                               │
│  ┌─────────┐                        │    ┌──────────┐               │
│  │ backend │  /flowbuilder/*        │    │    UI    │               │
│  │ :8080   │  /components/*         │    │ (Vite)   │               │
│  │         │  /health               │    └────┬─────┘               │
│  └────┬────┘                        │         │                     │
│       │         ┌──────────┐        │         │                     │
│       └────────►│  Caddy   │◄───────┘─────────┘                     │
│                 │ :3000    │                                         │
│                 └──────────┘                                         │
│                      │                                              │
│                 exposed to host                                      │
└─────────────────────────────────────────────────────────────────────┘
```

Semsource port map:

- `:8080` — service manager (component catalog, health, flow-builder API)
- `:8082` — graph-gateway (GraphQL `/graphql`, playground)
- `:7890` — websocket output (entity stream for SemSpec/SemDragon)
- `:9091` — metrics (Prometheus `/metrics`)

Caddy routing:

- `/graphql` → backend:8082 (graph-gateway)
- `/flowbuilder/*`, `/components/*`, `/health` → backend:8080 (service manager)
- `/*` → ui:5173 (Vite dev server)

(`backend` is a network alias for the semsource container.)

## Why Semsource IS the Backend (for graph data)

Semsource registers all semstreams components via `componentregistry.Register()` and builds its own semstreams config internally (`buildSemstreamsConfig`). The graph pipeline runs inside semsource:

1. Source components publish entities to NATS JetStream (`graph.ingest.entity` on GRAPH stream)
2. `graph-ingest` consumes from GRAPH stream, writes ENTITY_STATES KV bucket
3. `graph-index` watches ENTITY_STATES, builds OUTGOING/INCOMING/ALIAS/PREDICATE indexes
4. `graph-query` handles NATS request/reply for entity, relationships, pathSearch
5. `graph-gateway` serves HTTP GraphQL at `:8080/graphql` (playground enabled)
6. `websocket-output` consumes from GRAPH stream via JetStream, serves WS at `:7890/graph` for external consumers

No WebSocket hop, no federation processor, no bridge. Direct graph pipeline.

## E2E Fixture

A small, deterministic Go project checked into this repo:

```
e2e/fixtures/semsource/
├── semsource-e2e.json          # semsource config (namespace: e2e, watch: false)
├── src/
│   ├── main.go                 # ~24 lines, imports context/fmt/log/os/signal
│   ├── handler.go              # ~45 lines, Handler interface + DefaultHandler impl
│   └── handler_test.go         # ~15 lines
├── go.mod                      # module fixture-project, go 1.22
├── README.md                   # short doc
└── Dockerfile                  # minimal Dockerfile
```

Produces known entities:

- **AST**: `e2e.semsource.golang.data-fixture.function.src-main-go-main`, `e2e.semsource.golang.data-fixture.interface.src-handler-go-Handler`, etc.
- **Docs**: `e2e.semsource.web.data-fixture.doc.87457b`
- **Config**: `e2e.semsource.config.data-fixture.gomod.fixture-project`, `e2e.semsource.config.data-fixture.image.golang-1-22-alpine`

Total: ~16-27 entities (including duplicates from relationship triples). Fast ingestion (<2s).

Key config decisions:

- `watch: false` — ingest once, emit SEED events, no file watching (deterministic)
- `namespace: "e2e"` — distinct from production
- No `git`, `url`, or media sources — avoids network access during tests

## Docker Compose Setup

Semsource is activated via Docker Compose profile:

```bash
COMPOSE_PROFILES=semsource GRAPHQL_HOST=semsource:8080 \
  docker compose -f docker-compose.e2e.yml up
```

The `GRAPHQL_HOST` env var tells Caddy to route `/graphql` to semsource instead of the backend. Default (without semsource profile) routes to `backend:8082`.

## E2E Test Structure

```
e2e/
├── semsource-graph/
│   ├── graph-rendering.spec.ts      # Entities appear in DataView
│   ├── graph-interaction.spec.ts    # Select, hover, expand
│   ├── graph-filtering.spec.ts      # Filter by type/domain
│   └── helpers/
│       └── semsource-helpers.ts     # Wait-for-entity utilities
├── fixtures/
│   └── semsource/                   # Fixture Go project + config
```

## Determinism Strategy

1. **Fixed fixtures** — checked into repo, entity IDs derive from file paths + symbol names + namespace
2. **Polling with known IDs** — `waitForSemsourceEntities()` polls GraphQL until entities appear
3. **watch: false** — one SEED event per entity, no DELTAs or file-watching races
4. **Generous timeouts** — 30s wait (actual: 2-5s, Docker startup adds latency)
5. **Prefix assertions** — assert `e2e.semsource.*` entities exist, not exact counts

## Running

```bash
# Full semsource graph E2E
COMPOSE_PROFILES=semsource GRAPHQL_HOST=semsource:8080 \
  BACKEND_CONTEXT=../semstreams SEMSOURCE_CONTEXT=../semsource \
  npx playwright test e2e/semsource-graph/

# Core E2E only (no semsource)
BACKEND_CONTEXT=../semstreams npx playwright test --ignore-pattern='semsource-graph/**'
```
