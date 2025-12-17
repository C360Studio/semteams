package indexmanager

import (
	"context"
	"testing"
	"time"

	gtypes "github.com/c360/semstreams/graph"
	"github.com/c360/semstreams/message"
)

// TestOutgoingEntry verifies the OutgoingEntry struct structure and field access
func TestOutgoingEntry(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		entry    OutgoingEntry
		wantPred string
		wantTo   string
	}{
		{
			name: "basic entry",
			entry: OutgoingEntry{
				Predicate:  "ops.fleet.member_of",
				ToEntityID: "acme.ops.logistics.hq.fleet.rescue",
			},
			wantPred: "ops.fleet.member_of",
			wantTo:   "acme.ops.logistics.hq.fleet.rescue",
		},
		{
			name: "operator relationship",
			entry: OutgoingEntry{
				Predicate:  "robotics.operator.controlled_by",
				ToEntityID: "acme.platform.auth.main.user.alice",
			},
			wantPred: "robotics.operator.controlled_by",
			wantTo:   "acme.platform.auth.main.user.alice",
		},
		{
			name: "proximity relationship",
			entry: OutgoingEntry{
				Predicate:  "spatial.proximity.near",
				ToEntityID: "acme.telemetry.robotics.gcs1.drone.002",
			},
			wantPred: "spatial.proximity.near",
			wantTo:   "acme.telemetry.robotics.gcs1.drone.002",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.entry.Predicate != tt.wantPred {
				t.Errorf("Predicate = %v, want %v", tt.entry.Predicate, tt.wantPred)
			}
			if tt.entry.ToEntityID != tt.wantTo {
				t.Errorf("ToEntityID = %v, want %v", tt.entry.ToEntityID, tt.wantTo)
			}
		})
	}
}

// TestOutgoingIndex_HandleCreate verifies index creation for new entities with relationships
func TestOutgoingIndex_HandleCreate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		entityID    string
		triples     []message.Triple
		wantEntries int
		wantErr     bool
	}{
		{
			name:     "entity with single relationship triple",
			entityID: "acme.telemetry.robotics.gcs1.drone.001",
			triples: []message.Triple{
				{
					Subject:   "acme.telemetry.robotics.gcs1.drone.001",
					Predicate: "ops.fleet.member_of",
					Object:    "acme.ops.logistics.hq.fleet.rescue",
					Source:    "test",
					Timestamp: time.Now(),
				},
			},
			wantEntries: 1,
			wantErr:     false,
		},
		{
			name:     "entity with property triple only (no relationships)",
			entityID: "acme.telemetry.robotics.gcs1.drone.001",
			triples: []message.Triple{
				{
					Subject:   "acme.telemetry.robotics.gcs1.drone.001",
					Predicate: "robotics.battery.level",
					Object:    85.5, // Numeric value, not entity reference
					Source:    "test",
					Timestamp: time.Now(),
				},
			},
			wantEntries: 0, // Properties don't create outgoing entries
			wantErr:     false,
		},
		{
			name:     "entity with multiple relationships",
			entityID: "acme.telemetry.robotics.gcs1.drone.001",
			triples: []message.Triple{
				{
					Subject:   "acme.telemetry.robotics.gcs1.drone.001",
					Predicate: "ops.fleet.member_of",
					Object:    "acme.ops.logistics.hq.fleet.rescue",
					Source:    "test",
					Timestamp: time.Now(),
				},
				{
					Subject:   "acme.telemetry.robotics.gcs1.drone.001",
					Predicate: "robotics.operator.controlled_by",
					Object:    "acme.platform.auth.main.user.alice",
					Source:    "test",
					Timestamp: time.Now(),
				},
			},
			wantEntries: 2,
			wantErr:     false,
		},
		{
			name:     "entity with mixed properties and relationships",
			entityID: "acme.telemetry.robotics.gcs1.drone.001",
			triples: []message.Triple{
				{
					Subject:   "acme.telemetry.robotics.gcs1.drone.001",
					Predicate: "robotics.battery.level",
					Object:    85.5,
					Source:    "test",
					Timestamp: time.Now(),
				},
				{
					Subject:   "acme.telemetry.robotics.gcs1.drone.001",
					Predicate: "ops.fleet.member_of",
					Object:    "acme.ops.logistics.hq.fleet.rescue",
					Source:    "test",
					Timestamp: time.Now(),
				},
				{
					Subject:   "acme.telemetry.robotics.gcs1.drone.001",
					Predicate: "robotics.battery.voltage",
					Object:    12.6,
					Source:    "test",
					Timestamp: time.Now(),
				},
			},
			wantEntries: 1, // Only one relationship
			wantErr:     false,
		},
		{
			name:        "entity with no triples",
			entityID:    "acme.telemetry.robotics.gcs1.drone.001",
			triples:     []message.Triple{},
			wantEntries: 0,
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			// Create mock bucket
			mockBucket := NewMockKeyValue()

			// Create index with mock
			index := NewOutgoingIndex(mockBucket, nil, nil, nil, nil)

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
				// Verify the correct number of entries were stored
				entries, err := index.GetOutgoing(ctx, tt.entityID)
				if err != nil && tt.wantEntries > 0 {
					t.Errorf("GetOutgoing() error = %v", err)
					return
				}

				if len(entries) != tt.wantEntries {
					t.Errorf("GetOutgoing() returned %d entries, want %d", len(entries), tt.wantEntries)
				}
			}
		})
	}
}

// TestOutgoingIndex_HandleUpdate verifies index updates with diff logic
func TestOutgoingIndex_HandleUpdate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		entityID       string
		initialTriples []message.Triple
		updatedTriples []message.Triple
		wantAdded      int
		wantRemoved    int
		wantFinal      int
	}{
		{
			name:     "add new relationship",
			entityID: "acme.telemetry.robotics.gcs1.drone.001",
			initialTriples: []message.Triple{
				{
					Subject:   "acme.telemetry.robotics.gcs1.drone.001",
					Predicate: "ops.fleet.member_of",
					Object:    "acme.ops.logistics.hq.fleet.rescue",
					Source:    "test",
					Timestamp: time.Now(),
				},
			},
			updatedTriples: []message.Triple{
				{
					Subject:   "acme.telemetry.robotics.gcs1.drone.001",
					Predicate: "ops.fleet.member_of",
					Object:    "acme.ops.logistics.hq.fleet.rescue",
					Source:    "test",
					Timestamp: time.Now(),
				},
				{
					Subject:   "acme.telemetry.robotics.gcs1.drone.001",
					Predicate: "robotics.operator.controlled_by",
					Object:    "acme.platform.auth.main.user.alice",
					Source:    "test",
					Timestamp: time.Now(),
				},
			},
			wantAdded:   1,
			wantRemoved: 0,
			wantFinal:   2,
		},
		{
			name:     "remove relationship",
			entityID: "acme.telemetry.robotics.gcs1.drone.001",
			initialTriples: []message.Triple{
				{
					Subject:   "acme.telemetry.robotics.gcs1.drone.001",
					Predicate: "ops.fleet.member_of",
					Object:    "acme.ops.logistics.hq.fleet.rescue",
					Source:    "test",
					Timestamp: time.Now(),
				},
				{
					Subject:   "acme.telemetry.robotics.gcs1.drone.001",
					Predicate: "robotics.operator.controlled_by",
					Object:    "acme.platform.auth.main.user.alice",
					Source:    "test",
					Timestamp: time.Now(),
				},
			},
			updatedTriples: []message.Triple{
				{
					Subject:   "acme.telemetry.robotics.gcs1.drone.001",
					Predicate: "ops.fleet.member_of",
					Object:    "acme.ops.logistics.hq.fleet.rescue",
					Source:    "test",
					Timestamp: time.Now(),
				},
			},
			wantAdded:   0,
			wantRemoved: 1,
			wantFinal:   1,
		},
		{
			name:     "change relationship target (same predicate, different object)",
			entityID: "acme.telemetry.robotics.gcs1.drone.001",
			initialTriples: []message.Triple{
				{
					Subject:   "acme.telemetry.robotics.gcs1.drone.001",
					Predicate: "ops.fleet.member_of",
					Object:    "acme.ops.logistics.hq.fleet.rescue",
					Source:    "test",
					Timestamp: time.Now(),
				},
			},
			updatedTriples: []message.Triple{
				{
					Subject:   "acme.telemetry.robotics.gcs1.drone.001",
					Predicate: "ops.fleet.member_of",
					Object:    "acme.ops.logistics.hq.fleet.search", // Different fleet
					Source:    "test",
					Timestamp: time.Now(),
				},
			},
			wantAdded:   1,
			wantRemoved: 1,
			wantFinal:   1,
		},
		{
			name:     "no changes (idempotent update)",
			entityID: "acme.telemetry.robotics.gcs1.drone.001",
			initialTriples: []message.Triple{
				{
					Subject:   "acme.telemetry.robotics.gcs1.drone.001",
					Predicate: "ops.fleet.member_of",
					Object:    "acme.ops.logistics.hq.fleet.rescue",
					Source:    "test",
					Timestamp: time.Now(),
				},
			},
			updatedTriples: []message.Triple{
				{
					Subject:   "acme.telemetry.robotics.gcs1.drone.001",
					Predicate: "ops.fleet.member_of",
					Object:    "acme.ops.logistics.hq.fleet.rescue",
					Source:    "test",
					Timestamp: time.Now(),
				},
			},
			wantAdded:   0,
			wantRemoved: 0,
			wantFinal:   1,
		},
		{
			name:     "remove all relationships",
			entityID: "acme.telemetry.robotics.gcs1.drone.001",
			initialTriples: []message.Triple{
				{
					Subject:   "acme.telemetry.robotics.gcs1.drone.001",
					Predicate: "ops.fleet.member_of",
					Object:    "acme.ops.logistics.hq.fleet.rescue",
					Source:    "test",
					Timestamp: time.Now(),
				},
			},
			updatedTriples: []message.Triple{
				{
					Subject:   "acme.telemetry.robotics.gcs1.drone.001",
					Predicate: "robotics.battery.level",
					Object:    85.5, // Only property, no relationships
					Source:    "test",
					Timestamp: time.Now(),
				},
			},
			wantAdded:   0,
			wantRemoved: 1,
			wantFinal:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			// Create mock bucket
			mockBucket := NewMockKeyValue()

			// Create index
			index := NewOutgoingIndex(mockBucket, nil, nil, nil, nil)

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

			// Verify final state
			entries, err := index.GetOutgoing(ctx, tt.entityID)
			if err != nil && tt.wantFinal > 0 {
				t.Errorf("GetOutgoing() error = %v", err)
				return
			}

			if len(entries) != tt.wantFinal {
				t.Errorf("GetOutgoing() returned %d entries, want %d", len(entries), tt.wantFinal)
			}
		})
	}
}

// TestOutgoingIndex_HandleDelete verifies complete removal of outgoing relationships
func TestOutgoingIndex_HandleDelete(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		entityID       string
		initialTriples []message.Triple
		wantErr        bool
	}{
		{
			name:     "delete entity with relationships",
			entityID: "acme.telemetry.robotics.gcs1.drone.001",
			initialTriples: []message.Triple{
				{
					Subject:   "acme.telemetry.robotics.gcs1.drone.001",
					Predicate: "ops.fleet.member_of",
					Object:    "acme.ops.logistics.hq.fleet.rescue",
					Source:    "test",
					Timestamp: time.Now(),
				},
				{
					Subject:   "acme.telemetry.robotics.gcs1.drone.001",
					Predicate: "robotics.operator.controlled_by",
					Object:    "acme.platform.auth.main.user.alice",
					Source:    "test",
					Timestamp: time.Now(),
				},
			},
			wantErr: false,
		},
		{
			name:           "delete entity with no relationships",
			entityID:       "acme.telemetry.robotics.gcs1.drone.002",
			initialTriples: []message.Triple{},
			wantErr:        false,
		},
		{
			name:           "delete nonexistent entity (idempotent)",
			entityID:       "acme.telemetry.robotics.gcs1.drone.999",
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
			index := NewOutgoingIndex(mockBucket, nil, nil, nil, nil)

			// Setup initial state if provided
			if tt.initialTriples != nil {
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
			}

			// Perform delete
			err := index.HandleDelete(ctx, tt.entityID)
			if (err != nil) != tt.wantErr {
				t.Errorf("HandleDelete() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Verify entity is no longer in index (should return empty slice, not error)
			entries, err := index.GetOutgoing(ctx, tt.entityID)
			if err != nil {
				t.Errorf("GetOutgoing() should return empty slice for deleted entity, got error: %v", err)
			}
			if len(entries) != 0 {
				t.Errorf("GetOutgoing() should return empty slice for deleted entity, got %d entries", len(entries))
			}
		})
	}
}

// TestOutgoingIndex_GetOutgoing verifies querying all outgoing relationships
func TestOutgoingIndex_GetOutgoing(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		entityID     string
		setupTriples []message.Triple
		wantCount    int
		wantErr      bool
		checkEntries func(t *testing.T, entries []OutgoingEntry)
	}{
		{
			name:     "entity with single relationship",
			entityID: "acme.telemetry.robotics.gcs1.drone.001",
			setupTriples: []message.Triple{
				{
					Subject:   "acme.telemetry.robotics.gcs1.drone.001",
					Predicate: "ops.fleet.member_of",
					Object:    "acme.ops.logistics.hq.fleet.rescue",
					Source:    "test",
					Timestamp: time.Now(),
				},
			},
			wantCount: 1,
			wantErr:   false,
			checkEntries: func(t *testing.T, entries []OutgoingEntry) {
				if entries[0].Predicate != "ops.fleet.member_of" {
					t.Errorf("Expected predicate ops.fleet.member_of, got %s", entries[0].Predicate)
				}
				if entries[0].ToEntityID != "acme.ops.logistics.hq.fleet.rescue" {
					t.Errorf("Expected target acme.ops.logistics.hq.fleet.rescue, got %s", entries[0].ToEntityID)
				}
			},
		},
		{
			name:     "entity with multiple relationships",
			entityID: "acme.telemetry.robotics.gcs1.drone.001",
			setupTriples: []message.Triple{
				{
					Subject:   "acme.telemetry.robotics.gcs1.drone.001",
					Predicate: "ops.fleet.member_of",
					Object:    "acme.ops.logistics.hq.fleet.rescue",
					Source:    "test",
					Timestamp: time.Now(),
				},
				{
					Subject:   "acme.telemetry.robotics.gcs1.drone.001",
					Predicate: "robotics.operator.controlled_by",
					Object:    "acme.platform.auth.main.user.alice",
					Source:    "test",
					Timestamp: time.Now(),
				},
				{
					Subject:   "acme.telemetry.robotics.gcs1.drone.001",
					Predicate: "spatial.proximity.near",
					Object:    "acme.telemetry.robotics.gcs1.drone.002",
					Source:    "test",
					Timestamp: time.Now(),
				},
			},
			wantCount: 3,
			wantErr:   false,
			checkEntries: func(t *testing.T, entries []OutgoingEntry) {
				// Verify all three relationships are present
				predicates := make(map[string]bool)
				for _, e := range entries {
					predicates[e.Predicate] = true
				}
				expected := []string{
					"ops.fleet.member_of",
					"robotics.operator.controlled_by",
					"spatial.proximity.near",
				}
				for _, pred := range expected {
					if !predicates[pred] {
						t.Errorf("Expected predicate %s not found", pred)
					}
				}
			},
		},
		{
			name:         "entity not found",
			entityID:     "acme.telemetry.robotics.gcs1.drone.999",
			setupTriples: nil, // Don't create entity
			wantCount:    0,
			wantErr:      false, // Not found returns empty slice, not error
		},
		{
			name:     "entity with no relationships (only properties)",
			entityID: "acme.telemetry.robotics.gcs1.drone.001",
			setupTriples: []message.Triple{
				{
					Subject:   "acme.telemetry.robotics.gcs1.drone.001",
					Predicate: "robotics.battery.level",
					Object:    85.5,
					Source:    "test",
					Timestamp: time.Now(),
				},
			},
			wantCount: 0,
			wantErr:   false, // No relationships returns empty slice, not error
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			// Create mock bucket
			mockBucket := NewMockKeyValue()

			// Create index
			index := NewOutgoingIndex(mockBucket, nil, nil, nil, nil)

			// Setup test data if provided
			if tt.setupTriples != nil {
				entityState := &gtypes.EntityState{
					ID:        tt.entityID,
					Triples:   tt.setupTriples,
					Version:   1,
					UpdatedAt: time.Now(),
				}

				err := index.HandleCreate(ctx, tt.entityID, entityState)
				if err != nil {
					t.Fatalf("HandleCreate() error = %v", err)
				}
			}

			// Query outgoing relationships
			entries, err := index.GetOutgoing(ctx, tt.entityID)

			if (err != nil) != tt.wantErr {
				t.Errorf("GetOutgoing() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if len(entries) != tt.wantCount {
					t.Errorf("GetOutgoing() returned %d entries, want %d", len(entries), tt.wantCount)
				}

				if tt.checkEntries != nil {
					tt.checkEntries(t, entries)
				}
			}
		})
	}
}

// TestOutgoingIndex_GetOutgoingByPredicate verifies filtering by predicate
func TestOutgoingIndex_GetOutgoingByPredicate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		entityID     string
		predicate    string
		setupTriples []message.Triple
		wantCount    int
		wantErr      bool
		checkEntries func(t *testing.T, entries []OutgoingEntry)
	}{
		{
			name:      "filter by single predicate",
			entityID:  "acme.telemetry.robotics.gcs1.drone.001",
			predicate: "ops.fleet.member_of",
			setupTriples: []message.Triple{
				{
					Subject:   "acme.telemetry.robotics.gcs1.drone.001",
					Predicate: "ops.fleet.member_of",
					Object:    "acme.ops.logistics.hq.fleet.rescue",
					Source:    "test",
					Timestamp: time.Now(),
				},
				{
					Subject:   "acme.telemetry.robotics.gcs1.drone.001",
					Predicate: "robotics.operator.controlled_by",
					Object:    "acme.platform.auth.main.user.alice",
					Source:    "test",
					Timestamp: time.Now(),
				},
			},
			wantCount: 1,
			wantErr:   false,
			checkEntries: func(t *testing.T, entries []OutgoingEntry) {
				if entries[0].Predicate != "ops.fleet.member_of" {
					t.Errorf("Expected predicate ops.fleet.member_of, got %s", entries[0].Predicate)
				}
			},
		},
		{
			name:      "filter returns no matches",
			entityID:  "acme.telemetry.robotics.gcs1.drone.001",
			predicate: "spatial.proximity.near",
			setupTriples: []message.Triple{
				{
					Subject:   "acme.telemetry.robotics.gcs1.drone.001",
					Predicate: "ops.fleet.member_of",
					Object:    "acme.ops.logistics.hq.fleet.rescue",
					Source:    "test",
					Timestamp: time.Now(),
				},
			},
			wantCount: 0,
			wantErr:   false,
		},
		{
			name:      "multiple relationships with same predicate",
			entityID:  "acme.telemetry.robotics.gcs1.drone.001",
			predicate: "spatial.proximity.near",
			setupTriples: []message.Triple{
				{
					Subject:   "acme.telemetry.robotics.gcs1.drone.001",
					Predicate: "spatial.proximity.near",
					Object:    "acme.telemetry.robotics.gcs1.drone.002",
					Source:    "test",
					Timestamp: time.Now(),
				},
				{
					Subject:   "acme.telemetry.robotics.gcs1.drone.001",
					Predicate: "spatial.proximity.near",
					Object:    "acme.telemetry.robotics.gcs1.drone.003",
					Source:    "test",
					Timestamp: time.Now(),
				},
				{
					Subject:   "acme.telemetry.robotics.gcs1.drone.001",
					Predicate: "ops.fleet.member_of",
					Object:    "acme.ops.logistics.hq.fleet.rescue",
					Source:    "test",
					Timestamp: time.Now(),
				},
			},
			wantCount: 2,
			wantErr:   false,
			checkEntries: func(t *testing.T, entries []OutgoingEntry) {
				for _, e := range entries {
					if e.Predicate != "spatial.proximity.near" {
						t.Errorf("Expected all entries to have predicate spatial.proximity.near, got %s", e.Predicate)
					}
				}
			},
		},
		{
			name:         "entity not found",
			entityID:     "acme.telemetry.robotics.gcs1.drone.999",
			predicate:    "ops.fleet.member_of",
			setupTriples: nil,
			wantCount:    0,
			wantErr:      false, // Not found returns empty slice, not error
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			// Create mock bucket
			mockBucket := NewMockKeyValue()

			// Create index
			index := NewOutgoingIndex(mockBucket, nil, nil, nil, nil)

			// Setup test data if provided
			if tt.setupTriples != nil {
				entityState := &gtypes.EntityState{
					ID:        tt.entityID,
					Triples:   tt.setupTriples,
					Version:   1,
					UpdatedAt: time.Now(),
				}

				err := index.HandleCreate(ctx, tt.entityID, entityState)
				if err != nil {
					t.Fatalf("HandleCreate() error = %v", err)
				}
			}

			// Query with predicate filter
			entries, err := index.GetOutgoingByPredicate(ctx, tt.entityID, tt.predicate)

			if (err != nil) != tt.wantErr {
				t.Errorf("GetOutgoingByPredicate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if len(entries) != tt.wantCount {
					t.Errorf("GetOutgoingByPredicate() returned %d entries, want %d", len(entries), tt.wantCount)
				}

				if tt.checkEntries != nil {
					tt.checkEntries(t, entries)
				}
			}
		})
	}
}

// TestOutgoingIndex_ExtractsRelationshipsFromTriples verifies that OUTGOING_INDEX
// correctly extracts relationships from triples (single source of truth)
func TestOutgoingIndex_ExtractsRelationshipsFromTriples(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		entityID          string
		triples           []message.Triple
		expectedRelations []OutgoingEntry
	}{
		{
			name:     "single relationship from triple",
			entityID: "acme.telemetry.robotics.gcs1.drone.001",
			triples: []message.Triple{
				{
					Subject:   "acme.telemetry.robotics.gcs1.drone.001",
					Predicate: "ops.fleet.member_of",
					Object:    "acme.ops.logistics.hq.fleet.rescue",
					Source:    "test",
					Timestamp: time.Now(),
				},
			},
			expectedRelations: []OutgoingEntry{
				{
					Predicate:  "ops.fleet.member_of",
					ToEntityID: "acme.ops.logistics.hq.fleet.rescue",
				},
			},
		},
		{
			name:     "multiple relationships from triples",
			entityID: "acme.telemetry.robotics.gcs1.drone.001",
			triples: []message.Triple{
				{
					Subject:   "acme.telemetry.robotics.gcs1.drone.001",
					Predicate: "ops.fleet.member_of",
					Object:    "acme.ops.logistics.hq.fleet.rescue",
					Source:    "test",
					Timestamp: time.Now(),
				},
				{
					Subject:   "acme.telemetry.robotics.gcs1.drone.001",
					Predicate: "robotics.operator.controlled_by",
					Object:    "acme.platform.auth.main.user.alice",
					Source:    "test",
					Timestamp: time.Now(),
				},
			},
			expectedRelations: []OutgoingEntry{
				{
					Predicate:  "ops.fleet.member_of",
					ToEntityID: "acme.ops.logistics.hq.fleet.rescue",
				},
				{
					Predicate:  "robotics.operator.controlled_by",
					ToEntityID: "acme.platform.auth.main.user.alice",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			// Create mock bucket
			mockBucket := NewMockKeyValue()

			// Create index
			index := NewOutgoingIndex(mockBucket, nil, nil, nil, nil)

			// Create entity with triples (single source of truth)
			entityState := &gtypes.EntityState{
				ID:        tt.entityID,
				Triples:   tt.triples,
				Version:   1,
				UpdatedAt: time.Now(),
			}

			// Index the entity
			err := index.HandleCreate(ctx, tt.entityID, entityState)
			if err != nil {
				t.Fatalf("HandleCreate() error = %v", err)
			}

			// Query via OUTGOING_INDEX
			indexEntries, err := index.GetOutgoing(ctx, tt.entityID)
			if err != nil {
				t.Fatalf("GetOutgoing() error = %v", err)
			}

			// Verify correct extraction from triples
			if len(indexEntries) != len(tt.expectedRelations) {
				t.Errorf("Expected %d relationships, got %d", len(tt.expectedRelations), len(indexEntries))
			}

			// Build map from index results for comparison
			indexMap := make(map[string]string) // predicate -> target
			for _, entry := range indexEntries {
				indexMap[entry.Predicate] = entry.ToEntityID
			}

			// Verify each expected relationship is in the index
			for _, expected := range tt.expectedRelations {
				indexTarget, ok := indexMap[expected.Predicate]
				if !ok {
					t.Errorf("Expected relationship with predicate %s not found in index", expected.Predicate)
					continue
				}
				if expected.ToEntityID != indexTarget {
					t.Errorf("Target mismatch for predicate %s: expected=%s, got=%s",
						expected.Predicate, expected.ToEntityID, indexTarget)
				}
			}
		})
	}
}
