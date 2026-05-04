package runtimes

import (
	"strings"
	"sync"
	"testing"
)

// fakeRuntime is a test double for Runtime.
type fakeRuntime struct {
	name        string
	description string
	invoke      string
	templates   map[string]string
}

func (f *fakeRuntime) Name() string          { return f.name }
func (f *fakeRuntime) Description() string   { return f.description }
func (f *fakeRuntime) InvokeCommand() string { return f.invoke }
func (f *fakeRuntime) TemplateFor(family string) (string, bool) {
	t, ok := f.templates[family]
	return t, ok
}
func (f *fakeRuntime) SupportedFamilies() []string {
	out := make([]string, 0, len(f.templates))
	for k := range f.templates {
		out = append(out, k)
	}
	return out
}

func TestRegistry_Register_Get(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	rt := &fakeRuntime{name: "java-junit-testcontainers", invoke: "mvn verify"}
	if err := r.Register(rt); err != nil {
		t.Fatalf("Register: %v", err)
	}
	got, err := r.Get("java-junit-testcontainers")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != rt {
		t.Errorf("Get returned different instance")
	}
}

func TestRegistry_Register_NilRejected(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	if err := r.Register(nil); err == nil {
		t.Errorf("expected error for nil runtime")
	}
}

func TestRegistry_Register_EmptyNameRejected(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	if err := r.Register(&fakeRuntime{}); err == nil {
		t.Errorf("expected error for empty name")
	}
}

func TestRegistry_Register_DuplicateRejected(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	rt := &fakeRuntime{name: "x"}
	if err := r.Register(rt); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(rt); err == nil {
		t.Errorf("expected duplicate-registration error")
	} else if !strings.Contains(err.Error(), "already registered") {
		t.Errorf("expected 'already registered' in error, got %v", err)
	}
}

func TestRegistry_Get_NotFound(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	_, err := r.Get("ghost")
	if err == nil {
		t.Errorf("expected error for unregistered runtime")
	}
}

func TestRegistry_List_Sorted(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	for _, n := range []string{"python-pytest", "java-junit-testcontainers", "go-testing-net"} {
		if err := r.Register(&fakeRuntime{name: n}); err != nil {
			t.Fatal(err)
		}
	}
	got := r.List()
	want := []string{"go-testing-net", "java-junit-testcontainers", "python-pytest"}
	if len(got) != len(want) {
		t.Fatalf("List len: got %d, want %d", len(got), len(want))
	}
	for i, w := range want {
		if got[i].Name() != w {
			t.Errorf("List[%d]: got %q, want %q", i, got[i].Name(), w)
		}
	}
}

func TestRegistry_TemplateFor(t *testing.T) {
	t.Parallel()
	rt := &fakeRuntime{
		name:      "x",
		templates: map[string]string{"family.v1": "TEMPLATE BODY"},
	}
	got, ok := rt.TemplateFor("family.v1")
	if !ok {
		t.Fatal("expected template lookup to succeed")
	}
	if got != "TEMPLATE BODY" {
		t.Errorf("template body: got %q", got)
	}
	_, ok = rt.TemplateFor("ghost.v1")
	if ok {
		t.Errorf("expected lookup for unknown family to return ok=false")
	}
}

func TestRegistry_SupportedFamilies_Empty(t *testing.T) {
	t.Parallel()
	rt := &fakeRuntime{name: "x"}
	if got := rt.SupportedFamilies(); len(got) != 0 {
		t.Errorf("empty templates: got %v, want []", got)
	}
}

func TestRegistry_Len_NilReceiver(t *testing.T) {
	t.Parallel()
	// Nil-safe: mirrors families.Registry.Len's contract so
	// boot-time-failure paths can pass nil registries to logging
	// without typed-nil NPE. Callers in
	// registerProductFamiliesAndRuntimes depend on this.
	var r *Registry
	if r.Len() != 0 {
		t.Errorf("nil receiver Len(): got %d, want 0", r.Len())
	}
}

func TestRegistry_ConcurrentAccess(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = r.Register(&fakeRuntime{name: "rt-" + string(rune('a'+i))})
			_, _ = r.Get("rt-a")
			_ = r.List()
			_ = r.Len()
		}()
	}
	wg.Wait()
}
