package federation_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/c360studio/semstreams/federation"
	"github.com/c360studio/semstreams/message"
)

func TestEvent_Validate(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name    string
		event   federation.Event
		wantErr bool
	}{
		{
			name: "valid SEED event",
			event: federation.Event{
				Type:      federation.EventTypeSEED,
				SourceID:  "test-source",
				Namespace: "acme",
				Timestamp: now,
				Provenance: federation.Provenance{
					SourceType: "git",
					SourceID:   "test-source",
					Timestamp:  now,
					Handler:    "GitHandler",
				},
			},
			wantErr: false,
		},
		{
			name: "valid DELTA event with entities",
			event: federation.Event{
				Type:      federation.EventTypeDELTA,
				SourceID:  "test-source",
				Namespace: "acme",
				Timestamp: now,
				Entities: []federation.Entity{
					{
						ID:      "acme.platform.git.my-repo.commit.a1b2c3",
						Triples: []message.Triple{{Subject: "acme.platform.git.my-repo.commit.a1b2c3", Predicate: "git.commit.sha", Object: "a1b2c3"}},
					},
				},
				Provenance: federation.Provenance{
					SourceType: "git",
					SourceID:   "test-source",
					Timestamp:  now,
					Handler:    "GitHandler",
				},
			},
			wantErr: false,
		},
		{
			name: "valid RETRACT event",
			event: federation.Event{
				Type:        federation.EventTypeRETRACT,
				SourceID:    "test-source",
				Namespace:   "acme",
				Timestamp:   now,
				Retractions: []string{"acme.platform.git.my-repo.commit.a1b2c3"},
				Provenance: federation.Provenance{
					SourceType: "git",
					SourceID:   "test-source",
					Timestamp:  now,
					Handler:    "GitHandler",
				},
			},
			wantErr: false,
		},
		{
			name: "valid HEARTBEAT event",
			event: federation.Event{
				Type:      federation.EventTypeHEARTBEAT,
				SourceID:  "test-source",
				Namespace: "acme",
				Timestamp: now,
				Provenance: federation.Provenance{
					SourceType: "internal",
					SourceID:   "test-source",
					Timestamp:  now,
					Handler:    "Engine",
				},
			},
			wantErr: false,
		},
		{
			name: "missing type",
			event: federation.Event{
				SourceID:  "test-source",
				Namespace: "acme",
				Timestamp: now,
				Provenance: federation.Provenance{
					SourceType: "git",
					SourceID:   "test-source",
					Timestamp:  now,
					Handler:    "GitHandler",
				},
			},
			wantErr: true,
		},
		{
			name: "missing source ID",
			event: federation.Event{
				Type:      federation.EventTypeSEED,
				Namespace: "acme",
				Timestamp: now,
				Provenance: federation.Provenance{
					SourceType: "git",
					SourceID:   "test-source",
					Timestamp:  now,
					Handler:    "GitHandler",
				},
			},
			wantErr: true,
		},
		{
			name: "missing namespace",
			event: federation.Event{
				Type:     federation.EventTypeSEED,
				SourceID: "test-source",

				Timestamp: now,
				Provenance: federation.Provenance{
					SourceType: "git",
					SourceID:   "test-source",
					Timestamp:  now,
					Handler:    "GitHandler",
				},
			},
			wantErr: true,
		},
		{
			name: "unknown event type",
			event: federation.Event{
				Type:      federation.EventType("BOGUS"),
				SourceID:  "test-source",
				Namespace: "acme",
				Timestamp: now,
			},
			wantErr: true,
		},
		{
			name: "zero timestamp",
			event: federation.Event{
				Type:      federation.EventTypeSEED,
				SourceID:  "test-source",
				Namespace: "acme",
				Provenance: federation.Provenance{
					SourceType: "git",
					SourceID:   "test-source",
					Timestamp:  now,
					Handler:    "GitHandler",
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.event.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestEventPayload_JSONRoundTrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)

	original := &federation.EventPayload{
		Event: federation.Event{
			Type:      federation.EventTypeDELTA,
			SourceID:  "my-source",
			Namespace: "acme",
			Timestamp: now,
			Entities: []federation.Entity{
				{
					ID: "acme.platform.git.my-repo.commit.a1b2c3",
					Triples: []message.Triple{
						{
							Subject:   "acme.platform.git.my-repo.commit.a1b2c3",
							Predicate: "git.commit.sha",
							Object:    "a1b2c3",
							Timestamp: now,
						},
					},
					Edges: []federation.Edge{
						{
							FromID:   "acme.platform.git.my-repo.commit.a1b2c3",
							ToID:     "acme.platform.git.my-repo.author.alice",
							EdgeType: "authored_by",
							Weight:   1.0,
						},
					},
					Provenance: federation.Provenance{
						SourceType: "git",
						SourceID:   "my-source",
						Timestamp:  now,
						Handler:    "GitHandler",
					},
				},
			},
			Provenance: federation.Provenance{
				SourceType: "git",
				SourceID:   "my-source",
				Timestamp:  now,
				Handler:    "GitHandler",
			},
		},
	}

	data, err := original.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON() error = %v", err)
	}

	restored := &federation.EventPayload{}
	if err := restored.UnmarshalJSON(data); err != nil {
		t.Fatalf("UnmarshalJSON() error = %v", err)
	}

	if restored.Event.Type != original.Event.Type {
		t.Errorf("Type mismatch: got %v, want %v", restored.Event.Type, original.Event.Type)
	}
	if restored.Event.SourceID != original.Event.SourceID {
		t.Errorf("SourceID mismatch: got %v, want %v", restored.Event.SourceID, original.Event.SourceID)
	}
	if restored.Event.Namespace != original.Event.Namespace {
		t.Errorf("Namespace mismatch: got %v, want %v", restored.Event.Namespace, original.Event.Namespace)
	}
	if len(restored.Event.Entities) != len(original.Event.Entities) {
		t.Fatalf("Entities count mismatch: got %d, want %d", len(restored.Event.Entities), len(original.Event.Entities))
	}
	if restored.Event.Entities[0].ID != original.Event.Entities[0].ID {
		t.Errorf("Entity ID mismatch: got %v, want %v", restored.Event.Entities[0].ID, original.Event.Entities[0].ID)
	}
}

func TestEventPayload_Schema(t *testing.T) {
	p := &federation.EventPayload{}
	schema := p.Schema()

	if schema.Domain != "federation" {
		t.Errorf("Schema Domain = %q, want %q", schema.Domain, "federation")
	}
	if schema.Category != "graph_event" {
		t.Errorf("Schema Category = %q, want %q", schema.Category, "graph_event")
	}
	if schema.Version != "v1" {
		t.Errorf("Schema Version = %q, want %q", schema.Version, "v1")
	}
}

func TestEventPayload_Validate(t *testing.T) {
	now := time.Now()

	t.Run("valid payload", func(t *testing.T) {
		p := &federation.EventPayload{
			Event: federation.Event{
				Type:      federation.EventTypeSEED,
				SourceID:  "my-source",
				Namespace: "acme",
				Timestamp: now,
				Provenance: federation.Provenance{
					SourceType: "git",
					SourceID:   "my-source",
					Timestamp:  now,
					Handler:    "GitHandler",
				},
			},
		}
		if err := p.Validate(); err != nil {
			t.Errorf("Validate() unexpected error: %v", err)
		}
	})

	t.Run("invalid event", func(t *testing.T) {
		p := &federation.EventPayload{
			Event: federation.Event{},
		}
		if err := p.Validate(); err == nil {
			t.Error("Validate() expected error for empty event")
		}
	})
}

func TestEventPayload_PayloadRegistration(t *testing.T) {
	p := &federation.EventPayload{}
	schema := p.Schema()

	event := federation.Event{
		Type:      federation.EventTypeHEARTBEAT,
		SourceID:  "heartbeat-source",
		Namespace: "acme",
		Timestamp: time.Now(),
		Provenance: federation.Provenance{
			SourceType: "internal",
			SourceID:   "heartbeat-source",
			Timestamp:  time.Now(),
			Handler:    "Engine",
		},
	}

	payload := &federation.EventPayload{Event: event}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	restored := &federation.EventPayload{}
	if err := json.Unmarshal(data, restored); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if restored.Schema() != schema {
		t.Errorf("Schema mismatch after round-trip")
	}
}
