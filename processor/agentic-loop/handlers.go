package agenticloop

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/pkg/errs"
)

// Subject patterns for NATS publishing (concrete subjects, no wildcards).
const (
	subjectAgentRequest  = "agent.request"
	subjectAgentCreated  = "agent.created"
	subjectAgentFailed   = "agent.failed"
	subjectToolExecute   = "tool.execute"
	subjectAgentComplete = "agent.complete"
)

// Default durations for trajectory step timing (milliseconds).
// These are placeholder values until actual timing is measured.
const (
	defaultModelCallDurationMs = 100
	defaultToolCallDurationMs  = 50
)

// TaskMessage is an alias for agentic.TaskMessage for backward compatibility.
// This allows existing code to use agenticloop.TaskMessage without modification.
type TaskMessage = agentic.TaskMessage

// PublishedMessage represents a message published to NATS
type PublishedMessage struct {
	Subject string
	Data    []byte
}

// HandlerResult contains the results of a handler operation
type HandlerResult struct {
	LoopID               string
	State                agentic.LoopState
	PublishedMessages    []PublishedMessage
	PendingTools         []string
	TrajectorySteps      []agentic.TrajectoryStep
	ContextEvents        []agentic.ContextEvent
	RetryScheduled       bool
	MaxIterationsReached bool
	// CompletionState contains enriched completion data for KV persistence.
	// This is populated when a loop completes and is used by component.go
	// to write to the loops bucket with key pattern COMPLETE_{loopID}.
	CompletionState *agentic.LoopCompletedEvent
}

// MessageHandler handles incoming messages and coordinates loop execution
type MessageHandler struct {
	config            Config
	loopManager       *LoopManager
	trajectoryManager *TrajectoryManager
	compactor         *Compactor
	logger            *slog.Logger
}

// NewMessageHandler creates a new MessageHandler
func NewMessageHandler(config Config) *MessageHandler {
	loopManager := NewLoopManagerWithConfig(config.Context)
	return &MessageHandler{
		config:            config,
		loopManager:       loopManager,
		trajectoryManager: NewTrajectoryManager(),
		compactor:         NewCompactor(config.Context),
		logger:            slog.Default(),
	}
}

// SetLogger sets the logger for the handler
func (h *MessageHandler) SetLogger(logger *slog.Logger) {
	h.logger = logger
}

// configureLoopMetadata sets optional metadata on a newly created loop.
// Logs warnings if any metadata configuration fails, but does not fail the loop creation.
func (h *MessageHandler) configureLoopMetadata(loopID string, task TaskMessage) {
	// Set depth tracking on the loop entity
	if task.Depth > 0 || task.MaxDepth > 0 {
		if err := h.loopManager.SetDepth(loopID, task.Depth+1, task.MaxDepth); err != nil {
			h.logger.Warn("failed to set depth",
				slog.String("loop_id", loopID),
				slog.String("error", err.Error()))
		}
	}

	// Set parent loop ID if provided
	if task.ParentLoopID != "" {
		if err := h.loopManager.SetParentLoopID(loopID, task.ParentLoopID); err != nil {
			h.logger.Warn("failed to set parent loop ID",
				slog.String("loop_id", loopID),
				slog.String("error", err.Error()))
		}
	}

	// Set workflow context if provided (for loops created by workflow commands)
	if task.WorkflowSlug != "" || task.WorkflowStep != "" {
		if err := h.loopManager.SetWorkflowContext(loopID, task.WorkflowSlug, task.WorkflowStep); err != nil {
			h.logger.Warn("failed to set workflow context",
				slog.String("loop_id", loopID),
				slog.String("error", err.Error()))
		}
	}

	// Set user context if provided (for error notification routing)
	if task.ChannelType != "" || task.UserID != "" {
		if err := h.loopManager.SetUserContext(loopID, task.ChannelType, task.ChannelID, task.UserID); err != nil {
			h.logger.Warn("failed to set user context",
				slog.String("loop_id", loopID),
				slog.String("error", err.Error()))
		}
	}

	// Set timeout if configured
	if h.config.Timeout != "" {
		timeout, parseErr := time.ParseDuration(h.config.Timeout)
		if parseErr == nil {
			if err := h.loopManager.SetTimeout(loopID, timeout); err != nil {
				h.logger.Warn("failed to set timeout",
					slog.String("loop_id", loopID),
					slog.String("error", err.Error()))
			}
		}
	}
}

// buildInitialMessages constructs the initial message list for an agent request.
func (h *MessageHandler) buildInitialMessages(task TaskMessage) []agentic.ChatMessage {
	var messages []agentic.ChatMessage

	// If embedded context exists, include it as system message first
	if task.Context != nil && task.Context.Content != "" {
		messages = append(messages, agentic.ChatMessage{
			Role:    "system",
			Content: fmt.Sprintf("[Context]\n%s", task.Context.Content),
		})
	}

	// Add user prompt
	messages = append(messages, agentic.ChatMessage{
		Role:    "user",
		Content: task.Prompt,
	})

	return messages
}

// HandleTask processes an incoming task message and creates a new loop
func (h *MessageHandler) HandleTask(ctx context.Context, task TaskMessage) (HandlerResult, error) {
	// Check for cancellation before starting work
	if err := ctx.Err(); err != nil {
		return HandlerResult{}, err
	}

	// Check depth limit before creating loop
	if task.MaxDepth > 0 && task.Depth >= task.MaxDepth {
		return HandlerResult{}, errs.WrapInvalid(
			fmt.Errorf("max agent depth (%d) reached, cannot spawn child agent", task.MaxDepth),
			"agentic-loop",
			"HandleTask",
			"check depth limit",
		)
	}

	// Use provided loop_id if present, otherwise create new one
	var loopID string
	var err error

	if task.LoopID != "" {
		loopID, err = h.loopManager.CreateLoopWithID(task.LoopID, task.TaskID, task.Role, task.Model, h.config.MaxIterations)
		if err != nil {
			return HandlerResult{}, err
		}
	} else {
		loopID, err = h.loopManager.CreateLoop(task.TaskID, task.Role, task.Model, h.config.MaxIterations)
		if err != nil {
			return HandlerResult{}, err
		}
	}

	// Configure optional loop metadata (depth, workflow context, user context, etc.)
	h.configureLoopMetadata(loopID, task)

	// Start trajectory
	_, err = h.trajectoryManager.StartTrajectory(loopID)
	if err != nil {
		return HandlerResult{}, err
	}

	// Get loop entity
	entity, err := h.loopManager.GetLoop(loopID)
	if err != nil {
		return HandlerResult{}, err
	}

	// Add user prompt to context manager if enabled
	cm := h.loopManager.GetContextManager(loopID)
	if cm != nil {
		_ = cm.AddMessage(RegionRecentHistory, agentic.ChatMessage{
			Role:    "user",
			Content: task.Prompt,
		})
	}

	// If embedded context is present, add it directly (skips hydration)
	if task.Context != nil && task.Context.Content != "" {
		if cm != nil {
			_ = cm.AddMessage(RegionGraphEntities, agentic.ChatMessage{
				Role:    "system",
				Content: task.Context.Content,
			})
		}
		h.logger.Debug("Using embedded context",
			slog.String("loop_id", loopID),
			slog.Int("token_count", task.Context.TokenCount),
			slog.Int("entity_count", len(task.Context.Entities)))
	}

	// Build messages for initial request
	messages := h.buildInitialMessages(task)

	// Create initial agent request with structured ID for recovery
	request := agentic.AgentRequest{
		RequestID: h.loopManager.GenerateRequestID(loopID),
		LoopID:    loopID,
		Role:      task.Role,
		Model:     task.Model,
		Messages:  messages,
	}

	// Track request ID to loop ID mapping (cache for fast lookup)
	h.loopManager.TrackRequest(request.RequestID, loopID)

	requestMsg := message.NewBaseMessage(request.Schema(), &request, "agentic-loop")
	requestData, err := json.Marshal(requestMsg)
	if err != nil {
		return HandlerResult{}, err
	}

	// Record trajectory step (duration will be updated when response arrives)
	step := agentic.TrajectoryStep{
		Timestamp: time.Now(),
		StepType:  "model_call",
		RequestID: request.RequestID,
		Prompt:    task.Prompt,
	}

	// Build loop created event for dispatch sync
	created := agentic.LoopCreatedEvent{
		LoopID:           loopID,
		TaskID:           task.TaskID,
		Role:             task.Role,
		Model:            task.Model,
		WorkflowSlug:     task.WorkflowSlug,
		WorkflowStep:     task.WorkflowStep,
		ContextRequestID: task.ContextRequestID,
		MaxIterations:    entity.MaxIterations,
		CreatedAt:        time.Now(),
	}
	createdMsg := message.NewBaseMessage(created.Schema(), &created, "agentic-loop")
	createdData, err := json.Marshal(createdMsg)
	if err != nil {
		return HandlerResult{}, err
	}

	result := HandlerResult{
		LoopID: loopID,
		State:  entity.State,
		PublishedMessages: []PublishedMessage{
			{
				Subject: subjectAgentRequest + "." + loopID,
				Data:    requestData,
			},
			{
				Subject: subjectAgentCreated + "." + loopID,
				Data:    createdData,
			},
		},
		TrajectorySteps: []agentic.TrajectoryStep{step},
	}

	return result, nil
}

// HandleModelResponse processes a model response
func (h *MessageHandler) HandleModelResponse(ctx context.Context, loopID string, response agentic.AgentResponse) (HandlerResult, error) {
	// Check for cancellation before starting work
	if err := ctx.Err(); err != nil {
		return HandlerResult{}, err
	}

	// Check for timeout before processing
	if h.loopManager.IsTimedOut(loopID) {
		_ = h.loopManager.TransitionLoop(loopID, agentic.LoopStateFailed)
		if err := h.loopManager.UpdateCompletion(loopID, agentic.OutcomeFailed, "", "loop timeout exceeded"); err != nil {
			h.logger.Warn("failed to update completion for timed out loop",
				slog.String("loop_id", loopID),
				slog.String("error", err.Error()))
		}
		result := HandlerResult{
			LoopID: loopID,
			State:  agentic.LoopStateFailed,
		}
		// Publish failure events for reactive workflows to observe
		if failMsgs, err := h.BuildFailureMessages(loopID, "timeout", "loop timeout exceeded"); err == nil {
			result.PublishedMessages = failMsgs
		}
		return result, errs.WrapFatal(fmt.Errorf("loop timeout exceeded"), "agentic-loop", "HandleModelResponse", "check timeout")
	}

	entity, err := h.loopManager.GetLoop(loopID)
	if err != nil {
		return HandlerResult{}, err
	}

	// Check if max iterations reached
	if entity.Iterations >= entity.MaxIterations {
		return HandlerResult{}, errs.WrapFatal(
			fmt.Errorf("max iterations (%d) reached", entity.MaxIterations),
			"agentic-loop",
			"HandleModelResponse",
			"check max iterations",
		)
	}

	result := HandlerResult{
		LoopID:            loopID,
		State:             entity.State,
		PublishedMessages: []PublishedMessage{},
		TrajectorySteps:   []agentic.TrajectoryStep{},
		ContextEvents:     []agentic.ContextEvent{},
	}

	// Record trajectory step
	step := agentic.TrajectoryStep{
		Timestamp: time.Now(),
		StepType:  "model_call",
		RequestID: response.RequestID,
		Response:  response.Message.Content,
		TokensIn:  response.TokenUsage.PromptTokens,
		TokensOut: response.TokenUsage.CompletionTokens,
		Duration:  defaultModelCallDurationMs,
	}
	result.TrajectorySteps = append(result.TrajectorySteps, step)

	// Add assistant response to context manager if enabled
	cm := h.loopManager.GetContextManager(loopID)
	if cm != nil && response.Message.Content != "" {
		_ = cm.AddMessage(RegionRecentHistory, agentic.ChatMessage{
			Role:    "assistant",
			Content: response.Message.Content,
		})

		// Check if compaction is needed
		if h.compactor.ShouldCompact(cm) {
			result.ContextEvents = append(result.ContextEvents, agentic.ContextEvent{
				Type:        "compaction_starting",
				LoopID:      loopID,
				Iteration:   entity.Iterations,
				Utilization: cm.Utilization(),
			})

			// Perform compaction
			compactResult, compactErr := h.compactor.Compact(ctx, cm)
			if compactErr == nil {
				result.ContextEvents = append(result.ContextEvents, agentic.ContextEvent{
					Type:        "compaction_complete",
					LoopID:      loopID,
					Iteration:   entity.Iterations,
					TokensSaved: compactResult.EvictedTokens - compactResult.NewTokens,
					Summary:     compactResult.Summary,
				})
			}
		}
	}

	switch response.Status {
	case "tool_call":
		if err := h.handleToolCallResponse(&result, loopID, response.Message.ToolCalls); err != nil {
			return result, err
		}

	case "complete":
		if err := h.handleCompleteResponse(&result, loopID, entity, response.Message.Content); err != nil {
			return result, err
		}

	case "error":
		if err := h.loopManager.TransitionLoop(loopID, agentic.LoopStateFailed); err != nil {
			return result, err
		}
		result.State = agentic.LoopStateFailed

		// Update entity with completion data for KV persistence (enables SSE delivery)
		if err := h.loopManager.UpdateCompletion(loopID, agentic.OutcomeFailed, "", response.Error); err != nil {
			h.logger.Warn("failed to update completion for model error",
				slog.String("loop_id", loopID),
				slog.String("error", err.Error()))
		}

		// Publish failure events for reactive workflows to observe
		if failMsgs, err := h.BuildFailureMessages(loopID, "model_error", response.Error); err == nil {
			result.PublishedMessages = failMsgs
		}
	}

	return result, nil
}

// handleToolCallResponse processes tool call responses
func (h *MessageHandler) handleToolCallResponse(result *HandlerResult, loopID string, toolCalls []agentic.ToolCall) error {
	for _, toolCall := range toolCalls {
		if err := h.loopManager.AddPendingTool(loopID, toolCall.ID); err != nil {
			return err
		}
		h.loopManager.TrackToolCall(toolCall.ID, loopID)

		toolMsg := message.NewBaseMessage(toolCall.Schema(), &toolCall, "agentic-loop")
		toolData, err := json.Marshal(toolMsg)
		if err != nil {
			return err
		}
		result.PublishedMessages = append(result.PublishedMessages, PublishedMessage{
			Subject: subjectToolExecute + "." + toolCall.Name,
			Data:    toolData,
		})
	}
	result.PendingTools = h.loopManager.GetPendingTools(loopID)
	return nil
}

// handleCompleteResponse processes completion responses.
// It enriches the completion event with full context for rules-based orchestration.
func (h *MessageHandler) handleCompleteResponse(result *HandlerResult, loopID string, entity agentic.LoopEntity, responseContent string) error {
	if err := h.loopManager.TransitionLoop(loopID, agentic.LoopStateComplete); err != nil {
		return err
	}
	result.State = agentic.LoopStateComplete

	// Update entity with completion data for KV persistence (enables SSE delivery)
	if err := h.loopManager.UpdateCompletion(loopID, agentic.OutcomeSuccess, responseContent, ""); err != nil {
		return err
	}

	// Enriched completion event for rules-based orchestration.
	// Rules engine watches COMPLETE_* keys in KV and can trigger
	// follow-up actions (e.g., spawn editor when architect completes).
	completion := agentic.LoopCompletedEvent{
		LoopID:       loopID,
		TaskID:       entity.TaskID,
		Outcome:      agentic.OutcomeSuccess,
		Role:         entity.Role,
		Result:       responseContent,
		Model:        entity.Model,
		Iterations:   entity.Iterations,
		ParentLoopID: entity.ParentLoopID,
		WorkflowSlug: entity.WorkflowSlug,
		WorkflowStep: entity.WorkflowStep,
		CompletedAt:  time.Now(),
	}

	completionMsg := message.NewBaseMessage(completion.Schema(), &completion, "agentic-loop")
	completionData, err := json.Marshal(completionMsg)
	if err != nil {
		return err
	}
	result.PublishedMessages = append(result.PublishedMessages, PublishedMessage{
		Subject: subjectAgentComplete + "." + loopID,
		Data:    completionData,
	})

	// Pass completion state to component for KV write.
	// Component will write this to COMPLETE_{loopID} key for rules engine.
	result.CompletionState = &completion

	return nil
}

// HandleToolResult processes a tool execution result
func (h *MessageHandler) HandleToolResult(ctx context.Context, loopID string, toolResult agentic.ToolResult) (HandlerResult, error) {
	// Check for cancellation before processing
	if err := ctx.Err(); err != nil {
		return HandlerResult{}, err
	}

	// Check for timeout before processing
	if h.loopManager.IsTimedOut(loopID) {
		_ = h.loopManager.TransitionLoop(loopID, agentic.LoopStateFailed)
		if err := h.loopManager.UpdateCompletion(loopID, agentic.OutcomeFailed, "", "loop timeout exceeded"); err != nil {
			h.logger.Warn("failed to update completion for timed out loop",
				slog.String("loop_id", loopID),
				slog.String("error", err.Error()))
		}
		result := HandlerResult{
			LoopID: loopID,
			State:  agentic.LoopStateFailed,
		}
		// Publish failure events for reactive workflows to observe
		if failMsgs, err := h.BuildFailureMessages(loopID, "timeout", "loop timeout exceeded"); err == nil {
			result.PublishedMessages = failMsgs
		}
		return result, errs.WrapFatal(fmt.Errorf("loop timeout exceeded"), "agentic-loop", "HandleToolResult", "check timeout")
	}

	entity, err := h.loopManager.GetLoop(loopID)
	if err != nil {
		return HandlerResult{}, err
	}

	// Store this tool result for accumulation
	err = h.loopManager.StoreToolResult(loopID, toolResult)
	if err != nil {
		return HandlerResult{}, err
	}

	// Remove from pending tools
	err = h.loopManager.RemovePendingTool(loopID, toolResult.CallID)
	if err != nil {
		return HandlerResult{}, err
	}

	result := HandlerResult{
		LoopID:            loopID,
		State:             entity.State,
		PendingTools:      h.loopManager.GetPendingTools(loopID),
		PublishedMessages: []PublishedMessage{},
		TrajectorySteps:   []agentic.TrajectoryStep{},
		ContextEvents:     []agentic.ContextEvent{},
	}

	// Record trajectory step
	step := agentic.TrajectoryStep{
		Timestamp:  time.Now(),
		StepType:   "tool_call",
		ToolResult: toolResult.Content,
		Duration:   defaultToolCallDurationMs,
	}
	result.TrajectorySteps = append(result.TrajectorySteps, step)

	// Add tool result to context manager if enabled
	cm := h.loopManager.GetContextManager(loopID)
	if cm != nil {
		_ = cm.AddMessage(RegionToolResults, agentic.ChatMessage{
			Role:       "tool",
			ToolCallID: toolResult.CallID,
			Content:    toolResult.Content,
		})
	}

	// Check if all tools are complete
	if h.loopManager.AllToolsComplete(loopID) {
		return h.handleToolsComplete(ctx, loopID, entity, cm, &result)
	}

	return result, nil
}

// handleToolsComplete handles the case when all pending tools have completed
func (h *MessageHandler) handleToolsComplete(
	ctx context.Context,
	loopID string,
	entity agentic.LoopEntity,
	cm *ContextManager,
	result *HandlerResult,
) (HandlerResult, error) {
	// Check for cancellation before proceeding
	if err := ctx.Err(); err != nil {
		return *result, err
	}

	// Increment iteration counter
	err := h.loopManager.IncrementIteration(loopID)
	if err != nil {
		// Max iterations reached - mark as failed
		if transitionErr := h.loopManager.TransitionLoop(loopID, agentic.LoopStateFailed); transitionErr != nil {
			return *result, errs.Wrap(transitionErr, "agentic-loop", "handleToolsComplete", fmt.Sprintf("transition loop to failed state (original error: %v)", err))
		}
		result.State = agentic.LoopStateFailed
		result.MaxIterationsReached = true

		// Update entity with completion data for KV persistence (enables SSE delivery)
		errorMsg := fmt.Sprintf("max iterations (%d) reached", entity.MaxIterations)
		if updateErr := h.loopManager.UpdateCompletion(loopID, agentic.OutcomeFailed, "", errorMsg); updateErr != nil {
			h.logger.Warn("failed to update completion for max iterations",
				slog.String("loop_id", loopID),
				slog.String("error", updateErr.Error()))
		}

		// Publish failure events for reactive workflows to observe
		if failMsgs, fErr := h.BuildFailureMessages(loopID, "max_iterations", errorMsg); fErr == nil {
			result.PublishedMessages = failMsgs
		}

		return *result, nil
	}

	// Get the new iteration count for GC
	newIteration := h.loopManager.GetCurrentIteration(loopID)

	// Run GC on tool results if context management is enabled
	if cm != nil {
		evicted := cm.GCToolResults(newIteration)
		if evicted > 0 {
			result.ContextEvents = append(result.ContextEvents, agentic.ContextEvent{
				Type:      "gc_complete",
				LoopID:    loopID,
				Iteration: newIteration,
			})
		}
	}

	// Get ALL accumulated tool results
	allResults := h.loopManager.GetAndClearToolResults(loopID)

	// Build messages with ALL tool results, each with its tool_call_id
	toolMessages := make([]agentic.ChatMessage, len(allResults))
	for i, r := range allResults {
		toolMessages[i] = agentic.ChatMessage{
			Role:       "tool",
			ToolCallID: r.CallID,
			Content:    r.Content,
		}
	}

	// Check for cancellation before building request
	if err := ctx.Err(); err != nil {
		return *result, err
	}

	// All tools complete - send next agent request with ALL results
	request := agentic.AgentRequest{
		RequestID: h.loopManager.GenerateRequestID(loopID),
		LoopID:    loopID,
		Role:      entity.Role,
		Model:     entity.Model,
		Messages:  toolMessages,
	}

	// Track request ID to loop ID mapping (cache for fast lookup)
	h.loopManager.TrackRequest(request.RequestID, loopID)

	requestMsg := message.NewBaseMessage(request.Schema(), &request, "agentic-loop")
	requestData, err := json.Marshal(requestMsg)
	if err != nil {
		return *result, err
	}

	result.PublishedMessages = append(result.PublishedMessages, PublishedMessage{
		Subject: subjectAgentRequest + "." + loopID,
		Data:    requestData,
	})

	return *result, nil
}

// BuildFailureEvent creates a failure event for publishing (public wrapper).
// Used by the component to publish failure events when handler returns errors.
func (h *MessageHandler) BuildFailureEvent(loopID, reason, errorMsg string) (PublishedMessage, error) {
	return h.buildFailureEvent(loopID, reason, errorMsg)
}

// buildFailureEvent creates a failure event for publishing.
// Returns a single failure event for reactive workflows to observe.
func (h *MessageHandler) buildFailureEvent(loopID, reason, errorMsg string) (PublishedMessage, error) {
	entity, err := h.loopManager.GetLoop(loopID)
	if err != nil {
		return PublishedMessage{}, err
	}

	failure := agentic.LoopFailedEvent{
		LoopID:       loopID,
		TaskID:       entity.TaskID,
		Outcome:      agentic.OutcomeFailed,
		Reason:       reason,
		Error:        errorMsg,
		Role:         entity.Role,
		Model:        entity.Model,
		Iterations:   entity.Iterations,
		WorkflowSlug: entity.WorkflowSlug,
		WorkflowStep: entity.WorkflowStep,
		FailedAt:     time.Now(),
		// User routing info for error notifications
		ChannelType: entity.ChannelType,
		ChannelID:   entity.ChannelID,
		UserID:      entity.UserID,
	}

	failureMsg := message.NewBaseMessage(failure.Schema(), &failure, "agentic-loop")
	data, err := json.Marshal(failureMsg)
	if err != nil {
		return PublishedMessage{}, err
	}

	return PublishedMessage{
		Subject: subjectAgentFailed + "." + loopID,
		Data:    data,
	}, nil
}

// BuildFailureMessages creates failure events for publishing.
// Returns the standard failure event for reactive workflows to observe via KV watch or subject subscription.
func (h *MessageHandler) BuildFailureMessages(loopID, reason, errorMsg string) ([]PublishedMessage, error) {
	entity, err := h.loopManager.GetLoop(loopID)
	if err != nil {
		return nil, err
	}

	failure := agentic.LoopFailedEvent{
		LoopID:       loopID,
		TaskID:       entity.TaskID,
		Outcome:      agentic.OutcomeFailed,
		Reason:       reason,
		Error:        errorMsg,
		Role:         entity.Role,
		Model:        entity.Model,
		Iterations:   entity.Iterations,
		WorkflowSlug: entity.WorkflowSlug,
		WorkflowStep: entity.WorkflowStep,
		FailedAt:     time.Now(),
		ChannelType:  entity.ChannelType,
		ChannelID:    entity.ChannelID,
		UserID:       entity.UserID,
	}

	failureMsg := message.NewBaseMessage(failure.Schema(), &failure, "agentic-loop")
	data, err := json.Marshal(failureMsg)
	if err != nil {
		return nil, err
	}

	return []PublishedMessage{{
		Subject: subjectAgentFailed + "." + loopID,
		Data:    data,
	}}, nil
}

// GetLoop retrieves a loop entity (for testing)
func (h *MessageHandler) GetLoop(loopID string) (agentic.LoopEntity, error) {
	return h.loopManager.GetLoop(loopID)
}

// UpdateLoop updates a loop entity
func (h *MessageHandler) UpdateLoop(entity agentic.LoopEntity) error {
	return h.loopManager.UpdateLoop(entity)
}

// CancelLoop atomically cancels a loop and populates completion data.
func (h *MessageHandler) CancelLoop(loopID, cancelledBy string) (agentic.LoopEntity, error) {
	return h.loopManager.CancelLoop(loopID, cancelledBy)
}
