package graph

import (
	"context"
	"log/slog"
	"time"

	"github.com/c360/semstreams/pkg/errs"
	"github.com/c360/semstreams/processor/graph/clustering"
	"github.com/c360/semstreams/processor/graph/structuralindex"
)

// structuralIndexComputer computes k-core and pivot indices using a GraphProvider.
// It wraps the clustering.GraphProvider and adapts it for the structuralindex package.
type structuralIndexComputer struct {
	graphProvider clustering.GraphProvider
	pivotCount    int
	logger        *slog.Logger
}

// newStructuralIndexComputer creates a new structural index computer.
func newStructuralIndexComputer(
	provider clustering.GraphProvider,
	config StructuralIndexConfig,
	logger *slog.Logger,
) *structuralIndexComputer {
	pivotCount := config.Pivot.PivotCount
	if pivotCount <= 0 {
		pivotCount = structuralindex.DefaultPivotCount
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &structuralIndexComputer{
		graphProvider: provider,
		pivotCount:    pivotCount,
		logger:        logger,
	}
}

// Compute computes both k-core and pivot indices.
// Returns nil indices (not error) if provider is nil.
func (c *structuralIndexComputer) Compute(ctx context.Context) (*structuralindex.StructuralIndices, error) {
	if c.graphProvider == nil {
		return nil, errs.WrapInvalid(errs.ErrMissingConfig, "structuralIndexComputer", "Compute",
			"graph provider is nil")
	}

	startTime := time.Now()
	result := &structuralindex.StructuralIndices{}

	// Create adapter: clustering.GraphProvider -> structuralindex.GraphProvider
	adapter := &graphProviderAdapter{provider: c.graphProvider}

	// Compute k-core
	c.logger.Debug("computing k-core decomposition")
	kcoreComputer := structuralindex.NewKCoreComputer(adapter, c.logger)
	kcore, err := kcoreComputer.Compute(ctx)
	if err != nil {
		return nil, errs.WrapTransient(err, "structuralIndexComputer", "Compute",
			"k-core computation failed")
	}
	result.KCore = kcore
	c.logger.Debug("k-core computation complete",
		slog.Int("entity_count", kcore.EntityCount),
		slog.Int("max_core", kcore.MaxCore))

	// Compute pivot index
	c.logger.Debug("computing pivot index", slog.Int("pivot_count", c.pivotCount))
	pivotComputer := structuralindex.NewPivotComputer(adapter, c.pivotCount, c.logger)
	pivot, err := pivotComputer.Compute(ctx)
	if err != nil {
		return nil, errs.WrapTransient(err, "structuralIndexComputer", "Compute",
			"pivot computation failed")
	}
	result.Pivot = pivot
	c.logger.Debug("pivot computation complete",
		slog.Int("entity_count", pivot.EntityCount),
		slog.Int("pivot_count", len(pivot.Pivots)))

	c.logger.Info("structural indices computed",
		slog.Duration("elapsed", time.Since(startTime)),
		slog.Int("entities", kcore.EntityCount),
		slog.Int("max_core", kcore.MaxCore),
		slog.Int("pivots", len(pivot.Pivots)))

	return result, nil
}

// graphProviderAdapter adapts clustering.GraphProvider to structuralindex.GraphProvider.
// Both interfaces have the same methods, but we need an explicit adapter to satisfy
// the structuralindex.GraphProvider type without creating an import cycle.
type graphProviderAdapter struct {
	provider clustering.GraphProvider
}

// GetAllEntityIDs returns all entity IDs in the graph.
func (a *graphProviderAdapter) GetAllEntityIDs(ctx context.Context) ([]string, error) {
	return a.provider.GetAllEntityIDs(ctx)
}

// GetNeighbors returns the entity IDs connected to the given entity.
func (a *graphProviderAdapter) GetNeighbors(ctx context.Context, entityID, direction string) ([]string, error) {
	return a.provider.GetNeighbors(ctx, entityID, direction)
}

// GetEdgeWeight returns the weight of the edge between two entities.
func (a *graphProviderAdapter) GetEdgeWeight(ctx context.Context, fromID, toID string) (float64, error) {
	return a.provider.GetEdgeWeight(ctx, fromID, toID)
}
