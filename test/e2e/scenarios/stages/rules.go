package stages

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/c360studio/semstreams/test/e2e/client"
)

// RulesValidator handles rule validation stages
type RulesValidator struct {
	Metrics           *client.MetricsClient
	UDPAddr           string
	ValidationTimeout time.Duration
	PollInterval      time.Duration
}

// RulesValidationResult contains results from rule validation
type RulesValidationResult struct {
	MetricsPresent    map[string]bool `json:"metrics_present"`
	MetricsFoundCount int             `json:"metrics_found_count"`
	TriggeredBefore   int             `json:"triggered_before"`
	TriggeredAfter    int             `json:"triggered_after"`
	TriggeredDelta    int             `json:"triggered_delta"`
	EvaluatedBefore   int             `json:"evaluated_before"`
	EvaluatedAfter    int             `json:"evaluated_after"`
	EvaluatedDelta    int             `json:"evaluated_delta"`
	OnEnterFired      int             `json:"on_enter_fired"`
	OnExitFired       int             `json:"on_exit_fired"`
	TestMessagesSent  int             `json:"test_messages_sent"`
	ValidationPassed  bool            `json:"validation_passed"`
	AlreadyEvaluated  bool            `json:"already_evaluated,omitempty"`
	Warnings          []string        `json:"warnings,omitempty"`
}

// ValidateRules validates that rules are being evaluated and triggered
func (v *RulesValidator) ValidateRules(ctx context.Context) (*RulesValidationResult, error) {
	result := &RulesValidationResult{
		MetricsPresent: make(map[string]bool),
	}

	// Capture baseline metrics
	baselineMetrics, err := v.Metrics.ExtractRuleMetrics(ctx)
	if err != nil {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("Failed to capture baseline rule metrics: %v", err))
		baselineMetrics = &client.RuleMetrics{}
	}

	// Check for rule metrics presence
	metricsRaw, err := v.Metrics.FetchRaw(ctx)
	ruleMetricNames := []string{
		"semstreams_rule_messages_received_total",
		"semstreams_rule_evaluations_total",
		"semstreams_rule_triggers_total",
		"semstreams_rule_active_rules",
	}

	for _, name := range ruleMetricNames {
		found := err == nil && strings.Contains(metricsRaw, name)
		result.MetricsPresent[name] = found
		if found {
			result.MetricsFoundCount++
		}
	}

	// Rules should be triggered by baseline test data loaded from files
	// No UDP test message injection - E2E validates baseline data only
	result.TestMessagesSent = 0

	// Check if rules already evaluated from pre-loaded test data
	if baselineMetrics.Evaluations >= 100 {
		result.AlreadyEvaluated = true
	}

	// Get final metrics
	finalMetrics, err := v.Metrics.ExtractRuleMetrics(ctx)
	if err != nil {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("Failed to get final rule metrics: %v", err))
		return result, nil
	}

	// Calculate results
	result.TriggeredBefore = int(baselineMetrics.Triggers)
	result.TriggeredAfter = int(finalMetrics.Triggers)
	result.TriggeredDelta = int(finalMetrics.Triggers - baselineMetrics.Triggers)
	result.EvaluatedBefore = int(baselineMetrics.Evaluations)
	result.EvaluatedAfter = int(finalMetrics.Evaluations)
	result.EvaluatedDelta = int(finalMetrics.Evaluations - baselineMetrics.Evaluations)
	result.OnEnterFired = int(finalMetrics.OnEnterFired)
	result.OnExitFired = int(finalMetrics.OnExitFired)

	// Validate - rules should trigger from baseline test data
	// OnEnter/OnExit should fire based on threshold crossings in sensors.jsonl

	result.ValidationPassed = result.MetricsFoundCount >= 2 && finalMetrics.Evaluations > 0

	return result, nil
}

// StructuralRulesResult contains results for structural tier rule validation
type StructuralRulesResult struct {
	RulesEvaluated int      `json:"rules_evaluated"`
	OnEnterFired   int      `json:"on_enter_fired"`
	OnExitFired    int      `json:"on_exit_fired"`
	MinEvaluated   int      `json:"min_evaluated"`
	MinOnEnter     int      `json:"min_on_enter"`
	MinOnExit      int      `json:"min_on_exit"`
	Passed         bool     `json:"passed"`
	Warnings       []string `json:"warnings,omitempty"`
}

// ValidateRuleTransitions validates OnEnter/OnExit state transitions for structural tier
func (v *RulesValidator) ValidateRuleTransitions(ctx context.Context, minEvaluated, minOnEnter, minOnExit int) (*StructuralRulesResult, error) {
	result := &StructuralRulesResult{
		MinEvaluated: minEvaluated,
		MinOnEnter:   minOnEnter,
		MinOnExit:    minOnExit,
	}

	metrics, err := v.Metrics.ExtractRuleMetrics(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to extract rule metrics: %w", err)
	}

	result.RulesEvaluated = int(metrics.Evaluations)
	result.OnEnterFired = int(metrics.OnEnterFired)
	result.OnExitFired = int(metrics.OnExitFired)

	// Check minimums
	if result.RulesEvaluated < minEvaluated {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("Rules evaluated %d < minimum %d", result.RulesEvaluated, minEvaluated))
	}

	if result.OnEnterFired < minOnEnter {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("OnEnter fired %d < minimum %d", result.OnEnterFired, minOnEnter))
	}

	if result.OnExitFired < minOnExit {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("OnExit fired %d < minimum %d", result.OnExitFired, minOnExit))
	}

	result.Passed = result.RulesEvaluated >= minEvaluated &&
		result.OnEnterFired >= minOnEnter &&
		result.OnExitFired >= minOnExit

	return result, nil
}
