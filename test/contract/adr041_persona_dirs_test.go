package contract

import (
	"encoding/json"
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

// TestADR041_LegacyPersonaDirsDeleted asserts the ADR-041 Phase 3
// deletion guarantee: the 7 legacy dev-via-spec-* + source-curator
// persona dirs are gone. Inverts the Phase 1's additive-only guard
// once configs were re-wired to the MVP roster (Slice 2D-3a swapped
// rule role names from `dev-via-spec-*` to `researcher-* / builder /
// reviewer-*`; Slice 2D-1 removed the curator spawn rules; Phase 3
// deleted the inert persona dirs).
//
// The retained-legacy set (researcher, research-reviewer,
// source-registrar) stays gated by TestADR041_RetainedLegacyPersonaDirs
// below — their continued presence is the wiring contract for active
// research-iterative + e2e-research-with-source configs, not Phase 3
// cleanup target.
func TestADR041_LegacyPersonaDirsDeleted(t *testing.T) {
	root := "../../configs/personas/fragments"
	for _, dir := range []string{
		"dev-via-spec-planner",
		"dev-via-spec-architect",
		"dev-via-spec-builder",
		"dev-via-spec-reviewer",
		"dev-via-spec-qa-reviewer",
		"dev-via-spec-challenger",
		"source-curator",
	} {
		path := filepath.Join(root, dir)
		if _, err := os.Stat(path); err == nil {
			t.Errorf("legacy persona dir %s still present (ADR-041 Phase 3 deletion); inverse-guard regression — adding fragments back would silently spawn under retired roles since no rule names them", path)
		}
	}
}

// TestADR041_RetainedLegacyPersonaDirs locks the three legacy roles
// that ADR-041 Phase 3 intentionally retained — each has an active
// consumer (per the comment) and removing them needs a separate
// architectural call.
//
//   - `researcher` — spawned by coordinator/01-delegate-research.json
//     (coordinator front-door pattern) + default_role in 3 e2e configs.
//   - `research-reviewer` — spawned by research-iterative/01-research-to-reviewer.json.
//   - `source-registrar` — default_role in e2e-research-with-source.json
//     (R2 source-acquisition journey).
//
// When any of these consumers retire, prune the corresponding entry
// from this test and the persona dir alongside the wiring.
func TestADR041_RetainedLegacyPersonaDirs(t *testing.T) {
	root := "../../configs/personas/fragments"
	for _, dir := range []string{
		"researcher",
		"research-reviewer",
		"source-registrar",
	} {
		path := filepath.Join(root, dir, "00-identity.md")
		if _, err := os.Stat(path); err != nil {
			t.Errorf("retained-legacy persona dir %s identity fragment missing: %v (if intentional, also remove %s from this test and prune the consumer wiring)", path, err, dir)
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

// TestADR041_ReviewerQAArtifactShape asserts that the reviewer-qa
// fragments grade two surfaces consistent with ADR-041 wiring:
//   - the BUILDER TERMINAL via `read_loop_result` (action token set:
//     tests_passing / tests_failing) for the build-outcome side
//   - the EVIDENCE-GATE SUMMARY rendered server-side by
//     `evidence.Summarize` and injected as the `evidence_summary`
//     task property, blocks headed `## Check N — <target>` with an
//     Aggregate header per the gate's closed status enum (pass / fail
//     / unknown_kind / error)
//
// reviewer-qa grades a different surface from reviewer-spec and
// reviewer-research. spec grades the artifact's checks/actors/
// integration_points/tasks substance BEFORE the build; QA grades
// only `checks[]` evidence AFTER the build. research grades the
// research artifact's addressed_gaps/open_gaps/test_harness shape.
//
// The forbidden set catches three drift classes:
//
//  1. The pre-ADR-041 vocabulary drift where reviewer-qa prose said
//     "## Commitment N" while the actual `evidence.Summarize` renderer
//     emits "## Check N" (cmd/semteams/evidence/summary.go:66). The
//     wrong block heading wedges the LLM's pattern-match against the
//     real summary at runtime.
//  2. Cross-mode pollution from reviewer-spec's evaluation surface
//     (actors[] / integration_points[]) — reviewer-qa grades evidence,
//     not spec substance.
//  3. Cross-mode pollution from reviewer-research's exclusive
//     fields (addressed_gaps / open_gaps / substrate_mutations) —
//     those belong to the research artifact, not the spec artifact.
func TestADR041_ReviewerQAArtifactShape(t *testing.T) {
	root := "../../configs/personas/fragments/reviewer-qa"
	// Required: each must appear at least once across the corpus.
	// These are the surfaces reviewer-qa is *defined* to grade —
	// drift-back would silently produce a verdict that doesn't match
	// the wire shape it reads.
	requiredFieldRefs := []string{
		// Builder-terminal channel. The builder calls `builder_decide`
		// (not `decide`), so the action tokens appear in fragments as
		// backtick-quoted code spans or in `builder.action == ...`
		// prose form — not as `decide(action="...")`. Substring-only
		// check is the right grain.
		"read_loop_result",
		"tests_passing",
		"tests_failing",
		// Evidence-summary channel
		"evidence_summary",
		"Aggregate",
		"checks[]",
		// Rendered block heading the gate actually emits
		// (evidence/summary.go:66 fmt.Sprintf("## Check %d — %s")).
		"## Check",
		// Closed status enum from the gate (the four values
		// reviewer-qa branches on).
		"unknown_kind",
	}
	// Forbidden: drift classes per the docstring above. Anchored so
	// common English doesn't false-positive.
	forbiddenPrescriptions := []string{
		// Block-heading drift: gate renders "## Check N", not
		// "## Commitment N". Pre-ADR-041 reviewer-qa prose used the
		// stale vocabulary in two surfaces; rewrite per Slice 2D-5a
		// normalised on the renderer's actual heading.
		"## Commitment",
		"Commitment ",
		"commitments",
		// Spec-grade surface that reviewer-qa is NOT supposed to
		// re-litigate (reviewer-spec already accepted upstream).
		"actors[]",
		"integration_points[]",
		// Research-artifact-exclusive fields (reviewer-research's
		// surface, not reviewer-qa's).
		"addressed_gaps",
		"open_gaps",
		"substrate_mutations",
		// Wire-format bug guard (carried from
		// TestADR041_ResearcherPhaseIdentitiesDeclareDecideActions):
		// next_role is not an accepted decide arg under ADR-041.
		"next_role",
	}

	files, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read reviewer-qa dir: %v", err)
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
				t.Errorf("%s: contains forbidden token %q (reviewer-qa grades the builder terminal + the evidence-gate summary under ADR-041; commitment/spec-shape/research-shape vocabulary is not its surface)", path, forbidden)
			}
		}
		combined = append(combined, data...)
		combined = append(combined, '\n')
	}

	combinedStr := string(combined)
	for _, field := range requiredFieldRefs {
		if !strings.Contains(combinedStr, field) {
			t.Errorf("reviewer-qa fragments do not reference required surface %q anywhere (ADR-041 reviewer-qa grades the builder terminal via read_loop_result + the evidence_summary task property; the rendered block heading is `## Check N — <target>` and the closed status enum is pass/fail/unknown_kind/error)", field)
		}
	}
}

// TestADR041_RulePublishAgentRoleResolvesToPersonaDir asserts that every
// `publish_agent` rule action loaded by an active config names a `role`
// whose persona directory exists under
// `configs/personas/fragments/<role>/` with at minimum the
// 00-identity.md fragment. This is the wiring contract that turns the
// rule-spawn path into a properly-grounded loop.
//
// Why it matters: the upstream persona file-loader
// (persona/file_loader.go:142 — `role := parts[0]`) keys persona
// fragments by the literal directory name. A rule emitting
// `role: "reviewer-spec"` but missing `configs/personas/fragments/reviewer-spec/`
// silently spawns the loop with no persona prompt — the LLM gets the
// task-shell only and either no-ops or hallucinates a verdict.
//
// Phase 1 reviewer's todo #4 named this test as Slice 2D follow-up:
// it could not be written until the rule-spawn paths existed (Slice
// 2D-3a, commits 7af8040 + 730280e + 166f39e). Now they do, so this
// test guards against drift on either side — renaming a persona dir
// without re-wiring the rule, or adding a rule without authoring the
// persona dir.
//
// Scope: rules referenced by a config's `rules_files` list. The
// configs/rules/ tree also carries orphan dirs (agentic-workflow/,
// coordination/, escalation/, routing/, boid/, approval/, memory/)
// that mirror upstream samples but aren't loaded by any active
// config — they describe non-MVP roles (editor, general, etc.) that
// were never product-shelled into semteams. Phase 3 deletion sweep
// owns those; this test guards the runtime-wired surface.
func TestADR041_RulePublishAgentRoleResolvesToPersonaDir(t *testing.T) {
	configsRoot := "../../configs"
	personaRoot := "../../configs/personas/fragments"

	type publishAgentAction struct {
		Type string `json:"type"`
		Role string `json:"role"`
	}
	type rule struct {
		ID      string               `json:"id"`
		OnEnter []publishAgentAction `json:"on_enter"`
		OnExit  []publishAgentAction `json:"on_exit"`
	}
	type processorConfig struct {
		Name   string `json:"name"`
		Config struct {
			RulesFiles []string `json:"rules_files"`
		} `json:"config"`
	}
	type appConfig struct {
		Components map[string]processorConfig `json:"components"`
	}

	// Gather the rules_files referenced across all configs in
	// configs/*.json. Each path is "/app/configs/rules/..."; translate to
	// the local checkout root.
	configFiles, err := filepath.Glob(filepath.Join(configsRoot, "*.json"))
	if err != nil {
		t.Fatalf("glob configs: %v", err)
	}
	wiredRules := make(map[string]struct{})
	for _, cf := range configFiles {
		data, readErr := os.ReadFile(cf)
		if readErr != nil {
			t.Errorf("read config %s: %v", cf, readErr)
			continue
		}
		var ac appConfig
		if err := json.Unmarshal(data, &ac); err != nil {
			// Some configs may not unmarshal cleanly into this shape
			// (e.g. shapes with array-valued components). Skip rather
			// than fail — the rule-wiring contract only cares about
			// the configs that DO declare rules_files.
			continue
		}
		for _, comp := range ac.Components {
			for _, rf := range comp.Config.RulesFiles {
				// "/app/configs/rules/dev-via-spec/02-...json" →
				// "../../configs/rules/dev-via-spec/02-...json".
				local := strings.TrimPrefix(rf, "/app/configs/rules/")
				wiredRules[filepath.Join("../../configs/rules", local)] = struct{}{}
			}
		}
	}
	if len(wiredRules) == 0 {
		t.Fatal("no rules_files found across configs/*.json — test is structurally broken (or all configs were renamed)")
	}

	for rulePath := range wiredRules {
		data, readErr := os.ReadFile(rulePath)
		if readErr != nil {
			t.Errorf("read wired rule %s: %v", rulePath, readErr)
			continue
		}
		var r rule
		if err := json.Unmarshal(data, &r); err != nil {
			t.Errorf("parse wired rule %s: %v", rulePath, err)
			continue
		}
		actions := append([]publishAgentAction{}, r.OnEnter...)
		actions = append(actions, r.OnExit...)
		for _, action := range actions {
			if action.Type != "publish_agent" {
				continue
			}
			if action.Role == "" {
				// Rules that publish to the agentic-dispatch path (no
				// explicit role; coordinator decides) skip the check —
				// the dispatch component owns role resolution.
				continue
			}
			identityPath := filepath.Join(personaRoot, action.Role, "00-identity.md")
			info, statErr := os.Stat(identityPath)
			if statErr != nil {
				t.Errorf("%s (rule %q): publish_agent role %q does not resolve to a persona dir (expected %s): %v",
					rulePath, r.ID, action.Role, identityPath, statErr)
				continue
			}
			if info.Size() == 0 {
				t.Errorf("%s (rule %q): publish_agent role %q resolves to %s but the identity fragment is empty",
					rulePath, r.ID, action.Role, identityPath)
			}
		}
	}
}

// Smoke #24 (2026-05-14) finding: read_loop_result on the synthesize loop
// returns only the decide() reason, not the typed emit_research_artifact
// payload. The architect must `bash cat` the rendered markdown to access
// the structured artifact (actors/integration_points/tasks). The
// architect-spawn rule (06-phase-transition-to-architect.json) substitutes
// $entity.triple.research.artifact.path so the architect can bash a real
// path that emit_research_artifact stamped — NOT a hardcoded constant.
//
// This test pins:
//  1. The substitution token appears in the rule's prompt body.
//  2. The token is referenced inside a bash invocation (regression guard:
//     someone might delete the bash step while keeping the path
//     description prose).
//  3. The rule still allows bash + emit_dev_via_spec_artifact in tools.
//
// Failure mode if regressed: smoke #24 reproduces — architect terminates
// with needs_clarification when bash hits a non-existent worktree or
// when read_loop_result is mistakenly relied on for the artifact body.
func TestADR041_ArchitectSpawnRuleSubstitutesResearchArtifactPath(t *testing.T) {
	const rulePath = "../../configs/rules/research-mode-transition/06-phase-transition-to-architect.json"
	data, err := os.ReadFile(rulePath)
	if err != nil {
		t.Fatalf("read %s: %v", rulePath, err)
	}

	var r struct {
		OnEnter []struct {
			Role   string   `json:"role"`
			Tools  []string `json:"tools"`
			Prompt string   `json:"prompt"`
		} `json:"on_enter"`
	}
	if err := json.Unmarshal(data, &r); err != nil {
		t.Fatalf("parse %s: %v", rulePath, err)
	}
	if len(r.OnEnter) == 0 {
		t.Fatalf("%s has no on_enter actions", rulePath)
	}

	action := r.OnEnter[0]
	if action.Role != "researcher-architect" {
		t.Errorf("on_enter[0].role = %q, want %q", action.Role, "researcher-architect")
	}

	// (1) substitution token present
	const token = "$entity.triple.research.artifact.path"
	if !strings.Contains(action.Prompt, token) {
		t.Errorf("rule 06 prompt missing substitution token %q. Smoke #24 finding: architect cannot reach the rendered research artifact via read_loop_result; bash cat of the substituted path is the read channel. Without the substitution the chain wedges at architect.\n\nPrompt body:\n%s", token, action.Prompt)
	}

	// (2) substitution appears inside a bash-cat invocation. We pin the
	// exact string `cat $entity.triple.research.artifact.path` — that's
	// the literal pattern the LLM must emit. Matching "bash" alone is a
	// false-positive trap (the word "sandbox" contains it); matching
	// just the token is too loose (could be in descriptive prose only).
	const invocation = "cat " + token
	if !strings.Contains(action.Prompt, invocation) {
		t.Errorf("rule 06 prompt does not contain the literal bash invocation %q. The architect must be told to `cat` the substituted path explicitly — descriptive mention of the token is not enough; the LLM needs the exact shell command. Prompt body:\n%s",
			invocation, action.Prompt)
	}

	// (3) bash is still in the tools allowlist (regression guard for the
	// tool-list pullback that motivated the earlier 3f07c72 patch).
	hasBash := false
	hasEmitSpec := false
	for _, tool := range action.Tools {
		if tool == "bash" {
			hasBash = true
		}
		if tool == "emit_dev_via_spec_artifact" {
			hasEmitSpec = true
		}
	}
	if !hasBash {
		t.Errorf("rule 06 tools missing `bash`; the substitution prose tells the LLM to bash but the tool isn't enabled: %v", action.Tools)
	}
	if !hasEmitSpec {
		t.Errorf("rule 06 tools missing `emit_dev_via_spec_artifact`; the architect can't emit its terminal artifact: %v", action.Tools)
	}

	// (4) Persona-rule alignment: the architect persona's output contract
	// must also reference the same substitution token. Drift here means
	// the rule prompt names the path channel but the persona is teaching
	// a different read pattern — the smoke #24 wedge mode in fast-forward.
	const personaPath = "../../configs/personas/fragments/researcher-architect/10-output-contract.md"
	personaData, err := os.ReadFile(personaPath)
	if err != nil {
		t.Fatalf("read persona %s: %v", personaPath, err)
	}
	if !strings.Contains(string(personaData), token) {
		t.Errorf("persona fragment %s missing substitution token %q. The rule prompt teaches the bash-cat path channel; the persona must reference the same token or the LLM has two competing read patterns. Failure mode is smoke #24 in fast-forward.",
			personaPath, token)
	}
	if !strings.Contains(string(personaData), invocation) {
		t.Errorf("persona fragment %s missing literal invocation %q. The rule prompt and persona must teach the same exact shell command — partial mention of the token in descriptive prose isn't enough; the LLM needs the exact `cat <token>` shape in both surfaces.",
			personaPath, invocation)
	}
}

// In-flight ops observation rule (configs/rules/ops/observe-chain-progress.json)
// fires every N non-terminal loop completions to wake an
// ops-progress-observer that assesses chain liveness. Pins:
//
//  1. fire_every_n_events is present and > 0 (the trigger that makes
//     this rule in-flight rather than terminal — a regression to a
//     pure conditional trigger would silently turn this into a
//     duplicate of chain-terminal-observe.json).
//  2. Conditions exclude reviewer-qa (terminal, handled by the
//     sibling rule) and the three ops-* roles (infinite-recursion
//     guard — an ops observer completing must not fire another).
//  3. Spawned role is ops-progress-observer, and the persona dir
//     exists with a non-empty identity fragment.
//  4. The persona's identity fragment names "in-flight" so an author
//     refactoring the role can't accidentally collapse it into the
//     terminal observer.
//
// Motivated by smoke #25c (2026-05-14): a 12-min builder stall
// surfaced no graph-level signal until the watcher timeout. The
// in-flight observer is the prospective complement to the
// retrospective terminal observer.
type opsInFlightRule struct {
	FireEveryNEvents int `json:"fire_every_n_events"`
	Entity           struct {
		Pattern string `json:"pattern"`
	} `json:"entity"`
	Conditions []struct {
		Field    string `json:"field"`
		Operator string `json:"operator"`
		Value    any    `json:"value"`
	} `json:"conditions"`
	OnEnter []struct {
		Role  string   `json:"role"`
		Tools []string `json:"tools"`
	} `json:"on_enter"`
}

// findListCondition pulls the (operator, []string-value) pair from a
// rule condition whose field matches and whose value is a JSON array of
// strings. Fatals if the field isn't found or the value isn't list-shaped.
func findListCondition(t *testing.T, r opsInFlightRule, field string) (string, []string) {
	t.Helper()
	for _, c := range r.Conditions {
		if c.Field != field {
			continue
		}
		vals, ok := c.Value.([]any)
		if !ok {
			t.Fatalf("rule condition for field %q has non-list value %T; cannot extract enum", field, c.Value)
		}
		strs := make([]string, 0, len(vals))
		for _, v := range vals {
			if s, ok := v.(string); ok {
				strs = append(strs, s)
			}
		}
		return c.Operator, strs
	}
	t.Fatalf("rule has no condition for field %q", field)
	return "", nil
}

func TestADR041_OpsInFlightObservationRule(t *testing.T) {
	const rulePath = "../../configs/rules/ops/observe-chain-progress.json"
	const personaIdentity = "../../configs/personas/fragments/ops-progress-observer/00-identity.md"
	data, err := os.ReadFile(rulePath)
	if err != nil {
		t.Fatalf("read %s: %v", rulePath, err)
	}
	var r opsInFlightRule
	if err := json.Unmarshal(data, &r); err != nil {
		t.Fatalf("parse %s: %v", rulePath, err)
	}

	// (1) fire_every_n_events present + > 0 — the in-flight trigger.
	// Without this the rule degenerates into chain-terminal-observe.json.
	if r.FireEveryNEvents <= 0 {
		t.Errorf("rule fire_every_n_events = %d, want > 0", r.FireEveryNEvents)
	}

	// (1b) entity.pattern must scope the rule to agent.complete.> events.
	// Smoke #26 (2026-05-14) finding: without entity.pattern the rule
	// subscribes to ">" (all subjects), and the entity-state-watch path
	// evaluates the rule on every triple-write UPDATE during loop-
	// completion finalization (~20 evaluations per loop instead of one).
	// fire_every_n_events:5 then samples 1 in 5 evaluations, not 1 in 5
	// loop completions — yielding 40-50 fires per chain instead of the
	// intended 1-2. agent.complete.> is the right scope: one event per
	// loop completion. The persona prose ("every N loop completions")
	// only matches the implementation when this pattern is set.
	if r.Entity.Pattern != "agent.complete.>" {
		t.Errorf("rule entity.pattern = %q, want %q. Without this scope the rule fires on every entity-state UPDATE (per triple write), not per loop completion — fire_every_n_events:5 then yields ~10× the intended fire rate.",
			r.Entity.Pattern, "agent.complete.>")
	}

	// (2) Role exclusion: reviewer-qa (terminal — handled by sibling
	// rule) + the three ops-* roles (infinite-recursion guard).
	roleOp, roleValues := findListCondition(t, r, "agent.loop.role")
	if roleOp != "not_in" {
		t.Errorf("role-condition operator = %q, want %q", roleOp, "not_in")
	}
	mustExclude := map[string]bool{
		"reviewer-qa":           true,
		"ops-chain-observer":    true,
		"ops-progress-observer": true,
		"ops-analyst":           true,
	}
	for _, v := range roleValues {
		delete(mustExclude, v)
	}
	if len(mustExclude) > 0 {
		missing := make([]string, 0, len(mustExclude))
		for k := range mustExclude {
			missing = append(missing, k)
		}
		t.Errorf("role-condition not_in missing required exclusions %v (reviewer-qa overlap duplicates terminal observer; ops-* self-reference spins into infinite recursion)", missing)
	}

	// (3) Outcome enum must match framework's actual stamp values.
	// semstreams beta.72 stamps success/failed/cancelled. "failure"
	// (the framework uses "failed") and "timeout" (that's a
	// ToolErrorKind, not a loop outcome) are the common drift tokens.
	// Smoke #25c — the rule's motivating wedge — is the failed-builder
	// case; wrong tokens silently skip the wedge.
	_, outcomeValues := findListCondition(t, r, "agent.loop.outcome")
	validOutcomes := map[string]bool{"success": true, "failed": true, "cancelled": true}
	for _, v := range outcomeValues {
		if !validOutcomes[v] {
			t.Errorf("agent.loop.outcome value %q is not a framework-stamped outcome (valid: {success,failed,cancelled} per beta.72 agentic/constants.go)", v)
		}
	}

	// (4) Spawn action: role + persona dir + read-only tools allowlist.
	assertInFlightSpawnAction(t, r, personaIdentity)
}

// assertInFlightSpawnAction checks the rule's on_enter[0] action:
// role name, persona dir + identity fragment, "in-flight" token in
// the identity prose (refactor-into-terminal-observer guard), and
// the Phase 1 read-only tools allowlist (no write-tools — change-
// proposal tools land in Phase 2 behind an approval filter).
func assertInFlightSpawnAction(t *testing.T, r opsInFlightRule, personaIdentity string) {
	t.Helper()
	if len(r.OnEnter) == 0 {
		t.Fatalf("rule has no on_enter actions")
	}
	action := r.OnEnter[0]
	if action.Role != "ops-progress-observer" {
		t.Errorf("on_enter[0].role = %q, want %q", action.Role, "ops-progress-observer")
	}

	info, err := os.Stat(personaIdentity)
	if err != nil {
		t.Errorf("persona identity %s missing: %v", personaIdentity, err)
	} else if info.Size() == 0 {
		t.Errorf("persona identity %s is empty", personaIdentity)
	}
	if body, err := os.ReadFile(personaIdentity); err == nil {
		if !strings.Contains(strings.ToLower(string(body)), "in-flight") {
			t.Errorf("persona identity %s does not mention 'in-flight' (refactor-into-terminal-observer guard)", personaIdentity)
		}
	}

	forbiddenTools := map[string]bool{
		"create_rule":          true,
		"manage_flow":          true,
		"update_persona":       true,
		"manage_persona":       true,
		"manage_flow_template": true,
	}
	for _, tool := range action.Tools {
		if forbiddenTools[tool] {
			t.Errorf("tools allowlist contains forbidden Phase 1 write-tool %q (ADR-027 Phase 1 is read-only; tools today: %v)", tool, action.Tools)
		}
	}
}
