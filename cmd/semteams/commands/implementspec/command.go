// Package implementspec wires the /implement-spec power-user command.
//
// The command is intentionally small: it does not infer infrastructure, spawn
// a team directly, or create a parallel control plane. It analyzes existing
// proof.* facts on the selected run entity, stamps formal_claims.* via the
// same deterministic analyzer used by the tool, and records the explicit
// dev_from_task.requested marker only when the analyzer routes to implementation.
package implementspec

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/natsclient"
	agenticdispatch "github.com/c360studio/semstreams/processor/agentic-dispatch"
	agentictools "github.com/c360studio/semstreams/processor/agentic-tools"
	"github.com/c360studio/semstreams/types"

	"github.com/c360studio/semteams/cmd/semteams/chain"
	"github.com/c360studio/semteams/cmd/semteams/tools/analyzeproof"
)

const (
	// The dispatch permission model only exposes coarse built-ins today. Treat
	// implementation handoff as an approval action rather than ordinary submit.
	commandPermission = "approve"
	defaultReason     = "Human approved the OpenSpec artifact and requested implementation handoff."

	statusPassed = "passed"
)

// EntityReader reads one graph entity's triples into a predicate-object map.
type EntityReader interface {
	ReadEntity(ctx context.Context, entityID string) (map[string]any, error)
}

// TriplePublisher writes co-located graph triples atomically.
type TriplePublisher interface {
	Append(ctx context.Context, triples []message.Triple) error
}

// Dependencies groups the live graph read/write surfaces the command needs.
type Dependencies struct {
	Reader    EntityReader
	Publisher TriplePublisher
}

// DependenciesFactory creates graph dependencies from dispatch's NATS client.
type DependenciesFactory func(*natsclient.Client) (Dependencies, error)

// Option customizes Command construction, primarily for tests.
type Option func(*Command)

// Command implements agenticdispatch.CommandExecutor.
type Command struct {
	platform    types.PlatformMeta
	logger      *slog.Logger
	now         func() time.Time
	depsFactory DependenciesFactory
}

// NewCommand constructs the /implement-spec command executor.
func NewCommand(platform types.PlatformMeta, logger *slog.Logger, opts ...Option) *Command {
	if logger == nil {
		logger = slog.Default()
	}
	c := &Command{
		platform: platform,
		logger:   logger,
		now:      time.Now,
		depsFactory: func(client *natsclient.Client) (Dependencies, error) {
			if client == nil {
				return Dependencies{}, fmt.Errorf("nats client unavailable")
			}
			return Dependencies{
				Reader:    chain.NewNATSEntityReader(client, chain.DefaultGraphQueryEntitySubject),
				Publisher: agentictools.NewNATSTriplePublisher(client),
			}, nil
		},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// WithNow injects a clock for deterministic tests.
func WithNow(now func() time.Time) Option {
	return func(c *Command) {
		if now != nil {
			c.now = now
		}
	}
}

// WithDependenciesFactory injects graph dependencies for tests.
func WithDependenciesFactory(factory DependenciesFactory) Option {
	return func(c *Command) {
		if factory != nil {
			c.depsFactory = factory
		}
	}
}

// Config returns dispatch registration metadata for /implement-spec.
func (c *Command) Config() agenticdispatch.CommandConfig {
	return agenticdispatch.CommandConfig{
		Pattern:     `^/implement-spec(?:\s+(\S+))?(?:\s+(.*))?$`,
		Permission:  commandPermission,
		RequireLoop: false,
		Help:        "/implement-spec [change-slug] - analyze proof facts on the selected run and request implementation when ready",
	}
}

// Execute analyzes proof facts and, when ready, records the implementation request.
func (c *Command) Execute(
	ctx context.Context,
	cmdCtx *agenticdispatch.CommandContext,
	msg agentic.UserMessage,
	args []string,
	loopID string,
) (agentic.UserResponse, error) {
	if cmdCtx == nil {
		return agentic.UserResponse{}, fmt.Errorf("command context unavailable")
	}
	if !hasCommandPermission(cmdCtx, msg.UserID) {
		return agentic.UserResponse{}, fmt.Errorf("permission denied: requires %q", commandPermission)
	}

	deps, err := c.depsFactory(cmdCtx.NATSClient)
	if err != nil {
		return agentic.UserResponse{}, err
	}
	if deps.Reader == nil {
		return agentic.UserResponse{}, fmt.Errorf("entity reader unavailable")
	}
	if deps.Publisher == nil {
		return agentic.UserResponse{}, fmt.Errorf("triple publisher unavailable")
	}

	target, reason := parseArgs(args)
	runID, runEntityID, changeSlug, err := c.resolveTarget(cmdCtx, msg, target)
	if err != nil {
		return agentic.UserResponse{}, err
	}

	triples, err := deps.Reader.ReadEntity(ctx, runEntityID)
	if err != nil {
		return agentic.UserResponse{}, fmt.Errorf("read run entity %s: %w", runEntityID, err)
	}
	if changeSlug != "" {
		intentPred := fmt.Sprintf("change.%s.proposal.intent", changeSlug)
		if _, ok := triples[intentPred]; !ok {
			return agentic.UserResponse{}, fmt.Errorf("run %s has no OpenSpec change %q", runID, changeSlug)
		}
	}

	now := c.now().UTC()
	analysis := analyzeproof.Analyze(triples, now)
	out := analysis.Triples(runEntityID, now)
	ready := implementationReady(analysis, out)
	if ready {
		out = append(out, requestTriples(runEntityID, msg, changeSlug, reason, now)...)
	}
	if err := deps.Publisher.Append(ctx, out); err != nil {
		return agentic.UserResponse{}, fmt.Errorf("write implementation handoff facts for %s: %w", runEntityID, err)
	}

	c.logger.Info("/implement-spec evaluated",
		slog.String("run_id", runID),
		slog.String("run_entity_id", runEntityID),
		slog.String("change_slug", changeSlug),
		slog.String("formal_claims_status", analysis.Status),
		slog.Bool("implementation_requested", ready))

	return response(msg, responseContent(analysis, ready, changeSlug)), nil
}

func parseArgs(args []string) (target, reason string) {
	if len(args) > 0 {
		target = strings.TrimSpace(args[0])
	}
	if len(args) > 1 {
		reason = strings.TrimSpace(args[1])
	}
	return target, reason
}

func hasCommandPermission(cmdCtx *agenticdispatch.CommandContext, userID string) bool {
	if cmdCtx.HasPermission == nil {
		return false
	}
	return cmdCtx.HasPermission(userID, commandPermission)
}

func (c *Command) resolveTarget(
	cmdCtx *agenticdispatch.CommandContext,
	msg agentic.UserMessage,
	target string,
) (runID, runEntityID, changeSlug string, err error) {
	if isRunIdentifier(target) {
		return "", "", "", fmt.Errorf("/implement-spec takes an OpenSpec change slug, not a run id; select the run in the UI")
	}
	changeSlug = target
	runID = strings.TrimSpace(msg.RunID)
	if runID == "" {
		return "", "", "", fmt.Errorf("select a task before /implement-spec")
	}

	runEntityID, err = agentic.TryChainExecutionEntityID(c.platform.Org, c.platform.Platform, runID)
	if err != nil {
		return "", "", "", fmt.Errorf("resolve run entity for %q: %w", runID, err)
	}
	if err := authorizeSelectedRun(cmdCtx, msg, runID); err != nil {
		return "", "", "", err
	}

	return runID, runEntityID, changeSlug, nil
}

func authorizeSelectedRun(cmdCtx *agenticdispatch.CommandContext, msg agentic.UserMessage, runID string) error {
	if cmdCtx.LoopTracker == nil {
		return fmt.Errorf("loop tracker unavailable; cannot verify selected run access")
	}
	info := cmdCtx.LoopTracker.Get(runID)
	if info == nil {
		return fmt.Errorf("selected run %q is not tracked; refresh and select the task again", runID)
	}
	if msg.UserID == "" || info.UserID == "" || info.UserID != msg.UserID {
		return fmt.Errorf("permission denied for selected run %q", runID)
	}
	return nil
}

func implementationReady(analysis analyzeproof.Analysis, triples []message.Triple) bool {
	if analysis.Status != statusPassed {
		return false
	}
	for _, tr := range triples {
		if tr.Predicate == "formal_claims.route.implementation" && fmt.Sprint(tr.Object) == "present" {
			return true
		}
	}
	return false
}

func requestTriples(runEntityID string, msg agentic.UserMessage, changeSlug, reason string, now time.Time) []message.Triple {
	if reason == "" {
		reason = defaultReason
	}
	requestID := msg.MessageID
	if requestID == "" {
		requestID = uuid.NewString()
	}
	mk := func(pred string, obj any) message.Triple {
		return message.Triple{
			Subject: runEntityID, Predicate: pred, Object: obj,
			Source: "implement-spec-command", Timestamp: now, Confidence: 1.0,
		}
	}
	out := []message.Triple{
		mk("dev_from_task.requested", "implement-spec:"+requestID),
		mk("dev_from_task.requested_reason", reason),
		mk("dev_from_task.requested_at", now.Format(time.RFC3339Nano)),
		mk("dev_from_task.requested_by", msg.UserID),
	}
	if changeSlug != "" {
		out = append(out, mk("dev_from_task.requested_change_slug", changeSlug))
	}
	return out
}

func responseContent(analysis analyzeproof.Analysis, ready bool, changeSlug string) string {
	target := "the selected OpenSpec change"
	if changeSlug != "" {
		target = "OpenSpec change " + changeSlug
	}
	if ready {
		return fmt.Sprintf("%s is proof-ready; implementation handoff requested.", target)
	}
	return fmt.Sprintf("%s is not implementation-ready yet. Proof analysis status: %s with %d finding(s).", target, analysis.Status, len(analysis.Findings))
}

func response(msg agentic.UserMessage, content string) agentic.UserResponse {
	return agentic.UserResponse{
		ResponseID:  uuid.NewString(),
		ChannelType: msg.ChannelType,
		ChannelID:   msg.ChannelID,
		UserID:      msg.UserID,
		InReplyTo:   msg.MessageID,
		Type:        agentic.ResponseTypeStatus,
		Content:     content,
		Timestamp:   time.Now().UTC(),
	}
}

func isRunIdentifier(s string) bool {
	return isChainExecutionEntityID(s) || isLikelyBareRunID(s)
}

func isLikelyBareRunID(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" || strings.Contains(s, ".") {
		return false
	}
	if strings.HasPrefix(s, "loop_") || strings.HasPrefix(s, "run_") {
		return true
	}
	if _, err := uuid.Parse(s); err == nil {
		return true
	}
	return false
}

func isChainExecutionEntityID(s string) bool {
	parts := strings.Split(strings.TrimSpace(s), ".")
	if len(parts) != 6 {
		return false
	}
	return parts[2] == "agent" && parts[3] == "chain" && parts[4] == "execution"
}
