# Scenario: gateway-mcp

## Purpose

Validates MCP (Model Context Protocol) gateway operations including tool invocation, SSE transport, rate limiting, and error handling.

## Category

**Tier**: Gateway  
**Priority**: Medium

## What It Tests

This scenario tests the MCP JSON-RPC interface over SSE (Server-Sent Events), validating that AI tools can invoke graph operations through the standardized MCP protocol.

### Test Stages (6 total)

| Stage | Purpose | Assertion Type |
|-------|---------|----------------|
| verify-mcp-health | Health endpoint returns 200 | Hard failure |
| verify-schema-endpoint | Schema endpoint accessible | Soft (warning) |
| test-tool-invocation | GraphQL tool can be called | Soft (warning) |
| test-tool-with-variables | Tool accepts variables | Soft (warning) |
| test-error-handling | Invalid requests handled | No assertion |
| test-rate-limiting | Rate limiter is functional | No assertion |

## Assertions

### Strong Assertions

1. **MCP Health** (lines 142-160)
   - Verifies `/health` returns HTTP 200
   - Fails scenario if health check fails

### Weak Assertions

1. **Schema Endpoint** (lines 162-185)
   - Only logs warning if schema unavailable
   - Does not validate schema structure

2. **Tool Invocation** (lines 231-255)
   - Logs warning on failure, doesn't fail scenario
   - No response structure validation

### No-Op Assertions

1. **Error Handling** (lines 290-318)
   - Records whether errors are "handled"
   - Always returns nil (never fails)

2. **Rate Limiting** (lines 320-378)
   - Sends 25 rapid requests
   - Records counts but accepts any outcome:
   ```go
   // Rate limiting is working if either:
   // 1. Some requests were rate limited (429)
   // 2. All requests succeeded (rate limit not reached)
   return nil  // Always passes
   ```

## Configuration

```go
type MCPGatewayConfig struct {
    SetupDelay      time.Duration  // Default: 2s
    ValidationDelay time.Duration  // Default: 1s
    RequestTimeout  time.Duration  // Default: 30s
}
```

## Prerequisites

- SemStreams MCP gateway running on port 8081
- `/health` endpoint responding
- For tool tests: GraphQL gateway accessible

## Running

```bash
# Run gateway-mcp scenario
task e2e:gateway-mcp

# Or via CLI
./cmd/e2e/e2e run gateway-mcp
```

## Known Gaps

### 1. Rate Limiting Test is Effectively Disabled

The rate limiting test never fails:

```go
// From executeRateLimiting (line 375-377)
// Rate limiting is working if either:
// 1. Some requests were rate limited (429)
// 2. All requests succeeded (rate limit not reached)
return nil
```

**Recommendation**: Define expected behavior:
- If rate limiting is enabled, assert `rateLimitedCount > 0`
- If disabled, remove the test

### 2. SSE Stream Format Not Validated

The SSE parsing is lenient and falls back to raw JSON:

```go
// From executeMCPRequest (lines 216-222)
if lastData == "" {
    // Try reading as regular JSON response
    body, _ := io.ReadAll(resp.Body)
    if len(body) > 0 {
        lastData = string(body)
    }
}
```

**Recommendation**: Validate SSE format compliance:
- Check for proper `data:` prefixes
- Validate event stream structure

### 3. Tool Response Structure Ignored

Tool responses are recorded but not validated:

```go
result.Details["tool_invocation"] = map[string]any{
    "request_sent":    true,
    "response":        resp.Result,  // Not validated
    "has_error":       resp.Error != nil,
}
```

**Recommendation**: Add JSON-RPC 2.0 response validation.

### 4. Error Handling Has No Assertions

The error handling test records results but never fails:

```go
func (s *MCPGatewayScenario) executeErrorHandling(...) error {
    // Records details but...
    return nil  // Always passes
}
```

### 5. No Schema Validation

The schema endpoint test doesn't validate the schema structure:

```go
result.Details["mcp_schema"] = map[string]any{
    "response_length": len(body),  // Only checks length
}
```

## Example Output

```json
{
  "scenario_name": "gateway-mcp",
  "success": true,
  "metrics": {
    "stages_passed": 6,
    "stages_failed": 0,
    "rate_limit_requests": 25,
    "rate_limit_success": 25,
    "rate_limit_blocked": 0
  },
  "details": {
    "mcp_health": {
      "endpoint": "http://localhost:8081/health",
      "status_code": 200
    },
    "rate_limiting": {
      "requests_sent": 25,
      "successful": 25,
      "rate_limited": 0,
      "rate_limit_working": true
    }
  }
}
```

## MCP Protocol Reference

The scenario tests JSON-RPC 2.0 over SSE:

```json
// Request
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "tools/call",
  "params": {
    "name": "graphql",
    "arguments": {
      "query": "{ __typename }"
    }
  }
}

// Response (via SSE)
data: {"jsonrpc":"2.0","id":1,"result":{...}}
```

## Related Scenarios

- **gateway-graphql**: Tests direct GraphQL access (same underlying data)
- **tiered**: Populates data that MCP tools can query
