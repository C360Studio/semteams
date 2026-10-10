package contract

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/payloadregistry"
	agenticloop "github.com/c360studio/semstreams/processor/agentic-loop"
	"github.com/c360studio/semstreams/processor/rule"
	"github.com/c360studio/semstreams/types"
	"github.com/c360studio/semteams/internal/runtimecatalog"
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
			for phase, actions := range map[string][]rule.Action{"on_enter": definition.OnEnter, "on_exit": definition.OnExit} {
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
