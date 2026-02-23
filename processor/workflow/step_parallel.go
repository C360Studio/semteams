package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/processor/workflow/actions"
	"github.com/c360studio/semstreams/processor/workflow/aggregation"
	wfschema "github.com/c360studio/semstreams/processor/workflow/schema"
)

// ParallelStepExecutor handles parallel step execution
type ParallelStepExecutor struct {
	natsClient  *natsclient.Client
	execStore   *ExecutionStore
	logger      *slog.Logger
	aggregators *aggregation.Registry
}

// NewParallelStepExecutor creates a new parallel step executor
func NewParallelStepExecutor(
	natsClient *natsclient.Client,
	execStore *ExecutionStore,
	logger *slog.Logger,
) *ParallelStepExecutor {
	return &ParallelStepExecutor{
		natsClient:  natsClient,
		execStore:   execStore,
		logger:      logger,
		aggregators: aggregation.NewRegistry(),
	}
}

// ExecuteParallelStep launches all nested steps concurrently
func (e *ParallelStepExecutor) ExecuteParallelStep(
	ctx context.Context,
	exec *Execution,
	step *wfschema.StepDef,
	interpolator *interpolator,
) error {
	if len(step.Steps) == 0 {
		return fmt.Errorf("parallel step %s has no nested steps", step.Name)
	}

	e.logger.Info("Executing parallel step",
		slog.String("step", step.Name),
		slog.Int("nested_count", len(step.Steps)),
		slog.String("wait", step.Wait),
		slog.String("aggregator", step.Aggregator))

	var wg sync.WaitGroup
	var mu sync.Mutex
	errors := make(map[string]error)

	// Launch all nested steps concurrently
	for _, nestedStep := range step.Steps {
		wg.Add(1)
		go func(nested wfschema.StepDef) {
			defer wg.Done()

			err := e.executeNestedStep(ctx, exec, step.Name, &nested, interpolator)
			if err != nil {
				mu.Lock()
				errors[nested.Name] = err
				mu.Unlock()
			}
		}(nestedStep)
	}

	// Wait for all launches to complete (not for results)
	wg.Wait()

	// Check for launch errors
	if len(errors) > 0 {
		var errMsg string
		for name, err := range errors {
			if errMsg != "" {
				errMsg += "; "
			}
			errMsg += fmt.Sprintf("%s: %v", name, err)
		}
		return fmt.Errorf("failed to launch parallel steps: %s", errMsg)
	}

	// Save execution state with pending parallel tasks
	if err := e.execStore.Save(ctx, exec); err != nil {
		e.logger.Error("Failed to save execution with parallel tasks", "error", err)
	}

	e.logger.Info("Parallel step launched, waiting for results",
		slog.String("step", step.Name),
		slog.String("execution_id", exec.ID))

	return nil
}

// executeNestedStep executes a single nested step within a parallel block
func (e *ParallelStepExecutor) executeNestedStep(
	ctx context.Context,
	exec *Execution,
	parentStepName string,
	nested *wfschema.StepDef,
	interpolator *interpolator,
) error {
	// Resolve inputs and interpolate action fields (ADR-020)
	payloadRegistry := component.GlobalPayloadRegistry()
	var resolvedPayload json.RawMessage
	if len(nested.Inputs) > 0 {
		var interfaceType string
		for _, input := range nested.Inputs {
			if input.Interface != "" {
				interfaceType = input.Interface
				break
			}
		}
		var err error
		resolvedPayload, err = interpolator.ResolveInputs(nested.Inputs, interfaceType, payloadRegistry)
		if err != nil {
			return fmt.Errorf("failed to resolve nested step inputs: %w", err)
		}
	}
	action := interpolator.InterpolateActionDef(nested.Action, resolvedPayload)

	// Build action context
	actx := &actions.Context{
		NATSClient:  e.natsClient,
		Timeout:     30 * time.Second,
		ExecutionID: exec.ID,
	}

	// Currently only publish_agent is supported for parallel nested steps
	if action.Type != "publish_agent" {
		return fmt.Errorf("parallel steps only support publish_agent actions, got: %s", action.Type)
	}

	// Execute the action
	a := actions.NewPublishAgentAction(action.Subject, action.Role, action.Model, action.Prompt, action.TaskID)
	result := a.Execute(ctx, actx)

	if !result.Success {
		return fmt.Errorf("failed to launch nested step %s: %s", nested.Name, result.Error)
	}

	// Extract task_id and track it
	var taskInfo struct {
		TaskID string `json:"task_id"`
	}
	if err := json.Unmarshal(result.Output, &taskInfo); err == nil && taskInfo.TaskID != "" {
		exec.AddPendingParallelTask(taskInfo.TaskID, parentStepName, nested.Name)

		// Store secondary index for task_id -> execution_id lookup
		if err := e.execStore.SaveTaskIndex(ctx, taskInfo.TaskID, exec.ID); err != nil {
			e.logger.Error("Failed to save task index for parallel task", "error", err)
		}

		e.logger.Debug("Parallel task launched",
			slog.String("parent_step", parentStepName),
			slog.String("nested_step", nested.Name),
			slog.String("task_id", taskInfo.TaskID))
	}

	return nil
}

// HandleParallelResult processes a result from a parallel sub-task
func (e *ParallelStepExecutor) HandleParallelResult(
	ctx context.Context,
	exec *Execution,
	taskID string,
	output json.RawMessage,
	stepError string,
) (parentStepName string, allComplete bool, err error) {
	// Try to extract type info from BaseMessage wrapper
	unwrappedOutput, outputType := tryUnwrapBaseMessage(output)

	// Build the result
	result := ParallelResult{
		Status:     "success",
		Output:     unwrappedOutput,
		OutputType: outputType,
		Duration:   0, // Duration is calculated in RecordParallelResult using task StartedAt
	}
	if stepError != "" {
		result.Status = "failed"
		result.Error = stepError
	}

	// Record the result
	parentStepName, allComplete = exec.RecordParallelResult(taskID, result)

	// Delete secondary index
	_ = e.execStore.DeleteTaskIndex(ctx, taskID)

	// Save execution state
	if err := e.execStore.Save(ctx, exec); err != nil {
		e.logger.Error("Failed to save execution after parallel result", "error", err)
	}

	e.logger.Debug("Parallel result recorded",
		slog.String("task_id", taskID),
		slog.String("parent_step", parentStepName),
		slog.Bool("all_complete", allComplete))

	return parentStepName, allComplete, nil
}

// AggregateResults combines results from all parallel sub-tasks
func (e *ParallelStepExecutor) AggregateResults(
	ctx context.Context,
	exec *Execution,
	step *wfschema.StepDef,
) (StepResult, error) {
	results := exec.GetParallelResults(step.Name)

	// Convert to aggregation.AgentResult
	agentResults := make([]aggregation.AgentResult, len(results))
	for i, r := range results {
		agentResults[i] = aggregation.AgentResult{
			StepName: r.StepName,
			Status:   r.Status,
			Output:   r.Output,
			Error:    r.Error,
			TaskID:   r.TaskID,
			Entities: r.Entities,
		}
	}

	// Determine aggregator (default to "union")
	aggregatorName := step.Aggregator
	if aggregatorName == "" {
		aggregatorName = "union"
	}

	// Aggregate results
	aggregated, err := e.aggregators.Aggregate(ctx, aggregatorName, agentResults)
	if err != nil {
		return StepResult{}, fmt.Errorf("aggregation failed: %w", err)
	}

	// Check wait semantics
	success := e.checkWaitSemantics(step.Wait, agentResults)

	// Build step result
	stepResult := StepResult{
		StepName:    step.Name,
		Status:      "success",
		Output:      aggregated.Output,
		StartedAt:   exec.UpdatedAt,
		CompletedAt: time.Now(),
		Duration:    time.Since(exec.UpdatedAt),
		Iteration:   exec.GetIteration(),
	}

	if !success {
		stepResult.Status = "failed"
		stepResult.Error = aggregated.MergedErrors
	}

	// Clean up parallel state
	exec.ClearParallelState(step.Name)

	e.logger.Info("Parallel step aggregated",
		slog.String("step", step.Name),
		slog.String("aggregator", aggregatorName),
		slog.Int("success_count", aggregated.SuccessCount),
		slog.Int("failure_count", aggregated.FailureCount),
		slog.Bool("overall_success", success))

	return stepResult, nil
}

// checkWaitSemantics determines if the parallel step succeeded based on wait semantics
func (e *ParallelStepExecutor) checkWaitSemantics(wait string, results []aggregation.AgentResult) bool {
	if len(results) == 0 {
		return false
	}

	successCount := 0
	for _, r := range results {
		if r.Status == "success" {
			successCount++
		}
	}

	switch wait {
	case "any":
		// At least one success
		return successCount > 0
	case "majority":
		// More than half succeeded
		return float64(successCount) > float64(len(results))/2
	case "all", "":
		// All must succeed (default)
		return successCount == len(results)
	default:
		// Unknown wait value, default to all
		return successCount == len(results)
	}
}

// IsParallelStep checks if a step is a parallel step
func IsParallelStep(step *wfschema.StepDef) bool {
	return step.Type == "parallel" || len(step.Steps) > 0
}
