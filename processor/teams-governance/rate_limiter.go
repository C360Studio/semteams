package teamsgovernance

import (
	"context"
	"sync"
	"time"
)

// RateLimiter enforces request and token limits
type RateLimiter struct {
	// UserLimits maps user IDs to their buckets
	UserLimits sync.Map

	// SessionLimits maps session IDs to their buckets
	SessionLimits sync.Map

	// GlobalBucket for system-wide limits
	GlobalBucket *Bucket

	// Config holds rate limit configuration
	Config *RateLimitFilterConfig

	// Cleanup interval for expired buckets
	CleanupInterval time.Duration
}

// Bucket implements token bucket algorithm
type Bucket struct {
	// Capacity is maximum tokens
	Capacity int

	// RefillRate is tokens added per second
	RefillRate float64

	// Current token count
	Current int

	// LastRefill timestamp
	LastRefill time.Time

	// mu for concurrency safety
	mu sync.Mutex
}

// NewRateLimiter creates a new rate limiter from configuration
func NewRateLimiter(config *RateLimitFilterConfig) (*RateLimiter, error) {
	limiter := &RateLimiter{
		Config:          config,
		CleanupInterval: 5 * time.Minute,
	}

	// Initialize global bucket if configured
	if config.Global.RequestsPerMinute > 0 {
		limiter.GlobalBucket = NewBucket(
			config.Global.RequestsPerMinute,
			float64(config.Global.RequestsPerMinute)/60.0,
		)
	}

	return limiter, nil
}

// NewBucket creates a new token bucket
func NewBucket(capacity int, refillRate float64) *Bucket {
	return &Bucket{
		Capacity:   capacity,
		RefillRate: refillRate,
		Current:    capacity,
		LastRefill: time.Now(),
	}
}

// Name returns the filter name
func (r *RateLimiter) Name() string {
	return "rate_limiting"
}

// Process checks if request is within rate limits
func (r *RateLimiter) Process(_ context.Context, msg *Message) (*FilterResult, error) {
	// Calculate token cost (estimated from message size)
	tokenCost := r.estimateTokens(msg)

	// Check global limit first
	if r.GlobalBucket != nil {
		if !r.GlobalBucket.TryConsume(1) {
			return r.rateLimitExceeded(msg, "global", "requests_per_minute")
		}
	}

	// Check per-user limits
	if r.Config.PerUser.RequestsPerMinute > 0 && msg.UserID != "" {
		userBucket := r.getUserBucket(msg.UserID)
		if !userBucket.TryConsume(tokenCost) {
			return r.rateLimitExceeded(msg, msg.UserID, "per_user_tokens")
		}
	}

	// Check per-session limits
	if r.Config.PerSession.RequestsPerMinute > 0 && msg.SessionID != "" {
		sessionBucket := r.getSessionBucket(msg.SessionID)
		if !sessionBucket.TryConsume(tokenCost) {
			return r.rateLimitExceeded(msg, msg.SessionID, "per_session_tokens")
		}
	}

	// Build remaining capacity metadata
	metadata := make(map[string]any)
	metadata["tokens_consumed"] = tokenCost

	if msg.UserID != "" {
		if bucket, ok := r.UserLimits.Load(msg.UserID); ok {
			metadata["user_remaining"] = bucket.(*Bucket).Current
		}
	}

	if msg.SessionID != "" {
		if bucket, ok := r.SessionLimits.Load(msg.SessionID); ok {
			metadata["session_remaining"] = bucket.(*Bucket).Current
		}
	}

	result := NewFilterResult(true).WithConfidence(1.0)
	for k, v := range metadata {
		result.WithMetadata(k, v)
	}

	return result, nil
}

// getUserBucket returns or creates the bucket for a user
func (r *RateLimiter) getUserBucket(userID string) *Bucket {
	if bucket, ok := r.UserLimits.Load(userID); ok {
		return bucket.(*Bucket)
	}

	// Create new bucket with per-user limits
	// Tokens per hour converted to per-second refill rate
	tokensPerHour := r.Config.PerUser.TokensPerHour
	if tokensPerHour == 0 {
		tokensPerHour = 100000 // Default 100k tokens/hour
	}

	bucket := NewBucket(
		tokensPerHour/60, // Capacity: allow burst of 1 minute worth
		float64(tokensPerHour)/3600.0,
	)

	actual, _ := r.UserLimits.LoadOrStore(userID, bucket)
	return actual.(*Bucket)
}

// getSessionBucket returns or creates the bucket for a session
func (r *RateLimiter) getSessionBucket(sessionID string) *Bucket {
	if bucket, ok := r.SessionLimits.Load(sessionID); ok {
		return bucket.(*Bucket)
	}

	// Create new bucket with per-session limits
	tokensPerHour := r.Config.PerSession.TokensPerHour
	if tokensPerHour == 0 {
		tokensPerHour = 50000 // Default 50k tokens/hour per session
	}

	bucket := NewBucket(
		tokensPerHour/60,
		float64(tokensPerHour)/3600.0,
	)

	actual, _ := r.SessionLimits.LoadOrStore(sessionID, bucket)
	return actual.(*Bucket)
}

// TryConsume attempts to consume tokens from bucket
func (b *Bucket) TryConsume(tokens int) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Refill bucket based on elapsed time
	now := time.Now()
	elapsed := now.Sub(b.LastRefill).Seconds()
	tokensToAdd := int(elapsed * b.RefillRate)

	if tokensToAdd > 0 {
		b.Current = min(b.Current+tokensToAdd, b.Capacity)
		b.LastRefill = now
	}

	// Check if we have enough tokens
	if b.Current >= tokens {
		b.Current -= tokens
		return true
	}

	return false
}

// estimateTokens calculates approximate token count
func (r *RateLimiter) estimateTokens(msg *Message) int {
	// Rough estimate: ~4 characters per token
	textLen := len(msg.Content.Text)
	tokens := textLen / 4
	if tokens < 1 {
		tokens = 1
	}
	return tokens
}

// rateLimitExceeded creates rate limit violation
func (r *RateLimiter) rateLimitExceeded(msg *Message, identifier string, limitType string) (*FilterResult, error) {
	violation := NewViolation(r.Name(), SeverityMedium, msg).
		WithAction(ViolationActionBlocked).
		WithConfidence(1.0).
		WithDetail("identifier", identifier).
		WithDetail("limit_type", limitType)

	return NewFilterResult(false).
		WithViolation(violation).
		WithConfidence(1.0).
		WithMetadata("limit_type", limitType), nil
}

// GetUserRemaining returns remaining tokens for a user
func (r *RateLimiter) GetUserRemaining(userID string) int {
	if bucket, ok := r.UserLimits.Load(userID); ok {
		return bucket.(*Bucket).Current
	}
	return r.Config.PerUser.TokensPerHour / 60
}

// GetSessionRemaining returns remaining tokens for a session
func (r *RateLimiter) GetSessionRemaining(sessionID string) int {
	if bucket, ok := r.SessionLimits.Load(sessionID); ok {
		return bucket.(*Bucket).Current
	}
	return r.Config.PerSession.TokensPerHour / 60
}

// Reset resets all rate limit buckets (for testing)
func (r *RateLimiter) Reset() {
	r.UserLimits = sync.Map{}
	r.SessionLimits = sync.Map{}

	if r.GlobalBucket != nil {
		r.GlobalBucket.mu.Lock()
		r.GlobalBucket.Current = r.GlobalBucket.Capacity
		r.GlobalBucket.LastRefill = time.Now()
		r.GlobalBucket.mu.Unlock()
	}
}
