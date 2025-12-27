// Package main provides tier capability diff types for comparison
package main

// PathRAGDiff compares PathRAG graph traversal results (Tier 0 - runs on all tiers)
type PathRAGDiff struct {
	BaselineEntities int   `json:"baseline_entities"`
	TargetEntities   int   `json:"target_entities"`
	EntitiesDiff     int   `json:"entities_diff"`
	BaselineLatency  int64 `json:"baseline_latency_ms"`
	TargetLatency    int64 `json:"target_latency_ms"`
	BothScoresValid  bool  `json:"both_scores_valid"`
}

// StructuralIndexDiff compares k-core and pivot index results (Tier 0)
type StructuralIndexDiff struct {
	BaselineKCoreMax  int  `json:"baseline_kcore_max"`
	TargetKCoreMax    int  `json:"target_kcore_max"`
	KCoreMaxDiff      int  `json:"kcore_max_diff"`
	BaselinePivots    int  `json:"baseline_pivots"`
	TargetPivots      int  `json:"target_pivots"`
	PivotsDiff        int  `json:"pivots_diff"`
	BothKCoreVerified bool `json:"both_kcore_verified"`
	BothPivotVerified bool `json:"both_pivot_verified"`
}

// GraphRAGDiff compares GraphRAG query results (Tier 2 - semantic only)
type GraphRAGDiff struct {
	BaselineLocalSuccess  bool  `json:"baseline_local_success"`
	TargetLocalSuccess    bool  `json:"target_local_success"`
	BaselineGlobalSuccess bool  `json:"baseline_global_success"`
	TargetGlobalSuccess   bool  `json:"target_global_success"`
	BaselineLocalLatency  int64 `json:"baseline_local_latency_ms"`
	TargetLocalLatency    int64 `json:"target_local_latency_ms"`
	BaselineGlobalLatency int64 `json:"baseline_global_latency_ms"`
	TargetGlobalLatency   int64 `json:"target_global_latency_ms"`
}

// CommunityDiff compares community detection results
type CommunityDiff struct {
	BaselineTotal   int     `json:"baseline_total"`
	TargetTotal     int     `json:"target_total"`
	TotalDiff       int     `json:"total_diff"`
	BaselineLargest int     `json:"baseline_largest"`
	TargetLargest   int     `json:"target_largest"`
	BaselineAvgSize float64 `json:"baseline_avg_size"`
	TargetAvgSize   float64 `json:"target_avg_size"`
}
