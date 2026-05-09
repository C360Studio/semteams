package portresolver_test

import (
	"encoding/json"
	"testing"

	"github.com/c360studio/semstreams/config"
	"github.com/c360studio/semstreams/types"

	"github.com/c360studio/semteams/cmd/semteams/portresolver"
)

// makeConfig builds a minimal *config.Config with one component whose
// raw JSON config block carries the supplied port definitions. Mirrors
// the shape semstreams' loader produces from configs/*.json.
func makeConfig(t *testing.T, instance, name string, enabled bool, portsJSON string) *config.Config {
	t.Helper()
	body := []byte(`{"ports":` + portsJSON + `}`)
	if portsJSON == "" {
		body = []byte(`{}`)
	}
	cc := types.ComponentConfig{
		Type:    "processor",
		Name:    name,
		Enabled: enabled,
		Config:  json.RawMessage(body),
	}
	return &config.Config{
		Components: map[string]types.ComponentConfig{instance: cc},
	}
}

func TestSubject_NilConfig(t *testing.T) {
	if got := portresolver.Subject(nil, "any", "any"); got != "" {
		t.Errorf("nil config: got %q, want empty", got)
	}
}

func TestSubject_MissingComponent(t *testing.T) {
	cfg := makeConfig(t, "teams-loop", "agentic-loop", true,
		`{"outputs":[{"name":"agent.complete","subject":"agent.complete.*"}]}`)
	if got := portresolver.Subject(cfg, "missing-instance", "agent.complete"); got != "" {
		t.Errorf("missing component: got %q, want empty", got)
	}
}

func TestSubject_DisabledComponent(t *testing.T) {
	cfg := makeConfig(t, "teams-loop", "agentic-loop", false,
		`{"outputs":[{"name":"agent.complete","subject":"agent.complete.*"}]}`)
	if got := portresolver.Subject(cfg, "teams-loop", "agent.complete"); got != "" {
		t.Errorf("disabled component: got %q, want empty (disabled components do not contribute live subjects)", got)
	}
}

func TestSubject_NoPortsBlock(t *testing.T) {
	cfg := makeConfig(t, "teams-loop", "agentic-loop", true, "")
	if got := portresolver.Subject(cfg, "teams-loop", "agent.complete"); got != "" {
		t.Errorf("no ports block: got %q, want empty", got)
	}
}

func TestSubject_OutputPortFound(t *testing.T) {
	cfg := makeConfig(t, "teams-loop", "agentic-loop", true,
		`{"outputs":[{"name":"agent.complete","subject":"agent.complete.*","type":"jetstream"}]}`)
	if got := portresolver.Subject(cfg, "teams-loop", "agent.complete"); got != "agent.complete.*" {
		t.Errorf("output port: got %q, want %q", got, "agent.complete.*")
	}
}

func TestSubject_InputPortFound(t *testing.T) {
	cfg := makeConfig(t, "graph-query", "graph-query", true,
		`{"inputs":[{"name":"query_requests","subject":"graph.query.>","type":"nats-request"}]}`)
	if got := portresolver.Subject(cfg, "graph-query", "query_requests"); got != "graph.query.>" {
		t.Errorf("input port: got %q, want %q", got, "graph.query.>")
	}
}

func TestSubject_KVWritePortFound(t *testing.T) {
	cfg := makeConfig(t, "graph-ingest", "graph-ingest", true,
		`{"kv_write":[{"name":"entity_states","subject":"ENTITY_STATES","type":"kv-write"}]}`)
	if got := portresolver.Subject(cfg, "graph-ingest", "entity_states"); got != "ENTITY_STATES" {
		t.Errorf("kv_write port: got %q, want %q", got, "ENTITY_STATES")
	}
}

func TestSubject_PortNameNotFound(t *testing.T) {
	cfg := makeConfig(t, "teams-loop", "agentic-loop", true,
		`{"outputs":[{"name":"agent.complete","subject":"agent.complete.*"}]}`)
	if got := portresolver.Subject(cfg, "teams-loop", "missing-port"); got != "" {
		t.Errorf("port name not found: got %q, want empty", got)
	}
}

func TestSubject_PrefersInputsOverOutputs(t *testing.T) {
	// Same name in both directions — Inputs wins per the documented
	// ordering. Real configs don't typically duplicate names; this is a
	// defensive contract test.
	cfg := makeConfig(t, "weird", "weird-comp", true,
		`{"inputs":[{"name":"shared","subject":"in.subject"}],"outputs":[{"name":"shared","subject":"out.subject"}]}`)
	if got := portresolver.Subject(cfg, "weird", "shared"); got != "in.subject" {
		t.Errorf("inputs/outputs collision: got %q, want %q (Inputs wins)", got, "in.subject")
	}
}

func TestSubject_MalformedJSONReturnsEmpty(t *testing.T) {
	cc := types.ComponentConfig{
		Type:    "processor",
		Name:    "broken",
		Enabled: true,
		Config:  json.RawMessage(`{"ports": "not an object"}`),
	}
	cfg := &config.Config{Components: map[string]types.ComponentConfig{"broken": cc}}
	if got := portresolver.Subject(cfg, "broken", "any"); got != "" {
		t.Errorf("malformed ports json: got %q, want empty (caller falls back to default)", got)
	}
}

func TestSubjectOrDefault_FallsBackWhenEmpty(t *testing.T) {
	cfg := makeConfig(t, "teams-loop", "agentic-loop", true, "")
	const fallback = "agent.complete.>"
	if got := portresolver.SubjectOrDefault(cfg, "teams-loop", "agent.complete", fallback); got != fallback {
		t.Errorf("empty resolution: got %q, want fallback %q", got, fallback)
	}
}

func TestSubjectOrDefault_PrefersConfigSubject(t *testing.T) {
	cfg := makeConfig(t, "teams-loop", "agentic-loop", true,
		`{"outputs":[{"name":"agent.complete","subject":"custom.subject.>"}]}`)
	const fallback = "agent.complete.>"
	if got := portresolver.SubjectOrDefault(cfg, "teams-loop", "agent.complete", fallback); got != "custom.subject.>" {
		t.Errorf("config-defined subject: got %q, want %q (config wins over default)", got, "custom.subject.>")
	}
}

func TestSubjectOrDefault_EmptyDefaultStillEmpty(t *testing.T) {
	cfg := makeConfig(t, "teams-loop", "agentic-loop", true, "")
	if got := portresolver.SubjectOrDefault(cfg, "teams-loop", "agent.complete", ""); got != "" {
		t.Errorf("empty default: got %q, want empty (caller asked for empty as the explicit fallback)", got)
	}
}
