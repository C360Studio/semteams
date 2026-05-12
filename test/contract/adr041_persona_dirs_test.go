package contract

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// TestADR041_NewPersonaDirsExist verifies ADR-041 Phase 1 Slice 1a:
// the new role directories that implement the phase-as-sub-role
// naming scheme exist and contain at minimum the identity fragment.
//
// ADR-041 collapses 13 chain roles to 4 (coordinator, researcher,
// builder, reviewer) — researcher and reviewer are surfaced as
// phase-named sub-roles in the upstream depth-2 persona loader
// (file_loader.go enforces depth-2 only). The dirs below are the
// concrete sub-roles the rule engine will spawn loops into in
// Phase 2.
func TestADR041_NewPersonaDirsExist(t *testing.T) {
	root := "../../configs/personas/fragments"
	for _, dir := range []string{
		"researcher-plan",
		"researcher-gather",
		"researcher-synthesize",
		"researcher-architect",
		"reviewer-spec",
		"reviewer-qa",
		"reviewer-research",
		"builder",
	} {
		identityPath := filepath.Join(root, dir, "00-identity.md")
		info, err := os.Stat(identityPath)
		if err != nil {
			t.Errorf("expected identity fragment at %s: %v", identityPath, err)
			continue
		}
		if info.Size() == 0 {
			t.Errorf("identity fragment at %s is empty", identityPath)
		}
	}
}

// TestADR041_ResearcherPhaseIdentitiesReferenceTheirPhase asserts that
// each researcher phase's identity fragment opens with phase-specific
// framing — not the generic "researcher" framing inherited from the
// upstream copy. This is the structural signal that the content was
// re-authored for the phase, not just copied.
//
// The check is presence-of-phase-token, not prose-style. Re-authoring
// faithfulness (does the prose actually teach the phase contract) is
// graded by the reviewer pass, not by this test.
func TestADR041_ResearcherPhaseIdentitiesReferenceTheirPhase(t *testing.T) {
	root := "../../configs/personas/fragments"
	for phase, expectedToken := range map[string]string{
		"researcher-plan":       "PLAN phase",
		"researcher-gather":     "GATHER phase",
		"researcher-synthesize": "SYNTHESIZE phase",
		"researcher-architect":  "ARCHITECT phase",
	} {
		path := filepath.Join(root, phase, "00-identity.md")
		data, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("read %s: %v", path, err)
			continue
		}
		if !strings.Contains(string(data), expectedToken) {
			t.Errorf("identity fragment %s missing phase token %q; got first line: %s",
				path, expectedToken, firstLine(string(data)))
		}
	}
}

// TestADR041_OldPersonaDirsStillPresent verifies the ADR-041 phasing
// guarantee that Phase 1 is purely additive: the old role dirs remain
// loaded because configs still reference them. Deletion happens in
// Phase 3 once configs are wired to the new roster.
//
// If this test starts failing, either:
//   - Phase 3 has run (in which case update this test alongside the
//     config wiring), OR
//   - someone deleted old dirs prematurely (in which case the
//     existing 13-role configs broke silently).
func TestADR041_OldPersonaDirsStillPresent(t *testing.T) {
	root := "../../configs/personas/fragments"
	for _, dir := range []string{
		"dev-via-spec-planner",
		"dev-via-spec-architect",
		"dev-via-spec-builder",
		"dev-via-spec-reviewer",
		"dev-via-spec-qa-reviewer",
		"dev-via-spec-challenger",
		"research-reviewer",
		"researcher",
		"source-curator",
		"source-registrar",
	} {
		if _, err := os.Stat(filepath.Join(root, dir)); err != nil {
			t.Errorf("old persona dir %s missing (Phase 1 should be additive only): %v", dir, err)
		}
	}
}

// TestADR041_ResearcherPhaseIdentitiesDeclareDecideActions asserts that
// each researcher phase's identity fragment declares the decide-action
// terminals per the ADR-041 phase graph. This catches the protocol-level
// drift the Slice 1a reviewer pass flagged: persona prose referencing
// decide(next_role=...) would silently fail because the upstream decide
// tool doesn't accept a next_role arg — phase routes through action.
//
// The check is presence-of-action-token, not prose-style. Each phase's
// allow-list is enforced by the spawn rules + structural validator in
// Phase 2; this test guards against the persona-side prose drifting away
// from what the validator will accept.
func TestADR041_ResearcherPhaseIdentitiesDeclareDecideActions(t *testing.T) {
	root := "../../configs/personas/fragments"
	// Phase graph from ADR-041 §"Allowed transitions". Each entry is the
	// minimum set of decide-action tokens the identity must mention.
	// Back-edges from synthesize and architect emit the same `gather`
	// token as the forward path; the rule layer disambiguates by
	// reading the spawning loop's input phase, so the persona vocabulary
	// stays single-token per target phase (ADR-041 transition table).
	for phase, expectedActions := range map[string][]string{
		"researcher-plan":       {`action="gather"`, `action="needs_clarification"`, `action="emit"`},
		"researcher-gather":     {`action="synthesize"`, `action="needs_clarification"`},
		"researcher-synthesize": {`action="architect"`, `action="gather"`, `action="needs_clarification"`},
		"researcher-architect":  {`action="emit"`, `action="gather"`, `action="needs_clarification"`},
	} {
		path := filepath.Join(root, phase, "00-identity.md")
		data, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("read %s: %v", path, err)
			continue
		}
		body := string(data)
		// Guard against the wire-format bug that Slice 1a shipped with:
		// next_role is not an accepted decide arg.
		if strings.Contains(body, "next_role") {
			t.Errorf("%s: identity fragment references next_role (not an accepted decide arg); phase routes through action", path)
		}
		for _, action := range expectedActions {
			if !strings.Contains(body, action) {
				t.Errorf("%s: identity fragment missing required decide token %q", path, action)
			}
		}
	}
}

// TestADR041_InterFragmentDecideActionConsistency walks every *.md
// under each researcher-* phase dir and asserts that any `action="X"`
// token in the file appears in the identity's declared allow-list.
// This is the structural test the prior reviewer pass asked for:
// would have mechanically caught the Slice 1c-2-deferred drifts
// (e.g. researcher-plan/10-output-contract.md still terminating with
// action="planned" when the identity only allows gather/emit/
// needs_clarification).
//
// Scope is limited to researcher-* dirs because their allow-lists are
// declared in the ADR-041 phase graph. Reviewer-* dirs have their own
// allow-lists declared in the identities but the inherited fragments
// reference actions that may legitimately route through the chain
// (e.g. builder's tests_passing references), so the same shape of
// test doesn't apply cleanly there yet — handled by the role-specific
// terminal contract instead.
func TestADR041_InterFragmentDecideActionConsistency(t *testing.T) {
	root := "../../configs/personas/fragments"
	// Per ADR-041 phase graph: each phase's full allow-list. Same source
	// of truth as TestADR041_ResearcherPhaseIdentitiesDeclareDecideActions.
	allowLists := map[string]map[string]bool{
		"researcher-plan":       {"gather": true, "needs_clarification": true, "emit": true},
		"researcher-gather":     {"synthesize": true, "needs_clarification": true},
		"researcher-synthesize": {"architect": true, "gather": true, "needs_clarification": true},
		"researcher-architect":  {"emit": true, "gather": true, "needs_clarification": true},
	}
	// Regex for `decide(action="X"` tokens. Anchors on the `decide(`
	// prefix because that's the persona's own terminal call shape.
	// Bare `action="X"` references in prose are typically describing
	// other roles' actions ("the reviewer rejects with action='insufficient'")
	// and aren't drift — only the persona's own decide-call vocabulary
	// must match its allow-list.
	actionRE := regexp.MustCompile(`decide\(action="([a-z_]+)"`)
	for phase, allowed := range allowLists {
		phaseDir := filepath.Join(root, phase)
		entries, err := os.ReadDir(phaseDir)
		if err != nil {
			t.Errorf("readdir %s: %v", phaseDir, err)
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
				continue
			}
			path := filepath.Join(phaseDir, entry.Name())
			data, err := os.ReadFile(path)
			if err != nil {
				t.Errorf("read %s: %v", path, err)
				continue
			}
			for _, match := range actionRE.FindAllStringSubmatch(string(data), -1) {
				action := match[1]
				if !allowed[action] {
					t.Errorf("%s: references action=%q which is not in %s's allow-list %v",
						path, action, phase, sortedKeys(allowed))
				}
			}
		}
	}
}

// sortedKeys returns map keys in deterministic order for error messages.
func sortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// TestADR041_EmitToolPhaseOwnership asserts that each phase-scoped
// emit-tool call appears only in its owning phase's persona dir.
// Same drift class as TestADR041_InterFragmentDecideActionConsistency
// but for the emit-tool surface — would have caught the Slice 1c-1
// emit-contradiction in researcher-gather (where 3 fragments
// demanded emit_research_artifact while the identity forbade it).
//
// The owning-phase mapping comes from ADR-041's phase contract:
// each emit tool belongs to exactly one phase. A reference outside
// that phase (or in any other role's dir) signals drift.
func TestADR041_EmitToolPhaseOwnership(t *testing.T) {
	root := "../../configs/personas/fragments"
	emitTools := map[string]string{
		"emit_plan":                  "researcher-plan",
		"emit_research_artifact":     "researcher-synthesize",
		"emit_dev_via_spec_artifact": "researcher-architect",
	}
	dirs, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("readdir %s: %v", root, err)
	}
	for tool, owner := range emitTools {
		needle := tool + "("
		for _, d := range dirs {
			if !d.IsDir() {
				continue
			}
			roleName := d.Name()
			if roleName == owner {
				continue
			}
			roleDir := filepath.Join(root, roleName)
			entries, err := os.ReadDir(roleDir)
			if err != nil {
				t.Errorf("readdir %s: %v", roleDir, err)
				continue
			}
			for _, entry := range entries {
				if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
					continue
				}
				path := filepath.Join(roleDir, entry.Name())
				data, err := os.ReadFile(path)
				if err != nil {
					t.Errorf("read %s: %v", path, err)
					continue
				}
				// Skip the legacy persona dirs that ADR-041 will delete in
				// Phase 3 — they keep their original content for as long as
				// configs reference them. The new dirs are the ones under
				// scrutiny.
				if isLegacyDir(roleName) {
					continue
				}
				if strings.Contains(string(data), needle) {
					t.Errorf("%s: references %s%s but that emit tool belongs to phase %q",
						path, tool, "(", owner)
				}
			}
		}
	}
}

// isLegacyDir returns true for the pre-ADR-041 persona dirs that
// stay loaded during Phase 1+2 because configs still reference them.
// They are exempt from the ADR-041 ownership invariants because their
// purpose is to keep the legacy chain working until Phase 3.
func isLegacyDir(name string) bool {
	switch name {
	case
		"dev-via-spec-architect",
		"dev-via-spec-builder",
		"dev-via-spec-challenger",
		"dev-via-spec-planner",
		"dev-via-spec-qa-reviewer",
		"dev-via-spec-reviewer",
		"research-reviewer",
		"researcher",
		"source-curator",
		"source-registrar":
		return true
	}
	return false
}

// TestADR041_ResearcherPhaseIdentitiesRejectStaleTokens asserts that
// the researcher phase identities do NOT carry vocabulary inherited
// from the old dev-via-spec chain. The chain that ADR-041 replaces
// terminated researcher / planner / architect with decide actions
// like "planned" / "approved" (reviewer) / "challenger_accepted" /
// "dev_complete"; under the compressed roster these tokens are stale
// and emitting them from a new-phase persona prompt would route
// nowhere (no rule fires on them).
//
// This is an inverse check to TestADR041_ResearcherPhaseIdentitiesDeclareDecideActions
// — that test verifies presence of the expected token set; this one
// rejects presence of the dev-via-spec-vocabulary tokens. Catches
// drift-back regressions during slice-by-slice rewrites of the
// inherited fragments.
//
// Scope is limited to 00-identity.md per phase. The inherited
// fragments (10/20/30/40-*.md) still carry stale vocabulary as of
// Slice 1c-1 and are tracked for Slice 1c-2; this test guards the
// identity surface, which is the prompt's load-bearing top.
func TestADR041_ResearcherPhaseIdentitiesRejectStaleTokens(t *testing.T) {
	root := "../../configs/personas/fragments"
	staleTokens := []string{
		`action="planned"`,
		`action="dev_complete"`,
		`action="challenger_accepted"`,
		`action="research_completed"`,
	}
	for _, phase := range []string{
		"researcher-plan",
		"researcher-gather",
		"researcher-synthesize",
		"researcher-architect",
	} {
		path := filepath.Join(root, phase, "00-identity.md")
		data, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("read %s: %v", path, err)
			continue
		}
		body := string(data)
		for _, stale := range staleTokens {
			if strings.Contains(body, stale) {
				t.Errorf("%s: identity fragment references stale dev-via-spec token %q (ADR-041 phase vocabulary uses gather/synthesize/architect/emit/needs_clarification)", path, stale)
			}
		}
	}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// TestADR041_ReviewerSpecArtifactShape asserts that the reviewer-spec
// fragments evaluate the SPEC ARTIFACT shape
// (actors[]/integration_points[]/tasks[]/checks[]) emitted by
// researcher-architect via emit_dev_via_spec_artifact, NOT the old
// plan-shape vocabulary (goal/context/scope/epics) inherited from the
// dev-via-spec-reviewer fragments that the Phase 1 port re-homed.
//
// reviewer-spec is operationally no-op as of Phase 1 (no rule spawns
// it yet); the rewrite must land BEFORE Phase 2 rule wiring or every
// architect-emit artifact will draw a false-insufficient verdict
// because the reviewer is grading against plan-shape vocabulary the
// architect's structured artifact does not produce.
//
// Catches regression where someone re-introduces plan-shape "epic"
// decomposition prescription, "scope_in/scope_out" reasoning, or
// "Verifiable Outcomes section" framing into reviewer-spec evaluation
// fragments.
func TestADR041_ReviewerSpecArtifactShape(t *testing.T) {
	root := "../../configs/personas/fragments/reviewer-spec"
	// Required: each of these artifact-field references must appear in
	// at least one reviewer-spec fragment. If reviewer-spec doesn't
	// reference the structured fields, it isn't grading the artifact.
	requiredFieldRefs := []string{
		"actors[]",
		"integration_points[]",
		"tasks[]",
		"checks[]",
	}
	// Forbidden: prescriptive plan-shape vocabulary that the rewrite
	// retired. Mentioning them in a "not valid grounds for insufficient"
	// counterexample is fine; using them as the prescription is the
	// regression.
	forbiddenPrescriptions := []string{
		"Epic decomposition",
		"epic decomposition",
		"epics",
		"Verifiable Outcomes section",
		// scope_in/scope_out were the plan-shape's IN/OUT lists. The
		// spec artifact replaces them with tasks[]+integration_points[].
		"scope_in",
		"scope_out",
		// seed_requirement was the dev-via-spec-reviewer's plan-shape
		// reference to a planner's requirement-numbering scheme that
		// doesn't survive into the spec artifact's tasks[]/checks[]
		// shape.
		"seed_requirement",
	}

	files, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read reviewer-spec dir: %v", err)
	}

	combined := make([]byte, 0, 8192)
	for _, f := range files {
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".md") {
			continue
		}
		path := filepath.Join(root, f.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("read %s: %v", path, err)
			continue
		}
		body := string(data)
		// Forbidden-prescription check is per-file so the error message
		// names the file.
		for _, forbidden := range forbiddenPrescriptions {
			if strings.Contains(body, forbidden) {
				t.Errorf("%s: contains stale plan-shape prescription %q (reviewer-spec evaluates the spec artifact's structured fields under ADR-041; rewrite the section to artifact-shape)", path, forbidden)
			}
		}
		combined = append(combined, data...)
		combined = append(combined, '\n')
	}

	// Required-field-reference check is across all files combined: at
	// least one fragment must mention each artifact field. The
	// completeness checklist (20) carries most; 30 carries checks[]
	// substance; 10 covers the overall shape.
	combinedStr := string(combined)
	for _, field := range requiredFieldRefs {
		if !strings.Contains(combinedStr, field) {
			t.Errorf("reviewer-spec fragments do not reference artifact field %q anywhere (ADR-041 reviewer-spec grades against the spec artifact's structured shape — actors[]/integration_points[]/tasks[]/checks[])", field)
		}
	}
}

// TestADR041_ReviewerResearchArtifactShape asserts that the
// reviewer-research fragments evaluate the RESEARCH ARTIFACT shape
// (actors[]/integration_points[]/tasks[]/addressed_gaps[]/
// open_gaps[]/test_harness/substrate_mutations[]/revision) emitted
// by researcher-synthesize via emit_research_artifact, NOT the spec
// artifact shape (goal/context/checks[]/provenance + Task-with-grounds)
// that reviewer-spec grades.
//
// Per ADR-041 §addendum 2026-05-12 "reviewer-research collapse-vs-keep
// decision" the reviewer role has three modes (research/spec/qa).
// Each mode evaluates a different artifact shape; mixing the
// vocabularies forces the LLM to disambiguate which contract applies
// and grades the wrong fields.
//
// Note: `actors[]`, `integration_points[]`, and `tasks[]` appear on
// both artifacts (with different sub-shapes) — they're SHARED fields,
// not distinguishing ones. The forbidden-prescription set is the
// spec-EXCLUSIVE field surface (goal/context/checks[]/provenance/
// grounds_actors/grounds_integration_points/emit_dev_via_spec_artifact).
//
// Catches regression where someone re-edits reviewer-research toward
// spec-artifact vocabulary — that's reviewer-spec's surface, not
// this one.
func TestADR041_ReviewerResearchArtifactShape(t *testing.T) {
	root := "../../configs/personas/fragments/reviewer-research"
	// Required: research artifact's structured fields per
	// cmd/semteams/research/artifact.go:98-141. Combined corpus must
	// reference each (some may legitimately appear in only one
	// fragment — e.g. test_harness in the harness-gate,
	// substrate_mutations + revision in the stabilisation-check).
	requiredFieldRefs := []string{
		"actors",
		"integration_points",
		"tasks",
		"open_gaps",
		"addressed_gaps",
		"test_harness",
		"substrate_mutations",
		"revision",
	}
	// Forbidden: spec-EXCLUSIVE field surface. reviewer-research grades
	// the research artifact, not the spec artifact. Substring matches
	// are anchored where the bare token would false-positive on common
	// English ("goal" → "`goal`" + "## Goal" patterns; "context" →
	// "`context`" + "## Context" patterns) — prose like "the artifact's
	// goal is to..." stays legitimate.
	forbiddenPrescriptions := []string{
		"checks[]",
		"`goal`",
		"## Goal",
		"`context`",
		"## Context",
		"`provenance`",
		"grounds_actors",
		"grounds_integration_points",
		"emit_dev_via_spec_artifact",
	}

	files, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read reviewer-research dir: %v", err)
	}

	combined := make([]byte, 0, 8192)
	for _, f := range files {
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".md") {
			continue
		}
		path := filepath.Join(root, f.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("read %s: %v", path, err)
			continue
		}
		body := string(data)
		for _, forbidden := range forbiddenPrescriptions {
			if strings.Contains(body, forbidden) {
				t.Errorf("%s: contains spec-exclusive prescription %q (reviewer-research evaluates the research artifact under ADR-041; spec-only field surfaces — goal/context/checks[]/provenance + Task-with-grounds — belong to reviewer-spec)", path, forbidden)
			}
		}
		combined = append(combined, data...)
		combined = append(combined, '\n')
	}

	combinedStr := string(combined)
	for _, field := range requiredFieldRefs {
		if !strings.Contains(combinedStr, field) {
			t.Errorf("reviewer-research fragments do not reference research-artifact field %q anywhere (ADR-041 reviewer-research grades against the research artifact's structured shape — actors[]/integration_points[]/tasks[]/addressed_gaps[]/open_gaps[]/test_harness/substrate_mutations[]/revision)", field)
		}
	}
}
