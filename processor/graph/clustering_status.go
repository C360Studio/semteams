package graph

import "time"

// ClusteringStatus represents the current state of community detection process
type ClusteringStatus struct {
	Variant             string       `json:"variant"`
	LastRunTime         time.Time    `json:"last_run_time,omitempty"`
	LastRunResult       string       `json:"last_run_result,omitempty"`
	NextScheduled       time.Time    `json:"next_scheduled,omitempty"`
	EmbeddingCoverage   float64      `json:"embedding_coverage"`
	MinCoverage         float64      `json:"min_coverage"`
	CoverageSource      string       `json:"coverage_source,omitempty"`
	EnhancementWindow   WindowStatus `json:"enhancement_window"`
	Communities         int          `json:"communities,omitempty"`
	ConsecutiveFailures int          `json:"consecutive_failures,omitempty"`
	BlockingReason      string       `json:"blocking_reason,omitempty"`
	HealthStatus        string       `json:"health_status"`
}

// WindowStatus represents the enhancement window state within ClusteringStatus
type WindowStatus struct {
	Active        bool      `json:"active"`
	Mode          string    `json:"mode,omitempty"`
	Deadline      time.Time `json:"deadline,omitempty"`
	EntityChanges int       `json:"entity_changes,omitempty"`
	Threshold     int       `json:"threshold,omitempty"`
	AllTerminal   bool      `json:"all_terminal"`
}
