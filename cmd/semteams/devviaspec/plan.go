package devviaspec

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/payloadregistry"
)

// CategoryPlan is the Plan payload's category half (the full schema
// type is dev_via_spec.plan.v1). Sibling to CategoryArtifact within
// the dev_via_spec domain.
const CategoryPlan = "plan"

// PlanDependsOn records cross-arc references the plan grounds against.
// ResearchArtifactLoop pins which research run the plan responds to;
// PR D will surface this on the chain entity (today the chain entity
// pulls research lineage from its own predicates).
type PlanDependsOn struct {
	ResearchArtifactLoop string `json:"research_artifact_loop,omitempty"`
}

// Plan is the SemTeams-local payload representing the planner role's
// approved-pass output in the dev-via-spec arc. Emitted via
// emit_plan when the reviewer accepts the planner's submission
// (rule_03 fires the milestone subscriber).
//
// The persona supplies content via typed fields (goal, context,
// scope, epics); the tool renders the markdown structure
// (docs/plans/<slug>.md) with stable headings. Per ADR-038 D3:
// persona supplies substance, tool supplies format. No persona-side
// markdown formatting.
//
// Schema: dev_via_spec.plan.v1. LoopID, ProducedAt, and Slug are
// server-supplied (mirrors research.Artifact's pattern).
type Plan struct {
	LoopID   string `json:"loop_id"`
	Revision int    `json:"revision"`

	// Title is an optional one-line summary of the plan. Used to
	// derive a stable slug for the rendered docs/plans/<slug>.md
	// file. Empty falls back to a loop-id-suffixed slug.
	Title string `json:"title,omitempty"`

	// Goal is the concrete, testable target capability the plan
	// commits to (named interface, endpoint, component, capability).
	// Single string — the persona writes one sentence or one short
	// paragraph.
	Goal string `json:"goal"`

	// Context is why the work matters, naming at least one actor
	// from the upstream research artifact and the integration
	// boundary the work sits at. Single string.
	Context string `json:"context"`

	// ScopeIn enumerates work items that are explicitly in-scope.
	// Each item is one decomposable description string; the
	// rendered markdown projects them as a bullet list. Persona
	// owns granularity.
	ScopeIn []string `json:"scope_in,omitempty"`

	// ScopeOut enumerates work items that are explicitly excluded.
	// Each item should carry a one-line rationale per the
	// research-artifact integration_point coverage discipline.
	// Rendered as a parallel bullet list.
	ScopeOut []string `json:"scope_out,omitempty"`

	// Epics is interface-level decomposition. Each epic grounds
	// against an actor or integration boundary the context names.
	// Required (≥1) — a plan with no epics is not a plan, it's a
	// goal statement.
	Epics []string `json:"epics"`

	// DependsOn pins cross-arc references the plan grounds against.
	DependsOn PlanDependsOn `json:"depends_on,omitempty"`

	// Slug is server-derived (NOT LLM-supplied) by emit_plan at
	// emission time: lower-kebab-case from Title (or a loop-id
	// fallback) prefixed with the YYYY-MM-DD producedAt date.
	// Stable per (title, date) pair so re-emissions overwrite the
	// same path. Empty when the plan predates ADR-038 PR C Phase C2.
	Slug string `json:"slug,omitempty"`

	ProducedAt time.Time `json:"produced_at"`
}

// Schema implements message.Payload.
func (p *Plan) Schema() message.Type {
	return message.Type{Domain: Domain, Category: CategoryPlan, Version: SchemaVersion}
}

// Validate implements message.Payload. Structural well-formedness
// only — content-quality checks (epic granularity, scope coverage)
// live in the reviewer-spec persona, not here.
func (p *Plan) Validate() error {
	if p.LoopID == "" {
		return fmt.Errorf("loop_id required")
	}
	if p.Revision < 1 {
		return fmt.Errorf("revision must be >= 1, got %d", p.Revision)
	}
	if p.Goal == "" {
		return fmt.Errorf("goal required")
	}
	if p.Context == "" {
		return fmt.Errorf("context required")
	}
	if len(p.Epics) == 0 {
		return fmt.Errorf("epics required (>= 1) — a plan with no epics is a goal statement, not a plan")
	}
	if p.ProducedAt.IsZero() {
		return fmt.Errorf("produced_at required")
	}
	return nil
}

// MarshalJSON implements json.Marshaler.
func (p *Plan) MarshalJSON() ([]byte, error) {
	type Alias Plan
	return json.Marshal((*Alias)(p))
}

// UnmarshalJSON implements json.Unmarshaler.
func (p *Plan) UnmarshalJSON(data []byte) error {
	type Alias Plan
	return json.Unmarshal(data, (*Alias)(p))
}

// registerPlanPayload adds the Plan factory to the supplied registry.
// Called from RegisterPayloads alongside Artifact registration so the
// dev-via-spec package's full payload set is registered together.
func registerPlanPayload(reg *payloadregistry.Registry) error {
	return reg.Register(&payloadregistry.Registration{
		Factory:     func() any { return &Plan{} },
		Domain:      Domain,
		Category:    CategoryPlan,
		Version:     SchemaVersion,
		Description: "SemTeams dev-via-spec plan — structured planner output emitted at the planner's approved-pass terminal. ADR-038 PR C Phase C2.",
	})
}
