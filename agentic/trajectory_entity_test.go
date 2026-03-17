package agentic_test

import (
	"testing"
	"time"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/message"
	agvocab "github.com/c360studio/semstreams/vocabulary/agentic"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTrajectoryStepEntity_ToolCall(t *testing.T) {
	step := agentic.TrajectoryStep{
		Timestamp:     time.Date(2026, 3, 17, 14, 0, 0, 0, time.UTC),
		StepType:      "tool_call",
		ToolName:      "web_search",
		ToolArguments: map[string]any{"query": "deployment errors"},
		ToolResult:    "Found 3 results about deployment errors...",
		Duration:      1500,
	}

	entity := &agentic.TrajectoryStepEntity{
		Step:      step,
		Org:       "acme",
		Platform:  "ops",
		LoopID:    "loop123",
		StepIndex: 2,
	}

	t.Run("EntityID", func(t *testing.T) {
		got := entity.EntityID()
		assert.Equal(t, "acme.ops.agent.agentic-loop.step.loop123-2", got)
		assert.True(t, message.IsValidEntityID(got))
	})

	t.Run("Triples_metadata_only", func(t *testing.T) {
		triples := entity.Triples()

		preds := predicateSet(triples)

		required := []string{
			agvocab.StepType,
			agvocab.StepIndex,
			agvocab.StepLoop,
			agvocab.StepTimestamp,
			agvocab.StepDuration,
			agvocab.StepToolName,
		}
		for _, pred := range required {
			assert.True(t, preds[pred], "missing required predicate: %s", pred)
		}

		// Model-specific predicates should NOT be present
		assert.False(t, preds[agvocab.StepModel])
		assert.False(t, preds[agvocab.StepTokensIn])
		assert.False(t, preds[agvocab.StepTokensOut])

		// Verify values
		assert.Equal(t, "tool_call", objectFor(triples, agvocab.StepType))
		assert.Equal(t, 2, objectFor(triples, agvocab.StepIndex))
		assert.Equal(t, "web_search", objectFor(triples, agvocab.StepToolName))
		assert.Equal(t, int64(1500), objectFor(triples, agvocab.StepDuration))

		// Loop reference must be a valid entity ID
		loopRef, ok := objectFor(triples, agvocab.StepLoop).(string)
		require.True(t, ok)
		assert.True(t, message.IsValidEntityID(loopRef))
		assert.Equal(t, "acme.ops.agent.agentic-loop.execution.loop123", loopRef)

		// All triples should reference the step entity
		entityID := entity.EntityID()
		for _, tr := range triples {
			assert.Equal(t, entityID, tr.Subject)
			assert.Equal(t, float64(1.0), tr.Confidence)
		}
	})

	t.Run("ContentFields_tool_call", func(t *testing.T) {
		fields := entity.ContentFields()
		assert.Equal(t, "tool_result", fields[message.ContentRoleBody])
		assert.Equal(t, "tool_name", fields[message.ContentRoleTitle])
	})

	t.Run("RawContent_tool_call", func(t *testing.T) {
		content := entity.RawContent()
		assert.Equal(t, "web_search", content["tool_name"])
		assert.Equal(t, "Found 3 results about deployment errors...", content["tool_result"])
		assert.Contains(t, content["tool_arguments"], "deployment errors")
	})

	t.Run("StorageRef_initially_nil", func(t *testing.T) {
		assert.Nil(t, entity.StorageRef())
	})

	t.Run("SetStorageRef", func(t *testing.T) {
		ref := &message.StorageReference{
			StorageInstance: "objectstore-1",
			Key:             "2026/03/17/step/loop123-2",
			ContentType:     "application/json",
		}
		entity.SetStorageRef(ref)
		assert.Equal(t, ref, entity.StorageRef())
	})
}

func TestTrajectoryStepEntity_ModelCall(t *testing.T) {
	step := agentic.TrajectoryStep{
		Timestamp: time.Date(2026, 3, 17, 14, 0, 0, 0, time.UTC),
		StepType:  "model_call",
		Model:     "claude-sonnet",
		Response:  "Here is my analysis of the code...",
		TokensIn:  4832,
		TokensOut: 819,
		Duration:  3200,
	}

	entity := &agentic.TrajectoryStepEntity{
		Step:      step,
		Org:       "acme",
		Platform:  "ops",
		LoopID:    "loop456",
		StepIndex: 0,
	}

	t.Run("EntityID", func(t *testing.T) {
		got := entity.EntityID()
		assert.Equal(t, "acme.ops.agent.agentic-loop.step.loop456-0", got)
		assert.True(t, message.IsValidEntityID(got))
	})

	t.Run("Triples_metadata_only", func(t *testing.T) {
		triples := entity.Triples()

		preds := predicateSet(triples)

		required := []string{
			agvocab.StepType,
			agvocab.StepIndex,
			agvocab.StepLoop,
			agvocab.StepTimestamp,
			agvocab.StepDuration,
			agvocab.StepModel,
			agvocab.StepTokensIn,
			agvocab.StepTokensOut,
		}
		for _, pred := range required {
			assert.True(t, preds[pred], "missing required predicate: %s", pred)
		}

		// Tool-specific predicates should NOT be present
		assert.False(t, preds[agvocab.StepToolName])

		assert.Equal(t, "model_call", objectFor(triples, agvocab.StepType))
		assert.Equal(t, "claude-sonnet", objectFor(triples, agvocab.StepModel))
		assert.Equal(t, 4832, objectFor(triples, agvocab.StepTokensIn))
		assert.Equal(t, 819, objectFor(triples, agvocab.StepTokensOut))
	})

	t.Run("ContentFields_model_call", func(t *testing.T) {
		fields := entity.ContentFields()
		assert.Equal(t, "response", fields[message.ContentRoleBody])
		assert.Equal(t, "model", fields[message.ContentRoleTitle])
	})

	t.Run("RawContent_model_call", func(t *testing.T) {
		content := entity.RawContent()
		assert.Equal(t, "claude-sonnet", content["model"])
		assert.Equal(t, "Here is my analysis of the code...", content["response"])
	})
}

func TestTrajectoryStepEntity_EmptyToolResult(t *testing.T) {
	step := agentic.TrajectoryStep{
		Timestamp: time.Now(),
		StepType:  "tool_call",
		ToolName:  "noop_tool",
		Duration:  50,
	}

	entity := &agentic.TrajectoryStepEntity{
		Step:      step,
		Org:       "acme",
		Platform:  "ops",
		LoopID:    "loop789",
		StepIndex: 0,
	}

	t.Run("ContentFields_omits_empty_body", func(t *testing.T) {
		fields := entity.ContentFields()
		_, hasBody := fields[message.ContentRoleBody]
		assert.False(t, hasBody, "empty tool result should not map body role")
		assert.Equal(t, "tool_name", fields[message.ContentRoleTitle])
	})

	t.Run("RawContent_omits_empty_fields", func(t *testing.T) {
		content := entity.RawContent()
		_, hasResult := content["tool_result"]
		assert.False(t, hasResult)
		_, hasArgs := content["tool_arguments"]
		assert.False(t, hasArgs)
	})
}

func TestTrajectoryStepEntity_ContextCompaction(t *testing.T) {
	step := agentic.TrajectoryStep{
		Timestamp: time.Now(),
		StepType:  "context_compaction",
		Duration:  200,
	}

	entity := &agentic.TrajectoryStepEntity{
		Step:      step,
		Org:       "acme",
		Platform:  "ops",
		LoopID:    "loopGC",
		StepIndex: 3,
	}

	t.Run("ContentFields_nil_for_compaction", func(t *testing.T) {
		assert.Nil(t, entity.ContentFields())
	})

	t.Run("RawContent_nil_for_compaction", func(t *testing.T) {
		assert.Nil(t, entity.RawContent())
	})
}

// predicateSet collects predicates from triples for membership testing.
func predicateSet(triples []message.Triple) map[string]bool {
	s := make(map[string]bool, len(triples))
	for _, t := range triples {
		s[t.Predicate] = true
	}
	return s
}

// objectFor returns the Object for the first triple with the given predicate.
func objectFor(triples []message.Triple, predicate string) any {
	for _, t := range triples {
		if t.Predicate == predicate {
			return t.Object
		}
	}
	return nil
}
