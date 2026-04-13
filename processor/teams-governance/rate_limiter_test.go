package teamsgovernance

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenBucket_TryConsume(t *testing.T) {
	bucket := NewBucket(100, 10.0) // 100 capacity, 10 tokens/sec

	// Should succeed - have enough tokens
	assert.True(t, bucket.TryConsume(50))
	assert.Equal(t, 50, bucket.Current)

	// Should succeed
	assert.True(t, bucket.TryConsume(30))
	assert.Equal(t, 20, bucket.Current)

	// Should fail - not enough tokens
	assert.False(t, bucket.TryConsume(50))
	assert.Equal(t, 20, bucket.Current) // Unchanged
}

func TestTokenBucket_Refill(t *testing.T) {
	bucket := NewBucket(100, 100.0) // 100 capacity, 100 tokens/sec
	bucket.Current = 50
	bucket.LastRefill = time.Now().Add(-1 * time.Second) // 1 second ago

	// After 1 second at 100 tokens/sec, should refill to 100 (capped at capacity)
	assert.True(t, bucket.TryConsume(80))
	assert.Equal(t, 20, bucket.Current)
}

func TestTokenBucket_CapAtCapacity(t *testing.T) {
	bucket := NewBucket(100, 1000.0) // Very high refill rate
	bucket.Current = 0
	bucket.LastRefill = time.Now().Add(-10 * time.Second)

	// Should not exceed capacity
	bucket.TryConsume(1)
	assert.LessOrEqual(t, bucket.Current, 100)
}

func TestRateLimiter_PerUserLimit(t *testing.T) {
	limiter, err := NewRateLimiter(&RateLimitFilterConfig{
		PerUser: RateLimitDef{
			RequestsPerMinute: 60,
			TokensPerHour:     6000, // Allows ~100 tokens/min burst
		},
		Algorithm: AlgoTokenBucket,
	})
	require.NoError(t, err)

	// First requests should succeed (10 requests × ~3 tokens = 30 tokens, well within 100 capacity)
	for i := 0; i < 10; i++ {
		msg := &Message{
			ID:      "test",
			UserID:  "user1",
			Content: Content{Text: "short message"}, // ~3 tokens
		}
		result, err := limiter.Process(context.Background(), msg)
		require.NoError(t, err)
		assert.True(t, result.Allowed, "Request %d should be allowed", i)
	}
}

func TestRateLimiter_PerSessionLimit(t *testing.T) {
	limiter, err := NewRateLimiter(&RateLimitFilterConfig{
		PerUser: RateLimitDef{
			RequestsPerMinute: 60,
			TokensPerHour:     100000,
		},
		PerSession: RateLimitDef{
			RequestsPerMinute: 30,
			TokensPerHour:     500, // Low limit
		},
		Algorithm: AlgoTokenBucket,
	})
	require.NoError(t, err)

	// First requests should succeed
	msg := &Message{
		ID:        "test",
		UserID:    "user1",
		SessionID: "session1",
		Content:   Content{Text: "short message"},
	}

	result, err := limiter.Process(context.Background(), msg)
	require.NoError(t, err)
	assert.True(t, result.Allowed)
}

func TestRateLimiter_GlobalLimit(t *testing.T) {
	limiter, err := NewRateLimiter(&RateLimitFilterConfig{
		Global: RateLimitDef{
			RequestsPerMinute: 5, // Very low limit
		},
		Algorithm: AlgoTokenBucket,
	})
	require.NoError(t, err)

	// First few requests should succeed
	for i := 0; i < 5; i++ {
		msg := &Message{
			ID:      "test",
			UserID:  "user1",
			Content: Content{Text: "hi"},
		}
		result, err := limiter.Process(context.Background(), msg)
		require.NoError(t, err)
		assert.True(t, result.Allowed, "Request %d should be allowed", i)
	}

	// 6th request should be blocked
	msg := &Message{
		ID:      "test",
		UserID:  "user1",
		Content: Content{Text: "hi"},
	}
	result, err := limiter.Process(context.Background(), msg)
	require.NoError(t, err)
	assert.False(t, result.Allowed, "Should be rate limited")
	assert.NotNil(t, result.Violation)
	assert.Equal(t, "global", result.Violation.Details["identifier"])
}

func TestRateLimiter_EstimateTokens(t *testing.T) {
	limiter, err := NewRateLimiter(&RateLimitFilterConfig{
		PerUser: RateLimitDef{
			RequestsPerMinute: 60,
			TokensPerHour:     100000,
		},
	})
	require.NoError(t, err)

	testCases := []struct {
		text     string
		expected int
	}{
		{"hi", 1},                       // 2 chars / 4 = 0.5, minimum 1
		{"hello world", 2},              // 11 chars / 4 = 2.75 = 2
		{"This is a longer message", 6}, // 24 chars / 4 = 6
	}

	for _, tc := range testCases {
		msg := &Message{Content: Content{Text: tc.text}}
		tokens := limiter.estimateTokens(msg)
		assert.Equal(t, tc.expected, tokens, "Text: %s", tc.text)
	}
}

func TestRateLimiter_DifferentUsers(t *testing.T) {
	limiter, err := NewRateLimiter(&RateLimitFilterConfig{
		PerUser: RateLimitDef{
			RequestsPerMinute: 60,
			TokensPerHour:     3600, // Allows 60 tokens/min burst, 5 requests × 1 token = 5 tokens
		},
		Algorithm: AlgoTokenBucket,
	})
	require.NoError(t, err)

	// User 1 requests
	for i := 0; i < 5; i++ {
		msg := &Message{
			ID:      "test",
			UserID:  "user1",
			Content: Content{Text: "short"}, // 5 chars / 4 = 1 token
		}
		result, err := limiter.Process(context.Background(), msg)
		require.NoError(t, err)
		assert.True(t, result.Allowed)
	}

	// User 2 should have separate bucket
	msg := &Message{
		ID:      "test",
		UserID:  "user2",
		Content: Content{Text: "short"},
	}
	result, err := limiter.Process(context.Background(), msg)
	require.NoError(t, err)
	assert.True(t, result.Allowed, "User 2 should have separate limit")
}

func TestRateLimiter_Reset(t *testing.T) {
	limiter, err := NewRateLimiter(&RateLimitFilterConfig{
		Global: RateLimitDef{
			RequestsPerMinute: 2,
		},
		Algorithm: AlgoTokenBucket,
	})
	require.NoError(t, err)

	// Exhaust the limit
	for i := 0; i < 2; i++ {
		msg := &Message{ID: "test", Content: Content{Text: "hi"}}
		limiter.Process(context.Background(), msg)
	}

	// Should be limited
	msg := &Message{ID: "test", Content: Content{Text: "hi"}}
	result, _ := limiter.Process(context.Background(), msg)
	assert.False(t, result.Allowed)

	// Reset
	limiter.Reset()

	// Should work again
	result, err = limiter.Process(context.Background(), msg)
	require.NoError(t, err)
	assert.True(t, result.Allowed)
}

func TestRateLimiter_GetRemaining(t *testing.T) {
	limiter, err := NewRateLimiter(&RateLimitFilterConfig{
		PerUser: RateLimitDef{
			RequestsPerMinute: 60,
			TokensPerHour:     6000, // 100 tokens/minute capacity
		},
		Algorithm: AlgoTokenBucket,
	})
	require.NoError(t, err)

	// Initial remaining should be capacity
	remaining := limiter.GetUserRemaining("user1")
	assert.Equal(t, 100, remaining) // 6000/60 = 100

	// After consuming some tokens
	msg := &Message{
		ID:      "test",
		UserID:  "user1",
		Content: Content{Text: "hello"}, // ~1-2 tokens
	}
	limiter.Process(context.Background(), msg)

	remaining = limiter.GetUserRemaining("user1")
	assert.Less(t, remaining, 100)
}

func TestRateLimiter_MetadataReturned(t *testing.T) {
	limiter, err := NewRateLimiter(&RateLimitFilterConfig{
		PerUser: RateLimitDef{
			RequestsPerMinute: 60,
			TokensPerHour:     100000,
		},
		Algorithm: AlgoTokenBucket,
	})
	require.NoError(t, err)

	msg := &Message{
		ID:      "test",
		UserID:  "user1",
		Content: Content{Text: "hello world"},
	}

	result, err := limiter.Process(context.Background(), msg)
	require.NoError(t, err)
	assert.True(t, result.Allowed)

	// Check metadata
	_, ok := result.Metadata["tokens_consumed"]
	assert.True(t, ok, "Should include tokens_consumed")

	_, ok = result.Metadata["user_remaining"]
	assert.True(t, ok, "Should include user_remaining")
}
