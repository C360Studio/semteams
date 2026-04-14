package teamsmodel

import (
	"context"
	"sync"
	"time"
)

// EndpointThrottle controls request rate to a single API endpoint.
//
// It combines two mechanisms:
//   - A token bucket that limits requests per minute (if RequestsPerMinute > 0).
//   - A semaphore that caps concurrent in-flight requests (if MaxConcurrent > 0).
//
// Both limits are applied on every Acquire call. Either limit may be disabled
// independently by setting its value to 0.
//
// Throttle instances are shared across all clients that target the same endpoint
// URL + model pair, so the limits are enforced across the whole agent team.
type EndpointThrottle struct {
	mu         sync.Mutex
	tokens     float64
	maxTokens  float64
	refillRate float64 // tokens per nanosecond
	lastRefill time.Time

	// semaphore is nil when MaxConcurrent == 0 (unlimited concurrency).
	semaphore chan struct{}
}

// NewEndpointThrottle creates a throttle for the given limits.
// requestsPerMinute == 0 disables rate limiting.
// maxConcurrent == 0 disables concurrency limiting.
func NewEndpointThrottle(requestsPerMinute, maxConcurrent int) *EndpointThrottle {
	t := &EndpointThrottle{
		lastRefill: time.Now(),
	}

	if requestsPerMinute > 0 {
		t.maxTokens = float64(requestsPerMinute)
		t.tokens = float64(requestsPerMinute)
		// Convert requests/minute → tokens/nanosecond for time.Duration arithmetic.
		t.refillRate = float64(requestsPerMinute) / float64(time.Minute)
	}

	if maxConcurrent > 0 {
		t.semaphore = make(chan struct{}, maxConcurrent)
		for range maxConcurrent {
			t.semaphore <- struct{}{}
		}
	}

	return t
}

// Acquire blocks until a request slot is available or the context is cancelled.
// The caller must call Release when the request completes.
func (t *EndpointThrottle) Acquire(ctx context.Context) error {
	// Rate-limit: wait until a token is available.
	if t.refillRate > 0 {
		for {
			t.mu.Lock()
			t.refill()
			if t.tokens >= 1.0 {
				t.tokens--
				t.mu.Unlock()
				break
			}
			// Calculate how long until one token refills.
			need := (1.0 - t.tokens) / t.refillRate
			waitNs := time.Duration(need)
			t.mu.Unlock()

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(waitNs):
				// Loop and recheck — another goroutine may have consumed the token.
			}
		}
	}

	// Concurrency limit: acquire semaphore slot.
	if t.semaphore != nil {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.semaphore:
			// Slot acquired.
		}
	}

	return nil
}

// Release returns a concurrency slot to the semaphore.
// It is a no-op when MaxConcurrent == 0.
func (t *EndpointThrottle) Release() {
	if t.semaphore != nil {
		t.semaphore <- struct{}{}
	}
}

// refill adds tokens based on elapsed time since the last refill.
// Must be called with t.mu held.
func (t *EndpointThrottle) refill() {
	now := time.Now()
	elapsed := now.Sub(t.lastRefill)
	if elapsed <= 0 {
		return
	}
	t.tokens += float64(elapsed) * t.refillRate
	if t.tokens > t.maxTokens {
		t.tokens = t.maxTokens
	}
	t.lastRefill = now
}
