// Package emitdevviatestplan implements the SemTeams-local
// emit_dev_via_test_plan tool — the producer side of the Lisa
// (dev-via-test-plan) persona's terminal commit (see ADR-044 +
// configs/rules/dev-via-test/01-coordinator-dev-via-test-spawn.json).
//
// Lisa parses the user's prose ask into a Karpathy-shaped plan
// (goal + assumptions + non_goals + tasks) and calls this tool to
// stamp plan.* triples on the RUN ENTITY (agent.chain.execution.<runID>,
// whose id is derived from the run loop id in
// related_loops["dev-via-test-run"] via TryChainExecutionEntityID —
// ADR-053 Phase 3b, the same derivation run_scope=new uses to mint and
// symmetric with the autoresearch emit tools). The triples are the
// load-bearing artifact: Ralph reads per-task spec via lineage,
// the coordinator walks plan.task.<id>.status to pick the next
// ready task, and CBG reads plan.integration_test_command for
// the chain-end gate.
//
// The schema literally encodes Karpathy's four guidelines as
// required fields:
//
//	Rule 1 (think before coding)   → plan.assumptions[] required (may be empty, must be present)
//	Rule 2 (simplicity first)      → plan.non_goals[] required (may be empty)
//	Rule 3 (surgical changes)      → task.target_files[] required (≥1)
//	Rule 4 (goal-driven execution) → task.test_command required
//
// Per [[encode-principles-structurally]]: discipline lives in the
// schema, not persona prose. Lisa CANNOT emit without surfacing
// assumptions / non-goals / target files / test commands.
//
// Discipline note (framework-alignment review): see ADR-044
// §addendum 2026-06-03. No upstream equivalent — the
// plan-as-triples + Karpathy-required-schema pattern is
// SemTeams policy; the migration target is the same generic
// write_artifact upstream is sketching (ADR-028 §What's not built
// here). When upstream lands it, evaluate migration of all
// emit_*_artifact / emit_plan / emit_*_baseline / emit_dev_via_test_plan
// tools to the generic primitive — same posture as emitplan,
// emitautoresearchbaseline.
//
// Arrays (assumptions, non_goals, target_files, depends_on) are
// stamped as JSON-encoded strings in triple objects. The rule
// engine's `$entity.triple.X` substitution interpolates the value
// verbatim; downstream personas parse the JSON string back into
// an array. This avoids any per-element triple proliferation and
// keeps the substitution shape predictable.
package emitdevviatestplan

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/message"
	agentictools "github.com/c360studio/semstreams/processor/agentic-tools"
	"github.com/c360studio/semstreams/types"
)

// ToolName is the LLM-facing tool name.
const ToolName = "emit_dev_via_test_plan"

// toolSource tags the triples this tool publishes.
const toolSource = "dev-via-test-emit-plan"

// runLoopIDRoleKey is the related_loops key carrying the run (coordinator)
// LOOP id. The run entity id (agent.chain.execution.<runID>) is derived from
// it via the framework's canonical chain-execution entity-id function — the
// same derivation run_scope=new uses to mint, and symmetric with the
// autoresearch emit tools (ADR-053 Phase 3b). We anchor on the run loop id
// rather than a pre-computed entity id because rule 01 cannot thread the run
// entity id (it is minted by the same publish_agent action, so not yet visible
// to a sibling substitution); dev-via-test-run = $entity.instance is stable.
const runLoopIDRoleKey = "dev-via-test-run"

// Predicate constants stamped on the run (coordinator) entity.
// Plan-level. Per-task predicates are derived in triples() since
// they're keyed by task ID.
const (
	predicatePlanGoal               = "plan.goal"
	predicatePlanAssumptions        = "plan.assumptions"
	predicatePlanNonGoals           = "plan.non_goals"
	predicatePlanIntegrationTestCmd = "plan.integration_test_command"
	predicatePlanChainStartGitTag   = "plan.chain_start_git_tag"
	predicatePlanTaskCount          = "plan.task_count"
	predicatePlanGeneratedAt        = "plan.generated_at"
	predicatePlanRevision           = "plan.revision"
	predicatePlanCBGRetryBudget     = "plan.cbg_retry_budget"
	predicatePlanLisaRetryBudget    = "plan.lisa_retry_budget"

	// ADR-053 Phase 3b: the run-lifecycle marker moves here from rule 01's
	// add_triple (which seeded the coordinator loop). emit_plan now seeds it
	// on the run entity alongside plan.*. Currently unread (no rule/persona
	// consumes it), kept on the run entity for consistency with autoresearch +
	// forward-compat with Phase 4 lifecycle rules.
	predicateRunStatus = "dev_via_test.run.status"

	// Per-task predicate prefix. Full keys look like
	// plan.task.<id>.{goal,assumptions,non_goals,target_files,
	// test_command,expected_outcome,depends_on,status,position}.
	predicatePlanTaskPrefix = "plan.task."

	predicateTaskGoal            = "goal"
	predicateTaskAssumptions     = "assumptions"
	predicateTaskNonGoals        = "non_goals"
	predicateTaskTargetFiles     = "target_files"
	predicateTaskDependsOn       = "depends_on"
	predicateTaskTestCommand     = "test_command"
	predicateTaskExpectedOutcome = "expected_outcome"
	predicateTaskStatus          = "status"
	predicateTaskPosition        = "position"

	// taskStatusReady is the initial value stamped on every task
	// at plan emit. Coordinator (Slice 3) mutates this via
	// update_triple to in_progress / done / blocked.
	taskStatusReady = "ready"

	// defaultChainStartGitTag is the sentinel name CBG diffs against
	// at chain-end. Lisa's persona is responsible for actually
	// running `bash git tag <value>` before calling this tool (so
	// the tag exists on disk when CBG looks for it). Default value
	// stays a constant here; if a deployment ever needs a chain-id-
	// scoped tag we add a tool arg.
	defaultChainStartGitTag = "plan-start"

	// CBG dev-fixable retry budget (ADR-044 §addendum Slice 5). The
	// budget is plan-data so it is visible + tunable per run, but it
	// is CLAMPED here to [1, maxCBGRetryBudget] — the clamp is the
	// structural retry ceiling. Rule 07d's When clause gates
	// re-dispatch on `$state.iteration <= plan.cbg_retry_budget`;
	// clamping the source value guarantees the escalate branch
	// (`$state.iteration > budget`) always triggers within the
	// ceiling even if a plan over-specifies the budget. Absent / 0 →
	// defaultCBGRetryBudget (one auto-fix pass, then human).
	defaultCBGRetryBudget = 1
	maxCBGRetryBudget     = 5

	// Lisa (plan) re-plan budget (ADR-044 §addendum Slice 6). How many
	// times the plan-review gate (CBG in plan_review mode) may bounce
	// Lisa's plan back for a fidelity fix before escalating to the
	// user. Tuned independently from cbg_retry_budget — plan-retries
	// and work-retries have different cost/value. Same clamp posture:
	// the [1, maxLisaRetryBudget] clamp is the structural ceiling rule
	// 02d's escalate branch relies on.
	defaultLisaRetryBudget = 1
	maxLisaRetryBudget     = 5
)

// taskIDPattern restricts task IDs to lowercase alphanumeric +
// hyphens, 1-32 chars. Conservative: tighter than "any string"
// avoids triple-key shape surprises if the rule engine substitutes
// task IDs into prompts or predicate fragments.
var taskIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)

// Executor implements agentic.ToolExecutor for
// emit_dev_via_test_plan.
type Executor struct {
	publisher agentictools.TriplePublisher
	// remover clears the prior plan on a re-plan (revision > 1) so the
	// emit is an UPSERT, not an append (ADR-044 §addendum Slice 6).
	// May be nil — first-emit (revision == 1) never clears, so a nil
	// remover is fine for deployments that don't re-plan, but a
	// re-plan with a nil remover surfaces a clear error.
	remover  tripleRemover
	platform types.PlatformMeta
	logger   *slog.Logger
}

// NewExecutor constructs an Executor. Publisher must be non-nil. platform is
// used to derive the run entity id from the run loop id (ADR-053 Phase 3b).
// remover may be nil (no plan-review/re-plan path wired); a re-plan
// emit then errors rather than silently appending a stale-winning
// duplicate plan.
func NewExecutor(publisher agentictools.TriplePublisher, remover tripleRemover, platform types.PlatformMeta, logger *slog.Logger) *Executor {
	if publisher == nil {
		panic("emitdevviatestplan.NewExecutor: publisher must not be nil")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Executor{publisher: publisher, remover: remover, platform: platform, logger: logger}
}

// ListTools returns the LLM-facing schema. Per Karpathy /
// [[encode-principles-structurally]], assumptions + non_goals are
// REQUIRED arrays at the plan level (may be empty; must be
// present), and target_files + test_command are REQUIRED at the
// task level.
func (e *Executor) ListTools() []agentic.ToolDefinition {
	stringArray := func(desc string) map[string]any {
		return map[string]any{
			"type":        "array",
			"items":       map[string]any{"type": "string"},
			"description": desc,
		}
	}
	taskSchema := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"id":               map[string]any{"type": "string", "pattern": `^[a-z0-9][a-z0-9-]{0,31}$`, "description": "Stable per-task identifier (lowercase alphanumeric + hyphens, 1-32 chars). Used as the plan.task.<id>.* triple key and as Ralph's spawn-time target."},
			"goal":             map[string]any{"type": "string", "description": "Task-level goal in user's own words. Concrete and verifiable."},
			"assumptions":      stringArray("Task-local assumptions (Karpathy Rule 1). May be empty; emit [] explicitly if no task-local assumptions beyond plan-level."),
			"non_goals":        stringArray("Task-local anti-scope (Karpathy Rule 2). May be empty; emit [] explicitly."),
			"target_files":     stringArray("File globs Ralph may modify (Karpathy Rule 3 — surgical changes). REQUIRED ≥1. Empty means 'no scope' which is invalid; for genuinely cross-cutting work, pick the narrowest accurate set."),
			"depends_on":       stringArray("Task IDs that must complete before this one is ready. v1 coordinator is linear so this is ignored; v2 will topo-walk. Emit [] for the first task."),
			"test_command":     map[string]any{"type": "string", "description": "Karpathy Rule 4 — the executable acceptance command. Single shell command. Ralph iterates until this exits 0."},
			"expected_outcome": map[string]any{"type": "string", "description": "Human-readable 'done looks like' description. Used by CBG's diff review and operator-facing logging."},
		},
		"required": []string{"id", "goal", "assumptions", "non_goals", "target_files", "test_command"},
	}
	return []agentic.ToolDefinition{{
		Name:        ToolName,
		Description: "Emit the dev-via-test plan. Stamps plan.* triples on the run entity (resolved for you from the run loop id). Per ADR-044 + [[encode-principles-structurally]]: required fields encode Karpathy's four guidelines structurally — the schema rejects payloads missing assumptions, non_goals, target_files, or test_command. Call exactly once per dev-via-test arc, before the terminal decide(action='planned').",
		Parameters: map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"goal":                     map[string]any{"type": "string", "description": "Chain-level goal in user's own words. One sentence. Concrete and verifiable."},
				"assumptions":              stringArray("Plan-level assumptions (Karpathy Rule 1). Surface what Lisa is taking for granted about the environment, deps, semantics. May be empty; emit [] explicitly if no plan-level assumptions."),
				"non_goals":                stringArray("Plan-level anti-scope (Karpathy Rule 2). What this work explicitly excludes. May be empty; emit [] explicitly."),
				"integration_test_command": map[string]any{"type": "string", "description": "CBG's chain-end full acceptance gate. Runs once at chain end across all task scope. Must be a single shell command."},
				"revision":                 map[string]any{"type": "integer", "minimum": 1, "description": "Monotonic revision number, starting at 1. Bump on re-plan after coordinator amendment (Slice 3+ coordinator). Required so absent vs explicit-zero never silently coerces to 1."},
				"cbg_retry_budget":         map[string]any{"type": "integer", "minimum": 1, "maximum": maxCBGRetryBudget, "description": "OPTIONAL (default 1). How many times the chain-end reviewer (CBG) may bounce a task back for a bounded dev-fix before escalating to the user. Per ADR-044 §Slice 5. Clamped to [1,5] server-side — the clamp is the structural retry ceiling. Set 1 for one auto-fix pass then human (recommended); raise only if the work has a high chance of CBG catching a mechanically-fixable miss the per-task tests can't see."},
				"lisa_retry_budget":        map[string]any{"type": "integer", "minimum": 1, "maximum": maxLisaRetryBudget, "description": "OPTIONAL (default 1). How many times the plan-review gate (CBG in plan_review mode) may bounce THIS plan back to the planner for a fidelity fix before escalating to the user. Per ADR-044 §Slice 6. Clamped to [1,5] server-side. Independent of cbg_retry_budget (plan-retries vs work-retries). Set 1 for one re-plan pass then human."},
				"tasks": map[string]any{
					"type":        "array",
					"minItems":    1,
					"items":       taskSchema,
					"description": "Ordered task list. v1 coordinator walks in order; v2 will respect depends_on. Each task is decomposable enough for one Ralph inner loop to converge.",
				},
			},
			"required": []string{"goal", "assumptions", "non_goals", "integration_test_command", "revision", "tasks"},
		},
	}}
}

// Execute parses args, validates, stamps triples on the run entity.
func (e *Executor) Execute(ctx context.Context, call agentic.ToolCall) (agentic.ToolResult, error) {
	if call.Name != ToolName {
		return errResult(call, agentic.ToolErrorNotFound, "unknown tool: %s", call.Name)
	}

	plan, err := parseArgs(call.Arguments)
	if err != nil {
		return errResult(call, agentic.ToolErrorInvalidArgs, "%v", err)
	}

	runEntityID, err := runEntityFromCall(call, e.platform)
	if err != nil {
		return errResult(call, agentic.ToolErrorInternal, "%v", err)
	}

	now := time.Now().UTC()
	triples, err := plan.triples(runEntityID, now)
	if err != nil {
		return errResult(call, agentic.ToolErrorInternal, "build triples: %v", err)
	}

	// Re-plan UPSERT (ADR-044 §addendum Slice 6): on revision > 1 this
	// is a plan-review re-plan, so the prior plan on the run entity
	// must be REPLACED, not appended to (triples on the same
	// (subject, predicate) → first-match returns the stale one). Clear
	// the plan namespace before stamping. The clear is complete iff
	// the re-plan reuses the prior task IDs (amend-in-place, per Lisa's
	// re-plan contract) — a restructured re-plan that drops/renames a
	// task would orphan its triples. The removes are awaited
	// (request-reply) before the adds so graph-ingest can't reorder a
	// stale clear after the fresh stamp.
	if plan.Revision > 1 {
		if e.remover == nil {
			return errResult(call, agentic.ToolErrorInternal, "re-plan (revision %d > 1) requires a triple remover to upsert, but none is wired; cannot replace the prior plan without leaving stale triples", plan.Revision)
		}
		if err := e.clearPriorPlan(ctx, runEntityID, plan.taskIDs()); err != nil {
			return errResult(call, agentic.ToolErrorNetwork, "clear prior plan on %s for re-plan: %v", runEntityID, err)
		}
	}

	if err := e.publisher.AddTriplesBatch(ctx, triples); err != nil {
		return errResult(call, agentic.ToolErrorNetwork, "stamp plan triples on %s: %v", runEntityID, err)
	}

	body, _ := json.Marshal(map[string]any{
		"run_entity_id":            runEntityID,
		"task_count":               len(plan.Tasks),
		"chain_start_git_tag":      defaultChainStartGitTag,
		"integration_test_command": plan.IntegrationTestCommand,
		"revision":                 plan.Revision,
		"cbg_retry_budget":         plan.CBGRetryBudget,
	})

	e.logger.Info("emit_dev_via_test_plan stamped",
		slog.String("run_entity_id", runEntityID),
		slog.Int("task_count", len(plan.Tasks)),
		slog.Int("revision", plan.Revision),
		slog.String("integration_test_command", plan.IntegrationTestCommand))

	return agentic.ToolResult{
		CallID:   call.ID,
		Name:     call.Name,
		Content:  string(body),
		Metadata: map[string]any{"run_entity_id": runEntityID, "task_count": len(plan.Tasks)},
	}, nil
}

// allTaskFieldPredicates is the full set of per-task field suffixes
// the executor may have stamped. clearPriorPlan removes every one for
// every task ID so an optional field present in revision N but absent
// in N+1 (e.g. expected_outcome) doesn't survive as a stale leftover.
var allTaskFieldPredicates = []string{
	predicateTaskGoal, predicateTaskAssumptions, predicateTaskNonGoals,
	predicateTaskTargetFiles, predicateTaskDependsOn, predicateTaskTestCommand,
	predicateTaskExpectedOutcome, predicateTaskStatus, predicateTaskPosition,
}

// allPlanLevelPredicates is the fixed (non-task) plan predicate set.
var allPlanLevelPredicates = []string{
	predicatePlanGoal, predicatePlanAssumptions, predicatePlanNonGoals,
	predicatePlanIntegrationTestCmd, predicatePlanChainStartGitTag,
	predicatePlanTaskCount, predicatePlanRevision, predicatePlanCBGRetryBudget,
	predicatePlanLisaRetryBudget, predicatePlanGeneratedAt,
}

// clearPriorPlan removes the full plan namespace on the run entity so
// a re-plan UPSERTs rather than appends. Removes are sequential +
// awaited (request-reply) so they all land before the caller's
// AddTriplesBatch. A missing predicate is a no-op success.
func (e *Executor) clearPriorPlan(ctx context.Context, runEntityID string, taskIDs []string) error {
	for _, pred := range allPlanLevelPredicates {
		if err := e.remover.RemoveByPredicate(ctx, runEntityID, pred); err != nil {
			return fmt.Errorf("remove %s: %w", pred, err)
		}
	}
	for _, id := range taskIDs {
		prefix := predicatePlanTaskPrefix + id + "."
		for _, field := range allTaskFieldPredicates {
			if err := e.remover.RemoveByPredicate(ctx, runEntityID, prefix+field); err != nil {
				return fmt.Errorf("remove %s%s: %w", prefix, field, err)
			}
		}
	}
	return nil
}

func runEntityFromCall(call agentic.ToolCall, platform types.PlatformMeta) (string, error) {
	related, _ := call.Metadata[agentic.MetadataKeyRelatedLoops].(map[string]any)
	runLoopID, ok := related[runLoopIDRoleKey].(string)
	if !ok || runLoopID == "" {
		return "", fmt.Errorf("emit_dev_via_test_plan: related_loops[%q] missing or empty in call metadata; spawn rule must pin the run loop id at chain start", runLoopIDRoleKey)
	}
	runEntityID, err := agentic.TryChainExecutionEntityID(platform.Org, platform.Platform, runLoopID)
	if err != nil {
		return "", fmt.Errorf("emit_dev_via_test_plan: build run entity id from run loop %q: %w", runLoopID, err)
	}
	return runEntityID, nil
}

// task is the per-task spec shape after parse+validate.
type task struct {
	ID              string   `json:"id"`
	Goal            string   `json:"goal"`
	Assumptions     []string `json:"assumptions"`
	NonGoals        []string `json:"non_goals"`
	TargetFiles     []string `json:"target_files"`
	DependsOn       []string `json:"depends_on"`
	TestCommand     string   `json:"test_command"`
	ExpectedOutcome string   `json:"expected_outcome"`
}

// plan is the parsed+validated payload.
type plan struct {
	Goal                   string   `json:"goal"`
	Assumptions            []string `json:"assumptions"`
	NonGoals               []string `json:"non_goals"`
	IntegrationTestCommand string   `json:"integration_test_command"`
	Revision               int      `json:"revision"`
	CBGRetryBudget         int      `json:"cbg_retry_budget"`
	LisaRetryBudget        int      `json:"lisa_retry_budget"`
	Tasks                  []task   `json:"tasks"`
}

// taskIDs returns the plan's task IDs in order — the set
// clearPriorPlan removes per-task predicates for on a re-plan.
func (p *plan) taskIDs() []string {
	ids := make([]string, len(p.Tasks))
	for i, t := range p.Tasks {
		ids[i] = t.ID
	}
	return ids
}

func parseArgs(raw map[string]any) (*plan, error) {
	if raw == nil {
		return nil, fmt.Errorf("arguments are required")
	}
	// Round-trip through JSON so the array shapes land as []string
	// rather than []any (the unmarshaller handles the coercion).
	buf, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("marshal arguments: %w", err)
	}
	var p plan
	dec := json.NewDecoder(strings.NewReader(string(buf)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		return nil, fmt.Errorf("unmarshal arguments: %w", err)
	}
	// Per reviewer R1 (Slice 1 review): no default coercion. Absent
	// (Go zero-int) and explicit `0` both surface as the same
	// validator error — schema authoritative. Coordinator re-plan
	// (Slice 3+) must always supply revision explicitly.
	if err := p.validate(); err != nil {
		return nil, err
	}
	return &p, nil
}

// validate enforces Karpathy-as-schema: required arrays must be
// present (nil is rejected; [] is accepted), required scalars
// non-empty, task IDs unique + pattern-matched, target_files ≥1.
func (p *plan) validate() error {
	if strings.TrimSpace(p.Goal) == "" {
		return fmt.Errorf("plan.goal is required (non-empty)")
	}
	if p.Assumptions == nil {
		return fmt.Errorf("plan.assumptions is required (emit [] explicitly if no assumptions — Karpathy Rule 1)")
	}
	if p.NonGoals == nil {
		return fmt.Errorf("plan.non_goals is required (emit [] explicitly if nothing excluded — Karpathy Rule 2)")
	}
	if strings.TrimSpace(p.IntegrationTestCommand) == "" {
		return fmt.Errorf("plan.integration_test_command is required (CBG's chain-end gate — Karpathy Rule 4 at plan scope)")
	}
	if len(p.Tasks) == 0 {
		return fmt.Errorf("plan.tasks must contain at least one task")
	}
	if p.Revision < 1 {
		return fmt.Errorf("plan.revision is required and must be >= 1 (got %d); first emit is revision=1, bump on coordinator-requested re-plan", p.Revision)
	}

	// CBG retry budget (ADR-044 §Slice 5): optional knob, clamp to
	// [1, maxCBGRetryBudget]. Absent / 0 / negative → default; the
	// clamp is the structural retry ceiling, so even a plan that
	// over-specifies the budget cannot push rule 07d past the
	// escalate boundary. NOT an error (unlike revision) — a
	// missing budget is the common case and the default is correct.
	if p.CBGRetryBudget < 1 {
		p.CBGRetryBudget = defaultCBGRetryBudget
	}
	if p.CBGRetryBudget > maxCBGRetryBudget {
		p.CBGRetryBudget = maxCBGRetryBudget
	}

	// Lisa re-plan budget (ADR-044 §Slice 6): same clamp posture as
	// cbg_retry_budget. Absent / 0 / negative → default; the clamp is
	// the structural plan-retry ceiling.
	if p.LisaRetryBudget < 1 {
		p.LisaRetryBudget = defaultLisaRetryBudget
	}
	if p.LisaRetryBudget > maxLisaRetryBudget {
		p.LisaRetryBudget = maxLisaRetryBudget
	}

	seenIDs := make(map[string]struct{}, len(p.Tasks))
	for i, t := range p.Tasks {
		if !taskIDPattern.MatchString(t.ID) {
			return fmt.Errorf("tasks[%d].id %q does not match required pattern (lowercase alphanumeric + hyphens, 1-32 chars)", i, t.ID)
		}
		if _, dup := seenIDs[t.ID]; dup {
			return fmt.Errorf("tasks[%d].id %q is duplicated; task IDs must be unique within a plan", i, t.ID)
		}
		seenIDs[t.ID] = struct{}{}
		if strings.TrimSpace(t.Goal) == "" {
			return fmt.Errorf("tasks[%d=%s].goal is required (non-empty)", i, t.ID)
		}
		if t.Assumptions == nil {
			return fmt.Errorf("tasks[%d=%s].assumptions is required (emit [] explicitly)", i, t.ID)
		}
		if t.NonGoals == nil {
			return fmt.Errorf("tasks[%d=%s].non_goals is required (emit [] explicitly)", i, t.ID)
		}
		if len(t.TargetFiles) == 0 {
			return fmt.Errorf("tasks[%d=%s].target_files must contain at least one path (Karpathy Rule 3 — surgical changes; pick the narrowest accurate set)", i, t.ID)
		}
		if strings.TrimSpace(t.TestCommand) == "" {
			return fmt.Errorf("tasks[%d=%s].test_command is required (Karpathy Rule 4 — goal-driven execution)", i, t.ID)
		}
		if t.DependsOn == nil {
			t.DependsOn = []string{}
			p.Tasks[i] = t
		}
	}
	return nil
}

// triples assembles the per-emit triple batch. Returns an error
// only if JSON-encoding an array field somehow fails (impossible
// for the validated shape; defensive).
func (p *plan) triples(runEntityID string, now time.Time) ([]message.Triple, error) {
	base := func(pred string, obj any) message.Triple {
		return message.Triple{
			Subject:    runEntityID,
			Predicate:  pred,
			Object:     obj,
			Source:     toolSource,
			Timestamp:  now,
			Confidence: 1.0,
		}
	}
	jsonStr := func(v any) (string, error) {
		b, err := json.Marshal(v)
		if err != nil {
			return "", err
		}
		return string(b), nil
	}

	assumptionsJSON, err := jsonStr(p.Assumptions)
	if err != nil {
		return nil, fmt.Errorf("encode plan.assumptions: %w", err)
	}
	nonGoalsJSON, err := jsonStr(p.NonGoals)
	if err != nil {
		return nil, fmt.Errorf("encode plan.non_goals: %w", err)
	}

	out := make([]message.Triple, 0, 8+len(p.Tasks)*8)
	out = append(out,
		base(predicatePlanGoal, p.Goal),
		base(predicatePlanAssumptions, assumptionsJSON),
		base(predicatePlanNonGoals, nonGoalsJSON),
		base(predicatePlanIntegrationTestCmd, p.IntegrationTestCommand),
		base(predicatePlanChainStartGitTag, defaultChainStartGitTag),
		base(predicatePlanTaskCount, len(p.Tasks)),
		base(predicatePlanRevision, p.Revision),
		base(predicatePlanCBGRetryBudget, p.CBGRetryBudget),
		base(predicatePlanLisaRetryBudget, p.LisaRetryBudget),
		base(predicatePlanGeneratedAt, now.Format(time.RFC3339Nano)),
		// ADR-053 Phase 3b: run-lifecycle marker on the run entity (moved from
		// rule 01's add_triple on the coordinator loop). Idempotent across a
		// re-plan re-emit (same (subject,predicate,object) dedups).
		base(predicateRunStatus, "active"),
	)

	for i, t := range p.Tasks {
		taskAssumptionsJSON, err := jsonStr(t.Assumptions)
		if err != nil {
			return nil, fmt.Errorf("encode tasks[%d=%s].assumptions: %w", i, t.ID, err)
		}
		taskNonGoalsJSON, err := jsonStr(t.NonGoals)
		if err != nil {
			return nil, fmt.Errorf("encode tasks[%d=%s].non_goals: %w", i, t.ID, err)
		}
		taskTargetFilesJSON, err := jsonStr(t.TargetFiles)
		if err != nil {
			return nil, fmt.Errorf("encode tasks[%d=%s].target_files: %w", i, t.ID, err)
		}
		taskDependsOnJSON, err := jsonStr(t.DependsOn)
		if err != nil {
			return nil, fmt.Errorf("encode tasks[%d=%s].depends_on: %w", i, t.ID, err)
		}

		prefix := predicatePlanTaskPrefix + t.ID + "."
		out = append(out,
			base(prefix+predicateTaskGoal, t.Goal),
			base(prefix+predicateTaskAssumptions, taskAssumptionsJSON),
			base(prefix+predicateTaskNonGoals, taskNonGoalsJSON),
			base(prefix+predicateTaskTargetFiles, taskTargetFilesJSON),
			base(prefix+predicateTaskDependsOn, taskDependsOnJSON),
			base(prefix+predicateTaskTestCommand, t.TestCommand),
			base(prefix+predicateTaskStatus, taskStatusReady),
			base(prefix+predicateTaskPosition, i),
		)
		if strings.TrimSpace(t.ExpectedOutcome) != "" {
			out = append(out, base(prefix+predicateTaskExpectedOutcome, t.ExpectedOutcome))
		}
	}
	return out, nil
}

func errResult(call agentic.ToolCall, kind agentic.ToolErrorKind, format string, args ...any) (agentic.ToolResult, error) {
	return agentic.ToolResult{
		CallID:    call.ID,
		Name:      call.Name,
		Error:     fmt.Sprintf(format, args...),
		ErrorKind: kind,
	}, nil
}
