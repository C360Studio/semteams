package tcpbinaryprotobuf

import (
	"fmt"

	"github.com/c360studio/semteams/cmd/semteams/harness"
	"github.com/c360studio/semteams/cmd/semteams/verification/families"
)

// Name and Version are the registry-key components. Used by callers
// that need to look up this family without importing the type
// (e.g. a catalog entry referencing the family by FullName string).
const (
	Name    = "tcp.binary-protobuf"
	Version = "v1"
)

// Family implements families.Family for tcp-binary-protobuf sidecars.
// Stub validator in R3.7.2.d; concrete schema validation lands in
// R3.7.2.f alongside the architect persona's smoke_contract pinning.
type Family struct{}

// New returns a new Family instance. Single zero-value struct; the
// constructor exists for symmetry with other registry constructors
// and to give us a place to thread future config (e.g. version-
// specific behaviour) without breaking callers.
func New() *Family {
	return &Family{}
}

// Name implements families.Family.
func (*Family) Name() string { return Name }

// Version implements families.Family.
func (*Family) Version() string { return Version }

// Description implements families.Family.
func (*Family) Description() string {
	return "TCP transport carrying length-framed binary protobuf messages from a declared message-type vocabulary. Sidecars expose one or more TCP ports; drivers connect, write protobuf-encoded messages from the catalog's declared types, and observe responses on the same connection or a sibling output stream."
}

// ValidateContract is the stub validator (R3.7.2.d posture). Concrete
// schema validation against the harness's declared protobuf message
// types and TCP ports lands in R3.7.2.f when the architect persona
// pins the smoke_contract field structure.
//
// Today: returns nil for any non-nil harness; nil harness is a
// programming error (caller didn't look up the catalog entry).
func (*Family) ValidateContract(_ any, h *harness.Harness) error {
	if h == nil {
		return fmt.Errorf("ValidateContract: harness must not be nil (caller must look up catalog entry first)")
	}
	// TODO(R3.7.2.f): once smoke_contract structure is pinned, walk
	// the contract's scenarios and verify:
	//   - each given.message_type ∈ h.RealDependencies (allowlist)
	//   - each given.port ∈ h.Exposes.TCP[*].Port
	//   - each given.protocol matches h.Exposes.TCP[*].Protocol
	// Until then, structural validation is the architect persona's
	// own discipline and the reviewer's coverage check (R3.7.2.i).
	return nil
}

// Register registers this family on the supplied registry. Call
// from cmd/semteams/main.go after creating the registry; pattern
// matches research.RegisterPayloads / verification.RegisterPayloads.
func Register(reg *families.Registry) error {
	return reg.Register(New())
}
