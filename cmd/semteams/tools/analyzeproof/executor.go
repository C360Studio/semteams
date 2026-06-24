package analyzeproof

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/c360studio/semstreams/agentic"
	agentictools "github.com/c360studio/semstreams/processor/agentic-tools"
	"github.com/c360studio/semstreams/types"

	"github.com/c360studio/semteams/cmd/semteams/runanchor"
)

// ToolName is the LLM-facing tool name.
const ToolName = "analyze_proof_readiness"

// EntityReader reads a graph entity's triples into a predicate→object map.
type EntityReader interface {
	ReadEntity(ctx context.Context, entityID string) (map[string]any, error)
}

// Executor implements agentic.ToolExecutor for analyze_proof_readiness.
type Executor struct {
	reader    EntityReader
	publisher agentictools.TriplePublisher
	platform  types.PlatformMeta
	now       func() time.Time
	logger    *slog.Logger
}

// NewExecutor constructs an Executor. reader and publisher must be non-nil.
func NewExecutor(
	reader EntityReader,
	publisher agentictools.TriplePublisher,
	platform types.PlatformMeta,
	logger *slog.Logger,
) *Executor {
	if reader == nil {
		panic("analyzeproof.NewExecutor: reader must not be nil")
	}
	if publisher == nil {
		panic("analyzeproof.NewExecutor: publisher must not be nil")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Executor{
		reader: reader, publisher: publisher, platform: platform,
		now: time.Now, logger: logger,
	}
}

// Execute reads proof.* facts from the current run entity, analyzes them, and
// stamps the formal_claims.* envelope back onto the same run entity.
func (e *Executor) Execute(ctx context.Context, call agentic.ToolCall) (agentic.ToolResult, error) {
	if call.Name != ToolName {
		return errResult(call, agentic.ToolErrorNotFound, "unknown tool: %s", call.Name)
	}
	if err := parseArgs(call.Arguments); err != nil {
		return errResult(call, agentic.ToolErrorInvalidArgs, "%v", err)
	}

	_, runEntityID := runanchor.Anchor(call, e.platform.Org, e.platform.Platform)
	if runEntityID == "" {
		return errResult(call, agentic.ToolErrorInvalidArgs,
			"analyze_proof_readiness: no run anchor on the call; this tool reads proof facts off the run entity")
	}

	triples, err := e.reader.ReadEntity(ctx, runEntityID)
	if err != nil {
		return errResult(call, agentic.ToolErrorNetwork, "read run entity %s: %v", runEntityID, err)
	}

	now := e.now().UTC()
	facts := ParseProofFacts(triples)
	analysis := AnalyzeFacts(facts, now)
	out := analysis.Triples(runEntityID, now)
	if err := e.publisher.AddTriplesBatch(ctx, out); err != nil {
		return errResult(call, agentic.ToolErrorNetwork, "stamp formal_claims triples on %s: %v", runEntityID, err)
	}

	body, _ := json.Marshal(map[string]any{
		"run_entity_id": runEntityID,
		"status":        analysis.Status,
		"version":       analysis.Version,
		"finding_count": len(analysis.Findings),
		"findings":      analysis.Findings,
		"proof_facts":   summarizeProofFacts(facts),
	})
	e.logger.Info("analyze_proof_readiness stamped",
		slog.String("run_entity_id", runEntityID),
		slog.String("status", analysis.Status),
		slog.Int("finding_count", len(analysis.Findings)))

	return agentic.ToolResult{
		CallID:  call.ID,
		Name:    call.Name,
		Content: string(body),
		Metadata: map[string]any{
			"run_entity_id": runEntityID,
			"status":        analysis.Status,
			"finding_count": len(analysis.Findings),
		},
	}, nil
}

func summarizeProofFacts(facts ProofFacts) map[string]any {
	return map[string]any{
		"dependencies": summarizeDependencies(facts.Dependencies),
		"readiness":    summarizeReadiness(facts.Readiness),
		"evidence":     summarizeEvidence(facts.Evidence),
		"waivers":      summarizeWaivers(facts.Waivers),
	}
}

func summarizeDependencies(deps map[string]*Dependency) []map[string]any {
	out := make([]map[string]any, 0, len(deps))
	for _, id := range mapKeys(deps) {
		d := deps[id]
		out = append(out, map[string]any{
			"id":           d.ID,
			"kind":         d.Kind,
			"description":  d.Description,
			"required_for": d.RequiredFor,
			"status":       d.Status,
			"profile_ref":  d.ProfileRef,
			"next_route":   d.NextRoute,
		})
	}
	return out
}

func summarizeReadiness(readiness map[string]*Readiness) []map[string]any {
	out := make([]map[string]any, 0, len(readiness))
	for _, id := range mapKeys(readiness) {
		r := readiness[id]
		out = append(out, map[string]any{
			"id":                r.ID,
			"profile_ref":       r.ProfileRef,
			"status":            r.Status,
			"started_at":        r.StartedAt,
			"completed_at":      r.CompletedAt,
			"expires_at":        r.ExpiresAt,
			"probe_results":     r.ProbeResults,
			"smoke_command":     r.SmokeCommand,
			"smoke_status":      r.SmokeStatus,
			"attestation_ref":   r.AttestationRef,
			"evidence":          r.Evidence,
			"failure_signature": r.FailureSignature,
		})
	}
	return out
}

func summarizeEvidence(evidence map[string]*Evidence) []map[string]any {
	out := make([]map[string]any, 0, len(evidence))
	for _, id := range mapKeys(evidence) {
		ev := evidence[id]
		out = append(out, map[string]any{
			"id":         ev.ID,
			"kind":       ev.Kind,
			"uri":        ev.URI,
			"digest":     ev.Digest,
			"producer":   ev.Producer,
			"command":    ev.Command,
			"exit_code":  ev.ExitCode,
			"created_at": ev.CreatedAt,
			"covers":     ev.Covers,
		})
	}
	return out
}

func summarizeWaivers(waivers map[string]*Waiver) []map[string]any {
	out := make([]map[string]any, 0, len(waivers))
	for _, id := range mapKeys(waivers) {
		w := waivers[id]
		out = append(out, map[string]any{
			"id":            w.ID,
			"reason":        w.Reason,
			"approved_by":   w.ApprovedBy,
			"approved_at":   w.ApprovedAt,
			"expires_at":    w.ExpiresAt,
			"claims":        w.Claims,
			"dependencies":  w.Dependencies,
			"residual_risk": w.ResidualRisk,
			"status":        w.Status,
		})
	}
	return out
}

func parseArgs(raw map[string]any) error {
	if len(raw) == 0 {
		return nil
	}
	buf, err := json.Marshal(raw)
	if err != nil {
		return fmt.Errorf("marshal arguments: %w", err)
	}
	var a struct{}
	dec := json.NewDecoder(strings.NewReader(string(buf)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&a); err != nil {
		return fmt.Errorf("unmarshal arguments: %w", err)
	}
	return nil
}

func errResult(call agentic.ToolCall, kind agentic.ToolErrorKind, format string, args ...any) (agentic.ToolResult, error) {
	return agentic.ToolResult{
		CallID:    call.ID,
		Name:      call.Name,
		Error:     fmt.Sprintf(format, args...),
		ErrorKind: kind,
	}, nil
}
