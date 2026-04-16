//go:build integration

package teamsmemory_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/message"
	teamsmemory "github.com/c360studio/semteams/processor/teams-memory"

	operatingmodel "github.com/c360studio/semteams/operating-model"
)

// integrationProfileReader returns a fixed entry set so the publisher has
// something to assemble.
type integrationProfileReader struct {
	entries []operatingmodel.Entry
	version int
}

func (r integrationProfileReader) ReadOperatingModel(ctx context.Context, org, platform, userID string) (*operatingmodel.ProfileResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return &operatingmodel.ProfileResult{Entries: r.entries, Version: r.version}, nil
}

// TestIntegration_LoopCreated_PublishesProfileContext drives the end-to-end
// read path: publish a LoopCreatedEvent carrying user_id in metadata, wait
// for teams-memory to assemble + publish an operating_model.profile_context.v1
// payload on agent.context.profile.{loop_id}, assert the payload shape.
func TestIntegration_LoopCreated_PublishesProfileContext(t *testing.T) {
	natsClient := getSharedNATSClient(t)

	config := teamsmemory.DefaultConfig()
	config.StreamName = "AGENT"
	config.ConsumerNameSuffix = "profile-ctx-int-" + uniqueSuffix()
	config.Hydration.PostCompaction.Enabled = false
	config.Hydration.PreTask.Enabled = false
	config.Extraction.LLMAssisted.Enabled = false

	rawConfig, err := json.Marshal(config)
	require.NoError(t, err)

	deps := component.Dependencies{
		NATSClient: natsClient,
		Platform:   component.PlatformMeta{Org: "c360", Platform: "ops"},
	}
	comp, err := teamsmemory.NewComponent(rawConfig, deps)
	require.NoError(t, err)

	// Swap in a reader with real entries before Start so we exercise the
	// assembler end-to-end.
	tmComp, ok := comp.(*teamsmemory.Component)
	require.True(t, ok)
	tmComp.SetProfileReader(integrationProfileReader{
		entries: []operatingmodel.Entry{
			{
				EntryID: "e-int-1",
				Title:   "Weekly planning",
				Summary: "Mondays 9-10am",
				Cadence: "weekly",
				Status:  operatingmodel.StatusActive,
			},
			{
				EntryID: "e-int-2",
				Title:   "Finance emails",
				Summary: "Friday morning review",
				Status:  operatingmodel.StatusActive,
			},
		},
		version: 1,
	})

	lc, ok := comp.(component.LifecycleComponent)
	require.True(t, ok)
	require.NoError(t, lc.Initialize())

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	require.NoError(t, lc.Start(ctx))
	defer lc.Stop(5 * time.Second)
	waitForConsumerReady(t, comp)

	// Subscribe to the profile-context output channel.
	profileCh := subscribeCoreNATS(t, natsClient, "agent.context.profile.>")

	// Publish a LoopCreatedEvent with user_id in metadata.
	loopID := "loop-int-profilectx"
	event := &agentic.LoopCreatedEvent{
		LoopID:        loopID,
		TaskID:        "task-int",
		Role:          "general",
		Model:         "mock",
		MaxIterations: 20,
		CreatedAt:     time.Now().UTC(),
		Metadata: map[string]any{
			"user_id": "coby-int",
		},
	}
	baseMsg := message.NewBaseMessage(event.Schema(), event, "integration-test")
	data, err := baseMsg.MarshalJSON()
	require.NoError(t, err)
	require.NoError(t, natsClient.PublishToStream(ctx, "agent.created."+loopID, data))

	// Wait for the profile-context publish.
	var raw []byte
	select {
	case raw = <-profileCh:
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for agent.context.profile.%s", loopID)
	}

	// Decode the BaseMessage envelope and assert the typed payload.
	var baseOut message.BaseMessage
	require.NoError(t, baseOut.UnmarshalJSON(raw))
	got, ok := baseOut.Payload().(*operatingmodel.ProfileContext)
	require.True(t, ok, "payload should be *ProfileContext")

	assert.Equal(t, loopID, got.LoopID)
	assert.Equal(t, "coby-int", got.UserID)
	assert.Equal(t, 1, got.ProfileVersion)
	assert.NotZero(t, got.OperatingModel.EntryCount, "operating model slice should have entries")
	assert.True(t, got.HasOperatingModel(), "operating model slice should be populated")
	assert.Empty(t, got.LessonsLearned.Content, "lessons_learned is reserved/stub in v1")
}

// TestIntegration_LoopCreated_WithoutUserID_SkipsPublish verifies the
// "no-op skip" branch when LoopCreatedEvent.Metadata lacks user_id.
func TestIntegration_LoopCreated_WithoutUserID_SkipsPublish(t *testing.T) {
	natsClient := getSharedNATSClient(t)

	config := teamsmemory.DefaultConfig()
	config.StreamName = "AGENT"
	config.ConsumerNameSuffix = "profile-ctx-nouser-" + uniqueSuffix()
	config.Hydration.PostCompaction.Enabled = false
	config.Hydration.PreTask.Enabled = false
	config.Extraction.LLMAssisted.Enabled = false

	rawConfig, err := json.Marshal(config)
	require.NoError(t, err)

	deps := component.Dependencies{
		NATSClient: natsClient,
		Platform:   component.PlatformMeta{Org: "c360", Platform: "ops"},
	}
	comp, err := teamsmemory.NewComponent(rawConfig, deps)
	require.NoError(t, err)

	lc, ok := comp.(component.LifecycleComponent)
	require.True(t, ok)
	require.NoError(t, lc.Initialize())

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	require.NoError(t, lc.Start(ctx))
	defer lc.Stop(5 * time.Second)
	waitForConsumerReady(t, comp)

	profileCh := subscribeCoreNATS(t, natsClient, "agent.context.profile.>")

	loopID := "loop-int-nouser"
	event := &agentic.LoopCreatedEvent{
		LoopID: loopID, TaskID: "task", Role: "general",
		Model: "mock", MaxIterations: 20, CreatedAt: time.Now().UTC(),
		Metadata: nil, // explicitly no user_id
	}
	baseMsg := message.NewBaseMessage(event.Schema(), event, "integration-test")
	data, err := baseMsg.MarshalJSON()
	require.NoError(t, err)
	require.NoError(t, natsClient.PublishToStream(ctx, "agent.created."+loopID, data))

	// Expect no profile-context publish within a reasonable grace window.
	select {
	case raw := <-profileCh:
		t.Fatalf("expected no publish; got %s", string(raw))
	case <-time.After(1 * time.Second):
		// Good — skipped as designed.
	}
}
