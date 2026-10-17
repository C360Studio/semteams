package researchdecide

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/message"
	agentictools "github.com/c360studio/semstreams/processor/agentic-tools"
	"github.com/c360studio/semstreams/types"
)

type fakeTriplePublisher struct {
	mu      sync.Mutex
	triples []message.Triple
}

func (f *fakeTriplePublisher) Create(_ context.Context, _ string, _ message.Type, triples []message.Triple) error {
	return f.Append(context.Background(), triples)
}

func (f *fakeTriplePublisher) Append(_ context.Context, triples []message.Triple) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.triples = append(f.triples, triples...)
	return nil
}

func (f *fakeTriplePublisher) snapshot() []message.Triple {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]message.Triple, len(f.triples))
	copy(out, f.triples)
	return out
}

func newTestExecutor(restricted []string) (*Executor, *fakeTriplePublisher) {
	tp := &fakeTriplePublisher{}
	inner := agentictools.NewDecideExecutor(tp, types.PlatformMeta{Org: "c360", Platform: "test"}, restricted)
	return NewExecutor(inner, restricted, nil), tp
}

func makeSubtopics(n int) []any {
	out := make([]any, n)
	for i := range out {
		out[i] = fmt.Sprintf("subtopic-%d: boundary investigation", i+1)
	}
	return out
}

func TestListTools_AdvertisesSubtopicsBounds(t *testing.T) {
	exec, _ := newTestExecutor(nil)
	defs := exec.ListTools()
	if len(defs) != 1 {
		t.Fatalf("ListTools len = %d, want 1", len(defs))
	}
	props, ok := defs[0].Parameters["properties"].(map[string]any)
	if !ok {
		t.Fatalf("missing properties map: %#v", defs[0].Parameters)
	}
	subtopics, ok := props["subtopics"].(map[string]any)
	if !ok {
		t.Fatalf("missing subtopics schema: %#v", props["subtopics"])
	}
	if subtopics["minItems"] != 1 {
		t.Errorf("subtopics.minItems = %v, want 1", subtopics["minItems"])
	}
	if subtopics["maxItems"] != 4 {
		t.Errorf("subtopics.maxItems = %v, want 4", subtopics["maxItems"])
	}
}

func TestExecute_GatherValidSubtopicCounts(t *testing.T) {
	for _, count := range []int{1, 2, 3, 4} {
		t.Run(fmt.Sprintf("N=%d", count), func(t *testing.T) {
			exec, tp := newTestExecutor(nil)
			call := agentic.ToolCall{
				ID:     "call-valid",
				Name:   ToolName,
				LoopID: "11111111-1111-1111-1111-111111111111",
				Arguments: map[string]any{
					"action":    "gather",
					"reason":    "plan ready",
					"subtopics": makeSubtopics(count),
				},
				Metadata: map[string]any{
					agentic.MetadataKeyAgentRole:             "researcher-research-plan",
					agentic.MetadataKeyDecideActionAllowlist: []string{"gather", "emit", "needs_clarification"},
				},
			}
			res, err := exec.Execute(context.Background(), call)
			if err != nil {
				t.Fatalf("Execute(N=%d) err: %v", count, err)
			}
			if res.Error != "" {
				t.Fatalf("Execute(N=%d) unexpected error: %s", count, res.Error)
			}
			if !res.StopLoop {
				t.Errorf("Execute(N=%d) StopLoop = false, want true", count)
			}
			if len(tp.snapshot()) != 3 {
				t.Errorf("Execute(N=%d) published %d triples, want 3 (next-action, reason, subtopics)", count, len(tp.snapshot()))
			}
		})
	}
}

func TestExecute_GatherInvalidSubtopicCounts_RejectedPreMutation(t *testing.T) {
	cases := []struct {
		name      string
		subtopics any
		hasField  bool
	}{
		{name: "missing_subtopics", hasField: false},
		{name: "nil_subtopics", subtopics: nil, hasField: true},
		{name: "empty_subtopics_N0", subtopics: []any{}, hasField: true},
		{name: "too_many_N5", subtopics: makeSubtopics(5), hasField: true},
		{name: "too_many_N15", subtopics: makeSubtopics(15), hasField: true},
		{name: "blank_item", subtopics: []any{"valid subtopic", "   "}, hasField: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			exec, tp := newTestExecutor(nil)
			args := map[string]any{
				"action": "gather",
				"reason": "plan ready",
			}
			if tc.hasField {
				args["subtopics"] = tc.subtopics
			}
			call := agentic.ToolCall{
				ID:        "call-invalid",
				Name:      ToolName,
				LoopID:    "11111111-1111-1111-1111-111111111111",
				Arguments: args,
				Metadata: map[string]any{
					agentic.MetadataKeyAgentRole:             "researcher-research-plan",
					agentic.MetadataKeyDecideActionAllowlist: []string{"gather", "emit", "needs_clarification"},
				},
			}
			res, err := exec.Execute(context.Background(), call)
			if err != nil {
				t.Fatalf("Execute(%s) err = %v, want nil", tc.name, err)
			}
			if res.ErrorKind != agentic.ToolErrorInvalidArgs {
				t.Errorf("Execute(%s) ErrorKind = %v, want %v", tc.name, res.ErrorKind, agentic.ToolErrorInvalidArgs)
			}
			if res.StopLoop {
				t.Errorf("Execute(%s) StopLoop = true, want false so planner can self-correct", tc.name)
			}
			if !strings.Contains(res.Error, "subtopics") {
				t.Errorf("Execute(%s) Error = %q, want mention of subtopics", tc.name, res.Error)
			}
			if got := len(tp.snapshot()); got != 0 {
				t.Errorf("Execute(%s) published %d triples, want 0 (pre-mutation rejection)", tc.name, got)
			}
		})
	}
}

func TestExecute_SameLoopCorrection_RejectThenAccept(t *testing.T) {
	exec, tp := newTestExecutor(nil)
	loopID := "22222222-2222-2222-2222-222222222222"

	// Turn 1: planner attempts N=5 subtopics -> rejected pre-mutation, loop stays alive.
	badCall := agentic.ToolCall{
		ID:     "call-turn-1",
		Name:   ToolName,
		LoopID: loopID,
		Arguments: map[string]any{
			"action":    "gather",
			"reason":    "over-decomposed plan",
			"subtopics": makeSubtopics(5),
		},
	}
	res1, err := exec.Execute(context.Background(), badCall)
	if err != nil {
		t.Fatalf("turn 1 err: %v", err)
	}
	if res1.ErrorKind != agentic.ToolErrorInvalidArgs || res1.StopLoop {
		t.Fatalf("turn 1 = %+v, want InvalidArgs with StopLoop=false", res1)
	}
	if len(tp.snapshot()) != 0 {
		t.Fatalf("turn 1 published %d triples, want 0", len(tp.snapshot()))
	}

	// Turn 2: planner consolidates to N=4 subtopics -> accepted, triples stamped, loop stops.
	goodCall := agentic.ToolCall{
		ID:     "call-turn-2",
		Name:   ToolName,
		LoopID: loopID,
		Arguments: map[string]any{
			"action":    "gather",
			"reason":    "consolidated 4-subtopic plan",
			"subtopics": makeSubtopics(4),
		},
	}
	res2, err := exec.Execute(context.Background(), goodCall)
	if err != nil {
		t.Fatalf("turn 2 err: %v", err)
	}
	if res2.Error != "" || !res2.StopLoop {
		t.Fatalf("turn 2 = %+v, want success with StopLoop=true", res2)
	}
	if len(tp.snapshot()) != 3 {
		t.Fatalf("turn 2 published %d triples, want 3", len(tp.snapshot()))
	}
}

func TestExecute_NonGatherWithoutSubtopics_DelegatesUnchanged(t *testing.T) {
	exec, tp := newTestExecutor(nil)
	call := agentic.ToolCall{
		ID:     "call-coord",
		Name:   ToolName,
		LoopID: "33333333-3333-3333-3333-333333333333",
		Arguments: map[string]any{
			"action": "research",
			"reason": "delegate research question",
		},
	}
	res, err := exec.Execute(context.Background(), call)
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if res.Error != "" || !res.StopLoop {
		t.Fatalf("Execute = %+v, want success with StopLoop=true", res)
	}
	if len(tp.snapshot()) != 2 {
		t.Errorf("published %d triples, want 2 (next-action, reason)", len(tp.snapshot()))
	}
}

func TestExecute_OffAllowlistAndRestricted_DelegateToUpstream(t *testing.T) {
	exec, tp := newTestExecutor([]string{"ask_user"})

	offAllowlist := agentic.ToolCall{
		ID:     "call-off",
		Name:   ToolName,
		LoopID: "44444444-4444-4444-4444-444444444444",
		Arguments: map[string]any{
			"action": "not_allowed",
			"reason": "bad action",
		},
		Metadata: map[string]any{
			agentic.MetadataKeyDecideActionAllowlist: []string{"gather", "needs_clarification"},
		},
	}
	res1, err := exec.Execute(context.Background(), offAllowlist)
	if err != nil {
		t.Fatalf("offAllowlist err: %v", err)
	}
	if res1.ErrorKind != agentic.ToolErrorInvalidArgs || !strings.Contains(res1.Error, "not in the allowed set") {
		t.Errorf("offAllowlist result = %+v, want upstream allowlist error", res1)
	}

	restrictedCall := agentic.ToolCall{
		ID:     "call-restricted",
		Name:   ToolName,
		LoopID: "44444444-4444-4444-4444-444444444444",
		Arguments: map[string]any{
			"action": "ask_user",
			"reason": "need input",
		},
	}
	res2, err := exec.Execute(context.Background(), restrictedCall)
	if err != nil {
		t.Fatalf("restrictedCall err: %v", err)
	}
	if res2.ErrorKind != agentic.ToolErrorInvalidArgs || !strings.Contains(res2.Error, "restricted by deployment policy") {
		t.Errorf("restrictedCall result = %+v, want upstream restriction error", res2)
	}
	if len(tp.snapshot()) != 0 {
		t.Errorf("published %d triples, want 0", len(tp.snapshot()))
	}
}
