// Package natsclient provides JetStream stream management utilities.
package natsclient

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/c360/semstreams/pkg/errs"
)

// StreamConsumerConfig configures a JetStream consumer.
type StreamConsumerConfig struct {
	// StreamName is the name of the stream to consume from (required).
	StreamName string

	// ConsumerName is the durable consumer name. If empty, creates an ephemeral consumer.
	ConsumerName string

	// FilterSubject filters messages within the stream. If empty, receives all messages.
	FilterSubject string

	// DeliverPolicy determines where to start delivering messages.
	// Options: "all" (default), "last", "new", "by_start_time"
	DeliverPolicy string

	// AckPolicy determines how messages are acknowledged.
	// Options: "explicit" (default), "none", "all"
	AckPolicy string

	// MaxDeliver is the maximum number of delivery attempts (0 = unlimited).
	MaxDeliver int

	// AckWait is how long to wait for an ack before redelivery.
	// Default is 30 seconds.
	AckWait time.Duration

	// MaxAckPending limits the number of outstanding (unacknowledged) messages
	// that can be delivered to a consumer. This provides backpressure to prevent
	// overwhelming the consumer. 0 means unlimited (default NATS behavior).
	MaxAckPending int

	// AutoCreate enables automatic stream creation if it doesn't exist.
	AutoCreate bool

	// AutoCreateConfig is used when auto-creating a stream.
	// If nil, defaults are used based on FilterSubject.
	AutoCreateConfig *StreamAutoCreateConfig
}

// StreamAutoCreateConfig configures automatic stream creation.
type StreamAutoCreateConfig struct {
	// Subjects for the stream. If empty, derived from FilterSubject.
	Subjects []string

	// Storage type: "file" (default) or "memory"
	Storage string

	// Retention policy: "limits" (default), "interest", "work_queue"
	Retention string

	// MaxAge is the maximum age of messages (default 7 days).
	MaxAge time.Duration

	// MaxBytes is the maximum total size (0 = unlimited).
	MaxBytes int64

	// MaxMsgs is the maximum number of messages (0 = unlimited).
	MaxMsgs int64

	// Replicas is the number of replicas (default 1).
	Replicas int
}

// DefaultStreamConfig returns default auto-create configuration.
func DefaultStreamConfig() *StreamAutoCreateConfig {
	return &StreamAutoCreateConfig{
		Storage:   "file",
		Retention: "limits",
		MaxAge:    7 * 24 * time.Hour, // 7 days
		Replicas:  1,
	}
}

// EnsureStream creates a stream if it doesn't exist, or returns the existing one.
func (c *Client) EnsureStream(ctx context.Context, cfg jetstream.StreamConfig) (jetstream.Stream, error) {
	if c.Status() == StatusCircuitOpen {
		return nil, ErrCircuitOpen
	}

	if c.Status() != StatusConnected {
		return nil, ErrNotConnected
	}

	js, err := c.JetStream()
	if err != nil {
		return nil, err
	}

	// Try to get existing stream first
	stream, err := js.Stream(ctx, cfg.Name)
	if err == nil {
		return stream, nil
	}

	// If not found, create it
	if errors.Is(err, jetstream.ErrStreamNotFound) {
		stream, err = js.CreateStream(ctx, cfg)
		if err != nil {
			c.recordFailure()
			c.jsMetrics.recordError("create_stream")
			return nil, errs.WrapTransient(err, "Client", "EnsureStream", "failed to create stream "+cfg.Name)
		}
		c.resetCircuit()
		c.jsMetrics.trackStream(cfg.Name, stream)
		return stream, nil
	}

	c.recordFailure()
	return nil, errs.WrapTransient(err, "Client", "EnsureStream", "failed to get stream "+cfg.Name)
}

// ConsumeStreamWithConfig creates a JetStream consumer with full configuration.
// The handler receives the raw jetstream.Msg which includes Ack(), Nak(), and Term() methods.
// Handler MUST call one of these methods to acknowledge the message.
func (c *Client) ConsumeStreamWithConfig(
	ctx context.Context,
	cfg StreamConsumerConfig,
	handler func(ctx context.Context, msg jetstream.Msg),
) error {
	if cfg.StreamName == "" {
		return errs.WrapInvalid(
			fmt.Errorf("stream name is required"),
			"Client", "ConsumeStreamWithConfig", "missing stream name")
	}

	if c.Status() == StatusCircuitOpen {
		return ErrCircuitOpen
	}

	if c.Status() != StatusConnected {
		return ErrNotConnected
	}

	js, err := c.JetStream()
	if err != nil {
		return err
	}

	// Auto-create stream if enabled
	if cfg.AutoCreate {
		if err := c.ensureStreamForConsumer(ctx, js, cfg); err != nil {
			return err
		}
	}

	// Get the stream
	stream, err := js.Stream(ctx, cfg.StreamName)
	if err != nil {
		c.recordFailure()
		return errs.WrapTransient(err, "Client", "ConsumeStreamWithConfig",
			"failed to get stream "+cfg.StreamName)
	}

	// Build consumer configuration
	consumerCfg := c.buildConsumerConfig(cfg)

	// Create or update consumer
	consumer, err := stream.CreateOrUpdateConsumer(ctx, consumerCfg)
	if err != nil {
		c.recordFailure()
		return errs.WrapTransient(err, "Client", "ConsumeStreamWithConfig",
			"failed to create consumer for stream "+cfg.StreamName)
	}

	// Start consuming
	consumeCtx, err := consumer.Consume(func(msg jetstream.Msg) {
		// Create per-message context
		msgCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()

		// Wrap handler with panic recovery and default Nak
		c.safeHandleMessage(msgCtx, msg, handler)
	})
	if err != nil {
		c.recordFailure()
		return errs.WrapTransient(err, "Client", "ConsumeStreamWithConfig",
			"failed to start consuming from stream "+cfg.StreamName)
	}

	// Track consumer for cleanup
	c.consumersMu.Lock()
	if c.consumers == nil {
		c.consumers = make(map[string]jetstream.ConsumeContext)
	}
	consumerKey := cfg.StreamName + ":" + cfg.ConsumerName
	c.consumers[consumerKey] = consumeCtx
	c.consumersMu.Unlock()

	c.resetCircuit()
	return nil
}

// safeHandleMessage wraps the handler with panic recovery.
// If handler doesn't ack/nak/term the message, Nak is called by default.
func (c *Client) safeHandleMessage(ctx context.Context, msg jetstream.Msg, handler func(context.Context, jetstream.Msg)) {
	defer func() {
		if r := recover(); r != nil {
			c.logger.Printf("panic in message handler: %v", r)
			// Nak on panic to allow redelivery
			_ = msg.Nak()
		}
	}()

	handler(ctx, msg)
}

// buildConsumerConfig converts StreamConsumerConfig to jetstream.ConsumerConfig.
func (c *Client) buildConsumerConfig(cfg StreamConsumerConfig) jetstream.ConsumerConfig {
	consumerCfg := jetstream.ConsumerConfig{}

	// Set durable name if provided
	if cfg.ConsumerName != "" {
		consumerCfg.Durable = cfg.ConsumerName
	}

	// Set filter subject
	if cfg.FilterSubject != "" {
		consumerCfg.FilterSubject = cfg.FilterSubject
	}

	// Set deliver policy
	switch cfg.DeliverPolicy {
	case "last":
		consumerCfg.DeliverPolicy = jetstream.DeliverLastPolicy
	case "new":
		consumerCfg.DeliverPolicy = jetstream.DeliverNewPolicy
	case "by_start_time":
		consumerCfg.DeliverPolicy = jetstream.DeliverByStartTimePolicy
	default: // "all" or empty
		consumerCfg.DeliverPolicy = jetstream.DeliverAllPolicy
	}

	// Set ack policy
	switch cfg.AckPolicy {
	case "none":
		consumerCfg.AckPolicy = jetstream.AckNonePolicy
	case "all":
		consumerCfg.AckPolicy = jetstream.AckAllPolicy
	default: // "explicit" or empty
		consumerCfg.AckPolicy = jetstream.AckExplicitPolicy
	}

	// Set max deliver
	if cfg.MaxDeliver > 0 {
		consumerCfg.MaxDeliver = cfg.MaxDeliver
	}

	// Set ack wait
	if cfg.AckWait > 0 {
		consumerCfg.AckWait = cfg.AckWait
	} else {
		consumerCfg.AckWait = 30 * time.Second // Default
	}

	// Set max ack pending for backpressure
	if cfg.MaxAckPending > 0 {
		consumerCfg.MaxAckPending = cfg.MaxAckPending
	}

	return consumerCfg
}

// ensureStreamForConsumer auto-creates a stream if it doesn't exist.
func (c *Client) ensureStreamForConsumer(ctx context.Context, js jetstream.JetStream, cfg StreamConsumerConfig) error {
	// Check if stream exists
	_, err := js.Stream(ctx, cfg.StreamName)
	if err == nil {
		return nil // Stream exists
	}

	if !errors.Is(err, jetstream.ErrStreamNotFound) {
		return errs.WrapTransient(err, "Client", "ensureStreamForConsumer",
			"failed to check stream "+cfg.StreamName)
	}

	// Stream doesn't exist, create it
	autoConfig := cfg.AutoCreateConfig
	if autoConfig == nil {
		autoConfig = DefaultStreamConfig()
	}

	// Determine subjects
	subjects := autoConfig.Subjects
	if len(subjects) == 0 && cfg.FilterSubject != "" {
		// Derive subjects from filter subject
		subjects = []string{deriveStreamSubject(cfg.FilterSubject)}
	}
	if len(subjects) == 0 {
		return errs.WrapInvalid(
			fmt.Errorf("cannot auto-create stream without subjects"),
			"Client", "ensureStreamForConsumer", "no subjects for stream "+cfg.StreamName)
	}

	// Build stream config
	streamCfg := jetstream.StreamConfig{
		Name:     cfg.StreamName,
		Subjects: subjects,
		MaxAge:   autoConfig.MaxAge,
	}

	// Set storage
	switch autoConfig.Storage {
	case "memory":
		streamCfg.Storage = jetstream.MemoryStorage
	default:
		streamCfg.Storage = jetstream.FileStorage
	}

	// Set retention
	switch autoConfig.Retention {
	case "interest":
		streamCfg.Retention = jetstream.InterestPolicy
	case "work_queue":
		streamCfg.Retention = jetstream.WorkQueuePolicy
	default:
		streamCfg.Retention = jetstream.LimitsPolicy
	}

	// Set optional limits
	if autoConfig.MaxBytes > 0 {
		streamCfg.MaxBytes = autoConfig.MaxBytes
	}
	if autoConfig.MaxMsgs > 0 {
		streamCfg.MaxMsgs = autoConfig.MaxMsgs
	}
	if autoConfig.Replicas > 0 {
		streamCfg.Replicas = autoConfig.Replicas
	}

	// Create the stream
	_, err = js.CreateStream(ctx, streamCfg)
	if err != nil {
		c.recordFailure()
		return errs.WrapTransient(err, "Client", "ensureStreamForConsumer",
			"failed to auto-create stream "+cfg.StreamName)
	}

	c.logger.Printf("auto-created stream %s with subjects %v", cfg.StreamName, subjects)
	return nil
}

// deriveStreamSubject converts a filter subject to a stream subject pattern.
// For example: "events.graph.entity.*" becomes "events.graph.entity.>"
func deriveStreamSubject(filterSubject string) string {
	// If already has >, use as-is
	if strings.HasSuffix(filterSubject, ">") {
		return filterSubject
	}

	// Replace trailing * with > for broader stream coverage
	if strings.HasSuffix(filterSubject, "*") {
		return filterSubject[:len(filterSubject)-1] + ">"
	}

	// For exact subjects, add > wildcard
	return filterSubject + ".>"
}

// PublishToStreamWithAck publishes a message to a JetStream subject with acknowledgment.
// If AutoCreate is true and the stream doesn't exist, it will be created.
func (c *Client) PublishToStreamWithAck(
	ctx context.Context,
	subject string,
	data []byte,
) (*jetstream.PubAck, error) {
	if c.Status() == StatusCircuitOpen {
		return nil, ErrCircuitOpen
	}

	if c.Status() != StatusConnected {
		return nil, ErrNotConnected
	}

	js, err := c.JetStream()
	if err != nil {
		return nil, err
	}

	ack, err := js.Publish(ctx, subject, data)
	if err != nil {
		c.recordFailure()
		c.jsMetrics.recordError("publish_to_stream")
		return nil, errs.WrapTransient(err, "Client", "PublishToStreamWithAck",
			"failed to publish to subject "+subject)
	}

	c.resetCircuit()
	return ack, nil
}

// StopConsumer stops a specific consumer by stream and consumer name.
func (c *Client) StopConsumer(streamName, consumerName string) {
	c.consumersMu.Lock()
	defer c.consumersMu.Unlock()

	key := streamName + ":" + consumerName
	if consumeCtx, ok := c.consumers[key]; ok {
		consumeCtx.Stop()
		delete(c.consumers, key)
	}
}

// StopAllConsumers stops all active consumers.
func (c *Client) StopAllConsumers() {
	c.consumersMu.Lock()
	defer c.consumersMu.Unlock()

	for key, consumeCtx := range c.consumers {
		consumeCtx.Stop()
		delete(c.consumers, key)
	}
}
