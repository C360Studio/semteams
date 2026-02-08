// Package schema defines the workflow definition schema types.
// These types represent the YAML/JSON structure for defining workflows.
package schema

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

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

	// For publish_agent action
	Role   string `json:"role,omitempty"`
	Model  string `json:"model,omitempty"`
	Prompt string `json:"prompt,omitempty"`
	TaskID string `json:"task_id,omitempty"` // Optional, auto-generated if empty
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
	case "call", "publish":
		if strings.TrimSpace(a.Subject) == "" {
			return fmt.Errorf("%s action requires subject", a.Type)
		}
	case "publish_agent":
		if strings.TrimSpace(a.Subject) == "" {
			return fmt.Errorf("publish_agent action requires subject")
		}
		if strings.TrimSpace(a.Role) == "" {
			return fmt.Errorf("publish_agent action requires role")
		}
		if strings.TrimSpace(a.Model) == "" {
			return fmt.Errorf("publish_agent action requires model")
		}
		if strings.TrimSpace(a.Prompt) == "" {
			return fmt.Errorf("publish_agent action requires prompt")
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
