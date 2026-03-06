package federation_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/c360studio/semstreams/federation"
	"github.com/c360studio/semstreams/message"
)

func TestEntity_JSONRoundTrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)

	original := federation.Entity{
		ID: "acme.platform.git.my-repo.commit.a1b2c3",
		Triples: []message.Triple{
			{
				Subject:   "acme.platform.git.my-repo.commit.a1b2c3",
				Predicate: "git.commit.sha",
				Object:    "a1b2c3",
				Source:    "git",
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
		AdditionalProvenance: []federation.Provenance{
			{
				SourceType: "merge",
				SourceID:   "federation",
				Timestamp:  now,
				Handler:    "FederationProcessor",
			},
		},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var restored federation.Entity
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if restored.ID != original.ID {
		t.Errorf("ID = %q, want %q", restored.ID, original.ID)
	}
	if len(restored.Triples) != len(original.Triples) {
		t.Errorf("Triples len = %d, want %d", len(restored.Triples), len(original.Triples))
	}
	if len(restored.Edges) != len(original.Edges) {
		t.Errorf("Edges len = %d, want %d", len(restored.Edges), len(original.Edges))
	}
	if restored.Edges[0].EdgeType != "authored_by" {
		t.Errorf("Edge type = %q, want %q", restored.Edges[0].EdgeType, "authored_by")
	}
	if restored.Provenance.SourceType != original.Provenance.SourceType {
		t.Errorf("Provenance.SourceType = %q, want %q", restored.Provenance.SourceType, original.Provenance.SourceType)
	}
	if len(restored.AdditionalProvenance) != 1 {
		t.Errorf("AdditionalProvenance len = %d, want 1", len(restored.AdditionalProvenance))
	}
}

func TestEntity_JSONRoundTrip_MinimalEntity(t *testing.T) {
	original := federation.Entity{
		ID: "acme.platform.domain.system.type.instance",
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var restored federation.Entity
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if restored.ID != original.ID {
		t.Errorf("ID = %q, want %q", restored.ID, original.ID)
	}
	if restored.Edges != nil {
		t.Errorf("Edges should be nil for omitempty, got %v", restored.Edges)
	}
	if restored.AdditionalProvenance != nil {
		t.Errorf("AdditionalProvenance should be nil for omitempty, got %v", restored.AdditionalProvenance)
	}
}

func TestEdge_JSONRoundTrip_WithProperties(t *testing.T) {
	original := federation.Edge{
		FromID:   "acme.platform.git.repo.commit.abc",
		ToID:     "acme.platform.git.repo.file.main-go",
		EdgeType: "modifies",
		Weight:   0.75,
		Properties: map[string]any{
			"lines_added":   float64(42),
			"lines_removed": float64(10),
		},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var restored federation.Edge
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if restored.FromID != original.FromID {
		t.Errorf("FromID = %q, want %q", restored.FromID, original.FromID)
	}
	if restored.Weight != original.Weight {
		t.Errorf("Weight = %f, want %f", restored.Weight, original.Weight)
	}
	if restored.Properties["lines_added"] != float64(42) {
		t.Errorf("Properties[lines_added] = %v, want 42", restored.Properties["lines_added"])
	}
}

func TestProvenance_JSONRoundTrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)

	original := federation.Provenance{
		SourceType: "git",
		SourceID:   "my-source-123",
		Timestamp:  now,
		Handler:    "GitHandler",
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var restored federation.Provenance
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if restored != original {
		t.Errorf("Provenance round-trip mismatch: got %v, want %v", restored, original)
	}
}
