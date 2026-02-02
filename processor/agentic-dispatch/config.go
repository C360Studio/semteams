package agenticdispatch

import (
	"fmt"

	"github.com/c360studio/semstreams/component"
)

// Config represents the configuration for the router processor
type Config struct {
	DefaultRole  string                `json:"default_role"`
	DefaultModel string                `json:"default_model"`
	AutoContinue bool                  `json:"auto_continue"` // Continue last loop if exists
	Permissions  PermissionConfig      `json:"permissions"`
	StreamName   string                `json:"stream_name"`
	Ports        *component.PortConfig `json:"ports,omitempty"`
}

// PermissionConfig defines permission rules for the router
type PermissionConfig struct {
	View       []string `json:"view"`        // Who can view status, loops, history
	SubmitTask []string `json:"submit_task"` // Who can submit new tasks
	CancelOwn  bool     `json:"cancel_own"`  // Users can cancel their own loops
	CancelAny  []string `json:"cancel_any"`  // Who can cancel any loop
	Approve    []string `json:"approve"`     // Who can approve results
}

// Validate validates the configuration
func (c Config) Validate() error {
	if c.DefaultRole == "" {
		return fmt.Errorf("default_role is required")
	}
	if c.DefaultModel == "" {
		return fmt.Errorf("default_model is required")
	}
	return nil
}

// DefaultConfig returns the default configuration
func DefaultConfig() Config {
	return Config{
		DefaultRole:  "general",
		DefaultModel: "qwen2.5-coder:32b",
		AutoContinue: true,
		StreamName:   "USER",
		Permissions: PermissionConfig{
			View:       []string{"*"}, // Everyone can view
			SubmitTask: []string{"*"}, // Everyone can submit
			CancelOwn:  true,          // Users can cancel their own
			CancelAny:  []string{},    // No one can cancel others by default
			Approve:    []string{"*"}, // Everyone can approve
		},
		Ports: &component.PortConfig{
			Inputs: []component.PortDefinition{
				{
					Name:        "user.message",
					Type:        "jetstream",
					Subject:     "user.message.>",
					StreamName:  "USER",
					Required:    true,
					Description: "User messages from all channels",
				},
				{
					Name:        "agent.complete",
					Type:        "jetstream",
					Subject:     "agent.complete.*",
					StreamName:  "AGENT",
					Required:    true,
					Description: "Agent task completions",
				},
			},
			Outputs: []component.PortDefinition{
				{
					Name:        "agent.task",
					Type:        "jetstream",
					Subject:     "agent.task.*",
					StreamName:  "AGENT",
					Description: "Agent task requests",
				},
				{
					Name:        "agent.signal",
					Type:        "jetstream",
					Subject:     "agent.signal.*",
					StreamName:  "AGENT",
					Description: "Agent control signals",
				},
				{
					Name:        "user.response",
					Type:        "jetstream",
					Subject:     "user.response.>",
					StreamName:  "USER",
					Description: "Responses back to users",
				},
			},
		},
	}
}
