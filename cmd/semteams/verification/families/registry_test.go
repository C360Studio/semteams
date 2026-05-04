package families

import (
	"strings"
	"sync"
	"testing"

	"github.com/c360studio/semteams/cmd/semteams/harness"
)

// fakeFamily is a test double for Family. Implements the interface
// minimally so registry tests don't depend on a concrete family.
type fakeFamily struct {
	name        string
	version     string
	description string
	validateErr error
}

func (f *fakeFamily) Name() string        { return f.name }
func (f *fakeFamily) Version() string     { return f.version }
func (f *fakeFamily) Description() string { return f.description }
func (f *fakeFamily) ValidateContract(_ any, _ *harness.Harness) error {
	return f.validateErr
}

func TestFullName(t *testing.T) {
	t.Parallel()
	f := &fakeFamily{name: "tcp.binary-protobuf", version: "v1"}
	if got, want := FullName(f), "tcp.binary-protobuf.v1"; got != want {
		t.Errorf("FullName: got %q, want %q", got, want)
	}
}

func TestRegistry_Register_Get(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	f := &fakeFamily{name: "tcp.binary-protobuf", version: "v1", description: "test"}
	if err := r.Register(f); err != nil {
		t.Fatalf("Register: %v", err)
	}
	got, err := r.Get("tcp.binary-protobuf.v1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != f {
		t.Errorf("Get returned different instance")
	}
}

func TestRegistry_Register_NilRejected(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	if err := r.Register(nil); err == nil {
		t.Errorf("expected error for nil family")
	}
}

func TestRegistry_Register_EmptyNameRejected(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	if err := r.Register(&fakeFamily{version: "v1"}); err == nil {
		t.Errorf("expected error for empty name")
	}
}

func TestRegistry_Register_EmptyVersionRejected(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	if err := r.Register(&fakeFamily{name: "x"}); err == nil {
		t.Errorf("expected error for empty version")
	}
}

func TestRegistry_Register_DuplicateRejected(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	f := &fakeFamily{name: "x", version: "v1"}
	if err := r.Register(f); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(f); err == nil {
		t.Errorf("expected error on duplicate registration")
	} else if !strings.Contains(err.Error(), "already registered") {
		t.Errorf("expected 'already registered' in error, got %v", err)
	}
}

func TestRegistry_Register_DifferentVersionsCoexist(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	if err := r.Register(&fakeFamily{name: "x", version: "v1"}); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(&fakeFamily{name: "x", version: "v2"}); err != nil {
		t.Errorf("v2 should coexist with v1: %v", err)
	}
}

func TestRegistry_Get_NotFound(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	_, err := r.Get("ghost.v1")
	if err == nil {
		t.Errorf("expected error for unregistered family")
	}
	if !strings.Contains(err.Error(), "not registered") {
		t.Errorf("expected 'not registered' in error, got %v", err)
	}
}

func TestRegistry_List_Sorted(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	for _, n := range []string{"http.rest", "tcp.binary-protobuf", "kafka.topic"} {
		if err := r.Register(&fakeFamily{name: n, version: "v1"}); err != nil {
			t.Fatal(err)
		}
	}
	got := r.List()
	if len(got) != 3 {
		t.Fatalf("List len: got %d, want 3", len(got))
	}
	want := []string{"http.rest.v1", "kafka.topic.v1", "tcp.binary-protobuf.v1"}
	for i, w := range want {
		if FullName(got[i]) != w {
			t.Errorf("List[%d]: got %q, want %q", i, FullName(got[i]), w)
		}
	}
}

func TestRegistry_Len(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	if r.Len() != 0 {
		t.Errorf("empty registry Len(): got %d, want 0", r.Len())
	}
	_ = r.Register(&fakeFamily{name: "x", version: "v1"})
	if r.Len() != 1 {
		t.Errorf("after one register: got %d, want 1", r.Len())
	}
}

func TestRegistry_ConcurrentRegisterAndGet(t *testing.T) {
	t.Parallel()
	// Race detector should not surface anything during concurrent
	// register / get / list — registry is mutex-protected. Each
	// goroutine uses a distinct version key so registration races
	// are concurrent-no-conflict; duplicate-with-same-key is its own
	// test case (TestRegistry_Register_DuplicateRejected).
	r := NewRegistry()
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			f := &fakeFamily{name: "fam", version: "v" + string(rune('0'+i))}
			_ = r.Register(f)
			_, _ = r.Get(FullName(f))
			_ = r.List()
			_ = r.Len()
		}()
	}
	wg.Wait()
}
