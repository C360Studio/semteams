# Scenario: gateway-graphql

## Purpose

Validates GraphQL gateway operations including entity queries, relationship traversal, search capabilities (semantic, spatial, temporal), community queries, and error handling.

## Category

**Tier**: Gateway  
**Priority**: High

## What It Tests

This scenario tests the GraphQL API contract by executing actual queries against the gateway endpoint and verifying responses.

### Test Stages (10 total)

| Stage | Purpose | Assertion Type |
|-------|---------|----------------|
| verify-graphql-health | GraphQL endpoint responds | Hard failure |
| test-entity-query | Single entity lookup works | Soft (warning) |
| test-entities-batch | Batch entity loading works | Soft (warning) |
| test-entities-by-type | Type-filtered queries work | Soft (warning) |
| test-relationships-query | Relationship traversal works | Soft (warning) |
| test-semantic-search | Semantic similarity search works | Soft (warning) |
| test-spatial-search | Geographic bounding box search works | Soft (warning) |
| test-temporal-search | Time-range search works | Soft (warning) |
| test-community-query | Community queries work | Soft (warning) |
| test-error-handling | Invalid queries return proper errors | Hard failure |

## Assertions

### Strong Assertions

1. **GraphQL Health** (lines 172-185)
   - Executes `{ __typename }` introspection query
   - Fails if GraphQL endpoint doesn't respond

2. **Error Handling** (lines 346-372)
   - Verifies invalid query syntax returns error
   - Verifies missing required field returns error
   - Both must be handled correctly

### Weak Assertions (Warnings Only)

Most query tests only log warnings if queries fail:

```go
// From executeEntityQuery (line 199-201)
result.Warnings = append(result.Warnings, 
    fmt.Sprintf("Entity query returned error (may not exist): %v", err))
```

**Gap**: All search and query operations use warnings rather than failures. The tests verify queries execute but don't validate:
- Response structure correctness
- Result count expectations
- Data integrity
- Query performance

## Configuration

```go
type GraphQLGatewayConfig struct {
    SetupDelay      time.Duration  // Default: 2s
    ValidationDelay time.Duration  // Default: 1s  
    QueryTimeout    time.Duration  // Default: 10s
}
```

## Prerequisites

- SemStreams running with GraphQL gateway enabled
- GraphQL endpoint accessible at configured base URL
- For meaningful query tests: data should be populated

## Running

```bash
# Run gateway-graphql scenario
task e2e:gateway-graphql

# Or via CLI
./cmd/e2e/e2e run gateway-graphql
```

## Known Gaps

### 1. No Response Validation

Queries are executed but responses aren't validated for structure:

```go
// test-entity-query just checks if entity != nil
result.Details["entity_query"] = map[string]any{
    "query_executed": true,
    "entity_found":   entity != nil,  // No structure validation
}
```

**Recommendation**: Add schema validation for responses.

### 2. No Performance Assertions

Query timing is recorded but not validated:

```go
result.Metrics[fmt.Sprintf("%s_duration_ms", stage.name)] = 
    time.Since(stageStart).Milliseconds()
```

**Recommendation**: Add latency thresholds for query operations.

### 3. Entity May Not Exist

Test entity IDs are hardcoded patterns that may not exist:

```go
testID := "c360.semstreams.environmental.sensor.temperature.sensor-0"
```

**Recommendation**: Either:
- Ensure test data is populated before running
- Create entities as part of test setup
- Use entity discovery before queries

### 4. No Pagination Testing

Batch and list queries don't test pagination:

```go
"limit": 10,  // Always uses small limit, no pagination test
```

### 5. Community Query Shallow

Community query only tests level 0:

```go
resp, err := s.executeGraphQL(ctx, query, map[string]any{
    "level": 0,  // Only tests base level
})
```

## Example Output

```json
{
  "scenario_name": "gateway-graphql",
  "success": true,
  "metrics": {
    "stages_passed": 10,
    "stages_failed": 0,
    "verify-graphql-health_duration_ms": 45,
    "test-semantic-search_duration_ms": 234
  },
  "details": {
    "graphql_health": {"endpoint": "http://localhost:8080/graphql"},
    "entity_query": {"query_executed": true, "entity_found": false},
    "semantic_search": {"query_executed": true, "results_found": 0}
  },
  "warnings": [
    "Entity query returned error (may not exist): ..."
  ]
}
```

## Related Scenarios

- **gateway-mcp**: Tests MCP gateway (alternative API interface)
- **tiered**: Populates data that GraphQL queries can retrieve
