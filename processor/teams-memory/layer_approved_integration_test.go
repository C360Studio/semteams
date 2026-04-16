//go:build integration

package teamsmemory_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/natsclient"
	teamsmemory "github.com/c360studio/semteams/processor/teams-memory"

	operatingmodel "github.com/c360studio/semteams/operating-model"
)

// TestIntegration_LayerApproved_PublishesGraphMutation drives the end-to-end
// write path: publish a BaseMessage-wrapped LayerApproved on
// agent.operating_model.layer_approved.{user_id}, wait for teams-memory to
// emit the corresponding graph.mutation.{loop_id} message, assert the
// triples produced match the payload.
func TestIntegration_LayerApproved_PublishesGraphMutation(t *testing.T) {
	natsClient := getSharedNATSClient(t)

	config := teamsmemory.DefaultConfig()
	config.StreamName = "AGENT"
	config.ConsumerNameSuffix = "layer-approved-int-test-" + uniqueSuffix()
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

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	require.NoError(t, lc.Start(ctx))
	defer lc.Stop(5 * time.Second)
	waitForConsumerReady(t, comp)

	// Subscribe to graph.mutation.* to capture the emitted mutation.
	mutationCh := subscribeCoreNATS(t, natsClient, "graph.mutation.>")

	// Publish an approved layer.
	loopID := "loop-int-layerapproved"
	payload := &operatingmodel.LayerApproved{
		UserID:            "coby-int",
		LoopID:            loopID,
		Layer:             operatingmodel.LayerOperatingRhythms,
		ProfileVersion:    1,
		CheckpointSummary: "Weekly rhythms established",
		Entries: []operatingmodel.Entry{
			{
				EntryID: "e-int-1",
				Title:   "Weekly planning",
				Summary: "Mondays 9-10am",
				Cadence: "weekly",
				Trigger: "Monday 9am",
			},
		},
		ApprovedAt: time.Now().UTC(),
	}
	baseMsg := message.NewBaseMessage(payload.Schema(), payload, "integration-test")
	data, err := baseMsg.MarshalJSON()
	require.NoError(t, err)

	subject := "agent.operating_model.layer_approved." + payload.UserID
	require.NoError(t, natsClient.PublishToStream(ctx, subject, data))

	// Wait for the mutation to arrive on graph.mutation.{loop_id}.
	var mutation teamsmemory.GraphMutationMessage
	select {
	case raw := <-mutationCh:
		require.NoError(t, json.Unmarshal(raw, &mutation))
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for graph.mutation.%s", loopID)
	}

	assert.Equal(t, loopID, mutation.LoopID, "mutation loop_id should match payload")
	assert.Equal(t, "add_triples", mutation.Operation)
	assert.NotEmpty(t, mutation.Triples, "triples should be populated")

	// Sanity: expected predicates present (predicate-per-field).
	assertHasPredicate(t, mutation.Triples, operatingmodel.PredicateProfileVersion)
	assertHasPredicate(t, mutation.Triples, operatingmodel.PredicateLayerCheckpointSummary)
	assertHasPredicate(t, mutation.Triples, operatingmodel.PredicateEntryTitle)
	assertHasPredicate(t, mutation.Triples, operatingmodel.PredicateEntryCadence)
	assertHasPredicate(t, mutation.Triples, operatingmodel.PredicateEntryTrigger)
}

// TestIntegration_LayerApproved_WithoutPlatform_SkipsSilently verifies that a
// deployment with missing platform identity logs an error and declines to
// emit a malformed graph mutation, rather than producing triples with empty
// entity-ID parts.
func TestIntegration_LayerApproved_WithoutPlatform_SkipsSilently(t *testing.T) {
	natsClient := getSharedNATSClient(t)

	config := teamsmemory.DefaultConfig()
	config.StreamName = "AGENT"
	config.ConsumerNameSuffix = "layer-approved-noplatform-" + uniqueSuffix()
	config.Hydration.PostCompaction.Enabled = false
	config.Hydration.PreTask.Enabled = false
	config.Extraction.LLMAssisted.Enabled = false

	rawConfig, err := json.Marshal(config)
	require.NoError(t, err)

	// Deliberate empty Platform — triggers the warn+return branch.
	deps := component.Dependencies{NATSClient: natsClient}
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

	mutationCh := subscribeCoreNATS(t, natsClient, "graph.mutation.>")

	loopID := "loop-int-noplatform"
	payload := &operatingmodel.LayerApproved{
		UserID: "coby-int", LoopID: loopID,
		Layer: operatingmodel.LayerFriction, ProfileVersion: 1,
		CheckpointSummary: "x",
		Entries:           []operatingmodel.Entry{{EntryID: "e", Title: "t", Summary: "s"}},
		ApprovedAt:        time.Now().UTC(),
	}
	baseMsg := message.NewBaseMessage(payload.Schema(), payload, "integration-test")
	data, err := baseMsg.MarshalJSON()
	require.NoError(t, err)

	require.NoError(t, natsClient.PublishToStream(ctx,
		"agent.operating_model.layer_approved."+payload.UserID, data))

	// No mutation should arrive within a small grace window.
	select {
	case raw := <-mutationCh:
		t.Fatalf("expected no graph.mutation without platform; got %s", string(raw))
	case <-time.After(1 * time.Second):
		// Expected path.
	}

	// The component should have incremented its error counter: the missing-
	// platform branch in handleLayerApproved (layer_approved_handler.go) both
	// warns AND bumps errors. This assertion is coupled to that classification;
	// if a future change downgrades missing-platform to debug-only, revisit.
	require.Eventually(t, func() bool {
		return comp.Health().ErrorCount > 0
	}, 2*time.Second, 50*time.Millisecond, "errors should tick on missing platform")
}

// --- helpers ---

// subscribeCoreNATS starts a core-NATS subscription on the given subject and
// returns a channel delivering raw payload bytes. The subscription is
// cleaned up when the test ends.
func subscribeCoreNATS(t *testing.T, nc *natsclient.Client, subject string) <-chan []byte {
	t.Helper()
	raw := nc.GetConnection()
	require.NotNil(t, raw, "natsclient.Client has no live *nats.Conn")
	ch := make(chan []byte, 8)
	sub, err := raw.Subscribe(subject, func(m *nats.Msg) {
		out := make([]byte, len(m.Data))
		copy(out, m.Data)
		select {
		case ch <- out:
		default:
			t.Logf("subscribeCoreNATS: channel full for %q; dropping", subject)
		}
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = sub.Unsubscribe() })
	return ch
}

// assertHasPredicate verifies at least one triple in the slice uses the given
// predicate. Deliberately loose — we cover triple-cardinality in the unit
// tests; this just confirms the wire round-trip preserved the shape.
func assertHasPredicate(t *testing.T, triples []message.Triple, predicate string) {
	t.Helper()
	for _, tr := range triples {
		if tr.Predicate == predicate {
			return
		}
	}
	preds := make([]string, 0, len(triples))
	for _, tr := range triples {
		preds = append(preds, tr.Predicate)
	}
	t.Errorf("missing triple predicate %q; saw: %s", predicate, strings.Join(preds, ", "))
}
