package chain

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/nats-io/nats.go"
)

// DefaultLoopCompletedSubject is the upstream agentic-loop processor's
// output port for loop-complete events as of v1.0.0-beta.57. Used as
// the fallback when cmd/semteams/main.go cannot resolve the subject
// from the running agentic-loop component's port config — see
// portresolver.SubjectOrDefault wiring.
//
// Exported because main.go and tests both reference it. Operators can
// override per deployment by editing the agentic-loop component's
// `outputs[name="agent.complete"].subject` field; that override flows
// through portresolver and into NewCompletionSubscriber's subject
// argument.
const DefaultLoopCompletedSubject = "agent.complete.>"

// CompletionHandler is the narrow surface a chain-milestone stamper
// implements to receive agent.complete events. New milestones add
// implementations and register them with CompletionSubscriber.
//
// Implementations should be no-op for events that don't match their
// milestone (e.g., DispatchedStamper returns nil for events with a
// non-empty ParentLoopID). Returning an error is for write failures
// the caller should log; it does not abort other handlers in the
// dispatch chain.
//
// Implementations may assume sequential per-event invocation; the
// subscriber does not parallelize handlers. Implementations MUST NOT
// rely on panicking to surface errors — the subscriber recovers
// panics defensively (so one buggy handler can't kill the milestone
// subscription for every chain in the system) but treats them as a
// contract violation and logs accordingly.
type CompletionHandler interface {
	HandleLoopCompleted(ctx context.Context, ev *agentic.LoopCompletedEvent) error
}

// CompletionSubscriber subscribes once to agent.complete.> and demuxes
// each LoopCompletedEvent to every registered CompletionHandler. Per-
// handler errors are logged with handler context; one bad handler does
// not block siblings.
//
// Single subscription instead of one-per-milestone for two reasons:
// (1) message decoding happens once per event, not N times;
// (2) handlers added later (research milestone, evidence-summary
// milestone) plug in by registration, not by adding new NATS
// subscriptions.
type CompletionSubscriber struct {
	handlers []CompletionHandler
	subject  string
	logger   *slog.Logger
}

// NewCompletionSubscriber constructs a subscriber with the given
// handlers and NATS subject. Order is preserved — handlers run
// sequentially per event. nil entries are skipped so callers can pass
// slice indices that may be conditionally empty.
//
// subject is the NATS wildcard the subscription binds to. Empty falls
// back to DefaultLoopCompletedSubject — main.go is expected to resolve
// the live subject from agentic-loop's port config via
// portresolver.SubjectOrDefault and pass the result here. Empty +
// fallback exists for the test path and for configs that omit the
// port declaration entirely.
func NewCompletionSubscriber(handlers []CompletionHandler, subject string, logger *slog.Logger) *CompletionSubscriber {
	if logger == nil {
		logger = slog.Default()
	}
	if subject == "" {
		subject = DefaultLoopCompletedSubject
	}
	out := make([]CompletionHandler, 0, len(handlers))
	for _, h := range handlers {
		if h == nil {
			continue
		}
		out = append(out, h)
	}
	return &CompletionSubscriber{handlers: out, subject: subject, logger: logger}
}

// Start registers the NATS subscription and returns when active.
// Cancelling ctx drains and unsubscribes.
func (s *CompletionSubscriber) Start(ctx context.Context, client *natsclient.Client) error {
	if len(s.handlers) == 0 {
		s.logger.Info("chain completion subscriber: no handlers registered, skipping subscription")
		return nil
	}

	sub, err := client.Subscribe(ctx, s.subject, func(msgCtx context.Context, msg *nats.Msg) {
		s.handleMsg(msgCtx, msg.Data)
	})
	if err != nil {
		return err
	}

	go func() {
		<-ctx.Done()
		if unsubErr := sub.Unsubscribe(); unsubErr != nil {
			s.logger.Debug("chain completion subscriber: unsubscribe on context cancel",
				slog.String("error", unsubErr.Error()))
		}
	}()

	s.logger.Info("chain completion subscriber: NATS subscription active",
		slog.String("subject", s.subject),
		slog.Int("handlers", len(s.handlers)))
	return nil
}

// handleMsg decodes one BaseMessage envelope, filters to loop_completed,
// and dispatches to every registered handler. Errors per handler are
// logged but do not abort the chain.
func (s *CompletionSubscriber) handleMsg(ctx context.Context, data []byte) {
	var envelope struct {
		Type struct {
			Category string `json:"category"`
		} `json:"type"`
		Payload json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		s.logger.Debug("chain completion subscriber: skip malformed envelope",
			slog.String("error", err.Error()))
		return
	}
	if envelope.Type.Category != agentic.CategoryLoopCompleted {
		// agent.complete.> carries other categories (e.g. tool results in
		// some configurations). Skip non-completed.
		return
	}

	var ev agentic.LoopCompletedEvent
	if err := json.Unmarshal(envelope.Payload, &ev); err != nil {
		s.logger.Warn("chain completion subscriber: skip malformed LoopCompletedEvent",
			slog.String("error", err.Error()))
		return
	}

	for i, h := range s.handlers {
		s.invokeHandler(ctx, i, h, &ev)
	}
}

// invokeHandler runs one handler with a panic-recover guard. A handler
// that panics — programmer error, contract violation — must not kill
// the subscriber's NATS callback goroutine, since that would silently
// halt every milestone write for every chain across the deployment.
// Recovered panics are logged with handler index, loop_id, role, and
// recover value; the loop continues to siblings.
func (s *CompletionSubscriber) invokeHandler(ctx context.Context, idx int, h CompletionHandler, ev *agentic.LoopCompletedEvent) {
	// handler_type carries the concrete Go type name for forensic
	// debugging — index alone won't identify which stamper if the
	// handler slice is later refactored or reordered.
	handlerType := fmt.Sprintf("%T", h)
	defer func() {
		if r := recover(); r != nil {
			s.logger.Error("chain completion subscriber: handler panicked (contract violation)",
				slog.Int("handler_index", idx),
				slog.String("handler_type", handlerType),
				slog.String("loop_id", ev.LoopID),
				slog.String("role", ev.Role),
				slog.Any("recovered", r))
		}
	}()
	if err := h.HandleLoopCompleted(ctx, ev); err != nil {
		s.logger.Error("chain completion subscriber: handler returned error",
			slog.Int("handler_index", idx),
			slog.String("handler_type", handlerType),
			slog.String("loop_id", ev.LoopID),
			slog.String("role", ev.Role),
			slog.String("error", err.Error()))
	}
}
