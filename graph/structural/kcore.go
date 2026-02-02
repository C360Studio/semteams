package structural

import (
	"context"
	"log/slog"
	"sort"
	"time"

	gtypes "github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/pkg/errs"
)

// Provider is an alias to the shared interface in graph package.
// Abstracts the graph data source for structural index computation.
type Provider = gtypes.Provider

// KCoreComputer computes k-core decomposition of a graph.
//
// K-core decomposition uses the "peeling" algorithm:
//  1. Initialize degree for all vertices
//  2. Repeatedly remove the vertex with minimum degree
//  3. The core number of a vertex is its degree when removed
//
// Time complexity: O(V + E)
// Space complexity: O(V)
type KCoreComputer struct {
	provider Provider
	logger   *slog.Logger
}

// NewKCoreComputer creates a new k-core computer.
func NewKCoreComputer(provider Provider, logger *slog.Logger) *KCoreComputer {
	if logger == nil {
		logger = slog.Default()
	}
	return &KCoreComputer{
		provider: provider,
		logger:   logger,
	}
}

// Compute performs k-core decomposition on the graph.
// Returns a KCoreIndex with core numbers for all entities.
func (c *KCoreComputer) Compute(ctx context.Context) (*KCoreIndex, error) {
	startTime := time.Now()

	// Get all entity IDs
	entityIDs, err := c.provider.GetAllEntityIDs(ctx)
	if err != nil {
		return nil, errs.WrapTransient(err, "KCoreComputer", "Compute", "get entity IDs")
	}

	if len(entityIDs) == 0 {
		return &KCoreIndex{
			CoreNumbers: make(map[string]int),
			MaxCore:     0,
			CoreBuckets: make(map[int][]string),
			ComputedAt:  time.Now(),
			EntityCount: 0,
		}, nil
	}

	// Build adjacency and compute initial degrees
	// Using "both" direction since k-core treats edges as undirected
	degree := make(map[string]int, len(entityIDs))
	neighbors := make(map[string]map[string]bool, len(entityIDs))

	for _, entityID := range entityIDs {
		select {
		case <-ctx.Done():
			return nil, errs.WrapTransient(ctx.Err(), "KCoreComputer", "Compute", "context cancelled during degree computation")
		default:
		}

		neighborList, err := c.provider.GetNeighbors(ctx, entityID, "both")
		if err != nil {
			// Log warning but continue - entity may have no relationships
			c.logger.Warn("failed to get neighbors",
				slog.String("entity_id", entityID),
				slog.String("error", err.Error()))
			neighborList = []string{}
		}

		// Store neighbors as set for efficient lookup
		neighborSet := make(map[string]bool, len(neighborList))
		for _, n := range neighborList {
			neighborSet[n] = true
		}
		neighbors[entityID] = neighborSet
		degree[entityID] = len(neighborList)
	}

	// Peeling algorithm
	// Sort entities by degree to process in order
	sortedEntities := make([]string, len(entityIDs))
	copy(sortedEntities, entityIDs)
	sort.Slice(sortedEntities, func(i, j int) bool {
		return degree[sortedEntities[i]] < degree[sortedEntities[j]]
	})

	coreNumbers := make(map[string]int, len(entityIDs))
	removed := make(map[string]bool, len(entityIDs))

	for _, v := range sortedEntities {
		select {
		case <-ctx.Done():
			return nil, errs.WrapTransient(ctx.Err(), "KCoreComputer", "Compute", "context cancelled during peeling")
		default:
		}

		if removed[v] {
			continue
		}

		// Core number is current degree
		coreNumbers[v] = degree[v]
		removed[v] = true

		// Decrease degree of remaining neighbors
		for neighbor := range neighbors[v] {
			if !removed[neighbor] && degree[neighbor] > coreNumbers[v] {
				degree[neighbor]--
			}
		}
	}

	// Find max core
	maxCore := 0
	for _, core := range coreNumbers {
		if core > maxCore {
			maxCore = core
		}
	}

	// Build core buckets
	coreBuckets := make(map[int][]string)
	for entityID, core := range coreNumbers {
		coreBuckets[core] = append(coreBuckets[core], entityID)
	}

	elapsed := time.Since(startTime)
	c.logger.Info("k-core decomposition complete",
		slog.Int("entity_count", len(entityIDs)),
		slog.Int("max_core", maxCore),
		slog.Int("bucket_count", len(coreBuckets)),
		slog.Duration("elapsed", elapsed))

	return &KCoreIndex{
		CoreNumbers: coreNumbers,
		MaxCore:     maxCore,
		CoreBuckets: coreBuckets,
		ComputedAt:  time.Now(),
		EntityCount: len(entityIDs),
	}, nil
}

// ComputeIncremental updates k-core index for changed entities.
// For now, this performs a full recomputation. True incremental k-core
// algorithms exist but are complex and may not be worth the effort
// given typical graph sizes and computation frequency.
func (c *KCoreComputer) ComputeIncremental(ctx context.Context, _ []string) (*KCoreIndex, error) {
	// TODO: Implement true incremental k-core if performance requires it
	// For now, full recomputation is acceptable given:
	// - K-core is O(V+E), relatively fast
	// - Typically runs with community detection (not on every entity change)
	return c.Compute(ctx)
}
