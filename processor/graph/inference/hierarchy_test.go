package inference

import (
	"context"
	"errors"
	"sync"
	"testing"

	gtypes "github.com/c360/semstreams/graph"
	"github.com/c360/semstreams/message"
	"github.com/c360/semstreams/vocabulary"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// hierarchyMockTripleAdder records added triples for verification
type hierarchyMockTripleAdder struct {
	mu      sync.Mutex
	triples []message.Triple
	err     error
}

func (m *hierarchyMockTripleAdder) AddTriple(_ context.Context, triple message.Triple) error {
	if m.err != nil {
		return m.err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.triples = append(m.triples, triple)
	return nil
}

func (m *hierarchyMockTripleAdder) getTriples() []message.Triple {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]message.Triple, len(m.triples))
	copy(result, m.triples)
	return result
}

// mockEntityManager implements EntityManager for testing
type mockEntityManager struct {
	mu       sync.Mutex
	entities map[string]bool // entityID -> exists
	created  []*gtypes.EntityState
	err      error
}

func newMockEntityManager() *mockEntityManager {
	return &mockEntityManager{
		entities: make(map[string]bool),
		created:  make([]*gtypes.EntityState, 0),
	}
}

func (m *mockEntityManager) ExistsEntity(_ context.Context, id string) (bool, error) {
	if m.err != nil {
		return false, m.err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.entities[id], nil
}

func (m *mockEntityManager) CreateEntity(_ context.Context, entity *gtypes.EntityState) (*gtypes.EntityState, error) {
	if m.err != nil {
		return nil, m.err
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.entities[entity.ID] {
		return nil, errors.New("entity already exists")
	}

	m.entities[entity.ID] = true
	m.created = append(m.created, entity)
	return entity, nil
}

func (m *mockEntityManager) getCreatedEntities() []*gtypes.EntityState {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]*gtypes.EntityState, len(m.created))
	copy(result, m.created)
	return result
}

func (m *mockEntityManager) addExistingEntity(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entities[id] = true
}

func TestHierarchyInference_OnEntityCreated_Disabled(t *testing.T) {
	tripleAdder := &hierarchyMockTripleAdder{}
	entityManager := newMockEntityManager()

	config := HierarchyConfig{
		Enabled:         false, // Disabled
		CreateTypeEdges: true,
	}

	hi := NewHierarchyInference(entityManager, tripleAdder, nil, config, nil)

	err := hi.OnEntityCreated(context.Background(), "c360.logistics.sensor.document.temperature.sensor-001")
	require.NoError(t, err)

	// No triples should be added when disabled
	assert.Empty(t, tripleAdder.getTriples())
	assert.Empty(t, entityManager.getCreatedEntities())
}

func TestHierarchyInference_OnEntityCreated_InvalidEntityID(t *testing.T) {
	tripleAdder := &hierarchyMockTripleAdder{}
	entityManager := newMockEntityManager()

	config := HierarchyConfig{
		Enabled:         true,
		CreateTypeEdges: true,
	}

	hi := NewHierarchyInference(entityManager, tripleAdder, nil, config, nil)

	// 5-part entity ID should be skipped
	err := hi.OnEntityCreated(context.Background(), "c360.logistics.sensor.document.temperature")
	require.NoError(t, err)
	assert.Empty(t, tripleAdder.getTriples())

	// 7-part entity ID should be skipped
	err = hi.OnEntityCreated(context.Background(), "c360.logistics.sensor.document.temperature.zone.sensor")
	require.NoError(t, err)
	assert.Empty(t, tripleAdder.getTriples())
}

func TestHierarchyInference_OnEntityCreated_TypeEdgeOnly(t *testing.T) {
	tripleAdder := &hierarchyMockTripleAdder{}
	entityManager := newMockEntityManager()

	config := HierarchyConfig{
		Enabled:           true,
		CreateTypeEdges:   true,
		CreateSystemEdges: false,
		CreateDomainEdges: false,
	}

	hi := NewHierarchyInference(entityManager, tripleAdder, nil, config, nil)

	entityID := "c360.logistics.sensor.document.temperature.sensor-001"
	err := hi.OnEntityCreated(context.Background(), entityID)
	require.NoError(t, err)

	// Should create 1 container and 1 membership edge
	createdEntities := entityManager.getCreatedEntities()
	assert.Len(t, createdEntities, 1)
	assert.Equal(t, "c360.logistics.sensor.document.temperature.group", createdEntities[0].ID)

	triples := tripleAdder.getTriples()
	assert.Len(t, triples, 1)
	assert.Equal(t, entityID, triples[0].Subject)
	assert.Equal(t, vocabulary.HierarchyTypeMember, triples[0].Predicate)
	assert.Equal(t, "c360.logistics.sensor.document.temperature.group", triples[0].Object)
	assert.Equal(t, "inference.hierarchy", triples[0].Context)
	assert.Equal(t, 1.0, triples[0].Confidence)
}

func TestHierarchyInference_OnEntityCreated_AllLevels(t *testing.T) {
	tripleAdder := &hierarchyMockTripleAdder{}
	entityManager := newMockEntityManager()

	config := HierarchyConfig{
		Enabled:           true,
		CreateTypeEdges:   true,
		CreateSystemEdges: true,
		CreateDomainEdges: true,
	}

	hi := NewHierarchyInference(entityManager, tripleAdder, nil, config, nil)

	entityID := "c360.logistics.sensor.document.temperature.sensor-001"
	err := hi.OnEntityCreated(context.Background(), entityID)
	require.NoError(t, err)

	// Should create 3 containers
	createdEntities := entityManager.getCreatedEntities()
	assert.Len(t, createdEntities, 3)

	containerIDs := make(map[string]bool)
	for _, e := range createdEntities {
		containerIDs[e.ID] = true
	}
	assert.True(t, containerIDs["c360.logistics.sensor.document.temperature.group"]) // Type
	assert.True(t, containerIDs["c360.logistics.sensor.document.group.container"])   // System
	assert.True(t, containerIDs["c360.logistics.sensor.group.container.level"])      // Domain

	// Should create 3 membership edges
	triples := tripleAdder.getTriples()
	assert.Len(t, triples, 3)

	predicates := make(map[string]string) // predicate -> object
	for _, tr := range triples {
		assert.Equal(t, entityID, tr.Subject)
		predicates[tr.Predicate] = tr.Object.(string)
	}

	assert.Equal(t, "c360.logistics.sensor.document.temperature.group", predicates[vocabulary.HierarchyTypeMember])
	assert.Equal(t, "c360.logistics.sensor.document.group.container", predicates[vocabulary.HierarchySystemMember])
	assert.Equal(t, "c360.logistics.sensor.group.container.level", predicates[vocabulary.HierarchyDomainMember])
}

func TestHierarchyInference_ContainerReuse(t *testing.T) {
	tripleAdder := &hierarchyMockTripleAdder{}
	entityManager := newMockEntityManager()

	config := HierarchyConfig{
		Enabled:           true,
		CreateTypeEdges:   true,
		CreateSystemEdges: false,
		CreateDomainEdges: false,
	}

	hi := NewHierarchyInference(entityManager, tripleAdder, nil, config, nil)

	// Create first entity
	err := hi.OnEntityCreated(context.Background(), "c360.logistics.sensor.document.temperature.sensor-001")
	require.NoError(t, err)

	// Create second entity with same type prefix
	err = hi.OnEntityCreated(context.Background(), "c360.logistics.sensor.document.temperature.sensor-002")
	require.NoError(t, err)

	// Should only create 1 container (reused)
	createdEntities := entityManager.getCreatedEntities()
	assert.Len(t, createdEntities, 1)

	// But should have 2 membership edges
	triples := tripleAdder.getTriples()
	assert.Len(t, triples, 2)
}

func TestHierarchyInference_ContainerExistsInStorage(t *testing.T) {
	tripleAdder := &hierarchyMockTripleAdder{}
	entityManager := newMockEntityManager()

	// Pre-existing container in storage
	entityManager.addExistingEntity("c360.logistics.sensor.document.temperature.group")

	config := HierarchyConfig{
		Enabled:           true,
		CreateTypeEdges:   true,
		CreateSystemEdges: false,
		CreateDomainEdges: false,
	}

	hi := NewHierarchyInference(entityManager, tripleAdder, nil, config, nil)

	err := hi.OnEntityCreated(context.Background(), "c360.logistics.sensor.document.temperature.sensor-001")
	require.NoError(t, err)

	// Should NOT create container (already exists)
	createdEntities := entityManager.getCreatedEntities()
	assert.Empty(t, createdEntities)

	// But should still create membership edge
	triples := tripleAdder.getTriples()
	assert.Len(t, triples, 1)
}

func TestHierarchyInference_ContainerEntityProperties(t *testing.T) {
	tripleAdder := &hierarchyMockTripleAdder{}
	entityManager := newMockEntityManager()

	config := HierarchyConfig{
		Enabled:           true,
		CreateTypeEdges:   true,
		CreateSystemEdges: false,
		CreateDomainEdges: false,
	}

	hi := NewHierarchyInference(entityManager, tripleAdder, nil, config, nil)

	err := hi.OnEntityCreated(context.Background(), "c360.logistics.sensor.document.temperature.sensor-001")
	require.NoError(t, err)

	// Verify container entity has correct properties
	createdEntities := entityManager.getCreatedEntities()
	require.Len(t, createdEntities, 1)

	container := createdEntities[0]
	assert.Equal(t, "c360.logistics.sensor.document.temperature.group", container.ID)
	require.Len(t, container.Triples, 1)

	triple := container.Triples[0]
	assert.Equal(t, container.ID, triple.Subject)
	assert.Equal(t, "entity.type.class", triple.Predicate)
	assert.Equal(t, "hierarchy.container", triple.Object)
}

func TestHierarchyInference_ClearCache(t *testing.T) {
	tripleAdder := &hierarchyMockTripleAdder{}
	entityManager := newMockEntityManager()

	config := HierarchyConfig{
		Enabled:         true,
		CreateTypeEdges: true,
	}

	hi := NewHierarchyInference(entityManager, tripleAdder, nil, config, nil)

	// Create entity to populate cache
	err := hi.OnEntityCreated(context.Background(), "c360.logistics.sensor.document.temperature.sensor-001")
	require.NoError(t, err)

	assert.Equal(t, 1, hi.GetCacheStats())

	// Clear cache
	hi.ClearCache()

	assert.Equal(t, 0, hi.GetCacheStats())
}

func TestHierarchyInference_GetMetrics(t *testing.T) {
	tripleAdder := &hierarchyMockTripleAdder{}
	entityManager := newMockEntityManager()

	config := HierarchyConfig{
		Enabled:           true,
		CreateTypeEdges:   true,
		CreateSystemEdges: true,
		CreateDomainEdges: true,
	}

	hi := NewHierarchyInference(entityManager, tripleAdder, nil, config, nil)

	// Initial metrics
	containers, edges, failed := hi.GetMetrics()
	assert.Equal(t, int64(0), containers)
	assert.Equal(t, int64(0), edges)
	assert.Equal(t, int64(0), failed)

	// Create entity
	err := hi.OnEntityCreated(context.Background(), "c360.logistics.sensor.document.temperature.sensor-001")
	require.NoError(t, err)

	containers, edges, failed = hi.GetMetrics()
	assert.Equal(t, int64(3), containers) // 3 containers created
	assert.Equal(t, int64(3), edges)      // 3 edges created
	assert.Equal(t, int64(0), failed)
}

func TestDefaultHierarchyConfig(t *testing.T) {
	config := DefaultHierarchyConfig()

	assert.False(t, config.Enabled) // Opt-in
	assert.True(t, config.CreateTypeEdges)
	assert.True(t, config.CreateSystemEdges)
	assert.True(t, config.CreateDomainEdges)
}

func TestBuildContainerIDs(t *testing.T) {
	tripleAdder := &hierarchyMockTripleAdder{}
	entityManager := newMockEntityManager()

	hi := NewHierarchyInference(entityManager, tripleAdder, nil, DefaultHierarchyConfig(), nil)

	parts := []string{"org", "platform", "domain", "system", "type", "instance"}

	// Test type container ID
	typeID := hi.buildTypeContainerID(parts)
	assert.Equal(t, "org.platform.domain.system.type.group", typeID)

	// Test system container ID
	systemID := hi.buildSystemContainerID(parts)
	assert.Equal(t, "org.platform.domain.system.group.container", systemID)

	// Test domain container ID
	domainID := hi.buildDomainContainerID(parts)
	assert.Equal(t, "org.platform.domain.group.container.level", domainID)
}

func TestHierarchyInference_RaceConditionOnContainerCreate(t *testing.T) {
	tripleAdder := &hierarchyMockTripleAdder{}
	entityManager := newMockEntityManager()

	// Simulate race: container "exists" error during create
	entityManager.addExistingEntity("c360.logistics.sensor.document.temperature.group")

	config := HierarchyConfig{
		Enabled:           true,
		CreateTypeEdges:   true,
		CreateSystemEdges: false,
		CreateDomainEdges: false,
	}

	hi := NewHierarchyInference(entityManager, tripleAdder, nil, config, nil)

	// Even if container exists, edge should still be created
	err := hi.OnEntityCreated(context.Background(), "c360.logistics.sensor.document.temperature.sensor-001")
	require.NoError(t, err)

	triples := tripleAdder.getTriples()
	assert.Len(t, triples, 1)
}
