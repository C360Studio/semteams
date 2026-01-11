package inference

import (
	"context"
	"testing"
	"time"

	"github.com/c360/semstreams/graph/structural"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockStorage implements Storage for testing
type mockStorage struct {
	anomalies map[string]*StructuralAnomaly
}

func newMockStorage() *mockStorage {
	return &mockStorage{anomalies: make(map[string]*StructuralAnomaly)}
}

func (s *mockStorage) Save(_ context.Context, anomaly *StructuralAnomaly) error {
	s.anomalies[anomaly.ID] = anomaly
	return nil
}

func (s *mockStorage) Get(_ context.Context, id string) (*StructuralAnomaly, error) {
	return s.anomalies[id], nil
}

func (s *mockStorage) GetByStatus(_ context.Context, status AnomalyStatus) ([]*StructuralAnomaly, error) {
	var result []*StructuralAnomaly
	for _, a := range s.anomalies {
		if a.Status == status {
			result = append(result, a)
		}
	}
	return result, nil
}

func (s *mockStorage) GetByType(_ context.Context, t AnomalyType) ([]*StructuralAnomaly, error) {
	var result []*StructuralAnomaly
	for _, a := range s.anomalies {
		if a.Type == t {
			result = append(result, a)
		}
	}
	return result, nil
}

func (s *mockStorage) UpdateStatus(_ context.Context, id string, status AnomalyStatus, _, _ string) error {
	if a, ok := s.anomalies[id]; ok {
		a.Status = status
	}
	return nil
}

func (s *mockStorage) Delete(_ context.Context, id string) error {
	delete(s.anomalies, id)
	return nil
}

func (s *mockStorage) Watch(_ context.Context) (<-chan *StructuralAnomaly, error) {
	ch := make(chan *StructuralAnomaly)
	close(ch)
	return ch, nil
}

func (s *mockStorage) Cleanup(_ context.Context, _ time.Duration) (int, error) {
	return 0, nil
}

func (s *mockStorage) Count(_ context.Context) (map[AnomalyStatus]int, error) {
	counts := make(map[AnomalyStatus]int)
	for _, a := range s.anomalies {
		counts[a.Status]++
	}
	return counts, nil
}

func (s *mockStorage) SaveWithRevision(_ context.Context, anomaly *StructuralAnomaly, revision uint64) error {
	// For mock, revision > 0 checks if anomaly exists and is still pending
	if revision > 0 {
		existing, exists := s.anomalies[anomaly.ID]
		if !exists || existing.Status != StatusPending {
			return ErrConcurrentModification
		}
	}
	s.anomalies[anomaly.ID] = anomaly
	return nil
}

func (s *mockStorage) GetWithRevision(_ context.Context, id string) (*StructuralAnomaly, uint64, error) {
	anomaly, exists := s.anomalies[id]
	if !exists {
		return nil, 0, nil
	}
	return anomaly, 1, nil // Return revision 1 for test
}

func (s *mockStorage) IsDismissedPair(_ context.Context, _, _ string) (bool, error) {
	return false, nil
}

func (s *mockStorage) HasEntityAnomaly(_ context.Context, entityID string, anomalyType AnomalyType) (bool, error) {
	for _, a := range s.anomalies {
		if a.Type == anomalyType && a.EntityA == entityID {
			return true, nil
		}
	}
	return false, nil
}

// mockDetector implements Detector for testing
type mockDetector struct {
	name      string
	anomalies []*StructuralAnomaly
	err       error
	enabled   bool
}

func (d *mockDetector) Name() string {
	return d.name
}

func (d *mockDetector) Detect(_ context.Context) ([]*StructuralAnomaly, error) {
	if d.err != nil {
		return nil, d.err
	}
	return d.anomalies, nil
}

func (d *mockDetector) Configure(_ interface{}) error {
	return nil
}

func (d *mockDetector) SetDependencies(_ *DetectorDependencies) {
	// No-op for mock
}

func TestOrchestrator_NewOrchestrator(t *testing.T) {
	tests := []struct {
		name    string
		cfg     OrchestratorConfig
		wantErr bool
	}{
		{
			name: "valid config",
			cfg: OrchestratorConfig{
				Config:  DefaultConfig(),
				Storage: newMockStorage(),
			},
			wantErr: false,
		},
		{
			name: "missing storage",
			cfg: OrchestratorConfig{
				Config: DefaultConfig(),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orch, err := NewOrchestrator(tt.cfg)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, orch)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, orch)
			}
		})
	}
}

func TestOrchestrator_RegisterDetector(t *testing.T) {
	orch, err := NewOrchestrator(OrchestratorConfig{
		Config:  DefaultConfig(),
		Storage: newMockStorage(),
	})
	require.NoError(t, err)

	detector := &mockDetector{name: "test_detector"}
	orch.RegisterDetector(detector)

	registered := orch.GetRegisteredDetectors()
	assert.Contains(t, registered, "test_detector")
}

func TestOrchestrator_RunDetection_Disabled(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = false

	orch, err := NewOrchestrator(OrchestratorConfig{
		Config:  config,
		Storage: newMockStorage(),
	})
	require.NoError(t, err)

	result, err := orch.RunDetection(context.Background())
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Empty(t, result.Anomalies)
}

func TestOrchestrator_RunDetection_NoDependencies(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = true

	orch, err := NewOrchestrator(OrchestratorConfig{
		Config:  config,
		Storage: newMockStorage(),
	})
	require.NoError(t, err)

	// Don't set dependencies
	_, err = orch.RunDetection(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "dependencies not set")
}

func TestOrchestrator_RunDetection_WithDetectors(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = true
	config.SemanticGap.Enabled = true
	config.DetectionTimeout = 5 * time.Second

	storage := newMockStorage()
	orch, err := NewOrchestrator(OrchestratorConfig{
		Config:  config,
		Storage: storage,
	})
	require.NoError(t, err)

	// Create mock detector with anomalies
	anomaly := &StructuralAnomaly{
		ID:         "test-anomaly-1",
		Type:       AnomalySemanticStructuralGap,
		EntityA:    "entity-a",
		EntityB:    "entity-b",
		Confidence: 0.8,
		Status:     StatusPending,
	}
	detector := &mockDetector{
		name:      "semantic_gap",
		anomalies: []*StructuralAnomaly{anomaly},
		enabled:   true,
	}
	orch.RegisterDetector(detector)

	// Set minimal dependencies
	deps := &DetectorDependencies{
		StructuralIndices: &structural.StructuralIndices{
			KCore: &structural.KCoreIndex{
				CoreNumbers: map[string]int{"entity-a": 2, "entity-b": 3},
			},
			Pivot: &structural.PivotIndex{
				DistanceVectors: map[string][]int{"entity-a": {1, 2}, "entity-b": {2, 1}},
			},
		},
	}
	orch.SetDependencies(deps)

	result, err := orch.RunDetection(context.Background())
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Len(t, result.Anomalies, 1)
	assert.Equal(t, "test-anomaly-1", result.Anomalies[0].ID)

	// Verify anomaly was persisted
	_, exists := storage.anomalies["test-anomaly-1"]
	assert.True(t, exists)
}

func TestOrchestrator_RunDetection_MaxAnomaliesLimit(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = true
	config.MaxAnomaliesPerRun = 2
	config.SemanticGap.Enabled = true
	config.DetectionTimeout = 5 * time.Second

	storage := newMockStorage()
	orch, err := NewOrchestrator(OrchestratorConfig{
		Config:  config,
		Storage: storage,
	})
	require.NoError(t, err)

	// Create detector with more anomalies than limit
	anomalies := []*StructuralAnomaly{
		{ID: "a1", Type: AnomalySemanticStructuralGap, Status: StatusPending},
		{ID: "a2", Type: AnomalySemanticStructuralGap, Status: StatusPending},
		{ID: "a3", Type: AnomalySemanticStructuralGap, Status: StatusPending},
	}
	detector := &mockDetector{
		name:      "semantic_gap",
		anomalies: anomalies,
		enabled:   true,
	}
	orch.RegisterDetector(detector)

	deps := &DetectorDependencies{
		StructuralIndices: &structural.StructuralIndices{
			KCore: &structural.KCoreIndex{CoreNumbers: map[string]int{}},
			Pivot: &structural.PivotIndex{DistanceVectors: map[string][]int{}},
		},
	}
	orch.SetDependencies(deps)

	result, err := orch.RunDetection(context.Background())
	assert.NoError(t, err)
	assert.True(t, result.Truncated)
	assert.Len(t, result.Anomalies, 2)
}

func TestResult_Methods(t *testing.T) {
	now := time.Now()
	result := &Result{
		StartedAt:   now,
		CompletedAt: now.Add(time.Second),
		Anomalies: []*StructuralAnomaly{
			{Type: AnomalySemanticStructuralGap},
			{Type: AnomalySemanticStructuralGap},
			{Type: AnomalyCoreIsolation},
		},
	}

	assert.Equal(t, time.Second, result.Duration())
	assert.Equal(t, 3, result.AnomalyCount())

	byType := result.CountByType()
	assert.Equal(t, 2, byType[AnomalySemanticStructuralGap])
	assert.Equal(t, 1, byType[AnomalyCoreIsolation])
}

// mockRelationshipApplier implements RelationshipApplier for testing
type mockRelationshipApplier struct {
	applied []*RelationshipSuggestion
	err     error
}

func newMockApplier() *mockRelationshipApplier {
	return &mockRelationshipApplier{
		applied: make([]*RelationshipSuggestion, 0),
	}
}

func (m *mockRelationshipApplier) ApplyRelationship(_ context.Context, suggestion *RelationshipSuggestion) error {
	if m.err != nil {
		return m.err
	}
	m.applied = append(m.applied, suggestion)
	return nil
}

func (m *mockRelationshipApplier) AppliedCount() int {
	return len(m.applied)
}

func TestOrchestrator_ApplyVirtualEdges_HighConfidenceGaps(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = true
	config.VirtualEdges = VirtualEdgeConfig{
		AutoApply: AutoApplyConfig{
			Enabled:               true,
			MinSimilarity:         0.85,
			MinStructuralDistance: 4,
			PredicateTemplate:     "inferred.semantic.{band}",
		},
	}

	storage := newMockStorage()
	applier := newMockApplier()

	orch, err := NewOrchestrator(OrchestratorConfig{
		Config:  config,
		Storage: storage,
	})
	require.NoError(t, err)
	orch.SetApplier(applier)

	// Create anomalies that meet auto-apply threshold
	result := &Result{
		Anomalies: []*StructuralAnomaly{
			{
				ID:       "gap-1",
				Type:     AnomalySemanticStructuralGap,
				EntityA:  "entity-a",
				EntityB:  "entity-b",
				Evidence: Evidence{Similarity: 0.90, StructuralDistance: 5},
				Status:   StatusPending,
			},
		},
	}

	err = orch.applyVirtualEdges(context.Background(), config, result)
	assert.NoError(t, err)
	assert.Equal(t, 1, result.AutoApplied)
	assert.Equal(t, 0, result.QueuedForReview)
	assert.Equal(t, 1, applier.AppliedCount())

	// Verify the predicate was built correctly (0.90 = high)
	assert.Equal(t, "inferred.semantic.high", applier.applied[0].Predicate)
}

func TestOrchestrator_ApplyVirtualEdges_LowConfidenceGaps(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = true
	config.VirtualEdges = VirtualEdgeConfig{
		AutoApply: AutoApplyConfig{
			Enabled:               true,
			MinSimilarity:         0.85,
			MinStructuralDistance: 4,
			PredicateTemplate:     "inferred.semantic.{band}",
		},
	}

	storage := newMockStorage()
	applier := newMockApplier()

	orch, err := NewOrchestrator(OrchestratorConfig{
		Config:  config,
		Storage: storage,
	})
	require.NoError(t, err)
	orch.SetApplier(applier)

	// Create anomalies that DON'T meet auto-apply threshold
	result := &Result{
		Anomalies: []*StructuralAnomaly{
			{
				ID:       "gap-1",
				Type:     AnomalySemanticStructuralGap,
				EntityA:  "entity-a",
				EntityB:  "entity-b",
				Evidence: Evidence{Similarity: 0.75, StructuralDistance: 5}, // Below 0.85
				Status:   StatusPending,
			},
		},
	}

	err = orch.applyVirtualEdges(context.Background(), config, result)
	assert.NoError(t, err)
	assert.Equal(t, 0, result.AutoApplied)
	assert.Equal(t, 0, applier.AppliedCount())
}

func TestOrchestrator_ApplyVirtualEdges_StructuralDistanceTooLow(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = true
	config.VirtualEdges = VirtualEdgeConfig{
		AutoApply: AutoApplyConfig{
			Enabled:               true,
			MinSimilarity:         0.85,
			MinStructuralDistance: 4,
			PredicateTemplate:     "inferred.semantic.{band}",
		},
	}

	storage := newMockStorage()
	applier := newMockApplier()

	orch, err := NewOrchestrator(OrchestratorConfig{
		Config:  config,
		Storage: storage,
	})
	require.NoError(t, err)
	orch.SetApplier(applier)

	// Create anomaly with high similarity but low structural distance
	result := &Result{
		Anomalies: []*StructuralAnomaly{
			{
				ID:       "gap-1",
				Type:     AnomalySemanticStructuralGap,
				EntityA:  "entity-a",
				EntityB:  "entity-b",
				Evidence: Evidence{Similarity: 0.95, StructuralDistance: 3}, // Distance below 4
				Status:   StatusPending,
			},
		},
	}

	err = orch.applyVirtualEdges(context.Background(), config, result)
	assert.NoError(t, err)
	assert.Equal(t, 0, result.AutoApplied)
	assert.Equal(t, 0, applier.AppliedCount())
}

func TestOrchestrator_ApplyVirtualEdges_QueueForReview(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = true
	config.VirtualEdges = VirtualEdgeConfig{
		AutoApply: AutoApplyConfig{
			Enabled:               true,
			MinSimilarity:         0.85,
			MinStructuralDistance: 4,
			PredicateTemplate:     "inferred.semantic.{band}",
		},
		ReviewQueue: ReviewQueueConfig{
			Enabled:       true,
			MinSimilarity: 0.70,
			MaxSimilarity: 0.85,
		},
	}

	storage := newMockStorage()
	applier := newMockApplier()

	orch, err := NewOrchestrator(OrchestratorConfig{
		Config:  config,
		Storage: storage,
	})
	require.NoError(t, err)
	orch.SetApplier(applier)

	// Create anomaly that should go to review queue (similarity 0.78 is in range 0.70-0.85)
	result := &Result{
		Anomalies: []*StructuralAnomaly{
			{
				ID:       "gap-1",
				Type:     AnomalySemanticStructuralGap,
				EntityA:  "entity-a",
				EntityB:  "entity-b",
				Evidence: Evidence{Similarity: 0.78, StructuralDistance: 5},
				Status:   StatusPending,
			},
		},
	}

	err = orch.applyVirtualEdges(context.Background(), config, result)
	assert.NoError(t, err)
	assert.Equal(t, 0, result.AutoApplied)
	assert.Equal(t, 1, result.QueuedForReview)
	assert.Equal(t, 0, applier.AppliedCount())

	// Verify anomaly status was updated
	assert.Equal(t, StatusHumanReview, result.Anomalies[0].Status)
}

func TestOrchestrator_ApplyVirtualEdges_MixedAnomalies(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = true
	config.VirtualEdges = VirtualEdgeConfig{
		AutoApply: AutoApplyConfig{
			Enabled:               true,
			MinSimilarity:         0.85,
			MinStructuralDistance: 4,
			PredicateTemplate:     "inferred.semantic.{band}",
		},
		ReviewQueue: ReviewQueueConfig{
			Enabled:       true,
			MinSimilarity: 0.70,
			MaxSimilarity: 0.85,
		},
	}

	storage := newMockStorage()
	applier := newMockApplier()

	orch, err := NewOrchestrator(OrchestratorConfig{
		Config:  config,
		Storage: storage,
	})
	require.NoError(t, err)
	orch.SetApplier(applier)

	// Create mixed anomalies: 2 auto-apply, 1 queue, 1 skip, 1 non-semantic-gap
	result := &Result{
		Anomalies: []*StructuralAnomaly{
			{
				ID:       "gap-1",
				Type:     AnomalySemanticStructuralGap,
				EntityA:  "entity-a",
				EntityB:  "entity-b",
				Evidence: Evidence{Similarity: 0.92, StructuralDistance: 6}, // Auto-apply (high)
				Status:   StatusPending,
			},
			{
				ID:       "gap-2",
				Type:     AnomalySemanticStructuralGap,
				EntityA:  "entity-c",
				EntityB:  "entity-d",
				Evidence: Evidence{Similarity: 0.87, StructuralDistance: 5}, // Auto-apply (medium)
				Status:   StatusPending,
			},
			{
				ID:       "gap-3",
				Type:     AnomalySemanticStructuralGap,
				EntityA:  "entity-e",
				EntityB:  "entity-f",
				Evidence: Evidence{Similarity: 0.78, StructuralDistance: 5}, // Queue for review
				Status:   StatusPending,
			},
			{
				ID:       "gap-4",
				Type:     AnomalySemanticStructuralGap,
				EntityA:  "entity-g",
				EntityB:  "entity-h",
				Evidence: Evidence{Similarity: 0.60, StructuralDistance: 5}, // Skip (too low)
				Status:   StatusPending,
			},
			{
				ID:       "core-1",
				Type:     AnomalyCoreIsolation, // Non-semantic gap - should be skipped
				EntityA:  "entity-i",
				Evidence: Evidence{},
				Status:   StatusPending,
			},
		},
	}

	err = orch.applyVirtualEdges(context.Background(), config, result)
	assert.NoError(t, err)
	assert.Equal(t, 2, result.AutoApplied)
	assert.Equal(t, 1, result.QueuedForReview)
	assert.Equal(t, 2, applier.AppliedCount())

	// Verify predicates
	assert.Equal(t, "inferred.semantic.high", applier.applied[0].Predicate)
	assert.Equal(t, "inferred.semantic.medium", applier.applied[1].Predicate)
}

func TestOrchestrator_ApplyVirtualEdges_Disabled(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = true
	config.VirtualEdges = VirtualEdgeConfig{
		AutoApply: AutoApplyConfig{
			Enabled: false, // Disabled
		},
	}

	storage := newMockStorage()
	applier := newMockApplier()

	orch, err := NewOrchestrator(OrchestratorConfig{
		Config:  config,
		Storage: storage,
	})
	require.NoError(t, err)
	orch.SetApplier(applier)

	result := &Result{
		Anomalies: []*StructuralAnomaly{
			{
				ID:       "gap-1",
				Type:     AnomalySemanticStructuralGap,
				EntityA:  "entity-a",
				EntityB:  "entity-b",
				Evidence: Evidence{Similarity: 0.95, StructuralDistance: 10},
				Status:   StatusPending,
			},
		},
	}

	err = orch.applyVirtualEdges(context.Background(), config, result)
	assert.NoError(t, err)
	assert.Equal(t, 0, result.AutoApplied)
	assert.Equal(t, 0, applier.AppliedCount())
}

func TestOrchestrator_ApplyVirtualEdges_NoApplier(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = true
	config.VirtualEdges = VirtualEdgeConfig{
		AutoApply: AutoApplyConfig{
			Enabled:               true,
			MinSimilarity:         0.85,
			MinStructuralDistance: 4,
			PredicateTemplate:     "inferred.semantic.{band}",
		},
	}

	storage := newMockStorage()

	orch, err := NewOrchestrator(OrchestratorConfig{
		Config:  config,
		Storage: storage,
	})
	require.NoError(t, err)
	// Don't set applier

	result := &Result{
		Anomalies: []*StructuralAnomaly{
			{
				ID:       "gap-1",
				Type:     AnomalySemanticStructuralGap,
				EntityA:  "entity-a",
				EntityB:  "entity-b",
				Evidence: Evidence{Similarity: 0.95, StructuralDistance: 10},
				Status:   StatusPending,
			},
		},
	}

	err = orch.applyVirtualEdges(context.Background(), config, result)
	assert.NoError(t, err)
	assert.Equal(t, 0, result.AutoApplied)
}
