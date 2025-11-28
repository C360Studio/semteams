// Package rule - Mock KV implementation for testing
package rule

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

// mockKVBucket is a simple in-memory KV bucket for testing
type mockKVBucket struct {
	mu       sync.RWMutex
	data     map[string][]byte
	revision map[string]uint64
	nextRev  uint64
}

// newMockKVBucket creates a new mock KV bucket
func newMockKVBucket() *mockKVBucket {
	return &mockKVBucket{
		data:     make(map[string][]byte),
		revision: make(map[string]uint64),
		nextRev:  1,
	}
}

// Get retrieves a value from the bucket
func (m *mockKVBucket) Get(_ context.Context, key string) (jetstream.KeyValueEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	val, ok := m.data[key]
	if !ok {
		return nil, jetstream.ErrKeyNotFound
	}

	// Create a copy to prevent races
	valCopy := make([]byte, len(val))
	copy(valCopy, val)

	return &mockKVEntry{
		key:      key,
		value:    valCopy,
		revision: m.revision[key],
		created:  time.Now(),
	}, nil
}

// Put stores a value in the bucket
func (m *mockKVBucket) Put(_ context.Context, key string, value []byte) (uint64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Create a copy to prevent races
	valCopy := make([]byte, len(value))
	copy(valCopy, value)

	m.data[key] = valCopy
	m.revision[key] = m.nextRev
	rev := m.nextRev
	m.nextRev++

	return rev, nil
}

// Delete removes a key from the bucket
func (m *mockKVBucket) Delete(_ context.Context, key string, _ ...jetstream.KVDeleteOpt) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.data[key]; !ok {
		return jetstream.ErrKeyNotFound
	}

	delete(m.data, key)
	delete(m.revision, key)
	return nil
}

// Keys returns all keys in the bucket (deprecated, use ListKeys)
func (m *mockKVBucket) Keys(_ context.Context, _ ...jetstream.WatchOpt) ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	keys := make([]string, 0, len(m.data))
	for k := range m.data {
		keys = append(keys, k)
	}
	return keys, nil
}

// ListKeys returns a KeyLister for all keys
func (m *mockKVBucket) ListKeys(_ context.Context, _ ...jetstream.WatchOpt) (jetstream.KeyLister, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	keys := make([]string, 0, len(m.data))
	for k := range m.data {
		keys = append(keys, k)
	}
	return &mockKeyLister{keys: keys}, nil
}

// ListKeysFiltered returns a KeyLister for filtered keys
func (m *mockKVBucket) ListKeysFiltered(ctx context.Context, _ ...string) (jetstream.KeyLister, error) {
	return m.ListKeys(ctx)
}

// Stub implementations for remaining jetstream.KeyValue interface methods
func (m *mockKVBucket) GetRevision(_ context.Context, _ string, _ uint64) (jetstream.KeyValueEntry, error) {
	return nil, errors.New("not implemented")
}

func (m *mockKVBucket) PutString(ctx context.Context, key string, value string) (uint64, error) {
	return m.Put(ctx, key, []byte(value))
}

func (m *mockKVBucket) Create(_ context.Context, _ string, _ []byte, _ ...jetstream.KVCreateOpt) (uint64, error) {
	return 0, errors.New("not implemented")
}

func (m *mockKVBucket) Update(_ context.Context, _ string, _ []byte, _ uint64) (uint64, error) {
	return 0, errors.New("not implemented")
}

func (m *mockKVBucket) Purge(_ context.Context, _ string, _ ...jetstream.KVDeleteOpt) error {
	return errors.New("not implemented")
}

func (m *mockKVBucket) Watch(_ context.Context, _ string, _ ...jetstream.WatchOpt) (jetstream.KeyWatcher, error) {
	return nil, errors.New("not implemented")
}

func (m *mockKVBucket) WatchAll(_ context.Context, _ ...jetstream.WatchOpt) (jetstream.KeyWatcher, error) {
	return nil, errors.New("not implemented")
}

func (m *mockKVBucket) WatchFiltered(_ context.Context, _ []string, _ ...jetstream.WatchOpt) (jetstream.KeyWatcher, error) {
	return nil, errors.New("not implemented")
}

func (m *mockKVBucket) History(_ context.Context, _ string, _ ...jetstream.WatchOpt) ([]jetstream.KeyValueEntry, error) {
	return nil, errors.New("not implemented")
}

func (m *mockKVBucket) Bucket() string {
	return "mock-bucket"
}

func (m *mockKVBucket) PurgeDeletes(_ context.Context, _ ...jetstream.KVPurgeOpt) error {
	return errors.New("not implemented")
}

func (m *mockKVBucket) Status(_ context.Context) (jetstream.KeyValueStatus, error) {
	return nil, errors.New("not implemented")
}

// mockKVEntry implements jetstream.KeyValueEntry
type mockKVEntry struct {
	key      string
	value    []byte
	revision uint64
	created  time.Time
}

func (e *mockKVEntry) Bucket() string {
	return "mock-bucket"
}

func (e *mockKVEntry) Key() string {
	return e.key
}

func (e *mockKVEntry) Value() []byte {
	return e.value
}

func (e *mockKVEntry) Revision() uint64 {
	return e.revision
}

func (e *mockKVEntry) Created() time.Time {
	return e.created
}

func (e *mockKVEntry) Delta() uint64 {
	return 0
}

func (e *mockKVEntry) Operation() jetstream.KeyValueOp {
	return jetstream.KeyValuePut
}

// mockKeyLister implements jetstream.KeyLister
type mockKeyLister struct {
	keys []string
	ch   chan string
}

func (l *mockKeyLister) Keys() <-chan string {
	if l.ch == nil {
		l.ch = make(chan string, len(l.keys))
		for _, k := range l.keys {
			l.ch <- k
		}
		close(l.ch)
	}
	return l.ch
}

func (l *mockKeyLister) Stop() error {
	return nil
}
