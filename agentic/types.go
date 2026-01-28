package agentic

import (
	"fmt"
)

// AgentRequest represents a request to an agentic service
type AgentRequest struct {
	RequestID   string           `json:"request_id"`
	LoopID      string           `json:"loop_id"`
	Role        string           `json:"role"`
	Messages    []ChatMessage    `json:"messages"`
	Model       string           `json:"model"`
	MaxTokens   int              `json:"max_tokens,omitempty"`
	Temperature float64          `json:"temperature,omitempty"`
	Tools       []ToolDefinition `json:"tools,omitempty"`
}

// Validate checks if the AgentRequest is valid
func (r AgentRequest) Validate() error {
	if r.RequestID == "" {
		return fmt.Errorf("request_id required")
	}
	if len(r.Messages) == 0 {
		return fmt.Errorf("messages cannot be empty")
	}
	if r.Role != "architect" && r.Role != "editor" && r.Role != "general" {
		return fmt.Errorf("role must be one of: architect, editor, general")
	}
	return nil
}

// AgentResponse represents a response from an agentic service
type AgentResponse struct {
	RequestID  string      `json:"request_id"`
	Status     string      `json:"status"`
	Message    ChatMessage `json:"message,omitempty"`
	Error      string      `json:"error,omitempty"`
	TokenUsage TokenUsage  `json:"token_usage,omitempty"`
}

// Validate checks if the AgentResponse is valid
func (r AgentResponse) Validate() error {
	if r.Status != "complete" && r.Status != "tool_call" && r.Status != "error" {
		return fmt.Errorf("status must be one of: complete, tool_call, error")
	}
	return nil
}

// ChatMessage represents a message in a conversation
type ChatMessage struct {
	Role      string     `json:"role"`
	Content   string     `json:"content,omitempty"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}

// Validate checks if the ChatMessage is valid
func (m ChatMessage) Validate() error {
	if m.Role != "system" && m.Role != "user" && m.Role != "assistant" && m.Role != "tool" {
		return fmt.Errorf("role must be one of: system, user, assistant, tool")
	}
	if m.Content == "" && len(m.ToolCalls) == 0 {
		return fmt.Errorf("either content or tool_calls must be present")
	}
	return nil
}

// ModelConfig represents configuration for a language model
type ModelConfig struct {
	Temperature float64 `json:"temperature,omitempty"`
	MaxTokens   int     `json:"max_tokens,omitempty"`
}

// WithDefaults returns a copy of the config with default values applied
func (c ModelConfig) WithDefaults() ModelConfig {
	result := c
	if result.Temperature == 0 {
		result.Temperature = 0.2
	}
	if result.MaxTokens == 0 {
		result.MaxTokens = 4096
	}
	return result
}

// TokenUsage tracks token consumption for a request
type TokenUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

// Total returns the total number of tokens used
func (u TokenUsage) Total() int {
	return u.PromptTokens + u.CompletionTokens
}
