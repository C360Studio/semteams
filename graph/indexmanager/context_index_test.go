package indexmanager

import (
	"context"
	"testing"
	"time"

	gtypes "github.com/c360/semstreams/graph"
	"github.com/c360/semstreams/message"
)

// TestContextEntry verifies the ContextEntry struct structure and field access
func TestContextEntry(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		entry         ContextEntry
		wantEntityID  string
		wantPredicate string
	}{
		{
			name: "hierarchy inference context",
			entry: ContextEntry{
				EntityID:  "acme.iot.sensors.hvac.temperature.001",
				Predicate: "hierarchy.type.member",
			},
			wantEntityID:  "acme.iot.sensors.hvac.temperature.001",
			wantPredicate: "hierarchy.type.member",
		},
		{
			name: "structural inference context",
			entry: ContextEntry{
				EntityID:  "acme.telemetry.robotics.gcs1.drone.001",
				Predicate: "anomaly.correlation.strong",
			},
			wantEntityID:  "acme.telemetry.robotics.gcs1.drone.001",
			wantPredicate: "anomaly.correlation.strong",
		},
		{
			name: "community context",
			entry: ContextEntry{
				EntityID:  "acme.drones.fleet.alpha.uav.001",
				Predicate: "community.member",
			},
			wantEntityID:  "acme.drones.fleet.alpha.uav.001",
			wantPredicate: "community.member",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.entry.EntityID != tt.wantEntityID {
				t.Errorf("EntityID = %v, want %v", tt.entry.EntityID, tt.wantEntityID)
			}
			if tt.entry.Predicate != tt.wantPredicate {
				t.Errorf("Predicate = %v, want %v", tt.entry.Predicate, tt.wantPredicate)
			}
		})
	}
}

// TestContextIndex_HandleCreate verifies index creation for entities with context values
func TestContextIndex_HandleCreate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		entityID     string
		triples      []message.Triple
		wantContexts map[string]int // context value -> expected entry count
		wantErr      bool
	}{
		{
			name:     "entity with single context triple",
			entityID: "acme.iot.sensors.hvac.temperature.001",
			triples: []message.Triple{
				{
					Subject:   "acme.iot.sensors.hvac.temperature.001",
					Predicate: "hierarchy.type.member",
					Object:    "acme.iot.sensors.hvac.temperature.group",
					Context:   "inference.hierarchy",
					Source:    "test",
					Timestamp: time.Now(),
				},
			},
			wantContexts: map[string]int{
				"inference.hierarchy": 1,
			},
			wantErr: false,
		},
		{
			name:     "entity with no context (empty context field)",
			entityID: "acme.iot.sensors.hvac.temperature.001",
			triples: []message.Triple{
				{
					Subject:   "acme.iot.sensors.hvac.temperature.001",
					Predicate: "sensor.reading.value",
					Object:    25.5,
					Context:   "", // No context
					Source:    "test",
					Timestamp: time.Now(),
				},
			},
			wantContexts: map[string]int{}, // No contexts expected
			wantErr:      false,
		},
		{
			name:     "entity with multiple triples same context",
			entityID: "acme.iot.sensors.hvac.temperature.001",
			triples: []message.Triple{
				{
					Subject:   "acme.iot.sensors.hvac.temperature.001",
					Predicate: "hierarchy.type.member",
					Object:    "acme.iot.sensors.hvac.temperature.group",
					Context:   "inference.hierarchy",
					Source:    "test",
					Timestamp: time.Now(),
				},
				{
					Subject:   "acme.iot.sensors.hvac.temperature.001",
					Predicate: "hierarchy.type.sibling",
					Object:    "acme.iot.sensors.hvac.humidity.001",
					Context:   "inference.hierarchy",
					Source:    "test",
					Timestamp: time.Now(),
				},
			},
			wantContexts: map[string]int{
				"inference.hierarchy": 2, // Two entries for same context
			},
			wantErr: false,
		},
		{
			name:     "entity with multiple different contexts",
			entityID: "acme.iot.sensors.hvac.temperature.001",
			triples: []message.Triple{
				{
					Subject:   "acme.iot.sensors.hvac.temperature.001",
					Predicate: "hierarchy.type.member",
					Object:    "acme.iot.sensors.hvac.temperature.group",
					Context:   "inference.hierarchy",
					Source:    "test",
					Timestamp: time.Now(),
				},
				{
					Subject:   "acme.iot.sensors.hvac.temperature.001",
					Predicate: "anomaly.correlation.strong",
					Object:    "acme.iot.sensors.hvac.humidity.001",
					Context:   "inference.structural",
					Source:    "test",
					Timestamp: time.Now(),
				},
			},
			wantContexts: map[string]int{
				"inference.hierarchy":  1,
				"inference.structural": 1,
			},
			wantErr: false,
		},
		{
			name:     "entity with mixed context and no-context triples",
			entityID: "acme.iot.sensors.hvac.temperature.001",
			triples: []message.Triple{
				{
					Subject:   "acme.iot.sensors.hvac.temperature.001",
					Predicate: "sensor.reading.value",
					Object:    25.5,
					Context:   "", // No context
					Source:    "test",
					Timestamp: time.Now(),
				},
				{
					Subject:   "acme.iot.sensors.hvac.temperature.001",
					Predicate: "hierarchy.type.member",
					Object:    "acme.iot.sensors.hvac.temperature.group",
					Context:   "inference.hierarchy",
					Source:    "test",
					Timestamp: time.Now(),
				},
			},
			wantContexts: map[string]int{
				"inference.hierarchy": 1, // Only the triple with context
			},
			wantErr: false,
		},
		{
			name:         "entity with no triples",
			entityID:     "acme.iot.sensors.hvac.temperature.001",
			triples:      []message.Triple{},
			wantContexts: map[string]int{},
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			// Create mock bucket
			mockBucket := NewMockKeyValue()

			// Create index with mock
			index := NewContextIndex(mockBucket, nil, nil, nil)

			// Create entity state
			entityState := &gtypes.EntityState{
				ID:        tt.entityID,
				Triples:   tt.triples,
				Version:   1,
				UpdatedAt: time.Now(),
			}

			// Handle create operation
			err := index.HandleCreate(ctx, tt.entityID, entityState)

			if (err != nil) != tt.wantErr {
				t.Errorf("HandleCreate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				// Verify entries for each expected context
				for contextValue, wantCount := range tt.wantContexts {
					entries, err := index.GetEntriesByContext(ctx, contextValue)
					if err != nil {
						t.Errorf("GetEntriesByContext(%s) error = %v", contextValue, err)
						continue
					}
					if len(entries) != wantCount {
						t.Errorf("GetEntriesByContext(%s) returned %d entries, want %d",
							contextValue, len(entries), wantCount)
					}
				}
			}
		})
	}
}

// TestContextIndex_HandleUpdate verifies index updates with entity changes
func TestContextIndex_HandleUpdate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		entityID       string
		initialTriples []message.Triple
		updatedTriples []message.Triple
		wantContexts   map[string]int // context value -> expected entry count after update
	}{
		{
			name:     "add new context triple",
			entityID: "acme.iot.sensors.hvac.temperature.001",
			initialTriples: []message.Triple{
				{
					Subject:   "acme.iot.sensors.hvac.temperature.001",
					Predicate: "hierarchy.type.member",
					Object:    "acme.iot.sensors.hvac.temperature.group",
					Context:   "inference.hierarchy",
					Source:    "test",
					Timestamp: time.Now(),
				},
			},
			updatedTriples: []message.Triple{
				{
					Subject:   "acme.iot.sensors.hvac.temperature.001",
					Predicate: "hierarchy.type.member",
					Object:    "acme.iot.sensors.hvac.temperature.group",
					Context:   "inference.hierarchy",
					Source:    "test",
					Timestamp: time.Now(),
				},
				{
					Subject:   "acme.iot.sensors.hvac.temperature.001",
					Predicate: "anomaly.correlation.strong",
					Object:    "acme.iot.sensors.hvac.humidity.001",
					Context:   "inference.structural",
					Source:    "test",
					Timestamp: time.Now(),
				},
			},
			wantContexts: map[string]int{
				"inference.hierarchy":  1,
				"inference.structural": 1,
			},
		},
		{
			name:     "remove context triple",
			entityID: "acme.iot.sensors.hvac.temperature.001",
			initialTriples: []message.Triple{
				{
					Subject:   "acme.iot.sensors.hvac.temperature.001",
					Predicate: "hierarchy.type.member",
					Object:    "acme.iot.sensors.hvac.temperature.group",
					Context:   "inference.hierarchy",
					Source:    "test",
					Timestamp: time.Now(),
				},
				{
					Subject:   "acme.iot.sensors.hvac.temperature.001",
					Predicate: "anomaly.correlation.strong",
					Object:    "acme.iot.sensors.hvac.humidity.001",
					Context:   "inference.structural",
					Source:    "test",
					Timestamp: time.Now(),
				},
			},
			updatedTriples: []message.Triple{
				{
					Subject:   "acme.iot.sensors.hvac.temperature.001",
					Predicate: "hierarchy.type.member",
					Object:    "acme.iot.sensors.hvac.temperature.group",
					Context:   "inference.hierarchy",
					Source:    "test",
					Timestamp: time.Now(),
				},
			},
			wantContexts: map[string]int{
				"inference.hierarchy": 1,
				// inference.structural should have 0 entries for this entity
			},
		},
		{
			name:     "change predicate within same context",
			entityID: "acme.iot.sensors.hvac.temperature.001",
			initialTriples: []message.Triple{
				{
					Subject:   "acme.iot.sensors.hvac.temperature.001",
					Predicate: "hierarchy.type.member",
					Object:    "acme.iot.sensors.hvac.temperature.group",
					Context:   "inference.hierarchy",
					Source:    "test",
					Timestamp: time.Now(),
				},
			},
			updatedTriples: []message.Triple{
				{
					Subject:   "acme.iot.sensors.hvac.temperature.001",
					Predicate: "hierarchy.type.sibling",
					Object:    "acme.iot.sensors.hvac.humidity.001",
					Context:   "inference.hierarchy",
					Source:    "test",
					Timestamp: time.Now(),
				},
			},
			wantContexts: map[string]int{
				"inference.hierarchy": 1, // Same count, different predicate
			},
		},
		{
			name:     "remove all context triples",
			entityID: "acme.iot.sensors.hvac.temperature.001",
			initialTriples: []message.Triple{
				{
					Subject:   "acme.iot.sensors.hvac.temperature.001",
					Predicate: "hierarchy.type.member",
					Object:    "acme.iot.sensors.hvac.temperature.group",
					Context:   "inference.hierarchy",
					Source:    "test",
					Timestamp: time.Now(),
				},
			},
			updatedTriples: []message.Triple{
				{
					Subject:   "acme.iot.sensors.hvac.temperature.001",
					Predicate: "sensor.reading.value",
					Object:    25.5,
					Context:   "", // No context
					Source:    "test",
					Timestamp: time.Now(),
				},
			},
			wantContexts: map[string]int{}, // All contexts should be empty for this entity
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			// Create mock bucket
			mockBucket := NewMockKeyValue()

			// Create index
			index := NewContextIndex(mockBucket, nil, nil, nil)

			// Setup initial state
			initialState := &gtypes.EntityState{
				ID:        tt.entityID,
				Triples:   tt.initialTriples,
				Version:   1,
				UpdatedAt: time.Now(),
			}

			err := index.HandleCreate(ctx, tt.entityID, initialState)
			if err != nil {
				t.Fatalf("HandleCreate() error = %v", err)
			}

			// Perform update
			updatedState := &gtypes.EntityState{
				ID:        tt.entityID,
				Triples:   tt.updatedTriples,
				Version:   2,
				UpdatedAt: time.Now(),
			}

			err = index.HandleUpdate(ctx, tt.entityID, updatedState)
			if err != nil {
				t.Errorf("HandleUpdate() error = %v", err)
				return
			}

			// Verify entries for each expected context
			for contextValue, wantCount := range tt.wantContexts {
				entries, err := index.GetEntriesByContext(ctx, contextValue)
				if err != nil {
					t.Errorf("GetEntriesByContext(%s) error = %v", contextValue, err)
					continue
				}
				// Count entries for this entity
				entityCount := 0
				for _, e := range entries {
					if e.EntityID == tt.entityID {
						entityCount++
					}
				}
				if entityCount != wantCount {
					t.Errorf("GetEntriesByContext(%s) has %d entries for entity, want %d",
						contextValue, entityCount, wantCount)
				}
			}
		})
	}
}

// TestContextIndex_HandleDelete verifies removal of entity from all contexts
func TestContextIndex_HandleDelete(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		entityID       string
		initialTriples []message.Triple
		wantErr        bool
	}{
		{
			name:     "delete entity with context triples",
			entityID: "acme.iot.sensors.hvac.temperature.001",
			initialTriples: []message.Triple{
				{
					Subject:   "acme.iot.sensors.hvac.temperature.001",
					Predicate: "hierarchy.type.member",
					Object:    "acme.iot.sensors.hvac.temperature.group",
					Context:   "inference.hierarchy",
					Source:    "test",
					Timestamp: time.Now(),
				},
				{
					Subject:   "acme.iot.sensors.hvac.temperature.001",
					Predicate: "anomaly.correlation.strong",
					Object:    "acme.iot.sensors.hvac.humidity.001",
					Context:   "inference.structural",
					Source:    "test",
					Timestamp: time.Now(),
				},
			},
			wantErr: false,
		},
		{
			name:           "delete entity with no context triples",
			entityID:       "acme.iot.sensors.hvac.temperature.002",
			initialTriples: []message.Triple{},
			wantErr:        false,
		},
		{
			name:           "delete nonexistent entity (idempotent)",
			entityID:       "acme.iot.sensors.hvac.temperature.999",
			initialTriples: nil, // Don't create initial state
			wantErr:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			// Create mock bucket
			mockBucket := NewMockKeyValue()

			// Create index
			index := NewContextIndex(mockBucket, nil, nil, nil)

			// Collect context values for verification
			contextValues := make(map[string]bool)

			// Setup initial state if provided
			if tt.initialTriples != nil {
				initialState := &gtypes.EntityState{
					ID:        tt.entityID,
					Triples:   tt.initialTriples,
					Version:   1,
					UpdatedAt: time.Now(),
				}

				for _, triple := range tt.initialTriples {
					if triple.Context != "" {
						contextValues[triple.Context] = true
					}
				}

				err := index.HandleCreate(ctx, tt.entityID, initialState)
				if err != nil {
					t.Fatalf("HandleCreate() error = %v", err)
				}
			}

			// Perform delete
			err := index.HandleDelete(ctx, tt.entityID)
			if (err != nil) != tt.wantErr {
				t.Errorf("HandleDelete() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Verify entity is no longer in any context
			for contextValue := range contextValues {
				entries, err := index.GetEntriesByContext(ctx, contextValue)
				if err != nil {
					t.Errorf("GetEntriesByContext(%s) error = %v", contextValue, err)
					continue
				}
				for _, e := range entries {
					if e.EntityID == tt.entityID {
						t.Errorf("Entity %s should not be in context %s after delete",
							tt.entityID, contextValue)
					}
				}
			}
		})
	}
}

// TestContextIndex_GetEntriesByContext verifies querying entries by context value
func TestContextIndex_GetEntriesByContext(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		setupData []struct {
			entityID string
			triples  []message.Triple
		}
		queryContext string
		wantCount    int
		wantErr      bool
		checkEntries func(t *testing.T, entries []ContextEntry)
	}{
		{
			name: "single entity single context",
			setupData: []struct {
				entityID string
				triples  []message.Triple
			}{
				{
					entityID: "acme.iot.sensors.hvac.temperature.001",
					triples: []message.Triple{
						{
							Subject:   "acme.iot.sensors.hvac.temperature.001",
							Predicate: "hierarchy.type.member",
							Object:    "acme.iot.sensors.hvac.temperature.group",
							Context:   "inference.hierarchy",
							Source:    "test",
							Timestamp: time.Now(),
						},
					},
				},
			},
			queryContext: "inference.hierarchy",
			wantCount:    1,
			wantErr:      false,
			checkEntries: func(t *testing.T, entries []ContextEntry) {
				if entries[0].EntityID != "acme.iot.sensors.hvac.temperature.001" {
					t.Errorf("Expected entity ID acme.iot.sensors.hvac.temperature.001, got %s",
						entries[0].EntityID)
				}
				if entries[0].Predicate != "hierarchy.type.member" {
					t.Errorf("Expected predicate hierarchy.type.member, got %s",
						entries[0].Predicate)
				}
			},
		},
		{
			name: "multiple entities same context",
			setupData: []struct {
				entityID string
				triples  []message.Triple
			}{
				{
					entityID: "acme.iot.sensors.hvac.temperature.001",
					triples: []message.Triple{
						{
							Subject:   "acme.iot.sensors.hvac.temperature.001",
							Predicate: "hierarchy.type.member",
							Object:    "acme.iot.sensors.hvac.temperature.group",
							Context:   "inference.hierarchy",
							Source:    "test",
							Timestamp: time.Now(),
						},
					},
				},
				{
					entityID: "acme.iot.sensors.hvac.humidity.001",
					triples: []message.Triple{
						{
							Subject:   "acme.iot.sensors.hvac.humidity.001",
							Predicate: "hierarchy.type.member",
							Object:    "acme.iot.sensors.hvac.humidity.group",
							Context:   "inference.hierarchy",
							Source:    "test",
							Timestamp: time.Now(),
						},
					},
				},
			},
			queryContext: "inference.hierarchy",
			wantCount:    2,
			wantErr:      false,
			checkEntries: func(t *testing.T, entries []ContextEntry) {
				entityIDs := make(map[string]bool)
				for _, e := range entries {
					entityIDs[e.EntityID] = true
				}
				if !entityIDs["acme.iot.sensors.hvac.temperature.001"] {
					t.Error("Expected temperature sensor in results")
				}
				if !entityIDs["acme.iot.sensors.hvac.humidity.001"] {
					t.Error("Expected humidity sensor in results")
				}
			},
		},
		{
			name:         "context not found",
			setupData:    nil,
			queryContext: "nonexistent.context",
			wantCount:    0,
			wantErr:      false,
		},
		{
			name: "entity with multiple predicates same context",
			setupData: []struct {
				entityID string
				triples  []message.Triple
			}{
				{
					entityID: "acme.iot.sensors.hvac.temperature.001",
					triples: []message.Triple{
						{
							Subject:   "acme.iot.sensors.hvac.temperature.001",
							Predicate: "hierarchy.type.member",
							Object:    "acme.iot.sensors.hvac.temperature.group",
							Context:   "inference.hierarchy",
							Source:    "test",
							Timestamp: time.Now(),
						},
						{
							Subject:   "acme.iot.sensors.hvac.temperature.001",
							Predicate: "hierarchy.type.sibling",
							Object:    "acme.iot.sensors.hvac.humidity.001",
							Context:   "inference.hierarchy",
							Source:    "test",
							Timestamp: time.Now(),
						},
					},
				},
			},
			queryContext: "inference.hierarchy",
			wantCount:    2, // Two entries for same entity, different predicates
			wantErr:      false,
			checkEntries: func(t *testing.T, entries []ContextEntry) {
				predicates := make(map[string]bool)
				for _, e := range entries {
					predicates[e.Predicate] = true
				}
				if !predicates["hierarchy.type.member"] {
					t.Error("Expected hierarchy.type.member predicate")
				}
				if !predicates["hierarchy.type.sibling"] {
					t.Error("Expected hierarchy.type.sibling predicate")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			// Create mock bucket
			mockBucket := NewMockKeyValue()

			// Create index
			index := NewContextIndex(mockBucket, nil, nil, nil)

			// Setup test data
			for _, data := range tt.setupData {
				entityState := &gtypes.EntityState{
					ID:        data.entityID,
					Triples:   data.triples,
					Version:   1,
					UpdatedAt: time.Now(),
				}

				err := index.HandleCreate(ctx, data.entityID, entityState)
				if err != nil {
					t.Fatalf("HandleCreate() error = %v", err)
				}
			}

			// Query entries by context
			entries, err := index.GetEntriesByContext(ctx, tt.queryContext)

			if (err != nil) != tt.wantErr {
				t.Errorf("GetEntriesByContext() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if len(entries) != tt.wantCount {
					t.Errorf("GetEntriesByContext() returned %d entries, want %d",
						len(entries), tt.wantCount)
				}

				if tt.checkEntries != nil && len(entries) > 0 {
					tt.checkEntries(t, entries)
				}
			}
		})
	}
}

// TestContextIndex_GetEntityIDsByContext verifies deduplication of entity IDs
func TestContextIndex_GetEntityIDsByContext(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		setupData []struct {
			entityID string
			triples  []message.Triple
		}
		queryContext string
		wantCount    int
		wantErr      bool
	}{
		{
			name: "entity with multiple predicates returns once",
			setupData: []struct {
				entityID string
				triples  []message.Triple
			}{
				{
					entityID: "acme.iot.sensors.hvac.temperature.001",
					triples: []message.Triple{
						{
							Subject:   "acme.iot.sensors.hvac.temperature.001",
							Predicate: "hierarchy.type.member",
							Object:    "acme.iot.sensors.hvac.temperature.group",
							Context:   "inference.hierarchy",
							Source:    "test",
							Timestamp: time.Now(),
						},
						{
							Subject:   "acme.iot.sensors.hvac.temperature.001",
							Predicate: "hierarchy.type.sibling",
							Object:    "acme.iot.sensors.hvac.humidity.001",
							Context:   "inference.hierarchy",
							Source:    "test",
							Timestamp: time.Now(),
						},
					},
				},
			},
			queryContext: "inference.hierarchy",
			wantCount:    1, // Deduplicated to one entity ID
			wantErr:      false,
		},
		{
			name: "multiple entities returned correctly",
			setupData: []struct {
				entityID string
				triples  []message.Triple
			}{
				{
					entityID: "acme.iot.sensors.hvac.temperature.001",
					triples: []message.Triple{
						{
							Subject:   "acme.iot.sensors.hvac.temperature.001",
							Predicate: "hierarchy.type.member",
							Object:    "acme.iot.sensors.hvac.temperature.group",
							Context:   "inference.hierarchy",
							Source:    "test",
							Timestamp: time.Now(),
						},
					},
				},
				{
					entityID: "acme.iot.sensors.hvac.humidity.001",
					triples: []message.Triple{
						{
							Subject:   "acme.iot.sensors.hvac.humidity.001",
							Predicate: "hierarchy.type.member",
							Object:    "acme.iot.sensors.hvac.humidity.group",
							Context:   "inference.hierarchy",
							Source:    "test",
							Timestamp: time.Now(),
						},
					},
				},
			},
			queryContext: "inference.hierarchy",
			wantCount:    2,
			wantErr:      false,
		},
		{
			name:         "empty context returns empty slice",
			setupData:    nil,
			queryContext: "nonexistent.context",
			wantCount:    0,
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			// Create mock bucket
			mockBucket := NewMockKeyValue()

			// Create index
			index := NewContextIndex(mockBucket, nil, nil, nil)

			// Setup test data
			for _, data := range tt.setupData {
				entityState := &gtypes.EntityState{
					ID:        data.entityID,
					Triples:   data.triples,
					Version:   1,
					UpdatedAt: time.Now(),
				}

				err := index.HandleCreate(ctx, data.entityID, entityState)
				if err != nil {
					t.Fatalf("HandleCreate() error = %v", err)
				}
			}

			// Query entity IDs by context
			entityIDs, err := index.GetEntityIDsByContext(ctx, tt.queryContext)

			if (err != nil) != tt.wantErr {
				t.Errorf("GetEntityIDsByContext() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if len(entityIDs) != tt.wantCount {
					t.Errorf("GetEntityIDsByContext() returned %d IDs, want %d",
						len(entityIDs), tt.wantCount)
				}
			}
		})
	}
}

// TestContextIndex_GetAllContexts verifies listing all indexed context values
func TestContextIndex_GetAllContexts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		setupData []struct {
			entityID string
			triples  []message.Triple
		}
		wantContexts []string
		wantErr      bool
	}{
		{
			name: "multiple contexts from multiple entities",
			setupData: []struct {
				entityID string
				triples  []message.Triple
			}{
				{
					entityID: "acme.iot.sensors.hvac.temperature.001",
					triples: []message.Triple{
						{
							Subject:   "acme.iot.sensors.hvac.temperature.001",
							Predicate: "hierarchy.type.member",
							Object:    "acme.iot.sensors.hvac.temperature.group",
							Context:   "inference.hierarchy",
							Source:    "test",
							Timestamp: time.Now(),
						},
					},
				},
				{
					entityID: "acme.iot.sensors.hvac.humidity.001",
					triples: []message.Triple{
						{
							Subject:   "acme.iot.sensors.hvac.humidity.001",
							Predicate: "anomaly.correlation.strong",
							Object:    "acme.iot.sensors.hvac.temperature.001",
							Context:   "inference.structural",
							Source:    "test",
							Timestamp: time.Now(),
						},
					},
				},
			},
			wantContexts: []string{"inference.hierarchy", "inference.structural"},
			wantErr:      false,
		},
		{
			name:         "no contexts indexed",
			setupData:    nil,
			wantContexts: []string{},
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			// Create mock bucket
			mockBucket := NewMockKeyValue()

			// Create index
			index := NewContextIndex(mockBucket, nil, nil, nil)

			// Setup test data
			for _, data := range tt.setupData {
				entityState := &gtypes.EntityState{
					ID:        data.entityID,
					Triples:   data.triples,
					Version:   1,
					UpdatedAt: time.Now(),
				}

				err := index.HandleCreate(ctx, data.entityID, entityState)
				if err != nil {
					t.Fatalf("HandleCreate() error = %v", err)
				}
			}

			// Query all contexts
			contexts, err := index.GetAllContexts(ctx)

			if (err != nil) != tt.wantErr {
				t.Errorf("GetAllContexts() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if len(contexts) != len(tt.wantContexts) {
					t.Errorf("GetAllContexts() returned %d contexts, want %d",
						len(contexts), len(tt.wantContexts))
				}

				// Verify expected contexts are present
				contextMap := make(map[string]bool)
				for _, c := range contexts {
					contextMap[c] = true
				}
				for _, want := range tt.wantContexts {
					if !contextMap[want] {
						t.Errorf("Expected context %s not found in results", want)
					}
				}
			}
		})
	}
}

// TestContextIndex_ContextCancellation verifies proper context cancellation handling
func TestContextIndex_ContextCancellation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		operation string
	}{
		{"HandleCreate cancelled", "create"},
		{"HandleUpdate cancelled", "update"},
		{"HandleDelete cancelled", "delete"},
		{"GetEntriesByContext cancelled", "get_entries"},
		{"GetEntityIDsByContext cancelled", "get_ids"},
		{"GetAllContexts cancelled", "get_all"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create cancelled context
			ctx, cancel := context.WithCancel(context.Background())
			cancel() // Cancel immediately

			// Create mock bucket
			mockBucket := NewMockKeyValue()

			// Create index
			index := NewContextIndex(mockBucket, nil, nil, nil)

			entityState := &gtypes.EntityState{
				ID: "acme.iot.sensors.hvac.temperature.001",
				Triples: []message.Triple{
					{
						Subject:   "acme.iot.sensors.hvac.temperature.001",
						Predicate: "hierarchy.type.member",
						Object:    "acme.iot.sensors.hvac.temperature.group",
						Context:   "inference.hierarchy",
						Source:    "test",
						Timestamp: time.Now(),
					},
				},
				Version:   1,
				UpdatedAt: time.Now(),
			}

			var err error
			switch tt.operation {
			case "create":
				err = index.HandleCreate(ctx, entityState.ID, entityState)
			case "update":
				err = index.HandleUpdate(ctx, entityState.ID, entityState)
			case "delete":
				err = index.HandleDelete(ctx, entityState.ID)
			case "get_entries":
				_, err = index.GetEntriesByContext(ctx, "inference.hierarchy")
			case "get_ids":
				_, err = index.GetEntityIDsByContext(ctx, "inference.hierarchy")
			case "get_all":
				_, err = index.GetAllContexts(ctx)
			}

			if err == nil {
				t.Errorf("Expected context.Canceled error for %s, got nil", tt.operation)
			}
		})
	}
}

// TestContextIndex_SanitizesContextKeys verifies that context values are sanitized for NATS keys
func TestContextIndex_SanitizesContextKeys(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		contextValue string
		shouldWork   bool
	}{
		{
			name:         "normal dotted context",
			contextValue: "inference.hierarchy",
			shouldWork:   true,
		},
		{
			name:         "context with spaces",
			contextValue: "inference hierarchy", // Spaces should be sanitized
			shouldWork:   true,
		},
		{
			name:         "community ID context",
			contextValue: "comm-0-robotics",
			shouldWork:   true,
		},
		{
			name:         "batch ID context",
			contextValue: "flight-mission-001",
			shouldWork:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			// Create mock bucket
			mockBucket := NewMockKeyValue()

			// Create index
			index := NewContextIndex(mockBucket, nil, nil, nil)

			entityState := &gtypes.EntityState{
				ID: "acme.iot.sensors.hvac.temperature.001",
				Triples: []message.Triple{
					{
						Subject:   "acme.iot.sensors.hvac.temperature.001",
						Predicate: "hierarchy.type.member",
						Object:    "acme.iot.sensors.hvac.temperature.group",
						Context:   tt.contextValue,
						Source:    "test",
						Timestamp: time.Now(),
					},
				},
				Version:   1,
				UpdatedAt: time.Now(),
			}

			err := index.HandleCreate(ctx, entityState.ID, entityState)
			if tt.shouldWork && err != nil {
				t.Errorf("HandleCreate() should work for context %q, got error: %v",
					tt.contextValue, err)
			}
		})
	}
}
