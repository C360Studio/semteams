package agentic

import (
	"fmt"
	"time"

	"github.com/c360studio/semstreams/component"
)

// Builder functions for agentic payload types

func buildTaskMessage(fields map[string]any) (any, error) {
	msg := &TaskMessage{}

	if v, ok := fields["loop_id"].(string); ok {
		msg.LoopID = v
	}
	if v, ok := fields["task_id"].(string); ok {
		msg.TaskID = v
	}
	if v, ok := fields["role"].(string); ok {
		msg.Role = v
	}
	if v, ok := fields["model"].(string); ok {
		msg.Model = v
	}
	if v, ok := fields["prompt"].(string); ok {
		msg.Prompt = v
	}
	if v, ok := fields["workflow_slug"].(string); ok {
		msg.WorkflowSlug = v
	}
	if v, ok := fields["workflow_step"].(string); ok {
		msg.WorkflowStep = v
	}
	if v, ok := fields["callback"].(string); ok {
		msg.Callback = v
	}
	if v, ok := fields["channel_type"].(string); ok {
		msg.ChannelType = v
	}
	if v, ok := fields["channel_id"].(string); ok {
		msg.ChannelID = v
	}
	if v, ok := fields["user_id"].(string); ok {
		msg.UserID = v
	}

	if err := msg.Validate(); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	return msg, nil
}

func buildUserMessage(fields map[string]any) (any, error) {
	msg := &UserMessage{}

	if v, ok := fields["message_id"].(string); ok {
		msg.MessageID = v
	}
	if v, ok := fields["channel_type"].(string); ok {
		msg.ChannelType = v
	}
	if v, ok := fields["channel_id"].(string); ok {
		msg.ChannelID = v
	}
	if v, ok := fields["user_id"].(string); ok {
		msg.UserID = v
	}
	if v, ok := fields["content"].(string); ok {
		msg.Content = v
	}
	if v, ok := fields["reply_to"].(string); ok {
		msg.ReplyTo = v
	}
	if v, ok := fields["thread_id"].(string); ok {
		msg.ThreadID = v
	}
	if v, ok := fields["context_request_id"].(string); ok {
		msg.ContextRequestID = v
	}

	// Handle attachments slice
	if v, ok := fields["attachments"].([]any); ok {
		msg.Attachments = make([]Attachment, len(v))
		for i, item := range v {
			if attMap, ok := item.(map[string]any); ok {
				if typ, ok := attMap["type"].(string); ok {
					msg.Attachments[i].Type = typ
				}
				if name, ok := attMap["name"].(string); ok {
					msg.Attachments[i].Name = name
				}
				if url, ok := attMap["url"].(string); ok {
					msg.Attachments[i].URL = url
				}
				if content, ok := attMap["content"].(string); ok {
					msg.Attachments[i].Content = content
				}
				if mime, ok := attMap["mime_type"].(string); ok {
					msg.Attachments[i].MimeType = mime
				}
				if size, ok := attMap["size"].(float64); ok {
					msg.Attachments[i].Size = int64(size)
				} else if size, ok := attMap["size"].(int64); ok {
					msg.Attachments[i].Size = size
				}
			}
		}
	}

	// Handle metadata map
	if v, ok := fields["metadata"].(map[string]any); ok {
		msg.Metadata = make(map[string]string)
		for k, val := range v {
			if strVal, ok := val.(string); ok {
				msg.Metadata[k] = strVal
			}
		}
	}

	// Handle timestamp
	if v, ok := fields["timestamp"].(string); ok {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			msg.Timestamp = t
		}
	}

	if err := msg.Validate(); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	return msg, nil
}

func buildUserSignal(fields map[string]any) (any, error) {
	msg := &UserSignal{}

	if v, ok := fields["signal_id"].(string); ok {
		msg.SignalID = v
	}
	if v, ok := fields["type"].(string); ok {
		msg.Type = v
	}
	if v, ok := fields["loop_id"].(string); ok {
		msg.LoopID = v
	}
	if v, ok := fields["user_id"].(string); ok {
		msg.UserID = v
	}
	if v, ok := fields["channel_type"].(string); ok {
		msg.ChannelType = v
	}
	if v, ok := fields["channel_id"].(string); ok {
		msg.ChannelID = v
	}
	if v, ok := fields["payload"]; ok {
		msg.Payload = v
	}

	// Handle timestamp
	if v, ok := fields["timestamp"].(string); ok {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			msg.Timestamp = t
		}
	}

	if err := msg.Validate(); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	return msg, nil
}

func buildUserResponse(fields map[string]any) (any, error) {
	msg := &UserResponse{}

	if v, ok := fields["response_id"].(string); ok {
		msg.ResponseID = v
	}
	if v, ok := fields["channel_type"].(string); ok {
		msg.ChannelType = v
	}
	if v, ok := fields["channel_id"].(string); ok {
		msg.ChannelID = v
	}
	if v, ok := fields["user_id"].(string); ok {
		msg.UserID = v
	}
	if v, ok := fields["in_reply_to"].(string); ok {
		msg.InReplyTo = v
	}
	if v, ok := fields["thread_id"].(string); ok {
		msg.ThreadID = v
	}
	if v, ok := fields["type"].(string); ok {
		msg.Type = v
	}
	if v, ok := fields["content"].(string); ok {
		msg.Content = v
	}

	// Handle blocks slice
	if v, ok := fields["blocks"].([]any); ok {
		msg.Blocks = make([]ResponseBlock, len(v))
		for i, item := range v {
			if blockMap, ok := item.(map[string]any); ok {
				if typ, ok := blockMap["type"].(string); ok {
					msg.Blocks[i].Type = typ
				}
				if content, ok := blockMap["content"].(string); ok {
					msg.Blocks[i].Content = content
				}
				if lang, ok := blockMap["lang"].(string); ok {
					msg.Blocks[i].Lang = lang
				}
			}
		}
	}

	// Handle actions slice
	if v, ok := fields["actions"].([]any); ok {
		msg.Actions = make([]ResponseAction, len(v))
		for i, item := range v {
			if actionMap, ok := item.(map[string]any); ok {
				if id, ok := actionMap["id"].(string); ok {
					msg.Actions[i].ID = id
				}
				if typ, ok := actionMap["type"].(string); ok {
					msg.Actions[i].Type = typ
				}
				if label, ok := actionMap["label"].(string); ok {
					msg.Actions[i].Label = label
				}
				if signal, ok := actionMap["signal"].(string); ok {
					msg.Actions[i].Signal = signal
				}
				if style, ok := actionMap["style"].(string); ok {
					msg.Actions[i].Style = style
				}
			}
		}
	}

	// Handle timestamp
	if v, ok := fields["timestamp"].(string); ok {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			msg.Timestamp = t
		}
	}

	if err := msg.Validate(); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	return msg, nil
}

func buildAgentRequest(fields map[string]any) (any, error) {
	msg := &AgentRequest{}

	if v, ok := fields["request_id"].(string); ok {
		msg.RequestID = v
	}
	if v, ok := fields["loop_id"].(string); ok {
		msg.LoopID = v
	}
	if v, ok := fields["role"].(string); ok {
		msg.Role = v
	}
	if v, ok := fields["model"].(string); ok {
		msg.Model = v
	}

	// Handle max_tokens (JSON numbers as float64)
	if v, ok := fields["max_tokens"].(int); ok {
		msg.MaxTokens = v
	} else if v, ok := fields["max_tokens"].(float64); ok {
		msg.MaxTokens = int(v)
	}

	// Handle temperature
	if v, ok := fields["temperature"].(float64); ok {
		msg.Temperature = v
	}

	// Handle messages slice
	if v, ok := fields["messages"].([]any); ok {
		msg.Messages = make([]ChatMessage, len(v))
		for i, item := range v {
			if msgMap, ok := item.(map[string]any); ok {
				if role, ok := msgMap["role"].(string); ok {
					msg.Messages[i].Role = role
				}
				if content, ok := msgMap["content"].(string); ok {
					msg.Messages[i].Content = content
				}
				if toolCallID, ok := msgMap["tool_call_id"].(string); ok {
					msg.Messages[i].ToolCallID = toolCallID
				}
				// Handle nested tool_calls
				if toolCalls, ok := msgMap["tool_calls"].([]any); ok {
					msg.Messages[i].ToolCalls = make([]ToolCall, len(toolCalls))
					for j, tc := range toolCalls {
						if tcMap, ok := tc.(map[string]any); ok {
							if id, ok := tcMap["id"].(string); ok {
								msg.Messages[i].ToolCalls[j].ID = id
							}
							if name, ok := tcMap["name"].(string); ok {
								msg.Messages[i].ToolCalls[j].Name = name
							}
							if args, ok := tcMap["arguments"].(map[string]any); ok {
								msg.Messages[i].ToolCalls[j].Arguments = args
							}
						}
					}
				}
			}
		}
	}

	// Handle tools slice
	if v, ok := fields["tools"].([]any); ok {
		msg.Tools = make([]ToolDefinition, len(v))
		for i, item := range v {
			if toolMap, ok := item.(map[string]any); ok {
				if name, ok := toolMap["name"].(string); ok {
					msg.Tools[i].Name = name
				}
				if desc, ok := toolMap["description"].(string); ok {
					msg.Tools[i].Description = desc
				}
				if params, ok := toolMap["parameters"].(map[string]any); ok {
					msg.Tools[i].Parameters = params
				}
			}
		}
	}

	if err := msg.Validate(); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	return msg, nil
}

func buildAgentResponse(fields map[string]any) (any, error) {
	msg := &AgentResponse{}

	if v, ok := fields["request_id"].(string); ok {
		msg.RequestID = v
	}
	if v, ok := fields["status"].(string); ok {
		msg.Status = v
	}
	if v, ok := fields["error"].(string); ok {
		msg.Error = v
	}

	// Handle message (ChatMessage)
	if v, ok := fields["message"].(map[string]any); ok {
		if role, ok := v["role"].(string); ok {
			msg.Message.Role = role
		}
		if content, ok := v["content"].(string); ok {
			msg.Message.Content = content
		}
		if toolCallID, ok := v["tool_call_id"].(string); ok {
			msg.Message.ToolCallID = toolCallID
		}
		// Handle tool_calls in message
		if toolCalls, ok := v["tool_calls"].([]any); ok {
			msg.Message.ToolCalls = make([]ToolCall, len(toolCalls))
			for i, tc := range toolCalls {
				if tcMap, ok := tc.(map[string]any); ok {
					if id, ok := tcMap["id"].(string); ok {
						msg.Message.ToolCalls[i].ID = id
					}
					if name, ok := tcMap["name"].(string); ok {
						msg.Message.ToolCalls[i].Name = name
					}
					if args, ok := tcMap["arguments"].(map[string]any); ok {
						msg.Message.ToolCalls[i].Arguments = args
					}
				}
			}
		}
	}

	// Handle token_usage
	if v, ok := fields["token_usage"].(map[string]any); ok {
		if prompt, ok := v["prompt_tokens"].(float64); ok {
			msg.TokenUsage.PromptTokens = int(prompt)
		} else if prompt, ok := v["prompt_tokens"].(int); ok {
			msg.TokenUsage.PromptTokens = prompt
		}
		if completion, ok := v["completion_tokens"].(float64); ok {
			msg.TokenUsage.CompletionTokens = int(completion)
		} else if completion, ok := v["completion_tokens"].(int); ok {
			msg.TokenUsage.CompletionTokens = completion
		}
	}

	if err := msg.Validate(); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	return msg, nil
}

func buildToolCall(fields map[string]any) (any, error) {
	msg := &ToolCall{}

	if v, ok := fields["id"].(string); ok {
		msg.ID = v
	}
	if v, ok := fields["name"].(string); ok {
		msg.Name = v
	}
	if v, ok := fields["arguments"].(map[string]any); ok {
		msg.Arguments = v
	}

	if err := msg.Validate(); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	return msg, nil
}

func buildToolResult(fields map[string]any) (any, error) {
	msg := &ToolResult{}

	if v, ok := fields["call_id"].(string); ok {
		msg.CallID = v
	}
	if v, ok := fields["content"].(string); ok {
		msg.Content = v
	}
	if v, ok := fields["error"].(string); ok {
		msg.Error = v
	}
	if v, ok := fields["metadata"].(map[string]any); ok {
		msg.Metadata = v
	}

	if err := msg.Validate(); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	return msg, nil
}

func buildLoopCreatedEvent(fields map[string]any) (any, error) {
	msg := &LoopCreatedEvent{}

	if v, ok := fields["loop_id"].(string); ok {
		msg.LoopID = v
	}
	if v, ok := fields["task_id"].(string); ok {
		msg.TaskID = v
	}
	if v, ok := fields["role"].(string); ok {
		msg.Role = v
	}
	if v, ok := fields["model"].(string); ok {
		msg.Model = v
	}
	if v, ok := fields["workflow_slug"].(string); ok {
		msg.WorkflowSlug = v
	}
	if v, ok := fields["workflow_step"].(string); ok {
		msg.WorkflowStep = v
	}
	if v, ok := fields["context_request_id"].(string); ok {
		msg.ContextRequestID = v
	}

	// Handle max_iterations
	if v, ok := fields["max_iterations"].(int); ok {
		msg.MaxIterations = v
	} else if v, ok := fields["max_iterations"].(float64); ok {
		msg.MaxIterations = int(v)
	}

	// Handle created_at
	if v, ok := fields["created_at"].(string); ok {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			msg.CreatedAt = t
		}
	}

	if err := msg.Validate(); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	return msg, nil
}

func buildLoopCompletedEvent(fields map[string]any) (any, error) {
	msg := &LoopCompletedEvent{}

	if v, ok := fields["loop_id"].(string); ok {
		msg.LoopID = v
	}
	if v, ok := fields["task_id"].(string); ok {
		msg.TaskID = v
	}
	if v, ok := fields["outcome"].(string); ok {
		msg.Outcome = v
	}
	if v, ok := fields["role"].(string); ok {
		msg.Role = v
	}
	if v, ok := fields["result"].(string); ok {
		msg.Result = v
	}
	if v, ok := fields["model"].(string); ok {
		msg.Model = v
	}
	if v, ok := fields["parent_loop"].(string); ok {
		msg.ParentLoopID = v
	}

	// Handle iterations
	if v, ok := fields["iterations"].(int); ok {
		msg.Iterations = v
	} else if v, ok := fields["iterations"].(float64); ok {
		msg.Iterations = int(v)
	}

	// Handle completed_at
	if v, ok := fields["completed_at"].(string); ok {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			msg.CompletedAt = t
		}
	}

	if err := msg.Validate(); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	return msg, nil
}

func buildLoopFailedEvent(fields map[string]any) (any, error) {
	msg := &LoopFailedEvent{}

	if v, ok := fields["loop_id"].(string); ok {
		msg.LoopID = v
	}
	if v, ok := fields["task_id"].(string); ok {
		msg.TaskID = v
	}
	if v, ok := fields["outcome"].(string); ok {
		msg.Outcome = v
	}
	if v, ok := fields["reason"].(string); ok {
		msg.Reason = v
	}
	if v, ok := fields["error"].(string); ok {
		msg.Error = v
	}
	if v, ok := fields["role"].(string); ok {
		msg.Role = v
	}
	if v, ok := fields["model"].(string); ok {
		msg.Model = v
	}
	if v, ok := fields["workflow_slug"].(string); ok {
		msg.WorkflowSlug = v
	}
	if v, ok := fields["workflow_step"].(string); ok {
		msg.WorkflowStep = v
	}
	if v, ok := fields["channel_type"].(string); ok {
		msg.ChannelType = v
	}
	if v, ok := fields["channel_id"].(string); ok {
		msg.ChannelID = v
	}
	if v, ok := fields["user_id"].(string); ok {
		msg.UserID = v
	}

	// Handle iterations
	if v, ok := fields["iterations"].(int); ok {
		msg.Iterations = v
	} else if v, ok := fields["iterations"].(float64); ok {
		msg.Iterations = int(v)
	}

	// Handle failed_at
	if v, ok := fields["failed_at"].(string); ok {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			msg.FailedAt = t
		}
	}

	if err := msg.Validate(); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	return msg, nil
}

func buildLoopCancelledEvent(fields map[string]any) (any, error) {
	msg := &LoopCancelledEvent{}

	if v, ok := fields["loop_id"].(string); ok {
		msg.LoopID = v
	}
	if v, ok := fields["task_id"].(string); ok {
		msg.TaskID = v
	}
	if v, ok := fields["outcome"].(string); ok {
		msg.Outcome = v
	}
	if v, ok := fields["cancelled_by"].(string); ok {
		msg.CancelledBy = v
	}

	// Handle cancelled_at
	if v, ok := fields["cancelled_at"].(string); ok {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			msg.CancelledAt = t
		}
	}

	if err := msg.Validate(); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	return msg, nil
}

func buildContextEvent(fields map[string]any) (any, error) {
	msg := &ContextEvent{}

	if v, ok := fields["type"].(string); ok {
		msg.Type = v
	}
	if v, ok := fields["loop_id"].(string); ok {
		msg.LoopID = v
	}
	if v, ok := fields["summary"].(string); ok {
		msg.Summary = v
	}

	// Handle iteration
	if v, ok := fields["iteration"].(int); ok {
		msg.Iteration = v
	} else if v, ok := fields["iteration"].(float64); ok {
		msg.Iteration = int(v)
	}

	// Handle utilization
	if v, ok := fields["utilization"].(float64); ok {
		msg.Utilization = v
	}

	// Handle tokens_saved
	if v, ok := fields["tokens_saved"].(int); ok {
		msg.TokensSaved = v
	} else if v, ok := fields["tokens_saved"].(float64); ok {
		msg.TokensSaved = int(v)
	}

	if err := msg.Validate(); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	return msg, nil
}

// init registers all agentic payload types with the global PayloadRegistry.
// This enables BaseMessage.UnmarshalJSON to recreate typed payloads from JSON
// when the message type matches one of the agentic types.
func init() {
	// Register TaskMessage payload factory
	err := component.RegisterPayload(&component.PayloadRegistration{
		Domain:      Domain,
		Category:    CategoryTask,
		Version:     SchemaVersion,
		Description: "Agent task request",
		Factory:     func() any { return &TaskMessage{} },
		Builder:     buildTaskMessage,
	})
	if err != nil {
		panic("failed to register TaskMessage payload: " + err.Error())
	}

	// Register UserMessage payload factory
	err = component.RegisterPayload(&component.PayloadRegistration{
		Domain:      Domain,
		Category:    CategoryUserMessage,
		Version:     SchemaVersion,
		Description: "User message from any channel",
		Factory:     func() any { return &UserMessage{} },
		Builder:     buildUserMessage,
	})
	if err != nil {
		panic("failed to register UserMessage payload: " + err.Error())
	}

	// Register UserSignal payload factory
	err = component.RegisterPayload(&component.PayloadRegistration{
		Domain:      Domain,
		Category:    CategorySignal,
		Version:     SchemaVersion,
		Description: "User control signal",
		Factory:     func() any { return &UserSignal{} },
		Builder:     buildUserSignal,
	})
	if err != nil {
		panic("failed to register UserSignal payload: " + err.Error())
	}

	// Register UserResponse payload factory
	err = component.RegisterPayload(&component.PayloadRegistration{
		Domain:      Domain,
		Category:    CategoryUserResponse,
		Version:     SchemaVersion,
		Description: "User response to channel",
		Factory:     func() any { return &UserResponse{} },
		Builder:     buildUserResponse,
	})
	if err != nil {
		panic("failed to register UserResponse payload: " + err.Error())
	}

	// Register AgentResponse payload factory
	err = component.RegisterPayload(&component.PayloadRegistration{
		Domain:      Domain,
		Category:    CategoryResponse,
		Version:     SchemaVersion,
		Description: "Agent model response",
		Factory:     func() any { return &AgentResponse{} },
		Builder:     buildAgentResponse,
	})
	if err != nil {
		panic("failed to register AgentResponse payload: " + err.Error())
	}

	// Register ToolResult payload factory
	err = component.RegisterPayload(&component.PayloadRegistration{
		Domain:      Domain,
		Category:    CategoryToolResult,
		Version:     SchemaVersion,
		Description: "Tool execution result",
		Factory:     func() any { return &ToolResult{} },
		Builder:     buildToolResult,
	})
	if err != nil {
		panic("failed to register ToolResult payload: " + err.Error())
	}

	// Register AgentRequest payload factory
	err = component.RegisterPayload(&component.PayloadRegistration{
		Domain:      Domain,
		Category:    CategoryRequest,
		Version:     SchemaVersion,
		Description: "Agent model request",
		Factory:     func() any { return &AgentRequest{} },
		Builder:     buildAgentRequest,
	})
	if err != nil {
		panic("failed to register AgentRequest payload: " + err.Error())
	}

	// Register ToolCall payload factory
	err = component.RegisterPayload(&component.PayloadRegistration{
		Domain:      Domain,
		Category:    CategoryToolCall,
		Version:     SchemaVersion,
		Description: "Tool call request",
		Factory:     func() any { return &ToolCall{} },
		Builder:     buildToolCall,
	})
	if err != nil {
		panic("failed to register ToolCall payload: " + err.Error())
	}

	// Register LoopCreatedEvent payload factory
	err = component.RegisterPayload(&component.PayloadRegistration{
		Domain:      Domain,
		Category:    CategoryLoopCreated,
		Version:     SchemaVersion,
		Description: "Loop creation event",
		Factory:     func() any { return &LoopCreatedEvent{} },
		Builder:     buildLoopCreatedEvent,
	})
	if err != nil {
		panic("failed to register LoopCreatedEvent payload: " + err.Error())
	}

	// Register LoopCompletedEvent payload factory
	err = component.RegisterPayload(&component.PayloadRegistration{
		Domain:      Domain,
		Category:    CategoryLoopCompleted,
		Version:     SchemaVersion,
		Description: "Loop completion event",
		Factory:     func() any { return &LoopCompletedEvent{} },
		Builder:     buildLoopCompletedEvent,
	})
	if err != nil {
		panic("failed to register LoopCompletedEvent payload: " + err.Error())
	}

	// Register LoopFailedEvent payload factory
	err = component.RegisterPayload(&component.PayloadRegistration{
		Domain:      Domain,
		Category:    CategoryLoopFailed,
		Version:     SchemaVersion,
		Description: "Loop failure event",
		Factory:     func() any { return &LoopFailedEvent{} },
		Builder:     buildLoopFailedEvent,
	})
	if err != nil {
		panic("failed to register LoopFailedEvent payload: " + err.Error())
	}

	// Register LoopCancelledEvent payload factory
	err = component.RegisterPayload(&component.PayloadRegistration{
		Domain:      Domain,
		Category:    CategoryLoopCancelled,
		Version:     SchemaVersion,
		Description: "Loop cancellation event",
		Factory:     func() any { return &LoopCancelledEvent{} },
		Builder:     buildLoopCancelledEvent,
	})
	if err != nil {
		panic("failed to register LoopCancelledEvent payload: " + err.Error())
	}

	// Register ContextEvent payload factory
	err = component.RegisterPayload(&component.PayloadRegistration{
		Domain:      Domain,
		Category:    CategoryContextEvent,
		Version:     SchemaVersion,
		Description: "Context management event",
		Factory:     func() any { return &ContextEvent{} },
		Builder:     buildContextEvent,
	})
	if err != nil {
		panic("failed to register ContextEvent payload: " + err.Error())
	}
}
