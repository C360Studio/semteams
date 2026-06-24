// Package projectspecplan implements project_spec_tasks, the deterministic
// adapter from an approved OpenSpec change to the plan.* contract consumed by
// the existing dev-via-test Ralph + CBG primitive.
//
// It intentionally does not plan, review, or route. The create_change author
// and reviewer own spec quality; this tool only reprojects the execution-rich
// change.<slug>.task.* facts ADR-057 already requires into the plan.task.*
// shape ADR-044 already runs.
package projectspecplan

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/message"
	agentictools "github.com/c360studio/semstreams/processor/agentic-tools"
	"github.com/c360studio/semstreams/types"

	"github.com/c360studio/semteams/cmd/semteams/runanchor"
)

// ToolName is the LLM-facing tool name.
const ToolName = "project_spec_tasks"

const (
	changePrefix = "change."
	toolSource   = "spec-task-projector"

	runLoopEntityRoleKey = "run-loop-entity-id"

	approvedRunOutcome = "success"

	defaultChainStartGitTag = "plan-start"
	defaultCBGRetryBudget   = 1
	defaultLisaRetryBudget  = 1

	doneAuthorityPolicy    = "approved_openspec_change"
	doneAuthorityFinalGate = "reviewer-dev-via-test"

	taskStatusReady = "ready"
)

// EntityReader reads a graph entity's triples into a predicate→object map.
type EntityReader interface {
	ReadEntity(ctx context.Context, entityID string) (map[string]any, error)
}

// Executor implements agentic.ToolExecutor for project_spec_tasks.
type Executor struct {
	reader    EntityReader
	publisher agentictools.TriplePublisher
	platform  types.PlatformMeta
	now       func() time.Time
	logger    *slog.Logger
}

// NewExecutor constructs an Executor. reader and publisher must be non-nil.
func NewExecutor(reader EntityReader, publisher agentictools.TriplePublisher, platform types.PlatformMeta, logger *slog.Logger) *Executor {
	if reader == nil {
		panic("projectspecplan.NewExecutor: reader must not be nil")
	}
	if publisher == nil {
		panic("projectspecplan.NewExecutor: publisher must not be nil")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Executor{reader: reader, publisher: publisher, platform: platform, now: time.Now, logger: logger}
}

type projectArgs struct {
	// Slug is optional: when empty, the tool projects the single change present
	// on the run entity.
	Slug string `json:"slug"`
}

type projectedTask struct {
	ID              string
	Index           int
	Goal            string
	Assumptions     string
	NonGoals        string
	TargetFiles     string
	DependsOn       string
	TestCommand     string
	ExpectedOutcome string
}

type projection struct {
	Slug                   string
	Goal                   string
	Assumptions            string
	NonGoals               string
	IntegrationTestCommand string
	Tasks                  []projectedTask
}

// Execute reads an approved change from the run entity and stamps plan.* triples
// onto that same entity.
func (e *Executor) Execute(ctx context.Context, call agentic.ToolCall) (agentic.ToolResult, error) {
	if call.Name != ToolName {
		return errResult(call, agentic.ToolErrorNotFound, "unknown tool: %s", call.Name)
	}

	args, err := parseArgs(call.Arguments)
	if err != nil {
		return errResult(call, agentic.ToolErrorInvalidArgs, "%v", err)
	}

	runEntityID := e.runEntityFromCall(call)
	if runEntityID == "" {
		return errResult(call, agentic.ToolErrorInvalidArgs,
			"project_spec_tasks: no run anchor or related_loops[run-loop-entity-id] on the call; this tool projects a reviewed change on the current run entity")
	}

	triples, err := e.reader.ReadEntity(ctx, runEntityID)
	if err != nil {
		return errResult(call, agentic.ToolErrorNetwork, "read run entity %s: %v", runEntityID, err)
	}

	proj, err := buildProjection(triples, args.Slug)
	if err != nil {
		return errResult(call, agentic.ToolErrorInvalidArgs, "%v", err)
	}

	now := e.now().UTC()
	out := proj.triples(runEntityID, now)
	if err := e.publisher.AddTriplesBatch(ctx, out); err != nil {
		return errResult(call, agentic.ToolErrorNetwork, "stamp projected plan triples on %s: %v", runEntityID, err)
	}

	taskIDs := make([]string, len(proj.Tasks))
	for i, t := range proj.Tasks {
		taskIDs[i] = t.ID
	}
	body, _ := json.Marshal(map[string]any{
		"run_entity_id":            runEntityID,
		"slug":                     proj.Slug,
		"task_count":               len(proj.Tasks),
		"task_ids":                 taskIDs,
		"integration_test_command": proj.IntegrationTestCommand,
		"chain_start_git_tag":      defaultChainStartGitTag,
	})

	e.logger.Info("project_spec_tasks stamped",
		slog.String("run_entity_id", runEntityID),
		slog.String("slug", proj.Slug),
		slog.Int("task_count", len(proj.Tasks)),
		slog.String("integration_test_command", proj.IntegrationTestCommand))

	return agentic.ToolResult{
		CallID:  call.ID,
		Name:    call.Name,
		Content: string(body),
		Metadata: map[string]any{
			"run_entity_id": runEntityID,
			"slug":          proj.Slug,
			"task_count":    len(proj.Tasks),
		},
	}, nil
}

func (e *Executor) runEntityFromCall(call agentic.ToolCall) string {
	if _, runEntityID := runanchor.Anchor(call, e.platform.Org, e.platform.Platform); runEntityID != "" {
		return runEntityID
	}
	related, _ := call.Metadata[agentic.MetadataKeyRelatedLoops].(map[string]any)
	runEntityID, _ := related[runLoopEntityRoleKey].(string)
	return strings.TrimSpace(runEntityID)
}

func parseArgs(raw map[string]any) (projectArgs, error) {
	var a projectArgs
	if raw == nil {
		return a, nil
	}
	buf, err := json.Marshal(raw)
	if err != nil {
		return a, fmt.Errorf("marshal arguments: %w", err)
	}
	dec := json.NewDecoder(strings.NewReader(string(buf)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&a); err != nil {
		return a, fmt.Errorf("unmarshal arguments: %w", err)
	}
	a.Slug = strings.TrimSpace(a.Slug)
	return a, nil
}

func buildProjection(triples map[string]any, wantSlug string) (projection, error) {
	outcome := strings.TrimSpace(stringify(triples["agent.run.outcome"]))
	if outcome != approvedRunOutcome {
		return projection{}, fmt.Errorf("project_spec_tasks: run is not approved (agent.run.outcome=%q); the create_change reviewer must approve before implementation", outcome)
	}

	slug, err := resolveSlug(triples, wantSlug)
	if err != nil {
		return projection{}, err
	}

	base := changePrefix + slug + "."
	integration, err := requiredString(triples, base+"acceptance_command")
	if err != nil {
		return projection{}, err
	}
	goal := strings.TrimSpace(stringify(triples[base+"proposal.intent"]))
	if goal == "" {
		goal = "Implement OpenSpec change " + slug
	}
	nonGoals, err := arrayJSONOrDefault(triples[base+"proposal.scope_out"], base+"proposal.scope_out", 0)
	if err != nil {
		return projection{}, err
	}

	indices, err := taskIndices(triples, base+"task.")
	if err != nil {
		return projection{}, err
	}
	tasks := make([]projectedTask, 0, len(indices))
	for _, idx := range indices {
		taskBase := base + "task." + strconv.Itoa(idx) + "."
		task, err := projectTask(triples, taskBase, idx)
		if err != nil {
			return projection{}, err
		}
		tasks = append(tasks, task)
	}

	return projection{
		Slug:                   slug,
		Goal:                   goal,
		Assumptions:            "[]",
		NonGoals:               nonGoals,
		IntegrationTestCommand: integration,
		Tasks:                  tasks,
	}, nil
}

func projectTask(triples map[string]any, taskBase string, idx int) (projectedTask, error) {
	goal, err := requiredString(triples, taskBase+"goal")
	if err != nil {
		return projectedTask{}, err
	}
	targetFiles, err := requiredArrayJSON(triples, taskBase+"target_files", 1)
	if err != nil {
		return projectedTask{}, err
	}
	testCommand, err := requiredString(triples, taskBase+"test_command")
	if err != nil {
		return projectedTask{}, err
	}
	assumptions, err := requiredArrayJSON(triples, taskBase+"assumptions", 0)
	if err != nil {
		return projectedTask{}, err
	}
	nonGoals, err := requiredArrayJSON(triples, taskBase+"non_goals", 0)
	if err != nil {
		return projectedTask{}, err
	}
	dependsOn, err := arrayJSONOrDefault(triples[taskBase+"depends_on"], taskBase+"depends_on", 0)
	if err != nil {
		return projectedTask{}, err
	}

	return projectedTask{
		ID:              fmt.Sprintf("task-%d", idx),
		Index:           idx,
		Goal:            goal,
		Assumptions:     assumptions,
		NonGoals:        nonGoals,
		TargetFiles:     targetFiles,
		DependsOn:       dependsOn,
		TestCommand:     testCommand,
		ExpectedOutcome: strings.TrimSpace(stringify(triples[taskBase+"expected_outcome"])),
	}, nil
}

func (p projection) triples(runEntityID string, now time.Time) []message.Triple {
	mk := func(pred string, obj any) message.Triple {
		return message.Triple{
			Subject:    runEntityID,
			Predicate:  pred,
			Object:     obj,
			Source:     toolSource,
			Timestamp:  now,
			Confidence: 1.0,
		}
	}

	out := []message.Triple{
		mk("plan.goal", p.Goal),
		mk("plan.assumptions", p.Assumptions),
		mk("plan.non_goals", p.NonGoals),
		mk("plan.integration_test_command", p.IntegrationTestCommand),
		mk("plan.chain_start_git_tag", defaultChainStartGitTag),
		mk("plan.task_count", len(p.Tasks)),
		mk("plan.revision", 1),
		mk("plan.cbg_retry_budget", defaultCBGRetryBudget),
		mk("plan.lisa_retry_budget", defaultLisaRetryBudget),
		mk("plan.generated_at", now.Format(time.RFC3339Nano)),
		mk("plan.source_change_slug", p.Slug),
		mk("plan.projected_at", now.Format(time.RFC3339Nano)),
		mk("plan.done_authority.policy", doneAuthorityPolicy),
		mk("plan.done_authority.source_change", changePrefix+p.Slug),
		mk("plan.done_authority.final_gate", doneAuthorityFinalGate),
		mk("dev_via_test.run.status", "active"),
	}

	for _, t := range p.Tasks {
		prefix := "plan.task." + t.ID + "."
		out = append(out,
			mk(prefix+"goal", t.Goal),
			mk(prefix+"assumptions", t.Assumptions),
			mk(prefix+"non_goals", t.NonGoals),
			mk(prefix+"target_files", t.TargetFiles),
			mk(prefix+"depends_on", t.DependsOn),
			mk(prefix+"test_command", t.TestCommand),
			mk(prefix+"status", taskStatusReady),
			mk(prefix+"position", t.Index),
			mk(prefix+"source_change_task", changePrefix+p.Slug+".task."+strconv.Itoa(t.Index)),
		)
		if t.ExpectedOutcome != "" {
			out = append(out, mk(prefix+"expected_outcome", t.ExpectedOutcome))
		}
	}
	return out
}

func resolveSlug(triples map[string]any, want string) (string, error) {
	present := changeSlugs(triples)
	if want != "" {
		if !present[want] {
			return "", fmt.Errorf("project_spec_tasks: no change %q on the run entity (present: %s)", want, slugList(present))
		}
		return want, nil
	}
	switch len(present) {
	case 0:
		return "", fmt.Errorf("project_spec_tasks: no change.* triples on the run entity — nothing to project")
	case 1:
		return slugList(present), nil
	default:
		return "", fmt.Errorf("project_spec_tasks: %d changes on the run entity (%s) — pass slug to pick one", len(present), slugList(present))
	}
}

func changeSlugs(triples map[string]any) map[string]bool {
	out := map[string]bool{}
	for pred := range triples {
		rest, ok := strings.CutPrefix(pred, changePrefix)
		if !ok {
			continue
		}
		slug, _, ok := strings.Cut(rest, ".")
		if ok && slug != "" {
			out[slug] = true
		}
	}
	return out
}

func slugList(set map[string]bool) string {
	xs := make([]string, 0, len(set))
	for s := range set {
		xs = append(xs, s)
	}
	sort.Strings(xs)
	return strings.Join(xs, ", ")
}

func taskIndices(triples map[string]any, taskPrefix string) ([]int, error) {
	seen := map[int]bool{}
	for pred := range triples {
		if !strings.HasPrefix(pred, taskPrefix) {
			continue
		}
		rest := strings.TrimPrefix(pred, taskPrefix)
		raw, _, ok := strings.Cut(rest, ".")
		if !ok {
			continue
		}
		idx, err := strconv.Atoi(raw)
		if err != nil {
			continue
		}
		seen[idx] = true
	}
	if len(seen) == 0 {
		return nil, fmt.Errorf("project_spec_tasks: change has no task facts")
	}
	out := make([]int, 0, len(seen))
	for idx := range seen {
		out = append(out, idx)
	}
	sort.Ints(out)
	for want, got := range out {
		if got != want {
			return nil, fmt.Errorf("project_spec_tasks: change task indices must be contiguous from 0 (got %v)", out)
		}
	}
	return out, nil
}

func requiredString(triples map[string]any, pred string) (string, error) {
	got := strings.TrimSpace(stringify(triples[pred]))
	if got == "" {
		return "", fmt.Errorf("project_spec_tasks: required %s is missing or empty", pred)
	}
	return got, nil
}

func requiredArrayJSON(triples map[string]any, pred string, minLen int) (string, error) {
	val, ok := triples[pred]
	if !ok {
		return "", fmt.Errorf("project_spec_tasks: required %s is missing", pred)
	}
	return arrayJSON(val, pred, minLen)
}

func arrayJSONOrDefault(val any, pred string, minLen int) (string, error) {
	if val == nil || stringify(val) == "" {
		return "[]", nil
	}
	return arrayJSON(val, pred, minLen)
}

func arrayJSON(val any, pred string, minLen int) (string, error) {
	switch v := val.(type) {
	case string:
		var out []string
		if err := json.Unmarshal([]byte(v), &out); err != nil {
			return "", fmt.Errorf("project_spec_tasks: %s must be a JSON string array: %w", pred, err)
		}
		if len(out) < minLen {
			return "", fmt.Errorf("project_spec_tasks: %s must contain at least %d item(s)", pred, minLen)
		}
		return jsonStringArray(out), nil
	case []string:
		if len(v) < minLen {
			return "", fmt.Errorf("project_spec_tasks: %s must contain at least %d item(s)", pred, minLen)
		}
		return jsonStringArray(v), nil
	case []any:
		out := make([]string, 0, len(v))
		for i, item := range v {
			s, ok := item.(string)
			if !ok {
				return "", fmt.Errorf("project_spec_tasks: %s[%d] must be string", pred, i)
			}
			out = append(out, s)
		}
		if len(out) < minLen {
			return "", fmt.Errorf("project_spec_tasks: %s must contain at least %d item(s)", pred, minLen)
		}
		return jsonStringArray(out), nil
	default:
		return "", fmt.Errorf("project_spec_tasks: %s must be a JSON string array (got %T)", pred, val)
	}
}

func jsonStringArray(v []string) string {
	if v == nil {
		v = []string{}
	}
	buf, _ := json.Marshal(v)
	return string(buf)
}

func stringify(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case nil:
		return ""
	default:
		buf, _ := json.Marshal(x)
		return string(buf)
	}
}

func errResult(call agentic.ToolCall, kind agentic.ToolErrorKind, format string, args ...any) (agentic.ToolResult, error) {
	return agentic.ToolResult{
		CallID:    call.ID,
		Name:      call.Name,
		Error:     fmt.Sprintf(format, args...),
		ErrorKind: kind,
	}, nil
}
