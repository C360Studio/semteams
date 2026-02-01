package workflow

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/c360/semstreams/component"
)

// Config represents the configuration for the workflow processor
type Config struct {
	// Port configuration for inputs and outputs
	Ports *component.PortConfig `json:"ports,omitempty" schema:"type:ports,description:Port configuration for workflow inputs and outputs,category:basic"`

	// KV bucket for workflow definitions
	DefinitionsBucket string `json:"definitions_bucket" schema:"type:string,description:NATS KV bucket for workflow definitions,default:WORKFLOW_DEFINITIONS,category:advanced"`

	// KV bucket for workflow executions
	ExecutionsBucket string `json:"executions_bucket" schema:"type:string,description:NATS KV bucket for workflow execution state,default:WORKFLOW_EXECUTIONS,category:advanced"`

	// JetStream stream name for workflow messages
	StreamName string `json:"stream_name" schema:"type:string,description:JetStream stream name for workflow messages,default:WORKFLOW,category:advanced"`

	// Consumer name suffix for unique consumer identification
	ConsumerNameSuffix string `json:"consumer_name_suffix,omitempty" schema:"type:string,description:Suffix appended to consumer names for uniqueness,category:advanced"`

	// Default timeout for workflows without explicit timeout
	DefaultTimeout string `json:"default_timeout" schema:"type:string,description:Default timeout for workflows (e.g. 10m),default:10m,category:basic"`

	// Default max iterations for loop workflows
	DefaultMaxIterations int `json:"default_max_iterations" schema:"type:int,description:Default max iterations for loop workflows,default:10,min:1,max:100,category:basic"`

	// Request timeout for call actions
	RequestTimeout string `json:"request_timeout" schema:"type:string,description:Timeout for NATS request/response calls,default:30s,category:advanced"`
}

// Validate validates the configuration
func (c Config) Validate() error {
	if strings.TrimSpace(c.DefinitionsBucket) == "" {
		return fmt.Errorf("definitions_bucket is required")
	}

	if strings.TrimSpace(c.ExecutionsBucket) == "" {
		return fmt.Errorf("executions_bucket is required")
	}

	if strings.TrimSpace(c.StreamName) == "" {
		return fmt.Errorf("stream_name is required")
	}

	if strings.TrimSpace(c.DefaultTimeout) == "" {
		return fmt.Errorf("default_timeout is required")
	}

	// Parse default timeout to ensure it's valid
	duration, err := time.ParseDuration(c.DefaultTimeout)
	if err != nil {
		return fmt.Errorf("invalid default_timeout format: %w", err)
	}
	if duration <= 0 {
		return fmt.Errorf("default_timeout must be positive")
	}

	if c.DefaultMaxIterations <= 0 {
		return fmt.Errorf("default_max_iterations must be greater than 0")
	}

	if strings.TrimSpace(c.RequestTimeout) != "" {
		_, err := time.ParseDuration(c.RequestTimeout)
		if err != nil {
			return fmt.Errorf("invalid request_timeout format: %w", err)
		}
	}

	return nil
}

// DefaultConfig returns the default configuration
func DefaultConfig() Config {
	return Config{
		DefinitionsBucket:    "WORKFLOW_DEFINITIONS",
		ExecutionsBucket:     "WORKFLOW_EXECUTIONS",
		StreamName:           "WORKFLOW",
		DefaultTimeout:       "10m",
		DefaultMaxIterations: 10,
		RequestTimeout:       "30s",
		Ports: &component.PortConfig{
			Inputs: []component.PortDefinition{
				{
					Name:        "workflow.trigger",
					Type:        "jetstream",
					Subject:     "workflow.trigger.>",
					StreamName:  "WORKFLOW",
					Required:    true,
					Description: "Workflow trigger events",
				},
				{
					Name:        "workflow.step.complete",
					Type:        "jetstream",
					Subject:     "workflow.step.complete.>",
					StreamName:  "WORKFLOW",
					Required:    true,
					Description: "Step completion events from agents",
				},
				{
					Name:        "agent.complete",
					Type:        "jetstream",
					Subject:     "agent.complete.>",
					StreamName:  "AGENT",
					Required:    false,
					Description: "Agent completion events for step tracking",
				},
			},
			Outputs: []component.PortDefinition{
				{
					Name:        "workflow.events",
					Type:        "jetstream",
					Subject:     "workflow.events",
					StreamName:  "WORKFLOW",
					Description: "Workflow lifecycle events",
				},
				{
					Name:        "agent.task",
					Type:        "jetstream",
					Subject:     "agent.task.*",
					StreamName:  "AGENT",
					Description: "Agent task requests for call/publish_agent actions",
				},
			},
			KVWrite: []component.PortDefinition{
				{
					Name:        "definitions",
					Type:        "kv-write",
					Bucket:      "WORKFLOW_DEFINITIONS",
					Description: "Workflow definitions storage",
				},
				{
					Name:        "executions",
					Type:        "kv-write",
					Bucket:      "WORKFLOW_EXECUTIONS",
					Description: "Workflow execution state storage",
				},
			},
		},
	}
}

// Definition defines a workflow
type Definition struct {
	ID            string      `json:"id"`
	Name          string      `json:"name"`
	Description   string      `json:"description,omitempty"`
	Version       string      `json:"version,omitempty"`
	Enabled       bool        `json:"enabled"`
	Trigger       TriggerDef  `json:"trigger"`
	Steps         []StepDef   `json:"steps"`
	OnComplete    []ActionDef `json:"on_complete,omitempty"`
	OnFail        []ActionDef `json:"on_fail,omitempty"`
	Timeout       string      `json:"timeout,omitempty"`
	MaxIterations int         `json:"max_iterations,omitempty"`
}

// Validate validates the workflow definition
func (w *Definition) Validate() error {
	if strings.TrimSpace(w.ID) == "" {
		return fmt.Errorf("workflow id is required")
	}

	if strings.TrimSpace(w.Name) == "" {
		return fmt.Errorf("workflow name is required")
	}

	if err := w.Trigger.Validate(); err != nil {
		return fmt.Errorf("invalid trigger: %w", err)
	}

	if len(w.Steps) == 0 {
		return fmt.Errorf("workflow must have at least one step")
	}

	stepNames := make(map[string]bool)
	for i, step := range w.Steps {
		if err := step.Validate(); err != nil {
			return fmt.Errorf("invalid step[%d]: %w", i, err)
		}
		if stepNames[step.Name] {
			return fmt.Errorf("duplicate step name: %s", step.Name)
		}
		stepNames[step.Name] = true
	}

	// Validate step references
	for _, step := range w.Steps {
		if step.OnSuccess != "" && !stepNames[step.OnSuccess] && step.OnSuccess != "complete" {
			return fmt.Errorf("step %s references unknown on_success step: %s", step.Name, step.OnSuccess)
		}
		if step.OnFail != "" && !stepNames[step.OnFail] && step.OnFail != "fail" {
			return fmt.Errorf("step %s references unknown on_fail step: %s", step.Name, step.OnFail)
		}
	}

	if w.Timeout != "" {
		if _, err := time.ParseDuration(w.Timeout); err != nil {
			return fmt.Errorf("invalid timeout: %w", err)
		}
	}

	if w.MaxIterations < 0 {
		return fmt.Errorf("max_iterations cannot be negative")
	}

	for i, action := range w.OnComplete {
		if err := action.Validate(); err != nil {
			return fmt.Errorf("invalid on_complete[%d]: %w", i, err)
		}
	}

	for i, action := range w.OnFail {
		if err := action.Validate(); err != nil {
			return fmt.Errorf("invalid on_fail[%d]: %w", i, err)
		}
	}

	return nil
}

// TriggerDef defines how a workflow is triggered
type TriggerDef struct {
	Subject string `json:"subject"` // NATS subject to listen on
}

// Validate validates the trigger definition
func (t *TriggerDef) Validate() error {
	if strings.TrimSpace(t.Subject) == "" {
		return fmt.Errorf("trigger subject is required")
	}
	return nil
}

// StepDef defines a workflow step
type StepDef struct {
	Name      string        `json:"name"`
	Action    ActionDef     `json:"action"`
	Condition *ConditionDef `json:"condition,omitempty"`
	OnSuccess string        `json:"on_success,omitempty"` // Next step name or "complete"
	OnFail    string        `json:"on_fail,omitempty"`    // Next step name or "fail"
	Timeout   string        `json:"timeout,omitempty"`    // Step-specific timeout
}

// Validate validates the step definition
func (s *StepDef) Validate() error {
	if strings.TrimSpace(s.Name) == "" {
		return fmt.Errorf("step name is required")
	}

	if err := s.Action.Validate(); err != nil {
		return fmt.Errorf("invalid action: %w", err)
	}

	if s.Condition != nil {
		if err := s.Condition.Validate(); err != nil {
			return fmt.Errorf("invalid condition: %w", err)
		}
	}

	if s.Timeout != "" {
		if _, err := time.ParseDuration(s.Timeout); err != nil {
			return fmt.Errorf("invalid step timeout: %w", err)
		}
	}

	return nil
}

// ActionDef defines an action to execute
type ActionDef struct {
	Type    string          `json:"type"` // call, publish, publish_agent, set_state
	Subject string          `json:"subject,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
	Entity  string          `json:"entity,omitempty"`  // For set_state
	State   json.RawMessage `json:"state,omitempty"`   // For set_state
	Timeout string          `json:"timeout,omitempty"` // For call action
}

// Validate validates the action definition
func (a *ActionDef) Validate() error {
	validTypes := map[string]bool{
		"call":          true,
		"publish":       true,
		"publish_agent": true,
		"set_state":     true,
	}

	if !validTypes[a.Type] {
		return fmt.Errorf("invalid action type: %s (valid: call, publish, publish_agent, set_state)", a.Type)
	}

	switch a.Type {
	case "call", "publish", "publish_agent":
		if strings.TrimSpace(a.Subject) == "" {
			return fmt.Errorf("%s action requires subject", a.Type)
		}
	case "set_state":
		if strings.TrimSpace(a.Entity) == "" {
			return fmt.Errorf("set_state action requires entity")
		}
	}

	if a.Timeout != "" {
		if _, err := time.ParseDuration(a.Timeout); err != nil {
			return fmt.Errorf("invalid action timeout: %w", err)
		}
	}

	return nil
}

// ConditionDef defines a condition for step execution
type ConditionDef struct {
	Field    string `json:"field"`    // Path to field (e.g., "steps.review.output.issues_count")
	Operator string `json:"operator"` // eq, ne, gt, lt, gte, lte, exists, not_exists
	Value    any    `json:"value,omitempty"`
}

// Validate validates the condition definition
func (c *ConditionDef) Validate() error {
	if strings.TrimSpace(c.Field) == "" {
		return fmt.Errorf("condition field is required")
	}

	validOperators := map[string]bool{
		"eq":         true,
		"ne":         true,
		"gt":         true,
		"lt":         true,
		"gte":        true,
		"lte":        true,
		"exists":     true,
		"not_exists": true,
	}

	if !validOperators[c.Operator] {
		return fmt.Errorf("invalid condition operator: %s", c.Operator)
	}

	return nil
}
