package devviaspec

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func validPlan(epicCount int) *Plan {
	epics := make([]string, epicCount)
	for i := range epics {
		epics[i] = fmt.Sprintf("epic-%d: investigate boundary %d", i+1, i+1)
	}
	return &Plan{
		LoopID:     "loop-planner-001",
		Revision:   1,
		Title:      "Containment budget test plan",
		Goal:       "Verify planner fan-out ceiling",
		Context:    "Planner fan-out is bounded at 1..4 subtopics (#272)",
		Epics:      epics,
		ProducedAt: time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC),
	}
}

func TestPlanValidate_EpicsBounds(t *testing.T) {
	for _, count := range []int{1, 2, 3, 4} {
		t.Run(fmt.Sprintf("valid_count_%d", count), func(t *testing.T) {
			p := validPlan(count)
			if err := p.Validate(); err != nil {
				t.Fatalf("Validate(%d epics) unexpected error: %v", count, err)
			}
		})
	}

	for _, count := range []int{0, 5, 15} {
		t.Run(fmt.Sprintf("invalid_count_%d", count), func(t *testing.T) {
			p := validPlan(count)
			err := p.Validate()
			if err == nil {
				t.Fatalf("Validate(%d epics) = nil, want error", count)
			}
			if !strings.Contains(err.Error(), "epics") {
				t.Errorf("Validate(%d epics) error = %q, want mention of epics", count, err.Error())
			}
		})
	}

	t.Run("blank_epic_rejected", func(t *testing.T) {
		p := validPlan(2)
		p.Epics[1] = "   "
		err := p.Validate()
		if err == nil {
			t.Fatal("Validate(blank epic) = nil, want error")
		}
		if !strings.Contains(err.Error(), "epics[1]") {
			t.Errorf("Validate(blank epic) error = %q, want mention of epics[1]", err.Error())
		}
	})
}
