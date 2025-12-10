# End-to-End Testing

E2E tests validate SemStreams functionality in realistic deployment scenarios using Docker containers and real services.

## Philosophy

E2E tests follow the **Observer Pattern**: they run against real services in Docker containers, not mocks. Tests observe system behavior from the outside, just like production monitoring would.

### Key Principles

1. **Real Services**: Tests use actual NATS, graph processors, and embedding services
2. **Container Isolation**: Each test suite runs in isolated Docker Compose environments
3. **Observable Validation**: Tests query endpoints and KV buckets to verify behavior
4. **Graceful Degradation**: Tests validate fallback behavior when services are unavailable

## Quick Reference

```bash
# Most common commands
task e2e:semantic-indexes   # Fast development test (~5s)
task e2e:semantic           # Full semantic suite (~20s)
task e2e:health             # Core health checks (~3s)
task e2e:clean              # Clean up containers
```

## Test Categories

### Protocol Tests

Core pipeline validation without semantic features.

| Command | Duration | Purpose |
|---------|----------|---------|
| `task e2e:health` | ~3s | Component health checks |
| `task e2e:dataflow` | ~5s | Data pipeline validation |
| `task e2e:federation` | ~7s | Edge-to-cloud federation |
| `task e2e` | ~8s | All protocol tests |

### Semantic Tests

Semantic layer validation with indexing and graph processing.

| Command | Duration | Purpose | Dependencies |
|---------|----------|---------|--------------|
| `task e2e:semantic-basic` | ~5s | Basic entity processing | NATS only |
| `task e2e:semantic-indexes` | ~5s | Core indexing (fast) | NATS only |
| `task e2e:semantic-kitchen-sink` | ~15s | Full stack + metrics | NATS + SemEmbed |
| `task e2e:semantic` | ~20s | All semantic tests | NATS + SemEmbed |

### Tiered Inference Tests

Validation of different inference tiers.

| Command | Duration | Purpose | Dependencies |
|---------|----------|---------|--------------|
| `task e2e:tier0` | ~30s | Rules-only | NATS |
| `task e2e:tier1` | ~60s | BM25 + LPA | NATS |
| `task e2e:tier2` | ~90s | Neural + LLM | Full ML stack |
| `task e2e:tiers` | ~3min | All tiers comparison | Full stack |

## Test Selection Guide

### During Development

Fast feedback for iterative changes:

```bash
task e2e:semantic-indexes  # ~5s, NATS only
```

Use this for:
- Index implementation changes
- Query manager updates
- Entity processing logic

### Pre-Commit

Validate core functionality before committing:

```bash
task e2e
task e2e:semantic-indexes
```

### Pre-Merge

Full validation before merging:

```bash
task e2e:semantic
```

### Release Validation

Complete stack with all dependencies:

```bash
task e2e:tiers
```

## Test Scenarios

### core-health

Validates component availability and health endpoints.

**Coverage**:
- UDP input component
- JSON processors
- Output components (file, HTTP POST, WebSocket)

### core-dataflow

Tests complete data pipeline flow.

**Coverage**:
- UDP input to JSONFilter to JSONMap to File output
- Data transformation validation
- Message delivery confirmation

### semantic-basic

Basic semantic processing validation.

**Coverage**:
- Entity processing pipeline
- Graph processor initialization
- Entity storage in NATS KV

### semantic-indexes

Fast test for core indexing without external dependencies.

**Coverage**:
- Predicate index (entity property lookups)
- Spatial index (geo-location queries)
- Temporal index (time-based queries)
- Alias index (name resolution)
- Incoming/Outgoing indexes (relationship queries)

### semantic-kitchen-sink

Comprehensive stack validation including LLM integration.

**Coverage**:
- All indexes
- Embedding service connectivity
- HTTP Gateway endpoints
- Semantic search
- Prometheus metrics
- Multiple output types

## KV Validation

Tests validate actual data storage, not just component health.

### What Gets Validated

```
Before (Component Health Only):
  ✓ Graph processor healthy
  ✓ 5 entities sent
  Result: PASS

After (With KV Validation):
  ✓ Graph processor healthy
  ✓ 5 entities sent
  ✓ 4 entities found in ENTITY_STATES (80%)
  ✓ Predicate index has entries
  ✓ Spatial index has entries
  Result: PASS with validation: storage_rate=0.80
```

### Validation Thresholds

```go
const (
    DefaultMinStorageRate   = 0.80  // 80% of sent entities must be stored
    DefaultValidationTimeout = 5 * time.Second
)
```

### Index Validation

| Index | Bucket | Validation |
|-------|--------|------------|
| Predicate | `PREDICATE_INDEX` | Has entries after entity processing |
| Spatial | `SPATIAL_INDEX` | Has geohash entries |
| Alias | `ALIAS_INDEX` | Resolves known aliases |
| Incoming | `INCOMING_INDEX` | Tracks inbound references |

## Directory Structure

```
test/e2e/
├── client/
│   ├── observability.go    # HTTP client for component API
│   ├── nats.go             # NATS KV validation
│   └── metrics.go          # Prometheus metrics client
├── config/
│   ├── constants.go        # Test configuration
│   └── validation.go       # Validation thresholds
└── scenarios/
    ├── core_health.go
    ├── core_dataflow.go
    ├── core_federation.go
    ├── semantic_basic.go
    ├── semantic_indexes.go
    └── semantic_kitchen_sink.go

cmd/e2e/
└── main.go                 # Test runner CLI
```

## Docker Compose Files

| File | Purpose |
|------|---------|
| `docker-compose.semantic.yml` | Basic semantic (no embedding) |
| `docker-compose.semantic-kitchen.yml` | Full stack with SemEmbed |
| `docker-compose.rules.yml` | Rules processor testing |

## Running Tests

### Using Task Runner

```bash
# List available e2e tasks
task --list | grep e2e

# Run specific scenario
task e2e:semantic-indexes

# Run with cleanup
task e2e:clean && task e2e:semantic
```

### Direct CLI

```bash
# Build first
task build:e2e

# Run specific scenario
cd cmd/e2e && ./e2e --scenario semantic-indexes

# With verbose output
cd cmd/e2e && ./e2e --scenario semantic-kitchen-sink --verbose
```

### Debug Mode

Leave containers running for inspection:

```bash
task e2e:debug
docker logs -f semstreams-e2e-app
```

## Adding New Scenarios

1. Create scenario file in `test/e2e/scenarios/`:

```go
type MyScenario struct {
    client *client.Client
}

func (s *MyScenario) Name() string {
    return "my-scenario"
}

func (s *MyScenario) Description() string {
    return "Validates X functionality"
}

func (s *MyScenario) Setup(ctx context.Context) error {
    // Initialize clients, prepare test data
    return nil
}

func (s *MyScenario) Execute(ctx context.Context) (*Result, error) {
    // Run test logic, collect metrics
    return &Result{Passed: true, Metrics: map[string]any{}}, nil
}

func (s *MyScenario) Teardown(ctx context.Context) error {
    // Clean up resources
    return nil
}
```

2. Register in `cmd/e2e/main.go`:

```go
func createScenario(name string) (Scenario, error) {
    switch name {
    case "my-scenario":
        return scenarios.NewMyScenario(), nil
    // ...
    }
}
```

3. Add task to `Taskfile.yml`:

```yaml
e2e:my-scenario:
  desc: Run my scenario
  cmds:
    - task: e2e:run
      vars:
        SCENARIO: my-scenario
```

4. Update documentation.

## Troubleshooting

### Port Conflicts

```bash
# Check for port conflicts
task e2e:check-ports

# Kill specific port
lsof -ti:8080 | xargs kill -9

# Clean all containers
task e2e:clean
```

### Container Not Found

This is normal before first run. Containers are created during test execution.

### Services Not Healthy

```bash
# Check container logs
docker logs semstreams-e2e-app

# Check all container status
docker compose -f docker-compose.semantic-kitchen.yml ps
```

### NATS Connection Failed

```bash
# Check NATS is running
docker ps | grep nats

# Check NATS logs
docker logs semstreams-nats
```

### Bucket Does Not Exist

KV buckets are created by graph processor on startup. Wait for initialization (5-10s) or check graph processor logs.

### Storage Rate Below Threshold

Possible causes:
- Graph processor errors (check logs)
- Entity ID format mismatch
- Processing too slow (increase timeout)

### Missing Metrics

Metrics are populated after data processing. Ensure:
- Messages are being sent
- Graph processor is healthy
- Sufficient processing time (5s delay)

### SemEmbed Not Starting

```bash
# Check logs
docker logs semstreams-kitchen-semembed

# Verify port
lsof -i :8081

# Test health
curl http://localhost:8081/health
```

## CI Integration

### PR Checks (Fast)

```yaml
steps:
  - task e2e:health
  - task e2e:semantic-indexes
```

### Main Branch (Full)

```yaml
steps:
  - task e2e:semantic
```

### Release (Complete)

```yaml
steps:
  - task e2e:tiers
```

## External Dependencies

### SemEmbed

Lightweight Rust embedding service using fastembed-rs.

| Property | Value |
|----------|-------|
| Port | 8081 |
| Model | BAAI/bge-small-en-v1.5 |
| API | OpenAI-compatible /v1/embeddings |
| Used by | semantic-kitchen-sink only |

### seminstruct

OpenAI-compatible LLM proxy for summarization.

| Property | Value |
|----------|-------|
| Port | 8083 |
| Backend | semshimmy or OpenAI |
| Used by | tier2, semantic-kitchen-sink |

## Test Output

### Success

```
INFO [E2E] Running scenario: semantic-indexes
INFO Setting up scenario name=semantic-indexes
INFO Executing scenario name=semantic-indexes
INFO Scenario completed successfully duration=3.2s
✅ Semantic scenario PASSED name=semantic-indexes
```

### Failure

```
ERROR Scenario failed error="storage rate 0.60 below threshold 0.80"
ERROR Scenario FAILED name=semantic-basic
```

## Related Documentation

- [Testing Patterns](testing.md) - Unit and integration test patterns
- [Configuration](../reference/configuration.md) - Test environment configuration
- [Configuration](../basics/06-configuration.md) - Configuration capabilities and requirements
