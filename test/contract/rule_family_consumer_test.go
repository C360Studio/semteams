package contract

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/payloadregistry"
	"github.com/c360studio/semstreams/processor/rule"
	"github.com/c360studio/semstreams/types"

	"github.com/c360studio/semteams/internal/runtimecatalog"
)

const ruleFamilyFixtureDir = "../fixtures/rule-family-consumer"

// Expected JSON is decoded only into test-local primitive types. Production
// tags or custom unmarshallers must never transform both sides of the oracle.
type ruleFamilyExpected struct {
	Subject        string                    `json:"subject"`
	Role           string                    `json:"role"`
	Model          string                    `json:"model"`
	ToolChoice     *ruleFamilyExpectedChoice `json:"tool_choice"`
	ResponseFormat *ruleFamilyExpectedFormat `json:"response_format"`
}

type ruleFamilyExpectedChoice struct {
	Mode         string `json:"mode"`
	FunctionName string `json:"function_name,omitempty"`
}

type ruleFamilyExpectedFormat struct {
	Type   string         `json:"type"`
	Name   string         `json:"name"`
	Strict bool           `json:"strict"`
	Schema map[string]any `json:"schema"`
}

func ruleFamilyActual(subject, role, model string, choice *agentic.ToolChoice, format *agentic.ResponseFormat) ruleFamilyExpected {
	actual := ruleFamilyExpected{Subject: subject, Role: role, Model: model}
	if choice != nil {
		actual.ToolChoice = &ruleFamilyExpectedChoice{Mode: choice.Mode, FunctionName: choice.FunctionName}
	}
	if format != nil {
		actual.ResponseFormat = &ruleFamilyExpectedFormat{
			Type: format.Type, Name: format.Name, Strict: format.Strict, Schema: format.Schema,
		}
	}
	return actual
}

func ruleFamilyReadJSON(t *testing.T, path string, dst any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, dst); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
}

func ruleFamilyGoldens(t *testing.T) map[string]ruleFamilyExpected {
	t.Helper()
	var expected map[string]ruleFamilyExpected
	ruleFamilyReadJSON(t, filepath.Join(ruleFamilyFixtureDir, "expected.json"), &expected)
	if len(expected) != 3 {
		t.Fatalf("expected exactly the three reviewed cases, got %d", len(expected))
	}
	return expected
}

func ruleFamilyAction(t *testing.T, name string) rule.Action {
	t.Helper()
	// Definition and its []Action are the same public JSON shapes used by the
	// frozen rule file loader. This does not evaluate the rule's conditions.
	var definition rule.Definition
	ruleFamilyReadJSON(t, filepath.Join(ruleFamilyFixtureDir, name), &definition)
	if len(definition.OnEnter) != 1 {
		t.Fatalf("%s: want one on_enter action, got %d", name, len(definition.OnEnter))
	}
	action := definition.OnEnter[0]
	if action.Type != rule.ActionTypePublishAgent {
		t.Fatalf("%s: want publish_agent, got %q", name, action.Type)
	}
	return action
}

func ruleFamilyCompare(got, want ruleFamilyExpected) error {
	if !reflect.DeepEqual(got, want) {
		gotJSON, _ := json.Marshal(got)
		wantJSON, _ := json.Marshal(want)
		return fmt.Errorf("rule-family values differ: got %s, want %s", gotJSON, wantJSON)
	}
	return nil
}

func ruleFamilyDecoded(action rule.Action) ruleFamilyExpected {
	return ruleFamilyActual(action.Subject, action.Role, action.Model, action.ToolChoice, action.ResponseFormat)
}

func TestRuleFamilyDecode(t *testing.T) {
	for name, want := range ruleFamilyGoldens(t) {
		t.Run(name, func(t *testing.T) {
			if err := ruleFamilyCompare(ruleFamilyDecoded(ruleFamilyAction(t, name)), want); err != nil {
				t.Fatal(err)
			}
			var definition rule.Definition
			ruleFamilyReadJSON(t, filepath.Join(ruleFamilyFixtureDir, name), &definition)
			data, err := json.Marshal(definition)
			if err != nil {
				t.Fatal(err)
			}
			var roundTrip rule.Definition
			if err := json.Unmarshal(data, &roundTrip); err != nil {
				t.Fatal(err)
			}
			if len(roundTrip.OnEnter) != 1 {
				t.Fatalf("round trip lost action: got %d", len(roundTrip.OnEnter))
			}
			if err := ruleFamilyCompare(ruleFamilyDecoded(roundTrip.OnEnter[0]), want); err != nil {
				t.Fatalf("Definition JSON round trip: %v", err)
			}
		})
	}
}

// The publisher captures the actual public executor output. It proves emission,
// not NATS delivery, tool resolution, loop consumption, or provider forwarding.
type ruleFamilyPublisher struct {
	subjects []string
	messages [][]byte
}

func (p *ruleFamilyPublisher) Publish(_ context.Context, subject string, data []byte) error {
	p.subjects = append(p.subjects, subject)
	p.messages = append(p.messages, append([]byte(nil), data...))
	return nil
}

func ruleFamilyEmit(t *testing.T, action rule.Action) ruleFamilyExpected {
	t.Helper()
	publisher := &ruleFamilyPublisher{}
	executor := rule.NewActionExecutorFull(discardLogger(), nil, publisher, types.PlatformMeta{
		Org: "c360", Platform: "fixture",
	})
	// A canonical non-loop entity and absent graph/lifecycle dependencies keep
	// E3 run inheritance outside this E2 decode/current frozen emission proof.
	if err := executor.Execute(t.Context(), action, &rule.ExecutionContext{
		EntityID: "c360.fixture.rule.fixture.entity.input",
	}); err != nil {
		t.Fatalf("execute publish_agent: %v", err)
	}
	if len(publisher.messages) != 1 {
		t.Fatalf("one synchronous executor invocation emitted %d messages", len(publisher.messages))
	}
	registry := payloadregistry.New()
	if err := runtimecatalog.RegisterPayloads(registry); err != nil {
		t.Fatal(err)
	}
	envelope, err := message.NewDecoder(registry).Decode(publisher.messages[0])
	if err != nil {
		t.Fatalf("decode executor envelope with product catalog: %v", err)
	}
	task, ok := envelope.Payload().(*agentic.TaskMessage)
	if !ok {
		t.Fatalf("canonical payload = %T, want *agentic.TaskMessage", envelope.Payload())
	}
	if err := task.Validate(); err != nil {
		t.Fatalf("emitted task validation: %v", err)
	}
	return ruleFamilyActual(publisher.subjects[0], task.Role, task.Model, task.ToolChoice, task.ResponseFormat)
}

func TestRuleFamilyEmission(t *testing.T) {
	for name, want := range ruleFamilyGoldens(t) {
		t.Run(name, func(t *testing.T) {
			if err := ruleFamilyCompare(ruleFamilyEmit(t, ruleFamilyAction(t, name)), want); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// Reject both absent fields and changed values using the same oracle as the
// positive decode/emission tests. All mutations remain valid executor inputs.
func TestRuleFamilyOracleRejectsDrift(t *testing.T) {
	cases := []struct {
		name, fixture string
		mutate        func(map[string]any)
	}{
		{"required-omitted", "required-live.json", func(a map[string]any) { delete(a, "tool_choice") }},
		{"required-mode-drift", "required-live.json", func(a map[string]any) {
			a["tool_choice"].(map[string]any)["mode"] = "auto"
		}},
		{"function-name-drift", "forced-function.json", func(a map[string]any) {
			a["tool_choice"].(map[string]any)["function_name"] = "other_tool"
		}},
		{"format-omitted", "response-format.json", func(a map[string]any) { delete(a, "response_format") }},
		{"format-strict-drift", "response-format.json", func(a map[string]any) {
			a["response_format"].(map[string]any)["strict"] = false
		}},
		{"format-nested-schema-drift", "response-format.json", func(a map[string]any) {
			schema := a["response_format"].(map[string]any)["schema"].(map[string]any)
			properties := schema["properties"].(map[string]any)
			items := properties["evidence"].(map[string]any)["items"].(map[string]any)
			items["properties"].(map[string]any)["accepted"].(map[string]any)["type"] = "string"
		}},
	}
	expected := ruleFamilyGoldens(t)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var raw map[string]any
			ruleFamilyReadJSON(t, filepath.Join(ruleFamilyFixtureDir, tc.fixture), &raw)
			tc.mutate(raw["on_enter"].([]any)[0].(map[string]any))
			data, err := json.Marshal(raw)
			if err != nil {
				t.Fatal(err)
			}
			var definition rule.Definition
			if err := json.Unmarshal(data, &definition); err != nil {
				t.Fatal(err)
			}
			action := definition.OnEnter[0]
			if ruleFamilyCompare(ruleFamilyDecoded(action), expected[tc.fixture]) == nil {
				t.Fatal("decode oracle accepted changed input")
			}
			if ruleFamilyCompare(ruleFamilyEmit(t, action), expected[tc.fixture]) == nil {
				t.Fatal("emission oracle accepted changed input")
			}
		})
	}
}

func TestRuleFamilyLiveSourceProjection(t *testing.T) {
	var provenance struct {
		RequiredLive struct {
			Fixture    string   `json:"fixture"`
			SourcePath string   `json:"source_path"`
			Pointer    string   `json:"source_json_pointer"`
			SHA256     string   `json:"source_sha256"`
			Retained   []string `json:"retained_action_fields"`
			Omitted    []string `json:"omitted_action_fields"`
		} `json:"required_live"`
	}
	ruleFamilyReadJSON(t, filepath.Join(ruleFamilyFixtureDir, "provenance.json"), &provenance)
	p := provenance.RequiredLive
	if p.Pointer != "/on_enter/0" {
		t.Fatalf("unsupported live action selector %q", p.Pointer)
	}
	data, err := os.ReadFile(filepath.Join("../..", p.SourcePath))
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(data)); got != p.SHA256 {
		t.Fatalf("live source changed: got SHA256 %s, pinned %s; review fixture provenance", got, p.SHA256)
	}
	var source, fixture struct {
		OnEnter []map[string]any `json:"on_enter"`
	}
	if err := json.Unmarshal(data, &source); err != nil {
		t.Fatal(err)
	}
	ruleFamilyReadJSON(t, filepath.Join(ruleFamilyFixtureDir, p.Fixture), &fixture)
	if len(source.OnEnter) == 0 || len(fixture.OnEnter) != 1 {
		t.Fatal("missing source or fixture action")
	}
	projected := make(map[string]any)
	for _, key := range p.Retained {
		value, exists := source.OnEnter[0][key]
		if !exists {
			t.Fatalf("retained source field %s is absent", key)
		}
		projected[key] = value
	}
	for _, key := range p.Omitted {
		if _, exists := source.OnEnter[0][key]; !exists {
			t.Fatalf("declared omitted source field %s is absent", key)
		}
		if _, retained := projected[key]; retained {
			t.Fatalf("field %s is both retained and omitted", key)
		}
	}
	if len(p.Retained)+len(p.Omitted) != len(source.OnEnter[0]) {
		t.Fatal("provenance does not account for every source action field")
	}
	if !reflect.DeepEqual(projected, fixture.OnEnter[0]) {
		t.Fatal("required-live action differs from its declared current source projection")
	}
}
