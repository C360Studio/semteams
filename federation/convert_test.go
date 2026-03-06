package federation_test

import (
	"testing"
	"time"

	"github.com/c360studio/semstreams/federation"
	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
)

func TestToEntityState_BasicTriples(t *testing.T) {
	now := time.Now()
	entity := federation.Entity{
		ID: "acme.platform.git.repo.commit.abc123",
		Triples: []message.Triple{
			{Subject: "acme.platform.git.repo.commit.abc123", Predicate: "git.commit.sha", Object: "abc123", Source: "git", Timestamp: now},
			{Subject: "acme.platform.git.repo.commit.abc123", Predicate: "git.commit.message", Object: "fix bug", Source: "git", Timestamp: now},
		},
		Provenance: federation.Provenance{SourceType: "git", SourceID: "my-source", Timestamp: now, Handler: "GitHandler"},
	}

	state := federation.ToEntityState(entity, federation.NewFederationMessageType())

	if state.ID != entity.ID {
		t.Errorf("ID = %q, want %q", state.ID, entity.ID)
	}
	// 2 original triples + 1 provenance triple
	if len(state.Triples) != 3 {
		t.Errorf("Triples len = %d, want 3", len(state.Triples))
	}
	if state.MessageType.Domain != "federation" {
		t.Errorf("MessageType.Domain = %q, want %q", state.MessageType.Domain, "federation")
	}
}

func TestToEntityState_EdgesConvertedToTriples(t *testing.T) {
	now := time.Now()
	entity := federation.Entity{
		ID: "acme.platform.git.repo.commit.abc123",
		Edges: []federation.Edge{
			{FromID: "acme.platform.git.repo.commit.abc123", ToID: "acme.platform.git.repo.author.alice", EdgeType: "authored_by", Weight: 1.0},
			{FromID: "acme.platform.git.repo.commit.abc123", ToID: "acme.platform.git.repo.file.main-go", EdgeType: "modifies"},
		},
		Provenance: federation.Provenance{SourceType: "git", SourceID: "my-source", Timestamp: now, Handler: "GitHandler"},
	}

	state := federation.ToEntityState(entity, federation.NewFederationMessageType())

	// 2 edge triples + 1 weight triple + 1 provenance triple = 4
	if len(state.Triples) != 4 {
		t.Errorf("Triples len = %d, want 4", len(state.Triples))
	}

	// Check edge triples
	foundAuthoredBy := false
	foundModifies := false
	foundWeight := false
	for _, tr := range state.Triples {
		switch tr.Predicate {
		case "federation.edge.authored_by":
			foundAuthoredBy = true
			if tr.Object != "acme.platform.git.repo.author.alice" {
				t.Errorf("authored_by Object = %v, want alice entity", tr.Object)
			}
		case "federation.edge.modifies":
			foundModifies = true
		case "federation.edge.authored_by.weight":
			foundWeight = true
			if tr.Object != 1.0 {
				t.Errorf("weight Object = %v, want 1.0", tr.Object)
			}
		}
	}
	if !foundAuthoredBy {
		t.Error("missing federation.edge.authored_by triple")
	}
	if !foundModifies {
		t.Error("missing federation.edge.modifies triple")
	}
	if !foundWeight {
		t.Error("missing federation.edge.authored_by.weight triple")
	}
}

func TestToEntityState_ZeroWeightNotEmitted(t *testing.T) {
	now := time.Now()
	entity := federation.Entity{
		ID: "acme.platform.git.repo.commit.abc123",
		Edges: []federation.Edge{
			{FromID: "acme.platform.git.repo.commit.abc123", ToID: "acme.platform.git.repo.file.main-go", EdgeType: "modifies", Weight: 0},
		},
		Provenance: federation.Provenance{SourceType: "git", SourceID: "my-source", Timestamp: now, Handler: "GitHandler"},
	}

	state := federation.ToEntityState(entity, federation.NewFederationMessageType())

	for _, tr := range state.Triples {
		if tr.Predicate == "federation.edge.modifies.weight" {
			t.Error("zero-weight edge should not emit a weight triple")
		}
	}
}

func TestFromEntityState_ExtractsEdges(t *testing.T) {
	now := time.Now()
	state := &graph.EntityState{
		ID: "acme.platform.git.repo.commit.abc123",
		Triples: []message.Triple{
			{Subject: "acme.platform.git.repo.commit.abc123", Predicate: "git.commit.sha", Object: "abc123", Source: "git", Timestamp: now},
			{Subject: "acme.platform.git.repo.commit.abc123", Predicate: "federation.edge.authored_by", Object: "acme.platform.git.repo.author.alice", Source: "federation", Timestamp: now},
			{Subject: "acme.platform.git.repo.commit.abc123", Predicate: "federation.edge.modifies", Object: "acme.platform.git.repo.file.main-go", Source: "federation", Timestamp: now},
			{Subject: "acme.platform.git.repo.commit.abc123", Predicate: "federation.provenance.source_type", Object: "git", Source: "federation", Timestamp: now},
		},
	}

	prov := federation.Provenance{SourceType: "git", SourceID: "my-source", Timestamp: now, Handler: "GitHandler"}
	entity := federation.FromEntityState(state, prov)

	if entity.ID != state.ID {
		t.Errorf("ID = %q, want %q", entity.ID, state.ID)
	}

	// 1 regular triple (git.commit.sha) — edge and provenance triples extracted
	if len(entity.Triples) != 1 {
		t.Errorf("Triples len = %d, want 1", len(entity.Triples))
	}
	if entity.Triples[0].Predicate != "git.commit.sha" {
		t.Errorf("Triple predicate = %q, want %q", entity.Triples[0].Predicate, "git.commit.sha")
	}

	// 2 edges
	if len(entity.Edges) != 2 {
		t.Errorf("Edges len = %d, want 2", len(entity.Edges))
	}
}

func TestRoundTrip_EntityToEntityStateAndBack(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)
	original := federation.Entity{
		ID: "acme.platform.git.repo.commit.abc123",
		Triples: []message.Triple{
			{Subject: "acme.platform.git.repo.commit.abc123", Predicate: "git.commit.sha", Object: "abc123", Source: "git", Timestamp: now, Confidence: 1.0},
		},
		Edges: []federation.Edge{
			{FromID: "acme.platform.git.repo.commit.abc123", ToID: "acme.platform.git.repo.author.alice", EdgeType: "authored_by"},
			{FromID: "acme.platform.git.repo.commit.abc123", ToID: "acme.platform.git.repo.file.main-go", EdgeType: "modifies"},
		},
		Provenance: federation.Provenance{SourceType: "git", SourceID: "my-source", Timestamp: now, Handler: "GitHandler"},
	}

	// Entity → EntityState
	state := federation.ToEntityState(original, federation.NewFederationMessageType())

	// EntityState → Entity
	restored := federation.FromEntityState(state, original.Provenance)

	// Verify ID
	if restored.ID != original.ID {
		t.Errorf("ID = %q, want %q", restored.ID, original.ID)
	}

	// Verify triples (only regular triples survive round-trip)
	if len(restored.Triples) != len(original.Triples) {
		t.Errorf("Triples len = %d, want %d", len(restored.Triples), len(original.Triples))
	}

	// Verify edges
	if len(restored.Edges) != len(original.Edges) {
		t.Errorf("Edges len = %d, want %d", len(restored.Edges), len(original.Edges))
	}
	edgeSet := make(map[string]bool)
	for _, e := range restored.Edges {
		edgeSet[e.EdgeType] = true
	}
	if !edgeSet["authored_by"] {
		t.Error("missing authored_by edge after round-trip")
	}
	if !edgeSet["modifies"] {
		t.Error("missing modifies edge after round-trip")
	}
}

func TestToEntityState_EmptyEntity(t *testing.T) {
	entity := federation.Entity{
		ID:         "acme.platform.domain.system.type.instance",
		Provenance: federation.Provenance{SourceType: "test", SourceID: "test", Timestamp: time.Now(), Handler: "test"},
	}

	state := federation.ToEntityState(entity, federation.NewFederationMessageType())

	if state.ID != entity.ID {
		t.Errorf("ID = %q, want %q", state.ID, entity.ID)
	}
	// Only provenance triple
	if len(state.Triples) != 1 {
		t.Errorf("Triples len = %d, want 1", len(state.Triples))
	}
}

func TestFromEntityState_NoEdgeTriples(t *testing.T) {
	state := &graph.EntityState{
		ID: "acme.platform.git.repo.commit.abc123",
		Triples: []message.Triple{
			{Subject: "acme.platform.git.repo.commit.abc123", Predicate: "git.commit.sha", Object: "abc123"},
			{Subject: "acme.platform.git.repo.commit.abc123", Predicate: "git.commit.author", Object: "alice"},
		},
	}

	entity := federation.FromEntityState(state, federation.Provenance{})

	if len(entity.Triples) != 2 {
		t.Errorf("Triples len = %d, want 2", len(entity.Triples))
	}
	if len(entity.Edges) != 0 {
		t.Errorf("Edges len = %d, want 0", len(entity.Edges))
	}
}

func TestFromEntityState_EmptyState(t *testing.T) {
	state := &graph.EntityState{
		ID: "acme.platform.domain.system.type.instance",
	}

	entity := federation.FromEntityState(state, federation.Provenance{})

	if entity.ID != state.ID {
		t.Errorf("ID = %q, want %q", entity.ID, state.ID)
	}
	if entity.Triples != nil {
		t.Errorf("Triples should be nil for empty state, got %v", entity.Triples)
	}
	if entity.Edges != nil {
		t.Errorf("Edges should be nil for empty state, got %v", entity.Edges)
	}
}

func TestFromEntityState_ExtractsEdgeWeights(t *testing.T) {
	now := time.Now()
	state := &graph.EntityState{
		ID: "acme.platform.git.repo.commit.abc123",
		Triples: []message.Triple{
			{Subject: "acme.platform.git.repo.commit.abc123", Predicate: "federation.edge.authored_by",
				Object: "acme.platform.git.repo.author.alice", Source: "federation", Timestamp: now},
			{Subject: "acme.platform.git.repo.commit.abc123", Predicate: "federation.edge.authored_by.weight",
				Object: 0.75, Source: "federation", Timestamp: now},
		},
	}

	entity := federation.FromEntityState(state, federation.Provenance{})
	if len(entity.Edges) != 1 {
		t.Fatalf("Edges len = %d, want 1", len(entity.Edges))
	}
	if entity.Edges[0].Weight != 0.75 {
		t.Errorf("Edge weight = %f, want 0.75", entity.Edges[0].Weight)
	}
}

func TestRoundTrip_WeightedEdge(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)
	original := federation.Entity{
		ID: "acme.platform.git.repo.commit.abc123",
		Edges: []federation.Edge{
			{FromID: "acme.platform.git.repo.commit.abc123", ToID: "acme.platform.git.repo.author.alice", EdgeType: "authored_by", Weight: 0.9},
		},
		Provenance: federation.Provenance{SourceType: "git", SourceID: "my-source", Timestamp: now, Handler: "GitHandler"},
	}

	state := federation.ToEntityState(original, federation.NewFederationMessageType())
	restored := federation.FromEntityState(state, original.Provenance)

	if len(restored.Edges) != 1 {
		t.Fatalf("Edges len = %d, want 1", len(restored.Edges))
	}
	if restored.Edges[0].Weight != 0.9 {
		t.Errorf("Edge weight = %f, want 0.9", restored.Edges[0].Weight)
	}
	if restored.Edges[0].EdgeType != "authored_by" {
		t.Errorf("Edge type = %q, want %q", restored.Edges[0].EdgeType, "authored_by")
	}
}

func TestToEntityState_EmptyProvenance(t *testing.T) {
	entity := federation.Entity{
		ID: "acme.platform.domain.system.type.instance",
		Triples: []message.Triple{
			{Subject: "acme.platform.domain.system.type.instance", Predicate: "test.key", Object: "value"},
		},
	}

	state := federation.ToEntityState(entity, federation.NewFederationMessageType())

	// With empty SourceType, no provenance triple emitted
	if len(state.Triples) != 1 {
		t.Errorf("Triples len = %d, want 1 (no provenance triple for empty SourceType)", len(state.Triples))
	}
}
