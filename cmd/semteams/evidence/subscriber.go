package evidence

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/nats-io/nats.go"
)

// loopCompletedSubject is the NATS wildcard that matches every
// agent.complete.* message the agentic-loop processor publishes on
// loop completion. The subject shape comes from the loop processor's
// default output port config ("agent.complete.*"), confirmed in
// processor/agentic-loop/config.go at v1.0.0-beta.45.
const loopCompletedSubject = "agent.complete.>"

// NATSSubscriber wraps a Preprocessor and drives it from a NATS
// subscription on loopCompletedSubject. Lifecycle is tied to the
// context passed to Start; cancellation drains and closes the
// subscription.
//
// The subscription is queue-group-less (point subscription, not a
// queue group) because the preprocessor runs exactly once per
// deployment — it doesn't need competing-consumer semantics. If a
// second backend instance ever lands, add a queue group name to avoid
// duplicate triple writes.
type NATSSubscriber struct {
	preprocessor *Preprocessor
	logger       *slog.Logger
}

// NewNATSSubscriber constructs a subscriber that drives p from NATS.
// The subscriber does not own p's lifecycle; callers retain the
// Preprocessor reference.
func NewNATSSubscriber(p *Preprocessor, logger *slog.Logger) *NATSSubscriber {
	if logger == nil {
		logger = slog.Default()
	}
	return &NATSSubscriber{preprocessor: p, logger: logger}
}

// Start registers the NATS subscription and returns when the
// subscription is active. The subscription is cancelled when ctx is
// done. Returns an error only when the subscription cannot be
// registered (e.g. NATS not connected); runtime message errors are
// logged but do not propagate.
func (s *NATSSubscriber) Start(ctx context.Context, client *natsclient.Client) error {
	sub, err := client.Subscribe(ctx, loopCompletedSubject, func(msgCtx context.Context, msg *nats.Msg) {
		s.handleMsg(msgCtx, msg.Data)
	})
	if err != nil {
		return err
	}

	// Unsubscribe when the context is cancelled. Running in a goroutine
	// because ctx.Done() is a blocking channel; we must not block Start's
	// caller.
	go func() {
		<-ctx.Done()
		if unsubErr := sub.Unsubscribe(); unsubErr != nil {
			s.logger.Debug("evidence subscriber: unsubscribe on context cancel",
				slog.String("error", unsubErr.Error()))
		}
	}()

	s.logger.Info("evidence preprocessor: NATS subscription active",
		slog.String("subject", loopCompletedSubject))
	return nil
}

// handleMsg decodes one BaseMessage envelope from the AGENT stream,
// checks the category, and dispatches to HandleLoopCompleted. Errors
// are logged and swallowed — the subscription handler must not surface
// them to the NATS library.
func (s *NATSSubscriber) handleMsg(ctx context.Context, data []byte) {
	// Light envelope decode: extract category + raw payload without
	// importing the full message package (which would add a dep cycle).
	var envelope struct {
		Type struct {
			Category string `json:"category"`
		} `json:"type"`
		Payload json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		s.logger.Debug("evidence subscriber: skip malformed envelope",
			slog.String("error", err.Error()))
		return
	}

	if envelope.Type.Category != agentic.CategoryLoopCompleted {
		// agent.complete.> carries both loop.completed and tool.result
		// messages on the AGENT stream. Skip anything that isn't
		// loop_completed — we only care about builder loop terminals.
		return
	}

	var ev agentic.LoopCompletedEvent
	if err := json.Unmarshal(envelope.Payload, &ev); err != nil {
		s.logger.Warn("evidence subscriber: skip malformed LoopCompletedEvent",
			slog.String("error", err.Error()))
		return
	}

	if err := s.preprocessor.HandleLoopCompleted(ctx, &ev); err != nil {
		// HandleLoopCompleted is fail-soft and should not return errors;
		// this branch is a belt-and-suspenders guard.
		s.logger.Error("evidence subscriber: HandleLoopCompleted returned error",
			slog.String("loop_id", ev.LoopID),
			slog.String("error", err.Error()))
	}
}
