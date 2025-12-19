# tiered Scenario

## Purpose

The unified e2e test scenario covering all three tiers: Structural, Statistical, and Semantic. This single scenario replaces the previous separate scenarios (`tier0-rules-iot`, `semantic-basic`, `semantic-indexes`, `rules-graph`) with a variant-based approach.

## Three Variants

| Variant | What It Tests | Dependencies | Duration |
|---------|---------------|--------------|----------|
| `structural` | Rules-only, ZERO ML inference | NATS | ~30s |
| `statistical` | BM25 embeddings, communities | NATS | ~60s |
| `semantic` | Neural embeddings + LLM summaries | NATS + SemEmbed + SemInstruct | ~90s |

## Invocation

```bash
# Structural tier (rules-only, no embeddings)
task e2e:structural
./e2e --scenario tiered --variant structural

# Statistical tier (BM25, no external ML)
task e2e:statistical  
./e2e --scenario tiered --variant statistical

# Semantic tier (Neural embeddings + LLM)
task e2e:semantic
./e2e --scenario tiered --variant semantic
```

## Variant Details

### Structural Variant (`--variant structural`)

**Purpose**: Validate rules-only processing with ZERO ML inference.

**Key Assertions**:
- `ExpectedEmbeddings = 0` - No embeddings generated
- `ExpectedClusters = 0` - No clustering occurred
- OnEnter/OnExit rule transitions fire correctly
- Dynamic graph modifications (add_triple/remove_triple)

**Stages** (structural-only):
1. `validate-zero-embeddings` - Asserts embedding count = 0
2. `validate-zero-clusters` - Asserts clustering runs = 0
3. `validate-rule-transitions` - Validates OnEnter/OnExit state changes

**Use Case**: CI fast-path, deterministic validation, rules engine testing.

### Statistical Variant (`--variant statistical`)

**Purpose**: Validate BM25 lexical embeddings and community detection without external ML services.

**Key Assertions**:
- BM25 embeddings generated
- Communities detected via LPA
- All 7 indexes populated
- Search returns results

**Stages**: Full 16-stage validation minus LLM-specific checks.

**Use Case**: CI comprehensive, local development, no GPU required.

### Semantic Variant (`--variant semantic`)

**Purpose**: Full ML stack validation with neural embeddings and LLM summaries.

**Key Assertions**:
- Neural embeddings via SemEmbed
- LLM-enhanced community summaries
- Semantic search quality
- All 7 indexes populated

**Stages**: All 16 stages including LLM comparison.

**Use Case**: Release validation, full ML pipeline testing.

## Stage Matrix

| Stage | Structural | Statistical | Semantic |
|-------|------------|-------------|----------|
| verify-components | ✓ | ✓ | ✓ |
| send-mixed-data | ✓ | ✓ | ✓ |
| validate-processing | ✓ | ✓ | ✓ |
| verify-entity-count | ✓ | ✓ | ✓ |
| verify-entity-retrieval | ✓ | ✓ | ✓ |
| validate-entity-structure | ✓ | ✓ | ✓ |
| verify-index-population | ✓ | ✓ | ✓ |
| **validate-zero-embeddings** | ✓ | - | - |
| **validate-zero-clusters** | ✓ | - | - |
| **validate-rule-transitions** | ✓ | - | - |
| test-semantic-search | - | ✓ | ✓ |
| verify-search-quality | - | ✓ | ✓ |
| compare-statistical-semantic | - | ✓ | ✓ |
| **compare-communities** | - | - | ✓ |
| test-http-gateway | - | ✓ | ✓ |
| test-embedding-fallback | - | ✓ | ✓ |
| validate-rules | ✓ | ✓ | ✓ |
| validate-metrics | ✓ | ✓ | ✓ |
| verify-outputs | ✓ | ✓ | ✓ |

## Configuration

```go
type TieredConfig struct {
    Variant              string        // "structural", "statistical", "semantic"
    MessageCount         int           // Default: 20
    MessageInterval      time.Duration // Default: 50ms
    ValidationTimeout    time.Duration // Default: 30s
    PollInterval         time.Duration // Default: 100ms
    MinProcessed         int           // Default: 10
    MinExpectedEntities  int           // Default: 50
    
    // Structural tier constraints
    ExpectedEmbeddings   int           // Default: 0 (structural)
    ExpectedClusters     int           // Default: 0 (structural)
    MinRulesEvaluated    int           // Default: 5
    
    // URLs
    NatsURL              string        // Default: nats://localhost:4222
    MetricsURL           string        // Default: http://localhost:9090
    GatewayURL           string        // Default: http://localhost:8080/api-gateway
    OutputDir            string        // Default: test/e2e/results
}
```

## Assertions by Variant

### Structural Assertions (ZERO Constraints)

| Assertion | Description | Failure Mode |
|-----------|-------------|--------------|
| Zero embeddings | `embedding_requests_total = 0` | Hard failure |
| Zero clusters | `clustering_runs_total = 0` | Hard failure |
| OnEnter fired | At least 2 state entries | Warning |
| OnExit fired | At least 1 state exit | Warning |
| Rules evaluated | At least 5 evaluations | Warning |

### Statistical/Semantic Assertions

| Assertion | Description | Failure Mode |
|-----------|-------------|--------------|
| Entity count ≥ 50 | Entities indexed | Configurable |
| Required indexes | 5 core indexes populated | Strong |
| Required metrics | Prometheus metrics present | Strong |
| HTTP Gateway | 200 OK response | Medium |
| Rule metrics | Evaluations occurred | Medium |

## Backwards Compatibility

Legacy variant names are supported:
- `--variant core` → maps to `statistical`
- `--variant ml` → maps to `semantic`

## Output Files

Results saved to `OutputDir`:
- `comparison-structural-{timestamp}.json`
- `comparison-statistical-{timestamp}.json`
- `comparison-semantic-{timestamp}.json`

## Docker Compose Profiles

| Variant | Compose Profile | Command |
|---------|-----------------|---------|
| structural | `rules.yml` | `docker compose -f docker/compose/rules.yml` |
| statistical | `tiered.yml --profile statistical` | `docker compose -f docker/compose/tiered.yml --profile statistical` |
| semantic | `tiered.yml --profile semantic` | `docker compose -f docker/compose/tiered.yml --profile semantic` |

## Example Output

### Structural Variant
```
[tiered] Variant: structural
[tiered] Stage 1/12: verify-components (45ms)
[tiered] Stage 2/12: send-mixed-data (1.1s)
[tiered] Stage 3/12: validate-processing (2.0s)
[tiered] Stage 4/12: validate-zero-embeddings (23ms)
[tiered] ZERO embeddings confirmed (0 requests)
[tiered] Stage 5/12: validate-zero-clusters (18ms)
[tiered] ZERO clusters confirmed (0 runs)
[tiered] Stage 6/12: validate-rule-transitions (156ms)
[tiered] OnEnter: 3, OnExit: 2
[tiered] SUCCESS (Duration: 8.2s)
```

### Statistical Variant
```
[tiered] Variant: statistical (BM25 embeddings)
[tiered] Stage 1/16: verify-components (52ms)
...
[tiered] Stage 8/16: test-semantic-search (3.1s)
[tiered] BM25 fallback active
[tiered] SUCCESS (Duration: 45.3s)
```

### Semantic Variant
```
[tiered] Variant: semantic (neural embeddings)
[tiered] Stage 1/16: verify-components (48ms)
...
[tiered] Stage 8/16: test-semantic-search (4.2s)
[tiered] SemEmbed active: BAAI/bge-small-en-v1.5
[tiered] Stage 11/16: compare-communities (1.8s)
[tiered] LLM summaries detected: 5 communities
[tiered] SUCCESS (Duration: 72.1s)
```

## Related Files

- `test/e2e/scenarios/tiered.go` - Scenario implementation
- `test/e2e/client/metrics.go` - Metrics client with wait functions
- `test/e2e/client/tracer.go` - FlowTracer for message validation
- `test/e2e/client/nats.go` - NATS validation client
- `docker/compose/tiered.yml` - Docker Compose configuration
- `docker/compose/rules.yml` - Rules-only Docker Compose
