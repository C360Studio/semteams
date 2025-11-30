# Data Model: E2E Test Suite Improvements

**Feature**: 007-e2e-test-improvements
**Date**: 2025-11-30

## Entities

### NATSValidationClient

A thin wrapper around `natsclient.Client` providing convenience methods for E2E test validation.

```go
// NATSValidationClient wraps natsclient.Client for E2E test validation
type NATSValidationClient struct {
    client *natsclient.Client
}
```

**Fields**:
| Field | Type | Description |
|-------|------|-------------|
| client | *natsclient.Client | Existing NATS client (reused) |

**Methods**:
| Method | Parameters | Returns | Description |
|--------|------------|---------|-------------|
| NewNATSValidationClient | natsURL string | (*NATSValidationClient, error) | Create client using natsclient.NewClient() |
| Close | ctx | error | Delegates to natsclient.Client.Close() |
| GetEntity | ctx, entityID | (*EntityState, error) | Get entity from ENTITY_STATES via GetKeyValueBucket() |
| CountEntities | ctx | (int, error) | Count keys in ENTITY_STATES bucket |
| ValidateIndexPopulated | ctx, indexName | (bool, error) | Check if index bucket has entries |
| BucketExists | ctx, bucketName | (bool, error) | Try GetKeyValueBucket, check error |

**Underlying natsclient.Client Methods Used**:
- `Connect(ctx)` - Establish connection
- `GetKeyValueBucket(ctx, name)` - Get KV bucket handle
- `ListKeyValueBuckets(ctx)` - List all buckets
- `Close(ctx)` - Clean shutdown

### ValidationResult

Enhanced test result with quantitative metrics.

```go
// ValidationResult contains NATS KV validation outcomes
type ValidationResult struct {
    EntitiesSent     int      // Count of entities sent via UDP
    EntitiesStored   int      // Count of entities found in NATS KV
    StorageRate      float64  // EntitiesStored / EntitiesSent
    IndexesChecked   []string // Names of indexes validated
    IndexesPopulated int      // Count of indexes with entries
    MetricsVerified  []string // Names of metrics found
    MetricsMissing   []string // Names of expected metrics not found
}
```

**Fields**:
| Field | Type | Description |
|-------|------|-------------|
| EntitiesSent | int | Total entities sent through pipeline |
| EntitiesStored | int | Entities found in ENTITY_STATES bucket |
| StorageRate | float64 | Ratio of stored to sent (0.0-1.0) |
| IndexesChecked | []string | Which indexes were validated |
| IndexesPopulated | int | How many indexes had entries |
| MetricsVerified | []string | Prometheus metrics found |
| MetricsMissing | []string | Expected metrics not found |

### ValidationConfig

Configurable thresholds for test validation.

```go
// ValidationConfig holds configurable validation thresholds
type ValidationConfig struct {
    MinStorageRate    float64  // Minimum entity storage rate (default: 0.80)
    RequiredIndexes   []string // Indexes that must have entries
    RequiredMetrics   []string // Metrics that must be present
    ValidationTimeout time.Duration // Max time to wait for processing
}
```

**Fields**:
| Field | Type | Default | Description |
|-------|------|---------|-------------|
| MinStorageRate | float64 | 0.80 | Fail if storage rate below this |
| RequiredIndexes | []string | ["predicate", "spatial", "alias"] | Must have entries |
| RequiredMetrics | []string | (see research.md) | Must be present |
| ValidationTimeout | time.Duration | 5s | Max wait for processing |

## Relationships

```text
┌─────────────┐     uses      ┌────────────────┐
│ E2E Scenario│──────────────▶│  NATSClient    │
└─────────────┘               └────────────────┘
       │                              │
       │ produces                     │ queries
       ▼                              ▼
┌─────────────────┐           ┌──────────────────┐
│ValidationResult │           │ NATS KV Buckets  │
└─────────────────┘           │ - ENTITY_STATES  │
       │                      │ - PREDICATE_INDEX│
       │ compared against     │ - SPATIAL_INDEX  │
       ▼                      │ - ALIAS_INDEX    │
┌─────────────────┐           └──────────────────┘
│ValidationConfig │
└─────────────────┘
```

## State Transitions

### NATSClient Lifecycle

```text
[not_created] ──NewNATSClient()──▶ [connected]
                                        │
                                        │ Close()
                                        ▼
                                   [closed]
```

### Validation Flow

```text
[send_data] ──UDP messages──▶ [wait_processing]
                                    │
                                    │ ValidationTimeout
                                    ▼
                              [query_nats_kv]
                                    │
                                    │ GetEntity/CountEntities
                                    ▼
                              [compare_results]
                                    │
                    ┌───────────────┴───────────────┐
                    │                               │
            StorageRate >= Min           StorageRate < Min
                    │                               │
                    ▼                               ▼
                 [PASS]                          [FAIL]
```

## Validation Rules

### Entity Storage Validation

1. **Minimum Storage Rate**: `EntitiesStored / EntitiesSent >= MinStorageRate`
   - Default threshold: 80%
   - Accounts for UDP packet loss
   - Configurable per scenario

2. **Entity ID Format**: Entity IDs in test data must match expected format
   - Pattern: `{org}.{platform}.{domain}.{context}.{type}.{id}`
   - Example: `c360.platform1.iot.warehouse7.sensor.001`

### Index Validation

1. **Index Existence**: Each required index bucket must exist
2. **Index Population**: At least one entry in each required index
3. **Index Types**:
   - Predicate: Check for known property key
   - Spatial: Check for geo-location entry
   - Alias: Check for entity name mapping

### Metrics Validation

1. **Metric Presence**: All required metrics must be in /metrics output
2. **Non-Zero Values**: Counter metrics should have value > 0 after processing
3. **Metric Names**: Use exact Prometheus metric names from codebase

## NATS KV Bucket Schema

### ENTITY_STATES Bucket

**Key Format**: `{6-part-entity-id}`
**Value Format**: JSON EntityState

```json
{
  "id": "c360.platform1.iot.warehouse7.sensor.001",
  "type": "iot:Sensor",
  "properties": {
    "temperature": 23.5,
    "humidity": 65.0
  },
  "triples": [...],
  "updated_at": "2025-11-30T12:00:00Z"
}
```

### PREDICATE_INDEX Bucket

**Key Format**: `{predicate}`
**Value Format**: JSON array of entity IDs

```json
["c360.platform1.iot.warehouse7.sensor.001", "c360.platform1.iot.warehouse7.sensor.002"]
```

### SPATIAL_INDEX Bucket

**Key Format**: `{geohash}`
**Value Format**: JSON array of entity IDs

### ALIAS_INDEX Bucket

**Key Format**: `{alias}`
**Value Format**: Entity ID string
