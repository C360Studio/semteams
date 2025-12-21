# Core-Dataflow Scenario Review

**Reviewed**: 2025-12-20  
**File**: `test/e2e/scenarios/core_dataflow.go`  
**Status**: Functional but Weak Validation

---

## Scenario Overview

The `core-dataflow` scenario validates that data flows through the complete core pipeline: UDP input -> JSON processing -> File output. This proves the system can ingest, transform, and persist data.

**Duration**: ~10 seconds  
**Tier**: Core  
**Dependencies**: NATS only

---

## What's Tested

### Stage 1: Verify Components (`executeVerifyComponents`)

| Check | Implementation | Line |
|-------|----------------|------|
| GET `/components/list` | `client.GetComponents(ctx)` | L131 |
| Required components exist | Loop checking 4 components | L139-155 |

**Required Components**: `udp`, `json_filter`, `json_map`, `file`

### Stage 2: Send Data (`executeSendData`)

| Check | Implementation | Line |
|-------|----------------|------|
| UDP connection established | `net.Dial("udp", s.udpAddr)` | L161 |
| 10 test messages sent | Loop with `conn.Write(msgBytes)` | L174-189 |

**Test Message Format**:
```json
{
  "type": "test",
  "value": 0,         // 0, 10, 20, 30, 40, 50, 60, 70, 80, 90
  "timestamp": 1234567890,
  "sequence": 0       // 0-9
}
```

### Stage 3: Validate Processing (`executeValidateProcessing`)

| Check | Implementation | Line |
|-------|----------------|------|
| Wait 5 seconds | `time.After(s.config.ValidationDelay)` | L201 |
| Count file output lines | `client.CountFileOutputLines(ctx, ...)` | L206 |
| Minimum 5 lines written | `lineCount < s.config.MinProcessed` | L213-218 |

---

## Correctness Assessment

### Correct

1. **Component verification** - Properly checks pipeline components exist
2. **UDP message sending** - Successfully sends test messages with incrementing values
3. **Basic output validation** - Verifies file output contains data
4. **Graceful degradation** - Falls back to component-only validation if docker exec fails

### Issues Found

| Issue | Severity | Description |
|-------|----------|-------------|
| No content validation | **Medium** | Counts lines but doesn't verify message structure or content |
| No end-to-end correlation | **Medium** | Doesn't match sent sequence numbers to received output |
| Filter logic not validated | **Medium** | Assumes filter exists but doesn't verify it actually filters |
| Hardcoded 5s delay | **Low** | Fixed sleep instead of event-driven waiting |
| Fallback masks failures | **Low** | `executeValidateComponentsOnly` is very weak validation |
| Hardcoded container name | **Low** | `semstreams-e2e-app` may not match all configs |

---

## Gap Analysis

### Features Documented but Not Tested

| Feature | Documentation Source | Gap |
|---------|---------------------|-----|
| JSON filter logic | Flow config should define filter condition | Test doesn't verify filter actually works |
| JSON map transformation | Flow config should define field mappings | Test doesn't verify transformation applied |
| Message content integrity | Data flows through pipeline | Only line count, not content |
| Error handling | What happens with malformed JSON? | Not tested |

### Data Pipeline Gaps

The test validates that "data flows" but doesn't validate:

1. **Filter Correctness**: If filter is `value > 50`, only values 60, 70, 80, 90 should pass (4 messages). Test expects minimum 5, which suggests filter may not be configured or test expectations are wrong.

2. **Transformation Correctness**: The `json_map` component should transform fields, but the test doesn't verify output matches expected transformations.

3. **Ordering**: Messages should arrive in order (sequence 0-9), but this isn't validated.

4. **No-Loss Guarantee**: If all 10 messages should pass (no filter), test should expect 10, not 5.

---

## Recommendations

### Priority: Medium (Should Fix)

1. **Add content validation**
   - Parse output JSONL lines
   - Verify expected fields are present
   - Check values match what was sent (accounting for transformations)
   
   ```go
   // Example improvement
   lines, err := s.client.GetFileOutputLines(ctx, container, path)
   for _, line := range lines {
       var msg map[string]any
       json.Unmarshal(line, &msg)
       // Verify expected fields exist
       if msg["sequence"] == nil {
           result.Errors = append(result.Errors, "Missing sequence field")
       }
   }
   ```

2. **Validate filter behavior**
   - Document expected filter condition in test
   - Adjust `MinProcessed` to match expected filtered count
   - Or: Remove filter from test pipeline to test simple pass-through

3. **Correlate sent/received**
   - Add unique message IDs
   - Verify all expected IDs appear in output
   - This proves true end-to-end delivery

### Priority: Low (Nice to Have)

4. **Replace fixed delay with event-driven waiting**
   - Poll file line count until threshold met or timeout
   - More reliable than fixed 5s sleep

5. **Parameterize container name**
   - Accept container name in config
   - Or detect from compose file

6. **Strengthen or remove fallback**
   - Current fallback just counts components - meaningless
   - Either: fail test if docker exec unavailable
   - Or: implement alternative validation (metrics, logs)

---

## Feature Coverage Matrix

| Documented Feature | Tested? | Notes |
|--------------------|---------|-------|
| UDP ingestion | Yes | Messages sent successfully |
| JSON filter component | Partial | Component exists, but filter logic not validated |
| JSON map component | Partial | Component exists, but transformation not validated |
| File output | Yes | Line count verified |
| Message content integrity | **No** | Only counts lines |
| Filter condition correctness | **No** | Not checked |
| Transformation correctness | **No** | Not checked |
| Message ordering | **No** | Not checked |
| Error handling | **No** | Not tested |

---

## Test Output Metrics

| Metric | Description |
|--------|-------------|
| `verify-components_duration_ms` | Time to verify components |
| `send-data_duration_ms` | Time to send all messages |
| `validate-processing_duration_ms` | Time for validation |
| `messages_sent` | Number of UDP messages sent |
| `file_lines_written` | Lines found in output file |
| `component_count` | Fallback: number of components (if docker exec fails) |

---

## Conclusion

**Overall Assessment**: The `core-dataflow` scenario provides **basic validation** that the data pipeline is operational, but the validation is **weak**. The test proves data flows from input to output but doesn't verify correctness of filtering, transformation, or content integrity.

**Risk**: A bug in the filter or map components could go undetected because the test only counts output lines without validating their content.

**Recommendation**: Add content validation to verify messages are correctly filtered and transformed. This is medium priority - the test catches "nothing works" failures but misses "works incorrectly" bugs.
