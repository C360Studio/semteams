//go:build integration

package natstest

import (
	"context"
	"strings"
	"testing"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
)

// spec: openspec/specs/framework-adoption/spec.md#Requirement: Frozen framework adoption preserves qualified product behavior
func TestBrokerLifecycleAndJetStreamHelpers(t *testing.T) {
	var containerID string

	t.Run("subtest_starts_uses_and_cleans_up_broker", func(t *testing.T) {
		broker := New(t)
		require.NotNil(t, broker.Client)
		require.True(t, strings.HasPrefix(broker.URL, "nats://"), "unexpected broker URL %q", broker.URL)
		containerID = broker.containerID
		require.NotEmpty(t, containerID)

		ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
		defer cancel()

		inspect, err := broker.container.Inspect(ctx)
		require.NoError(t, err)
		require.Equal(t, expectedPinnedNATSImage, inspect.Config.Image)
		require.Contains(t, inspect.Config.Labels[testLabel], "pid ")

		received := make(chan []byte, 1)
		sub, err := broker.Client.Subscribe(ctx, "natstest.pubsub.check", func(_ context.Context, msg *nats.Msg) {
			received <- append([]byte(nil), msg.Data...)
		})
		require.NoError(t, err)
		t.Cleanup(func() { _ = sub.Unsubscribe() })
		require.NoError(t, broker.Client.GetConnection().FlushWithContext(ctx))

		require.NoError(t, broker.Client.Publish(ctx, "natstest.pubsub.check", []byte("hello-natstest")))
		select {
		case got := <-received:
			require.Equal(t, []byte("hello-natstest"), got)
		case <-ctx.Done():
			t.Fatal("timed out waiting for core NATS message")
		}

		bucket, err := broker.CreateKVBucket(ctx, "NATSTEST_KV")
		require.NoError(t, err)
		_, err = bucket.PutString(ctx, "greeting", "pinned-digest")
		require.NoError(t, err)
		entry, err := bucket.Get(ctx, "greeting")
		require.NoError(t, err)
		require.Equal(t, "pinned-digest", string(entry.Value()))

		stream, err := broker.CreateStream(ctx, "NATSTEST_STREAM", "natstest.stream.>")
		require.NoError(t, err)
		require.NoError(t, broker.Client.PublishToStream(ctx, "natstest.stream.item", []byte("stream-payload")))

		consumer, err := stream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
			Durable:       "natstest-consumer",
			FilterSubject: "natstest.stream.>",
			AckPolicy:     jetstream.AckExplicitPolicy,
		})
		require.NoError(t, err)
		msg, err := consumer.Next(jetstream.FetchContext(ctx))
		require.NoError(t, err)
		require.Equal(t, []byte("stream-payload"), msg.Data())
		require.NoError(t, msg.DoubleAck(ctx))
	})

	// After the subtest returns, its t.Cleanup has closed the client and removed
	// the container. Verify Docker reports the container gone.
	checkCtx, checkCancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer checkCancel()
	dockerCli, err := testcontainers.NewDockerClientWithOpts(checkCtx)
	require.NoError(t, err)
	defer func() { _ = dockerCli.Close() }()

	_, inspectErr := dockerCli.ContainerInspect(checkCtx, containerID)
	require.Error(t, inspectErr)
	require.True(t, cerrdefs.IsNotFound(inspectErr), "expected container %s to be removed on cleanup, got %v", containerID, inspectErr)
}
