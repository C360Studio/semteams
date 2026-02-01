package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/c360/semstreams/natsclient"
	"github.com/c360/semstreams/processor/workflow/actions"
)

// Default timeout when parsing fails
const defaultFallbackTimeout = 30 * time.Second

// Executor handles step execution for workflows
type Executor struct {
	natsClient     *natsclient.Client
	execStore      *ExecutionStore
	logger         *slog.Logger
	config         Config
	metrics        *workflowMetrics
	eventPublisher func(context.Context, Event) error
}

// NewExecutor creates a new step executor
func NewExecutor(
	natsClient *natsclient.Client,
	execStore *ExecutionStore,
	logger *slog.Logger,
	config Config,
	metrics *workflowMetrics,
	eventPublisher func(context.Context, Event) error,
) *Executor {
	return &Executor{
		natsClient:     natsClient,
		execStore:      execStore,
		logger:         logger,
		config:         config,
		metrics:        metrics,
		eventPublisher: eventPublisher,
	}
}

// StartExecution begins executing a workflow
func (e *Executor) StartExecution(ctx context.Context, workflow *Definition, exec *Execution) error {
	// Mark as running
	exec.MarkRunning()
	if err := e.execStore.Save(ctx, exec); err != nil {
		return fmt.Errorf("failed to save execution: %w", err)
	}

	// Publish started event
	e.publishEvent(ctx, Event{
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
func (e *Executor) ContinueExecution(ctx context.Context, workflow *Definition, exec *Execution, stepResult StepResult) error {
	// Record step result
	exec.RecordStepResult(stepResult.StepName, stepResult)
	if err := e.execStore.Save(ctx, exec); err != nil {
		e.logger.Error("Failed to save execution after step", "error", err)
	}

	// Publish step completed event
	e.publishEvent(ctx, Event{
		Type:        "step_completed",
		ExecutionID: exec.ID,
		WorkflowID:  exec.WorkflowID,
		StepName:    stepResult.StepName,
		Iteration:   exec.GetIteration(),
		Timestamp:   time.Now(),
	})

	// Determine next step based on result
	currentStep := e.findStepByName(workflow.Steps, stepResult.StepName)
	if currentStep == nil {
		return e.failExecution(ctx, workflow, exec, fmt.Sprintf("step not found: %s", stepResult.StepName))
	}

	var nextStepName string
	if stepResult.Status == "success" {
		nextStepName = currentStep.OnSuccess
	} else {
		nextStepName = currentStep.OnFail
	}

	// Handle special next step values using constants
	switch nextStepName {
	case StepNameComplete, "":
		if stepResult.Status == "success" {
			return e.completeExecution(ctx, workflow, exec)
		}
		return e.failExecution(ctx, workflow, exec, stepResult.Error)
	case StepNameFail:
		return e.failExecution(ctx, workflow, exec, stepResult.Error)
	}

	// Check for loop back
	if e.isLoopBack(workflow.Steps, stepResult.StepName, nextStepName) {
		return e.handleLoopIteration(ctx, workflow, exec, nextStepName)
	}

	// Move to next step
	return e.executeStep(ctx, workflow, exec, nextStepName)
}

// executeNextStep executes the next step in sequence
func (e *Executor) executeNextStep(ctx context.Context, workflow *Definition, exec *Execution) error {
	// Get a snapshot for reading
	snapshot := exec.Clone()

	if snapshot.CurrentStep >= len(workflow.Steps) {
		return e.completeExecution(ctx, workflow, exec)
	}

	step := workflow.Steps[snapshot.CurrentStep]
	return e.executeStep(ctx, workflow, exec, step.Name)
}

// executeStep executes a specific step
func (e *Executor) executeStep(ctx context.Context, workflow *Definition, exec *Execution, stepName string) error {
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
	interpolator := NewInterpolator(exec.Clone())
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
	e.publishEvent(ctx, Event{
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

// buildStepResult creates a StepResult from an action result
func buildStepResult(stepName string, result actions.Result, start time.Time, iteration int) StepResult {
	stepResult := StepResult{
		StepName:    stepName,
		Status:      "success",
		Output:      result.Output,
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
func (e *Executor) executeAction(ctx context.Context, workflow *Definition, exec *Execution, step *StepDef, interpolator *Interpolator) error {
	start := time.Now()

	// Interpolate subject and payload
	subject, err := interpolator.InterpolateString(step.Action.Subject)
	if err != nil {
		e.logger.Warn("Subject interpolation error", "step", step.Name, "error", err)
		subject = step.Action.Subject
	}
	payload, err := interpolator.InterpolateJSON(step.Action.Payload)
	if err != nil {
		e.logger.Warn("Payload interpolation error", "step", step.Name, "error", err)
		payload = step.Action.Payload
	}

	timeout := e.parseTimeout(step.Action.Timeout, e.config.RequestTimeout, step.Name)
	actx := &actions.Context{NATSClient: e.natsClient, Timeout: timeout}
	iteration := exec.GetIteration()

	switch step.Action.Type {
	case "call":
		action := actions.NewCallAction(subject, payload, timeout)
		result := action.Execute(ctx, actx)
		return e.ContinueExecution(ctx, workflow, exec, buildStepResult(step.Name, result, start, iteration))

	case "publish":
		action := actions.NewPublishAction(subject, payload)
		result := action.Execute(ctx, actx)
		return e.ContinueExecution(ctx, workflow, exec, buildStepResult(step.Name, result, start, iteration))

	case "publish_agent":
		action := actions.NewPublishAgentAction(subject, payload)
		result := action.Execute(ctx, actx)
		if !result.Success {
			return e.ContinueExecution(ctx, workflow, exec, buildStepResult(step.Name, result, start, iteration))
		}
		e.logger.Info("Waiting for agent completion", "step", step.Name, "execution_id", exec.ID)
		return nil

	case "set_state":
		entity, err := interpolator.InterpolateString(step.Action.Entity)
		if err != nil {
			e.logger.Warn("Entity interpolation error", "step", step.Name, "error", err)
			entity = step.Action.Entity
		}
		state, err := interpolator.InterpolateJSON(step.Action.State)
		if err != nil {
			e.logger.Warn("State interpolation error", "step", step.Name, "error", err)
			state = step.Action.State
		}
		action := actions.NewSetStateAction(entity, state)
		result := action.Execute(ctx, actx)
		return e.ContinueExecution(ctx, workflow, exec, buildStepResult(step.Name, result, start, iteration))

	default:
		return e.failExecution(ctx, workflow, exec, fmt.Sprintf("unknown action type: %s", step.Action.Type))
	}
}

// handleLoopIteration handles a loop back to an earlier step
func (e *Executor) handleLoopIteration(ctx context.Context, workflow *Definition, exec *Execution, nextStepName string) error {
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
func (e *Executor) isLoopBack(steps []StepDef, currentStep, nextStep string) bool {
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
func (e *Executor) completeExecution(ctx context.Context, workflow *Definition, exec *Execution) error {
	exec.MarkCompleted()
	if err := e.execStore.Save(ctx, exec); err != nil {
		e.logger.Error("Failed to save completed execution", "error", err)
	}

	// Execute on_complete actions using a clone for interpolation
	interpolator := NewInterpolator(exec.Clone())
	for _, action := range workflow.OnComplete {
		e.executeCompletionAction(ctx, interpolator, action)
	}

	// Publish completed event
	e.publishEvent(ctx, Event{
		Type:        "completed",
		ExecutionID: exec.ID,
		WorkflowID:  exec.WorkflowID,
		State:       exec.GetState(),
		Iteration:   exec.GetIteration(),
		Timestamp:   time.Now(),
	})

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
func (e *Executor) failExecution(ctx context.Context, workflow *Definition, exec *Execution, errMsg string) error {
	exec.MarkFailed(errMsg)
	if err := e.execStore.Save(ctx, exec); err != nil {
		e.logger.Error("Failed to save failed execution", "error", err)
	}

	// Execute on_fail actions using a clone for interpolation
	interpolator := NewInterpolator(exec.Clone())
	for _, action := range workflow.OnFail {
		e.executeCompletionAction(ctx, interpolator, action)
	}

	// Publish failed event
	e.publishEvent(ctx, Event{
		Type:        "failed",
		ExecutionID: exec.ID,
		WorkflowID:  exec.WorkflowID,
		State:       exec.GetState(),
		Error:       errMsg,
		Iteration:   exec.GetIteration(),
		Timestamp:   time.Now(),
	})

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
func (e *Executor) timeoutExecution(ctx context.Context, workflow *Definition, exec *Execution) error {
	exec.MarkTimedOut()
	if err := e.execStore.Save(ctx, exec); err != nil {
		e.logger.Error("Failed to save timed out execution", "error", err)
	}

	// Execute on_fail actions using a clone for interpolation
	interpolator := NewInterpolator(exec.Clone())
	for _, action := range workflow.OnFail {
		e.executeCompletionAction(ctx, interpolator, action)
	}

	// Publish timed_out event
	e.publishEvent(ctx, Event{
		Type:        "timed_out",
		ExecutionID: exec.ID,
		WorkflowID:  exec.WorkflowID,
		State:       exec.GetState(),
		Error:       "workflow timeout exceeded",
		Iteration:   exec.GetIteration(),
		Timestamp:   time.Now(),
	})

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
func (e *Executor) executeCompletionAction(ctx context.Context, interpolator *Interpolator, actionDef ActionDef) {
	subject, _ := interpolator.InterpolateString(actionDef.Subject)
	payload, _ := interpolator.InterpolateJSON(actionDef.Payload)

	timeout := e.parseTimeout(actionDef.Timeout, e.config.RequestTimeout, "completion_action")

	actx := &actions.Context{
		NATSClient: e.natsClient,
		Timeout:    timeout,
	}

	var action actions.Action
	switch actionDef.Type {
	case "publish":
		action = actions.NewPublishAction(subject, payload)
	case "publish_agent":
		action = actions.NewPublishAgentAction(subject, payload)
	default:
		e.logger.Warn("Unsupported completion action type", "type", actionDef.Type)
		return
	}

	result := action.Execute(ctx, actx)
	if !result.Success {
		e.logger.Warn("Completion action failed", "type", actionDef.Type, "error", result.Error)
	}
}

// findStepByName finds a step by name in the step list
func (e *Executor) findStepByName(steps []StepDef, name string) *StepDef {
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
func (e *Executor) publishEvent(ctx context.Context, event Event) {
	if e.eventPublisher != nil {
		if err := e.eventPublisher(ctx, event); err != nil {
			e.logger.Warn("Failed to publish workflow event", "type", event.Type, "error", err)
		}
	}
}

// HandleAgentComplete handles an agent.complete message
func (e *Executor) HandleAgentComplete(ctx context.Context, registry *Registry, execID string, output json.RawMessage, agentError string) error {
	// Get execution
	exec, err := e.execStore.Get(ctx, execID)
	if err != nil {
		return fmt.Errorf("execution not found: %w", err)
	}

	if exec.GetState().IsTerminal() {
		return nil // Already completed
	}

	// Get workflow
	workflow, ok := registry.Get(exec.WorkflowID)
	if !ok {
		return fmt.Errorf("workflow not found: %s", exec.WorkflowID)
	}

	// Build step result
	stepResult := StepResult{
		StepName:    exec.GetCurrentName(),
		Status:      "success",
		Output:      output,
		StartedAt:   exec.UpdatedAt,
		CompletedAt: time.Now(),
		Duration:    time.Since(exec.UpdatedAt),
		Iteration:   exec.GetIteration(),
	}

	if agentError != "" {
		stepResult.Status = "failed"
		stepResult.Error = agentError
	}

	return e.ContinueExecution(ctx, workflow, exec, stepResult)
}
