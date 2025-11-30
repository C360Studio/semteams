# Quickstart: E2E Test Suite Improvements

**Feature**: 007-e2e-test-improvements
**Date**: 2025-11-30

## Overview

This feature enhances E2E tests to validate actual data storage in NATS KV, not just component health status.

## Key Changes

1. **NATS Validation Client** (`test/e2e/client/nats.go`)
   - Thin wrapper around existing `natsclient.Client`
   - Convenience methods for E2E validation (CountEntities, ValidateIndexPopulated)
   - Reuses production NATS connection logic (circuit breaker, reconnection)

2. **Enhanced Scenarios**
   - `semantic_basic.go` - Now verifies entities in ENTITY_STATES
   - `semantic_indexes.go` - Now verifies index population
   - `semantic_kitchen_sink.go` - Updated metrics list

3. **Terminology Cleanup**
   - All "StreamKit" references replaced with "SemStreams"

## Running Tests

### Prerequisites

```bash
# Build E2E binary
task build:e2e

# Ensure Docker is running
docker info
```

### Run Individual Scenarios

```bash
# Basic semantic test (now with KV validation)
task e2e:semantic-basic

# Index validation test
task e2e:semantic-indexes

# Full stack test
task e2e:semantic-kitchen-sink
```

### Run All Semantic Tests

```bash
task e2e:semantic
```

## What Gets Validated

### Before (Component Health Only)
```
✓ Graph processor healthy
✓ UDP input healthy
✓ 5 entities sent
Result: PASS
```

### After (With KV Validation)
```
✓ Graph processor healthy
✓ UDP input healthy
✓ 5 entities sent
✓ 4 entities found in ENTITY_STATES (80%)
✓ Predicate index has entries
✓ Spatial index has entries
✓ Alias index has entries
✓ indexmanager_events_processed metric present
Result: PASS with validation: storage_rate=0.80, indexes_populated=3
```

## Configuration

### Validation Thresholds

Edit `test/e2e/config/constants.go`:

```go
const (
    // Minimum percentage of sent entities that must be stored
    DefaultMinStorageRate = 0.80

    // Timeout waiting for NATS processing
    DefaultValidationTimeout = 5 * time.Second
)
```

### Required Metrics

The metrics list is defined in `test/e2e/scenarios/semantic_kitchen_sink.go`:

```go
requiredMetrics := []string{
    "indexmanager_events_processed",
    "indexmanager_index_updates_total",
    "semstreams_cache_hits_total",
    "semstreams_cache_misses_total",
    "semstreams_json_filter_matched_total",
}
```

## Troubleshooting

### "NATS connection failed"

```bash
# Check if NATS is running
docker ps | grep nats

# Check NATS logs
docker logs semstreams-nats
```

### "Bucket does not exist"

The NATS KV buckets are created by the graph processor on startup. Ensure:
1. Graph processor is enabled in config
2. Graph processor has started successfully
3. Wait for initialization (5-10 seconds)

### "Storage rate below threshold"

If entity storage rate is below 80%:
1. Check graph processor logs for errors
2. Verify entity ID format matches expected pattern
3. Check NATS connection from within Docker network
4. Increase validation timeout if processing is slow

## Development

### Adding New Validation

To add validation to a scenario:

```go
// In scenario Execute() method:

// 1. Create NATS validation client (wraps natsclient.Client)
natsClient, err := client.NewNATSValidationClient(natsURL)
if err != nil {
    result.Errors = append(result.Errors, "NATS connection failed")
    return result, nil
}
defer natsClient.Close(ctx)

// 2. Query entity count (uses natsclient.GetKeyValueBucket internally)
stored, err := natsClient.CountEntities(ctx)
if err != nil {
    result.Warnings = append(result.Warnings, "Could not count entities")
}

// 3. Calculate storage rate
rate := float64(stored) / float64(sent)
result.Metrics["storage_rate"] = rate

// 4. Validate threshold
if rate < config.MinStorageRate {
    result.Errors = append(result.Errors,
        fmt.Sprintf("Storage rate %.2f below threshold %.2f", rate, config.MinStorageRate))
}
```

### Testing the Validation Client

```bash
# Run validation client unit tests
go test ./test/e2e/client/... -v

# Run with race detection
go test ./test/e2e/client/... -race
```
