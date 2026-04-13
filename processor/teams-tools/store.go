package teamtools

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/c360studio/semstreams/natsclient"
	"github.com/nats-io/nats.go/jetstream"
)

// KVToolCallStore implements ToolCallStore using NATS KV.
// It supports lazy initialization with retry logic and can be reset
// to allow re-initialization after failures.
type KVToolCallStore struct {
	client     *natsclient.Client
	bucketName string
	kv         jetstream.KeyValue
	mu         sync.RWMutex
	closed     bool
}

// NewKVToolCallStore creates a new KV-backed tool call store.
// Initialization is deferred until the first Store call.
func NewKVToolCallStore(client *natsclient.Client, bucketName string) *KVToolCallStore {
	return &KVToolCallStore{
		client:     client,
		bucketName: bucketName,
	}
}

// ensureInitialized lazily initializes the KV bucket with retry logic.
// It retries up to 3 times with exponential backoff.
func (s *KVToolCallStore) ensureInitialized(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.kv != nil {
		return nil
	}
	if s.closed {
		return fmt.Errorf("store is closed")
	}

	// Retry initialization up to 3 times with backoff
	var lastErr error
	for i := 0; i < 3; i++ {
		kv, err := s.client.CreateKeyValueBucket(ctx, jetstream.KeyValueConfig{
			Bucket: s.bucketName,
		})
		if err == nil {
			s.kv = kv
			return nil
		}
		lastErr = err

		// Check context before sleeping
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(i+1) * 100 * time.Millisecond):
		}
	}
	return fmt.Errorf("failed to initialize store after 3 attempts: %w", lastErr)
}

// Store persists a tool call record to the KV store.
// The key format is "{call_id}.{timestamp_ns}" to allow multiple records
// per call ID (e.g., retries).
func (s *KVToolCallStore) Store(ctx context.Context, record ToolCallRecord) error {
	if err := s.ensureInitialized(ctx); err != nil {
		return err
	}

	data, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("marshal record: %w", err)
	}

	key := fmt.Sprintf("%s.%d", record.Call.ID, record.StartTime.UnixNano())
	_, err = s.kv.Put(ctx, key, data)
	if err != nil {
		return fmt.Errorf("put record: %w", err)
	}

	return nil
}

// Close marks the store as closed. The underlying KV connection is managed
// by the natsclient and should not be closed here.
func (s *KVToolCallStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	s.kv = nil
	return nil
}

// Reset allows re-initialization after a failure.
// This is useful for testing or recovery scenarios.
func (s *KVToolCallStore) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.kv = nil
	s.closed = false
}

// InMemoryToolCallStore provides an in-memory implementation of ToolCallStore
// for testing purposes.
type InMemoryToolCallStore struct {
	records []ToolCallRecord
	mu      sync.Mutex
	closed  bool
}

// NewInMemoryToolCallStore creates a new in-memory store for testing.
func NewInMemoryToolCallStore() *InMemoryToolCallStore {
	return &InMemoryToolCallStore{
		records: make([]ToolCallRecord, 0),
	}
}

// Store adds a record to the in-memory store.
func (s *InMemoryToolCallStore) Store(_ context.Context, record ToolCallRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return fmt.Errorf("store is closed")
	}

	s.records = append(s.records, record)
	return nil
}

// Close marks the store as closed.
func (s *InMemoryToolCallStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	return nil
}

// Records returns all stored records. For testing only.
func (s *InMemoryToolCallStore) Records() []ToolCallRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]ToolCallRecord{}, s.records...)
}

// Reset clears all records and reopens the store. For testing only.
func (s *InMemoryToolCallStore) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = make([]ToolCallRecord, 0)
	s.closed = false
}
