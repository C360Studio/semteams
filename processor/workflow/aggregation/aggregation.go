// Package aggregation provides result aggregation strategies for parallel workflow steps.
package aggregation

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/c360studio/semstreams/pkg/types"
)

// AgentResult represents the result from a single parallel agent
type AgentResult struct {
	StepName string          `json:"step_name"`
	Status   string          `json:"status"` // types.StatusSuccess or types.StatusFailed
	Output   json.RawMessage `json:"output,omitempty"`
	Error    string          `json:"error,omitempty"`
	TaskID   string          `json:"task_id,omitempty"`
	Entities []string        `json:"entities,omitempty"` // Entity IDs referenced in output
}

// AggregatedResult is the combined result of aggregation
type AggregatedResult struct {
	Success        bool            `json:"success"`
	Output         json.RawMessage `json:"output,omitempty"`
	FailedSteps    []string        `json:"failed_steps,omitempty"`
	SuccessCount   int             `json:"success_count"`
	FailureCount   int             `json:"failure_count"`
	MergedErrors   string          `json:"merged_errors,omitempty"`
	EntityCount    int             `json:"entity_count,omitempty"` // Unique entities after dedup
	AggregatorUsed string          `json:"aggregator_used,omitempty"`
}

// Aggregator defines the interface for result aggregation strategies
type Aggregator interface {
	Name() string
	Aggregate(ctx context.Context, results []AgentResult) (*AggregatedResult, error)
}

// Registry holds registered aggregators
type Registry struct {
	mu          sync.RWMutex
	aggregators map[string]Aggregator
}

// NewRegistry creates a new aggregator registry with default aggregators
func NewRegistry() *Registry {
	r := &Registry{
		aggregators: make(map[string]Aggregator),
	}

	// Register default aggregators
	r.Register(&UnionAggregator{})
	r.Register(&FirstAggregator{})
	r.Register(&MajorityAggregator{})
	r.Register(&MergeAggregator{})
	r.Register(&EntityMergeAggregator{})

	return r
}

// Register adds an aggregator to the registry
func (r *Registry) Register(a Aggregator) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.aggregators[a.Name()] = a
}

// Get retrieves an aggregator by name
func (r *Registry) Get(name string) (Aggregator, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	a, ok := r.aggregators[name]
	return a, ok
}

// Aggregate uses the named aggregator to combine results
func (r *Registry) Aggregate(ctx context.Context, name string, results []AgentResult) (*AggregatedResult, error) {
	a, ok := r.Get(name)
	if !ok {
		return nil, fmt.Errorf("unknown aggregator: %s", name)
	}
	return a.Aggregate(ctx, results)
}

// DefaultRegistry is the global aggregator registry
var DefaultRegistry = NewRegistry()

// countResults counts successful and failed results
func countResults(results []AgentResult) (success, failure int, failed []string) {
	for _, r := range results {
		if r.Status == types.StatusSuccess {
			success++
		} else {
			failure++
			failed = append(failed, r.StepName)
		}
	}
	return
}

// collectErrors merges error messages from failed results
func collectErrors(results []AgentResult) string {
	var errors []string
	for _, r := range results {
		if r.Error != "" {
			errors = append(errors, fmt.Sprintf("%s: %s", r.StepName, r.Error))
		}
	}
	if len(errors) == 0 {
		return ""
	}
	merged := ""
	for i, e := range errors {
		if i > 0 {
			merged += "; "
		}
		merged += e
	}
	return merged
}
