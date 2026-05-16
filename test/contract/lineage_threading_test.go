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
// research-artifact emitter's loop_id forward via related_loops. Smoke #8
// run-2 wedge (architect needed research artifact loop_id for harness
// selection) is the motivating evidence. ADR-035 §D2 ("per-role rigour,
// not exhaustive backward reach") is the persona-content discipline this
// supports; semstreams beta.51 LineageTriplePrefix + auto-stamp at
// loop-creation time is the wire substrate.
//
// Under ADR-041 MVP roster (Slice 2D-3), three classes of spawn rule:
//
//  1. "Anchor" rules — the spawning loop IS the research-artifact
//     emitter. related_loops.researcher = "$entity.instance" stamps the
//     spawned loop's lineage at the emitter directly. Currently 01a
//     (synthesize emit → reviewer-research) and 01b (architect emit →
//     reviewer-spec).
//  2. "Forward" rules — the spawning loop inherits lineage from a prior
//     emitter and forwards it. related_loops.researcher =
//     "$entity.triple.lineage.researcher". Currently
//     02-reviewer-rejected-retry-research,
//     02-reviewer-approved-to-builder, 04-builder-decide-to-reviewer-qa.
//  3. "Allowlisted" rules — spawn a fresh researcher arc where prior
//     lineage would be SUPERSEDED or has not yet been established (phase
//     transitions within the researcher arc fire before any artifact is
//     emitted). See the DiscoveryWalk allowlist for rationales.
func TestLineageThreading_RuleCoverage(t *testing.T) {
	tests := []struct {
		path  string
		want  string
		notes string
	}{
		// Anchor rules — emitter loop becomes the lineage anchor.
		{
			path:  "../../configs/rules/research-mode-transition/01a-researcher-synthesize-to-reviewer-research.json",
			want:  "$entity.instance",
			notes: "synthesize emit → reviewer-research: lineage.researcher = synthesize's loop_id (pure-research arc emitter)",
		},
		{
			path:  "../../configs/rules/research-mode-transition/01b-researcher-architect-to-reviewer-spec.json",
			want:  "$entity.instance",
			notes: "architect emit → reviewer-spec: lineage.researcher = architect's loop_id (dev-via-spec arc emitter)",
		},
		// Forward rules — inherit and forward existing lineage anchor.
		{
			path:  "../../configs/rules/research-mode-transition/02-reviewer-rejected-retry-research.json",
			want:  "$entity.triple.lineage.researcher",
			notes: "reviewer insufficient → researcher-plan retry: forward reviewer's inherited lineage to the recovery researcher",
		},
		{
			path:  "../../configs/rules/dev-via-spec/02-reviewer-approved-to-builder.json",
			want:  "$entity.triple.lineage.researcher",
			notes: "reviewer-spec approved → builder: forward lineage.researcher (builder reads spec produced by architect; ancestry walks resolve to architect's loop)",
		},
		{
			path:  "../../configs/rules/dev-via-spec/04-builder-decide-to-reviewer-qa.json",
			want:  "$entity.triple.lineage.researcher",
			notes: "builder tests_passing → reviewer-qa: forward lineage.researcher (reviewer-qa grades against architect's checks via lineage)",
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
		"03-needs-clarification-to-researcher.json":             "ADR-041 Slice 2D-3 recovery rule. Spawns a fresh researcher-plan (the new research-artifact author) — forwarding the rejecting role's lineage would stamp lineage.researcher to a SUPERSEDED loop, violating the invariant. The next 01a/01b firing re-stamps lineage.researcher = $entity.instance of the new emitter; downstream loops see the new anchor.",
		"04-phase-transition-to-gather.json":                    "ADR-041 Slice 2D-3 within-researcher phase transition. lineage.researcher is established at the eventual emit boundary (researcher-synthesize emit_research_artifact OR researcher-architect emit_dev_via_spec_artifact). Within the arc, beta.51 auto-stamp threads agent.loop.parent for ancestry; lineage.researcher remains unset until emit. Threading $entity.instance here would create stale anchors.",
		"05a-phase-transition-to-synthesize-research-only.json": "ADR-041 addendum 2026-05-16 within-researcher phase transition (research_only branch). Same rationale as 04-phase-transition-to-gather: lineage.researcher anchor lives at the emit boundary, not at phase transitions.",
		"05b-phase-transition-to-synthesize-dev-via-spec.json":  "ADR-041 addendum 2026-05-16 within-researcher phase transition (dev_via_spec branch). Same rationale as 04-phase-transition-to-gather: lineage.researcher anchor lives at the emit boundary, not at phase transitions.",
		"06-phase-transition-to-architect.json":                 "ADR-041 Slice 2D-3 within-researcher phase transition. Same rationale as 04-phase-transition-to-gather: lineage.researcher anchor lives at the emit boundary, not at phase transitions.",
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

// TestLineageThreading_BuilderPromptSubstitution asserts the
// reviewer-spec → builder spawn rule's prompt body actually substitutes
// the spec artifact path into the prompt text. Without this, even a
// successful related_loops thread doesn't reach the builder's bash
// substrate (the builder doesn't see TaskMessage.Metadata directly; the
// prompt body is the reliable surface). Replaces the legacy architect-
// prompt test under ADR-041's compressed roster.
//
// Textual presence is necessary but not sufficient — the substitution
// resolves against the reviewer-spec triggering entity at fire time, but
// dev_via_spec.artifact.{path,slug} is stamped by emitspecartifact on
// the architect researcher entity. phasevalidator.SpecModeGate's
// forwardSpecArtifactTriples (cmd/semteams/phasevalidator/specmode.go)
// is the load-bearing hop that lands the triples on the reviewer-spec
// entity before this rule evaluates. If either side drops (this rule's
// token, the gate's forwarding), the substitution returns a literal
// "$entity.triple.dev_via_spec.artifact.path" string at builder spawn —
// bootstrap_workspace then rejects on path-traversal validation, but
// the failure happens at runtime after wasting an LLM call.
//
// The runtime-resolution invariant is exercised by the SpecModeGate
// tests (TestSpecModeGate_ForwardsSpecArtifactTriples in the
// phasevalidator package). This contract test pins the textual side
// only; the wire-level guarantee is the gate's tests.
func TestLineageThreading_BuilderPromptSubstitution(t *testing.T) {
	data, err := os.ReadFile("../../configs/rules/dev-via-spec/02-reviewer-approved-to-builder.json")
	if err != nil {
		t.Fatalf("read 02-reviewer-approved-to-builder: %v", err)
	}
	var rule ruleWithRelatedLoops
	if err := json.Unmarshal(data, &rule); err != nil {
		t.Fatalf("unmarshal 02-reviewer-approved-to-builder: %v", err)
	}
	if len(rule.OnEnter) == 0 {
		t.Fatal("02-reviewer-approved-to-builder: on_enter is empty")
	}
	prompt := rule.OnEnter[0].Prompt
	if !strings.Contains(prompt, "$entity.triple.dev_via_spec.artifact.path") {
		t.Errorf("02-reviewer-approved-to-builder prompt must substitute $entity.triple.dev_via_spec.artifact.path so the builder sees the literal spec path for bootstrap_workspace; got prompt:\n%s", prompt)
	}
	if !strings.Contains(strings.ToLower(prompt), "bootstrap_workspace") {
		t.Errorf("02-reviewer-approved-to-builder prompt must reference 'bootstrap_workspace' so the builder knows to seed its sandbox from the spec path")
	}
}
