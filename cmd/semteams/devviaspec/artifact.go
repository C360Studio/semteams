// Package devviaspec holds SemTeams-local payload types for the
// dev-via-spec arc. The Artifact type is the structured terminal output
// produced by the architect role at the close of a dev-via-spec chain:
// research → mode-transition → planner → reviewer → challenger → architect.
//
// See docs/adr/031-research-flow-and-semspec-handoff.md (§R3.3) for the
// design rationale and the chain-provenance model that motivates the
// Provenance fields.
//
// The Artifact carries:
//   - Goal + Context: the "why" grounding from the upstream research arc.
//   - Actors + IntegrationPoints: the structural inventory the architect
//     condenses from the planner/reviewer/challenger exchange.
//   - SeedRequirements: decomposable-grain work items, each grounded in
//     at least one actor and one integration point so they cannot float
//     free of the structural model.
//   - Provenance: the loop-ID chain so the artifact is traceable back
//     through every role that contributed to it.
//
// The Validate method enforces structural well-formedness only. Semantic
// checks (e.g. "every seed requirement is grounded in a named actor that
// exists in the Actors list") are reviewer-persona concerns, not
// executor concerns. The terminal architect calls this after the chain
// converges; at that point all structural fields should be present.
package devviaspec

import (
	"encoding/json"
	"fmt"

	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/payloadregistry"
)

// Payload type metadata (domain.category.version → "dev_via_spec.artifact.v1").
const (
	Domain           = "dev_via_spec"
	CategoryArtifact = "artifact"
	SchemaVersion    = "v1"
)

// Direction values for IntegrationPoint — shared with research package but
// kept local to avoid a cross-package dependency on a two-constant set.
const (
	DirectionRead  = "read"
	DirectionWrite = "write"
)

// Actor enumerates one external system, framework, library, or service
// the work will touch. The architect-as-enumerator gate verifies that
// every seed requirement is grounded in at least one named actor.
type Actor struct {
	Name string `json:"name"`
	Role string `json:"role"`
}

// IntegrationPoint enumerates one actor-to-actor data flow with
// direction. Direction may be empty during in-flight construction;
// the architect's terminal call should have all directions resolved.
type IntegrationPoint struct {
	From      string `json:"from"`
	To        string `json:"to"`
	Direction string `json:"direction"` // DirectionRead, DirectionWrite, or "" (in-flight)
	Data      string `json:"data"`
}

// SeedRequirement is a decomposable-grain work item that the architect
// scoped from the planner/reviewer/challenger exchange. Grounding fields
// trace it back to the structural inventory so future implementors know
// which actors and data flows a requirement touches.
type SeedRequirement struct {
	Title                    string   `json:"title"`
	Scope                    string   `json:"scope"`
	GroundsActors            []string `json:"grounds_actors,omitempty"`
	GroundsIntegrationPoints []string `json:"grounds_integration_points,omitempty"` // freeform "from→to" strings
}

// Provenance records the loop-ID chain for the entire dev-via-spec arc.
// Every role from research through architect writes its loop ID here so
// the artifact is traceable to the originating research run and every
// subsequent role exchange. ArchitectLoop is set server-side by the
// executor from the calling ToolCall; the LLM supplies the rest.
type Provenance struct {
	ResearchArtifactLoop string `json:"research_artifact_loop"`
	PlannerLoop          string `json:"planner_loop"`
	ReviewerLoop         string `json:"reviewer_loop"`
	ChallengerLoop       string `json:"challenger_loop"`
	ArchitectLoop        string `json:"architect_loop,omitempty"` // set by executor from ToolCall.LoopID
}

// Artifact is the SemTeams-local payload representing the terminal
// output of a dev-via-spec arc. It is emitted once per arc by the
// architect role after the planner/reviewer/challenger chain converges.
//
// GeneratedAt and Slug are set server-side by the executor — the LLM
// neither sees nor sets them (mirrors research.Artifact's server-side
// LoopID / ProducedAt pattern).
//
// Schema: dev_via_spec.artifact.v1.
type Artifact struct {
	Title             string             `json:"title"`
	Goal              string             `json:"goal"`
	Context           string             `json:"context"`
	Actors            []Actor            `json:"actors"`
	IntegrationPoints []IntegrationPoint `json:"integration_points"`
	SeedRequirements  []SeedRequirement  `json:"seed_requirements"`
	Provenance        Provenance         `json:"provenance"`
	GeneratedAt       string             `json:"generated_at"` // RFC3339, set by executor
	Slug              string             `json:"slug"`         // kebab-case with YYYY-MM-DD- prefix, set by executor
}

// Schema implements message.Payload.
func (a *Artifact) Schema() message.Type {
	return message.Type{Domain: Domain, Category: CategoryArtifact, Version: SchemaVersion}
}

// Validate implements message.Payload. Validates structural well-formedness
// only: required fields present, ≥1 actor, ≥1 seed requirement, integration
// point directions in {"read","write",""}. Semantic checks (e.g. every SR
// references an actor that exists in the Actors slice) are architect-persona
// concerns; Validate is called by the executor before writing any triples.
func (a *Artifact) Validate() error {
	if a.Title == "" {
		return fmt.Errorf("title required")
	}
	if a.Goal == "" {
		return fmt.Errorf("goal required")
	}
	if a.Context == "" {
		return fmt.Errorf("context required")
	}
	if len(a.Actors) == 0 {
		return fmt.Errorf("actors: at least one actor required")
	}
	if len(a.SeedRequirements) == 0 {
		return fmt.Errorf("seed_requirements: at least one seed requirement required")
	}
	for i, ip := range a.IntegrationPoints {
		switch ip.Direction {
		case DirectionRead, DirectionWrite, "":
			// Empty allowed: architect may not assign direction to all points.
		default:
			return fmt.Errorf("integration_points[%d].direction must be one of read/write (got %q)", i, ip.Direction)
		}
	}
	if a.Provenance.ResearchArtifactLoop == "" {
		return fmt.Errorf("provenance.research_artifact_loop required")
	}
	if a.Provenance.PlannerLoop == "" {
		return fmt.Errorf("provenance.planner_loop required")
	}
	if a.Provenance.ReviewerLoop == "" {
		return fmt.Errorf("provenance.reviewer_loop required")
	}
	if a.Provenance.ChallengerLoop == "" {
		return fmt.Errorf("provenance.challenger_loop required")
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

// RegisterPayloads registers SemTeams-local dev-via-spec payloads with
// the supplied registry. Call from cmd/semteams/main.go after
// research.RegisterPayloads so the full SemTeams-local type set layers
// on top of the framework's first-party payloads.
func RegisterPayloads(reg *payloadregistry.Registry) error {
	return reg.Register(&payloadregistry.Registration{
		Factory:     func() any { return &Artifact{} },
		Domain:      Domain,
		Category:    CategoryArtifact,
		Version:     SchemaVersion,
		Description: "SemTeams dev-via-spec artifact — terminal structured output of the architect role at the close of a research→planner→reviewer→challenger→architect chain. ADR-031 §R3.3.",
	})
}
