# SemStreams

> A composable stream processing framework designed to run anywhere.

SemStreams is a flow based framework that turns streaming data into a semantic knowledge graph, runs reactive rules, executes workflows, and orchestrates LLM-powered agents. One binary. NATS as the only dependency. Works offline, syncs when connected.

```
Sensors/Events → Knowledge Graph → Rules, Workflows, Agents → Action
```

**Built for the edge:**

- **Simple deployment** — single binary, ships as a Docker image
- **Progressive AI** — start with rules, add LLMs when you're ready. Or run both: deterministic where it matters, intelligent where it helps
- **Offline-first** — works disconnected, syncs when connectivity allows
- **Edge to cluster** — runs on a Raspberry Pi, scales when needed

## Quick Start

```bash
# Build
task build

# Run tests
task test

# Run with a flow configuration
./bin/semstreams --config configs/protocol-flow.json
```

Run `task --list` to see all available commands.

## How It Works: Continuous Intelligence

SemStreams implements the **OODA loop** — a decision-making cycle from military strategy (Boyd, 1986) that also appears in robotics as Sense-Think-Act:

| OODA | Sense-Think-Act | SemStreams |
|------|-----------------|------------|
| Observe | Sense | **Ingest** — events via UDP, WebSocket, file, API |
| Orient | Think | **Graph** — entities with typed relationships |
| Decide | Act | **React** — rules evaluate conditions |
| Act | Act | **Act** — rules fire, workflows orchestrate, agents reason |

The graph builds situational awareness; rules and agents close the loop.

Two core patterns power this:
- **Graphable** — Your types become graph entities ([docs](docs/basics/03-graphable-interface.md))
- **Payload Registry** — Messages serialize with type discrimination ([docs](docs/concepts/13-payload-registry.md))

## Progressive Capabilities

Start simple, add capabilities as your needs grow:

| Tier | What You Get | What You Need |
|------|--------------|---------------|
| **Structural** | Rules engine, explicit relationships, graph indexing | NATS only |
| **Statistical** | + BM25 search, community detection | + Search index |
| **Semantic** | + Neural embeddings, LLM-powered agents | + Embedding service, LLM |

Most deployments start with Structural. Add capabilities when the problem demands it.

Tiers aren't just about resources. Use rules when you need deterministic, auditable outcomes. Use agents when you need judgment and reasoning. Run both in the same flow — each handles what it does best.

## Architecture

Components connect via NATS subjects in flow-based configurations:

```
Input → Processor → Storage → Graph → Gateway
  │         │          │        │        │
 UDP    iot_sensor  ObjectStore KV+   GraphQL
 File   document    (raw docs)  Indexes  MCP
```

| Component Type | Examples | Role |
|----------------|----------|------|
| Input | UDP, WebSocket, File | Ingest external data |
| Processor | Graph, JSONMap, Rule | Transform and enrich |
| Output | File, HTTPPost, WebSocket | Export data |
| Storage | ObjectStore | Persist to NATS JetStream |
| Gateway | HTTP, GraphQL, MCP | Expose query APIs |

All state lives in NATS JetStream KV buckets—portable, syncable, queryable.

## Agentic AI

When you're ready for LLM-powered automation, SemStreams includes an optional agentic subsystem:

```
                    ┌─────────────────────────────────────────┐
                    │           Agentic Components            │
                    ├─────────────────────────────────────────┤
User Message ───────► agentic-dispatch ─────► agentic-loop   │
                    │       │                      │          │
                    │       │              ┌───────┴───────┐  │
                    │       │              ▼               ▼  │
                    │       │        agentic-model   agentic-tools
                    │       │              │               │  │
                    │       │              ▼               │  │
                    │       │           LLM API    ◄───────┘  │
                    │       │                                 │
                    │       ◄─────── agent.complete.* ────────│
                    └─────────────────────────────────────────┘
```

- **Modular** — 6 components that scale independently
- **OpenAI-compatible** — works with any OpenAI-compatible endpoint
- **Observable** — full trajectory capture for debugging

```bash
# Run agentic e2e tests
task e2e:agentic

# Or start the full agentic stack
./bin/semstreams --config configs/agentic.json
```

See [Agentic Quickstart](docs/basics/07-agentic-quickstart.md) to get started.

## Documentation

| Folder | Purpose |
|--------|---------|
| [docs/basics/](docs/basics/) | Getting started, core interfaces, quickstart guides |
| [docs/concepts/](docs/concepts/) | Background knowledge, algorithms, orchestration layers |
| [docs/advanced/](docs/advanced/) | Agentic components, clustering, performance tuning |
| [docs/operations/](docs/operations/) | Monitoring, troubleshooting, deployment |
| [docs/contributing/](docs/contributing/) | Development, testing, CI |

## Development

```bash
# Testing
task test               # Unit tests
task test:integration   # Integration tests (uses testcontainers)
task test:race          # Tests with race detector
task check              # Lint + test

# E2E Tests (requires Docker)
task e2e:core           # Health + dataflow (~10s)
task e2e:structural     # Rules + structural inference (~30s)
task e2e:statistical    # BM25 + community detection (~60s)
task e2e:semantic       # Neural embeddings + LLM (~90s)
task e2e:agentic        # Agent loop + tools (~30s)
task e2e:all            # All tiers sequentially
```

## Requirements

- Go 1.25+
- NATS Server with JetStream enabled
- Docker (for deployment and e2e tests)
- (Optional) Embedding service for Statistical/Semantic tiers
- (Optional) LLM service for Semantic tier and agentic system

## Status

This project is under active development. Expect breaking changes.

## License

See [LICENSE](LICENSE) for details.
