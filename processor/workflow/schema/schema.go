// Package schema defines the workflow definition schema types.
// These types represent the YAML/JSON structure for defining workflows.
package schema

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/c360studio/semstreams/pkg/errs"
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
		return errs.WrapInvalid(fmt.Errorf("workflow id is required"), "workflow-schema", "Validate", "validate id")
	}

	if strings.TrimSpace(w.Name) == "" {
		return errs.WrapInvalid(fmt.Errorf("workflow name is required"), "workflow-schema", "Validate", "validate name")
	}

	if err := w.Trigger.Validate(); err != nil {
		return errs.WrapInvalid(err, "workflow-schema", "Validate", "validate trigger")
	}

	if len(w.Steps) == 0 {
		return errs.WrapInvalid(fmt.Errorf("workflow must have at least one step"), "workflow-schema", "Validate", "validate steps")
	}

	stepNames := make(map[string]bool)
	for i, step := range w.Steps {
		if err := step.Validate(); err != nil {
			return errs.WrapInvalid(err, "workflow-schema", "Validate", fmt.Sprintf("validate step[%d]", i))
		}
		if stepNames[step.Name] {
			return errs.WrapInvalid(fmt.Errorf("duplicate step name: %s", step.Name), "workflow-schema", "Validate", "check duplicate step names")
		}
		stepNames[step.Name] = true
	}

	// Validate step references
	for _, step := range w.Steps {
		if step.OnSuccess != "" && !stepNames[step.OnSuccess] && step.OnSuccess != "complete" {
			return errs.WrapInvalid(fmt.Errorf("step %s references unknown on_success step: %s", step.Name, step.OnSuccess), "workflow-schema", "Validate", "validate on_success reference")
		}
		if step.OnFail != "" && !stepNames[step.OnFail] && step.OnFail != "fail" {
			return errs.WrapInvalid(fmt.Errorf("step %s references unknown on_fail step: %s", step.Name, step.OnFail), "workflow-schema", "Validate", "validate on_fail reference")
		}
	}

	if w.Timeout != "" {
		if _, err := time.ParseDuration(w.Timeout); err != nil {
			return errs.WrapInvalid(err, "workflow-schema", "Validate", "parse timeout")
		}
	}

	if w.MaxIterations < 0 {
		return errs.WrapInvalid(fmt.Errorf("max_iterations cannot be negative"), "workflow-schema", "Validate", "validate max_iterations")
	}

	for i, action := range w.OnComplete {
		if err := action.Validate(); err != nil {
			return errs.WrapInvalid(err, "workflow-schema", "Validate", fmt.Sprintf("validate on_complete[%d]", i))
		}
	}

	for i, action := range w.OnFail {
		if err := action.Validate(); err != nil {
			return errs.WrapInvalid(err, "workflow-schema", "Validate", fmt.Sprintf("validate on_fail[%d]", i))
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
		return errs.WrapInvalid(fmt.Errorf("trigger subject is required"), "workflow-schema", "TriggerDef.Validate", "validate subject")
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
		return errs.WrapInvalid(fmt.Errorf("step name is required"), "workflow-schema", "StepDef.Validate", "validate name")
	}

	if err := s.Action.Validate(); err != nil {
		return errs.WrapInvalid(err, "workflow-schema", "StepDef.Validate", "validate action")
	}

	if s.Condition != nil {
		if err := s.Condition.Validate(); err != nil {
			return errs.WrapInvalid(err, "workflow-schema", "StepDef.Validate", "validate condition")
		}
	}

	if s.Timeout != "" {
		if _, err := time.ParseDuration(s.Timeout); err != nil {
			return errs.WrapInvalid(err, "workflow-schema", "StepDef.Validate", "parse step timeout")
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
		return errs.WrapInvalid(fmt.Errorf("invalid action type: %s (valid: call, publish, publish_agent, set_state)", a.Type), "workflow-schema", "ActionDef.Validate", "validate type")
	}

	switch a.Type {
	case "call", "publish":
		if strings.TrimSpace(a.Subject) == "" {
			return errs.WrapInvalid(fmt.Errorf("%s action requires subject", a.Type), "workflow-schema", "ActionDef.Validate", "validate subject")
		}
	case "publish_agent":
		if strings.TrimSpace(a.Subject) == "" {
			return errs.WrapInvalid(fmt.Errorf("publish_agent action requires subject"), "workflow-schema", "ActionDef.Validate", "validate subject")
		}
		if strings.TrimSpace(a.Role) == "" {
			return errs.WrapInvalid(fmt.Errorf("publish_agent action requires role"), "workflow-schema", "ActionDef.Validate", "validate role")
		}
		if strings.TrimSpace(a.Model) == "" {
			return errs.WrapInvalid(fmt.Errorf("publish_agent action requires model"), "workflow-schema", "ActionDef.Validate", "validate model")
		}
		if strings.TrimSpace(a.Prompt) == "" {
			return errs.WrapInvalid(fmt.Errorf("publish_agent action requires prompt"), "workflow-schema", "ActionDef.Validate", "validate prompt")
		}
	case "set_state":
		if strings.TrimSpace(a.Entity) == "" {
			return errs.WrapInvalid(fmt.Errorf("set_state action requires entity"), "workflow-schema", "ActionDef.Validate", "validate entity")
		}
	}

	if a.Timeout != "" {
		if _, err := time.ParseDuration(a.Timeout); err != nil {
			return errs.WrapInvalid(err, "workflow-schema", "ActionDef.Validate", "parse action timeout")
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
		return errs.WrapInvalid(fmt.Errorf("condition field is required"), "workflow-schema", "ConditionDef.Validate", "validate field")
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
		return errs.WrapInvalid(fmt.Errorf("invalid condition operator: %s", c.Operator), "workflow-schema", "ConditionDef.Validate", "validate operator")
	}

	return nil
}
