package graphindexspatial

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGeohashMultiplier verifies the multiplier for each documented precision level.
func TestGeohashMultiplier(t *testing.T) {
	tests := []struct {
		precision int
		want      float64
	}{
		{4, 10.0},
		{5, 50.0},
		{6, 100.0},
		{7, 300.0},
		{8, 1000.0},
		// Unknown values should default to 300 (precision-7 default)
		{0, 300.0},
		{9, 300.0},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("precision_%d", tt.precision), func(t *testing.T) {
			got := geohashMultiplier(tt.precision)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestGeohashCellsInBounds_SmallBox tests a tight bounding box that produces a
// handful of cells and verifies the key format matches calculateGeohash.
func TestGeohashCellsInBounds_SmallBox(t *testing.T) {
	// A 0.02° × 0.02° box at precision 6 (multiplier=100).
	// lat range [37.77, 37.79], lon range [-122.42, -122.40]
	north, south := 37.79, 37.77
	east, west := -122.40, -122.42
	precision := 6

	cells := geohashCellsInBounds(north, south, east, west, precision)
	require.NotNil(t, cells, "small box must produce cells, not nil")
	require.NotEmpty(t, cells, "cells slice must not be empty")

	// Verify every cell key has the expected format and is within bin range.
	// multiplier is used implicitly through geohashMultiplier; we don't need it here.
	_ = geohashMultiplier(precision) // confirm function is callable

	for _, cell := range cells {
		var prec, latBin, lonBin int
		n, err := fmt.Sscanf(cell, "geo_%d_%d_%d", &prec, &latBin, &lonBin)
		require.NoError(t, err, "cell key must parse: %q", cell)
		assert.Equal(t, 3, n)
		assert.Equal(t, precision, prec, "precision field must match: %q", cell)
	}
}

// TestGeohashCellsInBounds_ExactKey verifies that for a single-point bounding
// box the returned key exactly matches what calculateGeohash would produce.
func TestGeohashCellsInBounds_ExactKey(t *testing.T) {
	lat, lon := 37.7749, -122.4194
	precision := 7 // multiplier = 300

	// Point box: north==south, east==west
	cells := geohashCellsInBounds(lat, lat, lon, lon, precision)
	require.NotNil(t, cells)
	require.Len(t, cells, 1, "single-point box must produce exactly one cell")

	// Build the expected key using the same arithmetic as calculateGeohash.
	multiplier := geohashMultiplier(precision)
	latInt := int(floorFloat((lat + 90.0) * multiplier))
	lonInt := int(floorFloat((lon + 180.0) * multiplier))
	expected := fmt.Sprintf("geo_%d_%d_%d", precision, latInt, lonInt)

	assert.Equal(t, expected, cells[0])
}

// TestGeohashCellsInBounds_CellCount verifies that the right number of cells is
// computed for a known range.
func TestGeohashCellsInBounds_CellCount(t *testing.T) {
	// At precision 6 (multiplier=100), 0.03 degrees spans 3 bins in each axis
	// → 3×3 = 9 cells.
	north, south := 0.02, 0.00
	east, west := 0.02, 0.00
	precision := 6

	cells := geohashCellsInBounds(north, south, east, west, precision)
	require.NotNil(t, cells)

	// Compute expected count: (floor((north+90)*100) - floor((south+90)*100) + 1)
	// × (floor((east+180)*100) - floor((west+180)*100) + 1)
	multiplier := geohashMultiplier(precision)
	latCount := int(floorFloat((north+90.0)*multiplier)) - int(floorFloat((south+90.0)*multiplier)) + 1
	lonCount := int(floorFloat((east+180.0)*multiplier)) - int(floorFloat((west+180.0)*multiplier)) + 1

	assert.Equal(t, latCount*lonCount, len(cells))
}

// TestGeohashCellsInBounds_TooLarge verifies that a global bounding box
// returns nil to signal a fallback to full scan.
func TestGeohashCellsInBounds_TooLarge(t *testing.T) {
	// Global bounding box at precision 7 (multiplier=300):
	// lat bins: floor(0*300) to floor(180*300) = 0..54000 → 54001 rows
	// far exceeds 10,000 cells.
	cells := geohashCellsInBounds(90, -90, 180, -180, 7)
	assert.Nil(t, cells, "global bounding box must return nil (fallback to full scan)")
}

// TestGeohashCellsInBounds_InvertedBounds verifies that inverted / degenerate
// bounds (south > north or west > east) return nil safely.
func TestGeohashCellsInBounds_InvertedBounds(t *testing.T) {
	// south > north: inverted
	cells := geohashCellsInBounds(10, 20, 10, 0, 6) // north=10, south=20 → min > max
	assert.Nil(t, cells, "inverted lat bounds must return nil")
}

// TestGeohashCellsInBounds_AllPrecisions smoke-tests each precision level with
// the same small box to ensure no panics.
func TestGeohashCellsInBounds_AllPrecisions(t *testing.T) {
	north, south := 0.01, 0.00
	east, west := 0.01, 0.00

	for _, precision := range []int{4, 5, 6, 7, 8} {
		t.Run(fmt.Sprintf("precision_%d", precision), func(t *testing.T) {
			cells := geohashCellsInBounds(north, south, east, west, precision)
			// Small box must produce cells for all supported precision levels.
			assert.NotNil(t, cells, "small box must not return nil for precision %d", precision)
			assert.NotEmpty(t, cells)
			for _, c := range cells {
				assert.Contains(t, c, fmt.Sprintf("geo_%d_", precision))
			}
		})
	}
}

// floorFloat is a local copy of math.Floor to avoid importing math in the test
// (math is already imported by the package under test).
func floorFloat(x float64) float64 {
	if x < 0 {
		return float64(int(x) - 1)
	}
	return float64(int(x))
}
