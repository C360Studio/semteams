package agenticloop

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/c360studio/semstreams/agentic"
)

// Subject patterns for NATS publishing (concrete subjects, no wildcards).
const (
	subjectAgentRequest  = "agent.request"
	subjectToolExecute   = "tool.execute"
	subjectAgentComplete = "agent.complete"
)

// Default durations for trajectory step timing (milliseconds).
// These are placeholder values until actual timing is measured.
const (
	defaultModelCallDurationMs = 100
	defaultToolCallDurationMs  = 50
)

// TaskMessage represents an incoming task request
type TaskMessage struct {
	LoopID string `json:"loop_id,omitempty"`
	TaskID string `json:"task_id"`
	Role   string `json:"role"`
	Model  string `json:"model"`
	Prompt string `json:"prompt"`

	// Workflow context (optional, set by workflow commands)
	WorkflowSlug string `json:"workflow_slug,omitempty"` // e.g., "add-user-auth"
	WorkflowStep string `json:"workflow_step,omitempty"` // e.g., "design"
}

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
	ContextEvents        []ContextEvent
	RetryScheduled       bool
	MaxIterationsReached bool
	// CompletionState contains enriched completion data for KV persistence.
	// This is populated when a loop completes and is used by component.go
	// to write to the loops bucket with key pattern COMPLETE_{loopID}.
	CompletionState map[string]any
}

// ContextEvent represents a context management event for publishing
type ContextEvent struct {
	Type        string  `json:"type"` // "compaction_starting", "compaction_complete", "gc_complete"
	LoopID      string  `json:"loop_id"`
	Iteration   int     `json:"iteration"`
	Utilization float64 `json:"utilization,omitempty"`
	TokensSaved int     `json:"tokens_saved,omitempty"`
	Summary     string  `json:"summary,omitempty"`
}

// MessageHandler handles incoming messages and coordinates loop execution
type MessageHandler struct {
	config            Config
	loopManager       *LoopManager
	trajectoryManager *TrajectoryManager
	compactor         *Compactor
}

// NewMessageHandler creates a new MessageHandler
func NewMessageHandler(config Config) *MessageHandler {
	loopManager := NewLoopManagerWithConfig(config.Context)
	return &MessageHandler{
		config:            config,
		loopManager:       loopManager,
		trajectoryManager: NewTrajectoryManager(),
		compactor:         NewCompactor(config.Context),
	}
}

// HandleTask processes an incoming task message and creates a new loop
func (h *MessageHandler) HandleTask(ctx context.Context, task TaskMessage) (HandlerResult, error) {
	// Check for cancellation before starting work
	if err := ctx.Err(); err != nil {
		return HandlerResult{}, err
	}

	// Use provided loop_id if present, otherwise create new one
	var loopID string
	var err error

	if task.LoopID != "" {
		// Loop ID provided - use it and create the loop with this ID
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

	// Set workflow context if provided (for loops created by workflow commands)
	if task.WorkflowSlug != "" || task.WorkflowStep != "" {
		_ = h.loopManager.SetWorkflowContext(loopID, task.WorkflowSlug, task.WorkflowStep)
	}

	// Set timeout if configured
	if h.config.Timeout != "" {
		timeout, parseErr := time.ParseDuration(h.config.Timeout)
		if parseErr == nil {
			_ = h.loopManager.SetTimeout(loopID, timeout)
		}
	}

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
	if cm := h.loopManager.GetContextManager(loopID); cm != nil {
		_ = cm.AddMessage(RegionRecentHistory, agentic.ChatMessage{
			Role:    "user",
			Content: task.Prompt,
		})
	}

	// Create initial agent request with structured ID for recovery
	request := agentic.AgentRequest{
		RequestID: h.loopManager.GenerateRequestID(loopID),
		LoopID:    loopID,
		Role:      task.Role,
		Model:     task.Model,
		Messages: []agentic.ChatMessage{
			{
				Role:    "user",
				Content: task.Prompt,
			},
		},
	}

	// Track request ID to loop ID mapping (cache for fast lookup)
	h.loopManager.TrackRequest(request.RequestID, loopID)

	requestData, err := json.Marshal(request)
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

	result := HandlerResult{
		LoopID: loopID,
		State:  entity.State,
		PublishedMessages: []PublishedMessage{
			{
				Subject: subjectAgentRequest + "." + loopID,
				Data:    requestData,
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
		return HandlerResult{
			LoopID: loopID,
			State:  agentic.LoopStateFailed,
		}, fmt.Errorf("loop timeout exceeded")
	}

	entity, err := h.loopManager.GetLoop(loopID)
	if err != nil {
		return HandlerResult{}, err
	}

	// Check if max iterations reached
	if entity.Iterations >= entity.MaxIterations {
		return HandlerResult{}, fmt.Errorf("max iterations (%d) reached", entity.MaxIterations)
	}

	result := HandlerResult{
		LoopID:            loopID,
		State:             entity.State,
		PublishedMessages: []PublishedMessage{},
		TrajectorySteps:   []agentic.TrajectoryStep{},
		ContextEvents:     []ContextEvent{},
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
			result.ContextEvents = append(result.ContextEvents, ContextEvent{
				Type:        "compaction_starting",
				LoopID:      loopID,
				Iteration:   entity.Iterations,
				Utilization: cm.Utilization(),
			})

			// Perform compaction
			compactResult, compactErr := h.compactor.Compact(ctx, cm)
			if compactErr == nil {
				result.ContextEvents = append(result.ContextEvents, ContextEvent{
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

		toolData, err := json.Marshal(toolCall)
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

	// Enriched completion event for rules-based orchestration.
	// Rules engine watches COMPLETE_* keys in KV and can trigger
	// follow-up actions (e.g., spawn editor when architect completes).
	completion := map[string]any{
		"loop_id":     loopID,
		"task_id":     entity.TaskID,
		"outcome":     "success",
		"role":        entity.Role,
		"result":      responseContent,
		"model":       entity.Model,
		"iterations":  entity.Iterations,
		"parent_loop": entity.ParentLoopID,
	}

	completionData, err := json.Marshal(completion)
	if err != nil {
		return err
	}
	result.PublishedMessages = append(result.PublishedMessages, PublishedMessage{
		Subject: subjectAgentComplete + "." + loopID,
		Data:    completionData,
	})

	// Pass completion state to component for KV write.
	// Component will write this to COMPLETE_{loopID} key for rules engine.
	result.CompletionState = completion

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
		return HandlerResult{
			LoopID: loopID,
			State:  agentic.LoopStateFailed,
		}, fmt.Errorf("loop timeout exceeded")
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
		ContextEvents:     []ContextEvent{},
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
		// Increment iteration counter
		err = h.loopManager.IncrementIteration(loopID)
		if err != nil {
			// Max iterations reached - mark as failed
			if transitionErr := h.loopManager.TransitionLoop(loopID, agentic.LoopStateFailed); transitionErr != nil {
				return result, fmt.Errorf("failed to transition loop to failed state: %w (original error: %v)", transitionErr, err)
			}
			result.State = agentic.LoopStateFailed
			result.MaxIterationsReached = true
			return result, nil
		}

		// Get the new iteration count for GC
		newIteration := h.loopManager.GetCurrentIteration(loopID)

		// Run GC on tool results if context management is enabled
		if cm != nil {
			evicted := cm.GCToolResults(newIteration)
			if evicted > 0 {
				result.ContextEvents = append(result.ContextEvents, ContextEvent{
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

		requestData, err := json.Marshal(request)
		if err != nil {
			return result, err
		}

		result.PublishedMessages = append(result.PublishedMessages, PublishedMessage{
			Subject: subjectAgentRequest + "." + loopID,
			Data:    requestData,
		})
	}

	return result, nil
}

// GetLoop retrieves a loop entity (for testing)
func (h *MessageHandler) GetLoop(loopID string) (agentic.LoopEntity, error) {
	return h.loopManager.GetLoop(loopID)
}

// UpdateLoop updates a loop entity
func (h *MessageHandler) UpdateLoop(entity agentic.LoopEntity) error {
	return h.loopManager.UpdateLoop(entity)
}
