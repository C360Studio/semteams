package renderopenspec

import (
	"context"
	"strings"
	"testing"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/types"

	"github.com/c360studio/semteams/cmd/semteams/openspec"
)

func platform() types.PlatformMeta { return types.PlatformMeta{Org: "c360", Platform: "ops"} }

const testRunEntity = "c360.ops.agent.chain.execution.loop_test"

// fakeReader returns a fixed triple map for the expected entity id; an unknown
// id returns an empty map (mirrors a graph miss).
type fakeReader struct {
	id      string
	triples map[string]any
	err     error
}

func (f fakeReader) ReadEntity(_ context.Context, id string) (map[string]any, error) {
	if f.err != nil {
		return nil, f.err
	}
	if id != f.id {
		return map[string]any{}, nil
	}
	return f.triples, nil
}

// changeTriples renders a Change to the predicate→object map a run entity would
// carry, plus a couple writer-only predicates (status + a §D6 task field) to
// prove render_openspec ignores them (only markdown-representable content
// reconstructs).
func changeTriples(c *openspec.Change) map[string]any {
	m := map[string]any{}
	for _, f := range c.Facts() {
		m[f.Predicate] = f.Object
	}
	base := openspec.ChangeEntityPrefix(c.Slug)
	m[base+"status"] = "draft"                        // writer-only — ignored by render
	m[base+"task.0.goal"] = "execution-rich, ignored" // §D6 writer-only — ignored
	return m
}

func sampleChange() *openspec.Change {
	return &openspec.Change{
		Slug:     "add-mfa",
		Proposal: &openspec.Proposal{Intent: "Protect accounts.", ScopeIn: []string{"TOTP"}, ScopeOut: []string{}, Approach: "Add a factor."},
		Deltas: []openspec.Delta{{
			Capability: "auth",
			Added: []openspec.Requirement{{
				Name: "TOTP verification", Statement: "The system SHALL require a valid TOTP code.",
				Scenarios: []openspec.Scenario{{Name: "bad code", Steps: []openspec.Step{
					{Keyword: "WHEN", Text: "an incorrect code is submitted"},
					{Keyword: "THEN", Text: "the login is rejected"},
				}}},
			}},
		}},
		Tasks: &openspec.Tasks{Sections: []openspec.TaskSection{{Name: "1. Core", Tasks: []openspec.Task{{Number: "1.1", Text: "add totp"}}}}},
	}
}

func callWith(args map[string]any) agentic.ToolCall {
	return agentic.ToolCall{
		ID: "call-1", Name: ToolName, Arguments: args,
		Metadata: map[string]any{agentic.MetadataKeyRunEntityID: testRunEntity},
	}
}

func TestRenderOpenSpec_HappyPath_AutoDiscoverSlug(t *testing.T) {
	reader := fakeReader{id: testRunEntity, triples: changeTriples(sampleChange())}
	ex := NewExecutor(reader, platform(), nil)

	res, err := ex.Execute(context.Background(), callWith(nil))
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if res.Error != "" {
		t.Fatalf("Execute result error: %s", res.Error)
	}
	for _, want := range []string{
		"# OpenSpec change: add-mfa",
		"<!-- openspec/changes/add-mfa/proposal.md -->",
		"<!-- openspec/changes/add-mfa/specs/auth/spec.md -->",
		"<!-- openspec/changes/add-mfa/tasks.md -->",
		"The system SHALL require a valid TOTP code.",
		"- [ ] 1.1 add totp",
	} {
		if !strings.Contains(res.Content, want) {
			t.Errorf("rendered content missing %q\n--- got ---\n%s", want, res.Content)
		}
	}
	// Writer-only state must NOT leak into the rendered markdown.
	if strings.Contains(res.Content, "execution-rich, ignored") {
		t.Error("render leaked a §D6 writer-only task field into the markdown")
	}
	if res.Metadata["slug"] != "add-mfa" {
		t.Errorf("metadata slug = %v, want add-mfa", res.Metadata["slug"])
	}
}

func TestRenderOpenSpec_ExplicitSlug(t *testing.T) {
	reader := fakeReader{id: testRunEntity, triples: changeTriples(sampleChange())}
	ex := NewExecutor(reader, platform(), nil)
	res, err := ex.Execute(context.Background(), callWith(map[string]any{"slug": "add-mfa"}))
	if err != nil || res.Error != "" {
		t.Fatalf("explicit-slug render failed: err=%v result=%s", err, res.Error)
	}
	if !strings.Contains(res.Content, "# OpenSpec change: add-mfa") {
		t.Error("explicit slug did not render the change")
	}
}

func TestRenderOpenSpec_SlugMismatch(t *testing.T) {
	reader := fakeReader{id: testRunEntity, triples: changeTriples(sampleChange())}
	ex := NewExecutor(reader, platform(), nil)
	res, _ := ex.Execute(context.Background(), callWith(map[string]any{"slug": "nope"}))
	if res.Error == "" || !strings.Contains(res.Error, "no change") {
		t.Errorf("expected a no-such-change error for a wrong slug; got %q", res.Error)
	}
}

func TestRenderOpenSpec_NoChangePresent(t *testing.T) {
	reader := fakeReader{id: testRunEntity, triples: map[string]any{"agent.run.phase": "executing"}}
	ex := NewExecutor(reader, platform(), nil)
	res, _ := ex.Execute(context.Background(), callWith(nil))
	if res.Error == "" || !strings.Contains(res.Error, "nothing to render") {
		t.Errorf("expected a nothing-to-render error on an entity with no change.*; got %q", res.Error)
	}
}

func TestRenderOpenSpec_MultipleChangesNeedsSlug(t *testing.T) {
	tr := changeTriples(sampleChange())
	tr["change.other-change.proposal.intent"] = "another"
	reader := fakeReader{id: testRunEntity, triples: tr}
	ex := NewExecutor(reader, platform(), nil)
	res, _ := ex.Execute(context.Background(), callWith(nil))
	if res.Error == "" || !strings.Contains(res.Error, "pass slug") {
		t.Errorf("expected an ambiguous-slug error with two changes; got %q", res.Error)
	}
}

func TestRenderOpenSpec_NoRunAnchor(t *testing.T) {
	reader := fakeReader{id: testRunEntity, triples: changeTriples(sampleChange())}
	ex := NewExecutor(reader, platform(), nil)
	// No run anchor on the call metadata.
	res, _ := ex.Execute(context.Background(), agentic.ToolCall{ID: "c", Name: ToolName})
	if res.Error == "" || !strings.Contains(res.Error, "no run anchor") {
		t.Errorf("expected a no-run-anchor error; got %q", res.Error)
	}
}
