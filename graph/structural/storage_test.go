package structural

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/context"
)

func TestNATSStructuralIndexStorage_KCore_InMemory(t *testing.T) {
	// Test with nil KV (uses in-memory test store)
	storage := NewNATSStructuralIndexStorage(nil)
	ctx := context.Background()

	// Initially empty
	index, err := storage.GetKCoreIndex(ctx)
	require.NoError(t, err)
	assert.Nil(t, index)

	// Save an index
	testIndex := &KCoreIndex{
		CoreNumbers: map[string]int{
			"A": 2,
			"B": 2,
			"C": 2,
			"D": 1,
		},
		MaxCore: 2,
		CoreBuckets: map[int][]string{
			1: {"D"},
			2: {"A", "B", "C"},
		},
		ComputedAt:  time.Now(),
		EntityCount: 4,
	}

	err = storage.SaveKCoreIndex(ctx, testIndex)
	require.NoError(t, err)

	// Retrieve it
	retrieved, err := storage.GetKCoreIndex(ctx)
	require.NoError(t, err)
	require.NotNil(t, retrieved)

	assert.Equal(t, testIndex.MaxCore, retrieved.MaxCore)
	assert.Equal(t, testIndex.EntityCount, retrieved.EntityCount)
	assert.Equal(t, testIndex.CoreNumbers, retrieved.CoreNumbers)
	assert.Equal(t, testIndex.CoreBuckets, retrieved.CoreBuckets)

	// Clear
	err = storage.Clear(ctx)
	require.NoError(t, err)

	// Should be empty again
	index, err = storage.GetKCoreIndex(ctx)
	require.NoError(t, err)
	assert.Nil(t, index)
}

func TestNATSStructuralIndexStorage_Pivot_InMemory(t *testing.T) {
	storage := NewNATSStructuralIndexStorage(nil)
	ctx := context.Background()

	// Initially empty
	index, err := storage.GetPivotIndex(ctx)
	require.NoError(t, err)
	assert.Nil(t, index)

	// Save an index
	testIndex := &PivotIndex{
		Pivots: []string{"P1", "P2"},
		DistanceVectors: map[string][]int{
			"P1": {0, 2},
			"P2": {2, 0},
			"A":  {1, 1},
			"B":  {2, 2},
		},
		ComputedAt:  time.Now(),
		EntityCount: 4,
	}

	err = storage.SavePivotIndex(ctx, testIndex)
	require.NoError(t, err)

	// Retrieve it
	retrieved, err := storage.GetPivotIndex(ctx)
	require.NoError(t, err)
	require.NotNil(t, retrieved)

	assert.Equal(t, testIndex.Pivots, retrieved.Pivots)
	assert.Equal(t, testIndex.EntityCount, retrieved.EntityCount)
	assert.Equal(t, testIndex.DistanceVectors, retrieved.DistanceVectors)

	// Clear
	err = storage.Clear(ctx)
	require.NoError(t, err)

	// Should be empty again
	index, err = storage.GetPivotIndex(ctx)
	require.NoError(t, err)
	assert.Nil(t, index)
}

func TestNATSStructuralIndexStorage_NilIndex(t *testing.T) {
	storage := NewNATSStructuralIndexStorage(nil)
	ctx := context.Background()

	// Saving nil should error
	err := storage.SaveKCoreIndex(ctx, nil)
	assert.Error(t, err)

	err = storage.SavePivotIndex(ctx, nil)
	assert.Error(t, err)
}

func TestNATSStructuralIndexStorage_EmptyIndex(t *testing.T) {
	storage := NewNATSStructuralIndexStorage(nil)
	ctx := context.Background()

	// Empty but valid indices should save
	emptyKCore := &KCoreIndex{
		CoreNumbers: make(map[string]int),
		CoreBuckets: make(map[int][]string),
		ComputedAt:  time.Now(),
	}

	err := storage.SaveKCoreIndex(ctx, emptyKCore)
	require.NoError(t, err)

	retrieved, err := storage.GetKCoreIndex(ctx)
	require.NoError(t, err)
	require.NotNil(t, retrieved)
	assert.Empty(t, retrieved.CoreNumbers)

	// Clear for next test
	storage.Clear(ctx)

	emptyPivot := &PivotIndex{
		Pivots:          []string{},
		DistanceVectors: make(map[string][]int),
		ComputedAt:      time.Now(),
	}

	err = storage.SavePivotIndex(ctx, emptyPivot)
	require.NoError(t, err)

	retrievedPivot, err := storage.GetPivotIndex(ctx)
	require.NoError(t, err)
	require.NotNil(t, retrievedPivot)
	assert.Empty(t, retrievedPivot.Pivots)
}

func TestNATSStructuralIndexStorage_Overwrite(t *testing.T) {
	storage := NewNATSStructuralIndexStorage(nil)
	ctx := context.Background()

	// Save initial index
	index1 := &KCoreIndex{
		CoreNumbers: map[string]int{"A": 1},
		MaxCore:     1,
		CoreBuckets: map[int][]string{1: {"A"}},
		ComputedAt:  time.Now(),
		EntityCount: 1,
	}

	err := storage.SaveKCoreIndex(ctx, index1)
	require.NoError(t, err)

	// Overwrite with new index
	index2 := &KCoreIndex{
		CoreNumbers: map[string]int{"X": 3, "Y": 3, "Z": 3},
		MaxCore:     3,
		CoreBuckets: map[int][]string{3: {"X", "Y", "Z"}},
		ComputedAt:  time.Now(),
		EntityCount: 3,
	}

	err = storage.SaveKCoreIndex(ctx, index2)
	require.NoError(t, err)

	// Should get the new index
	retrieved, err := storage.GetKCoreIndex(ctx)
	require.NoError(t, err)
	require.NotNil(t, retrieved)

	assert.Equal(t, 3, retrieved.MaxCore)
	assert.Equal(t, 3, retrieved.EntityCount)
	assert.Contains(t, retrieved.CoreNumbers, "X")
	assert.NotContains(t, retrieved.CoreNumbers, "A") // Old data gone in memory store
}

func TestNATSStructuralIndexStorage_MaxHopDistance(t *testing.T) {
	storage := NewNATSStructuralIndexStorage(nil)
	ctx := context.Background()

	// Test that MaxHopDistance values are preserved
	testIndex := &PivotIndex{
		Pivots: []string{"P"},
		DistanceVectors: map[string][]int{
			"P":           {0},
			"A":           {1},
			"Unreachable": {MaxHopDistance},
		},
		ComputedAt:  time.Now(),
		EntityCount: 3,
	}

	err := storage.SavePivotIndex(ctx, testIndex)
	require.NoError(t, err)

	retrieved, err := storage.GetPivotIndex(ctx)
	require.NoError(t, err)
	require.NotNil(t, retrieved)

	assert.Equal(t, []int{MaxHopDistance}, retrieved.DistanceVectors["Unreachable"])
}

func TestKeyGenerators(t *testing.T) {
	// Test key format consistency
	assert.Equal(t, "structural.kcore._meta", kcoreMetaKey())
	assert.Equal(t, "structural.kcore.entity.test-entity", kcoreEntityKey("test-entity"))
	assert.Equal(t, "structural.kcore.bucket.5", kcoreBucketKey(5))
	assert.Equal(t, "structural.pivot._meta", pivotMetaKey())
	assert.Equal(t, "structural.pivot.entity.test-entity", pivotEntityKey("test-entity"))
}

func TestParseCoreFromBucketKey(t *testing.T) {
	tests := []struct {
		key      string
		expected int
	}{
		{"structural.kcore.bucket.0", 0},
		{"structural.kcore.bucket.5", 5},
		{"structural.kcore.bucket.100", 100},
		{"invalid", 0},
		{"", 0},
	}

	for _, tc := range tests {
		t.Run(tc.key, func(t *testing.T) {
			result := parseCoreFromBucketKey(tc.key)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestParseTime(t *testing.T) {
	// Valid RFC3339 time
	timeStr := "2024-01-15T10:30:00Z"
	parsed, err := parseTime(timeStr)
	require.NoError(t, err)
	assert.Equal(t, 2024, parsed.Year())
	assert.Equal(t, time.January, parsed.Month())
	assert.Equal(t, 15, parsed.Day())

	// Invalid time
	_, err = parseTime("not-a-time")
	assert.Error(t, err)
}
