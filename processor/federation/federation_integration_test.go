//go:build integration

package federation_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/federation"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/natsclient"
	proc "github.com/c360studio/semstreams/processor/federation"
)

var (
	sharedTestClient *natsclient.TestClient
	sharedNATSClient *natsclient.Client
)

// TestMain sets up shared NATS container for all federation integration tests.
func TestMain(m *testing.M) {
	// Register federation payload so BaseMessage can deserialize EventPayload.
	if err := federation.RegisterPayload("federation"); err != nil {
		panic("Failed to register federation payload: " + err.Error())
	}

	streams := []natsclient.TestStreamConfig{
		{Name: "FEDERATION_EVENTS", Subjects: []string{"federation.graph.events"}},
		{Name: "FEDERATION_MERGED", Subjects: []string{"federation.graph.merged"}},
	}

	testClient, err := natsclient.NewSharedTestClient(
		natsclient.WithJetStream(),
		natsclient.WithStreams(streams...),
		natsclient.WithTestTimeout(5*time.Second),
		natsclient.WithStartTimeout(30*time.Second),
	)
	if err != nil {
		panic("Failed to create shared test client: " + err.Error())
	}

	sharedTestClient = testClient
	sharedNATSClient = testClient.Client

	exitCode := m.Run()

	sharedTestClient.Terminate()

	if exitCode != 0 {
		panic("tests failed")
	}
}

func getSharedNATSClient(t *testing.T) *natsclient.Client {
	t.Helper()
	if sharedNATSClient == nil {
		t.Fatal("Shared NATS client not initialized")
	}
	return sharedNATSClient
}

// publishFederationEvent wraps an Event in EventPayload → BaseMessage and publishes
// to the federation input subject.
func publishFederationEvent(t *testing.T, nc *natsclient.Client, event *federation.Event) {
	t.Helper()
	payload := &federation.EventPayload{Event: *event}
	baseMsg := message.NewBaseMessage(payload.Schema(), payload, "integration-test")
	data, err := json.Marshal(baseMsg)
	require.NoError(t, err, "Failed to marshal federation event")
	err = nc.PublishToStream(context.Background(), "federation.graph.events", data)
	require.NoError(t, err, "Failed to publish federation event")
}

// createAndStartComponent creates a federation processor with the given namespace,
// starts it, and returns a cleanup function.
func createAndStartComponent(t *testing.T, nc *natsclient.Client, namespace string) component.LifecycleComponent {
	t.Helper()
	config := proc.Config{
		LocalNamespace: namespace,
		MergePolicy:    proc.MergePolicyStandard,
		InputSubject:   "federation.graph.events",
		OutputSubject:  "federation.graph.merged",
		InputStream:    "FEDERATION_EVENTS",
		OutputStream:   "FEDERATION_MERGED",
	}

	rawConfig, err := json.Marshal(config)
	require.NoError(t, err)

	deps := component.Dependencies{
		NATSClient: nc,
	}

	comp, err := proc.NewComponent(rawConfig, deps)
	require.NoError(t, err)

	lc, ok := comp.(component.LifecycleComponent)
	require.True(t, ok, "Component must implement LifecycleComponent")

	err = lc.Initialize()
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	err = lc.Start(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { _ = lc.Stop(5 * time.Second) })

	// Allow consumer to attach.
	time.Sleep(200 * time.Millisecond)

	return lc
}

// subscribeForMergedEvents subscribes to the output subject and collects received events.
func subscribeForMergedEvents(t *testing.T, nc *natsclient.Client) (events func() []*federation.EventPayload) {
	t.Helper()
	var received []*federation.EventPayload
	var mu sync.Mutex

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	sub, err := nc.Subscribe(ctx, "federation.graph.merged", func(_ context.Context, msg *nats.Msg) {
		var base message.BaseMessage
		if err := json.Unmarshal(msg.Data, &base); err == nil {
			if ep, ok := base.Payload().(*federation.EventPayload); ok {
				mu.Lock()
				received = append(received, ep)
				mu.Unlock()
			}
		}
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = sub.Unsubscribe() })

	// Allow subscription to establish.
	time.Sleep(100 * time.Millisecond)

	return func() []*federation.EventPayload {
		mu.Lock()
		defer mu.Unlock()
		out := make([]*federation.EventPayload, len(received))
		copy(out, received)
		return out
	}
}

// TestIntegration_FederationFullFlow verifies end-to-end: publish a DELTA event
// with a local-namespace entity → merged event arrives on output with entity intact.
func TestIntegration_FederationFullFlow(t *testing.T) {
	nc := getSharedNATSClient(t)
	getReceived := subscribeForMergedEvents(t, nc)
	createAndStartComponent(t, nc, "acme")

	now := time.Now().UTC().Truncate(time.Millisecond)
	event := &federation.Event{
		Type:      federation.EventTypeDELTA,
		SourceID:  "integration-test",
		Namespace: "acme",
		Timestamp: now,
		Entities: []federation.Entity{
			{
				ID: "acme.platform.git.repo.commit.abc123",
				Triples: []message.Triple{
					{Subject: "acme.platform.git.repo.commit.abc123", Predicate: "git.sha", Object: "abc123"},
				},
				Edges: []federation.Edge{
					{FromID: "acme.platform.git.repo.commit.abc123", ToID: "acme.platform.git.repo.author.alice", EdgeType: "authored_by"},
				},
				Provenance: federation.Provenance{SourceType: "git", SourceID: "test", Timestamp: now, Handler: "TestHandler"},
			},
		},
		Provenance: federation.Provenance{SourceType: "git", SourceID: "test", Timestamp: now, Handler: "TestHandler"},
	}

	publishFederationEvent(t, nc, event)

	require.Eventually(t, func() bool {
		return len(getReceived()) > 0
	}, 5*time.Second, 100*time.Millisecond, "Should receive merged event on output")

	received := getReceived()
	require.Len(t, received, 1)

	merged := received[0]
	assert.Equal(t, federation.EventTypeDELTA, merged.Event.Type)
	require.Len(t, merged.Event.Entities, 1, "Local namespace entity should pass through")
	assert.Equal(t, "acme.platform.git.repo.commit.abc123", merged.Event.Entities[0].ID)
	assert.Len(t, merged.Event.Entities[0].Edges, 1, "Edge should be preserved")
}

// TestIntegration_FederationCrossOrgFiltered verifies that entities from a foreign
// namespace are silently filtered by the merge policy.
func TestIntegration_FederationCrossOrgFiltered(t *testing.T) {
	nc := getSharedNATSClient(t)
	getReceived := subscribeForMergedEvents(t, nc)
	createAndStartComponent(t, nc, "acme")

	now := time.Now().UTC().Truncate(time.Millisecond)
	event := &federation.Event{
		Type:      federation.EventTypeDELTA,
		SourceID:  "integration-test",
		Namespace: "foreign",
		Timestamp: now,
		Entities: []federation.Entity{
			{
				ID: "foreign.platform.git.repo.commit.xyz789",
				Triples: []message.Triple{
					{Subject: "foreign.platform.git.repo.commit.xyz789", Predicate: "git.sha", Object: "xyz789"},
				},
				Provenance: federation.Provenance{SourceType: "git", SourceID: "test", Timestamp: now, Handler: "TestHandler"},
			},
		},
		Provenance: federation.Provenance{SourceType: "git", SourceID: "test", Timestamp: now, Handler: "TestHandler"},
	}

	publishFederationEvent(t, nc, event)

	require.Eventually(t, func() bool {
		return len(getReceived()) > 0
	}, 5*time.Second, 100*time.Millisecond, "Should receive merged event (even if entities filtered)")

	received := getReceived()
	require.Len(t, received, 1)

	merged := received[0]
	assert.Equal(t, federation.EventTypeDELTA, merged.Event.Type)
	assert.Empty(t, merged.Event.Entities, "Cross-org entities should be filtered")
}

// TestIntegration_FederationHeartbeatPassthrough verifies that HEARTBEAT events
// pass through the federation processor unchanged.
func TestIntegration_FederationHeartbeatPassthrough(t *testing.T) {
	nc := getSharedNATSClient(t)
	getReceived := subscribeForMergedEvents(t, nc)
	createAndStartComponent(t, nc, "acme")

	now := time.Now().UTC().Truncate(time.Millisecond)
	event := &federation.Event{
		Type:       federation.EventTypeHEARTBEAT,
		SourceID:   "integration-test",
		Namespace:  "acme",
		Timestamp:  now,
		Provenance: federation.Provenance{SourceType: "internal", SourceID: "test", Timestamp: now, Handler: "Engine"},
	}

	publishFederationEvent(t, nc, event)

	require.Eventually(t, func() bool {
		return len(getReceived()) > 0
	}, 5*time.Second, 100*time.Millisecond, "Should receive heartbeat on output")

	received := getReceived()
	require.Len(t, received, 1)

	merged := received[0]
	assert.Equal(t, federation.EventTypeHEARTBEAT, merged.Event.Type)
	assert.Equal(t, "integration-test", merged.Event.SourceID)
}
