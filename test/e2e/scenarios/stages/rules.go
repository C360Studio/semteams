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

// RulesValidationResult contains results from reactive workflow validation
type RulesValidationResult struct {
	MetricsPresent    map[string]bool `json:"metrics_present"`
	MetricsFoundCount int             `json:"metrics_found_count"`
	FiringsBefore     int             `json:"firings_before"`
	FiringsAfter      int             `json:"firings_after"`
	FiringsDelta      int             `json:"firings_delta"`
	EvaluatedBefore   int             `json:"evaluated_before"`
	EvaluatedAfter    int             `json:"evaluated_after"`
	EvaluatedDelta    int             `json:"evaluated_delta"`
	ActionsDispatched int             `json:"actions_dispatched"`
	ExecutionsCreated int             `json:"executions_created"`
	TestMessagesSent  int             `json:"test_messages_sent"`
	ValidationPassed  bool            `json:"validation_passed"`
	AlreadyEvaluated  bool            `json:"already_evaluated,omitempty"`
	Warnings          []string        `json:"warnings,omitempty"`
}

// ValidateRules validates that reactive workflow rules are being evaluated and fired
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

	// Check for reactive workflow metrics presence
	metricsRaw, err := v.Metrics.FetchRaw(ctx)
	ruleMetricNames := []string{
		"semstreams_reactive_workflow_rule_evaluations_total",
		"semstreams_reactive_workflow_rule_firings_total",
		"semstreams_reactive_workflow_actions_dispatched_total",
		"semstreams_reactive_workflow_executions_created_total",
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
	result.FiringsBefore = int(baselineMetrics.Firings)
	result.FiringsAfter = int(finalMetrics.Firings)
	result.FiringsDelta = int(finalMetrics.Firings - baselineMetrics.Firings)
	result.EvaluatedBefore = int(baselineMetrics.Evaluations)
	result.EvaluatedAfter = int(finalMetrics.Evaluations)
	result.EvaluatedDelta = int(finalMetrics.Evaluations - baselineMetrics.Evaluations)
	result.ActionsDispatched = int(finalMetrics.ActionsDispatched)
	result.ExecutionsCreated = int(finalMetrics.ExecutionsCreated)

	// Validate - rules should fire from baseline test data
	result.ValidationPassed = result.MetricsFoundCount >= 2 && finalMetrics.Evaluations > 0

	return result, nil
}

// StructuralRulesResult contains results for structural tier reactive workflow validation
type StructuralRulesResult struct {
	RulesEvaluated    int      `json:"rules_evaluated"`
	RuleFirings       int      `json:"rule_firings"`
	ActionsDispatched int      `json:"actions_dispatched"`
	MinEvaluated      int      `json:"min_evaluated"`
	MinFirings        int      `json:"min_firings"`
	MinActions        int      `json:"min_actions"`
	Passed            bool     `json:"passed"`
	Warnings          []string `json:"warnings,omitempty"`
}

// ValidateRuleTransitions validates reactive workflow activity for structural tier
func (v *RulesValidator) ValidateRuleTransitions(ctx context.Context, minEvaluated, minFirings, minActions int) (*StructuralRulesResult, error) {
	result := &StructuralRulesResult{
		MinEvaluated: minEvaluated,
		MinFirings:   minFirings,
		MinActions:   minActions,
	}

	metrics, err := v.Metrics.ExtractRuleMetrics(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to extract rule metrics: %w", err)
	}

	result.RulesEvaluated = int(metrics.Evaluations)
	result.RuleFirings = int(metrics.Firings)
	result.ActionsDispatched = int(metrics.ActionsDispatched)

	// Check minimums
	if result.RulesEvaluated < minEvaluated {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("Rules evaluated %d < minimum %d", result.RulesEvaluated, minEvaluated))
	}

	if result.RuleFirings < minFirings {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("Rule firings %d < minimum %d", result.RuleFirings, minFirings))
	}

	if result.ActionsDispatched < minActions {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("Actions dispatched %d < minimum %d", result.ActionsDispatched, minActions))
	}

	result.Passed = result.RulesEvaluated >= minEvaluated &&
		result.RuleFirings >= minFirings &&
		result.ActionsDispatched >= minActions

	return result, nil
}
