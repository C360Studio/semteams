package structural

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockProvider implements clustering.Provider for testing
type mockProvider struct {
	entities  []string
	neighbors map[string][]string
}

func (m *mockProvider) GetAllEntityIDs(_ context.Context) ([]string, error) {
	return m.entities, nil
}

func (m *mockProvider) GetNeighbors(_ context.Context, entityID string, _ string) ([]string, error) {
	return m.neighbors[entityID], nil
}

func (m *mockProvider) GetEdgeWeight(_ context.Context, _, _ string) (float64, error) {
	return 1.0, nil
}

func TestKCoreComputer_EmptyGraph(t *testing.T) {
	provider := &mockProvider{
		entities:  []string{},
		neighbors: map[string][]string{},
	}

	computer := NewKCoreComputer(provider, nil)
	index, err := computer.Compute(context.Background())

	require.NoError(t, err)
	assert.NotNil(t, index)
	assert.Equal(t, 0, index.MaxCore)
	assert.Equal(t, 0, index.EntityCount)
	assert.Empty(t, index.CoreNumbers)
}

func TestKCoreComputer_SingleNode(t *testing.T) {
	provider := &mockProvider{
		entities:  []string{"A"},
		neighbors: map[string][]string{"A": {}},
	}

	computer := NewKCoreComputer(provider, nil)
	index, err := computer.Compute(context.Background())

	require.NoError(t, err)
	assert.Equal(t, 0, index.MaxCore)
	assert.Equal(t, 1, index.EntityCount)
	assert.Equal(t, 0, index.GetCore("A"))
}

func TestKCoreComputer_SimpleChain(t *testing.T) {
	// Chain: A -- B -- C -- D
	// All nodes should be core-1 (each has 1 or 2 neighbors)
	provider := &mockProvider{
		entities: []string{"A", "B", "C", "D"},
		neighbors: map[string][]string{
			"A": {"B"},
			"B": {"A", "C"},
			"C": {"B", "D"},
			"D": {"C"},
		},
	}

	computer := NewKCoreComputer(provider, nil)
	index, err := computer.Compute(context.Background())

	require.NoError(t, err)
	assert.Equal(t, 4, index.EntityCount)
	assert.Equal(t, 1, index.MaxCore)

	// End nodes should be core-1
	assert.Equal(t, 1, index.GetCore("A"))
	assert.Equal(t, 1, index.GetCore("D"))
	// Middle nodes should also be core-1 (after peeling removes ends)
	assert.Equal(t, 1, index.GetCore("B"))
	assert.Equal(t, 1, index.GetCore("C"))
}

func TestKCoreComputer_Triangle(t *testing.T) {
	// Triangle: A -- B -- C -- A
	// All nodes should be core-2 (each has exactly 2 neighbors in the clique)
	provider := &mockProvider{
		entities: []string{"A", "B", "C"},
		neighbors: map[string][]string{
			"A": {"B", "C"},
			"B": {"A", "C"},
			"C": {"A", "B"},
		},
	}

	computer := NewKCoreComputer(provider, nil)
	index, err := computer.Compute(context.Background())

	require.NoError(t, err)
	assert.Equal(t, 3, index.EntityCount)
	assert.Equal(t, 2, index.MaxCore)

	assert.Equal(t, 2, index.GetCore("A"))
	assert.Equal(t, 2, index.GetCore("B"))
	assert.Equal(t, 2, index.GetCore("C"))
}

func TestKCoreComputer_TriangleWithLeaf(t *testing.T) {
	// Triangle with a leaf: D -- A -- B -- C -- A
	// D should be core-1, A/B/C should be core-2
	provider := &mockProvider{
		entities: []string{"A", "B", "C", "D"},
		neighbors: map[string][]string{
			"A": {"B", "C", "D"},
			"B": {"A", "C"},
			"C": {"A", "B"},
			"D": {"A"},
		},
	}

	computer := NewKCoreComputer(provider, nil)
	index, err := computer.Compute(context.Background())

	require.NoError(t, err)
	assert.Equal(t, 4, index.EntityCount)
	assert.Equal(t, 2, index.MaxCore)

	assert.Equal(t, 1, index.GetCore("D"))
	assert.Equal(t, 2, index.GetCore("A"))
	assert.Equal(t, 2, index.GetCore("B"))
	assert.Equal(t, 2, index.GetCore("C"))
}

func TestKCoreComputer_DisconnectedComponents(t *testing.T) {
	// Two disconnected triangles
	provider := &mockProvider{
		entities: []string{"A", "B", "C", "X", "Y", "Z"},
		neighbors: map[string][]string{
			"A": {"B", "C"},
			"B": {"A", "C"},
			"C": {"A", "B"},
			"X": {"Y", "Z"},
			"Y": {"X", "Z"},
			"Z": {"X", "Y"},
		},
	}

	computer := NewKCoreComputer(provider, nil)
	index, err := computer.Compute(context.Background())

	require.NoError(t, err)
	assert.Equal(t, 6, index.EntityCount)
	assert.Equal(t, 2, index.MaxCore)

	// Both components should have core-2
	for _, id := range []string{"A", "B", "C", "X", "Y", "Z"} {
		assert.Equal(t, 2, index.GetCore(id), "entity %s should be core-2", id)
	}
}

func TestKCoreComputer_CompleteGraph(t *testing.T) {
	// K4 complete graph: all nodes connected to all others
	// Should be core-3 (each node has 3 neighbors)
	provider := &mockProvider{
		entities: []string{"A", "B", "C", "D"},
		neighbors: map[string][]string{
			"A": {"B", "C", "D"},
			"B": {"A", "C", "D"},
			"C": {"A", "B", "D"},
			"D": {"A", "B", "C"},
		},
	}

	computer := NewKCoreComputer(provider, nil)
	index, err := computer.Compute(context.Background())

	require.NoError(t, err)
	assert.Equal(t, 3, index.MaxCore)

	for _, id := range []string{"A", "B", "C", "D"} {
		assert.Equal(t, 3, index.GetCore(id))
	}
}

func TestKCoreComputer_HubAndSpoke(t *testing.T) {
	// Hub-and-spoke: H connected to A, B, C, D (no inter-spoke connections)
	// All should be core-1 (H has degree 4, but spokes only have degree 1)
	provider := &mockProvider{
		entities: []string{"H", "A", "B", "C", "D"},
		neighbors: map[string][]string{
			"H": {"A", "B", "C", "D"},
			"A": {"H"},
			"B": {"H"},
			"C": {"H"},
			"D": {"H"},
		},
	}

	computer := NewKCoreComputer(provider, nil)
	index, err := computer.Compute(context.Background())

	require.NoError(t, err)
	assert.Equal(t, 1, index.MaxCore)

	// All nodes should be core-1 (spokes limit the hub)
	for _, id := range []string{"H", "A", "B", "C", "D"} {
		assert.Equal(t, 1, index.GetCore(id))
	}
}

func TestKCoreIndex_FilterByMinCore(t *testing.T) {
	index := &KCoreIndex{
		CoreNumbers: map[string]int{
			"A": 1, "B": 2, "C": 2, "D": 3, "E": 1,
		},
		MaxCore: 3,
	}

	tests := []struct {
		name     string
		entities []string
		minCore  int
		expected []string
	}{
		{
			name:     "filter core >= 2",
			entities: []string{"A", "B", "C", "D", "E"},
			minCore:  2,
			expected: []string{"B", "C", "D"},
		},
		{
			name:     "filter core >= 3",
			entities: []string{"A", "B", "C", "D", "E"},
			minCore:  3,
			expected: []string{"D"},
		},
		{
			name:     "filter core >= 1 (all pass)",
			entities: []string{"A", "B", "C", "D", "E"},
			minCore:  1,
			expected: []string{"A", "B", "C", "D", "E"},
		},
		{
			name:     "filter core >= 0 (no filter)",
			entities: []string{"A", "B", "C"},
			minCore:  0,
			expected: []string{"A", "B", "C"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := index.FilterByMinCore(tt.entities, tt.minCore)
			assert.ElementsMatch(t, tt.expected, result)
		})
	}
}

func TestKCoreIndex_GetEntitiesInCore(t *testing.T) {
	index := &KCoreIndex{
		CoreBuckets: map[int][]string{
			1: {"A", "E"},
			2: {"B", "C"},
			3: {"D"},
		},
	}

	assert.ElementsMatch(t, []string{"A", "E"}, index.GetEntitiesInCore(1))
	assert.ElementsMatch(t, []string{"B", "C"}, index.GetEntitiesInCore(2))
	assert.ElementsMatch(t, []string{"D"}, index.GetEntitiesInCore(3))
	assert.Nil(t, index.GetEntitiesInCore(4))
}

func TestKCoreIndex_GetEntitiesAboveCore(t *testing.T) {
	index := &KCoreIndex{
		CoreBuckets: map[int][]string{
			1: {"A", "E"},
			2: {"B", "C"},
			3: {"D"},
		},
	}

	result := index.GetEntitiesAboveCore(2)
	assert.ElementsMatch(t, []string{"B", "C", "D"}, result)

	result = index.GetEntitiesAboveCore(3)
	assert.ElementsMatch(t, []string{"D"}, result)

	result = index.GetEntitiesAboveCore(1)
	assert.ElementsMatch(t, []string{"A", "E", "B", "C", "D"}, result)
}

func TestKCoreIndex_NilSafety(t *testing.T) {
	var nilIndex *KCoreIndex

	assert.Equal(t, 0, nilIndex.GetCore("A"))
	assert.Nil(t, nilIndex.FilterByMinCore([]string{"A"}, 1))
	assert.Nil(t, nilIndex.GetEntitiesInCore(1))
	assert.Nil(t, nilIndex.GetEntitiesAboveCore(1))
}

func TestKCoreComputer_BarbellGraph(t *testing.T) {
	// Barbell graph: two K3 triangles connected by a bridge
	// Triangle 1: A-B-C (core-2)
	// Triangle 2: X-Y-Z (core-2)
	// Bridge: C-X (reduces to core-1 when removed from dense subgraph)
	//
	// Expected: Bridge endpoints should still be core-2 because they have
	// 2 neighbors within their triangle. The bridge doesn't reduce their core.
	provider := &mockProvider{
		entities: []string{"A", "B", "C", "X", "Y", "Z"},
		neighbors: map[string][]string{
			"A": {"B", "C"},
			"B": {"A", "C"},
			"C": {"A", "B", "X"}, // Bridge to X
			"X": {"C", "Y", "Z"}, // Bridge from C
			"Y": {"X", "Z"},
			"Z": {"X", "Y"},
		},
	}

	computer := NewKCoreComputer(provider, nil)
	index, err := computer.Compute(context.Background())

	require.NoError(t, err)
	assert.Equal(t, 6, index.EntityCount)
	assert.Equal(t, 2, index.MaxCore)

	// All nodes should be core-2:
	// - A, B have 2 neighbors each in core-2 (triangle)
	// - C has 3 neighbors: A, B (core-2) + X (core-2) = at least 2 in core-2
	// - X has 3 neighbors: C (core-2) + Y, Z (core-2) = at least 2 in core-2
	// - Y, Z have 2 neighbors each in core-2 (triangle)
	for _, id := range []string{"A", "B", "C", "X", "Y", "Z"} {
		assert.Equal(t, 2, index.GetCore(id), "entity %s should be core-2", id)
	}
}

func TestKCoreComputer_BarbellWithWeakBridge(t *testing.T) {
	// Two K4 cliques connected by a single bridge node
	// K4 clique 1: A-B-C-D (each has 3 neighbors in clique, core-3)
	// K4 clique 2: W-X-Y-Z (each has 3 neighbors in clique, core-3)
	// Bridge node: M connected only to D and W (degree 2)
	//
	// K-core definition: maximal subgraph where every vertex has >= k neighbors IN that subgraph
	// M has degree 2, so it can be in 2-core (both neighbors D,W are also in 2-core)
	// But M cannot be in 3-core (would need 3 neighbors all in 3-core)
	provider := &mockProvider{
		entities: []string{"A", "B", "C", "D", "M", "W", "X", "Y", "Z"},
		neighbors: map[string][]string{
			// K4 clique 1
			"A": {"B", "C", "D"},
			"B": {"A", "C", "D"},
			"C": {"A", "B", "D"},
			"D": {"A", "B", "C", "M"}, // Also connected to bridge
			// Bridge node
			"M": {"D", "W"},
			// K4 clique 2
			"W": {"M", "X", "Y", "Z"}, // Also connected to bridge
			"X": {"W", "Y", "Z"},
			"Y": {"W", "X", "Z"},
			"Z": {"W", "X", "Y"},
		},
	}

	computer := NewKCoreComputer(provider, nil)
	index, err := computer.Compute(context.Background())

	require.NoError(t, err)
	assert.Equal(t, 9, index.EntityCount)
	assert.Equal(t, 3, index.MaxCore)

	// Bridge node M has degree 2, so core-2 (both neighbors are in 2-core or higher)
	assert.Equal(t, 2, index.GetCore("M"), "bridge node M should be core-2")

	// K4 clique nodes should be core-3
	for _, id := range []string{"A", "B", "C", "D"} {
		assert.Equal(t, 3, index.GetCore(id), "K4 node %s should be core-3", id)
	}
	for _, id := range []string{"W", "X", "Y", "Z"} {
		assert.Equal(t, 3, index.GetCore(id), "K4 node %s should be core-3", id)
	}
}

func TestKCoreComputer_ChainGraph(t *testing.T) {
	// Linear chain: A-B-C-D-E
	// All nodes should be core-1 (endpoints have degree 1, middle nodes have degree 2)
	// But in k-core peeling, when we remove degree-1 nodes, neighbors' degrees drop
	provider := &mockProvider{
		entities: []string{"A", "B", "C", "D", "E"},
		neighbors: map[string][]string{
			"A": {"B"},
			"B": {"A", "C"},
			"C": {"B", "D"},
			"D": {"C", "E"},
			"E": {"D"},
		},
	}

	computer := NewKCoreComputer(provider, nil)
	index, err := computer.Compute(context.Background())

	require.NoError(t, err)
	assert.Equal(t, 5, index.EntityCount)
	assert.Equal(t, 1, index.MaxCore)

	// All nodes end up as core-1
	for _, id := range []string{"A", "B", "C", "D", "E"} {
		assert.Equal(t, 1, index.GetCore(id), "chain node %s should be core-1", id)
	}
}
