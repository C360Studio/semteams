package contract

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestResearchPackPersonaDirsExist asserts that every role token the
// research pack spawns (via publish_agent actions) has a corresponding
// persona fragment directory on disk with at least an 00-identity.md
// file.
//
// ADR-042 §Phase 2 redesign MVP-3 introduces the
// `researcher-research-{plan,gather,synthesize}` persona dirs to back
// the MVP-2 rule pack's role tokens. This test catches the drift class
// where a rule spawns a role with no backing persona (silent
// degradation to whatever fallback persona the framework picks) or a
// persona dir is added without a rule that uses it (dead corpus).
//
// The coordinator role is exempt from the persona-dir requirement
// here — it is the substrate's default_role wired by
// flow-bootstrap.json and its corpus lives at
// configs/personas/fragments/coordinator/ (already pinned by the
// adr041 persona-dir tests). reviewer-research is exempt as well —
// it predates MVP-2 and is already covered by the adr041 tests.
//
// What stays in scope here: the three new MVP-3 dirs that this PR
// stands up. As additional category packs ship (web-research,
// dev-via-spec), each new role token they spawn should be enforced
// here too.
func TestResearchPackPersonaDirsExist(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(researchPackDir, "*.json"))
	if err != nil {
		t.Fatalf("glob research pack: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("no rule files found in research pack — wrong working directory?")
	}

	spawned := make(map[string]bool)
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		var rule researchRuleJSON
		if err := json.Unmarshal(data, &rule); err != nil {
			t.Fatalf("unmarshal %s: %v", path, err)
		}
		for _, a := range rule.OnEnter {
			if a.Type == "publish_agent" && a.Role != "" {
				spawned[a.Role] = true
			}
		}
	}

	// Every role the pack spawns must have a persona dir with an
	// 00-identity.md present. Coordinator + reviewer-research are
	// pinned by adr041_persona_dirs_test as well; asserting them
	// here too is defense-in-depth — if a future refactor renames
	// either, both tests fail and the drift surfaces faster.
	root := "../../configs/personas/fragments"
	for role := range spawned {
		identityPath := filepath.Join(root, role, "00-identity.md")
		info, err := os.Stat(identityPath)
		if err != nil {
			t.Errorf("role %q has no persona at %s: %v — add the directory + 00-identity.md, or remove the rule that spawns it",
				role, identityPath, err)
			continue
		}
		if info.Size() == 0 {
			t.Errorf("role %q identity fragment at %s is empty",
				role, identityPath)
		}
	}
}

// TestResearchPackPersonaDirsReferencePhase asserts each
// researcher-research-<phase> identity fragment opens with the
// phase token in its prose. This is the structural signal that the
// fragment was authored for the phase, not stub or copy from another
// role. Re-authoring faithfulness (does the prose teach the phase
// contract properly?) is the reviewer-pass's job, not this test's.
func TestResearchPackPersonaDirsReferencePhase(t *testing.T) {
	root := "../../configs/personas/fragments"
	for phase, expectedToken := range map[string]string{
		"researcher-research-plan":       "PLAN phase",
		"researcher-research-gather":     "GATHER phase",
		"researcher-research-synthesize": "SYNTHESIZE phase",
	} {
		path := filepath.Join(root, phase, "00-identity.md")
		data, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("read %s: %v", path, err)
			continue
		}
		if !strings.Contains(string(data), expectedToken) {
			t.Errorf("identity fragment %s missing phase token %q",
				path, expectedToken)
		}
	}
}

// TestResearchPackPersonaDirsCarryCategoryToken asserts each
// researcher-research-<phase> identity fragment names the `research`
// category explicitly. This pins the naming convention from
// configs/rules/research/README.md — without the category token,
// the persona could drift toward generic-researcher prose that
// downstream readers can't tell apart from the legacy
// researcher-{plan,gather,synthesize} dirs.
func TestResearchPackPersonaDirsCarryCategoryToken(t *testing.T) {
	root := "../../configs/personas/fragments"
	for _, dir := range []string{
		"researcher-research-plan",
		"researcher-research-gather",
		"researcher-research-synthesize",
	} {
		path := filepath.Join(root, dir, "00-identity.md")
		data, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("read %s: %v", path, err)
			continue
		}
		// Either "research category" or "research task category"
		// reads naturally in the prose; accept both.
		text := string(data)
		if !strings.Contains(text, "research category") &&
			!strings.Contains(text, "research task category") {
			t.Errorf("identity fragment %s does not name the `research category` — drift risk vs the legacy researcher-{plan,gather,synthesize} corpora",
				path)
		}
	}
}
