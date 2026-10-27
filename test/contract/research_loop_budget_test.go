package contract

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/payloadregistry"
	agenticloop "github.com/c360studio/semstreams/processor/agentic-loop"
	agentictools "github.com/c360studio/semstreams/processor/agentic-tools"
	"github.com/c360studio/semstreams/processor/rule"
	"github.com/c360studio/semstreams/types"
	"github.com/c360studio/semteams/cmd/semteams/research"
	"github.com/c360studio/semteams/cmd/semteams/tools/emitplan"
	"github.com/c360studio/semteams/cmd/semteams/tools/researchdecide"
	"github.com/c360studio/semteams/internal/runtimecatalog"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
)

// Literal pack policy, independent of the production decoder and cap resolver.
var researchRoleBudgets = map[string]int{
	"researcher-research-plan": 12, "researcher-research-gather": 20,
	"researcher-research-synthesize": 24, "reviewer-research": 8, "coordinator": 8,
	"autoresearch-baseline": 20, "autoresearch-propose": 20, "autoresearch-execute": 20,
	"autoresearch-synthesize": 30, "reviewer-autoresearch": 8,
}

func TestResearchSpawnLoopBudgets(t *testing.T) {
	registry := payloadregistry.New()
	require.NoError(t, runtimecatalog.RegisterPayloads(registry))
	decoder := message.NewDecoder(registry)
	for _, pack := range []string{"research", "autoresearch"} {
		paths, err := filepath.Glob(filepath.Join("../../configs/rules", pack, "*.json"))
		require.NoError(t, err)
		require.NotEmpty(t, paths)
		spawns := 0
		for _, path := range paths {
			var definition rule.Definition
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			require.NoError(t, json.Unmarshal(data, &definition))
			for phase, actions := range map[string][]rule.Action{"on_enter": definition.OnEnter, "on_exit": definition.OnExit, "actions": definition.Actions} {
				for i, action := range actions {
					if action.Type != rule.ActionTypePublishAgent {
						continue
					}
					spawns++
					t.Run(fmt.Sprintf("%s/%s/%s/%d", pack, filepath.Base(path), phase, i), func(t *testing.T) {
						budget, known := researchRoleBudgets[action.Role]
						require.True(t, known, "new role needs a reviewed iteration budget")
						require.Equal(t, strconv.Itoa(budget), action.LoopMaxIterations, "every live spawn must declare its role budget")
						// Execute the unchanged decoded action, but on a non-loop fixture entity.
						// This deliberately excludes condition evaluation, run mint/inheritance,
						// tool resolution and transport. Fanout substitution is real executor code.
						publisher := &ruleFamilyPublisher{}
						executor := rule.NewActionExecutorFull(discardLogger(), nil, publisher, types.PlatformMeta{Org: "c360", Platform: "budget-test"})
						entityID := "c360.budget-test.rule.fixture.entity.input"
						entity := &graph.EntityState{ID: entityID, Triples: []message.Triple{{
							Subject: entityID, Predicate: "coordinator.decision.subtopics", Object: `["primary evidence","conflicting evidence","remaining gaps"]`,
						}}}
						require.NoError(t, executor.Execute(t.Context(), action, &rule.ExecutionContext{EntityID: entityID, Entity: entity}))
						count := 1
						if action.ForEach != "" {
							count = 3
						}
						require.Len(t, publisher.messages, count)
						for child, wire := range publisher.messages {
							envelope, err := decoder.Decode(wire)
							require.NoError(t, err)
							task, ok := envelope.Payload().(*agentic.TaskMessage)
							require.True(t, ok)
							require.NoError(t, task.Validate())
							require.NotNil(t, task.MaxIterations)
							require.Equal(t, budget, *task.MaxIterations, "fanout child %d lost budget", child)
							require.Equal(t, action.Subject, publisher.subjects[child])
							for _, ceiling := range []int{50, 5} {
								cfg := agenticloop.DefaultConfig()
								cfg.MaxIterations = ceiling
								handler := agenticloop.NewMessageHandler(cfg)
								handler.SetLogger(discardLogger())
								result, err := handler.HandleTask(t.Context(), *task)
								require.NoError(t, err)
								entity, err := handler.GetLoop(result.LoopID)
								require.NoError(t, err)
								require.Equal(t, min(budget, ceiling), entity.MaxIterations, "public intake must narrow to the component ceiling")
							}
						}
					})
				}
			}
		}
		require.Positive(t, spawns, "pack must exercise actual spawns")
	}
}

type contractTripleRecorder struct {
	mu      sync.Mutex
	triples []message.Triple
}

func (r *contractTripleRecorder) Create(_ context.Context, _ string, _ message.Type, triples []message.Triple) error {
	return r.Append(context.Background(), triples)
}

func (r *contractTripleRecorder) Append(_ context.Context, triples []message.Triple) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.triples = append(r.triples, triples...)
	return nil
}

func (r *contractTripleRecorder) snapshot() []message.Triple {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]message.Triple, len(r.triples))
	copy(out, r.triples)
	return out
}

func loadRuleDefinition(t *testing.T, relPath string) rule.Definition {
	t.Helper()
	data, err := os.ReadFile(relPath)
	require.NoError(t, err)
	var def rule.Definition
	require.NoError(t, json.Unmarshal(data, &def))
	return def
}

func makeSubtopicSlice(n int) []any {
	out := make([]any, n)
	for i := range out {
		out[i] = fmt.Sprintf("subtopic-%d: verify boundary %d", i+1, i+1)
	}
	return out
}

// TestResearchPlannerFanOutSchemasAndEnforcement verifies the #272 planner
// fan-out containment budget (1..4 subtopics):
//  1. Both emit_plan.epics and decide.subtopics schemas declare minItems=1, maxItems=4.
//  2. Valid N=1 and N=4 planner decide(action="gather") calls stamp subtopics,
//     rule 02-plan-to-gather dispatches N gatherers, and rule 03b joins when all N complete.
//  3. Invalid missing, N=0, N=5, and N=15 planner decide calls (even when emit_plan
//     was skipped) fail pre-mutation with ToolErrorInvalidArgs and StopLoop=false,
//     write zero triples, and dispatch zero gatherers.
//  4. Rule 02-plan-to-gather's 1..4 element JSON string-array regex condition rejects
//     out-of-bounds subtopics in graph state as defense-in-depth.
//  5. Same-loop correction: N=5 rejected pre-mutation followed by N=4 accepted.
func TestResearchPlannerFanOutSchemasAndEnforcement(t *testing.T) {
	platform := types.PlatformMeta{Org: "c360", Platform: "budget-test"}
	registry := payloadregistry.New()
	require.NoError(t, runtimecatalog.RegisterPayloads(registry))
	decoder := message.NewDecoder(registry)

	t.Run("schemas_declare_min1_max4", func(t *testing.T) {
		planExec := emitplan.NewExecutor(&contractTripleRecorder{}, &ruleFamilyPublisher{}, platform, discardLogger(), t.TempDir())
		planDefs := planExec.ListTools()
		require.Len(t, planDefs, 1)
		planProps := planDefs[0].Parameters["properties"].(map[string]any)
		epicsSchema := planProps["epics"].(map[string]any)
		require.Equal(t, research.MinSubtopics, epicsSchema["minItems"])
		require.Equal(t, research.MaxSubtopics, epicsSchema["maxItems"])

		decideInner := agentictools.NewDecideExecutor(&contractTripleRecorder{}, platform, nil)
		decideExec := researchdecide.NewExecutor(decideInner, nil, discardLogger())
		decideDefs := decideExec.ListTools()
		require.Len(t, decideDefs, 1)
		decideProps := decideDefs[0].Parameters["properties"].(map[string]any)
		subtopicsSchema := decideProps["subtopics"].(map[string]any)
		require.Equal(t, research.MinSubtopics, subtopicsSchema["minItems"])
		require.Equal(t, research.MaxSubtopics, subtopicsSchema["maxItems"])
		required := decideDefs[0].Parameters["required"].([]string)
		require.NotContains(t, required, "subtopics", "subtopics must stay optional for non-fan-out roles")
	})

	rule02Def := loadRuleDefinition(t, "../../configs/rules/research/02-plan-to-gather.json")
	rule02, err := rule.NewExpressionRule(platform, "research", rule02Def)
	require.NoError(t, err)

	rule03bDef := loadRuleDefinition(t, "../../configs/rules/research/03b-synthesize-when-all-gathers-complete.json")
	rule03b, err := rule.NewExpressionRule(platform, "research", rule03bDef)
	require.NoError(t, err)

	for _, count := range []int{1, 4} {
		t.Run(fmt.Sprintf("valid_fanout_N=%d_dispatches_and_joins", count), func(t *testing.T) {
			recorder := &contractTripleRecorder{}
			decideExec := researchdecide.NewExecutor(
				agentictools.NewDecideExecutor(recorder, platform, nil),
				nil,
				discardLogger(),
			)
			loopID := fmt.Sprintf("11111111-1111-1111-1111-00000000000%d", count)
			res, err := decideExec.Execute(t.Context(), agentic.ToolCall{
				ID:     "call-plan",
				Name:   agentic.DecideToolName,
				LoopID: loopID,
				Arguments: map[string]any{
					"action":    "gather",
					"reason":    "decomposed research plan",
					"subtopics": makeSubtopicSlice(count),
				},
				Metadata: map[string]any{
					agentic.MetadataKeyAgentRole:             research.PlannerRole,
					agentic.MetadataKeyDecideActionAllowlist: []string{"gather", "emit", "needs_clarification"},
				},
			})
			require.NoError(t, err)
			require.Empty(t, res.Error)
			require.True(t, res.StopLoop)

			loopEntityID := agentic.LoopExecutionEntityID(platform.Org, platform.Platform, loopID)
			triples := append([]message.Triple{{
				Subject:   loopEntityID,
				Predicate: "agent.loop.role",
				Object:    research.PlannerRole,
			}}, recorder.snapshot()...)
			planEntity := &graph.EntityState{ID: loopEntityID, Triples: triples}

			require.True(t, rule02.EvaluateEntityState(t.Context(), planEntity), "rule 02 must match valid N=%d planner decision", count)

			pub02 := &ruleFamilyPublisher{}
			exec02 := rule.NewActionExecutorFull(discardLogger(), nil, pub02, platform)
			require.NoError(t, exec02.Execute(t.Context(), rule02Def.OnEnter[0], &rule.ExecutionContext{
				EntityID: loopEntityID,
				Entity:   planEntity,
			}))
			require.Len(t, pub02.messages, count, "rule 02 must dispatch %d gatherers", count)
			for i, wire := range pub02.messages {
				env, err := decoder.Decode(wire)
				require.NoError(t, err)
				task, ok := env.Payload().(*agentic.TaskMessage)
				require.True(t, ok)
				require.Equal(t, "researcher-research-gather", task.Role)
				require.NotNil(t, task.MaxIterations)
				require.Equal(t, 20, *task.MaxIterations, "gatherer %d must carry loop_max_iterations=20", i)
			}

			// Verify rule 03b join: does not fire before all N gatherers stamp completion; fires when Nth stamps.
			for g := 1; g <= count; g++ {
				if g < count {
					require.False(t, rule03b.EvaluateEntityState(t.Context(), planEntity), "rule 03b must not join before %d/%d gatherers complete", g-1, count)
				}
				planEntity.Triples = append(planEntity.Triples, message.Triple{
					Subject:   loopEntityID,
					Predicate: "research.gather.completed-subtopic",
					Object:    fmt.Sprintf("gather-loop-%d", g),
				})
			}
			require.True(t, rule03b.EvaluateEntityState(t.Context(), planEntity), "rule 03b must join once all %d gatherers complete", count)
		})
	}

	for _, tc := range []struct {
		name      string
		subtopics any
		hasField  bool
	}{
		{name: "missing_subtopics", hasField: false},
		{name: "empty_N=0", subtopics: []any{}, hasField: true},
		{name: "over_limit_N=5", subtopics: makeSubtopicSlice(5), hasField: true},
		{name: "over_limit_N=15", subtopics: makeSubtopicSlice(15), hasField: true},
	} {
		t.Run("rejected_"+tc.name, func(t *testing.T) {
			recorder := &contractTripleRecorder{}
			decideExec := researchdecide.NewExecutor(
				agentictools.NewDecideExecutor(recorder, platform, nil),
				nil,
				discardLogger(),
			)
			loopID := "22222222-2222-2222-2222-222222222222"
			args := map[string]any{
				"action": "gather",
				"reason": "attempting invalid subtopic fan-out without calling emit_plan",
			}
			if tc.hasField {
				args["subtopics"] = tc.subtopics
			}
			res, err := decideExec.Execute(t.Context(), agentic.ToolCall{
				ID:        "call-bad",
				Name:      agentic.DecideToolName,
				LoopID:    loopID,
				Arguments: args,
				Metadata: map[string]any{
					agentic.MetadataKeyAgentRole:             research.PlannerRole,
					agentic.MetadataKeyDecideActionAllowlist: []string{"gather", "emit", "needs_clarification"},
				},
			})
			require.NoError(t, err)
			require.Equal(t, agentic.ToolErrorInvalidArgs, res.ErrorKind)
			require.False(t, res.StopLoop, "planner loop must stay alive to self-correct")
			require.Empty(t, recorder.snapshot(), "zero triples may be written on rejected subtopics")

			loopEntityID := agentic.LoopExecutionEntityID(platform.Org, platform.Platform, loopID)
			planEntity := &graph.EntityState{
				ID: loopEntityID,
				Triples: []message.Triple{{
					Subject:   loopEntityID,
					Predicate: "agent.loop.role",
					Object:    research.PlannerRole,
				}},
			}
			require.False(t, rule02.EvaluateEntityState(t.Context(), planEntity), "rule 02 must not match when decide rejected pre-mutation")
		})
	}

	t.Run("rule02_defense_in_depth_rejects_unbounded_triples", func(t *testing.T) {
		for _, count := range []int{0, 5, 15} {
			raw, err := json.Marshal(makeSubtopicSlice(count))
			require.NoError(t, err)
			entityID := "c360.budget-test.agentic-loop.agent.execution.33333333-3333-3333-3333-333333333333"
			entity := &graph.EntityState{
				ID: entityID,
				Triples: []message.Triple{
					{Subject: entityID, Predicate: "agent.loop.role", Object: research.PlannerRole},
					{Subject: entityID, Predicate: "coordinator.decision.next-action", Object: "gather"},
					{Subject: entityID, Predicate: "coordinator.decision.subtopics", Object: string(raw)},
				},
			}
			require.False(t, rule02.EvaluateEntityState(t.Context(), entity), "rule 02 must reject synthetic subtopics count=%d via regex condition", count)
		}
	})

	t.Run("same_loop_correction_N5_rejected_then_N4_dispatched", func(t *testing.T) {
		recorder := &contractTripleRecorder{}
		decideExec := researchdecide.NewExecutor(
			agentictools.NewDecideExecutor(recorder, platform, nil),
			nil,
			discardLogger(),
		)
		loopID := "44444444-4444-4444-4444-444444444444"

		res1, err := decideExec.Execute(t.Context(), agentic.ToolCall{
			ID:     "call-1",
			Name:   agentic.DecideToolName,
			LoopID: loopID,
			Arguments: map[string]any{
				"action":    "gather",
				"reason":    "5 subtopics too wide",
				"subtopics": makeSubtopicSlice(5),
			},
		})
		require.NoError(t, err)
		require.Equal(t, agentic.ToolErrorInvalidArgs, res1.ErrorKind)
		require.False(t, res1.StopLoop)
		require.Empty(t, recorder.snapshot())

		res2, err := decideExec.Execute(t.Context(), agentic.ToolCall{
			ID:     "call-2",
			Name:   agentic.DecideToolName,
			LoopID: loopID,
			Arguments: map[string]any{
				"action":    "gather",
				"reason":    "consolidated to 4 subtopics",
				"subtopics": makeSubtopicSlice(4),
			},
		})
		require.NoError(t, err)
		require.Empty(t, res2.Error)
		require.True(t, res2.StopLoop)
		require.Len(t, recorder.snapshot(), 3)

		loopEntityID := agentic.LoopExecutionEntityID(platform.Org, platform.Platform, loopID)
		planEntity := &graph.EntityState{
			ID: loopEntityID,
			Triples: append([]message.Triple{{
				Subject:   loopEntityID,
				Predicate: "agent.loop.role",
				Object:    research.PlannerRole,
			}}, recorder.snapshot()...),
		}
		require.True(t, rule02.EvaluateEntityState(t.Context(), planEntity))
	})
}

// TestLiveCronRulesHaveCooldownAndLoopBudget enforces that any cron rule in
// configs/rules/**/*.json (plus the canonical scheduled pulse cron shape from
// ADR-060 §7 / #272) declares a positive cooldown duration and a positive
// loop_max_iterations on every publish_agent action.
func TestLiveCronRulesHaveCooldownAndLoopBudget(t *testing.T) {
	paths, err := filepath.Glob("../../configs/rules/*/*.json")
	require.NoError(t, err)
	require.NotEmpty(t, paths)

	validateCronDef := func(t *testing.T, label string, def rule.Definition) {
		t.Helper()
		cronRule, err := rule.NewCronRule(def)
		require.NoError(t, err, "%s: NewCronRule failed", label)
		require.Positive(t, cronRule.Cooldown(), "%s: cron rule must configure a positive cooldown to prevent overlapping/restart storms", label)
		for i, action := range def.Actions {
			if action.Type != rule.ActionTypePublishAgent {
				continue
			}
			budget, err := strconv.Atoi(action.LoopMaxIterations)
			require.NoError(t, err, "%s: action[%d] loop_max_iterations must be an integer", label, i)
			require.Positive(t, budget, "%s: action[%d] loop_max_iterations must be > 0", label, i)
		}
	}

	for _, path := range paths {
		def := loadRuleDefinition(t, path)
		if def.Type != rule.CronRuleType {
			continue
		}
		t.Run(filepath.Base(path), func(t *testing.T) {
			validateCronDef(t, path, def)
		})
	}

	// Pin the ADR-060 §7 / #272 scheduled pulse cron contract shape so the
	// validator is always exercised even while no file-backed cron rule is
	// enabled by default in flow-bootstrap.json.
	t.Run("canonical_scheduled_pulse_cron_shape", func(t *testing.T) {
		validateCronDef(t, "canonical_scheduled_pulse", rule.Definition{
			ID:       "scheduled_program_pulse",
			Type:     rule.CronRuleType,
			Name:     "Scheduled Program Pulse",
			Enabled:  true,
			Schedule: "0 */6 * * *",
			Cooldown: "4h",
			Actions: []rule.Action{{
				Type:              rule.ActionTypePublishAgent,
				Subject:           "agent.task.coordinator",
				Role:              "coordinator",
				LoopMaxIterations: "8",
				Model:             "fast",
				Prompt:            "Run scheduled portfolio pulse check.",
			}},
		})
	})
}

// TestReviewerRejectedRetry_PerEntityCounterScope mechanically pins the
// partial-containment boundary of rule 05-reviewer-rejected-retry.json
// (max_iterations: 3):
//   - For a single reviewer loop entity ID, StatefulEvaluator caps OnEnter
//     executions at 3 across re-entries.
//   - Across distinct reviewer loop entity IDs (each review pass in a live
//     research chain runs as a fresh reviewer-research loop entity), the
//     per-entity counter starts at 1 for each new entity. Total-run retry
//     caps across fresh loop identities remain tracked upstream
//     (semstreams#383, semstreams#1005, semengine#24).
func TestReviewerRejectedRetry_PerEntityCounterScope(t *testing.T) {
	platform := types.PlatformMeta{Org: "c360", Platform: "budget-test"}
	rule05Def := loadRuleDefinition(t, "../../configs/rules/research/05-reviewer-rejected-retry.json")
	require.Equal(t, 3, rule05Def.MaxIterations)

	rule05, err := rule.NewExpressionRule(platform, "research", rule05Def)
	require.NoError(t, err)

	reviewerEntity := func(instance string) *graph.EntityState {
		id := fmt.Sprintf("c360.budget-test.agentic-loop.agent.execution.%s", instance)
		return &graph.EntityState{
			ID: id,
			Triples: []message.Triple{
				{Subject: id, Predicate: "agent.loop.role", Object: "reviewer-research"},
				{Subject: id, Predicate: "coordinator.decision.next-action", Object: "insufficient"},
				{Subject: id, Predicate: "agent.lineage.researcher", Object: "00000000-0000-0000-0000-000000000001"},
			},
		}
	}

	t.Run("same_entity_capped_at_3", func(t *testing.T) {
		tracker := rule.NewStateTracker(newInMemoryKVBucket(), discardLogger())
		pub := &ruleFamilyPublisher{}
		actionExec := rule.NewActionExecutorFull(discardLogger(), nil, pub, platform)
		evaluator := rule.NewStatefulEvaluator(tracker, actionExec, discardLogger())

		entity := reviewerEntity("55555555-5555-5555-5555-555555555550")
		require.True(t, rule05.EvaluateEntityState(t.Context(), entity))

		for attempt := 1; attempt <= 5; attempt++ {
			_, err := evaluator.Evaluate(t.Context(), rule.Evaluation{
				Rule:              rule05Def,
				EntityID:          entity.ID,
				CurrentlyMatching: true,
				Entity:            entity,
			})
			require.NoError(t, err)
			// Exit match state so the next attempt is a fresh Entered transition on the same entity.
			_, err = evaluator.Evaluate(t.Context(), rule.Evaluation{
				Rule:              rule05Def,
				EntityID:          entity.ID,
				CurrentlyMatching: false,
				Entity:            entity,
			})
			require.NoError(t, err)
		}
		require.Len(t, pub.messages, 3, "same reviewer entity must be capped at max_iterations=3")
	})

	t.Run("distinct_reviewer_entities_each_start_at_iteration_1", func(t *testing.T) {
		tracker := rule.NewStateTracker(newInMemoryKVBucket(), discardLogger())
		pub := &ruleFamilyPublisher{}
		actionExec := rule.NewActionExecutorFull(discardLogger(), nil, pub, platform)
		evaluator := rule.NewStatefulEvaluator(tracker, actionExec, discardLogger())

		for pass := 1; pass <= 4; pass++ {
			entity := reviewerEntity(fmt.Sprintf("55555555-5555-5555-5555-55555555555%d", pass))
			require.True(t, rule05.EvaluateEntityState(t.Context(), entity))
			_, err := evaluator.Evaluate(t.Context(), rule.Evaluation{
				Rule:              rule05Def,
				EntityID:          entity.ID,
				CurrentlyMatching: true,
				Entity:            entity,
			})
			require.NoError(t, err)
		}
		require.Len(t, pub.messages, 4, "fresh reviewer loop entities do not share the per-entity max_iterations=3 counter (tracked upstream: semstreams#383, semstreams#1005, semengine#24)")
	})
}

type inMemoryKVBucket struct {
	jetstream.KeyValue
	mu   sync.RWMutex
	data map[string][]byte
}

func newInMemoryKVBucket() *inMemoryKVBucket {
	return &inMemoryKVBucket{data: make(map[string][]byte)}
}

func (b *inMemoryKVBucket) Get(_ context.Context, key string) (jetstream.KeyValueEntry, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	val, ok := b.data[key]
	if !ok {
		return nil, jetstream.ErrKeyNotFound
	}
	cp := append([]byte(nil), val...)
	return inMemoryKVEntry{value: cp}, nil
}

func (b *inMemoryKVBucket) Put(_ context.Context, key string, value []byte) (uint64, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.data[key] = append([]byte(nil), value...)
	return 1, nil
}

type inMemoryKVEntry struct {
	jetstream.KeyValueEntry
	value []byte
}

func (e inMemoryKVEntry) Value() []byte {
	return e.value
}
