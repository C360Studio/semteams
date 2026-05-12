package contract

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ruleWithRelatedLoops is the minimum shape we need to assert against —
// every spawn rule in scope must carry a related_loops map under
// on_enter[0] with a "researcher" key pointing at a substitution token.
type ruleWithRelatedLoops struct {
	OnEnter []struct {
		Type         string            `json:"type"`
		RelatedLoops map[string]string `json:"related_loops"`
		Prompt       string            `json:"prompt"`
	} `json:"on_enter"`
}

// TestLineageThreading_RuleCoverage pins the contract that every spawn
// rule in the research-mode-transition + dev-via-spec arcs threads the
// researcher's loop_id forward via related_loops. Smoke #8 run-2 wedge
// (architect needed research artifact loop_id for harness selection)
// is the motivating evidence. ADR-035 §D2 ("per-role rigour, not
// exhaustive backward reach") is the persona-content discipline this
// supports; semstreams beta.51 LineageTriplePrefix + auto-stamp at
// loop-creation time is the wire substrate.
func TestLineageThreading_RuleCoverage(t *testing.T) {
	tests := []struct {
		path  string
		want  string
		notes string
	}{
		// Source rules: stamp lineage.researcher from the just-completed
		// researcher loop's $entity.instance. Under ADR-040 only 01a
		// remains — the prior 01b (researcher-with-source-acquisition →
		// RR) was deleted because the role no longer exists; the post-
		// curator re-query researcher is plain `researcher` and is
		// handled by 01a's eq match. ADR-041 Slice 2D-1 deleted rules
		// 02/02b/02c (curator teardown); the replacement
		// 02-reviewer-rejected-retry-research arrives in Slice 2D-3
		// and will re-add its coverage entry here.
		{
			path:  "../../configs/rules/research-mode-transition/01a-research-to-reviewer-after-researcher.json",
			want:  "$entity.instance",
			notes: "researcher -> RR: lineage.researcher = researcher's loop_id",
		},
		// Forward-thread rules: read lineage.researcher from triggering entity.
		{
			path:  "../../configs/rules/research-mode-transition/03-stabilise-and-transition.json",
			want:  "$entity.triple.lineage.researcher",
			notes: "RR approved -> planner: forward lineage.researcher",
		},
		{
			path:  "../../configs/rules/dev-via-spec/01-planner-to-reviewer.json",
			want:  "$entity.triple.lineage.researcher",
			notes: "planner -> reviewer: forward lineage.researcher",
		},
		{
			path:  "../../configs/rules/dev-via-spec/02-reviewer-rejected-retry-planner.json",
			want:  "$entity.triple.lineage.researcher",
			notes: "reviewer rejected -> planner retry: forward lineage.researcher",
		},
		{
			path:  "../../configs/rules/dev-via-spec/03-reviewer-approved-to-challenger.json",
			want:  "$entity.triple.lineage.researcher",
			notes: "reviewer approved -> challenger: forward lineage.researcher",
		},
		{
			path:  "../../configs/rules/dev-via-spec/04-challenger-concerns-retry-planner.json",
			want:  "$entity.triple.lineage.researcher",
			notes: "challenger concerns -> planner retry: forward lineage.researcher",
		},
		{
			path:  "../../configs/rules/dev-via-spec/05-challenger-accept-to-architect.json",
			want:  "$entity.triple.lineage.researcher",
			notes: "challenger accept -> architect: forward lineage.researcher (architect consumes it for harness selection)",
		},
		{
			path:  "../../configs/rules/dev-via-spec/06-architect-emit-to-builder.json",
			want:  "$entity.triple.lineage.researcher",
			notes: "architect emit -> builder: forward lineage.researcher",
		},
		{
			path:  "../../configs/rules/dev-via-spec/07-builder-decide-to-qa-reviewer.json",
			want:  "$entity.triple.lineage.researcher",
			notes: "builder decide -> qa-reviewer: forward lineage.researcher",
		},
		{
			path:  "../../configs/rules/dev-via-spec/09-qa-reviewer-needs-clarification-to-architect.json",
			want:  "$entity.triple.lineage.researcher",
			notes: "ADR-039 Phase 1 recovery: qa-reviewer needs_clarification -> architect respawn — forward lineage.researcher so the new architect can re-read the research artifact when re-emitting the spec",
		},
	}

	for _, tt := range tests {
		t.Run(filepath.Base(tt.path), func(t *testing.T) {
			data, err := os.ReadFile(tt.path)
			if err != nil {
				t.Fatalf("read %s: %v", tt.path, err)
			}

			var rule ruleWithRelatedLoops
			if err := json.Unmarshal(data, &rule); err != nil {
				t.Fatalf("unmarshal %s: %v", tt.path, err)
			}

			if len(rule.OnEnter) == 0 {
				t.Fatalf("%s: on_enter is empty", tt.path)
			}

			action := rule.OnEnter[0]
			if action.Type != "publish_agent" {
				t.Fatalf("%s: expected on_enter[0].type=publish_agent, got %q", tt.path, action.Type)
			}

			got, ok := action.RelatedLoops["researcher"]
			if !ok {
				t.Fatalf("%s: related_loops[\"researcher\"] missing — %s", tt.path, tt.notes)
			}
			if got != tt.want {
				t.Errorf("%s: related_loops[\"researcher\"] = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

// TestLineageThreading_DiscoveryWalk catches accretion drift — any future
// publish_agent rule added under research-mode-transition or dev-via-spec
// without `related_loops.researcher` will fail this test unless explicitly
// allowlisted with a documented reason. Closes the gap that
// TestLineageThreading_RuleCoverage's enumeration leaves open.
func TestLineageThreading_DiscoveryWalk(t *testing.T) {
	// Allowlist: rules where lineage threading is intentionally skipped.
	// Each entry MUST have a one-line rationale.
	allowlist := map[string]string{
		"08-architect-needs-clarification-to-researcher.json": "ADR-039 Phase 1 recovery rule. Spawns a fresh researcher (the new research-artifact author) — forwarding the prior researcher's loop_id would stamp lineage.researcher to a SUPERSEDED loop, violating the invariant. The next 01a firing re-stamps lineage.researcher = $entity.instance of the recovery researcher; downstream architect/reviewer/etc. see the new artifact pointer.",
	}

	dirs := []string{
		"../../configs/rules/research-mode-transition",
		"../../configs/rules/dev-via-spec",
	}

	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("read %s: %v", dir, err)
		}
		for _, entry := range entries {
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
				continue
			}
			path := filepath.Join(dir, entry.Name())
			data, err := os.ReadFile(path)
			if err != nil {
				t.Errorf("read %s: %v", path, err)
				continue
			}
			var rule ruleWithRelatedLoops
			if err := json.Unmarshal(data, &rule); err != nil {
				t.Errorf("unmarshal %s: %v", path, err)
				continue
			}
			// Only rules with publish_agent on_enter actions are in scope.
			hasPublishAgent := false
			for _, action := range rule.OnEnter {
				if action.Type == "publish_agent" {
					hasPublishAgent = true
					break
				}
			}
			if !hasPublishAgent {
				continue
			}

			// Allowlisted? Skip but log the rationale for visibility.
			if reason, ok := allowlist[entry.Name()]; ok {
				t.Logf("%s: lineage threading skipped intentionally — %s", entry.Name(), reason)
				continue
			}

			// Required: related_loops.researcher must be present on the first
			// publish_agent action.
			first := rule.OnEnter[0]
			if first.Type != "publish_agent" {
				continue
			}
			if _, ok := first.RelatedLoops["researcher"]; !ok {
				t.Errorf("%s: publish_agent rule missing related_loops[\"researcher\"]; either thread lineage or add to allowlist with rationale", path)
			}
		}
	}
}

// TestLineageThreading_ArchitectPromptSubstitution asserts the architect's
// spawn rule's prompt body actually substitutes lineage.researcher into
// the prompt text — without this, even a successful related_loops thread
// doesn't reach the LLM (the LLM doesn't see TaskMessage.Metadata
// directly; the prompt body is the reliable surface).
func TestLineageThreading_ArchitectPromptSubstitution(t *testing.T) {
	data, err := os.ReadFile("../../configs/rules/dev-via-spec/05-challenger-accept-to-architect.json")
	if err != nil {
		t.Fatalf("read rule 05: %v", err)
	}
	var rule ruleWithRelatedLoops
	if err := json.Unmarshal(data, &rule); err != nil {
		t.Fatalf("unmarshal rule 05: %v", err)
	}
	if len(rule.OnEnter) == 0 {
		t.Fatal("rule 05: on_enter is empty")
	}
	prompt := rule.OnEnter[0].Prompt
	if !strings.Contains(prompt, "$entity.triple.lineage.researcher") {
		t.Errorf("rule 05 prompt must substitute $entity.triple.lineage.researcher into the body so the architect sees the literal loop_id; got prompt:\n%s", prompt)
	}
	if !strings.Contains(strings.ToLower(prompt), "research artifact") {
		t.Errorf("rule 05 prompt must reference 'research artifact' so the architect knows what the loop_id points at")
	}
}
