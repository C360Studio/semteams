# Go Patterns Reference

Shared patterns for all agents. Read relevant sections before writing code.

---

## Table-Driven Tests

```go
func TestValidateName(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        wantErr error
    }{
        {name: "valid", input: "my-stream", wantErr: nil},
        {name: "empty", input: "", wantErr: ErrInvalidName},
        {name: "too long", input: strings.Repeat("a", 257), wantErr: ErrInvalidName},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := ValidateName(tt.input)
            if tt.wantErr != nil {
                require.ErrorIs(t, err, tt.wantErr)
            } else {
                require.NoError(t, err)
            }
        })
    }
}
```

---

## Test Helpers

```go
func setupTestManager(t *testing.T) *StreamManager {
    t.Helper()
    
    db := setupTestDB(t)
    t.Cleanup(func() { db.Close() })
    
    return NewStreamManager(db)
}
```

---

## Parallel Tests

```go
func TestOperations(t *testing.T) {
    t.Parallel()

    t.Run("Create", func(t *testing.T) {
        t.Parallel()
        // ...
    })
}
```

**Don't parallelize** when tests share state.

---

## Context Testing

```go
func TestCreate_RespectsContext(t *testing.T) {
    mgr := setupTestManager(t)
    
    t.Run("cancelled", func(t *testing.T) {
        ctx, cancel := context.WithCancel(context.Background())
        cancel()
        
        _, err := mgr.Create(ctx, validConfig)
        require.ErrorIs(t, err, context.Canceled)
    })
    
    t.Run("timeout", func(t *testing.T) {
        ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
        defer cancel()
        time.Sleep(time.Millisecond)
        
        _, err := mgr.Create(ctx, validConfig)
        require.Error(t, err)
    })
}
```

---

## Error Testing

```go
// Test specific error types
require.ErrorIs(t, err, ErrNotFound)

// Test error message content
require.ErrorContains(t, err, "stream 'foo'")

// Test wrapped errors
var notFoundErr *NotFoundError
require.ErrorAs(t, err, &notFoundErr)
```

---

## Concurrency Tests

```go
func TestConcurrentCreate(t *testing.T) {
    mgr := setupTestManager(t)
    cfg := StreamConfig{Name: "contested"}
    
    const n = 10
    results := make(chan error, n)
    
    var wg sync.WaitGroup
    wg.Add(n)
    for i := 0; i < n; i++ {
        go func() {
            defer wg.Done()
            _, err := mgr.Create(context.Background(), cfg)
            results <- err
        }()
    }
    wg.Wait()
    close(results)
    
    var successes, conflicts int
    for err := range results {
        if err == nil {
            successes++
        } else if errors.Is(err, ErrStreamExists) {
            conflicts++
        } else {
            t.Errorf("unexpected: %v", err)
        }
    }
    
    assert.Equal(t, 1, successes)
    assert.Equal(t, n-1, conflicts)
}
```

---

## Race-Safe Test Patterns

```go
// BAD: Race condition
var result string
go func() { result = doThing() }()
time.Sleep(time.Second)
assert.Equal(t, "expected", result)  // RACE

// GOOD: Channel synchronization
resultCh := make(chan string, 1)
go func() { resultCh <- doThing() }()
select {
case result := <-resultCh:
    assert.Equal(t, "expected", result)
case <-time.After(5 * time.Second):
    t.Fatal("timeout")
}
```

---

## Integration Test Structure

```go
//go:build integration

package streams_test

func TestIntegration(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping integration test")
    }
    
    nc := setupTestNATS(t)
    js := setupJetStream(t, nc)
    mgr := NewStreamManager(js)
    
    t.Run("create with real NATS", func(t *testing.T) {
        id, err := mgr.Create(ctx, cfg)
        require.NoError(t, err)
        
        info, err := js.StreamInfo(cfg.Name)
        require.NoError(t, err)
        require.NotNil(t, info)
    })
}
```

---

## Context Propagation

```go
// Context is always first parameter
func (m *Manager) Create(ctx context.Context, cfg Config) (ID, error)

// Pass to all blocking operations
result, err := m.db.ExecContext(ctx, query, args...)
if err := m.nats.PublishCtx(ctx, subject, data); err != nil { ... }

// Check in loops
for _, item := range items {
    select {
    case <-ctx.Done():
        return ctx.Err()
    default:
    }
    process(ctx, item)
}
```

---

## Error Handling

```go
// Always wrap with context
if err != nil {
    return fmt.Errorf("create stream %q: %w", name, err)
}

// Sentinel errors for expected conditions
var (
    ErrNotFound = errors.New("not found")
    ErrExists   = errors.New("already exists")
)

// Check and wrap
if errors.Is(err, sql.ErrNoRows) {
    return nil, fmt.Errorf("stream %q: %w", id, ErrNotFound)
}

// Never discard errors
if err := file.Close(); err != nil {
    // Log or return
}
```

---

## Resource Management

```go
func (m *Manager) Process(ctx context.Context) error {
    conn, err := m.pool.Acquire(ctx)
    if err != nil {
        return fmt.Errorf("acquire: %w", err)
    }
    defer conn.Release()
    
    tx, err := conn.Begin(ctx)
    if err != nil {
        return fmt.Errorf("begin: %w", err)
    }
    defer tx.Rollback()  // No-op if committed
    
    // ... work ...
    
    return tx.Commit()
}
```

---

## Goroutine Patterns

```go
// Clear termination via context
func (m *Manager) StartWorker(ctx context.Context) {
    go func() {
        for {
            select {
            case <-ctx.Done():
                return
            case item := <-m.queue:
                m.process(ctx, item)
            }
        }
    }()
}

// Channel closed by sender
func produce(ctx context.Context) <-chan Item {
    ch := make(chan Item)
    go func() {
        defer close(ch)  // Sender closes
        for {
            select {
            case <-ctx.Done():
                return
            case ch <- generate():
            }
        }
    }()
    return ch
}

// WaitGroup.Add before goroutine
var wg sync.WaitGroup
for i := 0; i < n; i++ {
    wg.Add(1)  // BEFORE
    go func() {
        defer wg.Done()
        work()
    }()
}
wg.Wait()
```

---

## Mutex Patterns

```go
// Small critical sections
func (m *Manager) Get(id string) *Stream {
    m.mu.RLock()
    stream := m.streams[id]
    m.mu.RUnlock()
    return stream
}

// RWMutex for read-heavy
type Cache struct {
    mu    sync.RWMutex
    items map[string]Item
}

func (c *Cache) Get(key string) (Item, bool) {
    c.mu.RLock()
    defer c.mu.RUnlock()
    item, ok := c.items[key]
    return item, ok
}
```

---

## Interface Design

```go
// Accept interfaces, return structs
func NewManager(store StreamStore) *Manager {
    return &Manager{store: store}
}

// Small, focused interfaces
type Reader interface {
    Read(ctx context.Context, id string) (*Stream, error)
}

type Writer interface {
    Write(ctx context.Context, s *Stream) error
}
```

---

## Validation

```go
func (m *Manager) Create(ctx context.Context, cfg Config) (string, error) {
    if err := cfg.Validate(); err != nil {
        return "", fmt.Errorf("invalid config: %w", err)
    }
    // ... proceed
}

func (c Config) Validate() error {
    if c.Name == "" {
        return fmt.Errorf("name: %w", ErrRequired)
    }
    if len(c.Name) > 256 {
        return fmt.Errorf("name: %w", ErrTooLong)
    }
    return nil
}
```

---

## Attack Test Patterns

### Nil/Zero Inputs

```go
func TestAttack_NilConfig(t *testing.T) {
    mgr := setupTestManager(t)
    require.NotPanics(t, func() {
        _, _ = mgr.Create(context.Background(), nil)
    })
}
```

### Context Attacks

```go
func TestAttack_CancelledContext(t *testing.T) {
    ctx, cancel := context.WithCancel(context.Background())
    cancel()
    
    done := make(chan struct{})
    go func() {
        defer close(done)
        _, _ = mgr.Create(ctx, validConfig)
    }()
    
    select {
    case <-done:
    case <-time.After(5 * time.Second):
        t.Fatal("hung on cancelled context")
    }
}
```

### Goroutine Leaks

```go
func TestAttack_NoGoroutineLeak(t *testing.T) {
    before := runtime.NumGoroutine()
    
    mgr := setupTestManager(t)
    for i := 0; i < 100; i++ {
        ctx, cancel := context.WithCancel(context.Background())
        _, _ = mgr.Create(ctx, Config{Name: fmt.Sprintf("s-%d", i)})
        cancel()
    }
    mgr.Close()
    
    time.Sleep(100 * time.Millisecond)
    after := runtime.NumGoroutine()
    
    require.LessOrEqual(t, after, before+5, "leak: %d→%d", before, after)
}
```

### Concurrent Access

```go
func TestAttack_ConcurrentReadWrite(t *testing.T) {
    mgr := setupTestManager(t)
    id, _ := mgr.Create(context.Background(), validConfig)
    
    var wg sync.WaitGroup
    for i := 0; i < 100; i++ {
        wg.Add(2)
        go func() { defer wg.Done(); mgr.Get(ctx, id) }()
        go func() { defer wg.Done(); mgr.Update(ctx, id, cfg) }()
    }
    wg.Wait()
}
```

### Large Inputs

```go
func TestAttack_MassiveInput(t *testing.T) {
    _, err := mgr.Create(ctx, Config{Name: strings.Repeat("a", 1_000_000)})
    require.Error(t, err)
}
```

---

## Review Checklist

### Context

- [ ] First parameter on public functions
- [ ] Passed to all blocking operations
- [ ] Cancellation checked in loops
- [ ] No `context.Background()` in library code

### Errors

- [ ] All errors checked
- [ ] Wrapped with context (`%w`)
- [ ] Sentinel errors for expected conditions
- [ ] Actionable messages

### Concurrency

- [ ] Goroutines have termination conditions
- [ ] Channels closed by sender
- [ ] `WaitGroup.Add()` before goroutine
- [ ] Small mutex critical sections

### Resources

- [ ] `defer` for cleanup
- [ ] Bounded pools
- [ ] Timeouts on external calls

### Security

- [ ] No sensitive data in logs
- [ ] Parameterized SQL
- [ ] Input validated
- [ ] No unbounded reads
