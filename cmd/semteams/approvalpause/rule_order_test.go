package approvalpause

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"testing"

	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/pkg/lifecycle"
	rulepkg "github.com/c360studio/semstreams/processor/rule"
	"github.com/c360studio/semstreams/processor/rule/expression"
)

// This harness runs the checked-in rules through the frozen framework's actual
// matcher and action executor. Only lifecycle persistence is a deterministic
// fixture; no predicate comparison or rule-condition logic is reimplemented.
type approvalRules struct {
	rulepkg.LifecycleManager
	graph    *durableGraph
	phase    string
	from     string
	source   lifecycle.TransitionSource
	note     string
	pause    rulepkg.Definition
	resume   rulepkg.Definition
	executor *rulepkg.ActionExecutor
}

func newApprovalRules(t *testing.T, g *durableGraph) *approvalRules {
	t.Helper()
	h := &approvalRules{graph: g, phase: "executing"}
	for file, target := range map[string]*rulepkg.Definition{
		"12-executing-to-awaiting-on-approval.json":        &h.pause,
		"13-resume-awaiting-to-executing-on-approval.json": &h.resume,
	} {
		raw, err := os.ReadFile("../../../configs/rules/agent-run/" + file)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(raw, target); err != nil {
			t.Fatal(err)
		}
	}
	h.executor = rulepkg.NewActionExecutor(slog.New(slog.NewTextHandler(io.Discard, nil)))
	h.executor.SetLifecycleManager(h)
	return h
}

func (h *approvalRules) TransitionWith(_ context.Context, workflow, entityID, phase string, source lifecycle.TransitionSource, note string, mutate func(lifecycle.Participant) error) error {
	if workflow != "agent-run" || entityID != replayRunID || mutate != nil {
		return fmt.Errorf("unexpected lifecycle invocation")
	}
	if (h.phase != "executing" || phase != "awaiting_approval") && (h.phase != "awaiting_approval" || phase != "executing") {
		return fmt.Errorf("illegal lifecycle edge %s -> %s", h.phase, phase)
	}
	h.from, h.phase, h.source = h.phase, phase, source
	if note != "" {
		h.note = note
	}
	return nil
}
func (h *approvalRules) entity() *graph.EntityState {
	h.graph.mu.Lock()
	defer h.graph.mu.Unlock()
	triples := append([]message.Triple(nil), h.graph.triples...)
	for predicate, value := range map[string]string{
		"agent.run.phase": h.phase, "agent.run.last-transition-note": h.note,
		"agent.run.last-transition-from": h.from, "agent.run.last-transition-source": string(h.source),
	} {
		triples = append(triples, message.Triple{Subject: replayRunID, Predicate: predicate, Object: value})
	}
	return &graph.EntityState{ID: replayRunID, Triples: triples}
}
func (h *approvalRules) matches(t *testing.T, def rulepkg.Definition) bool {
	t.Helper()
	matched, err := rulepkg.Matches(context.Background(), def, h.entity())
	if err != nil {
		var evaluation *expression.EvaluationError
		if errors.As(err, &evaluation) && evaluation.Message == "required field not found" {
			return false
		}
		t.Fatal(err)
	}
	return matched
}
func (h *approvalRules) execute(t *testing.T, def rulepkg.Definition) {
	t.Helper()
	for _, action := range def.OnEnter {
		if err := h.executor.Execute(context.Background(), action, &rulepkg.ExecutionContext{EntityID: replayRunID, Entity: h.entity()}); err != nil {
			t.Fatal(err)
		}
	}
}
func (h *approvalRules) settle(t *testing.T) {
	t.Helper()
	for range 8 {
		if h.matches(t, h.pause) {
			h.execute(t, h.pause)
			continue
		}
		if h.matches(t, h.resume) {
			h.execute(t, h.resume)
			continue
		}
		return
	}
	t.Fatal("approval rules did not converge")
}
func pendingGate(t *testing.T, p *Pauser, id string) {
	t.Helper()
	if _, err := p.HandlePending(context.Background(), pendingExecution(id)); err != nil {
		t.Fatal(err)
	}
}
func answerGate(t *testing.T, p *Pauser, id string) {
	t.Helper()
	if _, err := p.HandleResponse(context.Background(), answeredExecution(id)); err != nil {
		t.Fatal(err)
	}
}

func TestApprovalRulesAnswerBeforePendingDoesNotBlockNextGate(t *testing.T) {
	g := &durableGraph{}
	p := NewPauser(g, g, testOrg, testPlatform)
	h := newApprovalRules(t, g)
	answerGate(t, p, "a")
	pendingGate(t, p, "a")
	h.settle(t)
	if h.phase != "executing" {
		t.Fatalf("already answered A phase=%s", h.phase)
	}
	p = NewPauser(g, g, testOrg, testPlatform)
	pendingGate(t, p, "b")
	h.settle(t)
	if h.phase != "awaiting_approval" {
		t.Fatalf("unanswered B did not pause: phase=%s", h.phase)
	}
}
func TestApprovalRulesFailedPendingThenAnswerDoesNotBlockNextGate(t *testing.T) {
	g := &durableGraph{writeErr: errors.New("pending write failed")}
	p := NewPauser(g, g, testOrg, testPlatform)
	h := newApprovalRules(t, g)
	if _, err := p.HandlePending(context.Background(), pendingExecution("a")); err == nil {
		t.Fatal("pending write unexpectedly succeeded")
	}
	g.writeErr = nil
	answerGate(t, p, "a")
	h.settle(t)
	pendingGate(t, p, "b")
	h.settle(t)
	if h.phase != "awaiting_approval" {
		t.Fatalf("failed A write blocked B: phase=%s", h.phase)
	}
}
func TestApprovalRulesLateFirstAnswerDoesNotResumeDifferentGate(t *testing.T) {
	g := &durableGraph{}
	p := NewPauser(g, g, testOrg, testPlatform)
	h := newApprovalRules(t, g)
	pendingGate(t, p, "b")
	h.settle(t)
	if h.phase != "awaiting_approval" {
		t.Fatal("B did not pause")
	}
	answerGate(t, p, "a")
	h.settle(t)
	if h.phase != "awaiting_approval" {
		t.Fatalf("late A resumed B: phase=%s", h.phase)
	}
	answerGate(t, p, "b")
	h.settle(t)
	if h.phase != "executing" {
		t.Fatalf("B answer did not resume: phase=%s", h.phase)
	}
}
func TestApprovalRulesAnswerBeforePauseTransitionDoesNotStrandRun(t *testing.T) {
	g := &durableGraph{}
	p := NewPauser(g, g, testOrg, testPlatform)
	h := newApprovalRules(t, g)
	pendingGate(t, p, "a")
	if !h.matches(t, h.pause) {
		t.Fatal("pending A should select pause")
	}
	answerGate(t, p, "a")
	h.execute(t, h.pause) // Already selected before the answer reached graph.
	h.settle(t)
	if h.phase != "executing" {
		t.Fatalf("early answer stranded run: phase=%s", h.phase)
	}
	pendingGate(t, p, "b")
	h.settle(t)
	if h.phase != "awaiting_approval" {
		t.Fatalf("B did not pause after early A: phase=%s", h.phase)
	}
}
func TestApprovalRulesNextGateDuringSelectedResumeSurvives(t *testing.T) {
	g := &durableGraph{}
	p := NewPauser(g, g, testOrg, testPlatform)
	h := newApprovalRules(t, g)
	pendingGate(t, p, "a")
	h.settle(t)
	answerGate(t, p, "a")
	if !h.matches(t, h.resume) {
		t.Fatal("A answer should select resume")
	}
	pendingGate(t, p, "b")
	h.execute(t, h.resume) // Resume was selected from the earlier A-only revision.
	h.settle(t)
	if h.phase != "awaiting_approval" {
		t.Fatalf("selected A resume lost B: phase=%s", h.phase)
	}
	answerGate(t, p, "b")
	h.settle(t)
	if h.phase != "executing" {
		t.Fatalf("B did not resume: phase=%s", h.phase)
	}
}
func TestApprovalRulesDuplicateFactsAndOtherPauseCauses(t *testing.T) {
	for _, cause := range []string{"clarification", "operator", "different_from"} {
		t.Run(cause, func(t *testing.T) {
			g := &durableGraph{}
			p := NewPauser(g, g, testOrg, testPlatform)
			h := newApprovalRules(t, g)
			pendingGate(t, p, "a")
			pendingGate(t, p, "a")
			h.settle(t)
			answerGate(t, p, "a")
			answerGate(t, p, "a")
			h.settle(t)
			values, _ := g.ReadPredicateValues(context.Background(), replayRunID, MarkerApprovalPending)
			if len(values) != 1 {
				t.Fatalf("duplicate delivery inflated pending set: %v", values)
			}
			h.phase = "awaiting_approval"
			h.from = "executing"
			h.note = "tool approval pending"
			if cause == "operator" {
				h.source = lifecycle.TransitionSourceOperator
			} else if cause == "different_from" {
				h.source = lifecycle.TransitionSourceRule
				h.from = "dispatched"
			} else {
				h.source = lifecycle.TransitionSourceRule
				if err := g.Append(context.Background(), []message.Triple{{Subject: replayRunID, Predicate: "agent.run.clarification-pending", Object: replayLoopID}}); err != nil {
					t.Fatal(err)
				}
			}
			h.settle(t)
			if h.phase != "awaiting_approval" {
				t.Fatalf("approval history resumed %s pause", cause)
			}
		})
	}
}

func TestApprovalRulesTerminalHistoryNeverResumes(t *testing.T) {
	for _, phase := range []string{"completed", "failed", "cancelled"} {
		t.Run(phase, func(t *testing.T) {
			g := &durableGraph{}
			p := NewPauser(g, g, testOrg, testPlatform)
			h := newApprovalRules(t, g)
			pendingGate(t, p, "a")
			h.settle(t)
			h.phase = phase
			answerGate(t, p, "a")
			pendingGate(t, p, "b")
			answerGate(t, p, "a")
			h.settle(t)
			if h.phase != phase {
				t.Fatalf("late approval changed terminal %s to %s", phase, h.phase)
			}
		})
	}
}
