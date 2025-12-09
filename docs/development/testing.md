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

# Run integration tests (requires NATS)
INTEGRATION_TESTS=1 go test ./processor/graph/...

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

Use `//go:build integration` for tests requiring external services:

```go
//go:build integration

package graph_test

import (
    "context"
    "testing"
    "time"

    "github.com/nats-io/nats.go"
)

func TestGraphProcessorIntegration(t *testing.T) {
    // Skip if NATS not available
    nc, err := nats.Connect(nats.DefaultURL)
    if err != nil {
        t.Skip("NATS not available")
    }
    defer nc.Close()

    // Test with real NATS
    // ...
}
```

### Environment Variable Check

Alternative approach:

```go
func TestIntegration(t *testing.T) {
    if os.Getenv("INTEGRATION_TESTS") != "1" {
        t.Skip("skipping integration test")
    }
    // ...
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

## Related Documentation

- [E2E Tests](e2e-tests.md) - End-to-end testing
- [Go Standards](../agent/go.md) - Go development standards
