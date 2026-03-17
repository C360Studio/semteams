package agentic_test

import (
	"testing"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/message"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModelEndpointEntityID(t *testing.T) {
	t.Run("valid constructions", func(t *testing.T) {
		tests := []struct {
			name         string
			org          string
			platform     string
			endpointName string
			want         string
		}{
			{
				name:         "standard endpoint",
				org:          "c360",
				platform:     "ops",
				endpointName: "claude-sonnet",
				want:         "c360.ops.agent.model-registry.endpoint.claude-sonnet",
			},
			{
				name:         "ollama local endpoint",
				org:          "c360",
				platform:     "ops",
				endpointName: "ollama-local",
				want:         "c360.ops.agent.model-registry.endpoint.ollama-local",
			},
			{
				name:         "openai endpoint",
				org:          "acme",
				platform:     "prod",
				endpointName: "openai-gpt4",
				want:         "acme.prod.agent.model-registry.endpoint.openai-gpt4",
			},
			{
				name:         "endpoint with underscore",
				org:          "myorg",
				platform:     "staging",
				endpointName: "custom_model",
				want:         "myorg.staging.agent.model-registry.endpoint.custom_model",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got := agentic.ModelEndpointEntityID(tt.org, tt.platform, tt.endpointName)
				require.Equal(t, tt.want, got)
				assert.True(t, message.IsValidEntityID(got), "result %q must pass IsValidEntityID", got)
			})
		}
	})

	t.Run("panics on invalid input", func(t *testing.T) {
		assert.Panics(t, func() {
			agentic.ModelEndpointEntityID("", "ops", "claude-sonnet")
		}, "empty org should panic")

		assert.Panics(t, func() {
			agentic.ModelEndpointEntityID("c360", "", "claude-sonnet")
		}, "empty platform should panic")

		assert.Panics(t, func() {
			agentic.ModelEndpointEntityID("c360", "ops", "")
		}, "empty endpointName should panic")

		assert.Panics(t, func() {
			agentic.ModelEndpointEntityID("c360.studio", "ops", "claude-sonnet")
		}, "dot in org should panic")

		assert.Panics(t, func() {
			agentic.ModelEndpointEntityID("c360", "ops", "endpoint.name")
		}, "dot in endpointName should panic")

		assert.Panics(t, func() {
			agentic.ModelEndpointEntityID("c360", "ops.prod", "claude-sonnet")
		}, "dot in platform should panic")
	})
}

func TestLoopExecutionEntityID(t *testing.T) {
	t.Run("valid constructions", func(t *testing.T) {
		tests := []struct {
			name     string
			org      string
			platform string
			loopID   string
			want     string
		}{
			{
				name:     "simple alphanumeric loop ID",
				org:      "c360",
				platform: "ops",
				loopID:   "abc123",
				want:     "c360.ops.agent.agentic-loop.execution.abc123",
			},
			{
				name:     "UUID-style loop ID with hyphens",
				org:      "c360",
				platform: "ops",
				loopID:   "550e8400-e29b-41d4-a716-446655440000",
				want:     "c360.ops.agent.agentic-loop.execution.550e8400-e29b-41d4-a716-446655440000",
			},
			{
				name:     "short loop ID",
				org:      "acme",
				platform: "prod",
				loopID:   "x1",
				want:     "acme.prod.agent.agentic-loop.execution.x1",
			},
			{
				name:     "loop ID with underscores",
				org:      "myorg",
				platform: "staging",
				loopID:   "loop_42",
				want:     "myorg.staging.agent.agentic-loop.execution.loop_42",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got := agentic.LoopExecutionEntityID(tt.org, tt.platform, tt.loopID)
				require.Equal(t, tt.want, got)
				assert.True(t, message.IsValidEntityID(got), "result %q must pass IsValidEntityID", got)
			})
		}
	})

	t.Run("panics on invalid input", func(t *testing.T) {
		assert.Panics(t, func() {
			agentic.LoopExecutionEntityID("", "ops", "abc123")
		}, "empty org should panic")

		assert.Panics(t, func() {
			agentic.LoopExecutionEntityID("c360", "", "abc123")
		}, "empty platform should panic")

		assert.Panics(t, func() {
			agentic.LoopExecutionEntityID("c360", "ops", "")
		}, "empty loopID should panic")

		assert.Panics(t, func() {
			agentic.LoopExecutionEntityID("c360.studio", "ops", "abc123")
		}, "dot in org should panic")

		assert.Panics(t, func() {
			agentic.LoopExecutionEntityID("c360", "ops", "loop.1")
		}, "dot in loopID should panic")

		assert.Panics(t, func() {
			agentic.LoopExecutionEntityID("c360", "ops.prod", "abc123")
		}, "dot in platform should panic")
	})
}

func TestTrajectoryStepEntityID(t *testing.T) {
	t.Run("valid constructions", func(t *testing.T) {
		tests := []struct {
			name      string
			org       string
			platform  string
			loopID    string
			stepIndex int
			want      string
		}{
			{
				name:      "first step",
				org:       "c360",
				platform:  "ops",
				loopID:    "abc123",
				stepIndex: 0,
				want:      "c360.ops.agent.agentic-loop.step.abc123-0",
			},
			{
				name:      "tenth step",
				org:       "c360",
				platform:  "ops",
				loopID:    "abc123",
				stepIndex: 9,
				want:      "c360.ops.agent.agentic-loop.step.abc123-9",
			},
			{
				name:      "UUID loop ID",
				org:       "c360",
				platform:  "ops",
				loopID:    "550e8400-e29b-41d4-a716-446655440000",
				stepIndex: 3,
				want:      "c360.ops.agent.agentic-loop.step.550e8400-e29b-41d4-a716-446655440000-3",
			},
			{
				name:      "different org and platform",
				org:       "acme",
				platform:  "prod",
				loopID:    "x1",
				stepIndex: 0,
				want:      "acme.prod.agent.agentic-loop.step.x1-0",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got := agentic.TrajectoryStepEntityID(tt.org, tt.platform, tt.loopID, tt.stepIndex)
				require.Equal(t, tt.want, got)
				assert.True(t, message.IsValidEntityID(got), "result %q must pass IsValidEntityID", got)
			})
		}
	})

	t.Run("panics on invalid input", func(t *testing.T) {
		assert.Panics(t, func() {
			agentic.TrajectoryStepEntityID("", "ops", "abc123", 0)
		}, "empty org should panic")

		assert.Panics(t, func() {
			agentic.TrajectoryStepEntityID("c360", "", "abc123", 0)
		}, "empty platform should panic")

		assert.Panics(t, func() {
			agentic.TrajectoryStepEntityID("c360", "ops", "", 0)
		}, "empty loopID should panic")

		assert.Panics(t, func() {
			agentic.TrajectoryStepEntityID("c360.studio", "ops", "abc123", 0)
		}, "dot in org should panic")

		assert.Panics(t, func() {
			agentic.TrajectoryStepEntityID("c360", "ops", "loop.1", 0)
		}, "dot in loopID should panic")

		assert.Panics(t, func() {
			agentic.TrajectoryStepEntityID("c360", "ops.prod", "abc123", 0)
		}, "dot in platform should panic")

		assert.Panics(t, func() {
			agentic.TrajectoryStepEntityID("c360", "ops", "abc123", -1)
		}, "negative stepIndex should panic")
	})
}
