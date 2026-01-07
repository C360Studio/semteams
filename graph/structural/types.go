// Package structuralindex provides structural graph indexing algorithms
// for query optimization and inference detection.
//
// The package implements two complementary indexing strategies:
//   - K-core decomposition: identifies the dense backbone of the graph
//   - Pivot-based distance indexing: enables O(1) distance estimation
//
// These indices support both query optimization (filtering noise, pruning traversals)
// and structural inference (detecting anomalies like semantic-structural gaps).
package structuralindex

import (
	"time"
)

const (
	// MaxHopDistance is the sentinel value indicating unreachable nodes
	// in pivot distance vectors. Using 255 allows single-byte storage.
	MaxHopDistance = 255

	// DefaultPivotCount is the default number of landmark nodes for distance indexing.
	// Values between 10-20 typically provide good accuracy/storage tradeoff.
	DefaultPivotCount = 16

	// DefaultPageRankIterations is the number of iterations for PageRank pivot selection.
	DefaultPageRankIterations = 20

	// DefaultPageRankDamping is the damping factor for PageRank (probability of following links).
	DefaultPageRankDamping = 0.85
)

// KCoreIndex stores k-core decomposition results.
//
// K-core decomposition identifies nested subgraphs where each node has
// at least k neighbors within the same subgraph. Higher core numbers
// indicate more central, densely connected nodes.
//
// Use cases:
//   - Filter noise: exclude low-core (peripheral) nodes from search results
//   - Hub detection: high-core nodes are graph backbone/hubs
//   - Anomaly detection: core demotion signals structural changes
type KCoreIndex struct {
	// CoreNumbers maps entity ID to its core number.
	// Core number k means the entity has at least k neighbors also in core k or higher.
	CoreNumbers map[string]int `json:"core_numbers"`

	// MaxCore is the highest core number found in the graph (the innermost core).
	MaxCore int `json:"max_core"`

	// CoreBuckets groups entities by core number for efficient filtering.
	// Key: core number, Value: slice of entity IDs in that core.
	CoreBuckets map[int][]string `json:"core_buckets"`

	// ComputedAt records when the index was computed.
	ComputedAt time.Time `json:"computed_at"`

	// EntityCount is the total number of entities in the index.
	EntityCount int `json:"entity_count"`
}

// GetCore returns the core number for an entity.
// Returns 0 if the entity is not in the index.
func (idx *KCoreIndex) GetCore(entityID string) int {
	if idx == nil || idx.CoreNumbers == nil {
		return 0
	}
	return idx.CoreNumbers[entityID]
}

// FilterByMinCore returns only entities with core number >= minCore.
// Useful for excluding peripheral/leaf nodes from query results.
// Returns nil if the index is nil, or the original slice if minCore <= 0.
func (idx *KCoreIndex) FilterByMinCore(entityIDs []string, minCore int) []string {
	if idx == nil || idx.CoreNumbers == nil {
		return nil
	}
	if minCore <= 0 {
		return entityIDs
	}

	filtered := make([]string, 0, len(entityIDs))
	for _, id := range entityIDs {
		if idx.CoreNumbers[id] >= minCore {
			filtered = append(filtered, id)
		}
	}
	return filtered
}

// GetEntitiesInCore returns all entities with exactly the specified core number.
func (idx *KCoreIndex) GetEntitiesInCore(core int) []string {
	if idx == nil || idx.CoreBuckets == nil {
		return nil
	}
	return idx.CoreBuckets[core]
}

// GetEntitiesAboveCore returns all entities with core number >= minCore.
func (idx *KCoreIndex) GetEntitiesAboveCore(minCore int) []string {
	if idx == nil || idx.CoreBuckets == nil {
		return nil
	}

	var result []string
	for core, entities := range idx.CoreBuckets {
		if core >= minCore {
			result = append(result, entities...)
		}
	}
	return result
}

// PivotIndex stores pivot-based distance vectors for O(1) distance estimation.
//
// The index pre-computes shortest path distances from every node to a small
// set of "pivot" nodes (selected by PageRank centrality). Distance between
// any two nodes can then be bounded using the triangle inequality:
//
//	lower_bound = max |d(A,pivot) - d(B,pivot)| over all pivots
//	upper_bound = min d(A,pivot) + d(B,pivot) over all pivots
//
// Use cases:
//   - Multi-hop filtering: quickly determine if two entities are within N hops
//   - PathRAG optimization: prune candidates before expensive traversal
//   - Semantic-structural gap detection: find semantically similar but structurally distant pairs
type PivotIndex struct {
	// Pivots is the ordered list of pivot entity IDs (selected by PageRank).
	// The order is significant: DistanceVectors[entity][i] corresponds to Pivots[i].
	Pivots []string `json:"pivots"`

	// DistanceVectors maps entity ID to its distance vector.
	// Vector[i] = shortest path distance to Pivots[i].
	// Value of MaxHopDistance (255) indicates unreachable.
	DistanceVectors map[string][]int `json:"distance_vectors"`

	// ComputedAt records when the index was computed.
	ComputedAt time.Time `json:"computed_at"`

	// EntityCount is the total number of entities in the index.
	EntityCount int `json:"entity_count"`
}

// EstimateDistance returns lower and upper bounds for the shortest path distance
// between two entities using the triangle inequality.
//
// Returns (MaxHopDistance, MaxHopDistance) if either entity is not in the index
// or if the entities are in disconnected components (no shared reachable pivots).
func (idx *PivotIndex) EstimateDistance(entityA, entityB string) (lower, upper int) {
	if idx == nil || idx.DistanceVectors == nil {
		return MaxHopDistance, MaxHopDistance
	}

	vecA, okA := idx.DistanceVectors[entityA]
	vecB, okB := idx.DistanceVectors[entityB]
	if !okA || !okB {
		return MaxHopDistance, MaxHopDistance
	}

	lower = 0
	upper = MaxHopDistance
	hasValidPivot := false

	for i := range idx.Pivots {
		if i >= len(vecA) || i >= len(vecB) {
			break
		}

		dA := vecA[i]
		dB := vecB[i]

		// Skip if either is unreachable from this pivot
		if dA == MaxHopDistance || dB == MaxHopDistance {
			continue
		}

		hasValidPivot = true

		// Triangle inequality: |d(A,P) - d(B,P)| <= d(A,B) <= d(A,P) + d(B,P)
		diff := dA - dB
		if diff < 0 {
			diff = -diff
		}
		sum := dA + dB

		if diff > lower {
			lower = diff
		}
		if sum < upper {
			upper = sum
		}
	}

	// If no pivot could reach both entities, they're in disconnected components
	if !hasValidPivot {
		return MaxHopDistance, MaxHopDistance
	}

	return lower, upper
}

// IsWithinHops returns true if the two entities are estimated to be within maxHops.
// Uses the lower bound from triangle inequality - if lower > maxHops, definitely not within range.
// If lower <= maxHops but upper > maxHops, result is uncertain (returns true to be conservative).
func (idx *PivotIndex) IsWithinHops(entityA, entityB string, maxHops int) bool {
	lower, _ := idx.EstimateDistance(entityA, entityB)
	return lower <= maxHops
}

// GetReachableCandidates returns entity IDs that might be within maxHops of source.
// Uses lower bound filtering - excludes entities definitely too far away.
// May include some entities that are actually farther (false positives are acceptable,
// false negatives are not). Entities in disconnected components are excluded.
func (idx *PivotIndex) GetReachableCandidates(source string, maxHops int) []string {
	if idx == nil || idx.DistanceVectors == nil {
		return nil
	}

	vecSource, ok := idx.DistanceVectors[source]
	if !ok {
		return nil
	}

	candidates := make([]string, 0)
	for entityID, vec := range idx.DistanceVectors {
		if entityID == source {
			continue
		}

		// Compute lower bound using triangle inequality
		lower := 0
		hasValidPivot := false
		for i := range idx.Pivots {
			if i >= len(vecSource) || i >= len(vec) {
				break
			}

			dSource := vecSource[i]
			dEntity := vec[i]

			// Skip if either is unreachable from this pivot
			if dSource == MaxHopDistance || dEntity == MaxHopDistance {
				continue
			}

			hasValidPivot = true

			diff := dSource - dEntity
			if diff < 0 {
				diff = -diff
			}
			if diff > lower {
				lower = diff
			}
		}

		// Exclude if no pivot can reach both (disconnected components)
		if !hasValidPivot {
			continue
		}

		// Include if lower bound is within range
		if lower <= maxHops {
			candidates = append(candidates, entityID)
		}
	}

	return candidates
}

// StructuralIndices bundles both indices together for convenience.
type StructuralIndices struct {
	KCore *KCoreIndex `json:"kcore,omitempty"`
	Pivot *PivotIndex `json:"pivot,omitempty"`
}
