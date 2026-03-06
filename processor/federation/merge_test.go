package federation_test

import (
	"testing"
	"time"

	"github.com/c360studio/semstreams/federation"
	proc "github.com/c360studio/semstreams/processor/federation"
)

// --- helpers ---

func entity(id string) federation.Entity {
	return federation.Entity{
		ID: id,
		Provenance: federation.Provenance{
			SourceType: "test",
			SourceID:   "test-source",
			Timestamp:  time.Now(),
			Handler:    "test",
		},
	}
}

func prov(sourceID string) federation.Provenance {
	return federation.Provenance{
		SourceType: "git",
		SourceID:   sourceID,
		Timestamp:  time.Now(),
		Handler:    "test",
	}
}

func seedEvent(ns string, entities ...federation.Entity) *federation.Event {
	return &federation.Event{
		Type:       federation.EventTypeSEED,
		SourceID:   "test-source",
		Namespace:  ns,
		Timestamp:  time.Now(),
		Entities:   entities,
		Provenance: prov("test-source"),
	}
}

func deltaEvent(ns string, entities ...federation.Entity) *federation.Event {
	return &federation.Event{
		Type:       federation.EventTypeDELTA,
		SourceID:   "test-source",
		Namespace:  ns,
		Timestamp:  time.Now(),
		Entities:   entities,
		Provenance: prov("test-source"),
	}
}

func retractEvent(ns string, ids ...string) *federation.Event {
	return &federation.Event{
		Type:        federation.EventTypeRETRACT,
		SourceID:    "test-source",
		Namespace:   ns,
		Timestamp:   time.Now(),
		Retractions: ids,
		Provenance:  prov("test-source"),
	}
}

// entityOrg extracts the org segment (first part) of a 6-part entity ID.
func entityOrg(id string) string {
	for i, ch := range id {
		if ch == '.' {
			return id[:i]
		}
	}
	return id
}

// --- Config tests ---

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     proc.Config
		wantErr bool
	}{
		{
			name:    "valid config",
			cfg:     proc.Config{LocalNamespace: "acme", MergePolicy: proc.MergePolicyStandard, InputSubject: "fed.events", OutputSubject: "fed.merged", InputStream: "FED_IN", OutputStream: "FED_OUT"},
			wantErr: false,
		},
		{
			name:    "missing namespace",
			cfg:     proc.Config{MergePolicy: proc.MergePolicyStandard, InputSubject: "fed.events", OutputSubject: "fed.merged", InputStream: "FED_IN", OutputStream: "FED_OUT"},
			wantErr: true,
		},
		{
			name:    "invalid merge policy",
			cfg:     proc.Config{LocalNamespace: "acme", MergePolicy: "bogus", InputSubject: "fed.events", OutputSubject: "fed.merged", InputStream: "FED_IN", OutputStream: "FED_OUT"},
			wantErr: true,
		},
		{
			name:    "missing input_subject",
			cfg:     proc.Config{LocalNamespace: "acme", MergePolicy: proc.MergePolicyStandard, OutputSubject: "fed.merged", InputStream: "FED_IN", OutputStream: "FED_OUT"},
			wantErr: true,
		},
		{
			name:    "missing output_subject",
			cfg:     proc.Config{LocalNamespace: "acme", MergePolicy: proc.MergePolicyStandard, InputSubject: "fed.events", InputStream: "FED_IN", OutputStream: "FED_OUT"},
			wantErr: true,
		},
		{
			name:    "missing input_stream",
			cfg:     proc.Config{LocalNamespace: "acme", MergePolicy: proc.MergePolicyStandard, InputSubject: "fed.events", OutputSubject: "fed.merged", OutputStream: "FED_OUT"},
			wantErr: true,
		},
		{
			name:    "missing output_stream",
			cfg:     proc.Config{LocalNamespace: "acme", MergePolicy: proc.MergePolicyStandard, InputSubject: "fed.events", OutputSubject: "fed.merged", InputStream: "FED_IN"},
			wantErr: true,
		},
		{
			name:    "default config is valid",
			cfg:     proc.DefaultConfig(),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := proc.DefaultConfig()
	if cfg.LocalNamespace == "" {
		t.Error("DefaultConfig().LocalNamespace must not be empty")
	}
	if cfg.MergePolicy == "" {
		t.Error("DefaultConfig().MergePolicy must not be empty")
	}
	if cfg.InputSubject == "" {
		t.Error("DefaultConfig().InputSubject must not be empty")
	}
	if cfg.OutputSubject == "" {
		t.Error("DefaultConfig().OutputSubject must not be empty")
	}
	if cfg.Ports == nil {
		t.Fatal("DefaultConfig().Ports must not be nil")
	}
	if len(cfg.Ports.Inputs) != 1 {
		t.Errorf("DefaultConfig() input ports = %d, want 1", len(cfg.Ports.Inputs))
	}
	if len(cfg.Ports.Outputs) != 1 {
		t.Errorf("DefaultConfig() output ports = %d, want 1", len(cfg.Ports.Outputs))
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("DefaultConfig().Validate() = %v, want nil", err)
	}
}

// --- MergeEntity tests ---

func TestMergeEntity_PublicMergesUnconditionally(t *testing.T) {
	cfg := proc.Config{LocalNamespace: "acme", MergePolicy: proc.MergePolicyStandard, InputSubject: "x", OutputSubject: "x", InputStream: "X", OutputStream: "X"}
	merger := proc.NewMerger(cfg)

	incoming := entity("public.platform.golang.stdlib-net-http.function.ListenAndServe")

	result, err := merger.MergeEntity(incoming, nil)
	if err != nil {
		t.Fatalf("MergeEntity public.* should not error: %v", err)
	}
	if result == nil {
		t.Fatal("MergeEntity public.* should return merged entity")
	}
	if result.ID != incoming.ID {
		t.Errorf("ID = %q, want %q", result.ID, incoming.ID)
	}
}

func TestMergeEntity_PublicMergesFromAnyOrg(t *testing.T) {
	cfg := proc.Config{LocalNamespace: "acme", MergePolicy: proc.MergePolicyStandard, InputSubject: "x", OutputSubject: "x", InputStream: "X", OutputStream: "X"}
	merger := proc.NewMerger(cfg)

	incoming := entity("public.platform.web.pkg-go-dev.doc.c821de")
	result, err := merger.MergeEntity(incoming, nil)
	if err != nil {
		t.Fatalf("MergeEntity public.* URL entity: %v", err)
	}
	if result == nil {
		t.Fatal("expected merged entity")
	}
}

func TestMergeEntity_OwnOrgMerges(t *testing.T) {
	cfg := proc.Config{LocalNamespace: "acme", MergePolicy: proc.MergePolicyStandard, InputSubject: "x", OutputSubject: "x", InputStream: "X", OutputStream: "X"}
	merger := proc.NewMerger(cfg)

	incoming := entity("acme.platform.golang.github-com-acme-gcs.function.NewController")
	result, err := merger.MergeEntity(incoming, nil)
	if err != nil {
		t.Fatalf("MergeEntity own org entity: %v", err)
	}
	if result == nil {
		t.Fatal("expected merged entity")
	}
}

func TestMergeEntity_CrossOrgRejected(t *testing.T) {
	cfg := proc.Config{LocalNamespace: "acme", MergePolicy: proc.MergePolicyStandard, InputSubject: "x", OutputSubject: "x", InputStream: "X", OutputStream: "X"}
	merger := proc.NewMerger(cfg)

	incoming := entity("other.platform.golang.github-com-other-lib.function.DoSomething")
	result, err := merger.MergeEntity(incoming, nil)
	if err == nil {
		t.Error("expected error for cross-org entity overwrite")
	}
	if result != nil {
		t.Error("result should be nil for rejected cross-org entity")
	}
}

func TestMergeEntity_ProvenanceAppended(t *testing.T) {
	cfg := proc.Config{LocalNamespace: "acme", MergePolicy: proc.MergePolicyStandard, InputSubject: "x", OutputSubject: "x", InputStream: "X", OutputStream: "X"}
	merger := proc.NewMerger(cfg)

	existingProv := federation.Provenance{SourceType: "git", SourceID: "source-a", Timestamp: time.Now(), Handler: "git"}
	existingEntity := federation.Entity{
		ID:         "public.platform.golang.stdlib.function.Foo",
		Provenance: existingProv,
	}

	incomingProv := federation.Provenance{SourceType: "ast", SourceID: "source-b", Timestamp: time.Now(), Handler: "ast"}
	incomingEntity := federation.Entity{
		ID:         "public.platform.golang.stdlib.function.Foo",
		Provenance: incomingProv,
	}

	result, err := merger.MergeEntity(incomingEntity, &existingEntity)
	if err != nil {
		t.Fatalf("MergeEntity: %v", err)
	}
	if result == nil {
		t.Fatal("expected merged entity")
	}

	if len(result.AdditionalProvenance) == 0 {
		t.Error("expected AdditionalProvenance to contain prior provenance records after merge")
	}
	found := false
	for _, ap := range result.AdditionalProvenance {
		if ap.SourceID == existingProv.SourceID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("existing provenance (source-a) not found in AdditionalProvenance")
	}
}

func TestMergeEntity_ProvenanceAppendedChain(t *testing.T) {
	cfg := proc.Config{LocalNamespace: "acme", MergePolicy: proc.MergePolicyStandard, InputSubject: "x", OutputSubject: "x", InputStream: "X", OutputStream: "X"}
	merger := proc.NewMerger(cfg)

	existingEntity := federation.Entity{
		ID:         "public.platform.golang.stdlib.function.Foo",
		Provenance: prov("source-b"),
		AdditionalProvenance: []federation.Provenance{
			{SourceType: "git", SourceID: "source-a", Timestamp: time.Now(), Handler: "git"},
		},
	}

	incomingEntity := federation.Entity{
		ID:         "public.platform.golang.stdlib.function.Foo",
		Provenance: prov("source-c"),
	}

	result, err := merger.MergeEntity(incomingEntity, &existingEntity)
	if err != nil {
		t.Fatalf("MergeEntity: %v", err)
	}

	if len(result.AdditionalProvenance) < 2 {
		t.Errorf("expected at least 2 additional provenance records, got %d", len(result.AdditionalProvenance))
	}
}

func TestMergeEntity_EdgeUnion(t *testing.T) {
	cfg := proc.Config{LocalNamespace: "acme", MergePolicy: proc.MergePolicyStandard, InputSubject: "x", OutputSubject: "x", InputStream: "X", OutputStream: "X"}
	merger := proc.NewMerger(cfg)

	existingEdge := federation.Edge{FromID: "public.a.b.c.d.e", ToID: "public.a.b.c.d.f", EdgeType: "calls"}
	existingEntity := federation.Entity{
		ID:         "public.platform.golang.stdlib.function.Foo",
		Edges:      []federation.Edge{existingEdge},
		Provenance: prov("s1"),
	}

	newEdge := federation.Edge{FromID: "public.a.b.c.d.e", ToID: "public.a.b.c.d.g", EdgeType: "imports"}
	incomingEntity := federation.Entity{
		ID:         "public.platform.golang.stdlib.function.Foo",
		Edges:      []federation.Edge{newEdge},
		Provenance: prov("s2"),
	}

	result, err := merger.MergeEntity(incomingEntity, &existingEntity)
	if err != nil {
		t.Fatalf("MergeEntity: %v", err)
	}

	edgeSet := make(map[string]bool)
	for _, e := range result.Edges {
		edgeSet[e.ToID+":"+e.EdgeType] = true
	}
	if !edgeSet["public.a.b.c.d.f:calls"] {
		t.Error("existing edge should be preserved in union")
	}
	if !edgeSet["public.a.b.c.d.g:imports"] {
		t.Error("new edge should be added in union")
	}
}

func TestMergeEntity_EdgeUnionDeduplicates(t *testing.T) {
	cfg := proc.Config{LocalNamespace: "acme", MergePolicy: proc.MergePolicyStandard, InputSubject: "x", OutputSubject: "x", InputStream: "X", OutputStream: "X"}
	merger := proc.NewMerger(cfg)

	sameEdge := federation.Edge{FromID: "public.a.b.c.d.e", ToID: "public.a.b.c.d.f", EdgeType: "calls"}
	existingEntity := federation.Entity{
		ID:         "public.platform.golang.stdlib.function.Foo",
		Edges:      []federation.Edge{sameEdge},
		Provenance: prov("s1"),
	}
	incomingEntity := federation.Entity{
		ID:         "public.platform.golang.stdlib.function.Foo",
		Edges:      []federation.Edge{sameEdge},
		Provenance: prov("s2"),
	}

	result, err := merger.MergeEntity(incomingEntity, &existingEntity)
	if err != nil {
		t.Fatalf("MergeEntity: %v", err)
	}
	if len(result.Edges) != 1 {
		t.Errorf("expected 1 edge after dedup union, got %d", len(result.Edges))
	}
}

func TestMergeEntity_EmptyID(t *testing.T) {
	cfg := proc.Config{LocalNamespace: "acme", MergePolicy: proc.MergePolicyStandard, InputSubject: "x", OutputSubject: "x", InputStream: "X", OutputStream: "X"}
	merger := proc.NewMerger(cfg)

	incoming := entity("nodots")
	_, err := merger.MergeEntity(incoming, nil)
	if err == nil {
		t.Error("expected error for entity ID with no org segment")
	}
}

// --- ApplyEvent tests ---

func TestApplyEvent_SEEDPublicEntitiesAccepted(t *testing.T) {
	cfg := proc.Config{LocalNamespace: "acme", MergePolicy: proc.MergePolicyStandard, InputSubject: "x", OutputSubject: "x", InputStream: "X", OutputStream: "X"}
	merger := proc.NewMerger(cfg)

	ev := seedEvent("public",
		entity("public.platform.golang.stdlib.function.Foo"),
		entity("public.platform.golang.stdlib.function.Bar"),
	)

	result, err := merger.ApplyEvent(ev, nil)
	if err != nil {
		t.Fatalf("ApplyEvent SEED public: %v", err)
	}
	if len(result.Entities) != 2 {
		t.Errorf("expected 2 entities, got %d", len(result.Entities))
	}
}

func TestApplyEvent_DELTAOwnOrgAccepted(t *testing.T) {
	cfg := proc.Config{LocalNamespace: "acme", MergePolicy: proc.MergePolicyStandard, InputSubject: "x", OutputSubject: "x", InputStream: "X", OutputStream: "X"}
	merger := proc.NewMerger(cfg)

	ev := deltaEvent("acme",
		entity("acme.platform.golang.github-com-acme-repo.function.DoWork"),
	)

	result, err := merger.ApplyEvent(ev, nil)
	if err != nil {
		t.Fatalf("ApplyEvent DELTA own org: %v", err)
	}
	if len(result.Entities) != 1 {
		t.Errorf("expected 1 entity, got %d", len(result.Entities))
	}
}

func TestApplyEvent_CrossOrgEntitiesFilteredOut(t *testing.T) {
	cfg := proc.Config{LocalNamespace: "acme", MergePolicy: proc.MergePolicyStandard, InputSubject: "x", OutputSubject: "x", InputStream: "X", OutputStream: "X"}
	merger := proc.NewMerger(cfg)

	ev := deltaEvent("mixed",
		entity("acme.platform.golang.github-com-acme-repo.function.Mine"),
		entity("other.platform.golang.github-com-other-repo.function.Theirs"),
		entity("public.platform.golang.stdlib.function.Shared"),
	)

	result, err := merger.ApplyEvent(ev, nil)
	if err != nil {
		t.Fatalf("ApplyEvent mixed: %v", err)
	}
	if len(result.Entities) != 2 {
		t.Errorf("expected 2 entities (own+public), got %d", len(result.Entities))
	}
	for _, e := range result.Entities {
		org := entityOrg(e.ID)
		if org != "acme" && org != "public" {
			t.Errorf("cross-org entity leaked through: %s", e.ID)
		}
	}
}

func TestApplyEvent_RETRACTWithinOwnScope(t *testing.T) {
	cfg := proc.Config{LocalNamespace: "acme", MergePolicy: proc.MergePolicyStandard, InputSubject: "x", OutputSubject: "x", InputStream: "X", OutputStream: "X"}
	merger := proc.NewMerger(cfg)

	ev := retractEvent("acme",
		"acme.platform.golang.github-com-acme-repo.function.OldFunc",
		"public.platform.golang.stdlib.function.OldPublic",
	)

	result, err := merger.ApplyEvent(ev, nil)
	if err != nil {
		t.Fatalf("ApplyEvent RETRACT own scope: %v", err)
	}
	if len(result.Retractions) != 2 {
		t.Errorf("expected 2 retractions, got %d", len(result.Retractions))
	}
}

func TestApplyEvent_RETRACTCrossOrgRejected(t *testing.T) {
	cfg := proc.Config{LocalNamespace: "acme", MergePolicy: proc.MergePolicyStandard, InputSubject: "x", OutputSubject: "x", InputStream: "X", OutputStream: "X"}
	merger := proc.NewMerger(cfg)

	ev := retractEvent("other",
		"other.platform.golang.github-com-other-repo.function.TheirFunc",
		"acme.platform.golang.github-com-acme-repo.function.MyFunc",
	)

	result, err := merger.ApplyEvent(ev, nil)
	if err != nil {
		t.Fatalf("ApplyEvent RETRACT cross-org: %v", err)
	}
	if len(result.Retractions) != 1 {
		t.Errorf("expected 1 retraction (cross-org filtered), got %d", len(result.Retractions))
	}
	if result.Retractions[0] != "acme.platform.golang.github-com-acme-repo.function.MyFunc" {
		t.Errorf("unexpected retraction ID: %s", result.Retractions[0])
	}
}

func TestApplyEvent_RETRACTPublicPasses(t *testing.T) {
	cfg := proc.Config{LocalNamespace: "acme", MergePolicy: proc.MergePolicyStandard, InputSubject: "x", OutputSubject: "x", InputStream: "X", OutputStream: "X"}
	merger := proc.NewMerger(cfg)

	ev := retractEvent("public",
		"public.platform.golang.stdlib.function.DeprecatedFunc",
	)

	result, err := merger.ApplyEvent(ev, nil)
	if err != nil {
		t.Fatalf("ApplyEvent RETRACT public: %v", err)
	}
	if len(result.Retractions) != 1 {
		t.Errorf("expected 1 retraction, got %d", len(result.Retractions))
	}
}

func TestApplyEvent_HEARTBEATPassesThrough(t *testing.T) {
	cfg := proc.Config{LocalNamespace: "acme", MergePolicy: proc.MergePolicyStandard, InputSubject: "x", OutputSubject: "x", InputStream: "X", OutputStream: "X"}
	merger := proc.NewMerger(cfg)

	ev := &federation.Event{
		Type:       federation.EventTypeHEARTBEAT,
		SourceID:   "test",
		Namespace:  "acme",
		Timestamp:  time.Now(),
		Provenance: prov("test"),
	}

	result, err := merger.ApplyEvent(ev, nil)
	if err != nil {
		t.Fatalf("ApplyEvent HEARTBEAT: %v", err)
	}
	if result.Type != federation.EventTypeHEARTBEAT {
		t.Errorf("Type = %q, want %q", result.Type, federation.EventTypeHEARTBEAT)
	}
}

func TestApplyEvent_NilEventReturnsError(t *testing.T) {
	cfg := proc.Config{LocalNamespace: "acme", MergePolicy: proc.MergePolicyStandard, InputSubject: "x", OutputSubject: "x", InputStream: "X", OutputStream: "X"}
	merger := proc.NewMerger(cfg)

	_, err := merger.ApplyEvent(nil, nil)
	if err == nil {
		t.Error("expected error for nil event")
	}
}

func TestApplyEvent_WithExistingStore_EdgeUnion(t *testing.T) {
	cfg := proc.Config{LocalNamespace: "acme", MergePolicy: proc.MergePolicyStandard, InputSubject: "x", OutputSubject: "x", InputStream: "X", OutputStream: "X"}
	merger := proc.NewMerger(cfg)

	existingEdge := federation.Edge{FromID: "public.a.b.c.d.e", ToID: "public.a.b.c.d.f", EdgeType: "calls"}
	existing := map[string]*federation.Entity{
		"public.platform.golang.stdlib.function.Foo": {
			ID:         "public.platform.golang.stdlib.function.Foo",
			Edges:      []federation.Edge{existingEdge},
			Provenance: prov("old-source"),
		},
	}

	newEdge := federation.Edge{FromID: "public.a.b.c.d.e", ToID: "public.a.b.c.d.g", EdgeType: "imports"}
	ev := deltaEvent("public",
		federation.Entity{
			ID:         "public.platform.golang.stdlib.function.Foo",
			Edges:      []federation.Edge{newEdge},
			Provenance: prov("new-source"),
		},
	)

	result, err := merger.ApplyEvent(ev, existing)
	if err != nil {
		t.Fatalf("ApplyEvent with store: %v", err)
	}
	if len(result.Entities) != 1 {
		t.Fatalf("expected 1 entity, got %d", len(result.Entities))
	}

	merged := result.Entities[0]
	edgeSet := make(map[string]bool)
	for _, e := range merged.Edges {
		edgeSet[e.ToID+":"+e.EdgeType] = true
	}
	if !edgeSet["public.a.b.c.d.f:calls"] {
		t.Error("existing edge should be preserved")
	}
	if !edgeSet["public.a.b.c.d.g:imports"] {
		t.Error("new edge should be added")
	}
}

func TestApplyEvent_SEEDEmptyEntities(t *testing.T) {
	cfg := proc.Config{LocalNamespace: "acme", MergePolicy: proc.MergePolicyStandard, InputSubject: "x", OutputSubject: "x", InputStream: "X", OutputStream: "X"}
	merger := proc.NewMerger(cfg)

	ev := seedEvent("acme")
	result, err := merger.ApplyEvent(ev, nil)
	if err != nil {
		t.Fatalf("ApplyEvent empty SEED: %v", err)
	}
	if len(result.Entities) != 0 {
		t.Errorf("expected 0 entities, got %d", len(result.Entities))
	}
}
