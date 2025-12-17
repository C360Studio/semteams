package inference

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNATSAnomalyStorage_InMemory(t *testing.T) {
	// Test with nil KV (in-memory mode)
	storage := NewNATSAnomalyStorage(nil, nil)
	require.NotNil(t, storage)
	require.NotNil(t, storage.testStore)
}

func TestNATSAnomalyStorage_Save(t *testing.T) {
	storage := NewNATSAnomalyStorage(nil, nil)
	ctx := context.Background()

	anomaly := &StructuralAnomaly{
		ID:         "test-123",
		Type:       AnomalySemanticStructuralGap,
		EntityA:    "entity-a",
		EntityB:    "entity-b",
		Confidence: 0.85,
		Status:     StatusPending,
		DetectedAt: time.Now(),
	}

	err := storage.Save(ctx, anomaly)
	assert.NoError(t, err)

	// Verify it was saved
	got, err := storage.Get(ctx, "test-123")
	assert.NoError(t, err)
	assert.NotNil(t, got)
	assert.Equal(t, "test-123", got.ID)
	assert.Equal(t, AnomalySemanticStructuralGap, got.Type)
	assert.Equal(t, 0.85, got.Confidence)
}

func TestNATSAnomalyStorage_Save_NilAnomaly(t *testing.T) {
	storage := NewNATSAnomalyStorage(nil, nil)
	ctx := context.Background()

	err := storage.Save(ctx, nil)
	assert.Error(t, err)
}

func TestNATSAnomalyStorage_Save_EmptyID(t *testing.T) {
	storage := NewNATSAnomalyStorage(nil, nil)
	ctx := context.Background()

	anomaly := &StructuralAnomaly{
		Type:   AnomalySemanticStructuralGap,
		Status: StatusPending,
	}

	err := storage.Save(ctx, anomaly)
	assert.Error(t, err)
}

func TestNATSAnomalyStorage_Get_NotFound(t *testing.T) {
	storage := NewNATSAnomalyStorage(nil, nil)
	ctx := context.Background()

	got, err := storage.Get(ctx, "nonexistent")
	assert.NoError(t, err)
	assert.Nil(t, got)
}

func TestNATSAnomalyStorage_Get_EmptyID(t *testing.T) {
	storage := NewNATSAnomalyStorage(nil, nil)
	ctx := context.Background()

	_, err := storage.Get(ctx, "")
	assert.Error(t, err)
}

func TestNATSAnomalyStorage_GetByStatus(t *testing.T) {
	storage := NewNATSAnomalyStorage(nil, nil)
	ctx := context.Background()

	// Save anomalies with different statuses
	anomalies := []*StructuralAnomaly{
		{ID: "a1", Type: AnomalySemanticStructuralGap, Status: StatusPending, DetectedAt: time.Now()},
		{ID: "a2", Type: AnomalyCoreIsolation, Status: StatusPending, DetectedAt: time.Now()},
		{ID: "a3", Type: AnomalyCoreDemotion, Status: StatusApproved, DetectedAt: time.Now()},
	}

	for _, a := range anomalies {
		require.NoError(t, storage.Save(ctx, a))
	}

	// Get pending
	pending, err := storage.GetByStatus(ctx, StatusPending)
	assert.NoError(t, err)
	assert.Len(t, pending, 2)

	// Get approved
	approved, err := storage.GetByStatus(ctx, StatusApproved)
	assert.NoError(t, err)
	assert.Len(t, approved, 1)

	// Get rejected (none)
	rejected, err := storage.GetByStatus(ctx, StatusRejected)
	assert.NoError(t, err)
	assert.Len(t, rejected, 0)
}

func TestNATSAnomalyStorage_GetByType(t *testing.T) {
	storage := NewNATSAnomalyStorage(nil, nil)
	ctx := context.Background()

	anomalies := []*StructuralAnomaly{
		{ID: "a1", Type: AnomalySemanticStructuralGap, Status: StatusPending, DetectedAt: time.Now()},
		{ID: "a2", Type: AnomalySemanticStructuralGap, Status: StatusApproved, DetectedAt: time.Now()},
		{ID: "a3", Type: AnomalyCoreIsolation, Status: StatusPending, DetectedAt: time.Now()},
	}

	for _, a := range anomalies {
		require.NoError(t, storage.Save(ctx, a))
	}

	semantic, err := storage.GetByType(ctx, AnomalySemanticStructuralGap)
	assert.NoError(t, err)
	assert.Len(t, semantic, 2)

	core, err := storage.GetByType(ctx, AnomalyCoreIsolation)
	assert.NoError(t, err)
	assert.Len(t, core, 1)
}

func TestNATSAnomalyStorage_UpdateStatus(t *testing.T) {
	storage := NewNATSAnomalyStorage(nil, nil)
	ctx := context.Background()

	anomaly := &StructuralAnomaly{
		ID:         "update-test",
		Type:       AnomalySemanticStructuralGap,
		Status:     StatusPending,
		DetectedAt: time.Now(),
	}
	require.NoError(t, storage.Save(ctx, anomaly))

	// Update status
	err := storage.UpdateStatus(ctx, "update-test", StatusApproved, "user@test.com", "Looks good")
	assert.NoError(t, err)

	// Verify
	got, err := storage.Get(ctx, "update-test")
	assert.NoError(t, err)
	assert.Equal(t, StatusApproved, got.Status)
	assert.Equal(t, "user@test.com", got.ReviewedBy)
	assert.Equal(t, "Looks good", got.ReviewNotes)
	assert.NotNil(t, got.ReviewedAt)
}

func TestNATSAnomalyStorage_UpdateStatus_NotFound(t *testing.T) {
	storage := NewNATSAnomalyStorage(nil, nil)
	ctx := context.Background()

	err := storage.UpdateStatus(ctx, "nonexistent", StatusApproved, "", "")
	assert.Error(t, err)
}

func TestNATSAnomalyStorage_Delete(t *testing.T) {
	storage := NewNATSAnomalyStorage(nil, nil)
	ctx := context.Background()

	anomaly := &StructuralAnomaly{
		ID:         "delete-test",
		Type:       AnomalySemanticStructuralGap,
		Status:     StatusPending,
		DetectedAt: time.Now(),
	}
	require.NoError(t, storage.Save(ctx, anomaly))

	// Delete
	err := storage.Delete(ctx, "delete-test")
	assert.NoError(t, err)

	// Verify gone
	got, err := storage.Get(ctx, "delete-test")
	assert.NoError(t, err)
	assert.Nil(t, got)
}

func TestNATSAnomalyStorage_Delete_NotFound(t *testing.T) {
	storage := NewNATSAnomalyStorage(nil, nil)
	ctx := context.Background()

	// Delete nonexistent - should not error
	err := storage.Delete(ctx, "nonexistent")
	assert.NoError(t, err)
}

func TestNATSAnomalyStorage_Cleanup(t *testing.T) {
	storage := NewNATSAnomalyStorage(nil, nil)
	ctx := context.Background()

	now := time.Now()

	// Old resolved anomalies
	oldAnomalies := []*StructuralAnomaly{
		{ID: "old1", Type: AnomalySemanticStructuralGap, Status: StatusApplied, DetectedAt: now.Add(-48 * time.Hour)},
		{ID: "old2", Type: AnomalyCoreIsolation, Status: StatusRejected, DetectedAt: now.Add(-48 * time.Hour)},
	}

	// Recent anomalies (should not be cleaned)
	recentAnomalies := []*StructuralAnomaly{
		{ID: "recent1", Type: AnomalySemanticStructuralGap, Status: StatusPending, DetectedAt: now},
		{ID: "recent2", Type: AnomalyCoreIsolation, Status: StatusApplied, DetectedAt: now},
	}

	for _, a := range append(oldAnomalies, recentAnomalies...) {
		require.NoError(t, storage.Save(ctx, a))
	}

	// Cleanup anomalies older than 24 hours
	deleted, err := storage.Cleanup(ctx, 24*time.Hour)
	assert.NoError(t, err)
	assert.Equal(t, 2, deleted)

	// Verify old resolved are gone
	got, _ := storage.Get(ctx, "old1")
	assert.Nil(t, got)
	got, _ = storage.Get(ctx, "old2")
	assert.Nil(t, got)

	// Verify recent remain
	got, _ = storage.Get(ctx, "recent1")
	assert.NotNil(t, got)
	got, _ = storage.Get(ctx, "recent2")
	assert.NotNil(t, got)
}

func TestNATSAnomalyStorage_Count(t *testing.T) {
	storage := NewNATSAnomalyStorage(nil, nil)
	ctx := context.Background()

	anomalies := []*StructuralAnomaly{
		{ID: "a1", Type: AnomalySemanticStructuralGap, Status: StatusPending, DetectedAt: time.Now()},
		{ID: "a2", Type: AnomalyCoreIsolation, Status: StatusPending, DetectedAt: time.Now()},
		{ID: "a3", Type: AnomalyCoreDemotion, Status: StatusApproved, DetectedAt: time.Now()},
		{ID: "a4", Type: AnomalyTransitivityGap, Status: StatusRejected, DetectedAt: time.Now()},
	}

	for _, a := range anomalies {
		require.NoError(t, storage.Save(ctx, a))
	}

	counts, err := storage.Count(ctx)
	assert.NoError(t, err)
	assert.Equal(t, 2, counts[StatusPending])
	assert.Equal(t, 1, counts[StatusApproved])
	assert.Equal(t, 1, counts[StatusRejected])
}

func TestNATSAnomalyStorage_Watch_InMemory(t *testing.T) {
	storage := NewNATSAnomalyStorage(nil, nil)
	ctx := context.Background()

	// In-memory mode returns closed channel
	ch, err := storage.Watch(ctx)
	assert.NoError(t, err)

	// Channel should be closed
	_, ok := <-ch
	assert.False(t, ok)
}
