// Package graph - Tests for Triple Cleanup (TDD - RED Phase)
package graph

import (
	"context"
	"testing"
	"time"

	gtypes "github.com/c360/semstreams/graph"
	"github.com/c360/semstreams/message"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// T041: Test ExpiredTripleCleanup - triples with expired ExpiresAt are cleaned up
func TestExpiredTripleCleanup(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Now()

	tests := []struct {
		name           string
		triples        []message.Triple
		expectRemoved  int
		expectRetained int
	}{
		{
			name: "remove expired triples",
			triples: []message.Triple{
				{
					Subject:   "c360.platform1.robotics.mav1.drone.001",
					Predicate: "proximity.near",
					Object:    "c360.platform1.robotics.mav1.drone.002",
					ExpiresAt: timePtr(now.Add(-1 * time.Minute)), // Expired 1 minute ago
				},
				{
					Subject:   "c360.platform1.robotics.mav1.drone.001",
					Predicate: "proximity.near",
					Object:    "c360.platform1.robotics.mav1.drone.003",
					ExpiresAt: timePtr(now.Add(-5 * time.Minute)), // Expired 5 minutes ago
				},
			},
			expectRemoved:  2,
			expectRetained: 0,
		},
		{
			name: "retain non-expired triples",
			triples: []message.Triple{
				{
					Subject:   "c360.platform1.robotics.mav1.drone.001",
					Predicate: "proximity.near",
					Object:    "c360.platform1.robotics.mav1.drone.002",
					ExpiresAt: timePtr(now.Add(5 * time.Minute)), // Expires in 5 minutes
				},
				{
					Subject:   "c360.platform1.robotics.mav1.drone.001",
					Predicate: "proximity.near",
					Object:    "c360.platform1.robotics.mav1.drone.003",
					ExpiresAt: timePtr(now.Add(10 * time.Minute)), // Expires in 10 minutes
				},
			},
			expectRemoved:  0,
			expectRetained: 2,
		},
		{
			name: "retain triples without expiration",
			triples: []message.Triple{
				{
					Subject:   "c360.platform1.robotics.mav1.drone.001",
					Predicate: "fleet.member_of",
					Object:    "fleet.alpha",
					ExpiresAt: nil, // Never expires
				},
			},
			expectRemoved:  0,
			expectRetained: 1,
		},
		{
			name: "mixed expired and valid triples",
			triples: []message.Triple{
				{
					Subject:   "c360.platform1.robotics.mav1.drone.001",
					Predicate: "proximity.near",
					Object:    "c360.platform1.robotics.mav1.drone.002",
					ExpiresAt: timePtr(now.Add(-1 * time.Minute)), // Expired
				},
				{
					Subject:   "c360.platform1.robotics.mav1.drone.001",
					Predicate: "proximity.near",
					Object:    "c360.platform1.robotics.mav1.drone.003",
					ExpiresAt: timePtr(now.Add(5 * time.Minute)), // Valid
				},
				{
					Subject:   "c360.platform1.robotics.mav1.drone.001",
					Predicate: "fleet.member_of",
					Object:    "fleet.alpha",
					ExpiresAt: nil, // Never expires
				},
			},
			expectRemoved:  1,
			expectRetained: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create entity state with triples
			entity := &gtypes.EntityState{
				ID:      "c360.platform1.robotics.mav1.drone.001",
				Triples: tt.triples,
				Version: 1,
			}

			// Create cleanup worker (will fail - type doesn't exist yet)
			worker := &TripleCleanupWorker{}

			// Execute cleanup
			removed, err := worker.CleanupExpiredTriples(ctx, entity)
			require.NoError(t, err)

			assert.Equal(t, tt.expectRemoved, removed, "Should remove %d triples", tt.expectRemoved)

			// Verify remaining triples count
			remaining := len(entity.Triples)
			assert.Equal(t, tt.expectRetained, remaining, "Should retain %d triples", tt.expectRetained)

			// Verify no expired triples remain
			for _, triple := range entity.Triples {
				if triple.ExpiresAt != nil {
					assert.True(t, triple.ExpiresAt.After(now), "Remaining triple should not be expired")
				}
			}
		})
	}
}

// T041a: Test cleanup worker initialization
func TestTripleCleanupWorker_Init(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		interval time.Duration
		wantErr  bool
	}{
		{
			name:     "valid interval",
			interval: 1 * time.Minute,
			wantErr:  false,
		},
		{
			name:     "zero interval should fail",
			interval: 0,
			wantErr:  true,
		},
		{
			name:     "negative interval should fail",
			interval: -1 * time.Minute,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// NewTripleCleanupWorker doesn't exist yet
			worker, err := NewTripleCleanupWorker(tt.interval)

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, worker)
			}
		})
	}
}

// T041b: Test cleanup worker background execution
func TestTripleCleanupWorker_BackgroundExecution(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Now()

	// Create entity with expired triple (unused in this test - just verifying start/stop)
	_ = &gtypes.EntityState{
		ID: "c360.platform1.robotics.mav1.drone.001",
		Triples: []message.Triple{
			{
				Subject:   "c360.platform1.robotics.mav1.drone.001",
				Predicate: "proximity.near",
				Object:    "c360.platform1.robotics.mav1.drone.002",
				ExpiresAt: timePtr(now.Add(-1 * time.Minute)), // Expired
			},
		},
		Version: 1,
	}

	// Create cleanup worker with short interval
	worker, err := NewTripleCleanupWorker(100 * time.Millisecond)
	require.NoError(t, err)

	// Start background worker
	err = worker.Start(ctx)
	require.NoError(t, err)

	// Wait for cleanup to occur
	time.Sleep(200 * time.Millisecond)

	// Stop worker
	err = worker.Stop()
	require.NoError(t, err)

	// Note: This test verifies the worker can start/stop
	// Integration tests would verify actual cleanup of stored entities
}

// T041c: Test IsExpired helper on Triple
func TestTriple_IsExpired(t *testing.T) {
	t.Parallel()

	now := time.Now()

	tests := []struct {
		name    string
		triple  message.Triple
		expired bool
	}{
		{
			name: "expired triple",
			triple: message.Triple{
				Subject:   "c360.platform1.robotics.mav1.drone.001",
				Predicate: "proximity.near",
				Object:    "c360.platform1.robotics.mav1.drone.002",
				ExpiresAt: timePtr(now.Add(-1 * time.Minute)),
			},
			expired: true,
		},
		{
			name: "valid triple",
			triple: message.Triple{
				Subject:   "c360.platform1.robotics.mav1.drone.001",
				Predicate: "proximity.near",
				Object:    "c360.platform1.robotics.mav1.drone.002",
				ExpiresAt: timePtr(now.Add(5 * time.Minute)),
			},
			expired: false,
		},
		{
			name: "nil ExpiresAt (never expires)",
			triple: message.Triple{
				Subject:   "c360.platform1.robotics.mav1.drone.001",
				Predicate: "fleet.member_of",
				Object:    "fleet.alpha",
				ExpiresAt: nil,
			},
			expired: false,
		},
		{
			name: "exact current time (not expired)",
			triple: message.Triple{
				Subject:   "c360.platform1.robotics.mav1.drone.001",
				Predicate: "proximity.near",
				Object:    "c360.platform1.robotics.mav1.drone.002",
				ExpiresAt: timePtr(now.Add(1 * time.Second)), // Future to avoid timing race in CI
			},
			expired: false, // Equal to now is NOT expired (must be After)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// IsExpired is already implemented in message.Triple
			result := tt.triple.IsExpired()
			assert.Equal(t, tt.expired, result)
		})
	}
}

// T041d: Test cleanup metrics
func TestTripleCleanupWorker_Metrics(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Now()

	entity := &gtypes.EntityState{
		ID: "c360.platform1.robotics.mav1.drone.001",
		Triples: []message.Triple{
			{
				Subject:   "c360.platform1.robotics.mav1.drone.001",
				Predicate: "proximity.near",
				Object:    "c360.platform1.robotics.mav1.drone.002",
				ExpiresAt: timePtr(now.Add(-1 * time.Minute)),
			},
			{
				Subject:   "c360.platform1.robotics.mav1.drone.001",
				Predicate: "proximity.near",
				Object:    "c360.platform1.robotics.mav1.drone.003",
				ExpiresAt: timePtr(now.Add(-2 * time.Minute)),
			},
		},
		Version: 1,
	}

	worker := &TripleCleanupWorker{}

	// Execute cleanup
	removed, err := worker.CleanupExpiredTriples(ctx, entity)
	require.NoError(t, err)
	assert.Equal(t, 2, removed)

	// Verify metrics are updated (metrics don't exist yet)
	metrics := worker.GetMetrics()
	assert.NotNil(t, metrics)
	// Note: Actual metric verification would check:
	// - triple_cleanup_runs_total
	// - triple_cleanup_removed_total
	// - triple_cleanup_duration_seconds
}

// T041e: Test batch cleanup
func TestTripleCleanupWorker_BatchCleanup(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Now()

	entities := []*gtypes.EntityState{
		{
			ID: "c360.platform1.robotics.mav1.drone.001",
			Triples: []message.Triple{
				{
					Subject:   "c360.platform1.robotics.mav1.drone.001",
					Predicate: "proximity.near",
					Object:    "c360.platform1.robotics.mav1.drone.002",
					ExpiresAt: timePtr(now.Add(-1 * time.Minute)),
				},
			},
		},
		{
			ID: "c360.platform1.robotics.mav1.drone.003",
			Triples: []message.Triple{
				{
					Subject:   "c360.platform1.robotics.mav1.drone.003",
					Predicate: "proximity.near",
					Object:    "c360.platform1.robotics.mav1.drone.004",
					ExpiresAt: timePtr(now.Add(-2 * time.Minute)),
				},
			},
		},
	}

	worker := &TripleCleanupWorker{}

	// Execute batch cleanup
	totalRemoved, err := worker.CleanupBatch(ctx, entities)
	require.NoError(t, err)
	assert.Equal(t, 2, totalRemoved, "Should remove 2 triples across all entities")

	// Verify each entity has no expired triples
	for _, entity := range entities {
		for _, triple := range entity.Triples {
			assert.False(t, triple.IsExpired(), "No expired triples should remain")
		}
	}
}

// Helper function to create time pointer
func timePtr(t time.Time) *time.Time {
	return &t
}
