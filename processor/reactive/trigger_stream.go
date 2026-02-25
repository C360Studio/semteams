package reactive

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

// SubjectConsumer manages JetStream subject consumers for message-triggered rules.
// It consumes messages from NATS subjects and triggers rule evaluation when
// messages arrive.
type SubjectConsumer struct {
	logger    *slog.Logger
	consumers map[string]jetstream.ConsumeContext // key -> consume context
	mu        sync.RWMutex

	// shutdown signals the consumer goroutines to stop
	shutdown     chan struct{}
	shutdownOnce sync.Once
}

// NewSubjectConsumer creates a new subject consumer.
func NewSubjectConsumer(logger *slog.Logger) *SubjectConsumer {
	return &SubjectConsumer{
		logger:    logger,
		consumers: make(map[string]jetstream.ConsumeContext),
		shutdown:  make(chan struct{}),
	}
}

// SubjectMessageHandler is called when a message arrives on a subject.
// The handler should call msg.Ack(), msg.Nak(), or msg.Term() to acknowledge the message.
type SubjectMessageHandler func(ctx context.Context, event SubjectMessageEvent, msg jetstream.Msg)

// SubjectMessageEvent represents a message received on a subject.
type SubjectMessageEvent struct {
	// Subject is the NATS subject the message was received on.
	Subject string

	// Data is the raw message payload.
	Data []byte

	// Timestamp is when the message was received.
	Timestamp time.Time

	// StreamSequence is the sequence number in the stream.
	StreamSequence uint64

	// DeliveryCount is how many times this message has been delivered.
	DeliveryCount uint64
}

// StartConsumer starts consuming messages from a JetStream stream/subject.
// The handler is called for each message received.
func (c *SubjectConsumer) StartConsumer(
	ctx context.Context,
	js jetstream.JetStream,
	streamName string,
	subject string,
	consumerName string,
	handler SubjectMessageHandler,
) error {
	key := consumerKey(streamName, consumerName)

	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if already consuming
	if _, exists := c.consumers[key]; exists {
		c.logger.Debug("Consumer already exists",
			"stream", streamName,
			"consumer", consumerName)
		return nil
	}

	// Get the stream
	stream, err := js.Stream(ctx, streamName)
	if err != nil {
		return &ConsumerError{
			Stream:   streamName,
			Consumer: consumerName,
			Op:       "get_stream",
			Cause:    err,
		}
	}

	// Create or update the consumer
	consumer, err := stream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		Durable:       consumerName,
		FilterSubject: subject,
		AckPolicy:     jetstream.AckExplicitPolicy,
		AckWait:       60 * time.Second, // Allow time for async processing
		MaxDeliver:    5,                // Retry up to 5 times
	})
	if err != nil {
		return &ConsumerError{
			Stream:   streamName,
			Consumer: consumerName,
			Op:       "create_consumer",
			Cause:    err,
		}
	}

	// Start consuming
	consumeCtx, err := consumer.Consume(func(msg jetstream.Msg) {
		c.handleMessage(ctx, subject, msg, handler)
	})
	if err != nil {
		return &ConsumerError{
			Stream:   streamName,
			Consumer: consumerName,
			Op:       "start_consume",
			Cause:    err,
		}
	}

	c.consumers[key] = consumeCtx
	c.logger.Info("Started subject consumer",
		"stream", streamName,
		"subject", subject,
		"consumer", consumerName)
	return nil
}

// StopConsumer stops a specific consumer.
func (c *SubjectConsumer) StopConsumer(streamName, consumerName string) {
	key := consumerKey(streamName, consumerName)

	c.mu.Lock()
	defer c.mu.Unlock()

	if consumeCtx, exists := c.consumers[key]; exists {
		consumeCtx.Stop()
		delete(c.consumers, key)
		c.logger.Info("Stopped subject consumer",
			"stream", streamName,
			"consumer", consumerName)
	}
}

// StopAll stops all active consumers.
// Safe to call multiple times.
func (c *SubjectConsumer) StopAll() {
	c.shutdownOnce.Do(func() {
		close(c.shutdown)
	})

	c.mu.Lock()
	defer c.mu.Unlock()

	for key, consumeCtx := range c.consumers {
		consumeCtx.Stop()
		c.logger.Debug("Stopped consumer", "key", key)
	}

	c.consumers = make(map[string]jetstream.ConsumeContext)
	c.logger.Info("Stopped all subject consumers")
}

// handleMessage processes an incoming message with panic recovery.
func (c *SubjectConsumer) handleMessage(
	ctx context.Context,
	subject string,
	msg jetstream.Msg,
	handler SubjectMessageHandler,
) {
	defer func() {
		if r := recover(); r != nil {
			c.logger.Error("Panic in message handler",
				"subject", subject,
				"error", r)
			// Nak on panic to allow redelivery
			_ = msg.Nak()
		}
	}()

	// Get message metadata
	meta, err := msg.Metadata()
	var streamSeq, deliveryCount uint64
	if err == nil && meta != nil {
		streamSeq = meta.Sequence.Stream
		deliveryCount = meta.NumDelivered
	}

	event := SubjectMessageEvent{
		Subject:        msg.Subject(),
		Data:           msg.Data(),
		Timestamp:      time.Now(),
		StreamSequence: streamSeq,
		DeliveryCount:  deliveryCount,
	}

	handler(ctx, event, msg)
}

// consumerKey creates a unique key for stream+consumer combination.
func consumerKey(stream, consumer string) string {
	return stream + ":" + consumer
}

// ConsumerError represents an error with a subject consumer.
type ConsumerError struct {
	Stream   string
	Consumer string
	Op       string
	Cause    error
}

// Error implements the error interface.
func (e *ConsumerError) Error() string {
	return "consumer " + e.Stream + ":" + e.Consumer + " " + e.Op + ": " + e.Cause.Error()
}

// Unwrap returns the underlying error.
func (e *ConsumerError) Unwrap() error {
	return e.Cause
}

// BuildRuleContextFromMessage builds a RuleContext from a subject message event.
// If stateLoader is provided, it loads state from KV using the extracted key.
func BuildRuleContextFromMessage(
	event SubjectMessageEvent,
	messageFactory func() any,
	stateLoader func(key string) (any, uint64, error),
	stateKeyFunc func(msg any) string,
) (*RuleContext, error) {
	// Deserialize the message into the expected type
	typedMsg := messageFactory()
	if err := json.Unmarshal(event.Data, typedMsg); err != nil {
		return nil, &MessageDeserializeError{Subject: event.Subject, Cause: err}
	}

	ctx := &RuleContext{
		Message: typedMsg,
		Subject: event.Subject,
	}

	// If we have a state loader and key function, load state
	if stateLoader != nil && stateKeyFunc != nil {
		key := stateKeyFunc(typedMsg)
		if key != "" {
			state, revision, err := stateLoader(key)
			if err != nil {
				return nil, &StateLoadError{Key: key, Cause: err}
			}
			ctx.State = state
			ctx.KVRevision = revision
			ctx.KVKey = key
		}
	}

	return ctx, nil
}

// BuildRuleContextFromMessageWithKV builds a RuleContext from a message with KV state lookup.
// This is the combined message+state trigger pattern.
func BuildRuleContextFromMessageWithKV(
	ctx context.Context,
	event SubjectMessageEvent,
	messageFactory func() any,
	bucket jetstream.KeyValue,
	stateFactory func() any,
	stateKeyFunc func(msg any) string,
) (*RuleContext, error) {
	// Deserialize the message
	typedMsg := messageFactory()
	if err := json.Unmarshal(event.Data, typedMsg); err != nil {
		return nil, &MessageDeserializeError{Subject: event.Subject, Cause: err}
	}

	ruleCtx := &RuleContext{
		Message: typedMsg,
		Subject: event.Subject,
	}

	// Extract key and load state
	key := stateKeyFunc(typedMsg)
	if key == "" {
		return nil, &StateKeyError{Subject: event.Subject, Message: "state key function returned empty key"}
	}

	entry, err := bucket.Get(ctx, key)
	if err != nil {
		return nil, &StateLoadError{Key: key, Cause: err}
	}

	// Deserialize state
	state := stateFactory()
	if err := json.Unmarshal(entry.Value(), state); err != nil {
		return nil, &UnmarshalError{Key: key, Cause: err}
	}

	ruleCtx.State = state
	ruleCtx.KVRevision = entry.Revision()
	ruleCtx.KVKey = key

	return ruleCtx, nil
}

// MessageDeserializeError represents an error deserializing a message.
type MessageDeserializeError struct {
	Subject string
	Cause   error
}

// Error implements the error interface.
func (e *MessageDeserializeError) Error() string {
	return "failed to deserialize message on " + e.Subject + ": " + e.Cause.Error()
}

// Unwrap returns the underlying error.
func (e *MessageDeserializeError) Unwrap() error {
	return e.Cause
}

// StateLoadError represents an error loading state from KV.
type StateLoadError struct {
	Key   string
	Cause error
}

// Error implements the error interface.
func (e *StateLoadError) Error() string {
	return "failed to load state for " + e.Key + ": " + e.Cause.Error()
}

// Unwrap returns the underlying error.
func (e *StateLoadError) Unwrap() error {
	return e.Cause
}

// StateKeyError represents an error extracting the state key from a message.
type StateKeyError struct {
	Subject string
	Message string
}

// Error implements the error interface.
func (e *StateKeyError) Error() string {
	return "state key error on " + e.Subject + ": " + e.Message
}
