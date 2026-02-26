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

**All CI checks must pass before pushing.** The CI workflow (`.github/workflows/ci.yml`) runs:

1. **Lint** — `go vet`, `go fmt` (must be clean), `revive` (warnings = failure)
2. **Test** — Unit tests with `-race`, integration tests with `-race`
3. **Build** — Cross-compile Linux binary
4. **Schema Validation** — `task schema:generate`, check for uncommitted changes

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

## Payload Registry Pattern

**Critical pattern for polymorphic JSON deserialization.** When adding new message types to the agentic system:

### Registration (in init())

Register payload types in an `init()` function within the package's `payload_registry.go`:

```go
func init() {
    err := component.RegisterPayload(&component.PayloadRegistration{
        Domain:      "agentic",           // Message domain
        Category:    "task",              // Message category
        Version:     "v1",                // Schema version
        Description: "Agent task request",
        Factory:     func() any { return &TaskMessage{} },
    })
    if err != nil {
        panic("failed to register payload: " + err.Error())
    }
}
```

### Required MarshalJSON Method

**Every payload type MUST implement MarshalJSON that wraps in BaseMessage:**

```go
func (t *TaskMessage) MarshalJSON() ([]byte, error) {
    type Alias TaskMessage
    return json.Marshal(&message.BaseMessage{
        Type: message.MessageType{
            Domain:   agentic.Domain,
            Category: agentic.CategoryTask,
            Version:  agentic.SchemaVersion,
        },
        Payload: (*Alias)(t),
    })
}
```

### Common Mistakes

1. **Missing MarshalJSON**: Payload serializes without type wrapper, deserialization fails
2. **Wrong type fields**: Domain/Category/Version don't match registration
3. **Forgotten import**: Package not imported, `init()` never runs

### Debugging Serialization Issues

```go
// Check if payload is registered
payloads := component.GlobalPayloadRegistry().ListPayloads()
for msgType := range payloads {
    fmt.Println(msgType)  // e.g., "agentic.task.v1"
}

// Verify JSON structure
data, _ := json.Marshal(msg)
fmt.Println(string(data))  // Should show {"type":{"domain":"..."},"payload":{...}}
```

See [Payload Registry Guide](docs/concepts/13-payload-registry.md) for complete documentation.
