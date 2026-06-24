package emitchange

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/types"
)

func platform() types.PlatformMeta { return types.PlatformMeta{Org: "c360", Platform: "ops"} }

const testRunLoopID = "run-loop-1234"

// fakePub records the stamped triples.
type fakePub struct {
	mu      sync.Mutex
	triples []message.Triple
}

func (f *fakePub) AddTriple(_ context.Context, t message.Triple) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.triples = append(f.triples, t)
	return nil
}

func (f *fakePub) AddTriplesBatch(_ context.Context, ts []message.Triple) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.triples = append(f.triples, ts...)
	return nil
}

func (f *fakePub) byPredicate() map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	m := make(map[string]any, len(f.triples))
	for _, t := range f.triples {
		m[t.Predicate] = t.Object
	}
	return m
}

// validPayload is a minimal compliant change used as the happy-path base; tests
// mutate a copy to exercise each rejection.
func validPayload() map[string]any {
	return map[string]any{
		"slug": "add-mfa",
		"proposal": map[string]any{
			"intent": "Mitigate password-only compromise.", "approach": "Add a TOTP step.",
			"scope_in": []string{"TOTP"}, "scope_out": []string{"WebAuthn"},
		},
		"design": map[string]any{
			"technical_approach": "Layer TOTP after password verification.",
			"decisions": []map[string]any{{
				"name": "TOTP over SMS", "body": "Avoid SMS gateway and SIM-swap dependency.",
			}},
			"data_flow": "password -> totp -> session",
			"file_changes": []map[string]any{{
				"path": "auth/totp.go", "kind": "new",
			}},
		},
		"deltas": []map[string]any{{
			"capability": "auth",
			"added": []map[string]any{{
				"name": "Multi-Factor Authentication", "statement": "The system SHALL require a second factor.",
				"scenarios": []map[string]any{{"name": "TOTP challenge", "steps": []map[string]any{
					{"kw": "WHEN", "text": "a valid password is submitted"}, {"kw": "THEN", "text": "a TOTP code is required"},
				}}},
			}},
		}},
		"tasks": []map[string]any{{
			"section": "1. TOTP", "number": "1.1", "text": "Generate secrets",
			"goal": "Per-user TOTP secrets", "target_files": []string{"auth/totp.go"},
			"test_command": "go test ./auth/...", "assumptions": []string{}, "non_goals": []string{},
			// Author writes the human requirement NAME; emit_change normalizes it
			// to the "<cap>/<rid>" key form (asserted in TestEmitChange_HappyPath).
			"requirement_ref": "auth/Multi-Factor Authentication",
		}},
		"acceptance_command": "go test ./...",
	}
}

func callWith(args map[string]any) agentic.ToolCall {
	return agentic.ToolCall{
		ID: "call-1", Name: ToolName, Arguments: args,
		Metadata: map[string]any{
			agentic.MetadataKeyRelatedLoops: map[string]any{runLoopIDRoleKey: testRunLoopID},
		},
	}
}

func TestEmitChange_HappyPath(t *testing.T) {
	pub := &fakePub{}
	ex := NewExecutor(pub, platform(), nil)
	res, err := ex.Execute(context.Background(), callWith(validPayload()))
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if res.Error != "" {
		t.Fatalf("unexpected tool error: %s", res.Error)
	}

	wantSubject, err := agentic.TryChainExecutionEntityID(platform().Org, platform().Platform, testRunLoopID)
	if err != nil {
		t.Fatalf("derive subject: %v", err)
	}
	for _, tr := range pub.triples {
		if tr.Subject != wantSubject {
			t.Fatalf("triple %q stamped on %q, want run entity %q", tr.Predicate, tr.Subject, wantSubject)
		}
	}

	m := pub.byPredicate()
	wantStr := map[string]string{
		// writer half
		"change.add-mfa.status":             "draft",
		"change.add-mfa.acceptance_command": "go test ./...",
		// content half (via openspec.Change.Facts)
		"change.add-mfa.proposal.intent":                                  "Mitigate password-only compromise.",
		"change.add-mfa.proposal.scope_in":                                `["TOTP"]`,
		"change.add-mfa.proposal.scope_out":                               `["WebAuthn"]`,
		"change.add-mfa.proposal.approach":                                "Add a TOTP step.",
		"change.add-mfa.design.technical_approach":                        "Layer TOTP after password verification.",
		"change.add-mfa.design.decisions":                                 `[{"name":"TOTP over SMS","body":"Avoid SMS gateway and SIM-swap dependency."}]`,
		"change.add-mfa.design.data_flow":                                 "password -> totp -> session",
		"change.add-mfa.design.file_changes":                              `[{"path":"auth/totp.go","kind":"new"}]`,
		"change.add-mfa.delta.auth.multi-factor-authentication.op":        "added",
		"change.add-mfa.delta.auth.multi-factor-authentication.statement": "The system SHALL require a second factor.",
		"change.add-mfa.task.0.text":                                      "Generate secrets",      // content (slice 5)
		"change.add-mfa.task.0.section":                                   "1. TOTP",               // content
		"change.add-mfa.task.0.number":                                    "1.1",                   // content
		"change.add-mfa.task.0.done":                                      "false",                 // content
		"change.add-mfa.task.0.goal":                                      "Per-user TOTP secrets", // writer (§D6)
		"change.add-mfa.task.0.test_command":                              "go test ./auth/...",    // writer
		"change.add-mfa.task.0.requirement_ref":                           "auth/multi-factor-authentication",
		"change.add-mfa.task.0.target_files":                              `["auth/totp.go"]`,
		"change.add-mfa.task.0.assumptions":                               `[]`,
		"change.add-mfa.task.0.non_goals":                                 `[]`,
	}
	for k, v := range wantStr {
		if got, _ := m[k].(string); got != v {
			t.Errorf("predicate %q = %q, want %q", k, got, v)
		}
	}
	// The content task index (text/section) and writer task index (goal) align on 0.
	if _, ok := m["change.add-mfa.task.0.text"]; !ok {
		t.Error("content task fact missing at index 0")
	}
	if _, ok := m["change.add-mfa.task.0.goal"]; !ok {
		t.Error("writer task fact missing at index 0 (index misalignment)")
	}
	// generated_at is stamped and RFC3339Nano-parseable.
	gen, _ := m["change.add-mfa.generated_at"].(string)
	if _, err := time.Parse(time.RFC3339Nano, gen); err != nil {
		t.Errorf("generated_at %q is not RFC3339Nano: %v", gen, err)
	}
}

// TestEmitChange_ExpectedOutcomeOptional pins that expected_outcome is omitted
// when empty and stamped when present (the only optional §D6 task field).
func TestEmitChange_ExpectedOutcomeOptional(t *testing.T) {
	pub := &fakePub{}
	ex := NewExecutor(pub, platform(), nil)
	if _, err := ex.Execute(context.Background(), callWith(validPayload())); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if _, ok := pub.byPredicate()["change.add-mfa.task.0.expected_outcome"]; ok {
		t.Error("expected_outcome must be omitted when empty")
	}

	pub2 := &fakePub{}
	ex2 := NewExecutor(pub2, platform(), nil)
	args := validPayload()
	args["tasks"].([]map[string]any)[0]["expected_outcome"] = "secrets persisted"
	if _, err := ex2.Execute(context.Background(), callWith(args)); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got, _ := pub2.byPredicate()["change.add-mfa.task.0.expected_outcome"].(string); got != "secrets persisted" {
		t.Errorf("expected_outcome = %q, want %q", got, "secrets persisted")
	}
}

func TestEmitChange_TaskDonePreserved(t *testing.T) {
	pub := &fakePub{}
	ex := NewExecutor(pub, platform(), nil)
	args := validPayload()
	args["tasks"].([]map[string]any)[0]["done"] = true
	if _, err := ex.Execute(context.Background(), callWith(args)); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got, _ := pub.byPredicate()["change.add-mfa.task.0.done"].(string); got != "true" {
		t.Errorf("done = %q, want true", got)
	}
}

// TestEmitChange_RemovedOnlyDelta covers a delta carrying only a REMOVED
// requirement (no added/modified) — a valid change that deprecates behavior.
func TestEmitChange_RemovedOnlyDelta(t *testing.T) {
	pub := &fakePub{}
	ex := NewExecutor(pub, platform(), nil)
	args := validPayload()
	args["deltas"] = []map[string]any{{
		"capability": "auth",
		"removed":    []map[string]any{{"name": "Remember Me", "rationale": "conflicts with MFA"}},
	}}
	// The task's requirement_ref points at a now-absent requirement; drop it.
	delete(args["tasks"].([]map[string]any)[0], "requirement_ref")
	if _, err := ex.Execute(context.Background(), callWith(args)); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	m := pub.byPredicate()
	if got, _ := m["change.add-mfa.delta.auth.remember-me.op"].(string); got != "removed" {
		t.Errorf("removed op = %q, want removed", got)
	}
	if got, _ := m["change.add-mfa.delta.auth.remember-me.rationale"].(string); got != "conflicts with MFA" {
		t.Errorf("removed rationale = %q", got)
	}
}

func TestEmitChange_TaskIndexAlignmentAcrossSections(t *testing.T) {
	pub := &fakePub{}
	ex := NewExecutor(pub, platform(), nil)
	args := validPayload()
	args["tasks"] = []map[string]any{
		{"section": "A", "text": "t0", "goal": "g0", "target_files": []string{"a"}, "test_command": "c", "assumptions": []string{}, "non_goals": []string{}},
		{"section": "A", "text": "t1", "goal": "g1", "target_files": []string{"a"}, "test_command": "c", "assumptions": []string{}, "non_goals": []string{}},
		{"section": "B", "text": "t2", "goal": "g2", "target_files": []string{"a"}, "test_command": "c", "assumptions": []string{}, "non_goals": []string{}},
	}
	if _, err := ex.Execute(context.Background(), callWith(args)); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	m := pub.byPredicate()
	// For each index, the content text and the writer goal must come from the same task.
	for i, want := range []struct{ text, goal, section string }{{"t0", "g0", "A"}, {"t1", "g1", "A"}, {"t2", "g2", "B"}} {
		p := "change.add-mfa.task." + string(rune('0'+i)) + "."
		if got, _ := m[p+"text"].(string); got != want.text {
			t.Errorf("task %d text = %q, want %q", i, got, want.text)
		}
		if got, _ := m[p+"goal"].(string); got != want.goal {
			t.Errorf("task %d goal = %q, want %q", i, got, want.goal)
		}
		if got, _ := m[p+"section"].(string); got != want.section {
			t.Errorf("task %d section = %q, want %q", i, got, want.section)
		}
	}
}

func TestEmitChange_Rejections(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(map[string]any)
		errSub string
	}{
		{"bad slug", func(a map[string]any) { a["slug"] = "Add MFA" }, "kebab"},
		{"missing proposal", func(a map[string]any) { delete(a, "proposal") }, "proposal is required"},
		{"missing acceptance_command", func(a map[string]any) { delete(a, "acceptance_command") }, "acceptance_command is required"},
		{"no deltas", func(a map[string]any) { a["deltas"] = []map[string]any{} }, "at least one"},
		{
			name: "statement without SHALL-class keyword",
			mutate: func(a map[string]any) {
				a["deltas"].([]map[string]any)[0]["added"].([]map[string]any)[0]["statement"] = "The system requires a second factor."
			},
			errSub: "SHALL/MUST/SHOULD/MAY",
		},
		{
			name: "scenario missing THEN",
			mutate: func(a map[string]any) {
				d := a["deltas"].([]map[string]any)[0]["added"].([]map[string]any)[0]
				d["scenarios"] = []map[string]any{{"name": "x", "steps": []map[string]any{{"kw": "WHEN", "text": "a"}, {"kw": "AND", "text": "b"}}}}
			},
			errSub: "WHEN and a THEN",
		},
		{
			name:   "task missing target_files",
			mutate: func(a map[string]any) { a["tasks"].([]map[string]any)[0]["target_files"] = []string{} },
			errSub: "target_files",
		},
		{
			name: "non-contiguous task sections",
			mutate: func(a map[string]any) {
				base := func(sec, text string) map[string]any {
					return map[string]any{"section": sec, "text": text, "goal": "g", "target_files": []string{"a"}, "test_command": "c", "assumptions": []string{}, "non_goals": []string{}}
				}
				a["tasks"] = []map[string]any{base("A", "t0"), base("B", "t1"), base("A", "t2")}
			},
			errSub: "non-contiguous",
		},
		{
			name: "modified requirement without previously",
			mutate: func(a map[string]any) {
				a["deltas"].([]map[string]any)[0]["modified"] = []map[string]any{{
					"name": "R", "statement": "The system SHALL x.",
					"scenarios": []map[string]any{{"name": "s", "steps": []map[string]any{{"kw": "WHEN", "text": "a"}, {"kw": "THEN", "text": "b"}}}},
				}}
			},
			errSub: "previously is required",
		},
		{
			name:   "requirement_ref points at an unknown capability",
			mutate: func(a map[string]any) { a["tasks"].([]map[string]any)[0]["requirement_ref"] = "billing/Whatever" },
			errSub: "no delta for capability",
		},
		{
			name:   "requirement_ref points at an unknown requirement",
			mutate: func(a map[string]any) { a["tasks"].([]map[string]any)[0]["requirement_ref"] = "auth/Nonexistent" },
			errSub: "no added/modified requirement",
		},
		{
			name: "two requirements in one capability slugify to the same rid",
			mutate: func(a map[string]any) {
				added := a["deltas"].([]map[string]any)[0]["added"].([]map[string]any)
				a["deltas"].([]map[string]any)[0]["added"] = append(added, map[string]any{
					"name": "multi factor authentication", "statement": "The system SHALL also do X.",
					"scenarios": []map[string]any{{"name": "s", "steps": []map[string]any{{"kw": "WHEN", "text": "a"}, {"kw": "THEN", "text": "b"}}}},
				})
			},
			errSub: "must be distinct within a capability",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pub := &fakePub{}
			ex := NewExecutor(pub, platform(), nil)
			args := validPayload()
			tt.mutate(args)
			res, err := ex.Execute(context.Background(), callWith(args))
			if err != nil {
				t.Fatalf("Execute returned a transport error (want tool error): %v", err)
			}
			if res.Error == "" {
				t.Fatalf("expected rejection, got success; stamped %d triples", len(pub.triples))
			}
			if !strings.Contains(res.Error, tt.errSub) {
				t.Errorf("error %q does not contain %q", res.Error, tt.errSub)
			}
			if len(pub.triples) != 0 {
				t.Errorf("rejection must stamp no triples, got %d", len(pub.triples))
			}
		})
	}
}

func TestEmitChange_MissingRunAnchor(t *testing.T) {
	pub := &fakePub{}
	ex := NewExecutor(pub, platform(), nil)
	call := agentic.ToolCall{ID: "c", Name: ToolName, Arguments: validPayload()} // no related_loops
	res, _ := ex.Execute(context.Background(), call)
	if res.Error == "" || !strings.Contains(res.Error, "related_loops") {
		t.Fatalf("want a related_loops error, got %q", res.Error)
	}
}

func TestEmitChange_UnknownToolName(t *testing.T) {
	pub := &fakePub{}
	ex := NewExecutor(pub, platform(), nil)
	res, _ := ex.Execute(context.Background(), agentic.ToolCall{ID: "c", Name: "nope"})
	if res.ErrorKind != agentic.ToolErrorNotFound {
		t.Fatalf("want ToolErrorNotFound, got %q (%s)", res.ErrorKind, res.Error)
	}
}

// sanity: the schema declares the tool and the OpenSpec + §D6 payload fields.
func TestEmitChange_SchemaShape(t *testing.T) {
	defs := (&Executor{}).ListTools()
	if len(defs) != 1 || defs[0].Name != ToolName {
		t.Fatalf("want one %q tool, got %+v", ToolName, defs)
	}
	raw, _ := json.Marshal(defs[0].Parameters)
	for _, req := range []string{
		"slug", "proposal", "deltas", "tasks", "acceptance_command",
		"intent", "scope_in", "scope_out", "approach",
		"technical_approach", "decisions", "data_flow", "file_changes",
		"capability", "added", "modified", "removed", "statement", "scenarios", "previously", "rationale",
		"section", "number", "text", "done",
		"goal", "target_files", "test_command", "assumptions", "non_goals", "expected_outcome", "requirement_ref",
	} {
		if !strings.Contains(string(raw), `"`+req+`"`) {
			t.Errorf("schema missing field %q", req)
		}
	}
}

func TestEmitChange_SchemaHasNoNullRequiredLists(t *testing.T) {
	defs := (&Executor{}).ListTools()
	if len(defs) != 1 {
		t.Fatalf("want one tool, got %d", len(defs))
	}

	var decoded any
	raw, err := json.Marshal(defs[0].Parameters)
	if err != nil {
		t.Fatalf("marshal schema: %v", err)
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("decode schema: %v", err)
	}

	assertNoNullRequired(t, "$", decoded)
}

func assertNoNullRequired(t *testing.T, path string, v any) {
	t.Helper()
	switch x := v.(type) {
	case map[string]any:
		for k, child := range x {
			childPath := path + "." + k
			if k == "required" && child == nil {
				t.Fatalf("%s is null; Gemini's OpenAI-compatible endpoint rejects required:null", childPath)
			}
			assertNoNullRequired(t, childPath, child)
		}
	case []any:
		for _, child := range x {
			assertNoNullRequired(t, path+"[]", child)
		}
	}
}
