package research

// Planner fan-out containment bounds (ADR-060 §7 / #272).
//
// Both the additive plan artifact schema (emit_plan.epics via
// devviaspec.Plan.Validate) and the active fan-out trigger
// (decide.subtopics via researchdecide.Executor + rule 02-plan-to-gather)
// enforce the same 1..4 subtopic range so the two surfaces cannot drift.
const (
	// MinSubtopics is the minimum number of subtopics/epics a research
	// planner may emit when transitioning to the gather phase. N=1 is the
	// non-decomposable single-gatherer case.
	MinSubtopics = 1

	// MaxSubtopics is the maximum number of parallel gather subtopics/epics
	// a research planner may emit in a single pass.
	MaxSubtopics = 4

	// PlannerRole is the canonical role token for the research planner loop.
	PlannerRole = "researcher-research-plan"

	// GatherAction is the decide action that fans out from PLAN to GATHER.
	GatherAction = "gather"
)
