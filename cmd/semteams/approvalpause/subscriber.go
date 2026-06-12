package approvalpause

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/nats-io/nats.go"
)

// DefaultApprovalPendingSubject is the upstream agentic-loop processor's output
// subject for approval-pending events. The agentic-loop publishes via
// component.ResolveSubject(outputs, "agent.approval_pending", loopID); when an
// operator does not declare the port the fallback is agent.approval_pending.<loopID>.
// Used as the wildcard fallback when cmd/semteams/main.go cannot resolve the subject
// from the running teams-loop component's port config — see portresolver.SubjectOrDefault.
//
// Operators can override per deployment by editing the teams-loop component's
// outputs[name="agent.approval_pending"].subject field.
//
// The `.>` (multi-token) default is deliberately the safe superset: it is only
// reached when the port is NOT declared, and matches whatever shape the publisher
// emits. The live wiring resolves the declared port's `agent.approval_pending.*`
// instead (mirrors chainpause's `agent.failed.>` default vs `agent.failed.*` port).
const DefaultApprovalPendingSubject = "agent.approval_pending.>"

// Subscriber wraps a Pauser and drives it from a core-NATS subscription on the
// configured subject (the same core-Subscribe shape as chainpause — approval-pending
// events are delivered to core subscribers in real time even though they are also
// captured by the AGENT JetStream stream). Lifecycle is tied to the Start context.
type Subscriber struct {
	pauser  *Pauser
	subject string
	logger  *slog.Logger
}

// NewSubscriber constructs a Subscriber. subject is the NATS wildcard the
// subscription binds to; empty falls back to DefaultApprovalPendingSubject. main.go
// resolves the live subject from teams-loop's port config via
// portresolver.SubjectOrDefault and passes the result here. Does not start the
// subscription; call Start to activate.
func NewSubscriber(p *Pauser, subject string, logger *slog.Logger) *Subscriber {
	if logger == nil {
		logger = slog.Default()
	}
	if subject == "" {
		subject = DefaultApprovalPendingSubject
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
			s.logger.Debug("approval-pause subscriber: unsubscribe on context cancel",
				slog.String("error", unsubErr.Error()))
		}
	}()

	s.logger.Info("approval-pause subscriber: NATS subscription active",
		slog.String("subject", s.subject))
	return nil
}

// handleMsg decodes the BaseMessage envelope, checks for the approval_pending
// category, and dispatches to HandlePending. Errors are logged and swallowed —
// subscription handlers must not surface them to the NATS library.
func (s *Subscriber) handleMsg(ctx context.Context, data []byte) {
	var envelope struct {
		Type struct {
			Category string `json:"category"`
		} `json:"type"`
		Payload json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		s.logger.Debug("approval-pause subscriber: skip malformed envelope",
			slog.String("error", err.Error()))
		return
	}

	if envelope.Type.Category != agentic.CategoryApprovalPending {
		// The wildcard could in theory carry other categories; skip non-pending.
		return
	}

	var ev agentic.ApprovalPendingEvent
	if err := json.Unmarshal(envelope.Payload, &ev); err != nil {
		s.logger.Warn("approval-pause subscriber: skip malformed ApprovalPendingEvent",
			slog.String("error", err.Error()))
		return
	}

	result, err := s.pauser.HandlePending(ctx, &ev)
	if err != nil {
		// Unlike chainpause (which writes 9 §D5 triples and can partially
		// succeed, so it logs the error AND still surfaces the partial result),
		// HandlePending is all-or-nothing — a single AddTriple, so an error means
		// Stamped==false and there is nothing further to surface. Early-return.
		s.logger.Error("approval-pause subscriber: HandlePending error",
			slog.String("loop_id", ev.LoopID),
			slog.String("tool_name", ev.ToolName),
			slog.String("error", err.Error()))
		return
	}
	if result.Stamped {
		s.logger.Info("approval-pause: tool-gate detected, run paused on approval",
			slog.String("loop_id", result.LoopID),
			slog.String("run_entity_id", result.RunEntityID),
			slog.String("tool_name", result.ToolName))
		return
	}
	// Not stamped + no error = a run-less loop (no run to pause). Debug, not Info —
	// this is the expected front-door / standalone case, not a problem.
	s.logger.Debug("approval-pause: gated loop has no run anchor, skipping run-phase pause",
		slog.String("loop_id", result.LoopID),
		slog.String("tool_name", result.ToolName))
}
