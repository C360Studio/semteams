# SemStreams E2E Tests

End-to-end tests for validating SemStreams functionality in realistic deployment scenarios.

## Test Philosophy

E2E tests follow the **Observer Pattern**: they run against real services in Docker containers, not mocks. Tests observe system behavior from the outside, just like production monitoring.

## Test Scenarios

### Protocol Layer Tests

#### `core-health`
**Purpose**: Validate core component health and availability
**Duration**: ~2-3 seconds
**Dependencies**: None (basic services only)
**Coverage**:
- UDP input component
- JSON processors (generic, filter, map)
- Output components (file, HTTP POST, WebSocket)

**Usage**:
```bash
task e2e:health
# or
cd cmd/e2e && ./e2e --scenario core-health
```

#### `core-dataflow`
**Purpose**: Test complete data pipeline
**Duration**: ~3-5 seconds
**Dependencies**: None
**Coverage**:
- UDP → JSONFilter → JSONMap → File output
- Data transformation pipeline
- Message delivery validation

**Usage**:
```bash
task e2e:dataflow
```

#### `core-federation`
**Purpose**: Test edge-to-cloud federation
**Duration**: ~5-7 seconds
**Dependencies**: Two SemStreams instances (edge + cloud)
**Coverage**:
- Edge: UDP input → WebSocket output
- Cloud: WebSocket input → File output
- Cross-instance data flow

**Usage**:
```bash
task e2e:federation
```

### IoT Example Tests

#### `iot-sensor-pipeline` 🆕 **NEW**
**Purpose**: Validate complete IoT sensor data pipeline with domain-specific processing
**Duration**: ~5-7 seconds
**Dependencies**: NATS + Graph Processor (auto-started)
**Coverage**:
- JSON sensor readings via UDP input
- IoT processor transformation (JSON → SensorReading Graphable)
- Graph processor storage with semantic triples
- Entity relationship creation (sensor → zone references)
- 6-part federated entity ID generation
- Domain-specific vocabulary predicates

**Why this test exists**: Demonstrates the full pipeline for domain-specific processors that implement the Graphable interface, serving as a reference for building custom processors.

**Usage**:
```bash
task e2e:iot-sensor
# or
cd cmd/e2e && ./e2e --scenario iot-sensor-pipeline
```

**Test Flow**:
1. Sends JSON sensor readings (temperature, humidity, pressure)
2. IoT processor transforms to SensorReading with organizational context
3. Graph processor creates entities with semantic relationships
4. Validates pipeline health and component availability

**Related Integration Tests**: `examples/processors/iot_sensor/integration_test.go`

### Semantic Layer Tests

#### `semantic-basic`
**Purpose**: Validate basic semantic processing
**Duration**: ~3-5 seconds
**Dependencies**: NATS (auto-started)
**Coverage**:
- UDP input → JSONGeneric parser
- Graph processor initialization
- Entity processing pipeline

**Usage**:
```bash
task e2e:semantic-basic
```

#### `semantic-indexes` 🚀 **NEW**
**Purpose**: Fast test for core indexing without external dependencies
**Duration**: ~3-5 seconds (optimized for CI)
**Dependencies**: NATS only (no embedding service, no external services)
**Coverage**:
- Predicate index (entity property indexing)
- Spatial index (geo-location queries)
- Temporal index (time-based queries)
- Alias index (entity name resolution)
- Incoming index (relationship queries)

**Why this test exists**: Provides fast feedback during development and CI without waiting for embedding services.

**Usage**:
```bash
task e2e:semantic-indexes
```

#### `semantic-kitchen-sink` 🔥 **ENHANCED**
**Purpose**: Comprehensive semantic stack validation
**Duration**: ~10-15 seconds
**Dependencies**: NATS + SemEmbed (auto-started via Docker Compose)
**Coverage**:
- All indexes (from semantic-indexes)
- **SemEmbed Embedding Service** (fastembed-rs):
  - HTTP embedder connectivity (port 8081)
  - OpenAI-compatible /v1/embeddings API
  - Embedding generation using BAAI/bge-small-en-v1.5
  - Semantic search queries
  - Result relevance ranking
- **HTTP Gateway**:
  - REST API endpoint validation
  - Semantic search via HTTP POST
  - Entity lookup via HTTP GET
  - Response structure validation
  - Latency measurement
- **Automatic Fallback**:
  - BM25 fallback when embedding service unavailable
  - Degraded mode validation
  - Lexical search continuity
- **Metrics Validation**:
  - Prometheus /metrics endpoint
  - Index operation counters
  - Search query metrics
  - Gateway request/response metrics
  - Cache hit/miss tracking
- **Multiple Outputs**:
  - File, HTTP POST, WebSocket, Object Store

**Usage**:
```bash
task e2e:semantic-kitchen-sink
```

## Test Suites

### Run All Protocol Tests
```bash
task e2e  # Runs core-health + core-dataflow
```

### Run All Semantic Tests
```bash
task e2e:semantic  # Runs semantic-basic + semantic-indexes + semantic-kitchen-sink
```

## When to Use Which Test

### During Development (Fast Feedback)
- `semantic-indexes` - Quick validation of indexing changes (~3-5s)
- `semantic-basic` - Graph processor changes
- `core-health` - Component health checks

### Pre-Commit (Medium Coverage)
- `task e2e` - Protocol layer validation
- `task e2e:semantic-indexes` - Core semantic validation

### Pre-Merge / Release (Full Coverage)
- `task e2e:semantic` - Complete semantic stack
- `task e2e:semantic-kitchen-sink` - SemEmbed integration and metrics

## Architecture

### Test Structure
```
test/e2e/
├── client/               # Observability client for querying SemStreams
├── config/              # Test configuration
├── scenarios/           # Test scenario implementations
│   ├── core_health.go
│   ├── core_dataflow.go
│   ├── core_federation.go
│   ├── semantic_basic.go
│   ├── semantic_indexes.go      # NEW - Fast core indexing tests
│   └── semantic_kitchen_sink.go # ENHANCED - Full semantic + SemEmbed + metrics
└── cmd/e2e/
    └── main.go         # Test runner CLI
```

### Docker Compose Files
```
semstreams/
├── docker-compose.semantic.yml         # Basic semantic (no embedding service)
└── docker-compose.semantic-kitchen.yml # Full semantic + SemEmbed + all outputs
```

## External Dependencies

### SemEmbed (fastembed-rs)
**Used by**: `semantic-kitchen-sink` only
**Port**: 8081
**Default Model**: BAAI/bge-small-en-v1.5
**API**: OpenAI-compatible /v1/embeddings endpoint
**Auto-managed**: Yes (via docker-compose.semantic-kitchen.yml)

**Technology**: Lightweight Rust-based embedding service using fastembed-rs
**Alternative**: TEI (Text Embeddings Inference) can be used as optional replacement

## Test Output

### Success Example
```
INFO [E2E] Running scenario: semantic-indexes
INFO Setting up scenario name=semantic-indexes
INFO Executing scenario name=semantic-indexes
INFO Scenario completed successfully duration=3.2s metrics=map[...]
```

### Failure Example
```
ERROR Scenario failed error="missing required metrics: [indexmanager_events_processed_total]"
ERROR Scenario FAILED name=semantic-kitchen-sink
```

## Troubleshooting

### Port Already in Use
```bash
task e2e:check-ports  # Check for port conflicts
task e2e:clean        # Clean up stale containers
```

### Services Not Healthy
```bash
docker compose -f docker-compose.semantic-kitchen.yml logs
docker compose -f docker-compose.semantic-kitchen.yml ps
```

### SemEmbed Not Starting
```bash
# Check SemEmbed logs
docker logs semstreams-kitchen-semembed

# Verify port 8081 is free
lsof -i :8081

# Manual health check
curl http://localhost:8081/health

# Test embeddings endpoint
curl -X POST http://localhost:8081/v1/embeddings \
  -H "Content-Type: application/json" \
  -d '{"input": "test query", "model": "BAAI/bge-small-en-v1.5"}'
```

### Metrics Missing
Metrics are only populated after data is processed. Ensure:
- Messages are being sent (check test output)
- Graph processor is healthy
- Sufficient processing time (5s delay in tests)

## Development

### Running Individual Tests
```bash
# Build first
task build:e2e

# Run specific scenario
cd cmd/e2e && ./e2e --scenario semantic-indexes
cd cmd/e2e && ./e2e --scenario semantic-kitchen-sink

# With verbose logging
cd cmd/e2e && ./e2e --scenario semantic-kitchen-sink --verbose
```

### Adding New Scenarios

1. Create scenario in `test/e2e/scenarios/`
2. Implement `Scenario` interface:
   ```go
   type Scenario interface {
       Name() string
       Description() string
       Setup(context.Context) error
       Execute(context.Context) (*Result, error)
       Teardown(context.Context) error
   }
   ```
3. Register in `cmd/e2e/main.go` `createScenario()`
4. Add task to `Taskfile.yml`
5. Update this README

## CI Integration

Fast tests run on every PR:
```yaml
- task e2e:health
- task e2e:semantic-indexes
```

Full tests run on main branch:
```yaml
- task e2e:semantic
```

## References

- [Port Allocation](../../docs/PORT_ALLOCATION.md)
- [Optional Services](../../semstreams/docs/OPTIONAL_SERVICES.md)
- [E2E Test Architecture](../../docs/architecture/)
