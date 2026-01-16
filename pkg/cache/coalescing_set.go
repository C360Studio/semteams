package cache

import (
	"context"
	"sync"
	"time"
)

// CoalescingSet collects keys over a time window and fires a callback with the batch.
// It deduplicates keys automatically using a map-based set structure.
// Follows the ticker-based background goroutine pattern from ttl.go.
type CoalescingSet struct {
	pending   map[string]struct{}
	mu        sync.Mutex
	window    time.Duration
	callback  func(keys []string)
	ticker    *time.Ticker
	shutdown  chan struct{}
	done      chan struct{}
	closeOnce sync.Once
}

// NewCoalescingSet creates a new CoalescingSet that fires the callback every window duration
// with the collected (deduplicated) keys. The background goroutine stops when ctx is cancelled
// or when Close() is called.
func NewCoalescingSet(ctx context.Context, window time.Duration, callback func([]string)) *CoalescingSet {
	c := &CoalescingSet{
		pending:  make(map[string]struct{}),
		window:   window,
		callback: callback,
		shutdown: make(chan struct{}),
		done:     make(chan struct{}),
	}

	// Handle zero or negative window by using minimum ticker duration
	tickerDuration := window
	if tickerDuration <= 0 {
		tickerDuration = 1 * time.Nanosecond
	}

	c.ticker = time.NewTicker(tickerDuration)

	// Start background goroutine
	go c.run(ctx)

	return c
}

// Add adds a key to the pending set. If the key already exists, it is deduplicated.
// Thread-safe.
func (c *CoalescingSet) Add(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pending[key] = struct{}{}
}

// Remove removes a key from the pending set if it exists.
// This is useful when an entity is deleted before the window expires.
// Thread-safe.
func (c *CoalescingSet) Remove(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.pending, key)
}

// PendingCount returns the number of keys currently pending in the set.
// Thread-safe.
func (c *CoalescingSet) PendingCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.pending)
}

// Close stops the background ticker and waits for cleanup to complete.
// It is idempotent - multiple calls are safe.
func (c *CoalescingSet) Close() error {
	// Use sync.Once to make Close() safe for concurrent calls
	c.closeOnce.Do(func() {
		close(c.shutdown)
	})

	// Wait for background goroutine to finish
	<-c.done

	return nil
}

// run is the background goroutine that fires the callback on each tick.
// Follows the pattern from ttl.go cleanup method.
func (c *CoalescingSet) run(ctx context.Context) {
	defer close(c.done)
	defer c.ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-c.shutdown:
			return
		case <-c.ticker.C:
			c.fireBatch()
		}
	}
}

// fireBatch collects the current pending keys, clears the set, and calls the callback.
// The callback is invoked OUTSIDE the lock to prevent deadlocks.
func (c *CoalescingSet) fireBatch() {
	// Lock to read and clear pending keys
	c.mu.Lock()
	if len(c.pending) == 0 {
		c.mu.Unlock()
		// Skip callback if no keys pending
		return
	}

	// Copy keys to slice
	keys := make([]string, 0, len(c.pending))
	for k := range c.pending {
		keys = append(keys, k)
	}

	// Clear the pending set for next window
	c.pending = make(map[string]struct{})
	c.mu.Unlock()

	// Call callback OUTSIDE the lock to prevent deadlock
	// Wrap in defer/recover to handle panics gracefully
	if c.callback != nil {
		func() {
			defer func() {
				if r := recover(); r != nil {
					// Callback panicked - recover gracefully and continue
					// In production, this could log the panic
					_ = r
				}
			}()
			c.callback(keys)
		}()
	}
}
