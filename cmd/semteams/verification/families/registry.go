package families

import (
	"fmt"
	"sort"
	"sync"

	"github.com/c360studio/semteams/cmd/semteams/harness"
)

// Family is the interface every protocol family implements. The
// concrete type lives in a sub-package (e.g. tcpbinaryprotobuf);
// the registry holds Family instances by name.
//
// Validate semantics: given a smoke_contract value (loosely typed
// — its concrete shape is the family's own decision in R3.7.2.f)
// and the catalog entry the architect named, return nil iff the
// contract's references are within the catalog's declared surface.
// Implementations may parse the contract however they like; the
// contract bytes are framework-neutral.
type Family interface {
	// Name returns the family identifier (e.g. "tcp.binary-protobuf").
	// Used for matrix lookup against runtime.SupportsFamily and as
	// the value architects emit in commitment.harness via the
	// catalog entry's family field.
	Name() string

	// Version returns the family's schema version (e.g. "v1").
	// Bumped when the contract shape itself migrates (additive
	// fields stay v1; breaking changes get a new version).
	Version() string

	// Description is a short human-readable summary used by
	// operators inspecting the catalog and by error messages.
	Description() string

	// ValidateContract checks that contract's references are
	// within harness's declared surface. The contract argument is
	// `any` because R3.7.2.d hasn't pinned the smoke_contract
	// structure yet (that's R3.7.2.f); concrete families type-
	// assert their expected shape and return a structured error if
	// the assertion fails. A nil harness is a programming error
	// (caller didn't look up the catalog entry); return an error
	// rather than panicking.
	ValidateContract(contract any, h *harness.Harness) error
}

// FullName returns the family's "name.version" identifier — the
// stable form catalog entries reference (e.g.
// "tcp.binary-protobuf.v1").
func FullName(f Family) string {
	return fmt.Sprintf("%s.%s", f.Name(), f.Version())
}

// Registry holds Family implementations keyed by FullName. Pattern-A
// per ADR-029 — boot-registry, dependency-injected, no KV backing.
type Registry struct {
	mu       sync.RWMutex
	families map[string]Family
}

// NewRegistry creates an empty registry.
func NewRegistry() *Registry {
	return &Registry{
		families: make(map[string]Family),
	}
}

// Register adds a family. Returns an error if a family with the
// same FullName is already registered — boot-time misconfiguration.
func (r *Registry) Register(f Family) error {
	if f == nil {
		return fmt.Errorf("family cannot be nil")
	}
	if f.Name() == "" {
		return fmt.Errorf("family Name() cannot be empty")
	}
	if f.Version() == "" {
		return fmt.Errorf("family Version() cannot be empty (use v1, v2, ...)")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	key := FullName(f)
	if _, exists := r.families[key]; exists {
		return fmt.Errorf("family %q already registered", key)
	}
	r.families[key] = f
	return nil
}

// Get returns the family registered for the given FullName
// (e.g. "tcp.binary-protobuf.v1"). Returns nil and an error if
// not found.
func (r *Registry) Get(fullName string) (Family, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	f, ok := r.families[fullName]
	if !ok {
		return nil, fmt.Errorf("family %q not registered", fullName)
	}
	return f, nil
}

// List returns every registered family sorted by FullName. The
// returned slice is a fresh copy; callers may mutate it freely.
func (r *Registry) List() []Family {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Family, 0, len(r.families))
	for _, f := range r.families {
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool {
		return FullName(out[i]) < FullName(out[j])
	})
	return out
}

// Len returns the count of registered families. Nil-safe: a nil
// receiver returns 0. Cheaper than len(List()) for callers that
// just want to know "is anything registered?" and avoids the
// typed-nil-through-interface gotcha when boot-time registration
// has failed and the registry is nil at the call site.
func (r *Registry) Len() int {
	if r == nil {
		return 0
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.families)
}
