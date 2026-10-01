//go:build integration

package contract

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/agentic/agentrun"
	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/pkg/lifecycle"
	agenticloop "github.com/c360studio/semstreams/processor/agentic-loop"
	rulepkg "github.com/c360studio/semstreams/processor/rule"
	"github.com/c360studio/semstreams/processor/rule/expression"
	"github.com/c360studio/semstreams/types"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
)

const replyRoot = "11111111-1111-4111-8111-111111111111"
const replyReviewer = "22222222-2222-4222-8222-222222222222"
const replyOldRoot = "33333333-3333-4333-8333-333333333333"
const replyOrigin = "c360.reply-test.agentic-loop.agent.execution." + replyRoot
const replyRun = "c360.reply-test.chain.agent.execution." + replyRoot
const replySource = "c360.reply-test.agentic-loop.agent.execution." + replyReviewer

// The real frozen matcher, stateful evaluator, ActionExecutor and Mint run here.
// Only graph/lifecycle persistence and the task transport are deterministic fixtures;
// match history uses a fresh NATS KV and survives evaluator recreation.
type replyHarness struct {
	rulepkg.LifecycleManager
	entities  map[string]*graph.EntityState
	runs      map[string]*agentrun.AgentRun
	tasks     []agentic.TaskMessage
	actions   *rulepkg.ActionExecutor
	evaluator *rulepkg.StatefulEvaluator
	bucket    jetstream.KeyValue
	logger    *slog.Logger
	revision  uint64
}

func newReplyHarness(t *testing.T, tc *natsclient.TestClient, phase string) *replyHarness {
	t.Helper()
	bucket, err := tc.CreateKVBucket(t.Context(), fmt.Sprintf("%s_%d", strings.ReplaceAll(t.Name(), "/", "_"), time.Now().UnixNano()))
	require.NoError(t, err)
	h := &replyHarness{entities: map[string]*graph.EntityState{}, runs: map[string]*agentrun.AgentRun{}, bucket: bucket,
		logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	h.runs[replyRun] = &agentrun.AgentRun{EntityIDField: replyRun, PhaseField: phase, OriginEntityID: replyOrigin}
	h.seed(replyOrigin, map[string]any{"agent.loop.role": "coordinator", "coordinator.decision.next-action": "autoresearch", "agent.loop.run": replyOldRoot})
	// Recovery coordinators can retain an inherited run before minting their own.
	h.append(message.Triple{Subject: replyOrigin, Predicate: "agent.loop.run", Object: replyRoot})
	h.seed(replyRun, map[string]any{"agent.run.phase": phase, "agent.run.origin-entity-id": replyOrigin})
	h.seed(replySource, map[string]any{"agent.loop.role": "reviewer-autoresearch", "agent.loop.outcome": "success", "coordinator.decision.next-action": "approved", "agent.lineage.run-loop-entity-id": replyRun, "agent.lineage.autoresearch-run": replyRoot})
	h.actions = rulepkg.NewActionExecutorFull(h.logger, h, h, types.PlatformMeta{Org: "c360", Platform: "reply-test"})
	h.actions.SetLifecycleManager(h)
	h.restart()
	return h
}
func (h *replyHarness) restart() {
	h.evaluator = rulepkg.NewStatefulEvaluator(rulepkg.NewStateTracker(h.bucket, h.logger), h, h.logger)
}
func (h *replyHarness) seed(id string, values map[string]any) {
	e := &graph.EntityState{ID: id}
	for p, v := range values {
		e.Triples = append(e.Triples, message.Triple{Subject: id, Predicate: p, Object: v})
	}
	h.entities[id] = e
}
func (h *replyHarness) append(tr message.Triple) {
	e := h.entities[tr.Subject]
	if e == nil {
		e = &graph.EntityState{ID: tr.Subject}
		h.entities[tr.Subject] = e
	}
	for _, old := range e.Triples {
		if old.Predicate == tr.Predicate && fmt.Sprint(old.Object) == fmt.Sprint(tr.Object) {
			return
		}
	}
	e.Triples = append(e.Triples, tr)
}
func (h *replyHarness) replace(id, p string, v any) {
	e := h.entities[id]
	out := e.Triples[:0]
	for _, tr := range e.Triples {
		if tr.Predicate != p {
			out = append(out, tr)
		}
	}
	e.Triples = out
	if v != nil {
		h.append(message.Triple{Subject: id, Predicate: p, Object: v})
	}
}
func (h *replyHarness) AddTriple(_ context.Context, _ string, tr message.Triple) (uint64, error) {
	h.append(tr)
	h.revision++
	return h.revision, nil
}
func (h *replyHarness) RemoveTriple(_ context.Context, _ string, id, p string) (uint64, error) {
	h.replace(id, p, nil)
	h.revision++
	return h.revision, nil
}
func (h *replyHarness) Publish(_ context.Context, _ string, data []byte) error {
	var env struct {
		Payload agentic.TaskMessage `json:"payload"`
	}
	if err := json.Unmarshal(data, &env); err != nil {
		return err
	}
	h.tasks = append(h.tasks, env.Payload)
	return nil
}
func (h *replyHarness) Create(_ context.Context, p lifecycle.Participant) error {
	if _, ok := h.runs[p.EntityID()]; ok {
		return lifecycle.ErrAlreadyExists
	}
	return fmt.Errorf("unexpected new run %s", p.EntityID())
}
func (h *replyHarness) Get(_ context.Context, _ string, id string) (lifecycle.Participant, error) {
	p, ok := h.runs[id]
	if !ok {
		return nil, fmt.Errorf("missing run %s", id)
	}
	return p, nil
}
func (h *replyHarness) Execute(ctx context.Context, a rulepkg.Action, ec *rulepkg.ExecutionContext) error {
	if a.Type == rulepkg.ActionTypeReconcilePredicates {
		// Persistence fixture for the unchanged iteration driver; the production
		// target index is private. When clauses and firing caps stay in the real evaluator.
		subject := ec.EntityID
		if a.Subject != "" {
			subject = ec.SubstituteVariables(ctx, a.Subject)
		}
		var value any
		if a.Object != "" {
			value = ec.SubstituteVariables(ctx, a.Object)
		}
		h.replace(subject, a.Predicate, value)
		return nil
	}
	return h.actions.Execute(ctx, a, ec)
}
func replyRule(t *testing.T, file string) rulepkg.Definition {
	t.Helper()
	raw, err := os.ReadFile("../../configs/rules/autoresearch/" + file)
	require.NoError(t, err)
	var def rulepkg.Definition
	require.NoError(t, json.Unmarshal(raw, &def))
	return def
}
func (h *replyHarness) evaluate(t *testing.T, def rulepkg.Definition, id string, rev uint64, bootstrap bool) {
	t.Helper()
	entity := h.entities[id]
	matched, err := rulepkg.Matches(t.Context(), def, entity)
	if err != nil {
		var eval *expression.EvaluationError
		if errors.As(err, &eval) && eval.Message == "required field not found" {
			matched = false
		} else {
			require.NoError(t, err)
		}
	}
	_, err = h.evaluator.Evaluate(t.Context(), rulepkg.Evaluation{Rule: def, EntityID: id, Entity: entity, CurrentlyMatching: matched, Revision: rev, Bootstrap: bootstrap})
	require.NoError(t, err)
}
func requireReplyTask(t *testing.T, h *replyHarness, source, kind, phase string) {
	t.Helper()
	require.Len(t, h.tasks, 1)
	task := h.tasks[0]
	require.Equal(t, replyRoot, task.ParentLoopID)
	require.Equal(t, replyRoot, task.RunID)
	require.Equal(t, "coordinator", task.Role)
	related, ok := task.Metadata[agentic.MetadataKeyRelatedLoops].(map[string]any)
	require.True(t, ok)
	require.Equal(t, source, related[kind])
	require.Equal(t, replyRoot, related["autoresearch-run"])
	require.Contains(t, task.Prompt, source)
	require.NotContains(t, task.Prompt, "$entity")
	require.Equal(t, phase, h.runs[replyRun].PhaseField)
	require.Equal(t, replyOrigin, h.runs[replyRun].OriginEntityID)
	require.Len(t, h.runs, 1)
}

func TestAutoresearchReplyNativeOrigin(t *testing.T) {
	tc := natsclient.NewTestClient(t, natsclient.WithNATSVersion("2.14.4"), natsclient.WithJetStream())
	t.Run("reviewer waits for successful run", func(t *testing.T) {
		h := newReplyHarness(t, tc, "executing")
		h.evaluate(t, replyRule(t, "08-reviewer-approved-to-coordinator.json"), replySource, 1, false)
		require.Empty(t, h.tasks, "orphan reviewer must not publish final coordinator directly")
		source, present := h.entities[replyRun].GetPropertyValue("autoresearch.reply.approved")
		require.True(t, present)
		require.Equal(t, replySource, source)
	})
	for _, tt := range []struct{ name, phase, predicate, forward, publish, kind string }{
		{"approved", "completed", "autoresearch.reply.approved", "14-completed-result-to-origin.json", "15-origin-approved-result-to-coordinator.json", "terminal"},
		{"clarification", "executing", "autoresearch.reply.clarification", "16-clarification-result-to-origin.json", "17-origin-clarification-to-coordinator.json", "rejecting"},
		{"failed", "failed", "autoresearch.reply.failed", "18-failed-result-to-origin.json", "19-origin-failed-to-coordinator.json", "failed"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			h := newReplyHarness(t, tc, tt.phase)
			h.append(message.Triple{Subject: replyRun, Predicate: tt.predicate, Object: replySource})
			forward, publish := replyRule(t, tt.forward), replyRule(t, tt.publish)
			h.evaluate(t, forward, replyRun, 10, false)
			h.evaluate(t, publish, replyOrigin, 20, false)
			requireReplyTask(t, h, replySource, tt.kind, tt.phase)
			h.evaluate(t, forward, replyRun, 10, false)
			h.evaluate(t, publish, replyOrigin, 19, false)
			h.evaluate(t, publish, replyOrigin, 21, false)
			h.restart()
			h.evaluate(t, forward, replyRun, 11, true)
			h.evaluate(t, publish, replyOrigin, 22, true)
			requireReplyTask(t, h, replySource, tt.kind, tt.phase)
		})
	}
}

func TestAutoresearchReplyPhaseAndOriginGuards(t *testing.T) {
	tc := natsclient.NewTestClient(t, natsclient.WithNATSVersion("2.14.4"), natsclient.WithJetStream())
	for _, phase := range []string{"dispatched", "executing", "awaiting_approval", "completed", "failed", "cancelled"} {
		t.Run(phase, func(t *testing.T) {
			for _, tt := range []struct{ file, predicate, phase string }{
				{"14-completed-result-to-origin.json", "autoresearch.reply.approved", "completed"},
				{"16-clarification-result-to-origin.json", "autoresearch.reply.clarification", "executing"},
				{"18-failed-result-to-origin.json", "autoresearch.reply.failed", "failed"},
			} {
				h := newReplyHarness(t, tc, phase)
				h.append(message.Triple{Subject: replyRun, Predicate: tt.predicate, Object: replySource})
				h.evaluate(t, replyRule(t, tt.file), replyRun, 1, false)
				_, present := h.entities[replyOrigin].GetPropertyValue(tt.predicate)
				require.Equal(t, phase == tt.phase, present, tt.file)
			}
		})
	}
	for _, tt := range []struct {
		name             string
		origins, sources []string
	}{
		{"missing origin", nil, []string{replySource}},
		{"foreign authority", []string{"peer.reply-test.agentic-loop.agent.execution." + replyRoot}, []string{replySource}},
		{"wrong instance", []string{"c360.reply-test.agentic-loop.agent.execution." + replyOldRoot}, []string{replySource}},
		{"two origins", []string{replyOrigin, "c360.reply-test.agentic-loop.agent.execution." + replyOldRoot}, []string{replySource}},
		{"missing source", []string{replyOrigin}, nil},
		{"ambiguous source", []string{replyOrigin}, []string{replySource, replyOrigin}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			h := newReplyHarness(t, tc, "completed")
			h.replace(replyRun, "agent.run.origin-entity-id", nil)
			for _, s := range tt.origins {
				h.append(message.Triple{Subject: replyRun, Predicate: "agent.run.origin-entity-id", Object: s})
			}
			for _, s := range tt.sources {
				h.append(message.Triple{Subject: replyRun, Predicate: "autoresearch.reply.approved", Object: s})
			}
			h.evaluate(t, replyRule(t, "14-completed-result-to-origin.json"), replyRun, 1, false)
			_, present := h.entities[replyOrigin].GetPropertyValue("autoresearch.reply.approved")
			require.False(t, present)
		})
	}
}

func TestAutoresearchReplyExistingRunMint(t *testing.T) {
	tc := natsclient.NewTestClient(t, natsclient.WithNATSVersion("2.14.4"), natsclient.WithJetStream())
	for _, phase := range []string{"completed", "failed", "awaiting_approval"} {
		t.Run(phase, func(t *testing.T) {
			h := newReplyHarness(t, tc, phase)
			h.append(message.Triple{Subject: replyOrigin, Predicate: "autoresearch.reply.approved", Object: replySource})
			h.evaluate(t, replyRule(t, "15-origin-approved-result-to-coordinator.json"), replyOrigin, 1, false)
			requireReplyTask(t, h, replySource, "terminal", phase)
		})
	}
	t.Run("Mint refuses different stored origin", func(t *testing.T) {
		h := newReplyHarness(t, tc, "completed")
		h.runs[replyRun].OriginEntityID = "c360.reply-test.agentic-loop.agent.execution." + replyOldRoot
		_, err := agentrun.Mint(t.Context(), h, "c360", "reply-test", replyRoot, replyOrigin)
		require.Error(t, err)
		require.Contains(t, err.Error(), "origin")
		require.Empty(t, h.tasks, "direct Mint has no publication surface; ActionExecutor's documented fallback is separate")
	})
}

func TestAutoresearchIterationCapAboveDefaultActionLimit(t *testing.T) {
	tc := natsclient.NewTestClient(t, natsclient.WithNATSVersion("2.14.4"), natsclient.WithJetStream())
	h := newReplyHarness(t, tc, "executing")
	h.seed(replyRun, map[string]any{
		"autoresearch.run.status": "active", "autoresearch.run.cap": 5, "autoresearch.iteration.pending": "initial",
		"autoresearch.run.command": "task test", "autoresearch.run.surface": "test/", "autoresearch.run.metric-parser": "seconds",
		"autoresearch.baseline.value": 10, "autoresearch.best.value": 7, "autoresearch.best.experiment-id": "baseline",
	})
	def := replyRule(t, "05-iteration-dispatch.json")
	for i := uint64(1); i <= 6; i++ {
		h.replace(replyRun, "autoresearch.iteration.pending", fmt.Sprint(i))
		h.evaluate(t, def, replyRun, i*2, false)
		_, pending := h.entities[replyRun].GetPropertyValue("autoresearch.iteration.pending")
		require.False(t, pending, "pending must clear on iteration %d", i)
		h.evaluate(t, def, replyRun, i*2+1, false)
	}
	require.Len(t, h.tasks, 6, "cap5 produces five proposals and one synthesis")
	for _, task := range h.tasks[:5] {
		require.Equal(t, "autoresearch-propose", task.Role)
	}
	require.Equal(t, "autoresearch-synthesize", h.tasks[5].Role)
	h.replace(replyRun, "autoresearch.iteration.pending", "stale completion")
	h.evaluate(t, def, replyRun, 100, false)
	h.restart()
	h.evaluate(t, def, replyRun, 101, true)
	require.Len(t, h.tasks, 6, "stopped driver stays stopped across stale completion/restart")
}

func TestAutoresearchReplySourceBoundaries(t *testing.T) {
	tc := natsclient.NewTestClient(t, natsclient.WithNATSVersion("2.14.4"), natsclient.WithJetStream())
	for _, tt := range []struct {
		name, file, role, outcome, decision, predicate string
		want                                           bool
	}{
		{"approval before terminal", "08-reviewer-approved-to-coordinator.json", "reviewer-autoresearch", "", "approved", "approved", false},
		{"approved terminal", "08-reviewer-approved-to-coordinator.json", "reviewer-autoresearch", "success", "approved", "approved", true},
		{"rejected review", "08-reviewer-approved-to-coordinator.json", "reviewer-autoresearch", "success", "insufficient", "approved", false},
		{"clarification before terminal", "10b-descended-needs-clarification-replan.json", "autoresearch-propose", "", "needs_clarification", "clarification", false},
		{"descended clarification", "10b-descended-needs-clarification-replan.json", "autoresearch-propose", "success", "needs_clarification", "clarification", true},
		{"voluntary execute clarification", "10b-descended-needs-clarification-replan.json", "autoresearch-execute", "success", "needs_clarification", "clarification", true},
		{"baseline failure", "12-baseline-loop-failed-run-outcome.json", "autoresearch-baseline", "failed", "", "failed", true},
		{"propose failure", "13-loop-failed-run-outcome.json", "autoresearch-propose", "failed", "", "failed", true},
		{"budgeted execute failure", "13-loop-failed-run-outcome.json", "autoresearch-execute", "failed", "", "failed", false},
		{"coordinator failure excluded", "13-loop-failed-run-outcome.json", "coordinator", "failed", "", "failed", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			h := newReplyHarness(t, tc, "executing")
			h.replace(replySource, "agent.loop.role", tt.role)
			h.replace(replySource, "agent.loop.outcome", tt.outcome)
			h.replace(replySource, "coordinator.decision.next-action", tt.decision)
			h.append(message.Triple{Subject: replySource, Predicate: "agent.run.entity-id", Object: replyRun})
			h.evaluate(t, replyRule(t, tt.file), replySource, 1, false)
			value, present := h.entities[replyRun].GetPropertyValue("autoresearch.reply." + tt.predicate)
			require.Equal(t, tt.want, present)
			if tt.want {
				require.Equal(t, replySource, value)
			}
			require.Empty(t, h.tasks, "source roles record facts; native publication belongs to the origin")
		})
	}
	t.Run("only original category coordinator consumes result", func(t *testing.T) {
		for _, tt := range []struct{ role, decision string }{{"autoresearch-propose", "autoresearch"}, {"coordinator", "research"}, {"coordinator", "respond_direct"}} {
			h := newReplyHarness(t, tc, "completed")
			h.replace(replyOrigin, "agent.loop.role", tt.role)
			h.replace(replyOrigin, "coordinator.decision.next-action", tt.decision)
			h.append(message.Triple{Subject: replyOrigin, Predicate: "autoresearch.reply.approved", Object: replySource})
			h.evaluate(t, replyRule(t, "15-origin-approved-result-to-coordinator.json"), replyOrigin, 1, false)
			require.Empty(t, h.tasks)
		}
	})
}

func TestAutoresearchReplyAmbiguousSourceAnchor(t *testing.T) {
	tc := natsclient.NewTestClient(t, natsclient.WithNATSVersion("2.14.4"), natsclient.WithJetStream())
	for _, tt := range []struct{ name, file, role, outcome, decision, anchor, predicate string }{
		{"clarification", "10b-descended-needs-clarification-replan.json", "autoresearch-propose", "success", "needs_clarification", "agent.lineage.run-loop-entity-id", "clarification"},
		{"baseline failure", "12-baseline-loop-failed-run-outcome.json", "autoresearch-baseline", "failed", "", "agent.run.entity-id", "failed"},
		{"descended failure", "13-loop-failed-run-outcome.json", "autoresearch-propose", "failed", "", "agent.lineage.run-loop-entity-id", "failed"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			h := newReplyHarness(t, tc, "executing")
			h.replace(replySource, "agent.loop.role", tt.role)
			h.replace(replySource, "agent.loop.outcome", tt.outcome)
			h.replace(replySource, "coordinator.decision.next-action", tt.decision)
			h.replace(replySource, tt.anchor, replyRun)
			h.append(message.Triple{Subject: replySource, Predicate: tt.anchor, Object: "c360.reply-test.chain.agent.execution." + replyOldRoot})
			h.evaluate(t, replyRule(t, tt.file), replySource, 1, false)
			_, present := h.entities[replyRun].GetPropertyValue("autoresearch.reply." + tt.predicate)
			require.False(t, present, "ambiguous source must not forward a result using the first anchor")
		})
	}
}

func TestAutoresearchReplyAdmitsChildOfTerminalRun(t *testing.T) {
	tc := natsclient.NewTestClient(t, natsclient.WithNATSVersion("2.14.4"), natsclient.WithJetStream())
	for _, tt := range []struct{ phase, file, predicate string }{
		{"completed", "15-origin-approved-result-to-coordinator.json", "autoresearch.reply.approved"},
		{"failed", "19-origin-failed-to-coordinator.json", "autoresearch.reply.failed"},
	} {
		t.Run(tt.phase, func(t *testing.T) {
			h := newReplyHarness(t, tc, tt.phase)
			h.append(message.Triple{Subject: replyOrigin, Predicate: tt.predicate, Object: replySource})
			h.evaluate(t, replyRule(t, tt.file), replyOrigin, 1, false)
			require.Len(t, h.tasks, 1)
			handler := agenticloop.NewMessageHandler(agenticloop.DefaultConfig())
			admitted, err := handler.HandleTask(t.Context(), h.tasks[0])
			require.NoError(t, err)
			require.True(t, admitted.Created)
			var requests []agentic.AgentRequest
			for _, pub := range admitted.PublishedMessages {
				if strings.HasPrefix(pub.Subject, "agent.request") {
					var envelope struct {
						Payload agentic.AgentRequest `json:"payload"`
					}
					require.NoError(t, json.Unmarshal(pub.Data, &envelope))
					requests = append(requests, envelope.Payload)
				}
			}
			require.Len(t, requests, 1)
			loop, err := handler.GetLoop(admitted.LoopID)
			require.NoError(t, err)
			require.Equal(t, replyRoot, loop.RunID)
			require.Equal(t, replyRoot, loop.ParentLoopID)
			require.Equal(t, tt.phase, h.runs[replyRun].PhaseField)
			require.Equal(t, replyOrigin, h.runs[replyRun].OriginEntityID)
			require.Len(t, h.runs, 1)
		})
	}
}
