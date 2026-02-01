# SemStreams Project Context

A stream processor that builds semantic knowledge graphs from event data using NATS JetStream.

## Tech Stack

- Go 1.25 + NATS JetStream (KV, ObjectStore)
- Prometheus (metrics), slog (logging)
- Task (task runner) — run `task --list` for all commands

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
| `component/` | Base component types, lifecycle, ports, schema |
| `message/` | Message types, Graphable interface, Triple |
| `graph/` | Knowledge graph operations, queries |
| `natsclient/` | NATS connection, KV buckets, JetStream |
| `processor/` | Data transformation processors |
| `config/` | Configuration loading and validation |
| `health/` | Health monitoring and status |
| `service/` | Flow service, component orchestration |

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
task lint               # Run linters
task check              # Run lint + test
```

## E2E Tests (Requires Docker)

E2E tests are tiered and require Docker infrastructure:

```bash
task e2e:core           # Health + dataflow (~30s)
task e2e:structural     # Rules + structural inference (~30s)
task e2e:statistical    # BM25 + community detection (~60s)
task e2e:semantic       # Neural embeddings + LLM (~90s)
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

## Orchestration Boundaries

SemStreams uses three orchestration layers. Respecting layer boundaries prevents design debt.

### Layer Summary

| Layer | Purpose | Owns |
|-------|---------|------|
| **Rules** | React to state, fire single actions | Conditions, triggers |
| **Workflow** | Multi-step coordination with limits | Step sequence, loop limits, timeouts |
| **Component** | Execute work | Execution mechanics |

### Rules of Thumb

1. **Rules trigger, they don't orchestrate** — A rule fires one action, not a sequence
2. **Workflows coordinate, they don't execute** — Workflows spawn components, not inline logic
3. **Components are workflow-agnostic** — Components don't know their caller
4. **State ownership is exclusive** — Only one layer owns any state
5. **If it needs a loop limit, it's a workflow** — Simple handoffs use rules; loops use workflows

### Anti-Patterns to Avoid

- Rule chains that build up state across multiple firings
- Workflows with inline processing logic (belongs in components)
- Components checking workflow context to decide behavior
- Both rules and workflows tracking the same state

### Quick Decision Guide

| Pattern | Use |
|---------|-----|
| A completes → B starts (no retry) | Rules |
| A → B → A → B... (max N times) | Workflow |
| Execute LLM call, process tools | Component |

See [Orchestration Layers](docs/concepts/12-orchestration-layers.md) for details.
