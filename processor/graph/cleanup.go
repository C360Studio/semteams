// Package graph - Triple cleanup worker implementation
package graph

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	gtypes "github.com/c360/semstreams/types/graph"
)

var (
	// ErrInvalidInterval is returned when cleanup interval is <= 0
	ErrInvalidInterval = errors.New("cleanup interval must be positive")
)

// TripleCleanupWorker periodically removes expired triples from entity states.
type TripleCleanupWorker struct {
	interval time.Duration
	logger   *slog.Logger

	// Control
	stopCh  chan struct{}
	wg      sync.WaitGroup
	mu      sync.Mutex
	running bool

	// Metrics (stub for now)
	totalRuns    int64
	totalRemoved int64
}

// CleanupMetrics holds cleanup worker metrics.
type CleanupMetrics struct {
	TotalRuns    int64
	TotalRemoved int64
}

// NewTripleCleanupWorker creates a new cleanup worker.
// Returns error if interval <= 0.
func NewTripleCleanupWorker(interval time.Duration) (*TripleCleanupWorker, error) {
	if interval <= 0 {
		return nil, ErrInvalidInterval
	}

	return &TripleCleanupWorker{
		interval: interval,
		logger:   slog.Default(),
		stopCh:   make(chan struct{}),
	}, nil
}

// Start begins the cleanup worker loop.
func (w *TripleCleanupWorker) Start(ctx context.Context) error {
	w.mu.Lock()
	if w.running {
		w.mu.Unlock()
		return nil // Already running
	}
	w.running = true
	w.mu.Unlock()

	w.wg.Add(1)
	go w.run(ctx)

	return nil
}

// Stop gracefully stops the cleanup worker.
func (w *TripleCleanupWorker) Stop() error {
	w.mu.Lock()
	if !w.running {
		w.mu.Unlock()
		return nil
	}
	w.running = false
	w.mu.Unlock()

	close(w.stopCh)
	w.wg.Wait()

	return nil
}

// IsRunning returns whether the worker is currently running.
func (w *TripleCleanupWorker) IsRunning() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.running
}

func (w *TripleCleanupWorker) run(ctx context.Context) {
	defer w.wg.Done()

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("Cleanup worker stopping due to context cancellation")
			return
		case <-w.stopCh:
			w.logger.Info("Cleanup worker stopping")
			return
		case <-ticker.C:
			w.logger.Debug("Running triple cleanup cycle")
			// TODO: Implement actual cleanup logic
			// 1. Scan ENTITY_STATES bucket for entities with expired triples
			// 2. For each entity, call CleanupExpiredTriples
			// 3. Update entity state with filtered triples
		}
	}
}

// CleanupExpiredTriples removes expired triples from a single entity.
// Returns the number of triples removed.
// Mutates the entity.Triples slice in place.
func (w *TripleCleanupWorker) CleanupExpiredTriples(ctx context.Context, entity *gtypes.EntityState) (int, error) {
	_ = ctx // Reserved for future cancellation support

	if entity == nil {
		return 0, nil
	}

	originalCount := len(entity.Triples)
	filtered := entity.Triples[:0] // Reuse backing array

	// Filter out expired triples
	for _, triple := range entity.Triples {
		if !triple.IsExpired() {
			filtered = append(filtered, triple)
		}
	}

	// Update entity triples with filtered slice
	entity.Triples = filtered

	removed := originalCount - len(entity.Triples)

	// Update metrics
	w.mu.Lock()
	w.totalRemoved += int64(removed)
	w.mu.Unlock()

	return removed, nil
}

// CleanupBatch removes expired triples from multiple entities.
// Returns the total number of triples removed across all entities.
func (w *TripleCleanupWorker) CleanupBatch(ctx context.Context, entities []*gtypes.EntityState) (int, error) {
	totalRemoved := 0

	for _, entity := range entities {
		removed, err := w.CleanupExpiredTriples(ctx, entity)
		if err != nil {
			return totalRemoved, err
		}
		totalRemoved += removed
	}

	// Update metrics
	w.mu.Lock()
	w.totalRuns++
	w.mu.Unlock()

	return totalRemoved, nil
}

// GetMetrics returns current cleanup metrics.
func (w *TripleCleanupWorker) GetMetrics() *CleanupMetrics {
	w.mu.Lock()
	defer w.mu.Unlock()

	return &CleanupMetrics{
		TotalRuns:    w.totalRuns,
		TotalRemoved: w.totalRemoved,
	}
}
