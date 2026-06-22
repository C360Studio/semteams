// Package emitchange implements the SemTeams-local emit_change tool — the
// producer side of the create_change journey's author role (ADR-057 §D5 +
// configs/rules/create-change/). It is the change.* analogue of
// emit_dev_via_test_plan: the author parses a prompt (plus ingested living-spec
// context) into a structured OpenSpec change and calls this tool to stamp
// change.<slug>.* triples on the RUN ENTITY for the reviewer gate + downstream
// hand-off.
//
// The schema encodes ADR-057's discipline structurally
// ([[encode-principles-structurally]]) — the validator rejects a payload that
// does not carry, per requirement, an RFC-2119 SHALL-class statement and ≥1
// Given/When/Then scenario with at least a WHEN and a THEN (§D3); and, per task,
// the §D6 execution-rich superset (goal + target_files≥1 + test_command +
// assumptions + non_goals) so the P4 dev-from-task seam is a reprojection, not a
// transform. Persona prose cannot enforce this; the schema can.
//
// Reuse boundary (ADR-057 slice-5 addendum): the markdown-representable content
// (proposal / design / delta / thin task fields) is mapped to facts by the pure
// openspec.Change.Facts(); this tool ADDS the writer-only graph-state the model
// does not carry — change.<slug>.status, change.<slug>.acceptance_command, and
// the per-task §D6 execution-rich fields — keyed on the same task index
// openspec uses, so a consumer reads one coherent change.<slug>.* set.
//
// Framework-alignment review: ADR-057 §Framework-alignment (the survey +
// alternatives-ruled-out + migration target — the same generic write_artifact
// upstream is sketching, ADR-028 §What's not built here, verified absent in
// semstreams beta.114). See cmd/semteams/tools/README.md.
//
// Arrays (scope, scenarios, task field lists) are stamped as JSON-encoded
// strings in single triple objects, the discipline ADR-057 §D1 +
// emit_dev_via_test_plan established (the rule engine substitutes triple objects
// but cannot iterate indexed predicates).
package emitchange

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/message"
	agentictools "github.com/c360studio/semstreams/processor/agentic-tools"
	"github.com/c360studio/semstreams/types"

	"github.com/c360studio/semteams/cmd/semteams/openspec"
)

// ToolName is the LLM-facing tool name.
const ToolName = "emit_change"

// toolSource tags the triples this tool publishes.
const toolSource = "create-change-emit"

// runLoopIDRoleKey is the related_loops key carrying the run (coordinator) LOOP
// id; the run entity id (agent.chain.execution.<runID>) is derived from it via
// the framework's canonical chain-execution entity-id function — symmetric with
// emit_dev_via_test_plan / the autoresearch emit tools (ADR-053 Phase 3b). The
// create-change spawn rule pins it at chain start.
const runLoopIDRoleKey = "create-change-run"

// Writer-only predicates (ADR-057 §D1/§D6) — the graph-state the OpenSpec model
// does not carry, so they live here rather than in openspec.Change.Facts.
const (
	predicateChangeStatus      = "status"             // change.<slug>.status
	predicateAcceptanceCmd     = "acceptance_command" // change.<slug>.acceptance_command
	predicateChangeGeneratedAt = "generated_at"       // change.<slug>.generated_at
	statusDraft                = "draft"              // create_change emits a draft; the reviewer gate promotes it

	// Per-task §D6 execution-rich fields, keyed change.<slug>.task.<i>.<field>
	// at the SAME index openspec.Change.Facts uses for the thin fields.
	taskFieldGoal            = "goal"
	taskFieldTargetFiles     = "target_files"
	taskFieldTestCommand     = "test_command"
	taskFieldAssumptions     = "assumptions"
	taskFieldNonGoals        = "non_goals"
	taskFieldExpectedOutcome = "expected_outcome"
	taskFieldRequirementRef  = "requirement_ref"
)

// slugPattern restricts change slugs to lower-kebab-case (OpenSpec change-folder
// convention), 1-64 chars, so the slug is a safe predicate fragment + folder
// name.
var slugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)

// shallPattern matches an RFC-2119 keyword as a whole word (OpenSpec writes them
// uppercase: "The system SHALL ..."). §D3 requires every added/modified
// requirement to carry one.
var shallPattern = regexp.MustCompile(`\b(SHALL|MUST|SHOULD|MAY)\b`)

// Executor implements agentic.ToolExecutor for emit_change.
type Executor struct {
	publisher agentictools.TriplePublisher
	platform  types.PlatformMeta
	logger    *slog.Logger
}

// NewExecutor constructs an Executor. publisher must be non-nil; platform is
// used to derive the run entity id from the run loop id.
func NewExecutor(publisher agentictools.TriplePublisher, platform types.PlatformMeta, logger *slog.Logger) *Executor {
	if publisher == nil {
		panic("emitchange.NewExecutor: publisher must not be nil")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Executor{publisher: publisher, platform: platform, logger: logger}
}

// Execute parses args, validates the ADR-057 §D3/§D6 discipline, and stamps
// change.<slug>.* triples on the run entity.
func (e *Executor) Execute(ctx context.Context, call agentic.ToolCall) (agentic.ToolResult, error) {
	if call.Name != ToolName {
		return errResult(call, agentic.ToolErrorNotFound, "unknown tool: %s", call.Name)
	}

	p, err := parseArgs(call.Arguments)
	if err != nil {
		return errResult(call, agentic.ToolErrorInvalidArgs, "%v", err)
	}

	runEntityID, err := runEntityFromCall(call, e.platform)
	if err != nil {
		return errResult(call, agentic.ToolErrorInternal, "%v", err)
	}

	now := time.Now().UTC()
	triples := p.triples(runEntityID, now)

	if err := e.publisher.AddTriplesBatch(ctx, triples); err != nil {
		return errResult(call, agentic.ToolErrorNetwork, "stamp change triples on %s: %v", runEntityID, err)
	}

	body, _ := json.Marshal(map[string]any{
		"run_entity_id": runEntityID,
		"slug":          p.Slug,
		"status":        statusDraft,
		"delta_count":   len(p.Deltas),
		"task_count":    len(p.Tasks),
	})
	e.logger.Info("emit_change stamped",
		slog.String("run_entity_id", runEntityID),
		slog.String("slug", p.Slug),
		slog.Int("delta_count", len(p.Deltas)),
		slog.Int("task_count", len(p.Tasks)))

	return agentic.ToolResult{
		CallID:   call.ID,
		Name:     call.Name,
		Content:  string(body),
		Metadata: map[string]any{"run_entity_id": runEntityID, "slug": p.Slug},
	}, nil
}

func runEntityFromCall(call agentic.ToolCall, platform types.PlatformMeta) (string, error) {
	related, _ := call.Metadata[agentic.MetadataKeyRelatedLoops].(map[string]any)
	runLoopID, ok := related[runLoopIDRoleKey].(string)
	if !ok || runLoopID == "" {
		return "", fmt.Errorf("emit_change: related_loops[%q] missing or empty in call metadata; the create-change spawn rule must pin the run loop id at chain start", runLoopIDRoleKey)
	}
	runEntityID, err := agentic.TryChainExecutionEntityID(platform.Org, platform.Platform, runLoopID)
	if err != nil {
		return "", fmt.Errorf("emit_change: build run entity id from run loop %q: %w", runLoopID, err)
	}
	return runEntityID, nil
}

// triples assembles the change.<slug>.* batch: the markdown-representable
// content via the pure openspec.Change.Facts(), plus the writer-only graph-state
// this tool owns. The content task index and the writer task index are the same
// (both flatten p.toChange().Tasks in order), so a consumer reads one coherent
// per-task set.
func (p *changePayload) triples(runEntityID string, now time.Time) []message.Triple {
	change := p.toChange()
	base := openspec.ChangeEntityPrefix(p.Slug) // "change.<slug>."

	out := make([]message.Triple, 0, 16+len(p.Tasks)*8)
	mk := func(pred string, obj string) message.Triple {
		return message.Triple{Subject: runEntityID, Predicate: pred, Object: obj, Source: toolSource, Timestamp: now, Confidence: 1.0}
	}

	// Content half — the pure, markdown-representable mapping (slice 5).
	for _, f := range change.Facts() {
		out = append(out, mk(f.Predicate, f.Object))
	}

	// Writer half — graph-state the model does not carry.
	out = append(out,
		mk(base+predicateChangeStatus, statusDraft),
		mk(base+predicateChangeGeneratedAt, now.Format(time.RFC3339Nano)),
	)
	if p.AcceptanceCommand != "" {
		out = append(out, mk(base+predicateAcceptanceCmd, p.AcceptanceCommand))
	}

	taskBase := base + "task."
	for i, t := range p.Tasks {
		prefix := taskBase + strconv.Itoa(i) + "."
		out = append(out,
			mk(prefix+taskFieldGoal, t.Goal),
			mk(prefix+taskFieldTargetFiles, jsonArray(t.TargetFiles)),
			mk(prefix+taskFieldTestCommand, t.TestCommand),
			mk(prefix+taskFieldAssumptions, jsonArray(t.Assumptions)),
			mk(prefix+taskFieldNonGoals, jsonArray(t.NonGoals)),
		)
		if t.ExpectedOutcome != "" {
			out = append(out, mk(prefix+taskFieldExpectedOutcome, t.ExpectedOutcome))
		}
		if t.RequirementRef != "" {
			out = append(out, mk(prefix+taskFieldRequirementRef, t.RequirementRef))
		}
	}
	return out
}

// toChange projects the payload onto the pure openspec.Change model (the
// markdown-representable content). Tasks are grouped into sections by contiguous
// runs of the same section name. groupTaskSections is order-preserving, so the
// flatten order equals the payload order — that ALONE is what keeps the
// writer-half per-task fields (executor stamps by p.Tasks position) index-aligned
// with the content half (openspec flattens c.Tasks in the same order). The
// separate section-contiguity check in validateTasks is for FAITHFUL section
// reconstruction on a graph round-trip (tasksFromIndex re-groups by section
// first-appearance), not for index alignment — alignment holds even for
// interleaved input.
func (p *changePayload) toChange() *openspec.Change {
	c := &openspec.Change{Slug: p.Slug}
	if p.Proposal != nil {
		c.Proposal = &openspec.Proposal{
			Intent:   p.Proposal.Intent,
			ScopeIn:  p.Proposal.ScopeIn,
			ScopeOut: p.Proposal.ScopeOut,
			Approach: p.Proposal.Approach,
		}
	}
	if p.Design != nil {
		c.Design = p.Design.toModel()
	}
	for _, d := range p.Deltas {
		c.Deltas = append(c.Deltas, d.toModel())
	}
	if len(p.Tasks) > 0 {
		c.Tasks = &openspec.Tasks{Sections: groupTaskSections(p.Tasks)}
	}
	return c
}

// groupTaskSections turns the flat payload task list into contiguous-run
// sections, preserving order so the flatten index equals the payload index.
func groupTaskSections(tasks []taskPayload) []openspec.TaskSection {
	var sections []openspec.TaskSection
	for _, t := range tasks {
		if len(sections) == 0 || sections[len(sections)-1].Name != t.Section {
			sections = append(sections, openspec.TaskSection{Name: t.Section})
		}
		s := &sections[len(sections)-1]
		s.Tasks = append(s.Tasks, openspec.Task{Number: t.Number, Text: t.Text, Done: t.Done})
	}
	return sections
}

func errResult(call agentic.ToolCall, kind agentic.ToolErrorKind, format string, args ...any) (agentic.ToolResult, error) {
	return agentic.ToolResult{
		CallID:    call.ID,
		Name:      call.Name,
		Error:     fmt.Sprintf(format, args...),
		ErrorKind: kind,
	}, nil
}

func jsonArray(ss []string) string {
	if ss == nil {
		ss = []string{}
	}
	b, _ := json.Marshal(ss)
	return string(b)
}

// trim is a spelling-shorthand used throughout validation.
func trim(s string) string { return strings.TrimSpace(s) }
