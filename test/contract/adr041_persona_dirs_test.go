package contract

import (
	"os"
	"path/filepath"
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
	for phase, expectedActions := range map[string][]string{
		"researcher-plan":       {`action="gather"`, `action="needs_clarification"`, `action="emit"`},
		"researcher-gather":     {`action="synthesize"`, `action="needs_clarification"`},
		"researcher-synthesize": {`action="architect"`, `action="regather"`, `action="needs_clarification"`},
		"researcher-architect":  {`action="emit"`, `action="regather"`, `action="needs_clarification"`},
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

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
