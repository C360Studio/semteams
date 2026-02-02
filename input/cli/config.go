package cli

import (
	"fmt"

	"github.com/c360studio/semstreams/component"
)

// Config represents the configuration for the CLI input component
type Config struct {
	UserID     string                `json:"user_id"`
	SessionID  string                `json:"session_id"`
	Prompt     string                `json:"prompt"`
	StreamName string                `json:"stream_name"`
	Ports      *component.PortConfig `json:"ports,omitempty"`
}

// Validate validates the configuration
func (c Config) Validate() error {
	if c.UserID == "" {
		return fmt.Errorf("user_id is required")
	}
	if c.SessionID == "" {
		return fmt.Errorf("session_id is required")
	}
	return nil
}

// DefaultConfig returns the default configuration
func DefaultConfig() Config {
	return Config{
		UserID:     "cli-user",
		SessionID:  "cli-session",
		Prompt:     "> ",
		StreamName: "USER",
		Ports: &component.PortConfig{
			Inputs: []component.PortDefinition{
				{
					Name:        "user.response",
					Type:        "jetstream",
					Subject:     "user.response.cli.>",
					StreamName:  "USER",
					Required:    true,
					Description: "Responses from the router",
				},
			},
			Outputs: []component.PortDefinition{
				{
					Name:        "user.message",
					Type:        "jetstream",
					Subject:     "user.message.cli.*",
					StreamName:  "USER",
					Description: "User messages to the router",
				},
				{
					Name:        "user.signal",
					Type:        "jetstream",
					Subject:     "user.signal.*",
					StreamName:  "USER",
					Description: "Control signals (cancel, etc.)",
				},
			},
		},
	}
}
