package contract

import (
	"testing"
)

// TestResearchPack_SynthesizeJoinGuardsEmptyFanout pins the empty-fan-out guard
// on research/03b. The synthesize JOIN fires on the planner entity when
// research.gather.completed_subtopic length_eq coordinator.decision.subtopics.length.
// Without a subtopics-present guard, a planner that terminates WITHOUT deciding
// gather (a loop-failure, surfaced by the ADR-053 4a′ run-failed journey) leaves
// BOTH fields absent and length_eq(absent, absent.length) evaluates 0==0 → a
// SPURIOUS synthesize join over zero gatherers. The coordinator.decision.subtopics
// length_gt 0 condition requires a real subtopics list before the join can fire.
//
// This is the CI-time pin for a behavior the e2e run-failed journey proves but
// CI does not run (no Docker stack in the unit lane).
func TestResearchPack_SynthesizeJoinGuardsEmptyFanout(t *testing.T) {
	r := loadRule(t, "../../configs/rules/research/03b-synthesize-when-all-gathers-complete.json")

	if !r.hasConditionField("coordinator.decision.subtopics", "length_gt") {
		t.Error("research/03b must guard the synthesize join on `coordinator.decision.subtopics length_gt 0` — " +
			"without it a planner that fails before deciding gather spuriously spawns an orphan synthesize " +
			"(0==0 empty-fan-out). Removing the guard regresses the ADR-053 4a′ run-failed journey.")
	}
	// The join must still key on the dynamic counter==subtopics-length match.
	if !r.hasConditionField("research.gather.completed_subtopic", "length_eq") {
		t.Error("research/03b must still join on research.gather.completed_subtopic length_eq subtopics.length")
	}
}
