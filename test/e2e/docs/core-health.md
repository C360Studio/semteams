# core-health Scenario

## Purpose

Validates that the SemStreams platform boots correctly and all core components are healthy. This is the most basic e2e test - if this fails, nothing else will work.

## Tier

**Core** - Platform bootstrapping validation

## Duration

~5 seconds

## Prerequisites

- Docker Compose environment running (`docker/compose/e2e.yml`)
- SemStreams container healthy (passes `/readyz` check)

## Invocation

```bash
task e2e:core:default
# or directly:
cd cmd/e2e && ./e2e --scenario core-health
```

## What It Tests

### Stage 1: Platform Health (`platform-health`)

Calls `GET /readyz` and verifies:
- HTTP response is 200 OK (or 503 with valid JSON)
- Response body contains `{"healthy": true}`

**Assertion**: Platform must report healthy status.

### Stage 2: Component Health (`component-health`)

Calls `GET /components/list` and verifies:

1. **Required components exist** (8 total):
   | Component | Type | Purpose |
   |-----------|------|---------|
   | `udp` | Input | Network data ingestion |
   | `json_generic` | Processor | JSON parsing |
   | `json_filter` | Processor | Conditional filtering |
   | `json_map` | Processor | Field transformation |
   | `objectstore` | Storage | NATS object store |
   | `file` | Output | File writing |
   | `httppost` | Output | HTTP POST forwarding |
   | `websocket` | Output | WebSocket streaming |

2. **All required components are enabled and healthy**
3. **Minimum 8 healthy components** (configurable via `MinHealthyComponents`)

## Assertions Summary

| Assertion | Location | Strength |
|-----------|----------|----------|
| Platform health = true | `executePlatformHealth` L128-133 | Strong |
| Required components exist | `executeComponentHealth` L160-167 | Strong |
| All required components healthy | `executeComponentHealth` L178-195 | Strong |
| Minimum component count | `executeComponentHealth` L170-175 | Medium (configurable threshold) |

## Gaps Identified

### Current Gaps

1. **No component state validation** - Only checks `healthy` boolean, doesn't verify `state` field values (e.g., "running" vs "starting")
2. **No startup time assertion** - `MaxStartupTime` is defined in config but never checked
3. **No component type validation** - Doesn't verify components are the expected type (input/processor/output)

### Recommendations

1. **Add startup time check** - Verify platform becomes healthy within `MaxStartupTime`
2. **Validate component state** - Assert `state == "running"` not just `healthy == true`

## Configuration

```go
type CoreHealthConfig struct {
    RequireAllHealthy    bool          // Default: true
    MinHealthyComponents int           // Default: 8
    MaxStartupTime       time.Duration // Default: 10s (NOT CURRENTLY USED)
    RequiredComponents   []string      // Default: [udp, json_generic, ...]
}
```

## Output Metrics

| Metric | Description |
|--------|-------------|
| `platform-health_duration_ms` | Time to check platform health |
| `component-health_duration_ms` | Time to check component health |
| `platform_healthy` | Boolean - platform health status |
| `total_components` | Number of components found |
| `healthy_components` | Number of healthy components |

## Related Files

- `test/e2e/scenarios/core_health.go` - Scenario implementation
- `test/e2e/client/observability.go` - HTTP client for health endpoints
- `docker/compose/e2e.yml` - Docker Compose configuration

## Example Output

```
[core-health] Starting scenario...
[core-health] Stage 1/2: platform-health
[core-health] Platform healthy: true
[core-health] Stage 2/2: component-health
[core-health] Found 12 components, 12 healthy
[core-health] All 8 required components present and healthy
[core-health] SUCCESS (Duration: 234ms)
```
