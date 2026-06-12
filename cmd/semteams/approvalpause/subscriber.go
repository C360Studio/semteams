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

// DefaultApprovalResponseSubject is the dispatch processor's output subject for
// approval-response events (the human's approve/reject/modify decision, published by
// POST /teams-dispatch/loops/{id}/approval). The agentic-loop ALSO subscribes here to
// resume the gated loop; this subscriber reads it in parallel to stamp the run-phase
// resume marker (4c PR-2). Same `.>` safe-superset rationale as the pending default.
const DefaultApprovalResponseSubject = "agent.approval_response.>"

// Subscriber wraps a Pauser and drives both halves of the 4c tool-gate from
// core-NATS subscriptions: the PAUSE on agent.approval_pending.* and the RESUME on
// agent.approval_response.* (same core-Subscribe shape as chainpause — both events
// are delivered to core subscribers in real time even though they are also captured
// by the AGENT JetStream stream). Lifecycle is tied to the Start context.
type Subscriber struct {
	pauser          *Pauser
	pendingSubject  string
	responseSubject string
	logger          *slog.Logger
}

// NewSubscriber constructs a Subscriber. pendingSubject/responseSubject are the NATS
// wildcards the two subscriptions bind to; empty falls back to the package defaults.
// main.go resolves the live subjects from the teams-loop / teams-dispatch port config
// via portresolver.SubjectOrDefault and passes the results here. Does not start the
// subscriptions; call Start to activate.
func NewSubscriber(p *Pauser, pendingSubject, responseSubject string, logger *slog.Logger) *Subscriber {
	if logger == nil {
		logger = slog.Default()
	}
	if pendingSubject == "" {
		pendingSubject = DefaultApprovalPendingSubject
	}
	if responseSubject == "" {
		responseSubject = DefaultApprovalResponseSubject
	}
	return &Subscriber{pauser: p, pendingSubject: pendingSubject, responseSubject: responseSubject, logger: logger}
}

// Start registers both NATS subscriptions and returns when they are active.
// Cancelling ctx drains and unsubscribes cleanly.
//
// Partial-Start contract: if the second subscribe fails, the FIRST subscription is
// still live (its cleanup goroutine is bound to ctx, not to this error path). The
// caller MUST cancel ctx on a non-nil Start error or the first subscription leaks —
// startApprovalPauseSubscriber satisfies this by propagating the error to boot, which
// aborts and cancels the parent ctx (the orphan drains). Do not change the caller to
// log-and-continue without tearing the first subscription down here.
func (s *Subscriber) Start(ctx context.Context, client *natsclient.Client) error {
	if err := s.subscribe(ctx, client, s.pendingSubject, s.handlePendingMsg, "pending"); err != nil {
		return err
	}
	if err := s.subscribe(ctx, client, s.responseSubject, s.handleResponseMsg, "response"); err != nil {
		return err
	}
	s.logger.Info("approval-pause subscriber: NATS subscriptions active",
		slog.String("pending_subject", s.pendingSubject),
		slog.String("response_subject", s.responseSubject))
	return nil
}

// subscribe binds one core-NATS subscription and arranges ctx-cancel cleanup.
func (s *Subscriber) subscribe(ctx context.Context, client *natsclient.Client, subject string, handle func(context.Context, []byte), label string) error {
	sub, err := client.Subscribe(ctx, subject, func(msgCtx context.Context, msg *nats.Msg) {
		handle(msgCtx, msg.Data)
	})
	if err != nil {
		return err
	}
	go func() {
		<-ctx.Done()
		if unsubErr := sub.Unsubscribe(); unsubErr != nil {
			s.logger.Debug("approval-pause subscriber: unsubscribe on context cancel",
				slog.String("which", label),
				slog.String("error", unsubErr.Error()))
		}
	}()
	return nil
}

// handlePendingMsg decodes a BaseMessage envelope, checks for the approval_pending
// category, and dispatches to HandlePending. Errors are logged and swallowed —
// subscription handlers must not surface them to the NATS library.
func (s *Subscriber) handlePendingMsg(ctx context.Context, data []byte) {
	category, payload, ok := decodeEnvelope(data, s.logger)
	if !ok || category != agentic.CategoryApprovalPending {
		return
	}
	var ev agentic.ApprovalPendingEvent
	if err := json.Unmarshal(payload, &ev); err != nil {
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

// handleResponseMsg decodes a BaseMessage envelope, checks for the approval_response
// category, and dispatches to HandleResponse (4c PR-2 resume).
func (s *Subscriber) handleResponseMsg(ctx context.Context, data []byte) {
	category, payload, ok := decodeEnvelope(data, s.logger)
	if !ok || category != agentic.CategoryApprovalResponse {
		return
	}
	var ev agentic.ApprovalResponse
	if err := json.Unmarshal(payload, &ev); err != nil {
		s.logger.Warn("approval-pause subscriber: skip malformed ApprovalResponse",
			slog.String("error", err.Error()))
		return
	}

	result, err := s.pauser.HandleResponse(ctx, &ev)
	if err != nil {
		s.logger.Error("approval-pause subscriber: HandleResponse error",
			slog.String("loop_id", ev.LoopID),
			slog.String("decision", ev.Decision),
			slog.String("error", err.Error()))
		return
	}
	if result.Stamped {
		s.logger.Info("approval-pause: approval resolved, run resuming",
			slog.String("loop_id", result.LoopID),
			slog.String("run_entity_id", result.RunEntityID),
			slog.String("decision", result.Decision))
		return
	}
	s.logger.Debug("approval-pause: approval response for a run-less loop, skipping run-phase resume",
		slog.String("loop_id", result.LoopID),
		slog.String("decision", result.Decision))
}

// decodeEnvelope unmarshals the BaseMessage shell, returning the payload category
// and raw payload. ok is false on a malformed envelope (logged at Debug).
func decodeEnvelope(data []byte, logger *slog.Logger) (category string, payload json.RawMessage, ok bool) {
	var envelope struct {
		Type struct {
			Category string `json:"category"`
		} `json:"type"`
		Payload json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		logger.Debug("approval-pause subscriber: skip malformed envelope",
			slog.String("error", err.Error()))
		return "", nil, false
	}
	return envelope.Type.Category, envelope.Payload, true
}
