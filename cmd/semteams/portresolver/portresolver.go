// Package portresolver looks up NATS subjects from a running semstreams
// component config, so the product shell's subscribers and request/reply
// callers stay aligned with whatever ports an operator wired in their
// deployment — instead of riding on hardcoded subject literals that
// silently break when ports change.
//
// The pattern: each subscriber/reader in cmd/semteams/ accepts its
// subject as a constructor parameter; cmd/semteams/main.go resolves the
// subject via portresolver.SubjectOrDefault at boot, passing the
// per-package default constant as the fallback when a config does not
// declare the port. Operators can override per deployment by editing
// the component's port definition in their JSON config.
//
// Why this exists: smoke #8 run-4 exposed a literal-string class of
// bug. Our chain milestone subscribers had `graph.query.entity`
// hardcoded; no agentic config ran graph-query. Result: silent
// timeouts, no chain triples landed. The fix has two layers:
//
//  1. PR #107 added graph-query to the affected configs and pinned
//     subject correctness with live-NATS integration tests. Closes the
//     immediate bug.
//
//  2. This package closes the architectural class: subjects come from
//     config (operator-controlled, the framework's source of truth);
//     constants are a backstop for configs that omit the port. A future
//     port rewire CAN'T silently break our subscribers — at worst, the
//     fallback constant fires and the integration tests + smoke catch
//     drift.
//
// See ADR-038 PR D / fix-plan.md Phase 2 for the broader rationale.
package portresolver

import (
	"encoding/json"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/config"
)

// Subject extracts the NATS subject for a named port on a named
// component instance. Walks the Inputs then Outputs slices of the
// component's port config (within that single component, in that
// order; the beta.160 port cutover deleted the kv_write side lane).
// Returns the empty string when:
//
//   - the component instance is not present in the config map, OR
//   - the component is disabled (Enabled=false), OR
//   - the component config does not declare a `ports` block, OR
//   - the ports block is not in the canonical beta.160 envelope
//     (strict decode fails; the caller falls back to its default), OR
//   - the named port does not appear in any direction.
//
// Returning empty rather than erroring is deliberate — the caller has
// the context to decide whether absence is fatal or whether to fall
// back to a default subject. SubjectOrDefault wraps the common pattern.
//
// IMPORTANT: This walks ONLY the operator's port overrides as declared
// in JSON config. It does NOT consult the component's Go-level default
// port set (e.g. agentic-loop's DefaultConfig() which declares
// agent.failed and agent.complete with their canonical subjects).
// Upstream's component lifecycle merges defaults into the live port
// set at runtime via component.MergePortConfigs, but those defaults
// live in Go code, not in the JSON Subject sees. To override an
// upstream default, an operator MUST add an explicit port entry to
// the component's config block. Otherwise the SubjectOrDefault caller
// gets the package fallback constant — which is correct under today's
// configs (no agentic config declares agent.failed) but means
// portresolver-resolved subjects are operator-overrides only, not
// "the live runtime port set." A future enhancement could fall back
// to upstream defaults via a componentregistry.DefaultPorts(name)
// lookup; tracked as a Phase 2 follow-up in fix-plan.md.
//
// Port names are unique within a single component by convention; if
// the same name appears in two directions the first match (Inputs >
// Outputs) wins. That ordering matches how component definitions are
// usually read (request flows are inputs first).
//
// beta.160 envelope: each PortDefinition carries a typed Config
// (Portable). Wire subjects live on nats/nats-request configs
// (Subject) and jetstream configs (Subjects — the FIRST entry is
// returned, matching the single-subject entries every semteams config
// declares). KV and store port kinds carry bucket identifiers, not
// wire subjects — no cmd/semteams caller resolves those, so they
// return "" here.
func Subject(cfg *config.Config, componentInstance, portName string) string {
	if cfg == nil {
		return ""
	}
	cc, ok := cfg.Components[componentInstance]
	if !ok || !cc.Enabled {
		return ""
	}
	var wrapped struct {
		Ports *component.PortConfig `json:"ports"`
	}
	if err := json.Unmarshal(cc.Config, &wrapped); err != nil || wrapped.Ports == nil {
		return ""
	}
	for _, lane := range [][]component.PortDefinition{wrapped.Ports.Inputs, wrapped.Ports.Outputs} {
		for _, p := range lane {
			if p.Name == portName {
				return portableSubject(p.Config)
			}
		}
	}
	return ""
}

// portableSubject extracts the wire subject from a typed port config.
// decodePortable yields value types (component/port_codec.go), so the
// switch uses value cases.
func portableSubject(pc component.Portable) string {
	switch c := pc.(type) {
	case component.NATSPort:
		return c.Subject
	case component.NATSRequestPort:
		return c.Subject
	case component.JetStreamPort:
		if len(c.Subjects) > 0 {
			return c.Subjects[0]
		}
	}
	return ""
}

// SubjectOrDefault returns the resolved subject from config, falling
// back to defaultSubject when the lookup returns empty. This is the
// pattern cmd/semteams/main.go uses at wire-time:
//
//	subject := portresolver.SubjectOrDefault(
//	    cfg, "teams-loop", "agent.complete",
//	    chain.DefaultLoopCompletedSubject,
//	)
//	chainSub := chain.NewCompletionSubscriber(handlers, subject, logger)
//
// Config wins when present; the package constant fires only as a
// graceful-degradation backstop for configs that omit port declarations
// or use a different component instance name.
func SubjectOrDefault(cfg *config.Config, componentInstance, portName, defaultSubject string) string {
	if s := Subject(cfg, componentInstance, portName); s != "" {
		return s
	}
	return defaultSubject
}
