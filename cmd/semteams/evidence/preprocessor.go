package evidence

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/types"
	"github.com/c360studio/semteams/cmd/semteams/verification"
)

// TriplePublisher is the narrow surface the preprocessor uses to write
// evidence triples onto the builder loop entity. Production wires the
// NATS graph-mutation surface; tests inject an in-memory recorder so
// they need no NATS connection.
//
// Kept local for consumer-side decoupling — upstream's
// processor/agentic-tools.TriplePublisher structurally satisfies this
// interface, so production wiring needs no adapter.
type TriplePublisher interface {
	AddTriple(ctx context.Context, triple message.Triple) error
}

// Preprocessor subscribes to builder-loop completion events and stamps
// evidence.summary + evidence.summary_ready triples on the loop entity
// so rule_07's two-AND condition can fire exactly once, in either
// ordering of the triple writes.
//
// The preprocessor is fail-soft: any error that prevents reaching a
// verdict stamps a "(no checks file — chain plumbing failure: <reason>)"
// summary string so qa-reviewer routes to needs_clarification rather
// than silently hanging. It never returns errors to its subscription
// handler — bad state should not trigger NATS redelivery.
type Preprocessor struct {
	registry      *Registry
	publisher     TriplePublisher
	workspaceRoot string // absolute path to sandbox workspace mount; "" disables
	platform      types.PlatformMeta
	logger        *slog.Logger
}

// New constructs a Preprocessor. workspaceRoot="" disables the
// preprocessor: HandleLoopCompleted returns immediately on every event.
// registry must already have its Checkers registered (call
// RegisterBuiltins before New if the standard set is wanted).
func New(reg *Registry, pub TriplePublisher, workspaceRoot string, platform types.PlatformMeta, logger *slog.Logger) *Preprocessor {
	if logger == nil {
		logger = slog.Default()
	}
	return &Preprocessor{
		registry:      reg,
		publisher:     pub,
		workspaceRoot: workspaceRoot,
		platform:      platform,
		logger:        logger,
	}
}

// HandleLoopCompleted is the subscription entry point. It filters for
// dev-via-spec-builder loops that ended with tests_passing or
// tests_failing, reads .evidence/checks.json from the builder's
// workspace, runs Registry.Run per check, renders evidence.Summarize,
// and stamps:
//
//   - evidence.summary  (Object = rendered markdown summary)
//   - evidence.summary_ready  (Object = "true")
//
// Both triples are on the builder's loop entity ID (via
// agentic.LoopExecutionEntityID). Fail-soft: any error stamps a
// "(no checks file — chain plumbing failure: <reason>)" summary so
// qa-reviewer's Rule 3 routes to needs_clarification. evidence_ready is
// still stamped "true" on the error path — qa-reviewer must see it to
// unblock rule_07's two-AND condition regardless of what the summary
// contains.
//
// Never returns a non-nil error to the caller. Subscription handlers
// must not retry on bad state — log and move on.
func (p *Preprocessor) HandleLoopCompleted(ctx context.Context, ev *agentic.LoopCompletedEvent) error {
	if p.workspaceRoot == "" {
		return nil // preprocessor disabled; non-sandbox deployments skip silently
	}
	if ev.Role != "dev-via-spec-builder" {
		return nil // only builder loops carry architect checks
	}
	// We stamp on every successful builder loop completion regardless of
	// next_action. Filtering tests_passing|tests_failing happens at
	// rule_07 (it ANDs coordinator.next_action with evidence.summary_ready);
	// duplicating the filter here is dead code, and the coordinator-action
	// signal is on a triple, not on the LoopCompletedEvent payload.
	// needs_clarification builders still complete with OutcomeSuccess, so
	// they get a stamped summary too — rule_07 just doesn't fire on them.
	if ev.Outcome != agentic.OutcomeSuccess {
		return nil
	}

	entityID := agentic.LoopExecutionEntityID(p.platform.Org, p.platform.Platform, ev.LoopID)

	summary, err := p.buildSummary(ctx, ev.LoopID)
	if err != nil {
		summary = fmt.Sprintf("(no checks file — chain plumbing failure: %v)", err)
		p.logger.Warn("evidence preprocessor: summary build failed; stamping plumbing-failure summary",
			slog.String("loop_id", ev.LoopID),
			slog.String("error", err.Error()))
	}

	p.stampTriples(ctx, entityID, summary, ev.LoopID)
	return nil
}

// buildSummary reads .evidence/checks.json from the loop's workspace,
// runs all checks through the registry, and returns the rendered
// Summarize output. Returns an error when the file is missing, cannot
// be parsed, or the workspace root is inaccessible.
//
// An empty checks slice is not an error: Summarize renders
// "(no checks)\n" for it, which is the correct signal for qa-reviewer's
// "under-specified architect" path.
func (p *Preprocessor) buildSummary(ctx context.Context, loopID string) (string, error) {
	checksPath := filepath.Join(p.workspaceRoot, loopID, ".evidence", "checks.json")

	data, err := os.ReadFile(checksPath) // #nosec G304 — path is under workspaceRoot/loopID, both operator-controlled
	if err != nil {
		return "", fmt.Errorf("read checks.json at %s: %w", checksPath, err)
	}

	var checks []verification.Check
	if err := json.Unmarshal(data, &checks); err != nil {
		return "", fmt.Errorf("parse checks.json: %w", err)
	}

	if len(checks) == 0 {
		// Empty file is chain-plumbing-healthy; architect emitted no
		// checks. Summarize renders "(no checks)\n" — qa-reviewer's
		// Rule 3 routes appropriately.
		return Summarize(nil), nil
	}

	// Build an evidence.Context for workspace-relative path checks.
	// The workspace root for a specific loop is workspaceRoot/<loopID>.
	loopWorkspaceRoot := filepath.Join(p.workspaceRoot, loopID)
	ec, err := NewContext(loopWorkspaceRoot)
	if err != nil {
		return "", fmt.Errorf("build evidence context for loop %s: %w", loopID, err)
	}

	summaries := make([]CheckSummary, len(checks))
	for i, c := range checks {
		results, _ := p.registry.Run(ctx, c.Evidence, ec)
		summaries[i] = CheckSummary{
			Index:   i + 1,
			Target:  c.Target,
			Results: results,
		}
	}

	return Summarize(summaries), nil
}

// stampTriples writes the two triples onto the builder's loop entity.
// Errors are logged but do not propagate — best-effort stamping is
// correct here; a partial stamp on the qa-reviewer read is worse than
// a full stamp that came slightly late, but a missed write still lets
// the operator diagnose via the slog output.
func (p *Preprocessor) stampTriples(ctx context.Context, entityID, summary, loopID string) {
	now := time.Now().UTC()

	summaryTriple := message.Triple{
		Subject:    entityID,
		Predicate:  "evidence.summary",
		Object:     summary,
		Source:     "evidence-preprocessor",
		Timestamp:  now,
		Confidence: 1.0,
	}
	if err := p.publisher.AddTriple(ctx, summaryTriple); err != nil {
		p.logger.Error("evidence preprocessor: failed to stamp evidence.summary triple",
			slog.String("loop_id", loopID),
			slog.String("entity_id", entityID),
			slog.String("error", err.Error()))
		// Do not return; attempt the ready triple regardless so
		// rule_07 can still fire with an empty summary.
	}

	readyTriple := message.Triple{
		Subject:    entityID,
		Predicate:  "evidence.summary_ready",
		Object:     "true",
		Source:     "evidence-preprocessor",
		Timestamp:  now,
		Confidence: 1.0,
	}
	if err := p.publisher.AddTriple(ctx, readyTriple); err != nil {
		p.logger.Error("evidence preprocessor: failed to stamp evidence.summary_ready triple",
			slog.String("loop_id", loopID),
			slog.String("entity_id", entityID),
			slog.String("error", err.Error()))
	}
}
