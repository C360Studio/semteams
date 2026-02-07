package agentic

import "github.com/c360studio/semstreams/component"

// init registers all agentic payload types with the global PayloadRegistry.
// This enables BaseMessage.UnmarshalJSON to recreate typed payloads from JSON
// when the message type matches one of the agentic types.
func init() {
	// Register TaskMessage payload factory
	err := component.RegisterPayload(&component.PayloadRegistration{
		Domain:      "agentic",
		Category:    "task",
		Version:     "v1",
		Description: "Agent task request",
		Factory:     func() any { return &TaskMessage{} },
	})
	if err != nil {
		panic("failed to register TaskMessage payload: " + err.Error())
	}

	// Register UserMessage payload factory
	err = component.RegisterPayload(&component.PayloadRegistration{
		Domain:      "agentic",
		Category:    "user_message",
		Version:     "v1",
		Description: "User message from any channel",
		Factory:     func() any { return &UserMessage{} },
	})
	if err != nil {
		panic("failed to register UserMessage payload: " + err.Error())
	}

	// Register UserSignal payload factory
	err = component.RegisterPayload(&component.PayloadRegistration{
		Domain:      "agentic",
		Category:    "signal",
		Version:     "v1",
		Description: "User control signal",
		Factory:     func() any { return &UserSignal{} },
	})
	if err != nil {
		panic("failed to register UserSignal payload: " + err.Error())
	}

	// Register UserResponse payload factory
	err = component.RegisterPayload(&component.PayloadRegistration{
		Domain:      "agentic",
		Category:    "user_response",
		Version:     "v1",
		Description: "User response to channel",
		Factory:     func() any { return &UserResponse{} },
	})
	if err != nil {
		panic("failed to register UserResponse payload: " + err.Error())
	}

	// Register AgentResponse payload factory
	err = component.RegisterPayload(&component.PayloadRegistration{
		Domain:      "agentic",
		Category:    "response",
		Version:     "v1",
		Description: "Agent model response",
		Factory:     func() any { return &AgentResponse{} },
	})
	if err != nil {
		panic("failed to register AgentResponse payload: " + err.Error())
	}

	// Register ToolResult payload factory
	err = component.RegisterPayload(&component.PayloadRegistration{
		Domain:      "agentic",
		Category:    "tool_result",
		Version:     "v1",
		Description: "Tool execution result",
		Factory:     func() any { return &ToolResult{} },
	})
	if err != nil {
		panic("failed to register ToolResult payload: " + err.Error())
	}
}
