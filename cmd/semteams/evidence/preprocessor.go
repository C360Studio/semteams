package evidence

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/types"
	"github.com/c360studio/semteams/cmd/semteams/slug"
	"github.com/c360studio/semteams/cmd/semteams/verification"
)

// defaultOutputDir is the markdown render target when
// SEMTEAMS_EVIDENCE_DIR is unset. Relative to the process working
// directory (the repo root in normal operation). Mirrors the sibling
// emit-tools (research / plan / consensus / spec).
const defaultOutputDir = "docs/evidence"

// envOutputDir is the operator override for the markdown output
// directory. Env-var rather than framework config so it does not drift
// the upstream config schema — same precedent as
// SEMTEAMS_RESEARCH_ARTIFACT_DIR / SEMTEAMS_PLAN_DIR / SEMTEAMS_CONSENSUS_DIR.
const envOutputDir = "SEMTEAMS_EVIDENCE_DIR"

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

// ChainResolver is the narrow surface the preprocessor uses to derive
// the canonical 6-part chain entity ID for the chain that contains the
// builder loop. cmd/semteams/chain.Resolver satisfies it structurally.
//
// Optional — leave unset to skip chain entity triple writes. ADR-038
// PR B drift-safe phasing: existing evidence.summary +
// evidence.summary_ready triples on the loop entity continue to land
// regardless; chain entity triples land on top when the resolver is
// wired. Rule_07's two-AND condition continues to match on the loop
// entity for now (rules can't trivially cross-reference a different
// entity); migrating the rule itself to chain entity matching is a
// separate follow-on once the rule engine's chain-entity surface is
// scoped — see ADR-038 D5 migration table.
type ChainResolver interface {
	ChainEntityID(ctx context.Context, loopID string) (string, error)
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
	outputDir     string // markdown render target for chain-entity evidence summary; resolved against env var + default at construction time
	platform      types.PlatformMeta
	logger        *slog.Logger
	chainResolver ChainResolver // nil → skip chain entity triples (drift-safe)
}

// New constructs a Preprocessor. workspaceRoot="" disables the
// preprocessor: HandleLoopCompleted returns immediately on every event.
// registry must already have its Checkers registered (call
// RegisterBuiltins before New if the standard set is wanted).
//
// outputDir is the directory where rendered markdown evidence summaries
// land (one per builder loop, named by slug.DeriveDated). Empty falls
// back to SEMTEAMS_EVIDENCE_DIR, then "docs/evidence". Markdown
// rendering only fires when SetChainResolver has been wired — the
// rendered file is the human-readable view of the chain.evidence.*
// triples, not a per-loop side-effect (loop-entity evidence.summary
// stays inline for rule_07 to read).
func New(reg *Registry, pub TriplePublisher, workspaceRoot, outputDir string, platform types.PlatformMeta, logger *slog.Logger) *Preprocessor {
	if logger == nil {
		logger = slog.Default()
	}
	if outputDir == "" {
		if envDir := strings.TrimSpace(os.Getenv(envOutputDir)); envDir != "" {
			outputDir = envDir
		} else {
			outputDir = defaultOutputDir
		}
	}
	return &Preprocessor{
		registry:      reg,
		publisher:     pub,
		workspaceRoot: workspaceRoot,
		outputDir:     outputDir,
		platform:      platform,
		logger:        logger,
	}
}

// SetChainResolver enables ADR-038 PR B Phase 5 chain entity triple
// writes alongside the existing loop-entity writes. Optional opt-in:
// leaving this unset keeps the preprocessor backward-compatible (loop-
// entity triples only; rule_07 unaffected).
//
// Wired in cmd/semteams/main.go after evidence.New when the chain
// package's Resolver is built.
func (p *Preprocessor) SetChainResolver(r ChainResolver) {
	p.chainResolver = r
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

// stampTriples writes the two triples onto the builder's loop entity
// (rule_07 fires on these). When SetChainResolver is wired (ADR-038 PR
// B Phase 5), additionally mirrors the same data onto the canonical
// chain entity for cross-arc consumers (ops agent, future analytics).
//
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

	// ADR-038 PR B Phase 5: mirror evidence triples onto the chain entity
	// for cross-arc visibility. Drift-safe — loop-entity triples above
	// continue to land regardless. Rule_07 keeps matching on the loop
	// entity for now; the rule-engine work to migrate that condition is
	// a separate follow-on (the engine's condition layer scopes to the
	// triggering entity, so chain-entity-rule firing needs more thought).
	p.stampChainTriples(ctx, summary, loopID, now)
}

// stampChainTriples writes chain.evidence.summary_ready and (when
// markdown rendering succeeds) chain.evidence.summary.path on the
// canonical 6-part chain entity. Fail-soft: a missing resolver,
// resolution error, render error, or per-triple write error logs Warn
// and continues — the loop-entity triples have already landed, so
// rule_07's downstream path is unaffected.
//
// ADR-038 PR C Phase C4: chain.evidence.summary (the inline text dump)
// is retired. The chain entity carries a reference (path) to the
// rendered markdown view; the canonical store is the loop entity
// (evidence.summary, which rule_07 reads inline) plus the on-disk
// markdown view. ADR-038 §D2 prescribes the path-only chain shape;
// §D3 names markdown the rendered view, not the canonical store.
//
// Race-free as of semstreams beta.57: persistHandlerResult now runs
// WriteLoopCompletion under a 2s budget BEFORE publishResults, so the
// builder's own agent.loop.parent is in graph KV by the time this
// preprocessor consumes the published agent.complete event. (Pre-fix
// behaviour was a documented race that left chain.evidence.* on a
// phantom chain entity when graph-write hadn't landed before publish;
// behaviourally inert because rule_07 reads loop-entity triples
// regardless, but cleaned up here now that the upstream fix is in.)
func (p *Preprocessor) stampChainTriples(ctx context.Context, summary, loopID string, now time.Time) {
	if p.chainResolver == nil {
		return
	}
	chainEntityID, err := p.chainResolver.ChainEntityID(ctx, loopID)
	if err != nil {
		p.logger.Warn("evidence preprocessor: chain ancestry walk failed; skipping chain triples",
			slog.String("loop_id", loopID),
			slog.String("error", err.Error()))
		return
	}

	// Render markdown BEFORE stamping triples so the path predicate is
	// only written when an on-disk file actually exists. Render failure
	// is non-fatal: chain.evidence.summary_ready still lands so
	// downstream consumers can route on readiness; the absence of
	// chain.evidence.summary.path is the "no markdown rendered" signal
	// (parallel to chain.research_artifact.path absence on legacy
	// research artifacts).
	relPath, renderErr := p.renderMarkdown(loopID, summary, now)
	if renderErr != nil {
		p.logger.Warn("evidence preprocessor: markdown render failed; chain.evidence.summary.path omitted",
			slog.String("loop_id", loopID),
			slog.String("chain_entity", chainEntityID),
			slog.String("error", renderErr.Error()))
		relPath = ""
	}

	chainReady := message.Triple{
		Subject:    chainEntityID,
		Predicate:  "chain.evidence.summary_ready",
		Object:     "true",
		Source:     "chain.evidence",
		Timestamp:  now,
		Confidence: 1.0,
	}
	if err := p.publisher.AddTriple(ctx, chainReady); err != nil {
		p.logger.Warn("evidence preprocessor: failed to stamp chain.evidence.summary_ready",
			slog.String("loop_id", loopID),
			slog.String("chain_entity", chainEntityID),
			slog.String("error", err.Error()))
		// Continue: path triple is independent.
	}

	if relPath == "" {
		return
	}
	chainPath := message.Triple{
		Subject:    chainEntityID,
		Predicate:  "chain.evidence.summary.path",
		Object:     relPath,
		Source:     "chain.evidence",
		Timestamp:  now,
		Confidence: 1.0,
	}
	if err := p.publisher.AddTriple(ctx, chainPath); err != nil {
		p.logger.Warn("evidence preprocessor: failed to stamp chain.evidence.summary.path",
			slog.String("loop_id", loopID),
			slog.String("chain_entity", chainEntityID),
			slog.String("path", relPath),
			slog.String("error", err.Error()))
	}
}

// renderMarkdown writes the rendered evidence summary to
// outputDir/<slug>.md. Slug is derived via slug.DeriveDated with the
// "evidence" fallback prefix — the architect doesn't supply a title
// (the preprocessor runs at builder-loop completion, which has no
// LLM-author input here), so the slug always lands as
// "<YYYY-MM-DD>-evidence-<loopID[:8]>". Same input → same path → safe
// to overwrite on retry.
//
// Returns the path written, joined under outputDir. When outputDir is
// relative (the default "docs/evidence" or a relative env override),
// the returned path is repo-relative and reads cleanly in `git diff` /
// markdown links. When outputDir is absolute (operator override or
// t.TempDir() in tests), the returned path is absolute. Downstream
// consumers reading chain.evidence.summary.path must tolerate both
// shapes (parallel to chain.research_artifact.path).
func (p *Preprocessor) renderMarkdown(loopID, summary string, now time.Time) (string, error) {
	if err := os.MkdirAll(p.outputDir, 0o755); err != nil {
		return "", fmt.Errorf("create output dir %s: %w", p.outputDir, err)
	}

	tmpl, err := template.New("evidence-summary").Parse(evidenceTemplateText)
	if err != nil {
		return "", fmt.Errorf("parse evidence markdown template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, map[string]any{
		"LoopID":     loopID,
		"ProducedAt": now.UTC().Format("2006-01-02 15:04 UTC"),
		"Summary":    summary,
	}); err != nil {
		return "", fmt.Errorf("execute evidence markdown template: %w", err)
	}

	filename := slug.DeriveDated("", loopID, "evidence", now) + ".md"
	fullPath := filepath.Join(p.outputDir, filename)
	if err := os.WriteFile(fullPath, buf.Bytes(), 0o644); err != nil {
		return "", fmt.Errorf("write evidence file %s: %w", fullPath, err)
	}

	return filepath.Join(p.outputDir, filename), nil
}

// evidenceTemplateText renders the chain-entity evidence summary as a
// readable markdown view. ADR-038 D3: tool template renders headings;
// the body is the existing Summarize() output (per-check aggregates +
// per-rule status lines). No persona-supplied content — this preprocessor
// runs at builder-loop completion with no LLM author. The header
// metadata (loop, produced) lets operators correlate the file back to a
// specific run; the body is the same prose rule_07 / qa-reviewer read
// inline from the loop entity.
const evidenceTemplateText = "# Evidence summary\n\n" +
	"> Loop: `{{ .LoopID }}`  ·  Produced: {{ .ProducedAt }}\n\n" +
	"{{ .Summary }}"
