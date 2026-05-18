package contract

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
)

// portDef is the subset of the canonical component port definition
// this test consumes. Mirrors the shape used by other contract tests
// in this package; intentionally minimal to keep parse failures
// localised.
type portDef struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	Subject    string `json:"subject"`
	StreamName string `json:"stream_name"`
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
//	port.stream_name = "AGENT" + port.subject = "tool.execute.*"
//
// The streams manager (config.streams.go EnsureStreams) derives
// subjects from the FIRST stream_name encountered per stream — so a
// stream named AGENT ends up with subjects=["agent.>"], and
// subsequent ports declaring stream_name=AGENT with subject
// "tool.execute.*" silently fail to route at runtime ("nats: no
// response from stream"). The contract: subject prefix must match
// the canonical stream for that family.
//
//	agent.>  → AGENT
//	tool.>   → TOOL
//	user.>   → USER  (or type="nats" for fire-and-forget)
//	graph.>  → GRAPH (or type="nats" for fire-and-forget)
//
// This test only enforces the agent/tool/user split because those
// are the streams the MVP roster exercises. Graph subjects can route
// via either nats (fire-and-forget, no stream) or jetstream; both
// shapes are accepted.
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
// Only enforces the rule when port.type == "jetstream" — nats-typed
// ports are fire-and-forget and don't trigger stream derivation.
func checkPortStreamWiring(p portDef) string {
	if p.Type != "jetstream" {
		return ""
	}
	want := canonicalStreamFor(p.Subject)
	if want == "" {
		// Subject doesn't carry a canonical mapping (entity-watch
		// patterns, internal subjects, etc.) — skip.
		return ""
	}
	if p.StreamName != want {
		return fmt.Sprintf("subject %q routes to stream_name=%q, want %q (mismatch causes silent nats: no response from stream at runtime)",
			p.Subject, p.StreamName, want)
	}
	return ""
}

// canonicalStreamFor returns the canonical stream name for the given
// subject, or "" if no canonical mapping applies. Matches the prefix
// against the canonical stream-family roots that
// streams.DeriveStreamSubjects honors.
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
//   - user.response.* NATS output — fire-and-forget reply prose for
//     channel routers to deliver.
//
// The bug class this catches: the MVP-1 bootstrap declared a
// self-loop input (subject equal to its own output, reading from
// agent.task.* on its own output stream) and was missing both the
// user.response output and the agent.complete input. Each shape
// would silently break a different rule path (no wake-up reply, no
// terminal publish, etc.) without surfacing a startup error.
func TestFlowBootstrapDispatchShape(t *testing.T) {
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

			// Track required port shapes by name.
			requiredInputs := map[string]struct {
				subject    string
				streamName string
				portType   string
			}{
				"user.message":   {"user.message.>", "USER", "jetstream"},
				"agent.complete": {"agent.complete.*", "AGENT", "jetstream"},
			}
			requiredOutputs := map[string]struct {
				subject    string
				streamName string
				portType   string
			}{
				"agent.task":    {"agent.task.*", "AGENT", "jetstream"},
				"user.response": {"user.response.*", "", "nats"},
			}

			outputSubjects := make(map[string]bool, len(dispatchCfg.Ports.Outputs))
			for _, p := range dispatchCfg.Ports.Outputs {
				outputSubjects[p.Subject] = true
			}

			// Self-loop input check: no input port may share a subject
			// with one of dispatch's own outputs. The MVP-1 bug had
			// dispatch reading from agent.task.* on AGENT (its own
			// output stream), which silently routed nothing.
			for _, in := range dispatchCfg.Ports.Inputs {
				if outputSubjects[in.Subject] {
					t.Errorf("dispatch input port %q subject %q matches one of dispatch's own outputs — self-loop dispatch is the MVP-1 wedge class",
						in.Name, in.Subject)
				}
			}

			// Required-input presence.
			for portName, want := range requiredInputs {
				found := false
				for _, in := range dispatchCfg.Ports.Inputs {
					if in.Name != portName {
						continue
					}
					found = true
					if in.Subject != want.subject {
						t.Errorf("dispatch input %q subject=%q, want %q",
							portName, in.Subject, want.subject)
					}
					if in.StreamName != want.streamName {
						t.Errorf("dispatch input %q stream_name=%q, want %q",
							portName, in.StreamName, want.streamName)
					}
					if in.Type != want.portType {
						t.Errorf("dispatch input %q type=%q, want %q",
							portName, in.Type, want.portType)
					}
				}
				if !found {
					t.Errorf("dispatch missing required input port %q (subject %q on stream %q)",
						portName, want.subject, want.streamName)
				}
			}

			// Required-output presence.
			for portName, want := range requiredOutputs {
				found := false
				for _, out := range dispatchCfg.Ports.Outputs {
					if out.Name != portName {
						continue
					}
					found = true
					if out.Subject != want.subject {
						t.Errorf("dispatch output %q subject=%q, want %q",
							portName, out.Subject, want.subject)
					}
					if out.StreamName != want.streamName {
						t.Errorf("dispatch output %q stream_name=%q, want %q",
							portName, out.StreamName, want.streamName)
					}
					if out.Type != want.portType {
						t.Errorf("dispatch output %q type=%q, want %q",
							portName, out.Type, want.portType)
					}
				}
				if !found {
					t.Errorf("dispatch missing required output port %q (subject %q type %q)",
						portName, want.subject, want.portType)
				}
			}
		})
	}
}
