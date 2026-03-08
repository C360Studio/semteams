# SemSource E2E Integration Architecture

Integration design for validating the full pipeline: semsource ingests code/docs, emits graph events via WebSocket, the semstreams backend receives them directly into graph-ingest, and the semstreams-ui DataView renders the graph via Sigma.js.

## Recommended Approach

**Option: Fixture directory with pre-recorded events as fallback.**

Use semsource running against a small, deterministic fixture directory checked into this repo. The fixture contains a handful of Go files, a README, and a go.mod -- enough to produce known entity IDs from the AST, doc, and config handlers. This gives true end-to-end coverage without depending on external repos or network access.

For CI environments where building semsource from source is too slow or impractical, provide a pre-recorded event fixture (a JSON file of captured `federation.Event` payloads) that a lightweight WebSocket replay server can emit. This gives fast, deterministic CI runs while the real semsource path remains available for local development.

## Data Flow

```
┌─────────────────────────────────────────────────────────────────┐
│ Host Machine (Playwright)                                       │
│                                                                 │
│  Playwright Tests ──► http://localhost:3000                     │
│                                                                 │
├─────────────────────────────────────────────────────────────────┤
│ Docker Network (e2e-net)                                        │
│                                                                 │
│  ┌───────────┐    ws://semsource:7890/graph    ┌────────────┐  │
│  │ semsource │ ──────────────────────────────► │  backend   │  │
│  │           │                                  │ (semstreams)│  │
│  │ ingests   │    WebSocket input               │            │  │
│  │ fixture/  │    ► entity.> subject routing    │ graph-     │  │
│  └───────────┘    ► graph-ingest (Graphable)    │ ingest     │  │
│       │           ► ENTITY_STATES KV            │ ► KV store │  │
│  reads from                                     │            │  │
│  volume mount                                   │  /graphql  │  │
│                                                 └─────┬──────┘  │
│                                                       │         │
│                    ┌──────────┐    ┌──────────┐       │         │
│                    │  Caddy   │ ◄──│    UI    │       │         │
│                    │ :3000    │    │ (Vite)   │◄──────┘         │
│                    └──────────┘    └──────────┘                  │
│                         │                                       │
│                    exposed to host                               │
└─────────────────────────────────────────────────────────────────┘
```

The critical path:

1. Semsource reads fixture files from a volume mount
2. Semsource emits one SEED event per entity via WebSocket on `:7890/graph`
3. The semstreams backend connects to semsource's WebSocket as a client (via WebSocket input in ModeClient)
4. Events land on `entity.>` subject routing; `EventPayload` implements `Graphable`, so graph-ingest handles them directly
5. graph-ingest stores entities in ENTITY_STATES KV bucket
6. Backend serves entities via the existing `/graphql` endpoint (pathSearch query)
7. UI fetches via GraphQL, transforms via `graphTransform.ts`, renders in SigmaCanvas

The UI does NOT connect to semsource directly. All data flows through the existing GraphQL path. No new UI code is needed for semsource integration -- the backend is the integration point.

### Key Design Decision: No Federation Processor

As of semstreams alpha.13, the federation processor has been removed from the pipeline. Semsource entities flow directly into graph-ingest:

- `EventPayload` implements `Graphable` (provides `EntityID()` and `Triples()`)
- graph-ingest processes them like any other Graphable message
- Entity ID format (6-part) provides namespace isolation without merge logic
- Relationships are expressed as triples, not separate Edge structs

**Breaking changes in alpha.13 that semsource must adopt:**

1. **Event carries one Entity, not many** -- one message per entity
2. **Edges removed** -- relationships are triples (semsource already does this)
3. **AdditionalProvenance removed** -- just `Entity.Provenance`
4. **Event-level Provenance removed** -- lives on Entity, Event has `SourceID`

**What stays the same:** `RegisterPayload(domain)`, `EventPayload` wrapper, event types (SEED/DELTA/RETRACT/HEARTBEAT), 6-part entity IDs, `Provenance` struct.

## Infrastructure Changes

### 1. Semsource Dockerfile

Semsource needs a Dockerfile. It does not have one today.

```
# semsource/docker/Dockerfile
FROM golang:1.25-alpine AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /semsource ./cmd/semsource

FROM alpine:3.21
COPY --from=builder /semsource /usr/local/bin/semsource
ENTRYPOINT ["semsource"]
CMD ["run", "--config", "/etc/semsource/config.json"]
```

### 2. E2E Fixture Directory

Create a small, self-contained fixture in this repo:

```
e2e/fixtures/semsource/
├── semsource-e2e.json          # semsource config for E2E
├── src/
│   ├── main.go                 # ~20 lines, imports one package
│   ├── handler.go              # ~30 lines, defines Handler interface + impl
│   └── handler_test.go         # ~15 lines
├── go.mod                      # minimal go.mod
├── README.md                   # short doc
└── Dockerfile                  # minimal Dockerfile
```

This produces a known, stable set of entities:

- **AST entities**: `acme.semsource.code.go.function.main`, `acme.semsource.code.go.type.Handler`, etc.
- **Doc entities**: `acme.semsource.docs.markdown.document.README`
- **Config entities**: `acme.semsource.config.go.module.fixture-project`, `acme.semsource.config.docker.dockerfile.Dockerfile`
- **Relationships**: `imports`, `implements`, `describes`, `defines` (expressed as triples)

Total: roughly 8-15 entities and 10-20 relationship triples. Small enough for fast ingestion (<2 seconds), large enough to test graph rendering, filtering, and selection.

### 3. Semsource E2E Config

```json
{
  "namespace": "e2e",
  "flow": {
    "outputs": [
      {
        "name": "graph_stream",
        "type": "network",
        "subject": "http://0.0.0.0:7890/graph"
      }
    ],
    "delivery_mode": "at-least-once",
    "ack_timeout": "5s"
  },
  "sources": [
    {
      "type": "ast",
      "path": "/data/fixture/src",
      "language": "go",
      "watch": false
    },
    {
      "type": "docs",
      "paths": ["/data/fixture/README.md"],
      "watch": false
    },
    {
      "type": "config",
      "paths": ["/data/fixture/go.mod", "/data/fixture/Dockerfile"],
      "watch": false
    }
  ]
}
```

Key decisions:
- `watch: false` -- ingest once and emit SEED, do not watch for changes (deterministic)
- No `git` source -- avoids needing a real git repo with history
- No `url` source -- avoids network access during tests
- Namespace `e2e` -- distinct from production namespaces

### 4. Backend Flow Config for E2E

The semstreams backend needs a flow config that includes the WebSocket input pointing at semsource. Create `configs/e2e-with-semsource.json` in the semstreams repo (or pass it via environment variable).

This config adds one component to the standard flow -- a WebSocket input that connects to semsource and routes events to `entity.>` subjects for graph-ingest:

```json
{
  "semsource-input": {
    "name": "semsource-input",
    "type": "input",
    "enabled": true,
    "config": {
      "mode": "client",
      "client": {
        "url": "ws://semsource:7890/graph",
        "reconnect": {
          "enabled": true,
          "max_retries": 30,
          "initial_interval": "500ms",
          "max_interval": "5s",
          "multiplier": 1.5
        }
      },
      "ports": {
        "inputs": [],
        "outputs": [
          {
            "name": "ws_data",
            "type": "jetstream",
            "subject": "entity.semsource",
            "stream_name": "ENTITY"
          }
        ]
      }
    }
  }
}
```

Note: The `url` uses `semsource` as the hostname -- this resolves within the Docker network. The output subject `entity.semsource` matches graph-ingest's `entity.>` subscription pattern.

### 5. Docker Compose Changes

Add semsource as an optional service in `docker-compose.e2e.yml`:

```yaml
services:
  # ... existing nats, backend, ui, caddy services ...

  # SemSource (optional, for graph integration E2E tests)
  semsource:
    build:
      context: ${SEMSOURCE_CONTEXT:-../semsource}
      dockerfile: docker/Dockerfile
    container_name: semstreams-ui-e2e-semsource
    volumes:
      - ./e2e/fixtures/semsource:/data/fixture:ro
      - ./e2e/fixtures/semsource/semsource-e2e.json:/etc/semsource/config.json:ro
    networks:
      - e2e-net
    healthcheck:
      test: ["CMD", "wget", "--spider", "-q", "http://localhost:7890/graph"]
      interval: 3s
      timeout: 2s
      retries: 15
    # No port exposure needed -- backend connects internally
```

The backend service needs `depends_on` updated to wait for semsource when running graph E2E tests. Use a Docker Compose profile to keep semsource optional:

```yaml
  semsource:
    profiles: ["semsource"]
    # ... rest of config
```

And conditionally add the dependency in the backend:

```yaml
  backend:
    depends_on:
      nats:
        condition: service_healthy
      semsource:
        condition: service_healthy
        required: false
```

### 6. Taskfile Targets

```yaml
  test:e2e:semsource-graph:
    desc: Run E2E tests with semsource graph integration
    env:
      BACKEND_CONTEXT: ../semstreams
      BACKEND_CONFIG: e2e-with-semsource.json
      SEMSOURCE_CONTEXT: ../semsource
      COMPOSE_PROFILES: semsource
    cmds:
      - npx playwright test e2e/semsource-graph/
```

### 7. Caddy Routing

No changes needed. The `/graphql` route already proxies to the backend. Semsource data flows through the existing GraphQL endpoint after the backend ingests it.

## E2E Test Scenarios

### File Structure

```
e2e/
├── semsource-graph/
│   ├── graph-rendering.spec.ts      # Entities appear in DataView
│   ├── graph-interaction.spec.ts    # Select, hover, expand
│   ├── graph-filtering.spec.ts      # Filter by type/domain
│   └── helpers/
│       └── semsource-helpers.ts     # Wait-for-entity utilities
├── fixtures/
│   └── semsource/
│       ├── semsource-e2e.json
│       ├── src/
│       │   ├── main.go
│       │   ├── handler.go
│       │   └── handler_test.go
│       ├── go.mod
│       ├── README.md
│       └── Dockerfile
```

### Test Helper: Wait for Entities

The key challenge is determinism. Semsource emits events asynchronously, and the backend processes them asynchronously. Tests must wait for entities to appear rather than assuming they exist.

```typescript
// e2e/semsource-graph/helpers/semsource-helpers.ts

import { Page, expect } from "@playwright/test";

/**
 * Known entity IDs from the E2E fixture.
 * These are deterministic because the fixture files are fixed.
 */
export const KNOWN_ENTITIES = {
  mainFunc: "e2e.semsource.code.go.function.main",
  handlerType: "e2e.semsource.code.go.type.Handler",
  readme: "e2e.semsource.docs.markdown.document.README",
  goMod: "e2e.semsource.config.go.module.fixture-project",
} as const;

/**
 * Wait for semsource entities to appear in the DataView.
 * Polls the GraphQL endpoint until at least `minEntities` are returned.
 */
export async function waitForSemsourceEntities(
  page: Page,
  minEntities: number = 3,
  timeout: number = 30000,
): Promise<void> {
  const startTime = Date.now();

  while (Date.now() - startTime < timeout) {
    const response = await page.request.post("/graphql", {
      data: {
        query: `query { pathSearch(startEntity: "*", maxDepth: 2, maxNodes: 50) { entities { id } } }`,
        variables: {},
      },
    });

    if (response.ok()) {
      const body = await response.json();
      const entities = body?.data?.pathSearch?.entities || [];
      const semsourceEntities = entities.filter(
        (e: { id: string }) =>
          e.id.startsWith("e2e.semsource."),
      );

      if (semsourceEntities.length >= minEntities) {
        return;
      }
    }

    await page.waitForTimeout(1000);
  }

  throw new Error(
    `Timed out waiting for semsource entities (waited ${timeout}ms, need ${minEntities})`,
  );
}
```

### Scenario 1: Graph Rendering (`graph-rendering.spec.ts`)

**Purpose**: Verify semsource entities appear in the DataView after ingestion.

Tests:
- Switch to Data view, entities load from GraphQL (semsource entities visible)
- Entity count matches expected range (8-15 entities from fixture)
- Entity types include `function`, `type`, `document`, `module`
- Relationship triples are rendered between entities
- SigmaCanvas renders nodes (check for canvas WebGL context or node count in Sigma)

### Scenario 2: Graph Interaction (`graph-interaction.spec.ts`)

**Purpose**: Verify selection, hover, and expansion work with real semsource entities.

Tests:
- Click a node in SigmaCanvas, GraphDetailPanel shows entity properties
- Entity detail panel shows correct 6-part ID breakdown
- Entity detail panel shows triples/properties from the ingested data
- Hover a node, hover state is visible (highlight)
- Expand a node, new neighbor entities appear

### Scenario 3: Graph Filtering (`graph-filtering.spec.ts`)

**Purpose**: Verify GraphFilters work with semsource entity types and domains.

Tests:
- Type filter dropdown contains semsource entity types (`function`, `type`, `document`, `module`)
- Filtering by type `function` shows only function entities
- Domain filter contains `code`, `docs`, `config` domains
- Filtering by domain `code` hides doc/config entities
- Search by entity name filters the graph
- Reset filters restores all entities

## Determinism Strategy

1. **Fixed fixtures**: The fixture directory is checked into the repo. Entity IDs are deterministic because they derive from file paths, symbol names, and the namespace.

2. **Polling with known IDs**: Tests poll the GraphQL endpoint for known entity IDs rather than assuming entities exist. The `waitForSemsourceEntities` helper abstracts this.

3. **watch: false**: Semsource ingests once and emits one SEED event per entity. No DELTAs or file-watching races.

4. **Generous timeouts**: Allow 30 seconds for entities to appear. The actual time is 2-5 seconds, but Docker startup, backend processing, and JetStream provisioning add latency.

5. **Entity ID assertions by prefix**: Assert that entities with prefix `e2e.semsource.` exist, rather than asserting exact counts (which could change if semsource handler behavior changes slightly).

## CI Considerations

### Option A: Build semsource from source (preferred for correctness)

- Requires Go 1.25 in the CI environment (already needed for semstreams backend)
- Docker builds semsource from `../semsource` context
- Total Docker build time: ~30-60s additional (Go binary is small)
- Total test time: ~45s startup + ~15s test execution
- GitHub Actions: works with standard `docker compose` -- no Docker-in-Docker needed since Playwright runs on the host

### Option B: Pre-recorded event replay (fallback for speed)

If semsource build time is prohibitive or the semsource repo is not available in CI:

- Capture semsource WebSocket output once: `wscat -c ws://localhost:7890/graph > events.jsonl`
- Create a tiny Go or Node.js WebSocket replay server that emits the recorded events
- Use this replay server instead of real semsource in CI
- Tradeoff: does not validate semsource behavior, only validates the UI's ability to render graph data

### GitHub Actions Matrix

```yaml
jobs:
  e2e:
    runs-on: ubuntu-latest
    strategy:
      matrix:
        suite:
          - name: "core"
            command: "npx playwright test --ignore-pattern='semsource-graph/**'"
          - name: "semsource-graph"
            command: "npx playwright test e2e/semsource-graph/"
            env:
              COMPOSE_PROFILES: semsource
              BACKEND_CONFIG: e2e-with-semsource.json
```

This keeps the core E2E suite fast (no semsource dependency) while running semsource integration tests as a separate matrix entry.

## Risks and Mitigations

| Risk | Likelihood | Mitigation |
|------|-----------|------------|
| Semsource handler output format changes break entity ID assertions | Medium | Assert by prefix (`e2e.semsource.`) not exact IDs. Pin semsource version in go.mod. |
| Semsource not updated to alpha.13 breaking changes (single entity per event) | Low | Semsource team confirmed edges-as-triples is already done. Single entity per event is the only change needed. |
| Docker build time too slow for CI | Low | Semsource is a single Go binary, ~20s build. Use Docker layer caching in CI. |
| WebSocket connection race (backend connects before semsource is ready) | Medium | Semsource healthcheck on `:7890/graph`. Backend reconnect config with retries. `depends_on` with health condition. |
| Flaky tests from timing issues | Medium | Polling with generous timeouts. No `waitForTimeout` in assertions -- only in polling loops. |
| Fixture directory becomes stale vs semsource handler expectations | Low | Fixture is 5 simple files. Validate with `semsource validate` in CI before running E2E. |

## Prerequisites Before Implementation

1. **Semsource Dockerfile** -- needs to be created in the semsource repo
2. **Semsource updated to semstreams alpha.13** -- single entity per event (edges-as-triples already done)
3. **Backend flow config** -- `e2e-with-semsource.json` with WebSocket input pointing at semsource, routing to `entity.>` for graph-ingest
4. **Semsource healthcheck endpoint** -- verify `:7890/graph` responds to HTTP GET (or add a `/health` endpoint)

No backend code changes needed. As of semstreams alpha.13:
- `EventPayload` implements `Graphable` -- graph-ingest handles it natively
- No federation processor or bridge processor required
- Entity ID format provides namespace isolation

## Implementation Order

1. Create `e2e/fixtures/semsource/` with fixture files and config
2. Create semsource Dockerfile (PR to semsource repo)
3. Write E2E test specs and helpers (this repo)
4. Create `e2e-with-semsource.json` backend config (simple WebSocket input only)
5. Add semsource service to `docker-compose.e2e.yml` with profile
6. Add Taskfile target `test:e2e:semsource-graph`
7. Wire up CI matrix
8. Run full integration validation
