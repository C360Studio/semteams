package chainpause

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/natsclient"
)

// DecisionVerb is the operator's resolution of a chain pause.
type DecisionVerb string

// Verb constants for the v1 closed operator namespace (ADR-037 §D2).
// apply_fix_and_retry is v2-reserved; no v3 verbs are defined yet.
const (
	VerbRetry DecisionVerb = "retry"
	VerbKill  DecisionVerb = "kill"
	VerbDefer DecisionVerb = "defer"
)

// Sentinel errors for verb validation. HTTP handlers use errors.Is to pick 400
// vs 500 status without brittle string matching (R4).
var (
	// ErrInvalidVerb is returned when the verb is not in the v1 closed set.
	ErrInvalidVerb = errors.New("decision verb not in v1 closed set")
	// ErrReservedVerb is returned when the verb is reserved for a future version.
	ErrReservedVerb = errors.New("decision verb reserved for future version")
)

// DecisionRequest is the HTTP body for POST /teams-loop/chain-pause/decide.
// Verb namespace is closed per ADR-037 §D2; the validator rejects unknown values.
type DecisionRequest struct {
	// FailedLoopID is the loop that triggered the pause.
	FailedLoopID string `json:"failed_loop_id"`
	// Verb is the operator's decision: retry | kill | defer.
	// v2-reserved: apply_fix_and_retry.
	// v3-reserved: (none in v1 scope).
	Verb string `json:"verb"`
	// Reason is free-text rationale, bounded at 1024 chars.
	Reason string `json:"reason,omitempty"`
	// UserID is optional; resolves via X-User-Id header first (ADR-030 identity seam).
	UserID string `json:"user_id,omitempty"`
}

// DecisionResponse is the HTTP response body for a successful decision.
type DecisionResponse struct {
	FailedLoopID string `json:"failed_loop_id"`
	Verb         string `json:"verb"`
	Accepted     bool   `json:"accepted"`
	Message      string `json:"message,omitempty"`
	Timestamp    string `json:"timestamp"`
}

// TaskPublisher is the narrow surface for re-publishing a TaskMessage on retry.
// The production wire is the NATS client publishing to the AGENT stream's
// agent.task.* subject; tests inject an in-memory recorder.
type TaskPublisher interface {
	PublishTask(ctx context.Context, subject string, task *agentic.TaskMessage) error
}

// PauseDataReader reads the role and original model from the §D5 triple set
// stamped on the chain entity at pause time (ADR-037 §D7, ADR-038 PR B
// Phase 3 — pre-Phase-3 the triples lived on the failed loop's entity).
//
// The production implementation makes a graph.query.entity NATS request and
// reads chain.paused.role + chain.paused.original_model from the returned
// EntityState. Tests inject a static in-memory implementation.
//
// Returns ("", "", nil) when the entity is not found — callers fall back to
// safe defaults ("dispatch", "claude-haiku").
type PauseDataReader interface {
	ReadPauseData(ctx context.Context, entityID string) (role, model string, err error)
}

// DecisionHandler handles operator decisions arriving at
// POST /teams-loop/chain-pause/decide. Only "operator" authority is wired in v1;
// the validator rejects coordinator and auto values at the HTTP boundary per ADR-037 §D3.
//
// ADR-038 PR B Phase 3: chain.decision.* / chain.resumed / chain.killed /
// chain.deferred all land on the canonical chain entity (resolved from
// the failed loop's ancestry). PauseDataReader reads chain.paused.role
// and chain.paused.original_model from the same chain entity at retry
// time, so the role+model used for the retry spawn match what the
// Pauser stamped at pause time.
type DecisionHandler struct {
	publisher TriplePublisher
	tasks     TaskPublisher
	pauseData PauseDataReader
	resolver  ChainEntityResolver
	logger    *slog.Logger
}

// NewDecisionHandler constructs a DecisionHandler. The resolver computes
// the chain entity ID for §D5 audit writes and PauseDataReader queries;
// cmd/semteams/chain.Resolver is the production wiring.
func NewDecisionHandler(pub TriplePublisher, tasks TaskPublisher, pauseData PauseDataReader, resolver ChainEntityResolver, logger *slog.Logger) *DecisionHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &DecisionHandler{
		publisher: pub,
		tasks:     tasks,
		pauseData: pauseData,
		resolver:  resolver,
		logger:    logger,
	}
}

// HandleDecision processes an operator's retry/kill/defer decision.
// Returns an error for unknown or reserved verbs — callers translate to HTTP 400.
// Partial triple write errors are returned but the action (retry/kill/defer) has
// already been dispatched; callers log but do not roll back.
func (h *DecisionHandler) HandleDecision(ctx context.Context, req DecisionRequest, actor string) error {
	switch DecisionVerb(req.Verb) {
	case VerbRetry, VerbKill, VerbDefer:
		// Valid v1 verbs — proceed.
	case "apply_fix_and_retry":
		// Reserved for v2; explicitly named so the error message is actionable.
		return fmt.Errorf("verb %q: %w (reserved for v2): valid verbs are retry, kill, defer", req.Verb, ErrReservedVerb)
	default:
		return fmt.Errorf("verb %q: %w: valid verbs are retry, kill, defer", req.Verb, ErrInvalidVerb)
	}

	reason := sanitiseReason(req.Reason)
	entityID, err := h.resolver.ChainEntityID(ctx, req.FailedLoopID)
	if err != nil {
		return fmt.Errorf("chainpause.DecisionHandler: resolve chain entity for failed loop %q: %w", req.FailedLoopID, err)
	}
	now := time.Now().UTC()

	// Write the decision audit trail (§D5 chain.decision.*).
	if err := h.writeDecisionTriples(ctx, entityID, req.Verb, actor, reason, now); err != nil {
		h.logger.Warn("chain-pause: partial decision triple write",
			slog.String("failed_loop_id", req.FailedLoopID),
			slog.String("verb", req.Verb),
			slog.String("error", err.Error()))
		// Do not return: the action (retry/kill/defer) must still be dispatched.
	}

	// Dispatch the action.
	switch DecisionVerb(req.Verb) {
	case VerbRetry:
		return h.retry(ctx, entityID, req.FailedLoopID, now)
	case VerbKill:
		return h.kill(ctx, entityID, now)
	case VerbDefer:
		return h.deferChain(ctx, entityID, now)
	}
	return nil // unreachable: switch above is exhaustive
}

// retry re-publishes a fresh TaskMessage for the failed loop's role so the
// chain resumes. The new loop reads upstream context via read_loop_result on
// the prior_loop_id pointer. Per ADR-037 §D7: no state replay; new loop ID.
//
// Role and model are read from the §D5 triple set stamped at pause time
// (chain.paused.role, chain.paused.original_model) so the retry faithfully
// re-spawns the original spawn shape. A "dispatch" / "claude-haiku" fallback
// is used only when the pause data reader cannot resolve the entity.
//
// If the retried loop still fails, another chain.paused.marker triple is
// written and the operator decides again.
func (h *DecisionHandler) retry(ctx context.Context, entityID, failedLoopID string, now time.Time) error {
	// Read role + model from the §D5 triple set written by the Pauser.
	role, model, err := h.pauseData.ReadPauseData(ctx, entityID)
	if err != nil {
		h.logger.Warn("chain-pause: pause data read failed; using fallback role=dispatch model=claude-haiku",
			slog.String("entity_id", entityID),
			slog.String("failed_loop_id", failedLoopID),
			slog.String("error", err.Error()))
	}
	if role == "" {
		role = "dispatch"
	}
	if model == "" {
		model = "claude-haiku"
	}

	// TODO(adr-037-d7, adr-036-bootstrap_workspace): v1 leaves builder retry
	// without workspace reset. If the failed role is builder, the
	// retried loop will see prior partial work in the worktree. Wire
	// bootstrap_workspace (ADR-036) before any real builder failure surfaces in
	// smoke. Earns its slot when the first builder retry smoke run lands.
	if role == "builder" {
		h.logger.Warn("chain-pause: builder retry without workspace reset (ADR-037 §D7 v1 limitation); prior partial work may be visible",
			slog.String("failed_loop_id", failedLoopID))
	}

	taskIDSuffix := failedLoopID
	if len(taskIDSuffix) > 8 {
		taskIDSuffix = taskIDSuffix[:8]
	}
	task := &agentic.TaskMessage{
		TaskID: fmt.Sprintf("chain-retry-%s-%d", taskIDSuffix, now.UnixNano()),
		Role:   role,
		Model:  model,
		Prompt: fmt.Sprintf("Chain retry initiated by operator. Prior failed loop: %s. Read context via read_loop_result(%s) and proceed per your role's persona.", failedLoopID, failedLoopID),
		Metadata: map[string]any{
			"chain_retry":   true,
			"prior_loop_id": failedLoopID,
			"decision_verb": "retry",
		},
	}

	// Publish to agent.task.<role> on the AGENT JetStream so agentic-loop picks
	// it up. Uses PublishToStream + NewBaseMessage envelope — the same pattern
	// that agentic-dispatch uses when spawning tasks (C1 fix; ADR-037 §D7).
	subject := "agent.task." + role
	if err := h.tasks.PublishTask(ctx, subject, task); err != nil {
		return fmt.Errorf("publish retry task: %w", err)
	}

	// Write chain.resumed triple with the new task ID as the value.
	return h.publisher.AddTriple(ctx, message.Triple{
		Subject:    entityID,
		Predicate:  "chain.decision.resumed_task_id",
		Object:     task.TaskID,
		Source:     "chainpause",
		Timestamp:  now,
		Confidence: 1.0,
	})
}

// kill terminates the chain cleanly. Writes chain.killed audit triple;
// no further rules fire on the failed lineage (per ADR-037 §D7).
func (h *DecisionHandler) kill(ctx context.Context, entityID string, now time.Time) error {
	return h.publisher.AddTriple(ctx, message.Triple{
		Subject:    entityID,
		Predicate:  "chain.decision.killed_at",
		Object:     now.Format(time.RFC3339),
		Source:     "chainpause",
		Timestamp:  now,
		Confidence: 1.0,
	})
}

// deferChain preserves chain state for later. The operator re-issues a retry
// decision via the same endpoint to resume.
func (h *DecisionHandler) deferChain(ctx context.Context, entityID string, now time.Time) error {
	return h.publisher.AddTriple(ctx, message.Triple{
		Subject:    entityID,
		Predicate:  "chain.decision.deferred_at",
		Object:     now.Format(time.RFC3339),
		Source:     "chainpause",
		Timestamp:  now,
		Confidence: 1.0,
	})
}

// writeDecisionTriples writes the §D5 chain.decision.* audit trail.
// Returns the first write error if any; all triples are attempted regardless.
func (h *DecisionHandler) writeDecisionTriples(ctx context.Context, entityID, verb, actor, reason string, now time.Time) error {
	triples := []message.Triple{
		{Subject: entityID, Predicate: "chain.decision.verb", Object: verb},
		{Subject: entityID, Predicate: "chain.decision.authority", Object: "operator"},
		{Subject: entityID, Predicate: "chain.decision.actor", Object: actor},
		{Subject: entityID, Predicate: "chain.decision.reason", Object: reason},
		{Subject: entityID, Predicate: "chain.decision.decided_at", Object: now.Format(time.RFC3339)},
	}
	var firstErr error
	for i := range triples {
		triples[i].Source = "chainpause"
		triples[i].Timestamp = now
		triples[i].Confidence = 1.0
		if err := h.publisher.AddTriple(ctx, triples[i]); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// sanitiseReason caps the free-text reason at 1024 bytes and strips control
// characters. Same discipline as ADR-030 sanitiseIdentity.
func sanitiseReason(s string) string {
	const maxBytes = 1024
	clean := sanitiseErrorShape(s) // reuse control-char stripper
	if len(clean) > maxBytes {
		clean = clean[:maxBytes] + "…"
	}
	return clean
}

// NATSTaskPublisher is a TaskPublisher backed by the shared NATS client.
// Publishes agent.task.* messages to the AGENT JetStream so the agentic-loop
// consumer picks them up. Messages are wrapped in a BaseMessage envelope — the
// same pattern that agentic-dispatch uses — so the downstream consumer can
// decode them correctly (C1 fix; ADR-037 §D7).
type NATSTaskPublisher struct {
	client *natsclient.Client
}

// NewNATSTaskPublisher constructs a TaskPublisher backed by the given NATS client.
func NewNATSTaskPublisher(client *natsclient.Client) *NATSTaskPublisher {
	return &NATSTaskPublisher{client: client}
}

// PublishTask wraps the TaskMessage in a BaseMessage envelope and publishes it
// to the AGENT JetStream via PublishToStream. This matches agentic-dispatch's
// publish pattern (component.go:741-749) so the agentic-loop consumer decodes
// the message correctly. Using core Publish (non-stream) or a bare JSON marshal
// silently drops the message at the consumer.
func (p *NATSTaskPublisher) PublishTask(ctx context.Context, subject string, task *agentic.TaskMessage) error {
	baseMsg := message.NewBaseMessage(task.Schema(), task, "chainpause-retry")
	data, err := json.Marshal(baseMsg)
	if err != nil {
		return fmt.Errorf("marshal base message: %w", err)
	}
	return p.client.PublishToStream(ctx, subject, data)
}

// NATSPauseDataReader reads role + model from the §D5 triple set on the chain
// entity via a graph.query.entity NATS request (ADR-037 §D7, ADR-038 PR B
// Phase 3). The triples chain.paused.role and chain.paused.original_model are
// written at pause time by the Pauser; the DecisionHandler resolves the chain
// entity ID for retry-time reads via the same chain.Resolver path.
//
// Returns ("", "", nil) when the entity is not found in the graph; callers fall
// back to safe defaults. Network errors are returned so callers can log and fall
// back.
type NATSPauseDataReader struct {
	client  *natsclient.Client
	subject string
}

// DefaultGraphQueryEntitySubject is the upstream graph-query processor's
// entity-read request/reply subject. Used as the fallback when
// cmd/semteams/main.go cannot resolve the subject from the running
// graph-query (or graph-ingest) component's port config — same shape
// as chain.DefaultGraphQueryEntitySubject. See portresolver.SubjectOrDefault
// wiring; operators can override per deployment.
const DefaultGraphQueryEntitySubject = "graph.query.entity"

// NewNATSPauseDataReader constructs a PauseDataReader backed by the given
// NATS client. subject is the entity-read RPC subject; empty falls back
// to DefaultGraphQueryEntitySubject. Mirrors chain.NewNATSEntityReader's
// resolution flow.
func NewNATSPauseDataReader(client *natsclient.Client, subject string) *NATSPauseDataReader {
	if subject == "" {
		subject = DefaultGraphQueryEntitySubject
	}
	return &NATSPauseDataReader{client: client, subject: subject}
}

// ReadPauseData queries graph.query.entity for the entity and extracts
// chain.paused.role and chain.paused.original_model from the response triples.
func (r *NATSPauseDataReader) ReadPauseData(ctx context.Context, entityID string) (role, model string, err error) {
	req := map[string]string{"id": entityID}
	reqData, err := json.Marshal(req)
	if err != nil {
		return "", "", fmt.Errorf("marshal entity query: %w", err)
	}

	const queryTimeout = 3 * time.Second
	queryCtx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	respData, err := r.client.Request(queryCtx, r.subject, reqData, queryTimeout)
	if err != nil {
		return "", "", fmt.Errorf("graph entity query: %w", err)
	}

	// The response is a JSON object that may be an EntityState directly or
	// wrapped in a QueryResponse envelope. Try EntityState first (simpler path
	// used by graph-ingest direct queries), then the envelope shape.
	var entity struct {
		ID      string `json:"id"`
		Triples []struct {
			Predicate string `json:"predicate"`
			Object    any    `json:"object"`
		} `json:"triples"`
	}
	if err := json.Unmarshal(respData, &entity); err != nil {
		return "", "", fmt.Errorf("decode entity response: %w", err)
	}

	for _, t := range entity.Triples {
		switch t.Predicate {
		case "chain.paused.role":
			if s, ok := t.Object.(string); ok {
				role = s
			}
		case "chain.paused.original_model":
			if s, ok := t.Object.(string); ok {
				model = s
			}
		}
	}
	return role, model, nil
}
