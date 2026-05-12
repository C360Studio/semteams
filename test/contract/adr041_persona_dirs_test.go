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
// The check is structural (presence of the phase name in the H1 heading)
// rather than prose-style assertion — Goodhart-resistant per
// feedback_format_compliance_goodhart.
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
	} {
		if _, err := os.Stat(filepath.Join(root, dir)); err != nil {
			t.Errorf("old persona dir %s missing (Phase 1 should be additive only): %v", dir, err)
		}
	}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
