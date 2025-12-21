# Core-Health Scenario Review

**Reviewed**: 2025-12-20  
**File**: `test/e2e/scenarios/core_health.go`  
**Status**: Generally Correct, Minor Gaps

---

## Scenario Overview

The `core-health` scenario validates that SemStreams boots correctly and all core components are healthy. This is the foundational e2e test - if it fails, no other tests are meaningful.

**Duration**: ~5 seconds  
**Tier**: Core  
**Dependencies**: NATS only

---

## What's Tested

### Stage 1: Platform Health (`executePlatformHealth`)

| Check | Implementation | Line |
|-------|----------------|------|
| GET `/readyz` returns valid response | `client.GetPlatformHealth(ctx)` | L128 |
| Platform reports `healthy: true` | `if !health.Healthy` | L133 |

### Stage 2: Component Health (`executeComponentHealth`)

| Check | Implementation | Line |
|-------|----------------|------|
| GET `/components/list` returns components | `client.GetComponents(ctx)` | L148 |
| All 8 required components exist | Loop checking `foundComponents[required]` | L160-167 |
| Minimum healthy component threshold met | `healthyCount < s.config.MinHealthyComponents` | L170-175 |
| All required components are enabled AND healthy | Loop with `RequireAllHealthy` flag | L178-195 |

### Required Components (8 total)

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

---

## Correctness Assessment

### Correct

1. **Platform health check** - Properly validates `/readyz` endpoint returns healthy status
2. **Required component verification** - Correctly checks all 8 components exist
3. **Healthy state validation** - Properly checks `Enabled && Healthy` for required components
4. **Error reporting** - Captures errors and warnings in result struct appropriately
5. **Configurable thresholds** - `CoreHealthConfig` allows customization

### Issues Found

| Issue | Severity | Description |
|-------|----------|-------------|
| `MaxStartupTime` unused | Low | Config defines `MaxStartupTime: 10s` but it's never checked |
| No component state validation | Low | Only checks `healthy` boolean, doesn't verify `state` field (e.g., "running") |
| No component type validation | Low | Doesn't verify components are correct type (input/processor/output) |

---

## Gap Analysis

### Features Documented but Not Tested

| Feature | Documentation Source | Gap |
|---------|---------------------|-----|
| Component startup time | `MaxStartupTime` in config | Defined but never validated |
| Component state machine | Architecture docs mention "running" state | Only checks healthy boolean |
| Gateway components | README lists GraphQL, MCP as components | Not in required list (intentional - gateway tier) |
| Graph processor | Architecture docs show Graph as core | Not in required list (intentional - tiered) |
| Rule processor | Architecture docs show Rule processor | Not in required list (intentional - tiered) |

### Missing Components from Required List

The test checks 8 "core" components but the architecture documentation lists additional components that could be validated at the core tier:

| Component | In Test? | Notes |
|-----------|----------|-------|
| `udp` | Yes | Input |
| `json_generic` | Yes | Processor |
| `json_filter` | Yes | Processor |
| `json_map` | Yes | Processor |
| `objectstore` | Yes | Storage |
| `file` | Yes | Output |
| `httppost` | Yes | Output |
| `websocket` | Yes | Output |
| `graph` | **No** | Core to semantic processing, but tiered |
| `rule` | **No** | Core to rules engine, but tiered |
| `graphql` | **No** | Gateway tier |
| `mcp` | **No** | Gateway tier |

**Assessment**: The exclusion of `graph`, `rule`, `graphql`, `mcp` is intentional - they belong to higher tiers. The test correctly focuses on protocol-layer components.

---

## Recommendations

### Priority: Low (Nice to Have)

1. **Validate `MaxStartupTime`**
   - The config defines this but never uses it
   - Either remove the config field or implement the check
   - Suggested: Add timing check in `executePlatformHealth`

2. **Add component state validation**
   - Currently only checks `healthy` boolean
   - Could additionally verify `state == "running"`
   - Low priority since `healthy` implies correct state

3. **Remove unused config field**
   - If `MaxStartupTime` won't be used, remove it to avoid confusion

### No Action Required

- Component list is appropriate for core tier
- Assertion strength is sufficient for health checks
- Error reporting is comprehensive

---

## Feature Coverage Matrix

| Documented Feature | Tested? | Notes |
|--------------------|---------|-------|
| Platform health endpoint (`/readyz`) | Yes | Strong assertion |
| Component listing (`/components/list`) | Yes | Strong assertion |
| Component health status | Yes | Checks `enabled && healthy` |
| Required component presence | Yes | All 8 verified |
| Minimum healthy threshold | Yes | Configurable |
| Startup timing | **No** | Config exists but unused |
| Component state machine | **No** | Only boolean health check |

---

## Test Output Metrics

The scenario captures these metrics (useful for debugging):

| Metric | Description |
|--------|-------------|
| `platform-health_duration_ms` | Time to check platform health |
| `component-health_duration_ms` | Time to check component health |
| `platform_healthy` | Boolean - platform health status |
| `total_components` | Number of components found |
| `healthy_components` | Number of healthy components |

---

## Conclusion

**Overall Assessment**: The `core-health` scenario is **correct and sufficient** for its purpose. It validates the essential platform bootstrapping requirements. The identified gaps are minor and relate to unused configuration fields rather than missing functionality.

**Recommendation**: No critical changes needed. Optionally clean up the unused `MaxStartupTime` config field.
