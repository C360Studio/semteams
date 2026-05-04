package runtimes

import (
	"fmt"
	"sort"
	"sync"
)

// Runtime is the interface every test runtime implements.
//
// The runtime owns its per-family templates: TemplateFor(family)
// returns the rendered-by-template-engine source for that combination
// or an empty string + false if the (family × runtime) cell is empty.
// An empty cell is a valid "this runtime doesn't know how to verify
// this family" signal — the schema gate at R3.7.2.f rejects
// commitments that pair an unsupported (family, runtime).
//
// Templates are returned as raw source (Go template syntax,
// language-specific). Render-time substitution happens at builder-
// invocation time using values drawn from the smoke contract +
// catalog entry. This is intentionally pre-substitution — the
// runtime's job is to KNOW WHICH template, not to RENDER it. Render
// is a downstream concern (R3.7.2.h evidence gate / builder).
type Runtime interface {
	// Name returns the runtime identifier (e.g.
	// "java-junit-testcontainers"). Used for matrix lookup against
	// commitment.runtime and as the value architects emit on
	// commitments. Hyphenated lowercase ASCII by convention.
	Name() string

	// Description is a short human-readable summary used by
	// operators inspecting the runtime catalog and by error messages.
	Description() string

	// InvokeCommand is the bash command shape the builder runs from
	// inside the sandbox to execute tests for this runtime. May
	// contain template placeholders (e.g. "<harness-profile>") that
	// the builder substitutes from the catalog entry. The runtime
	// returns the SHAPE; substitution is the builder's concern.
	//
	// Example: "mvn verify -P<harness-profile>"
	InvokeCommand() string

	// TemplateFor returns the test-code template for the given
	// family. Returns ("", false) when the cell is empty (this
	// runtime doesn't support that family yet).
	//
	// The family argument is the FullName ("name.version", e.g.
	// "tcp.binary-protobuf.v1") so the runtime can support multiple
	// versions of the same family separately.
	TemplateFor(familyFullName string) (string, bool)

	// SupportedFamilies returns the FullNames of all families this
	// runtime has templates for. Empty slice = no cells filled
	// (R3.7.2.d posture for java-junit-testcontainers; templates
	// arrive in R3.7.2.f).
	SupportedFamilies() []string
}

// Registry holds Runtime implementations keyed by Name. Pattern-A
// per ADR-029 — boot-registry, dependency-injected, no KV backing.
type Registry struct {
	mu       sync.RWMutex
	runtimes map[string]Runtime
}

// NewRegistry creates an empty registry.
func NewRegistry() *Registry {
	return &Registry{
		runtimes: make(map[string]Runtime),
	}
}

// Register adds a runtime. Returns an error if a runtime with the
// same Name is already registered — boot-time misconfiguration.
func (r *Registry) Register(rt Runtime) error {
	if rt == nil {
		return fmt.Errorf("runtime cannot be nil")
	}
	if rt.Name() == "" {
		return fmt.Errorf("runtime Name() cannot be empty")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.runtimes[rt.Name()]; exists {
		return fmt.Errorf("runtime %q already registered", rt.Name())
	}
	r.runtimes[rt.Name()] = rt
	return nil
}

// Get returns the runtime registered for the given Name. Returns
// nil and an error if not found.
func (r *Registry) Get(name string) (Runtime, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	rt, ok := r.runtimes[name]
	if !ok {
		return nil, fmt.Errorf("runtime %q not registered", name)
	}
	return rt, nil
}

// List returns every registered runtime sorted by Name.
func (r *Registry) List() []Runtime {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Runtime, 0, len(r.runtimes))
	for _, rt := range r.runtimes {
		out = append(out, rt)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name() < out[j].Name()
	})
	return out
}

// Len returns the count of registered runtimes.
func (r *Registry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.runtimes)
}
