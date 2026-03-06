package agentic

import (
	"fmt"
	"time"
)

// TrajectoryStep represents a single step in an agentic trajectory
type TrajectoryStep struct {
	Timestamp     time.Time      `json:"timestamp"`
	StepType      string         `json:"step_type"`
	RequestID     string         `json:"request_id,omitempty"`
	Prompt        string         `json:"prompt,omitempty"`
	Response      string         `json:"response,omitempty"`
	TokensIn      int            `json:"tokens_in,omitempty"`
	TokensOut     int            `json:"tokens_out,omitempty"`
	ToolName      string         `json:"tool_name,omitempty"`
	ToolArguments map[string]any `json:"tool_arguments,omitempty"`
	ToolResult    string         `json:"tool_result,omitempty"`
	Duration      int64          `json:"duration"`             // milliseconds
	Messages      []ChatMessage  `json:"messages,omitempty"`   // Full request messages (detail=full)
	ToolCalls     []ToolCall     `json:"tool_calls,omitempty"` // Assistant tool calls (detail=full)
	Model         string         `json:"model,omitempty"`      // Model used
}

// Validate checks if the TrajectoryStep is valid
func (s TrajectoryStep) Validate() error {
	if s.StepType != "model_call" && s.StepType != "tool_call" {
		return fmt.Errorf("step_type must be one of: model_call, tool_call")
	}
	if s.Timestamp.IsZero() {
		return fmt.Errorf("timestamp required")
	}
	return nil
}

// Trajectory represents the complete execution path of an agentic loop
type Trajectory struct {
	LoopID         string           `json:"loop_id"`
	StartTime      time.Time        `json:"start_time"`
	EndTime        *time.Time       `json:"end_time,omitempty"`
	Steps          []TrajectoryStep `json:"steps"`
	Outcome        string           `json:"outcome,omitempty"`
	TotalTokensIn  int              `json:"total_tokens_in"`
	TotalTokensOut int              `json:"total_tokens_out"`
	Duration       int64            `json:"duration"` // milliseconds
}

// AddStep adds a step to the trajectory and updates totals
func (t *Trajectory) AddStep(step TrajectoryStep) {
	t.Steps = append(t.Steps, step)
	t.TotalTokensIn += step.TokensIn
	t.TotalTokensOut += step.TokensOut
	t.Duration += step.Duration
}

// Complete marks the trajectory as complete and calculates final duration
func (t *Trajectory) Complete(outcome string) {
	t.Outcome = outcome
	now := time.Now()
	t.EndTime = &now
	// Calculate actual elapsed time in milliseconds
	t.Duration = now.Sub(t.StartTime).Milliseconds()
}

// NewTrajectory creates a new Trajectory with initialized values
func NewTrajectory(loopID string) Trajectory {
	return Trajectory{
		LoopID:    loopID,
		StartTime: time.Now(),
		Steps:     []TrajectoryStep{},
	}
}
