// Package emitartifact implements the SemTeams-local emit_research_artifact
// tool. The tool is the producer side of ADR-031's R3.2 mode-transition
// machinery: it writes the marker triples that downstream rules match on
// and publishes the typed research.artifact.v1 payload on a stable
// JetStream subject for audit / forward-compatibility.
//
// See cmd/semteams/research/artifact.go for the payload type and the
// 2026-04-30 addendum to docs/adr/031-research-flow-and-semspec-handoff.md
// for the stabilisation-as-converged-change-log model that motivates the
// triple set.
//
// Discipline note (commission-not-omission): this tool is the
// research-domain instance of upstream's planned generic write_artifact
// suite (ADR-028 §What's not built here, verified absent in semstreams
// beta.27). It mirrors the canonical agent-terminal-tool emission shape
// of upstream `decide` and `emit_diagnosis` deliberately — see
// ADR-031 §addendum 2026-04-30 "Framework-alignment review for R3.2
// emission shape" for the four alternatives we considered and ruled out
// with upstream evidence for each rejection.
//
// If you are reading this because you want to:
//
//   - Add a similar emit-* tool for another domain → read
//     cmd/semteams/tools/README.md "Discipline" section first. Survey
//     upstream; if a near-equivalent ships in semstreams, port don't
//     fork.
//   - Generalise this tool into a shape-agnostic emit_artifact in
//     product code → STOP. The migration target is upstream's planned
//     write_artifact, not a SemTeams-local generalisation. Open an
//     ADR addendum evaluating migration when upstream ships it.
//   - Stuff additional artifact content into the marker triples (e.g.
//     full actor names, integration-point text) → STOP. Marker triples
//     carry references and counts only. Content lives in the typed
//     payload; consumers query it via read_loop_result or subscribe
//     to the payload subject. See ADR-028 §Layer 2.
package emitartifact

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/message"
	agentictools "github.com/c360studio/semstreams/processor/agentic-tools"
	"github.com/c360studio/semstreams/types"

	"github.com/c360studio/semteams/cmd/semteams/research"
)

// ToolName is the LLM-facing tool name. Listed in agentic-tools
// allowed_tools per deployment that runs the research flow.
const ToolName = "emit_research_artifact"

// toolSource is the Source field on triples this tool publishes. Lets
// operators distinguish artifact-emitter triples from coordinator-decide
// or rule-driven mutations at a glance.
const toolSource = "research-emit-artifact"

// payloadSubjectPrefix is the JetStream subject namespace under which the
// typed Artifact payload is published. Per-loop suffix: one subject per
// research loop, multiple messages (one per revision) over the loop's
// lifetime. Stable across revisions so future audit subscribers can scope
// to a single research run.
const payloadSubjectPrefix = "research.artifact"

// Predicate names the executor stamps on the loop entity. Kept as
// string constants (rather than a separate vocabulary package) until a
// second consumer needs them — promote to vocabulary/research/ if we
// add a graph-query tool that filters on them.
const (
	predicateRevision                  = "research.artifact.revision"
	predicateActorsCount               = "research.artifact.actors_count"
	predicateIntegrationPointsCount    = "research.artifact.integration_points_count"
	predicateTasksCount                = "research.artifact.tasks_count"
	predicateAddressedGapsCount        = "research.artifact.addressed_gaps_count"
	predicateOpenGapsCount             = "research.artifact.open_gaps_count"
	predicateLastRevisionMutationCount = "research.artifact.last_revision_mutation_count"
	predicateProducedAt                = "research.artifact.produced_at"
	// predicateTestHarness is emitted only when the researcher chose
	// a catalog test harness (Artifact.TestHarness != ""). Absence of
	// the triple is the catalog-miss signal — paired with a
	// `needs_test_harness:` line in OpenGaps. R3.7.3 coordinator
	// routing keys off this triple's presence to decide between
	// auto-resume and harness-via-spec escalation. Object is the test
	// harness name (a stable catalog key, not free-text content) —
	// discipline rule preserved.
	predicateTestHarness = "research.artifact.test_harness"
)

// PayloadPublisher is the narrow surface the executor uses to publish
// the typed Artifact payload onto a stable NATS subject.
// *natsclient.Client satisfies it via its core Publish — no JetStream
// ack required because the payload is forward-compat audit, not a rule
// trigger. Core NATS has no replay buffer, so audit consumers must be
// subscribed at publish time; if durability is needed later, an operator
// can add a JetStream stream binding `research.artifact.>` without
// touching this code. Named for parity with agentictools.TriplePublisher.
type PayloadPublisher interface {
	Publish(ctx context.Context, subject string, data []byte) error
}

// Executor implements agentic.ToolExecutor for emit_research_artifact.
// It writes a deterministic set of marker triples on the calling loop's
// entity (so rule conditions can match revision / mutation-count without
// parsing JSON) and publishes the typed payload on a stable subject for
// audit / forward-compat consumers.
type Executor struct {
	publisher   agentictools.TriplePublisher
	natsPublish PayloadPublisher
	platform    types.PlatformMeta
	logger      *slog.Logger
}

// NewExecutor constructs an Executor.
func NewExecutor(publisher agentictools.TriplePublisher, natsPublish PayloadPublisher, platform types.PlatformMeta, logger *slog.Logger) *Executor {
	if logger == nil {
		logger = slog.Default()
	}
	return &Executor{
		publisher:   publisher,
		natsPublish: natsPublish,
		platform:    platform,
		logger:      logger,
	}
}

// ListTools returns the LLM-facing schema. The args mirror the Artifact
// JSON shape directly (no wrapping object) so the LLM can think in
// terms of the artifact's structure rather than nesting under a single
// field. loop_id and produced_at are derived server-side — the LLM
// neither sees nor sets them.
func (e *Executor) ListTools() []agentic.ToolDefinition {
	actorSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{"type": "string", "description": "Actor name (e.g. \"OSH driver framework\")."},
			"role": map[string]any{"type": "string", "description": "One-line role description for this actor."},
		},
		"required": []string{"name", "role"},
	}
	integrationPointSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"from":      map[string]any{"type": "string", "description": "Source actor name."},
			"to":        map[string]any{"type": "string", "description": "Destination actor name."},
			"data":      map[string]any{"type": "string", "description": "What data flows across this point."},
			"direction": map[string]any{"type": "string", "enum": []string{"read", "write"}, "description": "Direction of the data flow."},
		},
		"required": []string{"from", "to"},
	}
	mutationSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"tool":        map[string]any{"type": "string", "description": "Name of the substrate-modifying tool that ran (e.g. \"add_source_repo\")."},
			"args":        map[string]any{"type": "object", "description": "Original tool arguments, verbatim."},
			"loop_id":     map[string]any{"type": "string", "description": "Loop that emitted the mutation. Use this loop's id."},
			"revision":    map[string]any{"type": "integer", "minimum": 1, "description": "Artifact revision when the mutation landed."},
			"approved_by": map[string]any{"type": "string", "description": "Approver identity for gated mutations."},
			"status":      map[string]any{"type": "string", "enum": []string{"approved", "rejected", "executed", "failed"}},
			"result":      map[string]any{"type": "string", "description": "Free-form result summary."},
			"timestamp":   map[string]any{"type": "string", "description": "RFC3339 timestamp of the mutation."},
		},
		"required": []string{"tool", "loop_id", "revision", "status", "timestamp"},
	}
	return []agentic.ToolDefinition{{
		Name:        ToolName,
		Description: "Emit a research artifact snapshot for this loop. Writes marker triples on the loop entity (revision, counts, last-revision mutation count) so downstream rules can gate the dev-via-spec mode transition, and publishes the typed research.artifact.v1 payload on a stable subject for audit. Call once per researcher pass before submit_work; revision must monotonically increase across passes within the same research arc.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"revision":            map[string]any{"type": "integer", "minimum": 1, "description": "This pass's revision number, starting at 1 and monotonic."},
				"actors":              map[string]any{"type": "array", "items": actorSchema, "description": "External systems/frameworks/libraries the work touches, each with a one-line role."},
				"integration_points":  map[string]any{"type": "array", "items": integrationPointSchema, "description": "Actor-to-actor data flows with direction."},
				"tasks":               map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Decomposable-grain tasks derived from the research."},
				"addressed_gaps":      map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Reviewer-flagged gaps this pass closed."},
				"open_gaps":           map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Reviewer-flagged gaps still outstanding after this pass. If no test harness in the catalog matches this work, include a 'needs_test_harness: <one-line description of the integration target>' entry here so the reviewer can route the gap; the chain treats `needs_test_harness` as a structured marker, not free text."},
				"substrate_mutations": map[string]any{"type": "array", "items": mutationSchema, "description": "Append-only log of substrate-modifying tool calls across all revisions of this artifact (e.g. add_source_repo). Include prior revisions' entries verbatim plus any new ones from this pass."},
				"test_harness":        map[string]any{"type": "string", "description": "Name of the test harness from configs/harnesses.json that will verify the integration this research describes. Omit (or leave empty) if no catalog entry matches; in that case ALSO add a `needs_test_harness: ...` line to open_gaps so the reviewer sees the gap explicitly. The reviewer enforces the either/or."},
			},
			"required": []string{"revision", "actors", "integration_points", "tasks"},
		},
	}}
}

// Execute parses the args into a research.Artifact, validates it,
// publishes the marker triples, and publishes the typed payload.
// LLM-facing argument errors land on Result.Error with nil error so
// the framework retry loop can surface them; transport failures
// (publisher.AddTriple, natsPublish.Publish) land on Result.Error
// with ErrorKind=network.
func (e *Executor) Execute(ctx context.Context, call agentic.ToolCall) (agentic.ToolResult, error) {
	if call.Name != ToolName {
		return agentic.ToolResult{
			CallID:    call.ID,
			Name:      call.Name,
			Error:     fmt.Sprintf("unknown tool: %s", call.Name),
			ErrorKind: agentic.ToolErrorNotFound,
		}, nil
	}

	if call.LoopID == "" {
		return agentic.ToolResult{
			CallID:    call.ID,
			Name:      call.Name,
			Error:     "emit_research_artifact invoked without a loop_id; cannot resolve calling loop entity",
			ErrorKind: agentic.ToolErrorInternal,
		}, nil
	}

	artifact, err := parseArgsIntoArtifact(call.Arguments, call.LoopID)
	if err != nil {
		return agentic.ToolResult{
			CallID:    call.ID,
			Name:      call.Name,
			Error:     err.Error(),
			ErrorKind: agentic.ToolErrorInvalidArgs,
		}, nil
	}

	if err := artifact.Validate(); err != nil {
		return agentic.ToolResult{
			CallID:    call.ID,
			Name:      call.Name,
			Error:     fmt.Sprintf("artifact validation failed: %v", err),
			ErrorKind: agentic.ToolErrorInvalidArgs,
		}, nil
	}

	loopEntityID := agentic.LoopExecutionEntityID(e.platform.Org, e.platform.Platform, call.LoopID)
	now := artifact.ProducedAt

	triples := buildTriples(loopEntityID, artifact, now)
	// Short-circuit on the first triple failure: marker triples drive the
	// downstream rule, so without the full set the rule won't fire; the
	// payload would be an orphan. Partial state self-heals on retry —
	// triple predicates are last-wins on the loop entity, so the next
	// successful pass overwrites any half-emitted set.
	for _, triple := range triples {
		if err := e.publisher.AddTriple(ctx, triple); err != nil {
			return agentic.ToolResult{
				CallID:    call.ID,
				Name:      call.Name,
				Error:     fmt.Sprintf("publish %s triple: %v", triple.Predicate, err),
				ErrorKind: agentic.ToolErrorNetwork,
			}, nil
		}
	}

	subject := payloadSubjectPrefix + "." + call.LoopID
	payloadBytes, err := json.Marshal(artifact)
	if err != nil {
		// Marshalling a struct we own and just validated should be
		// infallible; report it as Internal rather than Network because
		// nothing transient about it.
		return agentic.ToolResult{
			CallID:    call.ID,
			Name:      call.Name,
			Error:     fmt.Sprintf("marshal artifact payload: %v", err),
			ErrorKind: agentic.ToolErrorInternal,
		}, nil
	}
	if err := e.natsPublish.Publish(ctx, subject, payloadBytes); err != nil {
		return agentic.ToolResult{
			CallID:    call.ID,
			Name:      call.Name,
			Error:     fmt.Sprintf("publish artifact payload to %s: %v", subject, err),
			ErrorKind: agentic.ToolErrorNetwork,
		}, nil
	}

	mutationCount := artifact.LatestRevisionMutationCount()
	resultMap := map[string]any{
		"loop_id":                      artifact.LoopID,
		"revision":                     artifact.Revision,
		"actors_count":                 len(artifact.Actors),
		"integration_points_count":     len(artifact.IntegrationPoints),
		"tasks_count":                  len(artifact.Tasks),
		"last_revision_mutation_count": mutationCount,
		"payload_subject":              subject,
		"loop_entity_id":               loopEntityID,
	}
	// Mirror the wire-payload + triple discipline: presence is the
	// signal, absence is the catalog-miss case. Avoid leaking an
	// explicit empty string to the next-turn LLM.
	if artifact.TestHarness != "" {
		resultMap["test_harness"] = artifact.TestHarness
	}
	resultJSON, err := json.Marshal(resultMap)
	if err != nil {
		return agentic.ToolResult{
			CallID:    call.ID,
			Name:      call.Name,
			Error:     fmt.Sprintf("marshal tool result: %v", err),
			ErrorKind: agentic.ToolErrorInternal,
		}, nil
	}

	e.logger.Info("emit_research_artifact published",
		slog.String("loop_id", artifact.LoopID),
		slog.Int("revision", artifact.Revision),
		slog.Int("actors", len(artifact.Actors)),
		slog.Int("integration_points", len(artifact.IntegrationPoints)),
		slog.Int("tasks", len(artifact.Tasks)),
		slog.Int("last_revision_mutations", mutationCount),
		slog.String("test_harness", artifact.TestHarness),
		slog.String("subject", subject))

	return agentic.ToolResult{
		CallID:  call.ID,
		Name:    call.Name,
		Content: string(resultJSON),
		Metadata: map[string]any{
			"loop_entity_id":               loopEntityID,
			"revision":                     artifact.Revision,
			"last_revision_mutation_count": mutationCount,
		},
	}, nil
}

// parseArgsIntoArtifact builds a research.Artifact from the LLM-supplied
// tool args. loop_id is taken from the ToolCall (LLMs cannot fake which
// loop they are emitting from); produced_at is server-supplied so audit
// timestamps trace to wall-clock time, not LLM-claimed time.
//
// Round-tripping through JSON keeps the unmarshal logic in one place
// (research.Artifact's existing UnmarshalJSON) and avoids re-implementing
// the field-level type checks the LLM is most likely to fumble.
func parseArgsIntoArtifact(raw map[string]any, loopID string) (*research.Artifact, error) {
	// The LLM should not be specifying loop_id or produced_at; strip them
	// if present so server-supplied values win without surprise.
	clean := make(map[string]any, len(raw))
	for k, v := range raw {
		if k == "loop_id" || k == "produced_at" {
			continue
		}
		clean[k] = v
	}
	clean["loop_id"] = loopID
	clean["produced_at"] = time.Now().UTC()

	bytes, err := json.Marshal(clean)
	if err != nil {
		return nil, fmt.Errorf("marshal tool args: %v", err)
	}
	var artifact research.Artifact
	if err := json.Unmarshal(bytes, &artifact); err != nil {
		return nil, fmt.Errorf("unmarshal tool args into artifact: %v", err)
	}
	return &artifact, nil
}

// buildTriples assembles the deterministic triple set in publish order.
// Counts ride as separate predicates rather than stuffed into one JSON
// blob so rule conditions can match on them without parsing.
//
// Timestamp object is RFC3339Nano for byte-exact correlation with the
// published payload (research.Artifact serialises ProducedAt at nano
// precision via the standard time.MarshalJSON).
//
// Discipline note: triples carry references and counts only — never
// free-text content. Adding actor names, integration-point text, or
// open-gap strings as triple objects violates upstream ADR-028 §Layer 2
// ("rules carry references, not content") and explodes downstream
// small-model context windows when a rule substitutes the predicate
// into a prompt. The full structured content lives in the typed
// payload published on research.artifact.{loop_id}; consumers read it
// via read_loop_result or by subscribing to the subject.
func buildTriples(loopEntityID string, a *research.Artifact, now time.Time) []message.Triple {
	base := func(pred string, obj any) message.Triple {
		return message.Triple{
			Subject:    loopEntityID,
			Predicate:  pred,
			Object:     obj,
			Source:     toolSource,
			Timestamp:  now,
			Confidence: 1.0,
		}
	}
	triples := make([]message.Triple, 0, 9)
	triples = append(triples,
		base(predicateRevision, a.Revision),
		base(predicateActorsCount, len(a.Actors)),
		base(predicateIntegrationPointsCount, len(a.IntegrationPoints)),
		base(predicateTasksCount, len(a.Tasks)),
		base(predicateAddressedGapsCount, len(a.AddressedGaps)),
		base(predicateOpenGapsCount, len(a.OpenGaps)),
		base(predicateLastRevisionMutationCount, a.LatestRevisionMutationCount()),
		base(predicateProducedAt, a.ProducedAt.UTC().Format(time.RFC3339Nano)),
	)
	if a.TestHarness != "" {
		triples = append(triples, base(predicateTestHarness, a.TestHarness))
	}
	return triples
}
