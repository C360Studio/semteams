# core-dataflow Scenario

## Purpose

Validates that data flows through the complete core pipeline: UDP input → JSON processing → File output. This proves the system can ingest, transform, and persist data.

## Tier

**Core** - Data pipeline validation

## Duration

~10 seconds

## Prerequisites

- Docker Compose environment running (`docker/compose/e2e.yml`)
- UDP port 14550 accessible
- File output component configured

## Invocation

```bash
task e2e:core:default
# or directly:
cd cmd/e2e && ./e2e --scenario core-dataflow
```

## What It Tests

### Stage 1: Verify Components (`verify-components`)

Calls `GET /components/list` and verifies these pipeline components exist:

| Component | Role in Pipeline |
|-----------|------------------|
| `udp` | Ingests UDP datagrams |
| `json_filter` | Filters messages by condition |
| `json_map` | Transforms JSON fields |
| `file` | Writes output to JSONL files |

**Assertion**: All 4 pipeline components must exist.

### Stage 2: Send Data (`send-data`)

Sends 10 test messages via UDP to `localhost:14550`:

```json
{
  "type": "test",
  "value": 0,        // 0, 10, 20, 30, 40, 50, 60, 70, 80, 90
  "timestamp": 1234567890,
  "sequence": 0      // 0-9
}
```

**Note**: Values increment by 10. If a filter condition is `value > 50`, only 4 messages pass (60, 70, 80, 90).

**Assertion**: Messages sent successfully (no hard failure on individual send errors).

### Stage 3: Validate Processing (`validate-processing`)

After 5-second delay, checks file output via `docker exec`:

```bash
docker exec semstreams-e2e-app sh -c "cat /tmp/streamkit-test*.jsonl | wc -l"
```

**Assertion**: At least 5 lines written to file output.

### Fallback Validation

If `docker exec` fails (e.g., container name mismatch), falls back to component-only validation:
- Just checks component count is non-zero
- Logs warning about weaker validation

## Assertions Summary

| Assertion | Location | Strength |
|-----------|----------|----------|
| Pipeline components exist | `executeVerifyComponents` L139-155 | Strong |
| UDP messages sent | `executeSendData` L158-193 | Medium (warns on failure) |
| File output ≥ 5 lines | `executeValidateProcessing` L210-218 | Strong |
| Fallback: components running | `executeValidateComponentsOnly` L226-238 | Weak |

## Gaps Identified

### Current Gaps

1. **No content validation** - Counts lines but doesn't verify message content/structure
2. **No end-to-end correlation** - Doesn't match sent sequence numbers to output
3. **Fallback is very weak** - If docker exec fails, just checks "components running"
4. **Filter logic not validated** - Test assumes filter exists but doesn't verify its behavior
5. **Hardcoded container name** - `semstreams-e2e-app` may not match all compose configs

### Recommendations

1. **Add content validation** - Parse output JSONL, verify expected fields present
2. **Correlate sequence numbers** - Send unique IDs, verify they appear in output
3. **Remove or strengthen fallback** - Fallback masks real failures
4. **Parameterize container name** - Accept via config instead of hardcoding

## Configuration

```go
type CoreDataflowConfig struct {
    MessageCount    int           // Default: 10
    MessageInterval time.Duration // Default: 100ms
    ValidationDelay time.Duration // Default: 5s
    MinProcessed    int           // Default: 5
}
```

## Test Data Flow

```
UDP:14550  →  json_filter  →  json_map  →  file:/tmp/streamkit-test*.jsonl
    │              │              │              │
    │              │              │              └── Writes JSONL lines
    │              │              └── Transforms fields
    │              └── Filters by condition (value > X)
    └── Receives 10 JSON messages
```

## Output Metrics

| Metric | Description |
|--------|-------------|
| `verify-components_duration_ms` | Time to verify components |
| `send-data_duration_ms` | Time to send all messages |
| `validate-processing_duration_ms` | Time for validation |
| `messages_sent` | Number of UDP messages sent |
| `file_lines_written` | Lines found in output file |

## Related Files

- `test/e2e/scenarios/core_dataflow.go` - Scenario implementation
- `test/e2e/client/observability.go` - Contains `CountFileOutputLines`
- `docker/compose/e2e.yml` - Docker Compose configuration

## Example Output

```
[core-dataflow] Starting scenario...
[core-dataflow] Stage 1/3: verify-components
[core-dataflow] Pipeline components verified: [udp, json_filter, json_map, file]
[core-dataflow] Stage 2/3: send-data
[core-dataflow] Sent 10 test messages via UDP
[core-dataflow] Stage 3/3: validate-processing
[core-dataflow] Verified 7 lines written to file output (minimum: 5)
[core-dataflow] SUCCESS (Duration: 6.2s)
```
