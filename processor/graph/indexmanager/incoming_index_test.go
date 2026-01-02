package indexmanager

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gtypes "github.com/c360/semstreams/graph"
	"github.com/c360/semstreams/message"
)

// TestIncomingEntry verifies that IncomingEntry struct serializes correctly
func TestIncomingEntry(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		entry    IncomingEntry
		wantJSON string
	}{
		{
			name: "hierarchy member relationship",
			entry: IncomingEntry{
				Predicate:    "hierarchy.type.member",
				FromEntityID: "acme.iot.sensors.hvac.temperature.001",
			},
			wantJSON: `{"predicate":"hierarchy.type.member","from_entity_id":"acme.iot.sensors.hvac.temperature.001"}`,
		},
		{
			name: "spatial proximity relationship",
			entry: IncomingEntry{
				Predicate:    "spatial.proximity.near",
				FromEntityID: "acme.iot.sensors.hvac.humidity.002",
			},
			wantJSON: `{"predicate":"spatial.proximity.near","from_entity_id":"acme.iot.sensors.hvac.humidity.002"}`,
		},
		{
			name: "fleet member relationship",
			entry: IncomingEntry{
				Predicate:    "ops.fleet.member_of",
				FromEntityID: "acme.drones.fleet.alpha.uav.001",
			},
			wantJSON: `{"predicate":"ops.fleet.member_of","from_entity_id":"acme.drones.fleet.alpha.uav.001"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test serialization
			data, err := json.Marshal(tt.entry)
			require.NoError(t, err)
			assert.JSONEq(t, tt.wantJSON, string(data))

			// Test deserialization
			var decoded IncomingEntry
			err = json.Unmarshal(data, &decoded)
			require.NoError(t, err)
			assert.Equal(t, tt.entry, decoded)
		})
	}
}

// TestIncomingIndex_HandleCreate verifies entity creation indexing
func TestIncomingIndex_HandleCreate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		entityID     string
		triples      []message.Triple
		wantContexts map[string][]IncomingEntry // targetEntityID -> expected entries
		wantErr      bool
	}{
		{
			name:     "entity with single relationship triple",
			entityID: "acme.iot.sensors.hvac.temperature.001",
			triples: []message.Triple{
				{
					Subject:     "acme.iot.sensors.hvac.temperature.001",
					Predicate:   "hierarchy.type.member",
					Object:      "acme.iot.sensors.hvac.temperature.group",
					
					Source:      "test",
					Timestamp:   time.Now(),
				},
			},
			wantContexts: map[string][]IncomingEntry{
				"acme.iot.sensors.hvac.temperature.group": {
					{Predicate: "hierarchy.type.member", FromEntityID: "acme.iot.sensors.hvac.temperature.001"},
				},
			},
			wantErr: false,
		},
		{
			name:     "entity with property triple only (no relationships)",
			entityID: "acme.iot.sensors.hvac.temperature.001",
			triples: []message.Triple{
				{
					Subject:     "acme.iot.sensors.hvac.temperature.001",
					Predicate:   "sensor.reading.value",
					Object:      72.5,
					
					Source:      "test",
					Timestamp:   time.Now(),
				},
			},
			wantContexts: map[string][]IncomingEntry{}, // No relationships = no incoming entries
			wantErr:      false,
		},
		{
			name:     "entity with multiple relationships to different targets",
			entityID: "acme.drones.fleet.alpha.uav.001",
			triples: []message.Triple{
				{
					Subject:     "acme.drones.fleet.alpha.uav.001",
					Predicate:   "spatial.proximity.near",
					Object:      "acme.drones.fleet.alpha.uav.002",
					
					Source:      "test",
					Timestamp:   time.Now(),
				},
				{
					Subject:     "acme.drones.fleet.alpha.uav.001",
					Predicate:   "ops.fleet.member_of",
					Object:      "acme.drones.fleet.alpha.fleet.main",
					
					Source:      "test",
					Timestamp:   time.Now(),
				},
			},
			wantContexts: map[string][]IncomingEntry{
				"acme.drones.fleet.alpha.uav.002": {
					{Predicate: "spatial.proximity.near", FromEntityID: "acme.drones.fleet.alpha.uav.001"},
				},
				"acme.drones.fleet.alpha.fleet.main": {
					{Predicate: "ops.fleet.member_of", FromEntityID: "acme.drones.fleet.alpha.uav.001"},
				},
			},
			wantErr: false,
		},
		{
			name:     "entity with multiple relationships to same target",
			entityID: "acme.iot.sensors.hvac.temperature.001",
			triples: []message.Triple{
				{
					Subject:     "acme.iot.sensors.hvac.temperature.001",
					Predicate:   "spatial.proximity.near",
					Object:      "acme.iot.sensors.hvac.humidity.002",
					
					Source:      "test",
					Timestamp:   time.Now(),
				},
				{
					Subject:     "acme.iot.sensors.hvac.temperature.001",
					Predicate:   "sensor.correlation.strong",
					Object:      "acme.iot.sensors.hvac.humidity.002",
					
					Source:      "test",
					Timestamp:   time.Now(),
				},
			},
			wantContexts: map[string][]IncomingEntry{
				"acme.iot.sensors.hvac.humidity.002": {
					{Predicate: "spatial.proximity.near", FromEntityID: "acme.iot.sensors.hvac.temperature.001"},
					{Predicate: "sensor.correlation.strong", FromEntityID: "acme.iot.sensors.hvac.temperature.001"},
				},
			},
			wantErr: false,
		},
		{
			name:         "entity with no triples",
			entityID:     "acme.iot.sensors.hvac.temperature.001",
			triples:      []message.Triple{},
			wantContexts: map[string][]IncomingEntry{},
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			// Create mock bucket
			mockBucket := NewMockKeyValue()

			// Create index
			index := NewIncomingIndex(mockBucket, nil, nil, nil)

			// Create entity state
			entityState := &gtypes.EntityState{
				ID:        tt.entityID,
				Triples:   tt.triples,
				Version:   1,
				UpdatedAt: time.Now(),
			}

			// Execute HandleCreate
			err := index.HandleCreate(ctx, tt.entityID, entityState)
			if (err != nil) != tt.wantErr {
				t.Errorf("HandleCreate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Verify entries were created for each target
			for targetID, wantEntries := range tt.wantContexts {
				entries, err := index.GetIncoming(ctx, targetID)
				require.NoError(t, err)
				assert.Len(t, entries, len(wantEntries), "Unexpected entry count for target %s", targetID)

				// Verify each expected entry exists
				for _, wantEntry := range wantEntries {
					found := false
					for _, gotEntry := range entries {
						if gotEntry.Predicate == wantEntry.Predicate && gotEntry.FromEntityID == wantEntry.FromEntityID {
							found = true
							break
						}
					}
					assert.True(t, found, "Expected entry %+v not found for target %s", wantEntry, targetID)
				}
			}
		})
	}
}

// TestIncomingIndex_HandleUpdate verifies entity update handling
func TestIncomingIndex_HandleUpdate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		entityID       string
		initialTriples []message.Triple
		updatedTriples []message.Triple
		wantContexts   map[string][]IncomingEntry
		wantErr        bool
	}{
		{
			name:     "add new relationship",
			entityID: "acme.iot.sensors.hvac.temperature.001",
			initialTriples: []message.Triple{
				{
					Subject:     "acme.iot.sensors.hvac.temperature.001",
					Predicate:   "sensor.reading.value",
					Object:      72.5,
					
					Source:      "test",
					Timestamp:   time.Now(),
				},
			},
			updatedTriples: []message.Triple{
				{
					Subject:     "acme.iot.sensors.hvac.temperature.001",
					Predicate:   "sensor.reading.value",
					Object:      72.5,
					
					Source:      "test",
					Timestamp:   time.Now(),
				},
				{
					Subject:     "acme.iot.sensors.hvac.temperature.001",
					Predicate:   "hierarchy.type.member",
					Object:      "acme.iot.sensors.hvac.temperature.group",
					
					Source:      "test",
					Timestamp:   time.Now(),
				},
			},
			wantContexts: map[string][]IncomingEntry{
				"acme.iot.sensors.hvac.temperature.group": {
					{Predicate: "hierarchy.type.member", FromEntityID: "acme.iot.sensors.hvac.temperature.001"},
				},
			},
			wantErr: false,
		},
		{
			name:     "change relationship predicate",
			entityID: "acme.iot.sensors.hvac.temperature.001",
			initialTriples: []message.Triple{
				{
					Subject:     "acme.iot.sensors.hvac.temperature.001",
					Predicate:   "spatial.proximity.near",
					Object:      "acme.iot.sensors.hvac.humidity.002",
					
					Source:      "test",
					Timestamp:   time.Now(),
				},
			},
			updatedTriples: []message.Triple{
				{
					Subject:     "acme.iot.sensors.hvac.temperature.001",
					Predicate:   "sensor.correlation.strong",
					Object:      "acme.iot.sensors.hvac.humidity.002",
					
					Source:      "test",
					Timestamp:   time.Now(),
				},
			},
			wantContexts: map[string][]IncomingEntry{
				"acme.iot.sensors.hvac.humidity.002": {
					{Predicate: "sensor.correlation.strong", FromEntityID: "acme.iot.sensors.hvac.temperature.001"},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			// Create mock bucket
			mockBucket := NewMockKeyValue()

			// Create index
			index := NewIncomingIndex(mockBucket, nil, nil, nil)

			// Setup initial state
			if len(tt.initialTriples) > 0 {
				initialState := &gtypes.EntityState{
					ID:        tt.entityID,
					Triples:   tt.initialTriples,
					Version:   1,
					UpdatedAt: time.Now(),
				}
				err := index.HandleCreate(ctx, tt.entityID, initialState)
				require.NoError(t, err)
			}

			// Execute HandleUpdate
			updatedState := &gtypes.EntityState{
				ID:        tt.entityID,
				Triples:   tt.updatedTriples,
				Version:   2,
				UpdatedAt: time.Now(),
			}
			err := index.HandleUpdate(ctx, tt.entityID, updatedState)
			if (err != nil) != tt.wantErr {
				t.Errorf("HandleUpdate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Verify entries for each target
			for targetID, wantEntries := range tt.wantContexts {
				entries, err := index.GetIncoming(ctx, targetID)
				require.NoError(t, err)

				// Verify expected entries exist
				for _, wantEntry := range wantEntries {
					found := false
					for _, gotEntry := range entries {
						if gotEntry.Predicate == wantEntry.Predicate && gotEntry.FromEntityID == wantEntry.FromEntityID {
							found = true
							break
						}
					}
					assert.True(t, found, "Expected entry %+v not found for target %s", wantEntry, targetID)
				}
			}
		})
	}
}

// TestIncomingIndex_AddIncomingReference verifies adding individual references
func TestIncomingIndex_AddIncomingReference(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		toEntityID   string
		fromEntityID string
		predicate    string
		existingRefs []IncomingEntry
		wantRefs     []IncomingEntry
		wantErr      bool
	}{
		{
			name:         "add first reference",
			toEntityID:   "acme.iot.sensors.hvac.temperature.group",
			fromEntityID: "acme.iot.sensors.hvac.temperature.001",
			predicate:    "hierarchy.type.member",
			existingRefs: nil,
			wantRefs: []IncomingEntry{
				{Predicate: "hierarchy.type.member", FromEntityID: "acme.iot.sensors.hvac.temperature.001"},
			},
			wantErr: false,
		},
		{
			name:         "add second reference from different entity",
			toEntityID:   "acme.iot.sensors.hvac.temperature.group",
			fromEntityID: "acme.iot.sensors.hvac.temperature.002",
			predicate:    "hierarchy.type.member",
			existingRefs: []IncomingEntry{
				{Predicate: "hierarchy.type.member", FromEntityID: "acme.iot.sensors.hvac.temperature.001"},
			},
			wantRefs: []IncomingEntry{
				{Predicate: "hierarchy.type.member", FromEntityID: "acme.iot.sensors.hvac.temperature.001"},
				{Predicate: "hierarchy.type.member", FromEntityID: "acme.iot.sensors.hvac.temperature.002"},
			},
			wantErr: false,
		},
		{
			name:         "add reference with different predicate from same entity",
			toEntityID:   "acme.iot.sensors.hvac.humidity.002",
			fromEntityID: "acme.iot.sensors.hvac.temperature.001",
			predicate:    "sensor.correlation.strong",
			existingRefs: []IncomingEntry{
				{Predicate: "spatial.proximity.near", FromEntityID: "acme.iot.sensors.hvac.temperature.001"},
			},
			wantRefs: []IncomingEntry{
				{Predicate: "spatial.proximity.near", FromEntityID: "acme.iot.sensors.hvac.temperature.001"},
				{Predicate: "sensor.correlation.strong", FromEntityID: "acme.iot.sensors.hvac.temperature.001"},
			},
			wantErr: false,
		},
		{
			name:         "duplicate reference is idempotent",
			toEntityID:   "acme.iot.sensors.hvac.temperature.group",
			fromEntityID: "acme.iot.sensors.hvac.temperature.001",
			predicate:    "hierarchy.type.member",
			existingRefs: []IncomingEntry{
				{Predicate: "hierarchy.type.member", FromEntityID: "acme.iot.sensors.hvac.temperature.001"},
			},
			wantRefs: []IncomingEntry{
				{Predicate: "hierarchy.type.member", FromEntityID: "acme.iot.sensors.hvac.temperature.001"},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			// Create mock bucket
			mockBucket := NewMockKeyValue()

			// Setup existing refs
			if len(tt.existingRefs) > 0 {
				data, _ := json.Marshal(tt.existingRefs)
				mockBucket.data[tt.toEntityID] = data
			}

			// Create index
			index := NewIncomingIndex(mockBucket, nil, nil, nil)

			// Execute AddIncomingReference
			err := index.AddIncomingReference(ctx, tt.toEntityID, tt.fromEntityID, tt.predicate)
			if (err != nil) != tt.wantErr {
				t.Errorf("AddIncomingReference() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Verify resulting refs
			entries, err := index.GetIncoming(ctx, tt.toEntityID)
			require.NoError(t, err)
			assert.Len(t, entries, len(tt.wantRefs))

			for _, wantRef := range tt.wantRefs {
				found := false
				for _, gotRef := range entries {
					if gotRef.Predicate == wantRef.Predicate && gotRef.FromEntityID == wantRef.FromEntityID {
						found = true
						break
					}
				}
				assert.True(t, found, "Expected ref %+v not found", wantRef)
			}
		})
	}
}

// TestIncomingIndex_RemoveIncomingReference verifies removing references
func TestIncomingIndex_RemoveIncomingReference(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		toEntityID   string
		fromEntityID string
		existingRefs []IncomingEntry
		wantRefs     []IncomingEntry
		wantDeleted  bool // Whether the key should be deleted entirely
		wantErr      bool
	}{
		{
			name:         "remove only reference deletes key",
			toEntityID:   "acme.iot.sensors.hvac.temperature.group",
			fromEntityID: "acme.iot.sensors.hvac.temperature.001",
			existingRefs: []IncomingEntry{
				{Predicate: "hierarchy.type.member", FromEntityID: "acme.iot.sensors.hvac.temperature.001"},
			},
			wantRefs:    []IncomingEntry{},
			wantDeleted: true,
			wantErr:     false,
		},
		{
			name:         "remove one of multiple references",
			toEntityID:   "acme.iot.sensors.hvac.temperature.group",
			fromEntityID: "acme.iot.sensors.hvac.temperature.001",
			existingRefs: []IncomingEntry{
				{Predicate: "hierarchy.type.member", FromEntityID: "acme.iot.sensors.hvac.temperature.001"},
				{Predicate: "hierarchy.type.member", FromEntityID: "acme.iot.sensors.hvac.temperature.002"},
			},
			wantRefs: []IncomingEntry{
				{Predicate: "hierarchy.type.member", FromEntityID: "acme.iot.sensors.hvac.temperature.002"},
			},
			wantDeleted: false,
			wantErr:     false,
		},
		{
			name:         "remove entity with multiple predicates removes all",
			toEntityID:   "acme.iot.sensors.hvac.humidity.002",
			fromEntityID: "acme.iot.sensors.hvac.temperature.001",
			existingRefs: []IncomingEntry{
				{Predicate: "spatial.proximity.near", FromEntityID: "acme.iot.sensors.hvac.temperature.001"},
				{Predicate: "sensor.correlation.strong", FromEntityID: "acme.iot.sensors.hvac.temperature.001"},
				{Predicate: "hierarchy.type.member", FromEntityID: "acme.iot.sensors.hvac.temperature.003"},
			},
			wantRefs: []IncomingEntry{
				{Predicate: "hierarchy.type.member", FromEntityID: "acme.iot.sensors.hvac.temperature.003"},
			},
			wantDeleted: false,
			wantErr:     false,
		},
		{
			name:         "remove nonexistent reference is idempotent",
			toEntityID:   "acme.iot.sensors.hvac.temperature.group",
			fromEntityID: "acme.iot.sensors.hvac.temperature.999",
			existingRefs: []IncomingEntry{
				{Predicate: "hierarchy.type.member", FromEntityID: "acme.iot.sensors.hvac.temperature.001"},
			},
			wantRefs: []IncomingEntry{
				{Predicate: "hierarchy.type.member", FromEntityID: "acme.iot.sensors.hvac.temperature.001"},
			},
			wantDeleted: false,
			wantErr:     false,
		},
		{
			name:         "remove from nonexistent target is idempotent",
			toEntityID:   "acme.iot.sensors.hvac.temperature.group",
			fromEntityID: "acme.iot.sensors.hvac.temperature.001",
			existingRefs: nil,
			wantRefs:     []IncomingEntry{},
			wantDeleted:  false, // Key doesn't exist, so no deletion
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			// Create mock bucket
			mockBucket := NewMockKeyValue()

			// Setup existing refs
			if len(tt.existingRefs) > 0 {
				data, _ := json.Marshal(tt.existingRefs)
				mockBucket.data[tt.toEntityID] = data
			}

			// Create index
			index := NewIncomingIndex(mockBucket, nil, nil, nil)

			// Execute RemoveIncomingReference
			err := index.RemoveIncomingReference(ctx, tt.toEntityID, tt.fromEntityID)
			if (err != nil) != tt.wantErr {
				t.Errorf("RemoveIncomingReference() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Verify key deletion if expected
			if tt.wantDeleted {
				_, exists := mockBucket.data[tt.toEntityID]
				assert.False(t, exists, "Key should have been deleted")
				return
			}

			// Verify resulting refs
			if len(tt.wantRefs) > 0 {
				entries, err := index.GetIncoming(ctx, tt.toEntityID)
				require.NoError(t, err)
				assert.Len(t, entries, len(tt.wantRefs))
			}
		})
	}
}

// TestIncomingIndex_GetIncoming verifies querying incoming relationships
func TestIncomingIndex_GetIncoming(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		entityID     string
		existingRefs []IncomingEntry
		wantRefs     []IncomingEntry
		wantErr      bool
	}{
		{
			name:     "entity with single incoming reference",
			entityID: "acme.iot.sensors.hvac.temperature.group",
			existingRefs: []IncomingEntry{
				{Predicate: "hierarchy.type.member", FromEntityID: "acme.iot.sensors.hvac.temperature.001"},
			},
			wantRefs: []IncomingEntry{
				{Predicate: "hierarchy.type.member", FromEntityID: "acme.iot.sensors.hvac.temperature.001"},
			},
			wantErr: false,
		},
		{
			name:     "entity with multiple incoming references",
			entityID: "acme.iot.sensors.hvac.temperature.group",
			existingRefs: []IncomingEntry{
				{Predicate: "hierarchy.type.member", FromEntityID: "acme.iot.sensors.hvac.temperature.001"},
				{Predicate: "hierarchy.type.member", FromEntityID: "acme.iot.sensors.hvac.temperature.002"},
				{Predicate: "hierarchy.type.member", FromEntityID: "acme.iot.sensors.hvac.temperature.003"},
			},
			wantRefs: []IncomingEntry{
				{Predicate: "hierarchy.type.member", FromEntityID: "acme.iot.sensors.hvac.temperature.001"},
				{Predicate: "hierarchy.type.member", FromEntityID: "acme.iot.sensors.hvac.temperature.002"},
				{Predicate: "hierarchy.type.member", FromEntityID: "acme.iot.sensors.hvac.temperature.003"},
			},
			wantErr: false,
		},
		{
			name:         "entity not found returns empty slice",
			entityID:     "acme.iot.sensors.hvac.temperature.999",
			existingRefs: nil,
			wantRefs:     []IncomingEntry{},
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			// Create mock bucket
			mockBucket := NewMockKeyValue()

			// Setup existing refs
			if len(tt.existingRefs) > 0 {
				data, _ := json.Marshal(tt.existingRefs)
				mockBucket.data[tt.entityID] = data
			}

			// Create index
			index := NewIncomingIndex(mockBucket, nil, nil, nil)

			// Execute GetIncoming
			entries, err := index.GetIncoming(ctx, tt.entityID)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetIncoming() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			assert.Len(t, entries, len(tt.wantRefs))
			for i, wantRef := range tt.wantRefs {
				assert.Equal(t, wantRef.Predicate, entries[i].Predicate)
				assert.Equal(t, wantRef.FromEntityID, entries[i].FromEntityID)
			}
		})
	}
}

// TestIncomingIndex_GetIncomingByPredicate verifies filtering by predicate
func TestIncomingIndex_GetIncomingByPredicate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		entityID     string
		predicate    string
		existingRefs []IncomingEntry
		wantRefs     []IncomingEntry
		wantErr      bool
	}{
		{
			name:      "filter by single predicate",
			entityID:  "acme.iot.sensors.hvac.humidity.002",
			predicate: "spatial.proximity.near",
			existingRefs: []IncomingEntry{
				{Predicate: "spatial.proximity.near", FromEntityID: "acme.iot.sensors.hvac.temperature.001"},
				{Predicate: "sensor.correlation.strong", FromEntityID: "acme.iot.sensors.hvac.temperature.001"},
				{Predicate: "spatial.proximity.near", FromEntityID: "acme.iot.sensors.hvac.temperature.003"},
			},
			wantRefs: []IncomingEntry{
				{Predicate: "spatial.proximity.near", FromEntityID: "acme.iot.sensors.hvac.temperature.001"},
				{Predicate: "spatial.proximity.near", FromEntityID: "acme.iot.sensors.hvac.temperature.003"},
			},
			wantErr: false,
		},
		{
			name:      "filter returns no matches",
			entityID:  "acme.iot.sensors.hvac.humidity.002",
			predicate: "nonexistent.predicate",
			existingRefs: []IncomingEntry{
				{Predicate: "spatial.proximity.near", FromEntityID: "acme.iot.sensors.hvac.temperature.001"},
			},
			wantRefs: []IncomingEntry{},
			wantErr:  false,
		},
		{
			name:         "entity not found returns empty slice",
			entityID:     "acme.iot.sensors.hvac.temperature.999",
			predicate:    "hierarchy.type.member",
			existingRefs: nil,
			wantRefs:     []IncomingEntry{},
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			// Create mock bucket
			mockBucket := NewMockKeyValue()

			// Setup existing refs
			if len(tt.existingRefs) > 0 {
				data, _ := json.Marshal(tt.existingRefs)
				mockBucket.data[tt.entityID] = data
			}

			// Create index
			index := NewIncomingIndex(mockBucket, nil, nil, nil)

			// Execute GetIncomingByPredicate
			entries, err := index.GetIncomingByPredicate(ctx, tt.entityID, tt.predicate)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetIncomingByPredicate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			assert.Len(t, entries, len(tt.wantRefs))
			for _, wantRef := range tt.wantRefs {
				found := false
				for _, gotRef := range entries {
					if gotRef.Predicate == wantRef.Predicate && gotRef.FromEntityID == wantRef.FromEntityID {
						found = true
						break
					}
				}
				assert.True(t, found, "Expected ref %+v not found", wantRef)
			}
		})
	}
}

// TestIncomingIndex_SymmetryWithOutgoing verifies IncomingEntry mirrors OutgoingEntry
func TestIncomingIndex_SymmetryWithOutgoing(t *testing.T) {
	t.Parallel()

	// IncomingEntry and OutgoingEntry should have symmetric structure
	incoming := IncomingEntry{
		Predicate:    "hierarchy.type.member",
		FromEntityID: "acme.iot.sensors.hvac.temperature.001",
	}

	outgoing := OutgoingEntry{
		Predicate:  "hierarchy.type.member",
		ToEntityID: "acme.iot.sensors.hvac.temperature.group",
	}

	// Both should serialize with same field patterns
	incomingJSON, _ := json.Marshal(incoming)
	outgoingJSON, _ := json.Marshal(outgoing)

	var incomingMap, outgoingMap map[string]interface{}
	json.Unmarshal(incomingJSON, &incomingMap)
	json.Unmarshal(outgoingJSON, &outgoingMap)

	// Both should have "predicate" field
	assert.Contains(t, incomingMap, "predicate")
	assert.Contains(t, outgoingMap, "predicate")

	// IncomingEntry uses "from_entity_id", OutgoingEntry uses "to_entity_id"
	assert.Contains(t, incomingMap, "from_entity_id")
	assert.Contains(t, outgoingMap, "to_entity_id")

	// Same number of fields (symmetric structure)
	assert.Equal(t, len(incomingMap), len(outgoingMap))
}
