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
		Provenance: federation.Provenance{
			SourceType: "git",
			SourceID:   "my-source",
			Timestamp:  now,
			Handler:    "GitHandler",
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
	if restored.Provenance.SourceType != original.Provenance.SourceType {
		t.Errorf("Provenance.SourceType = %q, want %q", restored.Provenance.SourceType, original.Provenance.SourceType)
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
