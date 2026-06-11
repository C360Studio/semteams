package contract

import (
	"encoding/json"
	"os"
	"testing"
)

// TestClarificationPolicy_RestrictedDecideActionsWired pins the SemTeams side of
// the run-level CLARIFICATION POLICY (ADR-053 Phase 4b / semstreams#239, beta.104).
//
// The framework's agentic-tools.restricted_decide_actions config bars named
// decide actions for EVERY coordinator task — front-door AND rule-spawned —
// composing with and taking precedence over the per-task action_allowlist (the
// front-door structural hole #239 named). SemTeams maps its product run mode onto
// the list: [] = interactive (ask_user available — the default); ["ask_user"] =
// autonomous (the coordinator must resolve without deferring to the user).
// cmd/semteams/main.go threads the value via extractRestrictedDecideActions →
// executors.RegisterBuiltins (mirroring extractLoopsBucket per ADR-029).
//
// This test pins that the lever is present + discoverable in both bootstrap
// configs and defaults to interactive (empty) so existing behavior is unchanged
// — autonomous is an operator opt-in, never the shipped default.
func TestClarificationPolicy_RestrictedDecideActionsWired(t *testing.T) {
	for _, cfg := range []string{"../../configs/flow-bootstrap.json", "../../configs/e2e-flow-bootstrap.json"} {
		raw, err := os.ReadFile(cfg) //nolint:gosec // test-controlled config path
		if err != nil {
			t.Fatalf("read %s: %v", cfg, err)
		}
		var doc struct {
			Components map[string]struct {
				Config struct {
					// pointer distinguishes absent (nil) from empty ([])
					Restricted *[]string `json:"restricted_decide_actions"`
				} `json:"config"`
			} `json:"components"`
		}
		if err := json.Unmarshal(raw, &doc); err != nil {
			t.Fatalf("unmarshal %s: %v", cfg, err)
		}
		at, ok := doc.Components["agentic-tools"]
		if !ok {
			t.Fatalf("%s: no agentic-tools component", cfg)
		}
		if at.Config.Restricted == nil {
			t.Errorf("%s: agentic-tools.config.restricted_decide_actions is ABSENT — the clarification-policy "+
				"lever must be present + discoverable (ADR-053 4b / semstreams#239). Empty [] = interactive default.", cfg)
			continue
		}
		if len(*at.Config.Restricted) != 0 {
			t.Errorf("%s: agentic-tools.restricted_decide_actions = %v, want [] — interactive is the shipped default; "+
				"autonomous (e.g. [\"ask_user\"]) is an operator opt-in, not committed to the substrate configs", cfg, *at.Config.Restricted)
		}
	}
}
