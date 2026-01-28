package agenticloop

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/c360/semstreams/agentic"
	"github.com/google/uuid"
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
}

// PublishedMessage represents a message published to NATS
type PublishedMessage struct {
	Subject string
	Data    []byte
}

// HandlerResult contains the results of a handler operation
type HandlerResult struct {
	LoopID               string
	EditorLoopID         string
	State                agentic.LoopState
	PublishedMessages    []PublishedMessage
	PendingTools         []string
	TrajectorySteps      []agentic.TrajectoryStep
	RetryScheduled       bool
	MaxIterationsReached bool
}

// MessageHandler handles incoming messages and coordinates loop execution
type MessageHandler struct {
	config            Config
	loopManager       *LoopManager
	trajectoryManager *TrajectoryManager
}

// NewMessageHandler creates a new MessageHandler
func NewMessageHandler(config Config) *MessageHandler {
	return &MessageHandler{
		config:            config,
		loopManager:       NewLoopManager(),
		trajectoryManager: NewTrajectoryManager(),
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

	// Create initial agent request
	request := agentic.AgentRequest{
		RequestID: uuid.New().String(),
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

	// Track request ID to loop ID mapping
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

	switch response.Status {
	case "tool_call":
		// Handle tool calls
		for _, toolCall := range response.Message.ToolCalls {
			// Add to pending tools
			err = h.loopManager.AddPendingTool(loopID, toolCall.ID)
			if err != nil {
				return result, err
			}

			// Track tool call ID to loop ID mapping
			h.loopManager.TrackToolCall(toolCall.ID, loopID)

			// Publish tool execution request as ToolCall
			toolData, err := json.Marshal(toolCall)
			if err != nil {
				return result, err
			}

			result.PublishedMessages = append(result.PublishedMessages, PublishedMessage{
				Subject: subjectToolExecute + "." + toolCall.Name,
				Data:    toolData,
			})
		}

		result.PendingTools = h.loopManager.GetPendingTools(loopID)

	case "complete":
		// Mark loop as complete
		err = h.loopManager.TransitionLoop(loopID, agentic.LoopStateComplete)
		if err != nil {
			return result, err
		}

		entity.State = agentic.LoopStateComplete
		result.State = agentic.LoopStateComplete

		// Publish completion
		completion := map[string]any{
			"loop_id": loopID,
			"task_id": entity.TaskID,
			"outcome": "success",
		}

		completionData, err := json.Marshal(completion)
		if err != nil {
			return result, err
		}

		result.PublishedMessages = append(result.PublishedMessages, PublishedMessage{
			Subject: subjectAgentComplete + "." + loopID,
			Data:    completionData,
		})

		// Check if architect role - spawn editor if so
		if entity.Role == "architect" {
			editorLoopID, err := h.spawnEditorLoop(entity)
			if err != nil {
				return result, err
			}
			result.EditorLoopID = editorLoopID

			// Create agent request for editor
			editorRequest := agentic.AgentRequest{
				RequestID: uuid.New().String(),
				LoopID:    editorLoopID,
				Role:      "editor",
				Model:     entity.Model,
				Messages: []agentic.ChatMessage{
					{
						Role:    "user",
						Content: "Implement based on architecture: " + response.Message.Content,
					},
				},
			}

			// Track request ID to loop ID mapping
			h.loopManager.TrackRequest(editorRequest.RequestID, editorLoopID)

			editorData, err := json.Marshal(editorRequest)
			if err != nil {
				return result, err
			}

			result.PublishedMessages = append(result.PublishedMessages, PublishedMessage{
				Subject: subjectAgentRequest + "." + editorLoopID,
				Data:    editorData,
			})
		}

	case "error":
		// Mark as failed
		err = h.loopManager.TransitionLoop(loopID, agentic.LoopStateFailed)
		if err != nil {
			return result, err
		}
		result.State = agentic.LoopStateFailed
	}

	return result, nil
}

// HandleToolResult processes a tool execution result
func (h *MessageHandler) HandleToolResult(ctx context.Context, loopID string, toolResult agentic.ToolResult) (HandlerResult, error) {
	// Check for cancellation before processing
	if err := ctx.Err(); err != nil {
		return HandlerResult{}, err
	}

	entity, err := h.loopManager.GetLoop(loopID)
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
	}

	// Record trajectory step
	step := agentic.TrajectoryStep{
		Timestamp:  time.Now(),
		StepType:   "tool_call",
		ToolResult: toolResult.Content,
		Duration:   defaultToolCallDurationMs,
	}
	result.TrajectorySteps = append(result.TrajectorySteps, step)

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

		// All tools complete - send next agent request
		request := agentic.AgentRequest{
			RequestID: uuid.New().String(),
			LoopID:    loopID,
			Role:      entity.Role,
			Model:     entity.Model,
			Messages: []agentic.ChatMessage{
				{
					Role:    "tool",
					Content: toolResult.Content,
				},
			},
		}

		// Track request ID to loop ID mapping
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

// spawnEditorLoop creates an editor loop from an architect completion.
// The architectEntity provides task context; the caller is responsible for
// building the editor's initial message with the architect's output.
func (h *MessageHandler) spawnEditorLoop(architectEntity agentic.LoopEntity) (string, error) {
	// Create editor loop with same task ID
	editorLoopID, err := h.loopManager.CreateLoop(
		architectEntity.TaskID,
		"editor",
		architectEntity.Model,
		architectEntity.MaxIterations,
	)
	if err != nil {
		return "", err
	}

	// Start trajectory for editor
	_, err = h.trajectoryManager.StartTrajectory(editorLoopID)
	if err != nil {
		return "", err
	}

	return editorLoopID, nil
}
