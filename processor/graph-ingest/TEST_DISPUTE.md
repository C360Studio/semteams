# Test Dispute: graph-ingest component_test.go

## Summary
The unit tests in `processor/graph-ingest/component_test.go` have incorrect assumptions about the natsclient API and NATS types.

## Issues

### 1. natsclient.MessageHandler doesn't exist
**Test assumes:**
```go
type mockNATSClient struct {
    subscribeFunc func(ctx context.Context, subject string, handler natsclient.MessageHandler) error
}
```

**Actual API:**
```go
// natsclient/client.go:623
func (m *Client) Subscribe(ctx context.Context, subject string, handler func(context.Context, []byte)) error
```

**Fix needed:** Change `natsclient.MessageHandler` to `func(context.Context, []byte)` in mock

### 2. mockKVEntry missing Bucket() method
**Test code:**
```go
type mockKVEntry struct {
    data []byte
}
```

**Actual interface requires:**
```go
// jetstream.KeyValueEntry requires:
Bucket() string  // MISSING in mockKVEntry
```

**Fix needed:** Add `Bucket() string` method to mockKVEntry

### 3. mockNATSClient doesn't match actual Client interface
**Test creates mocks with methods that don't exist or have wrong signatures**

The mock should match the actual natsclient.Client methods:
- JetStream() (jetstream.JetStream, error) ✓ OK
- Subscribe(ctx, subject, func(context.Context, []byte)) error ✗ WRONG
- Publish - doesn't exist in Client
- Request - doesn't exist in Client  
- GetConnection - doesn't exist in Client
- IsHealthy() bool ✓ OK

### 4. JetStream mock interface errors

Test references types that don't exist or have changed:
- `jetstream.JSOpt` (line 843, 867)
- `jetstream.ObjectStoreLister` (line 911)

## Root Cause

The test file appears to be generated based on an older or incorrect API specification. The natsclient.Client interface doesn't have the methods or signatures the test assumes.

## Recommendation

**Tester needs to:**
1. Update mockNATSClient to match actual natsclient.Client interface
2. Fix Subscribe handler signature from `natsclient.MessageHandler` to `func(context.Context, []byte)`
3. Add Bucket() method to mockKVEntry
4. Update JetStream mock to match current nats.go/jetstream interfaces
5. Remove mock methods that don't exist in real Client (Publish, Request, GetConnection)

**Spec says:** Component should implement Discoverable and LifecycleComponent

**Tests assume:** Specific natsclient API that doesn't match implementation

**Status:** Cannot proceed with implementation until tests are fixed. This is not a workaround situation - the test file has fundamental API mismatches.
