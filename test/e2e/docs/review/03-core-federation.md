# Core-Federation Scenario Review

**Reviewed**: 2025-12-20  
**File**: `test/e2e/scenarios/core_federation.go`  
**Status**: Weak Validation - Significant Gaps

---

## Scenario Overview

The `core-federation` scenario validates data flow between edge and cloud SemStreams instances, testing the UDP -> WebSocket -> File pipeline with ack/nack protocol verification.

**Duration**: ~10 seconds  
**Tier**: Core  
**Dependencies**: Two SemStreams instances (edge + cloud), NATS

---

## What's Tested

### Stage 1: Verify Edge Health (`verifyEdgeHealth`)

| Check | Implementation | Line |
|-------|----------------|------|
| Edge platform health | `edgeClient.GetPlatformHealth(ctx)` | L163 |
| Status == "healthy" | `health.Status != "healthy"` | L168 |

### Stage 2: Verify Cloud Health (`verifyCloudHealth`)

| Check | Implementation | Line |
|-------|----------------|------|
| Cloud platform health | `cloudClient.GetPlatformHealth(ctx)` | L178 |
| Status == "healthy" | `health.Status != "healthy"` | L183 |

### Stage 3: Send Test Data (`sendTestData`)

| Check | Implementation | Line |
|-------|----------------|------|
| UDP connection | `net.Dial("udp", s.udpAddr)` | L191 |
| Send 20 messages | Loop with `conn.Write(data)` | L196-219 |
| At least 1 message sent | `if sent == 0` | L227 |

**Test Message Format**:
```json
{
  "message_id": "fed-test-0",
  "timestamp": 1234567890,
  "sequence": 0,
  "test": "federation",
  "value": 0.0
}
```

### Stage 4: Verify Federation Flow (`verifyFederationFlow`)

| Check | Implementation | Line |
|-------|----------------|------|
| Wait 5 seconds | `time.Sleep(s.config.ValidationDelay)` | L234 |
| Get edge components | `edgeClient.GetComponents(ctx)` | L237 |
| Get cloud components | `cloudClient.GetComponents(ctx)` | L243 |
| Cloud has components | `len(cloudComponents) == 0` | L254 |

### Stage 5: Verify Ack Protocol (`verifyAckProtocol`)

| Check | Implementation | Line |
|-------|----------------|------|
| WebSocket connection | `websocket.DefaultDialer.Dial(s.wsURL, nil)` | L271 |
| Send test message | `wsConn.WriteJSON(testMsg)` | L285 |
| Read ack response | `wsConn.ReadJSON(&response)` | L292-297 |

### Stage 6: Verify Metrics (`verifyMetrics`)

| Check | Implementation | Line |
|-------|----------------|------|
| Edge component count | `edgeClient.GetComponents(ctx)` | L307 |
| Cloud component count | `cloudClient.GetComponents(ctx)` | L314 |

---

## Correctness Assessment

### Correct

1. **Health checks** - Both edge and cloud health are properly validated
2. **UDP message sending** - Successfully sends test messages with unique IDs
3. **Ack protocol setup** - Correctly establishes WebSocket connection and sends test message

### Issues Found

| Issue | Severity | Description |
|-------|----------|-------------|
| `MinMessagesOnCloud` unused | **High** | Config defines threshold (15) but never checked |
| No message arrival validation | **High** | Only checks cloud has components, not that messages arrived |
| No message correlation | **High** | Doesn't verify sent message IDs appeared on cloud |
| Ack failure is just warning | **Medium** | Ack/nack test doesn't fail if no ack received |
| Single ack test | **Medium** | Only tests one ack message, not protocol correctness |
| No latency measurement | **Low** | Doesn't measure edge-to-cloud latency |

---

## Gap Analysis

### Critical Gaps

#### 1. `MinMessagesOnCloud` is Defined but Never Used

The config defines:
```go
MinMessagesOnCloud: 15, // At least 75% should make it through
```

But `verifyFederationFlow` only checks:
```go
if len(cloudComponents) == 0 {
    return fmt.Errorf("no components running on cloud instance")
}
```

**This means the test passes even if zero messages are federated.**

#### 2. No Actual Federation Verification

The test sends 20 messages to edge but never verifies:
- Messages were forwarded over WebSocket
- Messages arrived at cloud
- Messages were written to cloud file output

Current "verification" is just checking cloud has some components running.

#### 3. No Message Correlation

Messages are sent with unique IDs (`fed-test-0`, `fed-test-1`, etc.) but the test never checks if these specific messages arrived on the cloud side.

### Medium Gaps

#### 4. Ack Protocol Test is Shallow

- Only sends one test message
- Failure to receive ack is just a warning, not a test failure
- Doesn't verify ack contains correct message ID
- Doesn't test nack scenarios

#### 5. No Error Recovery Testing

Documentation mentions ack/nack protocol for delivery confirmation, but test doesn't verify:
- What happens when cloud sends nack?
- Does edge retry on nack?
- What happens on connection drop?

### Features Documented but Not Tested

| Feature | Documentation Source | Gap |
|---------|---------------------|-----|
| Message delivery to cloud | Config has `MinMessagesOnCloud: 15` | Never checked |
| Ack/nack protocol | Config has `AckVerification: true` | Only logs warning on failure |
| Message correlation | Messages have `message_id` field | IDs not verified on cloud |
| Retry on failure | Federation docs mention reliability | Not tested |
| Bidirectional communication | Architecture shows ack flow | Only one-way test |
| Edge-to-cloud latency | Performance requirement | Not measured |

---

## Recommendations

### Priority: High (Must Fix)

1. **Validate message arrival on cloud**
   
   ```go
   // Instead of just checking components exist:
   lineCount, err := s.cloudClient.CountFileOutputLines(ctx, container, path)
   if lineCount < s.config.MinMessagesOnCloud {
       return fmt.Errorf("only %d/%d messages arrived on cloud",
           lineCount, s.config.MessageCount)
   }
   ```

2. **Use the `MinMessagesOnCloud` config value**
   - This threshold exists for a reason - implement the check
   - Fail the test if delivery rate is below threshold

3. **Correlate sent/received messages**
   - Query cloud for specific message IDs
   - Verify at least N of the sent IDs appeared

### Priority: Medium (Should Fix)

4. **Make ack verification a hard failure**
   - If `AckVerification: true`, no ack should fail the test
   - Verify ack contains the correct message ID

5. **Test multiple ack/nack scenarios**
   - Send multiple messages
   - Verify each gets proper ack
   - Consider testing nack scenarios

### Priority: Low (Nice to Have)

6. **Add latency measurement**
   - Timestamp messages on send
   - Check timestamp on cloud arrival
   - Report edge-to-cloud latency

7. **Test error recovery**
   - Simulate connection drop
   - Verify retry behavior

---

## Feature Coverage Matrix

| Documented Feature | Tested? | Notes |
|--------------------|---------|-------|
| Edge health | Yes | Strong assertion |
| Cloud health | Yes | Strong assertion |
| UDP ingestion on edge | Yes | Messages sent |
| WebSocket forwarding | **Partial** | Connection tested, data flow not verified |
| Cloud message receipt | **No** | Only checks components exist |
| Message delivery guarantee | **No** | MinMessagesOnCloud unused |
| Ack/nack protocol | **Partial** | Single message, warning only |
| Message correlation | **No** | IDs not tracked |
| Latency measurement | **No** | Not implemented |
| Error recovery/retry | **No** | Not tested |

---

## Test Output Metrics

| Metric | Description |
|--------|-------------|
| `messages_sent` | UDP messages sent to edge |
| `edge_component_count` | Components on edge instance |
| `cloud_component_count` | Components on cloud instance |
| `ack_type` | Type of ack response received (if any) |

---

## Architecture Tested

```
┌─────────────────┐         ┌─────────────────┐
│   Edge Instance │         │  Cloud Instance │
│                 │         │                 │
│  ┌───────────┐  │  WS     │  ┌───────────┐  │
│  │UDP Input  │──┼────────►│──│WS Input   │  │
│  └───────────┘  │         │  └───────────┘  │
│       │         │  ack    │       │         │
│       ▼         │◄────────┼───────┘         │ 
│  ┌───────────┐  │         │       ▼         │
│  │WS Output  │  │         │  ┌───────────┐  │
│  └───────────┘  │         │  │File Output│  │ ← NOT VERIFIED
│                 │         │  └───────────┘  │
└─────────────────┘         └─────────────────┘
      ▲                              ▲
   Tested                      Only component
                               existence checked
```

---

## Conclusion

**Overall Assessment**: The `core-federation` scenario has **significant validation gaps**. While it sets up the federation architecture correctly and sends test data, it fails to verify that data actually flows from edge to cloud. The `MinMessagesOnCloud` config value is defined but never used, meaning the test passes even with 0% message delivery.

**Risk**: Federation could be completely broken and this test would still pass, as long as both instances are healthy and have components running.

**Recommendation**: High priority fixes needed:
1. Implement message arrival validation using `MinMessagesOnCloud`
2. Add message ID correlation
3. Make ack verification a hard failure when enabled
