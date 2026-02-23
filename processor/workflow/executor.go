package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/pkg/errs"
	"github.com/c360studio/semstreams/processor/workflow/actions"
	wfschema "github.com/c360studio/semstreams/processor/workflow/schema"
)

// Default timeout when parsing fails
const defaultFallbackTimeout = 30 * time.Second

// baseMessageWireFormat is the JSON structure used to detect BaseMessage-wrapped responses.
// This is a simplified version that only extracts type info without full deserialization.
type baseMessageWireFormat struct {
	Type    message.Type    `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// tryUnwrapBaseMessage attempts to extract type info and payload from a BaseMessage-wrapped response.
// If the data is a valid BaseMessage with type info, returns (payload, type).
// If not a BaseMessage or type info is missing, returns (original data, nil).
// This enables type-aware interpolation for component responses while gracefully handling
// untyped responses (like LLM outputs).
func tryUnwrapBaseMessage(data json.RawMessage) (json.RawMessage, *message.Type) {
	if len(data) == 0 {
		return data, nil
	}

	var wire baseMessageWireFormat
	if err := json.Unmarshal(data, &wire); err != nil {
		return data, nil // Not valid JSON or not our format
	}

	// Check if we have valid type info (domain is required)
	if wire.Type.Domain == "" {
		return data, nil // No type info present
	}

	// Check if we have a payload (type without payload is unusual but possible)
	if len(wire.Payload) == 0 {
		return data, nil // No payload to extract
	}

	// Return the unwrapped payload with its type
	return wire.Payload, &wire.Type
}

// Executor handles step execution for workflows
type Executor struct {
	natsClient          *natsclient.Client
	execStore           *ExecutionStore
	logger              *slog.Logger
	config              Config
	metrics             *workflowMetrics
	eventPublisher      func(context.Context, event) error
	completionPublisher func(context.Context, *Execution, string) // for rules engine observability
	parallelExecutor    *ParallelStepExecutor
}

// NewExecutor creates a new step executor
func NewExecutor(
	natsClient *natsclient.Client,
	execStore *ExecutionStore,
	logger *slog.Logger,
	config Config,
	metrics *workflowMetrics,
	eventPublisher func(context.Context, event) error,
	completionPublisher func(context.Context, *Execution, string),
) *Executor {
	return &Executor{
		natsClient:          natsClient,
		execStore:           execStore,
		logger:              logger,
		config:              config,
		metrics:             metrics,
		eventPublisher:      eventPublisher,
		completionPublisher: completionPublisher,
		parallelExecutor:    NewParallelStepExecutor(natsClient, execStore, logger),
	}
}

// StartExecution begins executing a workflow
func (e *Executor) StartExecution(ctx context.Context, workflow *wfschema.Definition, exec *Execution) error {
	// Mark as running
	exec.MarkRunning()
	if err := e.execStore.Save(ctx, exec); err != nil {
		return errs.WrapTransient(err, "workflow-executor", "StartExecution", "save execution")
	}

	// Publish started event
	e.publishEvent(ctx, event{
		Type:        "started",
		ExecutionID: exec.ID,
		WorkflowID:  exec.WorkflowID,
		State:       exec.GetState(),
		Timestamp:   time.Now(),
	})

	// Start first step
	return e.executeNextStep(ctx, workflow, exec)
}

// ContinueExecution continues a workflow after a step completion
func (e *Executor) ContinueExecution(ctx context.Context, workflow *wfschema.Definition, exec *Execution, StepResult StepResult) error {
	// Record step result
	exec.RecordStepResult(StepResult.StepName, StepResult)
	if err := e.execStore.Save(ctx, exec); err != nil {
		e.logger.Error("Failed to save execution after step", "error", err)
	}

	// Publish step completed event
	e.publishEvent(ctx, event{
		Type:        "step_completed",
		ExecutionID: exec.ID,
		WorkflowID:  exec.WorkflowID,
		StepName:    StepResult.StepName,
		Iteration:   exec.GetIteration(),
		Timestamp:   time.Now(),
	})

	// Determine next step based on result
	currentStep := e.findStepByName(workflow.Steps, StepResult.StepName)
	if currentStep == nil {
		return e.failExecution(ctx, workflow, exec, fmt.Sprintf("step not found: %s", StepResult.StepName))
	}

	var nextStepName string
	if StepResult.Status == "success" {
		nextStepName = currentStep.OnSuccess
	} else {
		nextStepName = currentStep.OnFail
	}

	// Handle special next step values using constants
	switch nextStepName {
	case StepNameComplete, "":
		if StepResult.Status == "success" {
			return e.completeExecution(ctx, workflow, exec)
		}
		return e.failExecution(ctx, workflow, exec, StepResult.Error)
	case StepNameFail:
		return e.failExecution(ctx, workflow, exec, StepResult.Error)
	}

	// Check for loop back
	if e.isLoopBack(workflow.Steps, StepResult.StepName, nextStepName) {
		return e.handleLoopIteration(ctx, workflow, exec, nextStepName)
	}

	// Move to next step
	return e.executeStep(ctx, workflow, exec, nextStepName)
}

// executeNextStep executes the next step in sequence
func (e *Executor) executeNextStep(ctx context.Context, workflow *wfschema.Definition, exec *Execution) error {
	// Get a snapshot for reading
	snapshot := exec.Clone()

	if snapshot.CurrentStep >= len(workflow.Steps) {
		return e.completeExecution(ctx, workflow, exec)
	}

	step := workflow.Steps[snapshot.CurrentStep]
	return e.executeStep(ctx, workflow, exec, step.Name)
}

// executeStep executes a specific step
func (e *Executor) executeStep(ctx context.Context, workflow *wfschema.Definition, exec *Execution, stepName string) error {
	// Check timeout
	if exec.IsTimedOut() {
		return e.timeoutExecution(ctx, workflow, exec)
	}

	// Find step
	step := e.findStepByName(workflow.Steps, stepName)
	if step == nil {
		return e.failExecution(ctx, workflow, exec, fmt.Sprintf("step not found: %s", stepName))
	}

	// Update current step using thread-safe method
	for i, s := range workflow.Steps {
		if s.Name == stepName {
			exec.SetCurrentStep(i, stepName)
			break
		}
	}
	if err := e.execStore.Save(ctx, exec); err != nil {
		e.logger.Error("Failed to save execution before step", "error", err)
	}

	// Evaluate condition using a clone for safe reading
	interpolator := newInterpolator(exec.Clone())
	conditionMet, err := interpolator.EvaluateCondition(step.Condition)
	if err != nil {
		// Fail-safe: condition evaluation errors default to false (skip the step)
		e.logger.Warn("Condition evaluation error, defaulting to false (fail-safe)",
			"step", stepName,
			"error", err,
			"condition", step.Condition)
		conditionMet = false
	}

	if !conditionMet {
		// Skip this step
		e.logger.Info("Step condition not met, skipping", "step", stepName)
		result := StepResult{
			StepName:    stepName,
			Status:      "skipped",
			StartedAt:   time.Now(),
			CompletedAt: time.Now(),
			Iteration:   exec.GetIteration(),
		}
		return e.ContinueExecution(ctx, workflow, exec, result)
	}

	// Publish step started event
	e.publishEvent(ctx, event{
		Type:        "step_started",
		ExecutionID: exec.ID,
		WorkflowID:  exec.WorkflowID,
		StepName:    stepName,
		Iteration:   exec.GetIteration(),
		Timestamp:   time.Now(),
	})

	// Record metrics
	if e.metrics != nil {
		e.metrics.recordStepStarted(stepName)
	}

	// Execute the action
	return e.executeAction(ctx, workflow, exec, step, interpolator)
}

// buildStepResult creates a StepResult from an action result.
// If the output is a BaseMessage-wrapped response, extracts the payload and type info.
func buildStepResult(stepName string, result actions.Result, start time.Time, iteration int) StepResult {
	// Try to extract type info from BaseMessage wrapper
	output, outputType := tryUnwrapBaseMessage(result.Output)

	stepResult := StepResult{
		StepName:    stepName,
		Status:      "success",
		Output:      output,
		OutputType:  outputType,
		StartedAt:   start,
		CompletedAt: time.Now(),
		Duration:    result.Duration,
		Iteration:   iteration,
	}
	if !result.Success {
		stepResult.Status = "failed"
		stepResult.Error = result.Error
	}
	return stepResult
}

// executeAction executes the action for a step
func (e *Executor) executeAction(ctx context.Context, workflow *wfschema.Definition, exec *Execution, step *wfschema.StepDef, interpolator *interpolator) error {
	start := time.Now()

	// Check if this is a parallel step
	if IsParallelStep(step) {
		return e.parallelExecutor.ExecuteParallelStep(ctx, exec, step, interpolator)
	}

	// Resolve inputs and interpolate action fields (ADR-020)
	payloadRegistry := component.GlobalPayloadRegistry()
	var resolvedPayload json.RawMessage
	if len(step.Inputs) > 0 {
		// Get interface type from first input that has one (if any)
		var interfaceType string
		for _, input := range step.Inputs {
			if input.Interface != "" {
				interfaceType = input.Interface
				break
			}
		}
		var err error
		resolvedPayload, err = interpolator.ResolveInputs(step.Inputs, interfaceType, payloadRegistry)
		if err != nil {
			e.logger.Warn("Failed to resolve step inputs, falling back to action payload",
				"step", step.Name, "error", err)
		}
	}
	action := interpolator.InterpolateActionDef(step.Action, resolvedPayload)

	timeout := e.parseTimeout(action.Timeout, e.config.RequestTimeout, step.Name)
	actx := &actions.Context{
		NATSClient:  e.natsClient,
		Timeout:     timeout,
		ExecutionID: exec.ID, // Pass execution ID for callback correlation
	}
	iteration := exec.GetIteration()

	switch action.Type {
	case "call":
		a := actions.NewCallAction(action.Subject, action.Payload, timeout)
		result := a.Execute(ctx, actx)
		return e.ContinueExecution(ctx, workflow, exec, buildStepResult(step.Name, result, start, iteration))

	case "publish":
		a := actions.NewPublishAction(action.Subject, action.Payload)
		result := a.Execute(ctx, actx)
		return e.ContinueExecution(ctx, workflow, exec, buildStepResult(step.Name, result, start, iteration))

	case "publish_agent":
		a := actions.NewPublishAgentAction(action.Subject, action.Role, action.Model, action.Prompt, action.TaskID)
		result := a.Execute(ctx, actx)
		if !result.Success {
			return e.ContinueExecution(ctx, workflow, exec, buildStepResult(step.Name, result, start, iteration))
		}
		e.parkForAsyncResult(ctx, exec, step.Name, result.Output)
		return nil

	case "set_state":
		a := actions.NewSetStateAction(action.Entity, action.State)
		result := a.Execute(ctx, actx)
		return e.ContinueExecution(ctx, workflow, exec, buildStepResult(step.Name, result, start, iteration))

	case "publish_async":
		a := actions.NewPublishAsyncAction(action.Subject, action.Payload, action.TaskID)
		result := a.Execute(ctx, actx)
		if !result.Success {
			return e.ContinueExecution(ctx, workflow, exec, buildStepResult(step.Name, result, start, iteration))
		}
		e.parkForAsyncResult(ctx, exec, step.Name, result.Output)
		return nil

	default:
		return e.failExecution(ctx, workflow, exec, fmt.Sprintf("unknown action type: %s", action.Type))
	}
}

// parkForAsyncResult stores task_id for correlation and parks the workflow
// waiting for an async response via HandleAsyncStepResult.
func (e *Executor) parkForAsyncResult(ctx context.Context, exec *Execution, stepName string, output json.RawMessage) {
	var taskInfo struct {
		TaskID string `json:"task_id"`
	}
	if err := json.Unmarshal(output, &taskInfo); err == nil && taskInfo.TaskID != "" {
		exec.SetPendingTaskID(taskInfo.TaskID)
		if err := e.execStore.Save(ctx, exec); err != nil {
			e.logger.Error("Failed to save execution with pending task", "error", err)
		}
		// Store secondary index for task_id -> execution_id lookup
		if err := e.execStore.SaveTaskIndex(ctx, taskInfo.TaskID, exec.ID); err != nil {
			e.logger.Error("Failed to save task index", "error", err)
		}
		e.logger.Info("Waiting for async response",
			slog.String("step", stepName),
			slog.String("execution_id", exec.ID),
			slog.String("task_id", taskInfo.TaskID))
	} else {
		e.logger.Info("Waiting for async response",
			slog.String("step", stepName),
			slog.String("execution_id", exec.ID))
	}
}

// handleLoopIteration handles a loop back to an earlier step
func (e *Executor) handleLoopIteration(ctx context.Context, workflow *wfschema.Definition, exec *Execution, nextStepName string) error {
	maxIterations := workflow.MaxIterations
	if maxIterations == 0 {
		maxIterations = e.config.DefaultMaxIterations
	}

	iteration := exec.GetIteration()
	if iteration >= maxIterations {
		e.logger.Info("Max iterations reached",
			slog.String("execution_id", exec.ID),
			slog.Int("iterations", iteration),
			slog.Int("max", maxIterations))

		if e.metrics != nil {
			e.metrics.recordLoopMaxIterations(exec.WorkflowID)
		}

		// Check if we should complete or fail on max iterations
		// Default is to complete with the last step's result
		return e.completeExecution(ctx, workflow, exec)
	}

	// Increment iteration and continue
	exec.IncrementIteration()
	if e.metrics != nil {
		e.metrics.recordIteration(exec.WorkflowID)
	}

	e.logger.Info("Loop iteration",
		slog.String("execution_id", exec.ID),
		slog.Int("iteration", exec.GetIteration()),
		slog.String("next_step", nextStepName))

	return e.executeStep(ctx, workflow, exec, nextStepName)
}

// isLoopBack checks if transitioning to nextStep creates a loop
func (e *Executor) isLoopBack(steps []wfschema.StepDef, currentStep, nextStep string) bool {
	currentIdx := -1
	nextIdx := -1

	for i, step := range steps {
		if step.Name == currentStep {
			currentIdx = i
		}
		if step.Name == nextStep {
			nextIdx = i
		}
	}

	return nextIdx >= 0 && nextIdx <= currentIdx
}

// completeExecution marks the workflow as completed
func (e *Executor) completeExecution(ctx context.Context, workflow *wfschema.Definition, exec *Execution) error {
	exec.MarkCompleted()
	if err := e.execStore.Save(ctx, exec); err != nil {
		e.logger.Error("Failed to save completed execution", "error", err)
	}

	// Execute on_complete actions using a clone for interpolation
	interpolator := newInterpolator(exec.Clone())
	for _, action := range workflow.OnComplete {
		e.executeCompletionAction(ctx, interpolator, action)
	}

	// Publish completed event
	e.publishEvent(ctx, event{
		Type:        "completed",
		ExecutionID: exec.ID,
		WorkflowID:  exec.WorkflowID,
		State:       exec.GetState(),
		Iteration:   exec.GetIteration(),
		Timestamp:   time.Now(),
	})

	// Persist completion state for rules engine observability
	if e.completionPublisher != nil {
		e.completionPublisher(ctx, exec, "completed")
	}

	// Record metrics
	if e.metrics != nil {
		snapshot := exec.Clone()
		duration := time.Since(snapshot.StartedAt).Seconds()
		e.metrics.recordWorkflowCompleted(snapshot.WorkflowID, snapshot.Iteration, duration)
	}

	e.logger.Info("Workflow completed",
		slog.String("execution_id", exec.ID),
		slog.String("workflow_id", exec.WorkflowID),
		slog.Int("iterations", exec.GetIteration()))

	return nil
}

// failExecution marks the workflow as failed
func (e *Executor) failExecution(ctx context.Context, workflow *wfschema.Definition, exec *Execution, errMsg string) error {
	exec.MarkFailed(errMsg)
	if err := e.execStore.Save(ctx, exec); err != nil {
		e.logger.Error("Failed to save failed execution", "error", err)
	}

	// Execute on_fail actions using a clone for interpolation
	interpolator := newInterpolator(exec.Clone())
	for _, action := range workflow.OnFail {
		e.executeCompletionAction(ctx, interpolator, action)
	}

	// Publish failed event
	e.publishEvent(ctx, event{
		Type:        "failed",
		ExecutionID: exec.ID,
		WorkflowID:  exec.WorkflowID,
		State:       exec.GetState(),
		Error:       errMsg,
		Iteration:   exec.GetIteration(),
		Timestamp:   time.Now(),
	})

	// Persist completion state for rules engine observability
	if e.completionPublisher != nil {
		e.completionPublisher(ctx, exec, "failed")
	}

	// Record metrics
	if e.metrics != nil {
		snapshot := exec.Clone()
		duration := time.Since(snapshot.StartedAt).Seconds()
		e.metrics.recordWorkflowFailed(snapshot.WorkflowID, errMsg, snapshot.Iteration, duration)
	}

	e.logger.Warn("Workflow failed",
		slog.String("execution_id", exec.ID),
		slog.String("workflow_id", exec.WorkflowID),
		slog.String("error", errMsg))

	return nil
}

// timeoutExecution marks the workflow as timed out
func (e *Executor) timeoutExecution(ctx context.Context, workflow *wfschema.Definition, exec *Execution) error {
	exec.MarkTimedOut()
	if err := e.execStore.Save(ctx, exec); err != nil {
		e.logger.Error("Failed to save timed out execution", "error", err)
	}

	// Execute on_fail actions using a clone for interpolation
	interpolator := newInterpolator(exec.Clone())
	for _, action := range workflow.OnFail {
		e.executeCompletionAction(ctx, interpolator, action)
	}

	// Publish timed_out event
	e.publishEvent(ctx, event{
		Type:        "timed_out",
		ExecutionID: exec.ID,
		WorkflowID:  exec.WorkflowID,
		State:       exec.GetState(),
		Error:       "workflow timeout exceeded",
		Iteration:   exec.GetIteration(),
		Timestamp:   time.Now(),
	})

	// Persist completion state for rules engine observability
	if e.completionPublisher != nil {
		e.completionPublisher(ctx, exec, "timed_out")
	}

	// Record metrics
	if e.metrics != nil {
		snapshot := exec.Clone()
		duration := time.Since(snapshot.StartedAt).Seconds()
		e.metrics.recordWorkflowTimeout(snapshot.WorkflowID, snapshot.Iteration, duration)
	}

	e.logger.Warn("Workflow timed out",
		slog.String("execution_id", exec.ID),
		slog.String("workflow_id", exec.WorkflowID))

	return nil
}

// executeCompletionAction executes an on_complete or on_fail action
func (e *Executor) executeCompletionAction(ctx context.Context, interpolator *interpolator, actionDef wfschema.ActionDef) {
	// Interpolate action fields - completion actions don't have step inputs
	action := interpolator.InterpolateActionDef(actionDef, nil)

	timeout := e.parseTimeout(action.Timeout, e.config.RequestTimeout, "completion_action")

	actx := &actions.Context{
		NATSClient: e.natsClient,
		Timeout:    timeout,
	}

	var a actions.Action
	switch action.Type {
	case "publish":
		a = actions.NewPublishAction(action.Subject, action.Payload)
	case "publish_agent":
		a = actions.NewPublishAgentAction(action.Subject, action.Role, action.Model, action.Prompt, action.TaskID)
	default:
		e.logger.Warn("Unsupported completion action type", "type", action.Type)
		return
	}

	result := a.Execute(ctx, actx)
	if !result.Success {
		e.logger.Warn("Completion action failed", "type", action.Type, "error", result.Error)
	}
}

// findStepByName finds a step by name in the step list
func (e *Executor) findStepByName(steps []wfschema.StepDef, name string) *wfschema.StepDef {
	for i := range steps {
		if steps[i].Name == name {
			return &steps[i]
		}
	}
	return nil
}

// parseTimeout parses a timeout string, falling back to default with logging
func (e *Executor) parseTimeout(timeout string, defaultTimeout string, context string) time.Duration {
	if timeout != "" {
		d, err := time.ParseDuration(timeout)
		if err != nil {
			e.logger.Warn("Failed to parse timeout, using default",
				slog.String("context", context),
				slog.String("timeout", timeout),
				slog.String("error", err.Error()),
				slog.String("fallback", defaultTimeout))
		} else {
			return d
		}
	}

	if d, err := time.ParseDuration(defaultTimeout); err == nil {
		return d
	}

	e.logger.Warn("Failed to parse default timeout, using hardcoded fallback",
		slog.String("context", context),
		slog.String("default_timeout", defaultTimeout),
		slog.Duration("fallback", defaultFallbackTimeout))

	return defaultFallbackTimeout
}

// publishEvent publishes a workflow event
func (e *Executor) publishEvent(ctx context.Context, event event) {
	if e.eventPublisher != nil {
		if err := e.eventPublisher(ctx, event); err != nil {
			e.logger.Warn("Failed to publish workflow event", "type", event.Type, "error", err)
		}
	}
}

// HandleAsyncStepResult handles an async step result message from any executor.
// This is the generic callback handler that works with agentic, HTTP, or custom executors.
func (e *Executor) HandleAsyncStepResult(ctx context.Context, registry *Registry, execID string, output json.RawMessage, stepError string) error {
	// Get execution
	exec, err := e.execStore.Get(ctx, execID)
	if err != nil {
		return errs.WrapTransient(err, "workflow-executor", "HandleAsyncStepResult", "get execution")
	}

	if exec.GetState().IsTerminal() {
		return nil // Already completed
	}

	// Clear pending task ID and secondary index
	if exec.PendingTaskID != "" {
		_ = e.execStore.DeleteTaskIndex(ctx, exec.PendingTaskID)
		exec.ClearPendingTaskID()
	}

	// Get workflow
	workflow, ok := registry.Get(exec.WorkflowID)
	if !ok {
		return errs.WrapInvalid(fmt.Errorf("workflow not found: %s", exec.WorkflowID), "workflow-executor", "HandleAgentComplete", "get workflow")
	}

	// Try to extract type info from BaseMessage wrapper
	unwrappedOutput, outputType := tryUnwrapBaseMessage(output)

	// Build step result
	stepResult := StepResult{
		StepName:    exec.GetCurrentName(),
		Status:      "success",
		Output:      unwrappedOutput,
		OutputType:  outputType,
		StartedAt:   exec.UpdatedAt,
		CompletedAt: time.Now(),
		Duration:    time.Since(exec.UpdatedAt),
		Iteration:   exec.GetIteration(),
	}

	if stepError != "" {
		stepResult.Status = "failed"
		stepResult.Error = stepError
	}

	return e.ContinueExecution(ctx, workflow, exec, stepResult)
}

// HandleParallelStepResult handles a result from a parallel sub-task.
// Returns true if the parallel step is complete and workflow should continue.
func (e *Executor) HandleParallelStepResult(ctx context.Context, registry *Registry, taskID string, output json.RawMessage, stepError string) error {
	// Look up execution by task ID
	exec, err := e.execStore.GetByTaskID(ctx, taskID)
	if err != nil {
		return errs.WrapTransient(err, "workflow-executor", "HandleParallelStepResult", "get execution by task")
	}

	if exec.GetState().IsTerminal() {
		return nil // Already completed
	}

	// Handle the parallel result
	parentStepName, allComplete, err := e.parallelExecutor.HandleParallelResult(ctx, exec, taskID, output, stepError)
	if err != nil {
		return err
	}

	if !allComplete {
		// Still waiting for more results
		return nil
	}

	// Get workflow
	workflow, ok := registry.Get(exec.WorkflowID)
	if !ok {
		return errs.WrapInvalid(fmt.Errorf("workflow not found: %s", exec.WorkflowID), "workflow-executor", "HandleParallelStepResult", "get workflow")
	}

	// Find the parallel step definition
	step := e.findStepByName(workflow.Steps, parentStepName)
	if step == nil {
		return fmt.Errorf("parallel step not found: %s", parentStepName)
	}

	// Aggregate results
	stepResult, err := e.parallelExecutor.AggregateResults(ctx, exec, step)
	if err != nil {
		return e.failExecution(ctx, workflow, exec, fmt.Sprintf("aggregation failed: %v", err))
	}

	// Continue workflow
	return e.ContinueExecution(ctx, workflow, exec, stepResult)
}
