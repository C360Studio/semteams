# SemStreams Project Context

A stream processor that builds semantic knowledge graphs from event data using NATS JetStream.

## Tech Stack

- Go 1.25 + NATS JetStream (KV, ObjectStore)
- Prometheus (metrics), slog (logging)
- Task (task runner) — run `task --list` for all commands
- `ui/` — Svelte 5 + SvelteKit 2 + TypeScript frontend (subtree-imported from
  semstreams-ui on 2026-04-10, see `ui/.claude/CLAUDE.md` for UI conventions)

## Architecture

```
Events → Graphable Interface → Knowledge Graph → Queries
```

Flow-based component architecture:
- **Input**: UDP, WebSocket, File — ingest external data
- **Processor**: Graph, JSONMap, Rule — transform and enrich
- **Output**: File, HTTPPost, WebSocket — export data
- **Storage**: ObjectStore — persist to NATS JetStream
- **Gateway**: HTTP, GraphQL, MCP — expose query APIs

## Key Packages

| Package | Purpose |
|---------|---------|
| `component/` | Base component types, lifecycle, ports, schema, payload registry |
| `message/` | Message types, Graphable interface, Triple, BaseMessage |
| `graph/` | Knowledge graph operations, queries |
| `natsclient/` | NATS connection, KV buckets, JetStream |
| `processor/` | Data transformation processors |
| `config/` | Configuration loading and validation |
| `health/` | Health monitoring and status |
| `service/` | Flow service, component orchestration |
| `agentic/` | Agentic types, payload registrations, state machine |
| `processor/agentic-loop/` | Loop orchestrator, state machine, trajectory |
| `processor/agentic-model/` | LLM endpoint caller, retry logic |
| `processor/agentic-tools/` | Tool dispatch, executor registry |
| `processor/agentic-dispatch/` | User message routing, commands |
| `processor/agentic-memory/` | Graph-backed persistent memory |
| `processor/agentic-governance/` | PII filtering, rate limiting, content governance |
| `ui/` | Svelte 5 + SvelteKit 2 frontend (graph explorer, flow builder, agentic UI) |

## Core Interface

Domain types implement `Graphable` to become graph entities:

```go
type Graphable interface {
    EntityID() string          // 6-part federated identifier
    Triples() []message.Triple // Facts about this entity
}
```

## Entity ID Format

6-part hierarchical: `org.platform.domain.system.type.instance`

Example: `acme.ops.robotics.gcs.drone.001`

## Common Tasks

```bash
task build              # Build binary
task test               # Run unit tests
task test:integration   # Run integration tests (uses testcontainers)
task test:race          # Run tests with race detector
task lint               # Run Go linters
task check              # Go lint + test (fast, no Node required)
task check:all          # Go + UI lint + type-check + test (requires Node)

# UI tasks (frontend) — see ui/Taskfile.yml for the full list
task ui:dev             # Start Vite dev server
task ui:test            # Run Vitest unit/component tests
task ui:test:e2e        # Run Playwright E2E tests
task ui:lint            # Run ESLint on ui/
task ui:check           # Run svelte-check (TypeScript)
task ui:build           # Production build
```

**Note:** `task check` stays Go-only so backend iteration doesn't require Node.
`task check:all` runs both and is the pre-push verification target for changes
touching `ui/`. The two workflows (`.github/workflows/ci.yml` for Go,
`.github/workflows/ui.yml` for UI) run independently via path filters.

## E2E Tests (Requires Docker)

E2E tests are tiered and require Docker infrastructure:

```bash
task e2e:core           # Health + dataflow (~10s)
task e2e:structural     # Rules + structural inference (~30s)
task e2e:statistical    # BM25 + community detection (~60s)
task e2e:semantic       # Neural embeddings + LLM (~90s)
task e2e:agentic        # Agent loop + tools (~30s)
task e2e:all            # Run all tiers sequentially
```

**Agent guidance**: E2E tests require Docker and take significant time. For TDD workflows:
- Use `task test` and `task test:integration` for rapid feedback
- E2E tests are for final validation, not iterative development
- If e2e fails, check `task e2e:check-ports` for port conflicts

## Testing Patterns

- Unit tests: Standard `*_test.go` files
- Integration tests: `//go:build integration` tag, uses testcontainers
- E2E tests: Full Docker stack, tiered by capability
- Always run with `-race` flag for concurrency checks

## CI Requirements (IMPORTANT)

**All CI checks must pass before pushing.** Two workflows run:

**`.github/workflows/ci.yml`** (Go backend):
1. **Lint** — `go vet`, `go fmt` (must be clean), `revive` (warnings = failure)
2. **Test** — Unit tests with `-race`, integration tests with `-race`
3. **Build** — Cross-compile Linux binary
4. **Schema Validation** — `task schema:generate`, check for uncommitted changes

**`.github/workflows/ui.yml`** (Svelte frontend, path-filtered to `ui/**`):
1. **Lint** — `npm run lint` (ESLint)
2. **Type Check** — `npm run check` (svelte-check)
3. **Unit Tests** — `npm run test:unit`
4. **Build** — `npm run build`

Before pushing, run these locally:

```bash
task lint                    # Must pass with no warnings (revive warnings = CI fail)
go test -race ./...          # Unit tests with race detector
task schema:generate         # Generate schemas
git diff schemas/ specs/     # Must show no changes (commit if there are)
go test ./test/contract/...  # Contract tests
```

**Common CI failures:**
- Revive lint warnings (fix all warnings, they indicate potential issues)
- Uncommitted schema changes after `task schema:generate`
- Race conditions detected in tests
- Unformatted code (`go fmt` not run)

## Architectural Identity (Not an Event Bus)

SemStreams is NOT a simple event bus or pub/sub framework. It is a knowledge graph engine where the communication model is a consequence of the data model.

### The KV Twofer

Every NATS KV bucket gives you three interfaces from one write:

- **State**: `kv.Get(key)` — current value, right now
- **Events**: `kv.Watch(pattern)` — fires on every change (fan-out to all watchers)
- **History**: Replay from any revision — audit trail at no extra cost

**The write IS the event.** No separate event bus. No dual-write problem. Internal processors react to state changes via KV watch, not pub/sub topics. See [KV Twofer](docs/concepts/02-kv-twofer.md).

### Facts vs Requests

| Communication type | Primitive | Restart behavior |
|---|---|---|
| Fact about the world (entity state, index, current status) | KV Watch | Re-delivers all current values (correct recovery) |
| Request to do something (task, LLM call, tool execution) | JetStream Stream | Resumes from last ack (no re-execution) |

Use `/kv-or-stream` for the full 4-test decision heuristic. See [Streams vs KV Watches](docs/concepts/03-streams-vs-kv-watches.md).

### Inference Tiers

| Tier | Method | Requires |
|------|--------|----------|
| 0 | Explicit triples + rules only | Nothing extra |
| 1 | + BM25 statistical embeddings | Text content (pure Go) |
| 2 | + Neural semantic embeddings | Text + external embedding service |

Tiers only affect entities with text content. Telemetry-only entities cluster via explicit relationships regardless of tier. See [Real-Time Inference](docs/concepts/00-real-time-inference.md).

## Orchestration Boundaries

Two layers: **Reactive Engine** (conditions + actions + workflows) and **Components** (execute work).

| Pattern | Use |
|---------|-----|
| A completes → B starts (no retry) | Reactive rule (single trigger) |
| A → B → A → B... (max N times) | Reactive workflow (loop limits, timeouts) |
| Execute LLM call, process tools | Component |

**Key rules**: Rules trigger, they don't orchestrate. Workflows coordinate, they don't execute. Components are workflow-agnostic. State ownership is exclusive.

Use `/orchestration-check` for the full decision framework. See [Orchestration Layers](docs/concepts/14-orchestration-layers.md).

## Payload Registry

Polymorphic JSON deserialization via type-discriminated envelopes. Every new message type needs:

1. `init()` registration in `payload_registry.go` with domain/category/version/factory
2. `MarshalJSON` method wrapping payload in `BaseMessage` (use type alias to avoid recursion)
3. Package import (blank import if needed) so `init()` runs

Use `/new-payload` for the step-by-step checklist with code templates. See [Payload Registry Guide](docs/concepts/15-payload-registry.md).

