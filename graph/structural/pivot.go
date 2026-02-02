package structural

import (
	"context"
	"log/slog"
	"sort"
	"time"

	"github.com/c360studio/semstreams/pkg/errs"
)

// PivotComputer computes pivot-based distance index for a graph.
//
// The algorithm:
//  1. Select k pivot nodes using PageRank (central nodes make better pivots)
//  2. Run BFS from each pivot to compute distances to all reachable nodes
//  3. Store distance vectors for each node
//
// Distance between any two nodes can then be bounded using triangle inequality:
//
//	lower = max |d(A,pivot) - d(B,pivot)| over all pivots
//	upper = min d(A,pivot) + d(B,pivot) over all pivots
//
// Time complexity: O(k * (V + E)) where k = pivot count
// Space complexity: O(V * k)
type PivotComputer struct {
	provider   Provider
	pivotCount int
	logger     *slog.Logger
}

// NewPivotComputer creates a new pivot computer.
func NewPivotComputer(provider Provider, pivotCount int, logger *slog.Logger) *PivotComputer {
	if pivotCount <= 0 {
		pivotCount = DefaultPivotCount
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &PivotComputer{
		provider:   provider,
		pivotCount: pivotCount,
		logger:     logger,
	}
}

// Compute builds the pivot index for the graph.
func (c *PivotComputer) Compute(ctx context.Context) (*PivotIndex, error) {
	startTime := time.Now()

	// Get all entity IDs
	entityIDs, err := c.provider.GetAllEntityIDs(ctx)
	if err != nil {
		return nil, errs.WrapTransient(err, "PivotComputer", "Compute", "get entity IDs")
	}

	if len(entityIDs) == 0 {
		return &PivotIndex{
			Pivots:          []string{},
			DistanceVectors: make(map[string][]int),
			ComputedAt:      time.Now(),
			EntityCount:     0,
		}, nil
	}

	// Build adjacency for BFS
	neighbors, err := c.buildAdjacency(ctx, entityIDs)
	if err != nil {
		return nil, err
	}

	// Select pivots using PageRank
	pivots, err := c.selectPivots(ctx, entityIDs, neighbors)
	if err != nil {
		return nil, err
	}

	// Compute distance vectors via BFS from each pivot
	distanceVectors, err := c.computeDistanceVectors(ctx, entityIDs, neighbors, pivots)
	if err != nil {
		return nil, err
	}

	elapsed := time.Since(startTime)
	c.logger.Info("pivot index computation complete",
		slog.Int("entity_count", len(entityIDs)),
		slog.Int("pivot_count", len(pivots)),
		slog.Duration("elapsed", elapsed))

	return &PivotIndex{
		Pivots:          pivots,
		DistanceVectors: distanceVectors,
		ComputedAt:      time.Now(),
		EntityCount:     len(entityIDs),
	}, nil
}

// buildAdjacency constructs the adjacency list for all entities.
func (c *PivotComputer) buildAdjacency(ctx context.Context, entityIDs []string) (map[string][]string, error) {
	neighbors := make(map[string][]string, len(entityIDs))

	for _, entityID := range entityIDs {
		select {
		case <-ctx.Done():
			return nil, errs.WrapTransient(ctx.Err(), "PivotComputer", "buildAdjacency", "context cancelled")
		default:
		}

		neighborList, err := c.provider.GetNeighbors(ctx, entityID, "both")
		if err != nil {
			// Log warning but continue
			c.logger.Warn("failed to get neighbors",
				slog.String("entity_id", entityID),
				slog.String("error", err.Error()))
			neighborList = []string{}
		}
		neighbors[entityID] = neighborList
	}

	return neighbors, nil
}

// selectPivots selects pivot nodes using PageRank.
// Central nodes (high PageRank) make better pivots because they're
// more likely to be on shortest paths between other nodes.
func (c *PivotComputer) selectPivots(ctx context.Context, entityIDs []string, neighbors map[string][]string) ([]string, error) {
	n := len(entityIDs)
	if n == 0 {
		return []string{}, nil
	}

	// Limit pivot count to entity count
	pivotCount := c.pivotCount
	if pivotCount > n {
		pivotCount = n
	}

	// Compute out-degrees for normalization
	outDegree := make(map[string]int, n)
	for id, neighs := range neighbors {
		outDegree[id] = len(neighs)
	}

	// Run PageRank to get scores
	scores, err := c.runPageRank(ctx, entityIDs, neighbors, outDegree)
	if err != nil {
		return nil, err
	}

	// Sort and select top-k pivots
	return c.selectTopPivots(entityIDs, scores, pivotCount)
}

// runPageRank executes the PageRank algorithm and returns final scores.
func (c *PivotComputer) runPageRank(ctx context.Context, entityIDs []string, neighbors map[string][]string, outDegree map[string]int) (map[string]float64, error) {
	n := len(entityIDs)
	damping := DefaultPageRankDamping
	teleport := (1.0 - damping) / float64(n)

	// Initialize scores
	scores := make(map[string]float64, n)
	initialScore := 1.0 / float64(n)
	for _, id := range entityIDs {
		scores[id] = initialScore
	}

	for iter := 0; iter < DefaultPageRankIterations; iter++ {
		select {
		case <-ctx.Done():
			return nil, errs.WrapTransient(ctx.Err(), "PivotComputer", "runPageRank", "context cancelled")
		default:
		}

		newScores := make(map[string]float64, n)
		for _, id := range entityIDs {
			newScores[id] = teleport
		}

		// Collect and redistribute dangling node scores
		danglingScore := 0.0
		for _, id := range entityIDs {
			if outDegree[id] == 0 {
				danglingScore += scores[id]
			}
		}
		if danglingScore > 0 {
			danglingContribution := damping * danglingScore / float64(n)
			for _, id := range entityIDs {
				newScores[id] += danglingContribution
			}
		}

		// Distribute scores along edges
		for id, neighs := range neighbors {
			if outDegree[id] == 0 {
				continue
			}
			contribution := damping * scores[id] / float64(outDegree[id])
			for _, neighbor := range neighs {
				newScores[neighbor] += contribution
			}
		}

		scores = newScores
	}

	return scores, nil
}

// selectTopPivots sorts entities by score and returns top-k as pivots.
func (c *PivotComputer) selectTopPivots(entityIDs []string, scores map[string]float64, pivotCount int) ([]string, error) {
	type entityScore struct {
		id    string
		score float64
	}

	entitySet := make(map[string]bool, len(entityIDs))
	for _, id := range entityIDs {
		entitySet[id] = true
	}

	sorted := make([]entityScore, 0, len(entityIDs))
	for id, score := range scores {
		if entitySet[id] {
			sorted = append(sorted, entityScore{id: id, score: score})
		}
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].score > sorted[j].score
	})

	pivots := make([]string, pivotCount)
	for i := 0; i < pivotCount; i++ {
		pivots[i] = sorted[i].id
	}

	c.logger.Debug("selected pivots via PageRank",
		slog.Int("pivot_count", pivotCount),
		slog.Float64("top_score", sorted[0].score),
		slog.Float64("min_pivot_score", sorted[pivotCount-1].score))

	return pivots, nil
}

// computeDistanceVectors runs BFS from each pivot to compute distances.
func (c *PivotComputer) computeDistanceVectors(ctx context.Context, entityIDs []string, neighbors map[string][]string, pivots []string) (map[string][]int, error) {
	// Initialize distance vectors with MaxHopDistance (unreachable)
	distanceVectors := make(map[string][]int, len(entityIDs))
	for _, id := range entityIDs {
		vec := make([]int, len(pivots))
		for i := range vec {
			vec[i] = MaxHopDistance
		}
		distanceVectors[id] = vec
	}

	// BFS from each pivot
	for pivotIdx, pivot := range pivots {
		select {
		case <-ctx.Done():
			return nil, errs.WrapTransient(ctx.Err(), "PivotComputer", "computeDistanceVectors", "context cancelled during BFS")
		default:
		}

		c.bfsFromPivot(pivot, pivotIdx, neighbors, distanceVectors)
	}

	return distanceVectors, nil
}

// bfsFromPivot performs BFS from a pivot and updates distance vectors.
func (c *PivotComputer) bfsFromPivot(pivot string, pivotIdx int, neighbors map[string][]string, distanceVectors map[string][]int) {
	// Defensive check: ensure pivot has a distance vector
	pivotVec, exists := distanceVectors[pivot]
	if !exists || pivotIdx >= len(pivotVec) {
		c.logger.Warn("pivot missing from distance vectors, skipping BFS",
			slog.String("pivot", pivot),
			slog.Int("pivot_idx", pivotIdx))
		return
	}

	visited := make(map[string]bool)
	queue := []string{pivot}
	distance := 0

	visited[pivot] = true
	pivotVec[pivotIdx] = 0

	for len(queue) > 0 {
		// Process all nodes at current distance
		nextQueue := []string{}

		for _, node := range queue {
			for _, neighbor := range neighbors[node] {
				if !visited[neighbor] {
					visited[neighbor] = true
					// Skip neighbors not in our entity set (they won't have distance vectors)
					if _, exists := distanceVectors[neighbor]; !exists {
						continue
					}
					distanceVectors[neighbor][pivotIdx] = distance + 1
					nextQueue = append(nextQueue, neighbor)
				}
			}
		}

		queue = nextQueue
		distance++

		// Safety limit to prevent runaway in case of bugs
		if distance > MaxHopDistance {
			break
		}
	}
}

// ComputeIncremental updates pivot index for changed entities.
// For now, this performs a full recomputation.
func (c *PivotComputer) ComputeIncremental(ctx context.Context, _ []string) (*PivotIndex, error) {
	// TODO: Consider incremental updates for large graphs
	// Options:
	// - Only recompute BFS for affected pivots
	// - Update distance vectors for affected nodes only
	// For now, full recomputation is acceptable given typical usage patterns
	return c.Compute(ctx)
}
