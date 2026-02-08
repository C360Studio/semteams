# Testing Patterns

Guidelines for writing unit tests, integration tests, and test helpers in SemStreams.

## Test Organization

```
project/
├── pkg/
│   └── example/
│       ├── example.go
│       └── example_test.go      # Unit tests
├── processor/
│   └── graph/
│       ├── processor.go
│       ├── processor_test.go    # Unit tests
│       └── integration_test.go  # Integration tests
└── test/
    └── e2e/                     # End-to-end tests
```

## Running Tests

```bash
# Run all unit tests
go test ./...

# Run with race detection
go test -race ./...

# Run specific package
go test ./processor/graph/...

# Run integration tests (requires Docker for testcontainers)
go test -tags=integration ./...

# Run integration tests with race detection
go test -race -tags=integration ./...

# Run with coverage
go test -cover ./...
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

## Table-Driven Tests

Use table-driven tests for comprehensive coverage:

```go
func TestConditionOperators(t *testing.T) {
    tests := []struct {
        name     string
        operator string
        value    any
        actual   any
        want     bool
    }{
        {
            name:     "eq string match",
            operator: "eq",
            value:    "active",
            actual:   "active",
            want:     true,
        },
        {
            name:     "eq string no match",
            operator: "eq",
            value:    "active",
            actual:   "inactive",
            want:     false,
        },
        {
            name:     "lt int true",
            operator: "lt",
            value:    20,
            actual:   15,
            want:     true,
        },
        {
            name:     "gt float true",
            operator: "gt",
            value:    50.0,
            actual:   75.5,
            want:     true,
        },
        {
            name:     "in slice match",
            operator: "in",
            value:    []string{"a", "b", "c"},
            actual:   "b",
            want:     true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            cond := Condition{
                Operator: tt.operator,
                Value:    tt.value,
            }
            got := cond.Evaluate(tt.actual)
            if got != tt.want {
                t.Errorf("got %v, want %v", got, tt.want)
            }
        })
    }
}
```

### Guidelines

- Each test case should be independent
- Use descriptive names that explain the scenario
- Test both success and failure cases
- Test edge cases (nil, empty, boundary values)

## Mock Implementations

### Mock KV Bucket

For testing KV-dependent code:

```go
type MockKV struct {
    data    map[string][]byte
    mu      sync.RWMutex
    history map[string][]kv.KeyValueEntry
}

func NewMockKV() *MockKV {
    return &MockKV{
        data:    make(map[string][]byte),
        history: make(map[string][]kv.KeyValueEntry),
    }
}

func (m *MockKV) Get(ctx context.Context, key string) (kv.KeyValueEntry, error) {
    m.mu.RLock()
    defer m.mu.RUnlock()

    data, ok := m.data[key]
    if !ok {
        return nil, kv.ErrKeyNotFound
    }
    return &MockEntry{key: key, value: data}, nil
}

func (m *MockKV) Put(ctx context.Context, key string, value []byte) (uint64, error) {
    m.mu.Lock()
    defer m.mu.Unlock()

    m.data[key] = value
    return 1, nil
}

func (m *MockKV) Delete(ctx context.Context, key string) error {
    m.mu.Lock()
    defer m.mu.Unlock()

    delete(m.data, key)
    return nil
}
```

### Mock Graph Provider

For testing clustering:

```go
type MockGraphProvider struct {
    entities  []string
    neighbors map[string][]string
    weights   map[string]float64
}

func NewMockGraphProvider() *MockGraphProvider {
    return &MockGraphProvider{
        neighbors: make(map[string][]string),
        weights:   make(map[string]float64),
    }
}

func (m *MockGraphProvider) GetAllEntityIDs(ctx context.Context) ([]string, error) {
    return m.entities, nil
}

func (m *MockGraphProvider) GetNeighbors(ctx context.Context, id, direction string) ([]string, error) {
    key := id + ":" + direction
    return m.neighbors[key], nil
}

func (m *MockGraphProvider) GetEdgeWeight(ctx context.Context, from, to string) (float64, error) {
    key := from + "->" + to
    if w, ok := m.weights[key]; ok {
        return w, nil
    }
    return 1.0, nil
}

// Helper methods for test setup
func (m *MockGraphProvider) AddEntity(id string) {
    m.entities = append(m.entities, id)
}

func (m *MockGraphProvider) AddEdge(from, to string, weight float64) {
    m.neighbors[from+":outgoing"] = append(m.neighbors[from+":outgoing"], to)
    m.neighbors[to+":incoming"] = append(m.neighbors[to+":incoming"], from)
    m.weights[from+"->"+to] = weight
}
```

## Integration Tests

### Build Tag Convention

Use `//go:build integration` for tests requiring external services like NATS:

```go
//go:build integration

package graph_test

import (
    "context"
    "testing"
    "time"

    "github.com/c360/semstreams/natsclient"
)

func TestGraphProcessorIntegration(t *testing.T) {
    // Create test client - testcontainers will start NATS automatically
    testClient := natsclient.NewTestClient(t,
        natsclient.WithJetStream(),
        natsclient.WithKV())
    natsClient := testClient.Client

    // Test with real NATS
    // ...
}
```

### Running Integration Tests

Integration tests are excluded from normal `go test ./...` runs. To run them:

```bash
# Run all integration tests
go test -tags=integration ./...

# Run with race detection (recommended)
go test -race -tags=integration ./...

# Run specific package integration tests
go test -tags=integration -v ./processor/graph/...
```

### Shared Test Client Pattern

For packages with multiple integration tests, use a shared NATS container via `TestMain`:

```go
//go:build integration

package mypackage

import (
    "log"
    "testing"
    "time"

    "github.com/c360/semstreams/natsclient"
)

var (
    sharedTestClient *natsclient.TestClient
    sharedNATSClient *natsclient.Client
)

func TestMain(m *testing.M) {
    // Build tag ensures this only runs with -tags=integration
    testClient, err := natsclient.NewSharedTestClient(
        natsclient.WithJetStream(),
        natsclient.WithKV(),
        natsclient.WithTestTimeout(5*time.Second),
        natsclient.WithStartTimeout(30*time.Second),
    )
    if err != nil {
        log.Fatalf("Failed to create shared test client: %v", err)
    }

    sharedTestClient = testClient
    sharedNATSClient = testClient.Client

    exitCode := m.Run()

    sharedTestClient.Terminate()

    if exitCode != 0 {
        log.Fatal("tests failed")
    }
}

func getSharedNATSClient(t *testing.T) *natsclient.Client {
    if sharedNATSClient == nil {
        t.Fatal("Shared NATS client not initialized")
    }
    return sharedNATSClient
}
```

## Concurrent Test Patterns

### Using Channels for Synchronization

```go
func TestConcurrentUpdates(t *testing.T) {
    processor := NewProcessor()
    done := make(chan struct{})
    errCh := make(chan error, 10)

    // Start concurrent workers
    for i := 0; i < 10; i++ {
        go func(id int) {
            defer func() { done <- struct{}{} }()

            err := processor.Update(ctx, fmt.Sprintf("entity-%d", id))
            if err != nil {
                errCh <- err
            }
        }(i)
    }

    // Wait for all workers
    for i := 0; i < 10; i++ {
        <-done
    }
    close(errCh)

    // Check for errors
    for err := range errCh {
        t.Errorf("worker error: %v", err)
    }
}
```

### Testing with WaitGroup

```go
func TestParallelProcessing(t *testing.T) {
    var wg sync.WaitGroup
    results := make([]string, 10)

    for i := 0; i < 10; i++ {
        wg.Add(1)
        go func(idx int) {
            defer wg.Done()
            results[idx] = process(idx)
        }(i)
    }

    wg.Wait()

    // Verify results
    for i, r := range results {
        if r == "" {
            t.Errorf("result[%d] is empty", i)
        }
    }
}
```

### Avoid Sleep in Tests

Instead of `time.Sleep`, use explicit synchronization:

```go
// BAD: Arbitrary sleep
func TestBad(t *testing.T) {
    go startWorker()
    time.Sleep(100 * time.Millisecond)  // Flaky!
    assertWorkerReady()
}

// GOOD: Explicit synchronization
func TestGood(t *testing.T) {
    ready := make(chan struct{})
    go startWorkerWithSignal(ready)

    select {
    case <-ready:
        assertWorkerReady()
    case <-time.After(1 * time.Second):
        t.Fatal("worker did not become ready")
    }
}
```

## Test Helpers

### Test Context with Timeout

```go
func testContext(t *testing.T) context.Context {
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    t.Cleanup(cancel)
    return ctx
}

func TestWithContext(t *testing.T) {
    ctx := testContext(t)

    result, err := processor.Process(ctx, input)
    // ...
}
```

### Test Entity Builder

```go
type EntityBuilder struct {
    entity *EntityState
}

func NewEntityBuilder(id string) *EntityBuilder {
    return &EntityBuilder{
        entity: &EntityState{
            ID:      id,
            Triples: []Triple{},
            Version: 1,
        },
    }
}

func (b *EntityBuilder) WithTriple(predicate string, object any) *EntityBuilder {
    b.entity.Triples = append(b.entity.Triples, Triple{
        Predicate: predicate,
        Object:    object,
    })
    return b
}

func (b *EntityBuilder) Build() *EntityState {
    return b.entity
}

// Usage
entity := NewEntityBuilder("sensor-001").
    WithTriple("entity.type", "sensor").
    WithTriple("sensor.temperature", 25.5).
    Build()
```

### Assert Helpers

```go
func assertEntityExists(t *testing.T, kv KVBucket, id string) {
    t.Helper()

    entry, err := kv.Get(context.Background(), id)
    if err != nil {
        t.Fatalf("entity %s not found: %v", id, err)
    }
    if entry == nil {
        t.Fatalf("entity %s is nil", id)
    }
}

func assertTripleValue(t *testing.T, entity *EntityState, predicate string, expected any) {
    t.Helper()

    for _, triple := range entity.Triples {
        if triple.Predicate == predicate {
            if triple.Object != expected {
                t.Errorf("triple %s: got %v, want %v", predicate, triple.Object, expected)
            }
            return
        }
    }
    t.Errorf("triple %s not found", predicate)
}
```

## Testing Graphable Implementations

When implementing the `Graphable` interface, test two key properties:

### EntityID Determinism

The same input must always produce the same ID:

```go
func TestSensorReading_EntityID(t *testing.T) {
    s := &SensorReading{
        DeviceID:   "sensor-042",
        SensorType: "temperature",
        OrgID:      "acme",
        Platform:   "logistics",
    }

    expected := "acme.logistics.environmental.sensor.temperature.sensor-042"
    assert.Equal(t, expected, s.EntityID())
}
```

### Triple Completeness

Verify all expected triples are generated:

```go
func TestSensorReading_Triples(t *testing.T) {
    s := &SensorReading{
        DeviceID:     "sensor-042",
        SensorType:   "temperature",
        Value:        23.5,
        Unit:         "celsius",
        ZoneEntityID: "acme.logistics.facility.zone.area.warehouse-7",
        OrgID:        "acme",
        Platform:     "logistics",
    }

    triples := s.Triples()

    // Find measurement triple
    var found bool
    for _, tr := range triples {
        if tr.Predicate == "sensor.measurement.celsius" {
            assert.Equal(t, 23.5, tr.Object)
            found = true
        }
    }
    assert.True(t, found, "measurement triple not found")
}
```

## Race Detection

Always run tests with race detection:

```bash
go test -race ./...
```

### Common Race Patterns

```go
// BAD: Race on shared state
type Counter struct {
    value int
}

func (c *Counter) Inc() {
    c.value++  // Race!
}

// GOOD: Use atomic or mutex
type Counter struct {
    value atomic.Int64
}

func (c *Counter) Inc() {
    c.value.Add(1)
}
```

## Benchmark Tests

```go
func BenchmarkConditionEvaluate(b *testing.B) {
    cond := Condition{
        Field:    "battery.level",
        Operator: "lt",
        Value:    20,
    }
    entity := &EntityState{
        Triples: []Triple{{Predicate: "battery.level", Object: 15}},
    }

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        cond.Evaluate(entity)
    }
}

func BenchmarkLPADetection(b *testing.B) {
    // Setup large graph
    provider := setupLargeGraph(1000)
    detector := NewLPADetector(provider, NewMockStorage())

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _, _ = detector.DetectCommunities(context.Background())
    }
}
```

Run benchmarks:

```bash
go test -bench=. ./processor/graph/clustering/
go test -bench=BenchmarkLPA -benchmem ./...
```

## Coverage

### Generate Coverage Report

```bash
# Generate coverage profile
go test -coverprofile=coverage.out ./...

# View in browser
go tool cover -html=coverage.out

# Show function coverage
go tool cover -func=coverage.out
```

### Coverage Guidelines

- **Critical paths**: 80%+ coverage
- **Edge cases**: Always test nil, empty, boundary values
- **Error paths**: Test error handling

## Test Organization Tips

### Naming Conventions

```go
// Test<Function>_<Scenario>
func TestEvaluate_StringEquality(t *testing.T) {}
func TestEvaluate_NumericLessThan(t *testing.T) {}
func TestEvaluate_NilValue(t *testing.T) {}

// Test<Type>_<Method>_<Scenario>
func TestCondition_Evaluate_MatchingValue(t *testing.T) {}
func TestCondition_Evaluate_NonMatchingValue(t *testing.T) {}
```

### Test File Organization

```go
// condition_test.go

// === Helpers ===

func newTestCondition(op string, val any) *Condition { ... }

// === Unit Tests ===

func TestCondition_Evaluate(t *testing.T) { ... }
func TestCondition_Validate(t *testing.T) { ... }

// === Integration Tests ===

func TestCondition_WithRealEntities(t *testing.T) { ... }

// === Benchmarks ===

func BenchmarkCondition_Evaluate(b *testing.B) { ... }
```

## Agentic E2E Tests

The agentic tier validates the LLM-powered agent loop with tool execution.

### Running Agentic Tests

```bash
# Run agentic tier (~30s)
task e2e:agentic

# Start services for manual testing
task e2e:agentic:up

# Run tests against already-running services
task e2e:agentic:test

# View logs
task e2e:agentic:logs

# Clean up
task e2e:agentic:clean
```

### Mock LLM Server

The agentic E2E tests use a mock LLM server that simulates OpenAI-compatible responses:

- Responds to `/v1/chat/completions` with predetermined responses
- Simulates tool calls when prompted
- Returns configurable delays for latency testing

### Test Scenarios

The agentic tests validate:

1. **Loop Creation**: Task message spawns new loop
2. **Tool Execution**: Model requests tools, tools execute, results returned
3. **State Machine**: Loop progresses through states correctly
4. **Completion**: Loop completes and trajectory is saved
5. **Rule Trigger**: Rules can spawn agent tasks

### Debugging Agentic E2E Failures

```bash
# Check loop state
nats kv get AGENT_LOOPS <loop_id>

# Check trajectory
nats kv get AGENT_TRAJECTORIES <loop_id>

# View mock LLM logs
docker logs mock-llm

# View agentic component logs
docker logs semstreams-agentic-app 2>&1 | grep agentic
```

### Common Issues

| Issue | Cause | Fix |
|-------|-------|-----|
| Loop never starts | Task message not published | Check rule trigger conditions |
| Tool not found | Executor not registered | Verify tool registration in init() |
| Model timeout | Mock LLM not responding | Restart mock-llm container |
| State stuck | Pending tools never resolve | Check agentic-tools logs |

## Related Documentation

- [E2E Tests](02-e2e-tests.md) - End-to-end testing
- [Agentic Quickstart](../basics/07-agentic-quickstart.md) - Getting started with agents
- [Troubleshooting](../operations/02-troubleshooting.md) - Common issues
