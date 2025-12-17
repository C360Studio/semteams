package inference

import (
	"context"
	"testing"
	"time"

	"github.com/c360/semstreams/processor/graph/structuralindex"
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
		StructuralIndices: &structuralindex.StructuralIndices{
			KCore: &structuralindex.KCoreIndex{
				CoreNumbers: map[string]int{"entity-a": 2, "entity-b": 3},
			},
			Pivot: &structuralindex.PivotIndex{
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
		StructuralIndices: &structuralindex.StructuralIndices{
			KCore: &structuralindex.KCoreIndex{CoreNumbers: map[string]int{}},
			Pivot: &structuralindex.PivotIndex{DistanceVectors: map[string][]int{}},
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
