# SemStreams E2E Tests

End-to-end tests for validating SemStreams functionality in realistic deployment scenarios.

## Test Philosophy

E2E tests follow the **Observer Pattern**: they run against real services in Docker containers, not mocks. Tests observe system behavior from the outside, just like production monitoring.

## Quick Start

```bash
# 4 E2E tasks - one per tier
task e2e:core        # Platform boots, data flows (~10s)
task e2e:structural  # Rules + structural inference (~30s)
task e2e:statistical # BM25 + community detection (~60s)
task e2e:semantic    # Neural embeddings + LLM (~90s)

# Cleanup
task e2e:clean

# List all e2e tasks
task --list | grep e2e
```

## Test Tiers

### Core (`task e2e:core`)

Platform boots, data flows. Validates basic health and dataflow.

| Duration | Purpose | Dependencies |
|----------|---------|--------------|
| ~10s | Component health + data pipeline | NATS only |

### Structural (`task e2e:structural`)

Rules + structural inference. Deterministic behavior, no embeddings.

| Duration | Purpose | Dependencies |
|----------|---------|--------------|
| ~30s | Stateful rules + PathRAG | NATS only |

### Statistical (`task e2e:statistical`)

BM25 + community detection. No external ML services required.

| Duration | Purpose | Dependencies |
|----------|---------|--------------|
| ~60s | BM25 embeddings + LPA communities | NATS only |

### Semantic (`task e2e:semantic`)

Neural embeddings + LLM. Full ML stack validation.

| Duration | Purpose | Dependencies |
|----------|---------|--------------|
| ~90s | Neural embeddings + LLM summaries | NATS + SemEmbed + SemInstruct |

## Assertion Strategy

| Tier | What We Assert | What We DON'T Assert |
|------|----------------|---------------------|
| **Core** | Health endpoints, data flows | - |
| **Structural** | Entities in KV, predicates indexed, anomaly flags in index, PathRAG edges | LLM response quality |
| **Statistical** | Above + BM25 embeddings, communities detected | LLM summaries |
| **Semantic** | Above + LLM summary quality, semantic search relevance | - |

## Test Scenarios

Detailed documentation for each scenario is available in [test/e2e/docs/](./docs/).

| Scenario | Variant | Tier | Purpose | Doc |
|----------|---------|------|---------|-----|
| core-health | - | Core | Component availability and health endpoints | [docs/core-health.md](./docs/core-health.md) |
| core-dataflow | - | Core | Complete data pipeline: UDP → JSONFilter → JSONMap → File | [docs/core-dataflow.md](./docs/core-dataflow.md) |
| core-federation | - | Core | Edge-to-cloud data flow with ack/nack protocol | [docs/core-federation.md](./docs/core-federation.md) |
| tiered | `structural` | Structural | Rules-only, ZERO embeddings/clusters, OnEnter/OnExit | [docs/tiered.md](./docs/tiered.md) |
| tiered | `statistical` | Statistical | BM25 embeddings, LPA communities, no external ML | [docs/tiered.md](./docs/tiered.md) |
| tiered | `semantic` | Semantic | Neural embeddings + LLM summaries | [docs/tiered.md](./docs/tiered.md) |

## Directory Structure

```
test/e2e/
├── client/
│   ├── observability.go    # HTTP client for component API
│   ├── nats.go             # NATS KV validation
│   └── metrics.go          # Prometheus metrics client
├── config/
│   └── constants.go        # Test configuration
└── scenarios/
    ├── core_health.go
    ├── core_dataflow.go
    ├── core_federation.go
    └── tiered.go           # Structural + Statistical + Semantic (via --variant)

cmd/e2e/
└── main.go                 # Test runner CLI

taskfiles/e2e/
├── common.yml              # Shared tasks (clean, check-ports)
├── core.yml                # Core protocol tests
├── structural.yml          # Structural tier
├── statistical.yml         # Statistical tier
├── semantic.yml            # Semantic tier
└── federation.yml          # Federation tests

docker/compose/
├── e2e.yml                 # Core E2E tests
├── structural.yml          # Structural tier
├── tiered.yml              # Statistical + Semantic (profiles: statistical, semantic)
└── federation.yml          # Edge-to-cloud
```

## Running Tests

### Using Task Runner

```bash
task e2e:core        # Run core tests
task e2e:structural  # Run structural tests
task e2e:clean       # Clean up containers
```

### Direct CLI

```bash
task build:e2e
cd cmd/e2e && ./e2e --list
cd cmd/e2e && ./e2e --scenario tiered --variant structural
cd cmd/e2e && ./e2e --scenario tiered --variant statistical
cd cmd/e2e && ./e2e --scenario tiered --variant semantic
```

## NATS KV Validation

Tests validate actual data storage, not just component health.

### Index Validation by Tier

| Tier | Indexes Validated |
|------|-------------------|
| Structural | ENTITY_STATES, PREDICATE, SPATIAL, TEMPORAL, ALIAS, INCOMING, OUTGOING |
| Statistical | All above + EMBEDDING_INDEX (BM25), COMMUNITY_INDEX |
| Semantic | All above + EMBEDDING_INDEX (neural), enhanced communities |

## External Dependencies

### SemEmbed (Semantic Tier)
- **Port**: 8081
- **Model**: BAAI/bge-small-en-v1.5
- **API**: OpenAI-compatible /v1/embeddings

### SemInstruct (Semantic Tier)
- **Port**: 8083
- **Backend**: shimmy or OpenAI
- **API**: OpenAI-compatible /v1/chat/completions

## Troubleshooting

```bash
task e2e:check-ports              # Check for port conflicts
task e2e:clean                    # Clean up containers
docker logs semstreams-tiered-app # Check app logs
docker logs semstreams-tiered-nats # Check NATS logs
```

## CI Integration

### PR Checks
```yaml
- task e2e:core
- task e2e:structural
```

### Main Branch
```yaml
- task e2e:core
- task e2e:structural
- task e2e:statistical
```

### Release
```yaml
- task e2e:semantic
```

## References

- [E2E Testing Guide](../../docs/contributing/02-e2e-tests.md)
- [Configuration](../../docs/basics/06-configuration.md)
