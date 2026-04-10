# Runtime Observability Integration Architecture

**Date**: 2025-11-17
**Status**: Planning / Investigation
**Related BD Issues**: semstreams-0uh5, semstreams-8cj7, semstreams-zixp

## Overview

This document guides the investigation of existing observability infrastructure in semstreams to inform the integration strategy for the Runtime Visualization Panel UI.

**Goal**: Leverage existing infrastructure (Prometheus, message logger, service manager HTTP) before creating new endpoints.

---

## UI Requirements (from semstreams-8cj7)

The Runtime Visualization Panel needs:

### 1. Logs Tab

- **Data**: Real-time component logs with timestamp, level, component name, message
- **Format**: Streaming (SSE preferred)
- **Filters**: By level (DEBUG/INFO/WARN/ERROR), by component
- **Expected Endpoint**: `GET /flows/{id}/runtime/logs` (SSE)

### 2. Metrics Tab

- **Data**: Component throughput (msgs/sec), error rates, CPU/memory, status
- **Format**: JSON polling (1-10s intervals)
- **Expected Endpoint**: `GET /flows/{id}/runtime/metrics` (JSON)

### 3. Health Tab

- **Data**: Component status (running/degraded/error), uptime, last activity, issue details
- **Format**: JSON polling (5s interval)
- **Expected Endpoint**: `GET /flows/{id}/runtime/health` (JSON)

---

## Existing Infrastructure to Investigate

### 1. Service Manager HTTP Service

**Location**: `semstreams/service/http/` (or similar)

**Questions**:

- What endpoints does it currently expose?
- Does it provide health checks?
- Does it expose component status?
- Can it stream logs via SSE?
- What's the base URL pattern?

**Investigation Steps**:

```bash
# Find HTTP service implementation
cd semstreams
find . -name "*http*" -type f | grep -E "\.(go|mod)$"

# Look for route definitions
grep -r "http\." --include="*.go" | grep -E "(Handle|Get|Post)"

# Look for existing endpoints
grep -r "/metrics\|/health\|/logs" --include="*.go"
```

**Expected Findings**:

- [ ] List of current HTTP endpoints
- [ ] Health check implementation
- [ ] Component status API (if exists)
- [ ] Log streaming capability (if exists)

---

### 2. Prometheus Integration

**Metrics Endpoint**: `/metrics` (standard Prometheus format)

**Questions**:

- What metrics are already exposed?
- Are component-level metrics available?
- Can we get throughput (msgs/sec)?
- Can we get error rates?
- Can we get component uptime?
- Is CPU/memory exposed per component?

**Investigation Steps**:

```bash
# Find Prometheus instrumentation
cd semstreams
grep -r "prometheus" --include="*.go"

# Look for metric definitions
grep -r "NewCounter\|NewGauge\|NewHistogram" --include="*.go"

# Check if metrics are per-component
grep -r "WithLabelValues\|Labels{" --include="*.go"
```

**Example Prometheus Queries** (if metrics exist):

```promql
# Component throughput
rate(component_messages_processed_total[1m])

# Error rate
rate(component_errors_total[1m])

# Component uptime
time() - component_start_time_seconds
```

**Expected Findings**:

- [ ] List of available Prometheus metrics
- [ ] Whether metrics are per-component
- [ ] Whether throughput/errors/uptime are available
- [ ] Metric naming conventions

---

### 3. Message Logger Service

**Location**: Possibly `semstreams/messagelogger/` or similar

**Questions**:

- What is the message logger service?
- Does it track message flow across NATS?
- Does it provide traces/spans?
- Can it stream events in real-time?
- Should we add a "Traces" or "Message Flow" tab?

**Investigation Steps**:

```bash
# Find message logger implementation
cd semstreams
find . -name "*messagelogger*" -o -name "*message_logger*"

# Look for message tracking
grep -r "messagelogger\|message_logger" --include="*.go"

# Check for NATS subscriptions
grep -r "Subscribe.*logger\|logger.*Subscribe" --include="*.go"
```

**Potential Capabilities**:

- Message tracing across components
- End-to-end latency tracking
- Message routing visualization
- Dropped message detection

**Expected Findings**:

- [ ] Message logger architecture
- [ ] What data it captures
- [ ] How to query/stream message events
- [ ] Whether it duplicates log functionality
- [ ] Whether we should add a "Traces" tab

---

### 4. NATS System Events

**Questions**:

- Can components publish lifecycle events to NATS?
- Are there system subjects for logs/metrics/health?
- Can UI subscribe via SSE proxy?
- Would this be better than HTTP polling?

**Investigation Steps**:

```bash
# Find NATS event publishing
cd semstreams
grep -r "nc.Publish\|js.Publish" --include="*.go" | grep -i "event\|system\|log"

# Look for system subjects
grep -r "nats.subject" --include="*.go" | grep -i "system\|event"

# Check for existing subscriptions
grep -r "nc.Subscribe\|js.Subscribe" --include="*.go"
```

**Potential System Subjects**:

- `system.component.{id}.logs` - Component log stream
- `system.component.{id}.metrics` - Component metrics
- `system.component.{id}.status` - Component status changes
- `system.flow.{id}.events` - Flow lifecycle events

**Expected Findings**:

- [ ] Whether system subjects exist
- [ ] Event formats/schemas
- [ ] Whether SSE proxy exists or is needed
- [ ] Performance implications (polling vs streaming)

---

## Integration Options Analysis

### Option 1: Use Prometheus Metrics (for Metrics Tab)

**Pros**:

- ✅ Already instrumented (likely)
- ✅ Standard observability pattern
- ✅ No new backend code needed
- ✅ Can leverage existing monitoring infrastructure

**Cons**:

- ❌ Requires Prometheus server running
- ❌ Adds dependency (Prometheus API)
- ❌ May not have all needed metrics
- ❌ Query complexity (PromQL in frontend?)

**Implementation**:

```typescript
// Option 1A: Query Prometheus directly from frontend
const response = await fetch(
  `http://prometheus:9090/api/v1/query?query=rate(component_messages_total[1m])`,
);

// Option 1B: Backend proxy to Prometheus
const response = await fetch(`/flowbuilder/flows/${flowId}/runtime/metrics`);
// Backend queries Prometheus and returns formatted JSON
```

**Recommendation**: Use backend proxy (Option 1B) to:

- Abstract Prometheus details from UI
- Handle authentication
- Format data consistently
- Allow future swapping of metric backend

---

### Option 2: Use Message Logger (for Logs/Traces Tab)

**Pros**:

- ✅ May already track message flow
- ✅ Could provide end-to-end tracing
- ✅ Richer than simple logs
- ✅ NATS-native integration

**Cons**:

- ❌ May not exist yet
- ❌ May not capture component logs
- ❌ Different from traditional logs
- ❌ Requires message logger enabled

**Implementation**:

```typescript
// Subscribe to message logger events via SSE
const eventSource = new EventSource(
  `/flowbuilder/flows/${flowId}/runtime/traces`,
);

eventSource.addEventListener("trace", (event) => {
  const trace = JSON.parse(event.data);
  // trace.componentPath: ["udp-source", "processor", "nats-sink"]
  // trace.latency: 5.2ms
  // trace.messageId: "msg-123"
});
```

**Consideration**: May warrant a separate "Traces" tab if rich enough.

---

### Option 3: NATS System Events (for Real-time Streaming)

**Pros**:

- ✅ Real-time (no polling needed)
- ✅ NATS-native architecture
- ✅ Scalable pub/sub
- ✅ Can stream logs, metrics, health

**Cons**:

- ❌ Requires SSE proxy (backend → NATS → frontend)
- ❌ May need new system subjects
- ❌ More complex architecture
- ❌ Connection management (reconnects)

**Implementation**:

```go
// Backend SSE proxy
func streamComponentLogs(w http.ResponseWriter, r *http.Request, flowID string) {
    // Set SSE headers
    w.Header().Set("Content-Type", "text/event-stream")

    // Subscribe to NATS subject
    sub, err := nc.Subscribe(fmt.Sprintf("system.flow.%s.logs.>", flowID), func(msg *nats.Msg) {
        // Forward NATS message as SSE event
        fmt.Fprintf(w, "event: log\ndata: %s\n\n", msg.Data)
        w.(http.Flusher).Flush()
    })
    defer sub.Unsubscribe()

    <-r.Context().Done()
}
```

**Recommendation**: Best for Logs tab (real-time streaming requirement).

---

### Option 4: New HTTP Endpoints (Minimal Implementation)

If existing infrastructure doesn't provide needed data:

**Logs Endpoint**:

```go
// GET /flowbuilder/flows/{id}/runtime/logs (SSE)
// Streams component logs from all components in flow

type LogEvent struct {
    Timestamp string `json:"timestamp"`
    Level     string `json:"level"`     // DEBUG, INFO, WARN, ERROR
    Component string `json:"component"`
    Message   string `json:"message"`
}
```

**Metrics Endpoint**:

```go
// GET /flowbuilder/flows/{id}/runtime/metrics (JSON)
// Polls Prometheus or internal metrics store

type MetricsResponse struct {
    Timestamp  string              `json:"timestamp"`
    Components []ComponentMetrics  `json:"components"`
}

type ComponentMetrics struct {
    Name       string  `json:"name"`
    Throughput float64 `json:"throughput"`  // msgs/sec
    ErrorRate  float64 `json:"errorRate"`   // errors/sec
    Status     string  `json:"status"`      // healthy, degraded, error
    CPU        float64 `json:"cpu"`         // percentage
    Memory     int64   `json:"memory"`      // bytes
}
```

**Health Endpoint**:

```go
// GET /flowbuilder/flows/{id}/runtime/health (JSON)
// Component health status and diagnostics

type HealthResponse struct {
    Timestamp string           `json:"timestamp"`
    Overall   OverallHealth    `json:"overall"`
    Components []ComponentHealth `json:"components"`
}

type ComponentHealth struct {
    Name         string  `json:"name"`
    Status       string  `json:"status"`       // running, degraded, error
    StartTime    string  `json:"startTime"`    // ISO timestamp
    LastActivity string  `json:"lastActivity"` // ISO timestamp
    Details      *IssueDetails `json:"details"` // nil if healthy
}
```

---

## Investigation Plan

### Phase 1: Discovery (Current Step)

**Tasks**:

- [ ] Locate service manager HTTP service code
- [ ] Document existing HTTP endpoints
- [ ] Review Prometheus metrics implementation
- [ ] Check for message logger service
- [ ] Review NATS system event patterns

**Deliverable**: Inventory of existing capabilities

---

### Phase 2: Gap Analysis

**Tasks**:

- [ ] Map UI requirements to existing capabilities
- [ ] Identify gaps (what's missing)
- [ ] Evaluate integration options
- [ ] Consider message logger as separate tab

**Deliverable**: Requirements coverage matrix

| UI Requirement       | Existing Source | Gap | Recommendation             |
| -------------------- | --------------- | --- | -------------------------- |
| Real-time logs       | NATS events?    | TBD | SSE from NATS              |
| Component throughput | Prometheus      | TBD | Query via backend          |
| Error rates          | Prometheus      | TBD | Query via backend          |
| Component status     | HTTP health?    | TBD | New endpoint or Prometheus |
| Uptime               | Prometheus      | TBD | Calculate from start_time  |
| Message traces       | Message logger? | TBD | New "Traces" tab?          |

---

### Phase 3: Architecture Decision

**Decision Points**:

1. **Use Prometheus for Metrics?**
   - YES: Implement backend proxy to Prometheus
   - NO: Create new metrics aggregation endpoint

2. **Use Message Logger for Tracing?**
   - YES: Add "Traces" tab to UI
   - NO: Implement simple log streaming

3. **Use NATS Events for Logs?**
   - YES: Implement SSE proxy to NATS
   - NO: Create file-based log streaming

4. **Create New Endpoints?**
   - Minimal: Only for gaps in existing infrastructure
   - Full: Implement all three endpoints independently

**Deliverable**: Architecture Decision Record (ADR)

---

### Phase 4: Implementation Plan

Based on architecture decision:

**If using Prometheus**:

- [ ] Implement Prometheus query backend
- [ ] Create `/metrics` proxy endpoint
- [ ] Update UI to use backend proxy

**If using Message Logger**:

- [ ] Design "Traces" tab UI
- [ ] Implement message logger query API
- [ ] Add trace visualization

**If using NATS Events**:

- [ ] Define system event subjects
- [ ] Implement SSE proxy
- [ ] Add component event publishing

**If creating new endpoints**:

- [ ] Implement logs SSE endpoint
- [ ] Implement metrics polling endpoint
- [ ] Implement health polling endpoint

---

## Decision Criteria

When evaluating options, consider:

1. **Reuse over Reinvention**: Prefer existing infrastructure
2. **Standards Compliance**: Use Prometheus if available (industry standard)
3. **Scalability**: NATS pub/sub scales better than HTTP polling
4. **Simplicity**: Fewer moving parts = easier to maintain
5. **Feature Richness**: Message logger may provide better insights than logs
6. **User Experience**: Real-time streaming > polling for logs
7. **Development Time**: Leverage what exists to ship faster

---

---

## Investigation Findings

### ServiceManager HTTP Service (Port 8080)

**Location**: `semstreams/service/service_manager.go`

**Unified HTTP Server**: ServiceManager creates a single HTTP server that orchestrates all service endpoints.

**System Endpoints Available**:

| Endpoint           | Method | Description                                    | Response Format                           |
| ------------------ | ------ | ---------------------------------------------- | ----------------------------------------- |
| `/health`          | GET    | Aggregated system health (all services + NATS) | JSON (`health.Status`)                    |
| `/healthz`         | GET    | Simple liveness probe                          | Text: "OK"                                |
| `/readyz`          | GET    | Readiness check (all services running?)        | Text: "READY" / "NOT READY"               |
| `/services`        | GET    | List all services with status                  | JSON: `{services: [], count: N}`          |
| `/services/health` | GET    | Detailed health of all services                | JSON: `{overall: {...}, services: [...]}` |
| `/openapi.json`    | GET    | OpenAPI specification                          | JSON (OpenAPI 3.0)                        |
| `/docs`            | GET    | Swagger UI                                     | HTML                                      |

**Service-Specific Endpoints**:

- Services implementing `HTTPHandler` interface register routes at `/{service-prefix}/*`
- Gateway components register at `/{component-name}/*`

**Health Status Structure** (`health.Status`):

```go
type Status struct {
    Name       string   // Component/service name
    Healthy    bool     // Overall health flag
    Message    string   // Status message
    Details    []Status // Sub-component statuses (hierarchical)
}
```

**Key Insight**: `/services/health` already provides component-level health with hierarchical status!

---

### Message Logger Service (Port 8080)

**Location**: `semstreams/service/message_logger.go`, `message_logger_http.go`

**Purpose**: Monitors NATS message traffic across subjects (`process.>`, `input.>`, `events.>`)

**Storage**: Circular buffer (max 10,000 entries in memory)

**HTTP Endpoints** (registered at `/message-logger/*`):

| Endpoint                      | Method | Parameters                 | Description                    |
| ----------------------------- | ------ | -------------------------- | ------------------------------ |
| `/message-logger/entries`     | GET    | `?limit=N&subject=pattern` | Get recent message log entries |
| `/message-logger/stats`       | GET    | -                          | Message statistics             |
| `/message-logger/subjects`    | GET    | -                          | List monitored NATS subjects   |
| `/message-logger/kv/{bucket}` | GET    | -                          | Query KV bucket contents       |

**MessageLogEntry Structure**:

```go
type MessageLogEntry struct {
    Timestamp   time.Time       // When message observed
    Subject     string          // NATS subject
    MessageType string          // Component-specific type
    MessageID   string          // Unique message ID
    Summary     string          // Human-readable summary
    RawData     json.RawMessage // Full message payload
    Metadata    map[string]any  // Additional metadata
}
```

**Key Capabilities**:

- ✅ Real-time NATS message observation
- ✅ Subject filtering support
- ✅ Limit/pagination support
- ✅ Full message payloads available
- ❌ No SSE streaming (polling only)
- ❌ No structured component logs (only NATS messages)

**Potential UI Use**:

- Could power "Message Flow" or "Traces" tab showing NATS message routing
- NOT suitable for component logs (different data model)

---

### Prometheus Metrics Service (Port 9090)

**Location**: `semstreams/service/metrics.go`, `metric/handler.go`

**Separate HTTP Server**: Dedicated metrics server at `:9090/metrics` (standard Prometheus pattern)

**Endpoints**:

- `GET :9090/metrics` - Prometheus metrics (OpenMetrics format)
- `GET :9090/health` - Metrics server health check
- `GET :9090/` - HTML info page with links

**Metric Namespace**: `semstreams_*`

**Flow-Level Metrics** (`semstreams/engine/metrics.go`):

```promql
# Flow lifecycle
semstreams_flow_deploys_total{flow_id, status}          # Counter
semstreams_flow_starts_total{flow_id, status}           # Counter
semstreams_flow_stops_total{flow_id, status}            # Counter

# Operation latency
semstreams_flow_deploy_duration_seconds{flow_id}        # Histogram
semstreams_flow_start_duration_seconds{flow_id}         # Histogram
semstreams_flow_stop_duration_seconds{flow_id}          # Histogram
semstreams_flow_validate_duration_seconds{flow_id}      # Histogram

# Validation errors
semstreams_flow_validation_errors_total{flow_id, error_type}  # Counter

# Active flows
semstreams_flow_active_flows                             # Gauge
```

**Component-Level Metrics** (examples from `input/websocket/`):

```promql
# Message throughput
semstreams_websocket_messages_received_total{component}  # Counter
semstreams_websocket_messages_published_total{component, subject}  # Counter
semstreams_websocket_messages_dropped_total{component}   # Counter

# Connection health
semstreams_websocket_connections_active                  # Gauge
semstreams_websocket_connections_total                   # Counter
semstreams_websocket_reconnect_attempts                  # Counter

# Request/Reply metrics
semstreams_websocket_requests_sent_total{component}      # Counter
semstreams_websocket_replies_received_total{component}   # Counter
semstreams_websocket_request_timeouts_total{component}   # Counter
semstreams_websocket_request_duration_seconds{component} # Histogram

# Queue metrics
semstreams_websocket_queue_depth                         # Gauge
semstreams_websocket_queue_utilization                   # Gauge
```

**Similar patterns exist for**:

- JSON processors (`processor/json_filter/`, `json_map/`)
- Graph processors (`processor/graph/querymanager/`, `indexmanager/`)
- Storage components (`storage/objectstore/`)
- NATS JetStream (`natsclient/jetstream_metrics.go`)

**Key Capabilities**:

- ✅ Per-component metrics (throughput, errors, latency)
- ✅ Industry-standard Prometheus format
- ✅ Rich metric types (counters, gauges, histograms)
- ✅ Label-based filtering (by component, subject, etc.)
- ⚠️ Requires Prometheus server running (already part of infrastructure)
- ⚠️ Requires PromQL queries (should proxy via backend)

---

## Gap Analysis: UI Requirements vs Existing Capabilities

### 1. Logs Tab

**UI Requirements**:

- Real-time component logs
- Filter by level (DEBUG/INFO/WARN/ERROR)
- Filter by component
- Streaming updates (SSE preferred)

**Existing Infrastructure**:

| Requirement         | Source             | Gap                               |
| ------------------- | ------------------ | --------------------------------- |
| Component logs      | ❌ Not available   | **Need new endpoint**             |
| Real-time streaming | ❌ No SSE endpoint | **Need SSE implementation**       |
| Filter by level     | ❌ N/A             | **Need structured logging**       |
| Filter by component | ❌ N/A             | **Need component identification** |

**Message Logger Evaluation**:

- ❌ Tracks NATS messages, NOT component logs
- ❌ No log levels (DEBUG/INFO/WARN/ERROR)
- ❌ Different data model (message traces vs logs)
- ✅ Could power separate "Message Flow" tab

**Recommendation**: **Build new `/flows/{id}/runtime/logs` SSE endpoint**

- Aggregate logs from all components in flow
- Add structured logging to components (level, component, message)
- Stream via SSE (not available in message-logger)

---

### 2. Metrics Tab

**UI Requirements**:

- Component throughput (msgs/sec)
- Error rates
- CPU/memory usage
- Component status
- Polling (1-10s intervals)

**Existing Infrastructure**:

| Requirement      | Source                                           | Status                      |
| ---------------- | ------------------------------------------------ | --------------------------- |
| Throughput       | Prometheus: `rate(messages_published_total[1m])` | ✅ Available                |
| Error rates      | Prometheus: `rate(messages_dropped_total[1m])`   | ✅ Available                |
| CPU/memory       | ❓ TBD (process-level?)                          | ⚠️ May not be per-component |
| Component status | `/services/health` endpoint                      | ✅ Available                |

**Recommendation**: **Proxy Prometheus + `/services/health`**

Create `/flows/{id}/runtime/metrics` endpoint that:

1. Queries Prometheus for throughput/error metrics
2. Queries `/services/health` for component status
3. Combines into UI-friendly JSON
4. Returns component-level metrics array

**Example PromQL queries**:

```promql
# Component throughput (messages/sec)
rate(semstreams_websocket_messages_published_total{component="udp-source"}[1m])

# Error rate
rate(semstreams_websocket_messages_dropped_total{component="processor"}[1m])
```

**Benefit**: Leverages existing Prometheus infrastructure, no new instrumentation needed!

---

### 3. Health Tab

**UI Requirements**:

- Component status (running/degraded/error)
- Uptime tracking
- Last activity timestamp
- Issue details (for degraded/error components)
- Polling (5s interval)

**Existing Infrastructure**:

| Requirement      | Source                                              | Status                             |
| ---------------- | --------------------------------------------------- | ---------------------------------- |
| Component status | `/services/health`                                  | ✅ Available (hierarchical health) |
| Uptime           | Prometheus: `time() - component_start_time_seconds` | ⚠️ Need start_time metric          |
| Last activity    | ❌ Not tracked                                      | ⚠️ Need new metric/endpoint        |
| Issue details    | `/services/health` - `Message` field                | ✅ Available                       |

**Existing `/services/health` Response**:

```json
{
  "overall": {
    "name": "services",
    "healthy": true,
    "message": "All systems operational",
    "details": []
  },
  "services": [
    {
      "name": "component-manager",
      "healthy": true,
      "message": "Running 3 components",
      "details": [
        {
          "name": "udp-source",
          "healthy": true,
          "message": "Processing messages"
        },
        {
          "name": "processor",
          "healthy": false,
          "message": "NATS connection lost",
          "details": []
        }
      ]
    }
  ]
}
```

**Recommendation**: **Enhance `/services/health` or create `/flows/{id}/runtime/health`**

Add to response:

- `start_time`: Component startup timestamp (for uptime calculation)
- `last_activity`: Last message processed timestamp
- `details`: Preserve existing hierarchical health structure

**Alternative**: Query Prometheus for uptime:

```promql
# Component uptime (seconds)
time() - component_start_time_seconds{component="udp-source"}
```

**Decision**: Start with `/services/health` as-is, add Prometheus uptime queries if available.

---

## Integration Architecture Decision

### Recommended Approach: Hybrid Pattern

**For Logs Tab**: Build new SSE endpoint

- **Why**: Message logger tracks NATS messages, not component logs
- **Endpoint**: `GET /flows/{id}/runtime/logs` (SSE)
- **Implementation**: Aggregate component logs, stream via SSE

**For Messages Tab**: Use Message Logger (MVP REQUIRED)

- **Why**: Tracking message flow through NATS is critical for debugging
- **Endpoint**: `GET /flows/{id}/runtime/messages` (SSE or polling)
- **Implementation**: Wrap `/message-logger/entries` with flow filtering
- **Value**: See message routing, latency, dropped messages, NATS subject activity

**For Metrics Tab**: Proxy to Prometheus + Health endpoint

- **Why**: Metrics already exist, avoid duplication
- **Endpoint**: `GET /flows/{id}/runtime/metrics` (JSON polling)
- **Implementation**: Backend queries Prometheus, formats for UI

**For Health Tab**: Use existing `/services/health` + optional Prometheus uptime

- **Why**: Health infrastructure already exists
- **Endpoint**: `GET /flows/{id}/runtime/health` (JSON polling)
- **Implementation**: Query `/services/health`, optionally augment with Prometheus uptime

---

## Implementation Plan (Updated)

### Phase 1: Metrics Tab (Use Existing Infrastructure) - MVP

**Backend**: Create Prometheus proxy endpoint

- Endpoint: `GET /flows/{id}/runtime/metrics`
- Query Prometheus for component metrics
- Query `/services/health` for status
- Return combined JSON

**Effort**: Low (2-4 hours)
**Risk**: Low (all data sources exist)

---

### Phase 2: Health Tab (Use Existing + Enhance) - MVP

**Backend**: Enhance health endpoint

- Endpoint: `GET /flows/{id}/runtime/health`
- Base data from `/services/health`
- Add `start_time` and `last_activity` fields
- Optional: Query Prometheus for uptime

**Effort**: Medium (4-6 hours)
**Risk**: Low (builds on existing health system)

---

### Phase 3: Messages Tab (Message Logger Integration) - MVP

**Backend**: Wrap message logger with flow filtering

- Endpoint: `GET /flows/{id}/runtime/messages` (SSE or polling)
- Filter `/message-logger/entries` by flow component subjects
- Add flow-specific filtering (only messages from flow components)
- Stream or poll message events
- **Critical**: Without this, debugging NATS message flow is extremely difficult

**Response Format**:

```json
{
  "timestamp": "2025-11-17T14:23:01Z",
  "messages": [
    {
      "timestamp": "2025-11-17T14:23:01.234Z",
      "subject": "process.udp-source.data",
      "message_id": "msg-12345",
      "component": "udp-source",
      "direction": "published",
      "summary": "UDP packet received: 128 bytes",
      "metadata": {
        "size": 128,
        "source_ip": "192.168.1.100"
      }
    }
  ]
}
```

**Effort**: Medium (4-6 hours)
**Risk**: Low (message logger already exists, just need filtering)

---

### Phase 4: Logs Tab (New SSE Endpoint) - MVP

**Backend**: Build log streaming endpoint

- Endpoint: `GET /flows/{id}/runtime/logs` (SSE)
- Add structured logging to components (level, component, message)
- Aggregate logs from flow components
- Stream via SSE with filtering support

**Effort**: High (8-12 hours)
**Risk**: Medium (new infrastructure, SSE complexity)

---

## Next Steps

1. **Create BD Issues for Backend Work**:
   - ✅ semstreams-0uh5: Marked complete with findings
   - ✅ semstreams-8cj7: Updated with backend integration plan
   - 🆕 Backend: Prometheus metrics proxy endpoint
   - 🆕 Backend: Health endpoint enhancement
   - 🆕 Backend: Message logger filtering endpoint (MVP)
   - 🆕 Backend: SSE logs streaming endpoint (MVP)
   - 📋 semstreams-zixp: Can be unblocked (clear API contracts defined)

2. **Update UI for 4 Tabs** (not 3):
   - Logs (component stdout/stderr)
   - Messages (NATS message flow) ← NEW
   - Metrics (Prometheus)
   - Health (component status)

3. **Implementation Order**:
   - **Phase 1**: Metrics tab (2-4 hours) - Lowest effort
   - **Phase 2**: Health tab (4-6 hours) - Builds on existing
   - **Phase 3**: Messages tab (4-6 hours) - Critical for NATS debugging
   - **Phase 4**: Logs tab (8-12 hours) - New infrastructure

4. **Total MVP Effort**: 18-28 hours (all 4 tabs)

5. **Architecture Decision**: Message logger is REQUIRED for MVP
   - Debugging NATS message flow without it is extremely difficult
   - Provides unique insight into message routing, latency, subject activity
   - Already exists, just needs flow-specific filtering

---

## Questions for Backend Team

1. What HTTP endpoints does service manager currently expose?
2. What Prometheus metrics are available? Per-component?
3. Does message logger exist? What does it track?
4. Are there NATS system subjects for component events?
5. What's the preferred observability pattern for semstreams?
6. Is there a monitoring dashboard already using these metrics?

---

## Success Metrics

- [ ] Clear understanding of existing infrastructure
- [ ] Decision made on Prometheus integration
- [ ] Decision made on message logger usage
- [ ] Minimal endpoint design (if needed)
- [ ] No duplicate functionality
- [ ] Backend team aligned on approach
- [ ] E2E tests unblocked with clear API contracts
