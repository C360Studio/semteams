package contract

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
)

// portDef is the subset of the canonical beta.160 component port
// envelope this test consumes ({name, config:{kind,...}}). Mirrors the
// shape used by other contract tests in this package; intentionally
// minimal to keep parse failures localised.
type portDef struct {
	Name   string `json:"name"`
	Config struct {
		Kind       string   `json:"kind"`
		Subject    string   `json:"subject"`
		StreamName string   `json:"stream_name"`
		Subjects   []string `json:"subjects"`
		Interface  struct {
			Type    string `json:"type"`
			Version string `json:"version"`
		} `json:"interface"`
	} `json:"config"`
}

// wireSubjects returns every wire subject a port declares: jetstream
// ports carry a subjects array, nats/nats-request a single subject.
// KV/store kinds carry buckets, not subjects — empty.
func (p portDef) wireSubjects() []string {
	switch p.Config.Kind {
	case "jetstream":
		return p.Config.Subjects
	case "nats", "nats-request":
		if p.Config.Subject != "" {
			return []string{p.Config.Subject}
		}
	}
	return nil
}

type portsBlock struct {
	Inputs  []portDef `json:"inputs"`
	Outputs []portDef `json:"outputs"`
}

// TestFlowBootstrapStreamWiring asserts that bootstrap-style configs
// (configs/flow-bootstrap.json + configs/e2e-flow-bootstrap.json)
// route their JetStream-typed port subjects to the canonical stream
// for that subject family.
//
// Failure mode this catches (the MVP-1 → MVP-5 wedge):
//
//	stream_name = "AGENT" + subjects = ["tool.execute.*"]
//
// A subject published into a stream whose declared filter doesn't
// cover it silently fails to route at runtime ("nats: no response
// from stream"). The contract: subject prefix must match the
// canonical stream for that family.
//
//	agent.>  → AGENT
//	tool.>   → TOOL
//	user.>   → USER  (or kind="nats" for fire-and-forget)
//	graph.>  → (no stream; nats/nats-request request-reply at beta.160)
//
// This test only enforces the agent/tool/user split because those
// are the streams the roster exercises. discovery.* subjects are
// deliberately NOT stream-captured at beta.160 (tool discovery is
// request-reply on discovery.tool.list) — they carry no mapping here.
func TestFlowBootstrapStreamWiring(t *testing.T) {
	for _, configPath := range []string{
		"../../configs/flow-bootstrap.json",
		"../../configs/e2e-flow-bootstrap.json",
	} {
		t.Run(configPath, func(t *testing.T) {
			data, err := os.ReadFile(configPath)
			if err != nil {
				t.Fatalf("read %s: %v", configPath, err)
			}
			var cfg struct {
				Components map[string]struct {
					Config json.RawMessage `json:"config"`
				} `json:"components"`
			}
			if err := json.Unmarshal(data, &cfg); err != nil {
				t.Fatalf("unmarshal %s: %v", configPath, err)
			}

			for compName, comp := range cfg.Components {
				var compCfg struct {
					Ports portsBlock `json:"ports"`
				}
				// Some components have no ports block — skip those.
				if len(comp.Config) == 0 {
					continue
				}
				if err := json.Unmarshal(comp.Config, &compCfg); err != nil {
					// Component config may not even have a ports
					// shape (e.g. services-only entries). Skip.
					continue
				}
				for _, p := range compCfg.Ports.Inputs {
					if violation := checkPortStreamWiring(p); violation != "" {
						t.Errorf("%s.inputs[%s]: %s",
							compName, p.Name, violation)
					}
				}
				for _, p := range compCfg.Ports.Outputs {
					if violation := checkPortStreamWiring(p); violation != "" {
						t.Errorf("%s.outputs[%s]: %s",
							compName, p.Name, violation)
					}
				}
			}
		})
	}
}

// checkPortStreamWiring inspects a single port definition and returns
// an empty string when the wiring is canonical or a violation
// description when it isn't.
//
// Only enforces the rule when kind == "jetstream" — nats/nats-request
// ports are streamless and don't trigger stream derivation.
func checkPortStreamWiring(p portDef) string {
	if p.Config.Kind != "jetstream" {
		return ""
	}
	for _, subject := range p.Config.Subjects {
		want := canonicalStreamFor(subject)
		if want == "" {
			// Subject doesn't carry a canonical mapping (entity-watch
			// patterns, internal subjects, etc.) — skip.
			continue
		}
		if p.Config.StreamName != want {
			return fmt.Sprintf("subject %q routes to stream_name=%q, want %q (mismatch causes silent nats: no response from stream at runtime)",
				subject, p.Config.StreamName, want)
		}
	}
	return ""
}

// canonicalStreamFor returns the canonical stream name for the given
// subject, or "" if no canonical mapping applies. Matches the prefix
// against the canonical stream-family roots the configured streams
// declare.
func canonicalStreamFor(subject string) string {
	switch {
	case strings.HasPrefix(subject, "agent."):
		return "AGENT"
	case strings.HasPrefix(subject, "tool."):
		return "TOOL"
	case strings.HasPrefix(subject, "user."):
		return "USER"
	default:
		return ""
	}
}

// TestFlowBootstrapDispatchShape asserts the dispatch component
// (teams-dispatch) declares the canonical port shape on bootstrap
// configs:
//
//   - user.message.> JetStream input (USER stream) — the cross-channel
//     submission path that channel routers publish onto.
//   - agent.complete.* JetStream input (AGENT stream) — drives the
//     wake-up coordinator path when chains terminate.
//   - agent.task.* JetStream output (AGENT stream) — spawns initial
//     and rule-driven loops.
//   - user.response.> JetStream output (USER stream) — typed
//     agentic.user_response/v1 messages for channel routers to deliver.
//
// The bug class this catches: the MVP-1 bootstrap declared a
// self-loop input (subject equal to its own output, reading from
// agent.task.* on its own output stream) and was missing both the
// user.response output and the agent.complete input. Each shape
// would silently break a different rule path (no wake-up reply, no
// terminal publish, etc.) without surfacing a startup error.
func TestFlowBootstrapDispatchShape(t *testing.T) {
	type wantPort struct {
		subject    string
		streamName string
		kind       string
	}
	for _, configPath := range []string{
		"../../configs/flow-bootstrap.json",
		"../../configs/e2e-flow-bootstrap.json",
	} {
		t.Run(configPath, func(t *testing.T) {
			data, err := os.ReadFile(configPath)
			if err != nil {
				t.Fatalf("read %s: %v", configPath, err)
			}
			var cfg struct {
				Components map[string]struct {
					Config json.RawMessage `json:"config"`
				} `json:"components"`
			}
			if err := json.Unmarshal(data, &cfg); err != nil {
				t.Fatalf("unmarshal %s: %v", configPath, err)
			}
			dispatch, ok := cfg.Components["teams-dispatch"]
			if !ok {
				t.Fatal("teams-dispatch component missing")
			}
			var dispatchCfg struct {
				Ports portsBlock `json:"ports"`
			}
			if err := json.Unmarshal(dispatch.Config, &dispatchCfg); err != nil {
				t.Fatalf("unmarshal dispatch ports: %v", err)
			}

			requiredInputs := map[string]wantPort{
				"user.message":   {"user.message.>", "USER", "jetstream"},
				"agent.complete": {"agent.complete.*", "AGENT", "jetstream"},
			}
			requiredOutputs := map[string]wantPort{
				"agent.task":    {"agent.task.*", "AGENT", "jetstream"},
				"user.response": {"user.response.>", "USER", "jetstream"},
			}

			outputSubjects := map[string]bool{}
			for _, p := range dispatchCfg.Ports.Outputs {
				for _, s := range p.wireSubjects() {
					outputSubjects[s] = true
				}
			}

			// Self-loop input check: no input port may share a subject
			// with one of dispatch's own outputs. The MVP-1 bug had
			// dispatch reading from agent.task.* on AGENT (its own
			// output stream), which silently routed nothing.
			for _, in := range dispatchCfg.Ports.Inputs {
				for _, s := range in.wireSubjects() {
					if outputSubjects[s] {
						t.Errorf("dispatch input port %q subject %q matches one of dispatch's own outputs — self-loop dispatch is the MVP-1 wedge class",
							in.Name, s)
					}
				}
			}

			checkLane := func(lane []portDef, required map[string]wantPort, laneName string) {
				for portName, want := range required {
					found := false
					for _, p := range lane {
						if p.Name != portName {
							continue
						}
						found = true
						subjects := p.wireSubjects()
						if len(subjects) != 1 || subjects[0] != want.subject {
							t.Errorf("dispatch %s %q subjects=%v, want [%q]",
								laneName, portName, subjects, want.subject)
						}
						if p.Config.StreamName != want.streamName {
							t.Errorf("dispatch %s %q stream_name=%q, want %q",
								laneName, portName, p.Config.StreamName, want.streamName)
						}
						if p.Config.Kind != want.kind {
							t.Errorf("dispatch %s %q kind=%q, want %q",
								laneName, portName, p.Config.Kind, want.kind)
						}
						if portName == "user.response" {
							if p.Config.Interface.Type != "agentic.user_response" || p.Config.Interface.Version != "v1" {
								t.Errorf("dispatch %s %q interface=%s/%s, want agentic.user_response/v1",
									laneName, portName, p.Config.Interface.Type, p.Config.Interface.Version)
							}
						}
					}
					if !found {
						t.Errorf("dispatch missing required %s port %q (subject %q kind %q)",
							laneName, portName, want.subject, want.kind)
					}
				}
			}
			checkLane(dispatchCfg.Ports.Inputs, requiredInputs, "input")
			checkLane(dispatchCfg.Ports.Outputs, requiredOutputs, "output")
		})
	}
}

// TestDurationConfigFieldsAreNanosecondNumbers pins the JSON dialect of
// raw time.Duration config fields: they unmarshal as NUMBERS of
// nanoseconds, not duration strings. The upstream graph-query README
// shows "5s" — which fails the actual struct decode, and (worse) a
// component that fails to create does NOT fail the stack: the first
// beta.160 confirmation cycle ran research-mvp GREEN with graph-query
// silently absent. This pin makes the dialect a CI failure instead.
func TestDurationConfigFieldsAreNanosecondNumbers(t *testing.T) {
	targets := map[string]string{
		"graph-query":   "query_timeout",
		"graph-gateway": "query_timeout",
	}
	for _, configPath := range []string{
		"../../configs/flow-bootstrap.json",
		"../../configs/e2e-flow-bootstrap.json",
	} {
		data, err := os.ReadFile(configPath)
		if err != nil {
			t.Fatalf("read %s: %v", configPath, err)
		}
		var cfg struct {
			Components map[string]struct {
				Config map[string]json.RawMessage `json:"config"`
			} `json:"components"`
		}
		if err := json.Unmarshal(data, &cfg); err != nil {
			t.Fatalf("unmarshal %s: %v", configPath, err)
		}
		for comp, field := range targets {
			raw, ok := cfg.Components[comp].Config[field]
			if !ok {
				continue // field omitted → component default applies; fine
			}
			var n float64
			if err := json.Unmarshal(raw, &n); err != nil {
				t.Errorf("%s: %s.%s = %s — time.Duration fields take a NUMBER of nanoseconds; "+
					"a duration string fails component decode, and component-create failures do NOT fail the stack",
					configPath, comp, field, string(raw))
			}
		}
	}
}
