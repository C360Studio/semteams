package chainpause

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/nats-io/nats.go"
)

// DefaultLoopFailedSubject is the upstream agentic-loop processor's
// output port for loop-failure events as of v1.0.0-beta.57. Used as
// the fallback when cmd/semteams/main.go cannot resolve the subject
// from the running agentic-loop component's port config — see
// portresolver.SubjectOrDefault wiring.
//
// Operators can override per deployment by editing the agentic-loop
// component's `outputs[name="agent.failed"].subject` field.
const DefaultLoopFailedSubject = "agent.failed.>"

// Subscriber wraps a Pauser and drives it from a NATS subscription on
// the configured subject. Lifecycle is tied to the context passed to
// Start.
type Subscriber struct {
	pauser  *Pauser
	subject string
	logger  *slog.Logger
}

// NewSubscriber constructs a Subscriber. subject is the NATS wildcard
// the subscription binds to; empty falls back to DefaultLoopFailedSubject.
// main.go is expected to resolve the live subject from agentic-loop's
// port config via portresolver.SubjectOrDefault and pass the result
// here. Does not start the subscription; call Start to activate.
func NewSubscriber(p *Pauser, subject string, logger *slog.Logger) *Subscriber {
	if logger == nil {
		logger = slog.Default()
	}
	if subject == "" {
		subject = DefaultLoopFailedSubject
	}
	return &Subscriber{pauser: p, subject: subject, logger: logger}
}

// Start registers the NATS subscription and returns when it is active.
// Cancelling ctx drains and unsubscribes cleanly.
func (s *Subscriber) Start(ctx context.Context, client *natsclient.Client) error {
	sub, err := client.Subscribe(ctx, s.subject, func(msgCtx context.Context, msg *nats.Msg) {
		s.handleMsg(msgCtx, msg.Data)
	})
	if err != nil {
		return err
	}

	go func() {
		<-ctx.Done()
		if unsubErr := sub.Unsubscribe(); unsubErr != nil {
			s.logger.Debug("chain-pause subscriber: unsubscribe on context cancel",
				slog.String("error", unsubErr.Error()))
		}
	}()

	s.logger.Info("chain-pause subscriber: NATS subscription active",
		slog.String("subject", s.subject))
	return nil
}

// handleMsg decodes the BaseMessage envelope, checks for the loop_failed
// category, and dispatches to HandleFailed. Errors are logged and swallowed
// — subscription handlers must not surface them to the NATS library.
func (s *Subscriber) handleMsg(ctx context.Context, data []byte) {
	var envelope struct {
		Type struct {
			Category string `json:"category"`
		} `json:"type"`
		Payload json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		s.logger.Debug("chain-pause subscriber: skip malformed envelope",
			slog.String("error", err.Error()))
		return
	}

	if envelope.Type.Category != agentic.CategoryLoopFailed {
		// agent.failed.> can carry other categories in theory; skip non-failed.
		return
	}

	var ev agentic.LoopFailedEvent
	if err := json.Unmarshal(envelope.Payload, &ev); err != nil {
		s.logger.Warn("chain-pause subscriber: skip malformed LoopFailedEvent",
			slog.String("error", err.Error()))
		return
	}

	result, err := s.pauser.HandleFailed(ctx, &ev)
	if err != nil {
		// Two error classes share this branch: resolver failure (zero
		// triples written, error wraps the resolve step) and per-triple
		// AddTriple failure (partial §D5 cluster). The wrapped error
		// string disambiguates; the message intentionally stays generic.
		s.logger.Error("chain-pause subscriber: HandleFailed error",
			slog.String("failed_loop_id", ev.LoopID),
			slog.String("role", ev.Role),
			slog.String("error", err.Error()))
	}
	if result.FailedLoopID != "" {
		// Only log when HandleFailed actually processed the event (managed role).
		s.logger.Info("chain-pause: loop failure detected, pause triples written",
			slog.String("failed_loop_id", result.FailedLoopID),
			slog.String("role", result.Role),
			slog.String("cause", result.Cause),
			slog.String("classification", result.Classification))
	}
}
