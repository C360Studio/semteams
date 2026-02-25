// Package testutil provides testing utilities for the reactive workflow engine.
package testutil

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

// InMemoryKV implements a mock KV store for testing.
// It supports basic CRUD operations and watch functionality.
type InMemoryKV struct {
	mu       sync.RWMutex
	name     string
	data     map[string]*kvEntry
	revision uint64
	watchers []chan kvEvent
}

type kvEntry struct {
	key       string
	value     []byte
	revision  uint64
	created   time.Time
	operation jetstream.KeyValueOp
}

type kvEvent struct {
	entry *kvEntry
}

// NewInMemoryKV creates a new in-memory KV store.
func NewInMemoryKV(name string) *InMemoryKV {
	return &InMemoryKV{
		name:     name,
		data:     make(map[string]*kvEntry),
		revision: 0,
		watchers: make([]chan kvEvent, 0),
	}
}

// Bucket returns the bucket name.
func (kv *InMemoryKV) Bucket() string {
	return kv.name
}

// Get retrieves a value by key.
func (kv *InMemoryKV) Get(_ context.Context, key string) (jetstream.KeyValueEntry, error) {
	kv.mu.RLock()
	defer kv.mu.RUnlock()

	entry, ok := kv.data[key]
	if !ok || entry.operation == jetstream.KeyValueDelete {
		return nil, jetstream.ErrKeyNotFound
	}

	return &mockKVEntry{entry: entry}, nil
}

// Put stores a value.
func (kv *InMemoryKV) Put(_ context.Context, key string, value []byte) (uint64, error) {
	kv.mu.Lock()
	defer kv.mu.Unlock()

	kv.revision++
	entry := &kvEntry{
		key:       key,
		value:     value,
		revision:  kv.revision,
		created:   time.Now(),
		operation: jetstream.KeyValuePut,
	}
	kv.data[key] = entry

	// Notify watchers
	kv.notifyWatchers(entry)

	return kv.revision, nil
}

// Create stores a value only if the key doesn't exist.
func (kv *InMemoryKV) Create(_ context.Context, key string, value []byte) (uint64, error) {
	kv.mu.Lock()
	defer kv.mu.Unlock()

	if existing, ok := kv.data[key]; ok && existing.operation != jetstream.KeyValueDelete {
		return 0, errors.New("key already exists")
	}

	kv.revision++
	entry := &kvEntry{
		key:       key,
		value:     value,
		revision:  kv.revision,
		created:   time.Now(),
		operation: jetstream.KeyValuePut,
	}
	kv.data[key] = entry

	// Notify watchers
	kv.notifyWatchers(entry)

	return kv.revision, nil
}

// Update stores a value only if the revision matches.
func (kv *InMemoryKV) Update(_ context.Context, key string, value []byte, revision uint64) (uint64, error) {
	kv.mu.Lock()
	defer kv.mu.Unlock()

	existing, ok := kv.data[key]
	if !ok {
		return 0, jetstream.ErrKeyNotFound
	}
	if existing.revision != revision {
		return 0, errors.New("revision mismatch")
	}

	kv.revision++
	entry := &kvEntry{
		key:       key,
		value:     value,
		revision:  kv.revision,
		created:   existing.created,
		operation: jetstream.KeyValuePut,
	}
	kv.data[key] = entry

	// Notify watchers
	kv.notifyWatchers(entry)

	return kv.revision, nil
}

// Delete removes a key.
func (kv *InMemoryKV) Delete(_ context.Context, key string, _ ...jetstream.KVDeleteOpt) error {
	kv.mu.Lock()
	defer kv.mu.Unlock()

	if _, ok := kv.data[key]; !ok {
		return nil
	}

	kv.revision++
	entry := &kvEntry{
		key:       key,
		value:     nil,
		revision:  kv.revision,
		created:   time.Now(),
		operation: jetstream.KeyValueDelete,
	}
	kv.data[key] = entry

	// Notify watchers
	kv.notifyWatchers(entry)

	return nil
}

// Purge removes all values.
func (kv *InMemoryKV) Purge(_ context.Context, key string, _ ...jetstream.KVDeleteOpt) error {
	return kv.Delete(context.Background(), key)
}

// Watch starts watching for changes to keys matching the pattern.
func (kv *InMemoryKV) Watch(_ context.Context, keys string, _ ...jetstream.WatchOpt) (jetstream.KeyWatcher, error) {
	kv.mu.Lock()
	defer kv.mu.Unlock()

	ch := make(chan kvEvent, 100)
	kv.watchers = append(kv.watchers, ch)

	return &mockKeyWatcher{
		kv:      kv,
		ch:      ch,
		pattern: keys,
	}, nil
}

// WatchAll watches all keys.
func (kv *InMemoryKV) WatchAll(_ context.Context, _ ...jetstream.WatchOpt) (jetstream.KeyWatcher, error) {
	return kv.Watch(context.Background(), ">")
}

// Keys returns all keys.
func (kv *InMemoryKV) Keys(_ context.Context, _ ...jetstream.WatchOpt) ([]string, error) {
	kv.mu.RLock()
	defer kv.mu.RUnlock()

	keys := make([]string, 0, len(kv.data))
	for k, entry := range kv.data {
		if entry.operation != jetstream.KeyValueDelete {
			keys = append(keys, k)
		}
	}
	return keys, nil
}

// History returns the history for a key (simplified - returns current only).
func (kv *InMemoryKV) History(_ context.Context, key string, _ ...jetstream.WatchOpt) ([]jetstream.KeyValueEntry, error) {
	kv.mu.RLock()
	defer kv.mu.RUnlock()

	entry, ok := kv.data[key]
	if !ok {
		return nil, jetstream.ErrKeyNotFound
	}

	return []jetstream.KeyValueEntry{&mockKVEntry{entry: entry}}, nil
}

// Status returns the bucket status.
func (kv *InMemoryKV) Status(_ context.Context) (jetstream.KeyValueStatus, error) {
	return &mockKVStatus{name: kv.name}, nil
}

// ListKeys returns a channel of keys (for iteration).
func (kv *InMemoryKV) ListKeys(_ context.Context, _ ...jetstream.WatchOpt) (jetstream.KeyLister, error) {
	keys, _ := kv.Keys(context.Background())
	return &mockKeyLister{keys: keys}, nil
}

// notifyWatchers sends an event to all watchers.
func (kv *InMemoryKV) notifyWatchers(entry *kvEntry) {
	for _, ch := range kv.watchers {
		select {
		case ch <- kvEvent{entry: entry}:
		default:
			// Channel full, skip
		}
	}
}

// removeWatcher removes a watcher channel.
func (kv *InMemoryKV) removeWatcher(ch chan kvEvent) {
	kv.mu.Lock()
	defer kv.mu.Unlock()

	for i, w := range kv.watchers {
		if w == ch {
			kv.watchers = append(kv.watchers[:i], kv.watchers[i+1:]...)
			break
		}
	}
}

// GetData returns a copy of all data for inspection.
func (kv *InMemoryKV) GetData() map[string][]byte {
	kv.mu.RLock()
	defer kv.mu.RUnlock()

	result := make(map[string][]byte, len(kv.data))
	for k, entry := range kv.data {
		if entry.operation != jetstream.KeyValueDelete {
			result[k] = entry.value
		}
	}
	return result
}

// GetEntry returns a specific entry for inspection.
func (kv *InMemoryKV) GetEntry(key string) ([]byte, uint64, bool) {
	kv.mu.RLock()
	defer kv.mu.RUnlock()

	entry, ok := kv.data[key]
	if !ok || entry.operation == jetstream.KeyValueDelete {
		return nil, 0, false
	}
	return entry.value, entry.revision, true
}

// Clear removes all data.
func (kv *InMemoryKV) Clear() {
	kv.mu.Lock()
	defer kv.mu.Unlock()

	kv.data = make(map[string]*kvEntry)
	kv.revision = 0
}

// mockKVEntry implements jetstream.KeyValueEntry.
type mockKVEntry struct {
	entry *kvEntry
}

func (e *mockKVEntry) Bucket() string                  { return "" }
func (e *mockKVEntry) Key() string                     { return e.entry.key }
func (e *mockKVEntry) Value() []byte                   { return e.entry.value }
func (e *mockKVEntry) Revision() uint64                { return e.entry.revision }
func (e *mockKVEntry) Created() time.Time              { return e.entry.created }
func (e *mockKVEntry) Delta() uint64                   { return 0 }
func (e *mockKVEntry) Operation() jetstream.KeyValueOp { return e.entry.operation }

// mockKeyWatcher implements jetstream.KeyWatcher.
type mockKeyWatcher struct {
	kv      *InMemoryKV
	ch      chan kvEvent
	pattern string
	stopped bool
}

func (w *mockKeyWatcher) Updates() <-chan jetstream.KeyValueEntry {
	out := make(chan jetstream.KeyValueEntry, 100)
	go func() {
		defer close(out)
		for event := range w.ch {
			if w.stopped {
				return
			}
			out <- &mockKVEntry{entry: event.entry}
		}
	}()
	return out
}

func (w *mockKeyWatcher) Stop() error {
	w.stopped = true
	w.kv.removeWatcher(w.ch)
	close(w.ch)
	return nil
}

// mockKVStatus implements jetstream.KeyValueStatus.
type mockKVStatus struct {
	name string
}

func (s *mockKVStatus) Bucket() string                    { return s.name }
func (s *mockKVStatus) Values() uint64                    { return 0 }
func (s *mockKVStatus) History() int64                    { return 1 }
func (s *mockKVStatus) TTL() time.Duration                { return 0 }
func (s *mockKVStatus) BackingStore() string              { return "memory" }
func (s *mockKVStatus) Bytes() uint64                     { return 0 }
func (s *mockKVStatus) IsCompressed() bool                { return false }
func (s *mockKVStatus) StreamInfo() *jetstream.StreamInfo { return nil }
func (s *mockKVStatus) LimitMarkerTTL() time.Duration     { return 0 }
func (s *mockKVStatus) Metadata() map[string]string       { return nil }

// mockKeyLister implements jetstream.KeyLister.
type mockKeyLister struct {
	keys []string
	idx  int
}

func (l *mockKeyLister) Keys() <-chan string {
	ch := make(chan string, len(l.keys))
	go func() {
		defer close(ch)
		for _, k := range l.keys {
			ch <- k
		}
	}()
	return ch
}

func (l *mockKeyLister) Stop() error { return nil }

// PutJSON is a helper that marshals and stores a value.
func (kv *InMemoryKV) PutJSON(ctx context.Context, key string, v any) (uint64, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return 0, err
	}
	return kv.Put(ctx, key, data)
}

// GetJSON is a helper that retrieves and unmarshals a value.
func (kv *InMemoryKV) GetJSON(ctx context.Context, key string, v any) error {
	entry, err := kv.Get(ctx, key)
	if err != nil {
		return err
	}
	return json.Unmarshal(entry.Value(), v)
}
