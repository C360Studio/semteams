package contract

import (
	"os"
	"strings"
	"testing"
)

// ADR-053 §4b — the autonomous coordinator persona overlay.
//
// Autonomous deployments (agentic-tools.restricted_decide_actions=["ask_user"])
// load configs/personas/fragments-autonomous on top of the base coordinator
// fragments via the -persona-overlay seam (loadPersonaFragments, ADR-029). The
// overlay fragment tells the coordinator UPFRONT that ask_user is barred so it
// resolves ambiguity with respond_direct instead of burning an iteration on a
// policy-rejected ask_user. These are CHEAP structural pins against silent
// reversion — the behavioral effect (the LLM skipping the attempt) is a
// real-LLM-smoke concern; the overlay LOADS + does-not-break-the-flow is proven
// by the clarification-autonomous mock journey.

// TestAutonomousOverlayFragmentPresent pins that the autonomous coordinator
// overlay fragment exists at the path the clarification-autonomous Taskfile
// target loads (PERSONA_OVERLAY=configs/personas/fragments-autonomous), under the
// coordinator/ role dir. The digit filename prefix is a human-readable
// convention only — the file loader sets no Category/Priority and the prompt
// registry sorts on those alone, so same-category fragments assemble in
// non-deterministic order. The overlay fragment must therefore be self-contained
// (it states the autonomous policy in full, not as a positional override of the
// base contract). This test checks presence + content, not assembly order.
func TestAutonomousOverlayFragmentPresent(t *testing.T) {
	const path = "../../configs/personas/fragments-autonomous/coordinator/12-autonomous-clarification-policy.md"
	raw, err := os.ReadFile(path) //nolint:gosec // test-controlled config path
	if err != nil {
		t.Fatalf("autonomous coordinator overlay fragment missing at %s: %v\n"+
			"The clarification-autonomous journey loads configs/personas/fragments-autonomous as the "+
			"-persona-overlay; without this fragment the overlay is a no-op and the coordinator keeps "+
			"attempting policy-barred ask_user calls.", path, err)
	}
	body := strings.ToLower(string(raw))

	// Semantic markers (not exact wording — persona prose stays editable): the
	// fragment must establish autonomous mode, that ask_user is barred, and that
	// the coordinator resolves via respond_direct.
	for _, want := range []string{"autonomous", "ask_user", "respond_direct"} {
		if !strings.Contains(body, want) {
			t.Errorf("autonomous overlay fragment must mention %q (it teaches the coordinator that ask_user "+
				"is barred and to resolve via respond_direct). Got a fragment that omits it.", want)
		}
	}
	// It must tell the coordinator NOT to use ask_user (the whole point of the
	// overlay). Accept any of the natural phrasings.
	if !strings.Contains(body, "barred") && !strings.Contains(body, "do not attempt") && !strings.Contains(body, "not retry") {
		t.Error("autonomous overlay fragment must explicitly tell the coordinator ask_user is barred / not to attempt it " +
			"(else it doesn't change behavior vs the base persona)")
	}
}

// TestBaseDecisionContractIsClarificationPolicyAware pins that the SHARED base
// coordinator decision contract is mode-aware: it must NOT unconditionally
// prefer ask_user (the old wording that made autonomous deployments burn an
// iteration), and must reference the clarification policy + the respond_direct
// fallback so a deployment that sets restricted_decide_actions but forgets the
// overlay still recovers gracefully.
func TestBaseDecisionContractIsClarificationPolicyAware(t *testing.T) {
	const path = "../../configs/personas/fragments/coordinator/10-decision-contract.md"
	raw, err := os.ReadFile(path) //nolint:gosec // test-controlled config path
	if err != nil {
		t.Fatalf("base coordinator decision contract missing at %s: %v", path, err)
	}
	body := string(raw)
	lower := strings.ToLower(body)

	// Must reference the clarification policy / autonomous mode so the contract
	// is correct in both modes.
	if !strings.Contains(lower, "clarification policy") && !strings.Contains(lower, "autonomous") {
		t.Error("base decision contract must be clarification-policy-aware (reference the policy / autonomous mode) " +
			"so it is correct for both interactive and autonomous deployments — ADR-053 §4b")
	}
	// Must NOT carry the old unconditional 'prefer ask_user over guessing'
	// without the availability caveat — that wording actively pushed autonomous
	// coordinators toward the rejected action.
	if strings.Contains(body, "prefer `ask_user` over guessing.") {
		t.Error("base decision contract still ends an ambiguity line with the UNCONDITIONAL " +
			"'prefer `ask_user` over guessing.' — it must be qualified by ask_user availability (the clarification " +
			"policy), else autonomous deployments burn an iteration on a policy-barred ask_user (ADR-053 §4b)")
	}
}
