package devviaspec

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/payloadregistry"
)

// CategoryConsensus is the Consensus payload's category half (the
// full schema type is dev_via_spec.consensus.v1). Sibling to
// CategoryArtifact and CategoryPlan within the dev_via_spec domain.
const CategoryConsensus = "consensus"

// ConsensusDependsOn records cross-arc references the consensus
// grounds against. PlanLoop names the planner whose plan was
// accepted; ReviewerLoop names the reviewer who approved before the
// challenger's signoff. Both optional but encouraged so the architect
// can hop back through chain ancestry without re-walking.
type ConsensusDependsOn struct {
	PlanLoop     string `json:"plan_loop,omitempty"`
	ReviewerLoop string `json:"reviewer_loop,omitempty"`
}

// Consensus is the SemTeams-local payload representing the
// dev-via-spec-challenger's accept-terminal output. Emitted via
// emit_consensus when the challenger's adversarial probing surfaces
// no execution-blocking concerns and the chain is ready to advance
// to the architect.
//
// The persona supplies content via typed fields (summary,
// chain_consensus bullets, considered_concerns); the tool renders
// the markdown structure (docs/consensus/<slug>.md) with stable
// headings. Per ADR-038 D3.
//
// Schema: dev_via_spec.consensus.v1. LoopID, ProducedAt, and Slug
// are server-supplied (mirrors Plan and research.Artifact).
type Consensus struct {
	LoopID string `json:"loop_id"`

	// Title is an optional one-line summary. Used to derive a stable
	// slug for the rendered docs/consensus/<slug>.md file.
	Title string `json:"title,omitempty"`

	// Summary is the challenger's one-line accept rationale. Single
	// string; required. The architect curates this as the spec's
	// chain_consensus entry, so it should densely cite the chain
	// (actor names, integration boundaries, epic decomposition).
	Summary string `json:"summary"`

	// ChainConsensus is dense bullets the architect curates into the
	// terminal dev_via_spec.artifact. Each entry should cite chain
	// specifics — actor names, integration boundaries, epic
	// decomposition — drawn from the reviewer's approved summary.
	// Required (>= 1) — an empty consensus is just an opinion.
	ChainConsensus []string `json:"chain_consensus"`

	// ConsideredConcerns optionally documents concerns raised across
	// challenger iterations and how they resolved. Useful audit trail
	// for the architect and for cross-arc consumers; the chain
	// produced this consensus, not just the final pass.
	ConsideredConcerns []string `json:"considered_concerns,omitempty"`

	// DependsOn pins cross-arc references the consensus grounds
	// against. Optional in v1; encouraged for chain-graph navigation.
	DependsOn ConsensusDependsOn `json:"depends_on,omitempty"`

	// Slug is server-derived (NOT LLM-supplied). Empty when the
	// consensus predates ADR-038 PR C Phase C3.
	Slug string `json:"slug,omitempty"`

	ProducedAt time.Time `json:"produced_at"`
}

// Schema implements message.Payload.
func (c *Consensus) Schema() message.Type {
	return message.Type{Domain: Domain, Category: CategoryConsensus, Version: SchemaVersion}
}

// Validate implements message.Payload. Structural well-formedness
// only — content quality (citation density, concern resolution
// completeness) lives in the persona, not here.
func (c *Consensus) Validate() error {
	if c.LoopID == "" {
		return fmt.Errorf("loop_id required")
	}
	if c.Summary == "" {
		return fmt.Errorf("summary required")
	}
	if len(c.ChainConsensus) == 0 {
		return fmt.Errorf("chain_consensus required (>= 1) — an empty consensus is just an opinion")
	}
	if c.ProducedAt.IsZero() {
		return fmt.Errorf("produced_at required")
	}
	return nil
}

// MarshalJSON implements json.Marshaler.
func (c *Consensus) MarshalJSON() ([]byte, error) {
	type Alias Consensus
	return json.Marshal((*Alias)(c))
}

// UnmarshalJSON implements json.Unmarshaler.
func (c *Consensus) UnmarshalJSON(data []byte) error {
	type Alias Consensus
	return json.Unmarshal(data, (*Alias)(c))
}

// registerConsensusPayload adds the Consensus factory to the
// supplied registry. Called from RegisterPayloads alongside Artifact
// and Plan registration.
func registerConsensusPayload(reg *payloadregistry.Registry) error {
	return reg.Register(&payloadregistry.Registration{
		Factory:     func() any { return &Consensus{} },
		Domain:      Domain,
		Category:    CategoryConsensus,
		Version:     SchemaVersion,
		Description: "SemTeams dev-via-spec consensus — structured challenger output emitted at the challenger's accept terminal. ADR-038 PR C Phase C3.",
	})
}
