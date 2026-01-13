package rule

import "time"

// Status represents the current status of rule evaluation for debug observability
type Status struct {
	DebounceDelayMs    int       `json:"debounce_delay_ms"`
	PendingEvaluations int       `json:"pending_evaluations"`
	TotalEvaluations   int       `json:"total_evaluations"`
	TotalTriggers      int       `json:"total_triggers"`
	DebouncedCount     int       `json:"debounced_count"` // Matches Tester's test expectations
	RulesLoaded        int       `json:"rules_loaded"`
	LastEvaluationTime time.Time `json:"last_evaluation_time,omitempty"`
}
