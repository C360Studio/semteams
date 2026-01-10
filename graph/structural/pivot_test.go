package structural

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPivotComputer_EmptyGraph(t *testing.T) {
	provider := &mockGraphProvider{
		entities:  []string{},
		neighbors: map[string][]string{},
	}

	computer := NewPivotComputer(provider, 4, nil)
	index, err := computer.Compute(context.Background())

	require.NoError(t, err)
	assert.NotNil(t, index)
	assert.Equal(t, 0, index.EntityCount)
	assert.Empty(t, index.Pivots)
	assert.Empty(t, index.DistanceVectors)
}

func TestPivotComputer_SingleNode(t *testing.T) {
	provider := &mockGraphProvider{
		entities:  []string{"A"},
		neighbors: map[string][]string{"A": {}},
	}

	computer := NewPivotComputer(provider, 4, nil)
	index, err := computer.Compute(context.Background())

	require.NoError(t, err)
	assert.Equal(t, 1, index.EntityCount)
	assert.Equal(t, 1, len(index.Pivots))
	assert.Equal(t, "A", index.Pivots[0])

	// A's distance to itself (as pivot) should be 0
	assert.Equal(t, 0, index.DistanceVectors["A"][0])
}

func TestPivotComputer_SimpleChain(t *testing.T) {
	// Chain: A -- B -- C -- D
	provider := &mockGraphProvider{
		entities: []string{"A", "B", "C", "D"},
		neighbors: map[string][]string{
			"A": {"B"},
			"B": {"A", "C"},
			"C": {"B", "D"},
			"D": {"C"},
		},
	}

	// Use all 4 nodes as pivots for simple verification
	computer := NewPivotComputer(provider, 4, nil)
	index, err := computer.Compute(context.Background())

	require.NoError(t, err)
	assert.Equal(t, 4, index.EntityCount)
	assert.Equal(t, 4, len(index.Pivots))

	// Test distance estimation
	// A to D should be distance 3
	lower, upper := index.EstimateDistance("A", "D")
	assert.LessOrEqual(t, lower, 3, "lower bound should be <= actual distance")
	assert.GreaterOrEqual(t, upper, 3, "upper bound should be >= actual distance")

	// A to B should be distance 1
	lower, upper = index.EstimateDistance("A", "B")
	assert.LessOrEqual(t, lower, 1)
	assert.GreaterOrEqual(t, upper, 1)
}

func TestPivotComputer_Triangle(t *testing.T) {
	// Triangle: A -- B -- C -- A
	provider := &mockGraphProvider{
		entities: []string{"A", "B", "C"},
		neighbors: map[string][]string{
			"A": {"B", "C"},
			"B": {"A", "C"},
			"C": {"A", "B"},
		},
	}

	computer := NewPivotComputer(provider, 3, nil)
	index, err := computer.Compute(context.Background())

	require.NoError(t, err)
	assert.Equal(t, 3, index.EntityCount)

	// All pairs should be distance 1
	for _, pair := range [][2]string{{"A", "B"}, {"B", "C"}, {"A", "C"}} {
		lower, upper := index.EstimateDistance(pair[0], pair[1])
		assert.LessOrEqual(t, lower, 1, "lower bound for %s-%s", pair[0], pair[1])
		assert.GreaterOrEqual(t, upper, 1, "upper bound for %s-%s", pair[0], pair[1])
	}
}

func TestPivotComputer_DisconnectedComponents(t *testing.T) {
	// Two disconnected pairs
	provider := &mockGraphProvider{
		entities: []string{"A", "B", "X", "Y"},
		neighbors: map[string][]string{
			"A": {"B"},
			"B": {"A"},
			"X": {"Y"},
			"Y": {"X"},
		},
	}

	computer := NewPivotComputer(provider, 4, nil)
	index, err := computer.Compute(context.Background())

	require.NoError(t, err)

	// Connected pairs should have low distance
	lower, upper := index.EstimateDistance("A", "B")
	assert.LessOrEqual(t, lower, 1)
	assert.GreaterOrEqual(t, upper, 1)

	// Disconnected pairs should have MaxHopDistance
	lower, upper = index.EstimateDistance("A", "X")
	assert.Equal(t, MaxHopDistance, lower)
	assert.Equal(t, MaxHopDistance, upper)
}

func TestPivotComputer_PageRankSelectsCentralNodes(t *testing.T) {
	// Star graph: H is the hub connected to A, B, C, D
	// H should be selected as a pivot (highest PageRank)
	provider := &mockGraphProvider{
		entities: []string{"H", "A", "B", "C", "D"},
		neighbors: map[string][]string{
			"H": {"A", "B", "C", "D"},
			"A": {"H"},
			"B": {"H"},
			"C": {"H"},
			"D": {"H"},
		},
	}

	// Select only 1 pivot - should be H
	computer := NewPivotComputer(provider, 1, nil)
	index, err := computer.Compute(context.Background())

	require.NoError(t, err)
	assert.Equal(t, 1, len(index.Pivots))
	assert.Equal(t, "H", index.Pivots[0], "hub should be selected as pivot")
}

func TestPivotIndex_EstimateDistance_TriangleInequality(t *testing.T) {
	// Verify triangle inequality bounds are valid
	// Graph: A -- B -- C -- D -- E (chain of 5)
	provider := &mockGraphProvider{
		entities: []string{"A", "B", "C", "D", "E"},
		neighbors: map[string][]string{
			"A": {"B"},
			"B": {"A", "C"},
			"C": {"B", "D"},
			"D": {"C", "E"},
			"E": {"D"},
		},
	}

	computer := NewPivotComputer(provider, 5, nil)
	index, err := computer.Compute(context.Background())
	require.NoError(t, err)

	// Test all pairs - bounds should satisfy: lower <= actual <= upper
	actualDistances := map[[2]string]int{
		{"A", "B"}: 1, {"A", "C"}: 2, {"A", "D"}: 3, {"A", "E"}: 4,
		{"B", "C"}: 1, {"B", "D"}: 2, {"B", "E"}: 3,
		{"C", "D"}: 1, {"C", "E"}: 2,
		{"D", "E"}: 1,
	}

	for pair, actual := range actualDistances {
		lower, upper := index.EstimateDistance(pair[0], pair[1])
		assert.LessOrEqual(t, lower, actual, "lower bound for %s-%s", pair[0], pair[1])
		assert.GreaterOrEqual(t, upper, actual, "upper bound for %s-%s", pair[0], pair[1])
	}
}

func TestPivotIndex_IsWithinHops(t *testing.T) {
	index := &PivotIndex{
		Pivots: []string{"P1", "P2"},
		DistanceVectors: map[string][]int{
			"A":  {0, 3},
			"B":  {1, 2},
			"C":  {2, 1},
			"P1": {0, 3},
			"P2": {3, 0},
		},
	}

	// A-B: lower = |0-1| = 1, upper = 0+1 = 1 (considering P1) or |3-2|=1, 3+2=5 (P2)
	// Best bounds: lower=1, upper=1
	assert.True(t, index.IsWithinHops("A", "B", 1))
	assert.True(t, index.IsWithinHops("A", "B", 2))
	assert.False(t, index.IsWithinHops("A", "B", 0))
}

func TestPivotIndex_GetReachableCandidates(t *testing.T) {
	// Simple index where we can predict results
	index := &PivotIndex{
		Pivots: []string{"P"},
		DistanceVectors: map[string][]int{
			"P": {0},
			"A": {1},
			"B": {2},
			"C": {3},
			"D": {MaxHopDistance}, // Unreachable
		},
	}

	// From A (distance 1 to P), candidates within 2 hops
	// Lower bounds from A: P=|1-0|=1, B=|1-2|=1, C=|1-3|=2
	candidates := index.GetReachableCandidates("A", 2)
	assert.Contains(t, candidates, "P")
	assert.Contains(t, candidates, "B")
	assert.Contains(t, candidates, "C")
	assert.NotContains(t, candidates, "A") // Source excluded
	assert.NotContains(t, candidates, "D") // Unreachable
}

func TestPivotIndex_NilSafety(t *testing.T) {
	var nilIndex *PivotIndex

	lower, upper := nilIndex.EstimateDistance("A", "B")
	assert.Equal(t, MaxHopDistance, lower)
	assert.Equal(t, MaxHopDistance, upper)

	assert.True(t, nilIndex.IsWithinHops("A", "B", MaxHopDistance))
	assert.Nil(t, nilIndex.GetReachableCandidates("A", 5))
}

func TestPivotIndex_UnknownEntity(t *testing.T) {
	index := &PivotIndex{
		Pivots: []string{"P"},
		DistanceVectors: map[string][]int{
			"P": {0},
			"A": {1},
		},
	}

	// Unknown entity should return MaxHopDistance bounds
	lower, upper := index.EstimateDistance("A", "UNKNOWN")
	assert.Equal(t, MaxHopDistance, lower)
	assert.Equal(t, MaxHopDistance, upper)

	lower, upper = index.EstimateDistance("UNKNOWN", "A")
	assert.Equal(t, MaxHopDistance, lower)
	assert.Equal(t, MaxHopDistance, upper)
}

func TestPivotComputer_LimitsPivotCount(t *testing.T) {
	// Graph with only 3 nodes, but requesting 10 pivots
	provider := &mockGraphProvider{
		entities: []string{"A", "B", "C"},
		neighbors: map[string][]string{
			"A": {"B"},
			"B": {"A", "C"},
			"C": {"B"},
		},
	}

	computer := NewPivotComputer(provider, 10, nil)
	index, err := computer.Compute(context.Background())

	require.NoError(t, err)
	// Should be limited to 3 (entity count)
	assert.Equal(t, 3, len(index.Pivots))
}

func TestPivotComputer_DefaultPivotCount(t *testing.T) {
	provider := &mockGraphProvider{
		entities:  []string{},
		neighbors: map[string][]string{},
	}

	// Zero pivot count should use default
	computer := NewPivotComputer(provider, 0, nil)
	assert.Equal(t, DefaultPivotCount, computer.pivotCount)

	// Negative pivot count should use default
	computer = NewPivotComputer(provider, -5, nil)
	assert.Equal(t, DefaultPivotCount, computer.pivotCount)
}

func TestPivotComputer_DanglingNodes(t *testing.T) {
	// Graph with dangling nodes (no outgoing edges)
	// D is a "sink" - has incoming edges but no outgoing
	// PageRank should still work correctly, redistributing D's score
	provider := &mockGraphProvider{
		entities: []string{"A", "B", "C", "D"},
		neighbors: map[string][]string{
			"A": {"B", "D"},
			"B": {"C", "D"},
			"C": {"D"},
			"D": {}, // Dangling node - no outgoing edges
		},
	}

	computer := NewPivotComputer(provider, 4, nil)
	index, err := computer.Compute(context.Background())

	require.NoError(t, err)
	assert.Equal(t, 4, index.EntityCount)
	assert.Len(t, index.Pivots, 4)

	// D should be selected as a pivot (likely first) because it receives
	// PageRank from A, B, C and redistributes uniformly
	assert.Contains(t, index.Pivots, "D")

	// All entities should have distance vectors
	for _, id := range []string{"A", "B", "C", "D"} {
		assert.Contains(t, index.DistanceVectors, id)
	}
}
