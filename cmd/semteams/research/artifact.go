// Package research holds SemTeams-local payload types for the
// research flow. The Artifact type is the stable-or-not snapshot
// produced by each researcher pass; revisions are monotonic from 1.
//
// See docs/adr/031-research-flow-and-semspec-handoff.md, in
// particular the 2026-04-30 addendum "Stabilisation as converged
// change-log", for the design rationale and the stabilisation
// predicate.
//
// Stabilised is a *derived* predicate, not a stored field:
//
//	stabilised := reviewer_approved == true &&
//	              substrate_mutations.delta(latest_revision) == ∅
//
// The reviewer-as-enumerator approves on output-side criteria; the
// run is stabilised when no new substrate mutations were needed in
// the latest revision. The dev-via-spec mode transition (R3.2+)
// gates on this predicate via a rule, not by reading a flag on the
// artifact.
package research

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/payloadregistry"
)

// Payload type metadata (domain.category.version → "research.artifact.v1").
const (
	Domain           = "research"
	CategoryArtifact = "artifact"
	SchemaVersion    = "v1"
)

// MutationStatus enumerates the lifecycle states of a substrate
// mutation recorded on an Artifact.
type MutationStatus string

// MutationStatus values.
const (
	MutationStatusApproved MutationStatus = "approved"
	MutationStatusRejected MutationStatus = "rejected"
	MutationStatusExecuted MutationStatus = "executed"
	MutationStatusFailed   MutationStatus = "failed"
)

// Direction values for IntegrationPoint.
const (
	DirectionRead  = "read"
	DirectionWrite = "write"
)

// Actor enumerates one external system / framework / library the
// research target touches. The reviewer-as-enumerator gate verifies
// every required actor appears with non-empty role text.
type Actor struct {
	Name string `json:"name"`
	Role string `json:"role"`
}

// IntegrationPoint enumerates one actor-to-actor data flow with
// direction. The reviewer's enumerator gate verifies every required
// pair appears with a defined direction.
type IntegrationPoint struct {
	From      string `json:"from"`
	To        string `json:"to"`
	Data      string `json:"data"`
	Direction string `json:"direction"` // DirectionRead or DirectionWrite
}

// Mutation records one substrate-modifying action a research run
// took. Append-only across revisions. The mutation log is the
// honest answer to "what did this run change in the corpus,"
// auditable independent of the post-hoc actor list.
type Mutation struct {
	Tool       string          `json:"tool"`                  // Tool name (e.g. "add_source_repo").
	Args       json.RawMessage `json:"args"`                  // Original tool args, verbatim.
	LoopID     string          `json:"loop_id"`               // Loop that emitted the mutation.
	Revision   int             `json:"revision"`              // Artifact revision when the mutation landed.
	ApprovedBy string          `json:"approved_by,omitempty"` // Approver identity for gated mutations.
	Status     MutationStatus  `json:"status"`
	Result     string          `json:"result,omitempty"` // Tool result summary (free-form).
	Timestamp  time.Time       `json:"timestamp"`
}

// Artifact is the SemTeams-local payload representing one
// researcher-pass snapshot. Revisions are monotonic from 1; the
// mutation log is monotonic and append-only across revisions.
//
// Schema: research.artifact.v1.
type Artifact struct {
	LoopID             string             `json:"loop_id"`
	Revision           int                `json:"revision"`
	Actors             []Actor            `json:"actors"`
	IntegrationPoints  []IntegrationPoint `json:"integration_points"`
	SeedRequirements   []string           `json:"seed_requirements"`
	AddressedGaps      []string           `json:"addressed_gaps,omitempty"`
	OpenGaps           []string           `json:"open_gaps,omitempty"`
	SubstrateMutations []Mutation         `json:"substrate_mutations,omitempty"`
	ProducedAt         time.Time          `json:"produced_at"`
}

// Schema implements message.Payload.
func (a *Artifact) Schema() message.Type {
	return message.Type{Domain: Domain, Category: CategoryArtifact, Version: SchemaVersion}
}

// Validate implements message.Payload. Validates structural
// well-formedness only; reviewer-as-enumerator semantic checks
// (required actors named, integration-point directions present,
// seed-requirement granularity) live in the reviewer persona, not
// here. An in-flight artifact may legitimately have empty actors
// or empty directions while the run is still iterating.
func (a *Artifact) Validate() error {
	if a.LoopID == "" {
		return fmt.Errorf("loop_id required")
	}
	if a.Revision < 1 {
		return fmt.Errorf("revision must be >= 1, got %d", a.Revision)
	}
	if a.ProducedAt.IsZero() {
		return fmt.Errorf("produced_at required")
	}
	for i, m := range a.SubstrateMutations {
		if m.Tool == "" {
			return fmt.Errorf("substrate_mutations[%d].tool required", i)
		}
		if m.LoopID == "" {
			return fmt.Errorf("substrate_mutations[%d].loop_id required", i)
		}
		if m.Revision < 1 {
			return fmt.Errorf("substrate_mutations[%d].revision must be >= 1, got %d", i, m.Revision)
		}
		switch m.Status {
		case MutationStatusApproved, MutationStatusRejected, MutationStatusExecuted, MutationStatusFailed:
		default:
			return fmt.Errorf("substrate_mutations[%d].status must be one of approved/rejected/executed/failed (got %q)", i, m.Status)
		}
		if m.Timestamp.IsZero() {
			return fmt.Errorf("substrate_mutations[%d].timestamp required", i)
		}
	}
	for i, ip := range a.IntegrationPoints {
		switch ip.Direction {
		case DirectionRead, DirectionWrite, "":
			// Empty allowed during in-flight construction; reviewer enforces presence.
		default:
			return fmt.Errorf("integration_points[%d].direction must be one of read/write (got %q)", i, ip.Direction)
		}
	}
	return nil
}

// MarshalJSON implements json.Marshaler.
func (a *Artifact) MarshalJSON() ([]byte, error) {
	type Alias Artifact
	return json.Marshal((*Alias)(a))
}

// UnmarshalJSON implements json.Unmarshaler.
func (a *Artifact) UnmarshalJSON(data []byte) error {
	type Alias Artifact
	return json.Unmarshal(data, (*Alias)(a))
}

// LatestRevisionMutationCount returns the number of substrate
// mutations whose Revision matches the artifact's current
// Revision. Useful for the stabilisation rule: a revision with
// zero new mutations is a candidate for "stabilised" provided the
// reviewer has approved it.
func (a *Artifact) LatestRevisionMutationCount() int {
	n := 0
	for _, m := range a.SubstrateMutations {
		if m.Revision == a.Revision {
			n++
		}
	}
	return n
}

// RegisterPayloads registers SemTeams-local research payloads with
// the supplied registry. Call from cmd/semteams/main.go after
// payloadbuiltins.Register so the SemTeams-local types layer on top
// of the framework's first-party set.
func RegisterPayloads(reg *payloadregistry.Registry) error {
	return reg.Register(&payloadregistry.Registration{
		Factory:     func() any { return &Artifact{} },
		Domain:      Domain,
		Category:    CategoryArtifact,
		Version:     SchemaVersion,
		Description: "SemTeams research artifact — revision-keyed snapshot of a research pass with append-only substrate-mutation log. ADR-031 §addendum 2026-04-30.",
	})
}
