package federation_test

import (
	"sync"
	"testing"
	"time"

	"github.com/c360studio/semstreams/federation"
	"github.com/c360studio/semstreams/message"
)

func makeEntity(id string, triples ...message.Triple) *federation.Entity {
	return &federation.Entity{
		ID:      id,
		Triples: triples,
		Provenance: federation.Provenance{
			SourceType: "test",
			SourceID:   "test-source",
			Timestamp:  time.Now(),
			Handler:    "TestHandler",
		},
	}
}

func TestStore_Upsert_NewEntity(t *testing.T) {
	s := federation.NewStore()

	entity := makeEntity("acme.platform.git.repo.commit.a1b2c3",
		message.Triple{Subject: "acme.platform.git.repo.commit.a1b2c3", Predicate: "git.sha", Object: "a1b2c3"},
	)

	changed := s.Upsert(entity)
	if !changed {
		t.Error("Upsert() should return true for new entity")
	}
	if s.Count() != 1 {
		t.Errorf("Count() = %d, want 1", s.Count())
	}
}

func TestStore_Upsert_UnchangedEntity(t *testing.T) {
	s := federation.NewStore()

	entity := makeEntity("acme.platform.git.repo.commit.a1b2c3",
		message.Triple{Subject: "acme.platform.git.repo.commit.a1b2c3", Predicate: "git.sha", Object: "a1b2c3"},
	)

	s.Upsert(entity)
	changed := s.Upsert(entity)
	if changed {
		t.Error("Upsert() should return false when entity is unchanged")
	}
	if s.Count() != 1 {
		t.Errorf("Count() = %d, want 1", s.Count())
	}
}

func TestStore_Upsert_PropertiesIsolated(t *testing.T) {
	s := federation.NewStore()

	e := &federation.Entity{
		ID: "acme.platform.git.repo.commit.abc",
		Edges: []federation.Edge{
			{FromID: "a.b.c.d.e.f", ToID: "a.b.c.d.e.g", EdgeType: "calls",
				Properties: map[string]any{"key": "original"}},
		},
		Provenance: federation.Provenance{SourceType: "test", SourceID: "test", Timestamp: time.Now(), Handler: "test"},
	}
	s.Upsert(e)

	// External mutation after store
	e.Edges[0].Properties["key"] = "mutated"

	got := s.Get("acme.platform.git.repo.commit.abc")
	if got.Edges[0].Properties["key"] != "original" {
		t.Error("Store should isolate Properties from external mutation")
	}
}

func TestStore_Upsert_ChangedEntity(t *testing.T) {
	s := federation.NewStore()

	id := "acme.platform.git.repo.commit.a1b2c3"
	entity1 := makeEntity(id,
		message.Triple{Subject: id, Predicate: "git.sha", Object: "a1b2c3"},
	)
	entity2 := makeEntity(id,
		message.Triple{Subject: id, Predicate: "git.sha", Object: "a1b2c3"},
		message.Triple{Subject: id, Predicate: "git.message", Object: "fix: bug"},
	)

	s.Upsert(entity1)
	changed := s.Upsert(entity2)
	if !changed {
		t.Error("Upsert() should return true when entity content changes")
	}
	if s.Count() != 1 {
		t.Errorf("Count() = %d, want 1", s.Count())
	}
}

func TestStore_Upsert_NilEntity(t *testing.T) {
	s := federation.NewStore()
	changed := s.Upsert(nil)
	if changed {
		t.Error("Upsert(nil) should return false")
	}
}

func TestStore_Get_Existing(t *testing.T) {
	s := federation.NewStore()

	id := "acme.platform.git.repo.commit.a1b2c3"
	entity := makeEntity(id,
		message.Triple{Subject: id, Predicate: "git.sha", Object: "a1b2c3"},
	)
	s.Upsert(entity)

	got := s.Get(id)
	if got == nil {
		t.Fatal("Get() should return entity")
	}
	if got.ID != id {
		t.Errorf("Get().ID = %q, want %q", got.ID, id)
	}

	// Verify it's a copy
	got.ID = "mutated"
	got2 := s.Get(id)
	if got2.ID != id {
		t.Error("Get() should return a copy, not a reference")
	}
}

func TestStore_Get_NonExistent(t *testing.T) {
	s := federation.NewStore()
	got := s.Get("nonexistent.id.here.foo.bar.baz")
	if got != nil {
		t.Error("Get() should return nil for non-existent entity")
	}
}

func TestStore_Remove_Existing(t *testing.T) {
	s := federation.NewStore()

	id := "acme.platform.git.repo.commit.a1b2c3"
	s.Upsert(makeEntity(id))

	removed := s.Remove(id)
	if !removed {
		t.Error("Remove() should return true for existing entity")
	}
	if s.Count() != 0 {
		t.Errorf("Count() = %d, want 0", s.Count())
	}
}

func TestStore_Remove_NonExistent(t *testing.T) {
	s := federation.NewStore()

	removed := s.Remove("nonexistent.id.here.foo.bar.baz")
	if removed {
		t.Error("Remove() should return false for non-existent entity")
	}
}

func TestStore_Snapshot(t *testing.T) {
	s := federation.NewStore()

	entities := []*federation.Entity{
		makeEntity("acme.platform.git.repo.commit.a1b2c3"),
		makeEntity("acme.platform.git.repo.commit.d4e5f6"),
		makeEntity("acme.platform.git.repo.author.alice"),
	}

	for _, e := range entities {
		s.Upsert(e)
	}

	snap := s.Snapshot()
	if len(snap) != 3 {
		t.Errorf("Snapshot() len = %d, want 3", len(snap))
	}

	// Verify snapshot is independent copy — mutations don't affect store
	snap[0].ID = "mutated"
	snap2 := s.Snapshot()
	for _, e := range snap2 {
		if e.ID == "mutated" {
			t.Error("Snapshot() should return copies, not references")
		}
	}
}

func TestStore_Snapshot_Empty(t *testing.T) {
	s := federation.NewStore()
	snap := s.Snapshot()
	if snap == nil {
		t.Error("Snapshot() should return empty slice, not nil")
	}
	if len(snap) != 0 {
		t.Errorf("Snapshot() len = %d, want 0", len(snap))
	}
}

func TestStore_SnapshotMap(t *testing.T) {
	s := federation.NewStore()

	id1 := "acme.platform.git.repo.commit.a1b2c3"
	id2 := "acme.platform.git.repo.commit.d4e5f6"
	s.Upsert(makeEntity(id1))
	s.Upsert(makeEntity(id2))

	m := s.SnapshotMap()
	if len(m) != 2 {
		t.Fatalf("SnapshotMap() len = %d, want 2", len(m))
	}
	if m[id1] == nil {
		t.Errorf("SnapshotMap() missing entity %q", id1)
	}
	if m[id2] == nil {
		t.Errorf("SnapshotMap() missing entity %q", id2)
	}

	// Verify it's a copy
	m[id1].ID = "mutated"
	m2 := s.SnapshotMap()
	if m2[id1].ID != id1 {
		t.Error("SnapshotMap() should return copies, not references")
	}
}

func TestStore_Count(t *testing.T) {
	s := federation.NewStore()

	if s.Count() != 0 {
		t.Errorf("Count() = %d, want 0", s.Count())
	}

	s.Upsert(makeEntity("acme.platform.git.repo.commit.a1b2c3"))
	if s.Count() != 1 {
		t.Errorf("Count() = %d, want 1", s.Count())
	}

	s.Upsert(makeEntity("acme.platform.git.repo.commit.d4e5f6"))
	if s.Count() != 2 {
		t.Errorf("Count() = %d, want 2", s.Count())
	}

	s.Remove("acme.platform.git.repo.commit.a1b2c3")
	if s.Count() != 1 {
		t.Errorf("Count() = %d, want 1 after remove", s.Count())
	}
}

func TestStore_ConcurrentAccess(t *testing.T) {
	s := federation.NewStore()
	const workers = 20
	const ops = 100

	var wg sync.WaitGroup
	wg.Add(workers)

	for i := range workers {
		go func(workerID int) {
			defer wg.Done()
			for j := range ops {
				id := "acme.platform.git.repo.commit.test"
				if j%2 == 0 {
					id = "acme.platform.git.repo.author.alice"
				}

				switch j % 4 {
				case 0, 1:
					s.Upsert(makeEntity(id))
				case 2:
					s.Remove(id)
				case 3:
					_ = s.Snapshot()
					_ = s.Count()
				}
				_ = workerID
			}
		}(i)
	}

	wg.Wait()
}
