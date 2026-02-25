package reactive

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

// NOTE: This implementation follows the reactive workflow engine design from ADR-021.
// Critical: Use checkAndClearOwnRevision for atomic skip detection to prevent race conditions.

// KVWatcher manages KV watch operations for state-triggered rules.
// It watches one or more KV buckets for state changes and triggers
// rule evaluation when matching keys are updated.
type KVWatcher struct {
	logger   *slog.Logger
	watchers map[string]jetstream.KeyWatcher // bucket:pattern -> watcher
	mu       sync.RWMutex

	// ownRevisions tracks revisions we wrote to prevent feedback loops.
	// Key is the KV key, value is the revision we wrote.
	ownRevisions map[string]uint64
	revisionMu   sync.Mutex // Use Mutex instead of RWMutex for atomic check-and-clear

	// shutdown signals the watcher goroutines to stop
	shutdown     chan struct{}
	shutdownOnce sync.Once
}

// NewKVWatcher creates a new KV watcher.
func NewKVWatcher(logger *slog.Logger) *KVWatcher {
	return &KVWatcher{
		logger:       logger,
		watchers:     make(map[string]jetstream.KeyWatcher),
		ownRevisions: make(map[string]uint64),
		shutdown:     make(chan struct{}),
	}
}

// WatchHandler is called when a KV entry is updated.
// The handler should return quickly; long-running operations should be done asynchronously.
type WatchHandler func(ctx context.Context, event KVWatchEvent)

// KVWatchEvent represents a KV state change event.
type KVWatchEvent struct {
	// Bucket is the KV bucket name.
	Bucket string

	// Key is the KV entry key.
	Key string

	// Value is the raw KV entry value (JSON).
	Value []byte

	// Revision is the KV entry revision.
	Revision uint64

	// Operation indicates the type of change.
	Operation KVOperation

	// Timestamp is when the event was received.
	Timestamp time.Time
}

// KVOperation indicates the type of KV state change.
type KVOperation int

const (
	// KVOperationPut indicates a create or update.
	KVOperationPut KVOperation = iota
	// KVOperationDelete indicates a deletion.
	KVOperationDelete
)

// String returns a human-readable name for the operation.
func (op KVOperation) String() string {
	switch op {
	case KVOperationPut:
		return "put"
	case KVOperationDelete:
		return "delete"
	default:
		return "unknown"
	}
}

// StartWatch starts watching a KV bucket for changes matching the pattern.
// The handler is called for each matching update.
func (w *KVWatcher) StartWatch(
	ctx context.Context,
	bucket jetstream.KeyValue,
	pattern string,
	handler WatchHandler,
) error {
	bucketName := bucket.Bucket()
	key := watcherKey(bucketName, pattern)

	w.mu.Lock()
	defer w.mu.Unlock()

	// Check if already watching this bucket+pattern
	if _, exists := w.watchers[key]; exists {
		w.logger.Debug("Watcher already exists", "bucket", bucketName, "pattern", pattern)
		return nil
	}

	// Create the watcher
	watcher, err := bucket.Watch(ctx, pattern)
	if err != nil {
		return &WatchError{Bucket: bucketName, Pattern: pattern, Cause: err}
	}

	w.watchers[key] = watcher

	// Start the goroutine to handle updates
	go w.handleUpdates(ctx, bucketName, watcher, handler)

	w.logger.Info("Started KV watcher", "bucket", bucketName, "pattern", pattern)
	return nil
}

// StopWatch stops watching a specific bucket+pattern.
func (w *KVWatcher) StopWatch(bucketName, pattern string) error {
	key := watcherKey(bucketName, pattern)

	w.mu.Lock()
	defer w.mu.Unlock()

	watcher, exists := w.watchers[key]
	if !exists {
		return nil // Not watching, nothing to do
	}

	if err := watcher.Stop(); err != nil {
		w.logger.Warn("Error stopping watcher",
			"bucket", bucketName,
			"pattern", pattern,
			"error", err)
	}

	delete(w.watchers, key)
	w.logger.Info("Stopped KV watcher", "bucket", bucketName, "pattern", pattern)
	return nil
}

// StopAll stops all active watchers.
// Safe to call multiple times.
func (w *KVWatcher) StopAll() {
	w.shutdownOnce.Do(func() {
		close(w.shutdown)
	})

	w.mu.Lock()
	defer w.mu.Unlock()

	for key, watcher := range w.watchers {
		if err := watcher.Stop(); err != nil {
			w.logger.Warn("Error stopping watcher", "key", key, "error", err)
		}
	}

	w.watchers = make(map[string]jetstream.KeyWatcher)
	w.logger.Info("Stopped all KV watchers")
}

// RecordOwnRevision records a revision that we wrote, to prevent feedback loops.
// Call this after successfully writing to KV so we skip evaluation when we see our own update.
func (w *KVWatcher) RecordOwnRevision(key string, revision uint64) {
	w.revisionMu.Lock()
	defer w.revisionMu.Unlock()
	w.ownRevisions[key] = revision
}

// checkAndClearOwnRevision atomically checks if this is our own revision and clears it.
// Returns true if this revision should be skipped (was our own write).
// This is atomic to prevent race conditions where two goroutines could both
// check and clear the same revision.
func (w *KVWatcher) checkAndClearOwnRevision(key string, revision uint64) bool {
	w.revisionMu.Lock()
	defer w.revisionMu.Unlock()

	ownRev, exists := w.ownRevisions[key]
	if exists && ownRev == revision {
		delete(w.ownRevisions, key)
		return true
	}
	return false
}

// handleUpdates processes updates from a KV watcher.
func (w *KVWatcher) handleUpdates(
	ctx context.Context,
	bucketName string,
	watcher jetstream.KeyWatcher,
	handler WatchHandler,
) {
	defer func() {
		if r := recover(); r != nil {
			w.logger.Error("Panic in handleUpdates", "bucket", bucketName, "error", r)
		}
	}()

	for {
		select {
		case <-ctx.Done():
			if err := watcher.Stop(); err != nil {
				w.logger.Warn("Error stopping watcher on context cancellation",
					"bucket", bucketName, "error", err)
			}
			return
		case <-w.shutdown:
			if err := watcher.Stop(); err != nil {
				w.logger.Warn("Error stopping watcher on shutdown",
					"bucket", bucketName, "error", err)
			}
			return
		case entry, ok := <-watcher.Updates():
			if !ok {
				// Channel closed, watcher stopped
				return
			}
			if entry == nil {
				// Nil entry indicates initial state complete
				continue
			}

			key := entry.Key()
			revision := entry.Revision()

			// Atomically check if we should skip (our own revision) and clear
			if w.checkAndClearOwnRevision(key, revision) {
				w.logger.Debug("Skipping self-generated update",
					"bucket", bucketName,
					"key", key,
					"revision", revision)
				continue
			}

			// Determine operation
			op := KVOperationPut
			if entry.Operation() == jetstream.KeyValueDelete {
				op = KVOperationDelete
			}

			// Build and dispatch event
			event := KVWatchEvent{
				Bucket:    bucketName,
				Key:       key,
				Value:     entry.Value(),
				Revision:  revision,
				Operation: op,
				Timestamp: time.Now(),
			}

			handler(ctx, event)
		}
	}
}

// WatchError represents an error starting a KV watch.
type WatchError struct {
	Bucket  string
	Pattern string
	Cause   error
}

// Error implements the error interface.
func (e *WatchError) Error() string {
	return "failed to watch " + e.Bucket + ":" + e.Pattern + ": " + e.Cause.Error()
}

// Unwrap returns the underlying error.
func (e *WatchError) Unwrap() error {
	return e.Cause
}

// watcherKey creates a unique key for bucket+pattern combination.
func watcherKey(bucket, pattern string) string {
	return bucket + ":" + pattern
}

// MatchesPattern checks if a key matches a NATS-style wildcard pattern.
// Supports * (single token) and > (multi-token suffix) wildcards.
func MatchesPattern(key, pattern string) bool {
	return matchPattern(key, pattern, 0, 0)
}

// matchPattern is the recursive implementation of pattern matching.
func matchPattern(key, pattern string, ki, pi int) bool {
	for pi < len(pattern) && ki < len(key) {
		if pattern[pi] == '*' {
			// * matches a single token (up to next .)
			// Skip to the next . or end in the key
			for ki < len(key) && key[ki] != '.' {
				ki++
			}
			pi++
			// If pattern has more after *, expect a . in pattern
			if pi < len(pattern) && pattern[pi] == '.' {
				pi++
				if ki < len(key) && key[ki] == '.' {
					ki++
				} else {
					return false
				}
			}
		} else if pattern[pi] == '>' {
			// > matches everything remaining
			return true
		} else if pattern[pi] == key[ki] {
			pi++
			ki++
		} else {
			return false
		}
	}

	// Check if we've consumed both strings (or pattern ends with >)
	if pi < len(pattern) && pattern[pi] == '>' {
		return true
	}
	return pi == len(pattern) && ki == len(key)
}

// BuildRuleContextFromKV builds a RuleContext from a KV watch event.
// The stateFactory is used to create a typed instance for unmarshaling.
func BuildRuleContextFromKV(event KVWatchEvent, stateFactory func() any) (*RuleContext, error) {
	if event.Operation == KVOperationDelete {
		// For deletes, we don't have state to unmarshal
		return &RuleContext{
			State:      nil,
			KVRevision: event.Revision,
			KVKey:      event.Key,
		}, nil
	}

	// Create typed state instance and unmarshal
	state := stateFactory()
	if err := json.Unmarshal(event.Value, state); err != nil {
		return nil, &UnmarshalError{Key: event.Key, Cause: err}
	}

	return &RuleContext{
		State:      state,
		KVRevision: event.Revision,
		KVKey:      event.Key,
	}, nil
}

// UnmarshalError represents an error unmarshaling KV state.
type UnmarshalError struct {
	Key   string
	Cause error
}

// Error implements the error interface.
func (e *UnmarshalError) Error() string {
	return "failed to unmarshal state for " + e.Key + ": " + e.Cause.Error()
}

// Unwrap returns the underlying error.
func (e *UnmarshalError) Unwrap() error {
	return e.Cause
}
