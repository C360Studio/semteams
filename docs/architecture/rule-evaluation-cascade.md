# Rule Evaluation Cascade Issue

**Status**: Investigation Complete
**Date**: 2026-01-05
**Severity**: Performance

## Problem Statement

During E2E structural tier testing, we observed runaway rule evaluations:

| Metric | Observed | Expected |
|--------|----------|----------|
| Rule evaluations | 62,367 | ~500 |
| Entity updates | 13,599 | ~127 |
| Stabilization | Timeout (60s) | < 5s |
| Rule triggers | 0 | 3+ |

The rule processor never stabilizes, consuming excessive CPU while failing to trigger any rules.

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         Entity Processing Flow                               │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  ┌──────────────┐     ┌─────────────────┐     ┌─────────────────────────┐   │
│  │ File Input   │────▶│ Graph Processor │────▶│ ENTITY_STATES KV Bucket │   │
│  │ (sensors.jsonl)   │ (entity creation)│     │                         │   │
│  └──────────────┘     └─────────────────┘     └───────────┬─────────────┘   │
│                                                           │                  │
│                              ┌────────────────────────────┤                  │
│                              │                            │                  │
│                              ▼                            ▼                  │
│  ┌───────────────────────────────────┐    ┌───────────────────────────────┐ │
│  │     Hierarchy Inference           │    │     Rule Processor            │ │
│  │     (KV Watcher: WatchAll)        │    │     (KV Watcher: c360.>)      │ │
│  │                                   │    │                               │ │
│  │  OnEntityCreated():               │    │  Entity Watcher:              │ │
│  │  - Creates container entities     │◀───│  - Adds to Coalescer          │ │
│  │  - Adds hierarchy triples         │    │  - Evaluates all rules        │ │
│  │                                   │    │                               │ │
│  └───────────────────────────────────┘    └───────────────────────────────┘ │
│           │                                         ▲                        │
│           │                                         │                        │
│           └─────────Writes trigger re-evaluation────┘                        │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

## Root Causes

### 1. Hierarchy Inference Processes Its Own Containers

**File**: `processor/graph/inference/hierarchy.go:143-153`

The `OnEntityCreated()` function only validates that an entity has 6 parts:

```go
parts := strings.Split(entityID, ".")
if len(parts) != 6 {
    return nil  // Skip non-6-part entities
}
```

Container entities ARE 6-part IDs:
- Type: `org.platform.domain.system.type.group`
- System: `org.platform.domain.system.group.container`
- Domain: `org.platform.domain.group.container.level`

This creates a cascade:

```
Source entity: c360.logistics.environmental.sensor.temperature.temp-001
  │
  ├─▶ Creates: c360.logistics.environmental.sensor.temperature.group
  │      │
  │      ├─▶ Processed (6 parts!), creates system container
  │      │      │
  │      │      └─▶ Creates: c360.logistics.environmental.sensor.group.container
  │      │             │
  │      │             └─▶ Processed (6 parts!), cascades further...
  │      │
  │      └─▶ Creates domain container...
  │
  ├─▶ Creates: c360.logistics.environmental.sensor.group.container
  │      └─▶ Also processed (6 parts!)...
  │
  └─▶ Creates: c360.logistics.environmental.group.container.level
         └─▶ Also processed (6 parts!)...
```

**Impact**: Each of 73 source entities triggers multiple rounds of container creation and processing.

### 2. 1 Nanosecond Debounce Ticker

**File**: `pkg/cache/coalescing_set.go:35-38`

```go
tickerDuration := window
if tickerDuration <= 0 {
    tickerDuration = 1 * time.Nanosecond  // Fires constantly!
}
c.ticker = time.NewTicker(tickerDuration)
```

With `DebounceDelayMs=0` (production default per `processor/rule/config.go:116`), the coalescer fires every nanosecond. This prevents any batching of entity evaluations.

**Impact**: Instead of batching rapid KV writes into single evaluation, every write immediately triggers evaluation.

### 3. Rule Processor Watches All Entities

**File**: `configs/structural.json`

```json
"entity_watch_patterns": ["c360.>"]
```

This pattern matches ALL entities including hierarchy-generated containers. Combined with the cascade above, the rule processor sees thousands of entity updates and evaluates rules for each.

**Impact**: 4 rules × ~15,000 entity evaluations = ~60,000 rule evaluations.

## Missing Test Coverage

**File**: `processor/rule/entity_watcher_integration_test.go`

Current test uses 100ms debounce (line 80):

```go
config.DebounceDelayMs = 100 * time.Millisecond
```

Missing test scenarios:
1. Debounce=0 behavior (production default)
2. Hierarchy inference + rule processor interaction
3. Assertion that evaluation counts stay bounded

## Proposed Fixes

### Fix 1: Guard Against Container Entities in Hierarchy Inference

Add check in `OnEntityCreated()` to skip entities that are already containers:

```go
// Skip container entities to prevent cascade
if isContainerEntity(entityID) {
    return nil
}

func isContainerEntity(entityID string) bool {
    return strings.HasSuffix(entityID, ".group") ||
           strings.HasSuffix(entityID, ".container") ||
           strings.HasSuffix(entityID, ".level")
}
```

### Fix 2: Consider Non-Zero Default Debounce

Change default in `processor/rule/config.go`:

```go
DebounceDelayMs: 10 * time.Millisecond,  // Batch rapid updates
```

Or at minimum, don't use 1ns ticker when debounce=0:

```go
if tickerDuration <= 0 {
    tickerDuration = 10 * time.Millisecond  // Reasonable minimum
}
```

### Fix 3: Add Integration Test for Evaluation Bounds

```go
func TestEntityWatcher_EvaluationCountBounded(t *testing.T) {
    // Create 100 entities
    // Assert evaluations < 100 * numRules * 2
    // (allows for some reevaluation but prevents runaway)
}
```

## Related Files

| Component | File | Line |
|-----------|------|------|
| Hierarchy Inference | `processor/graph/inference/hierarchy.go` | 143-153 |
| Coalescing Set | `pkg/cache/coalescing_set.go` | 35-38 |
| Entity Watcher | `processor/rule/entity_watcher.go` | 111-114 |
| Rule Config Defaults | `processor/rule/config.go` | 116 |
| Integration Test | `processor/rule/entity_watcher_integration_test.go` | 80 |
| Structural Config | `configs/structural.json` | - |

## References

- E2E test output showing 62K evaluations
- Test data: `testdata/semantic/sensors.jsonl` (41 lines)
- Expected entity count: 73 source + 54 containers = 127 total
