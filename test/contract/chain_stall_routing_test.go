package contract

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/c360studio/semteams/cmd/semteams/chainstall"
)

// TestChainStallRoutingTable_CoversEveryProducerReason pins the routing
// table to its producers. Adding a new structural-gate reject reason in
// any of the producer packages — phasevalidator/phase.go,
// phasevalidator/specmode.go, phasevalidator/qamode.go,
// chain/needs_review.go, recoverycounter/counter.go — without updating
// chainstall's reasonToRoute fails this test.
//
// Producer constants are package-private (rejectReasonInvalidEdge,
// qaModeRejectZeroTests, etc.); the test cannot import them. Instead it
// greps the .go files for AddTriple writes against the relevant
// reject_reason / classification predicates and extracts the literal
// token strings. The token must then appear in chainstall.AllReasons().
//
// Detection scope: any string literal assigned to a const named with
// pattern (rejectReason* | qaModeReject* | specModeReject* |
// needsReview*Default | *Action) in the producer files. Imprecise on
// purpose — false positives are caught by the inverse pin
// (TestChainStallRoutingTable_EveryReasonHasAProducer).
func TestChainStallRoutingTable_CoversEveryProducerReason(t *testing.T) {
	repoRoot := repoRootFromTestDir(t)
	producerFiles := []string{
		"cmd/semteams/phasevalidator/phase.go",
		"cmd/semteams/phasevalidator/specmode.go",
		"cmd/semteams/phasevalidator/qamode.go",
		"cmd/semteams/chain/needs_review.go",
		"cmd/semteams/recoverycounter/counter.go",
	}
	// constLiteralRe captures the value of any `const someName = "literal"`
	// where `someName` is a recognised reject-reason naming convention.
	constLiteralRe := regexp.MustCompile(`(?m)\b(rejectReason\w+|qaModeReject\w+|specModeReject\w+|needsReviewClassificationDefault|insufficientAction|needsClarificationAction|approvedAction|acceptAction)\s*=\s*"([^"]+)"`)

	knownReasons := make(map[string]struct{})
	for _, r := range chainstall.AllReasons() {
		knownReasons[r] = struct{}{}
	}

	var producerReasons []string
	for _, rel := range producerFiles {
		path := filepath.Join(repoRoot, rel)
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		matches := constLiteralRe.FindAllStringSubmatch(string(body), -1)
		for _, m := range matches {
			constName, literal := m[1], m[2]
			// Only the *reject* + *classification* constants name a stall
			// reason chainstall must route. Action constants
			// (approvedAction / acceptAction / insufficientAction /
			// needsClarificationAction) are non-rejection terminals — the
			// regex captures them for coverage but they're filtered out
			// here.
			if isStallReasonConstant(constName) {
				producerReasons = append(producerReasons, literal)
			}
		}
	}
	sort.Strings(producerReasons)

	for _, r := range producerReasons {
		if _, ok := knownReasons[r]; !ok {
			t.Errorf("producer reason %q is not in chainstall.reasonToRoute — add an entry in cmd/semteams/chainstall/routing.go and update the routing-table tests", r)
		}
	}

	// Sanity floor: the producers known today emit at least these tokens.
	// If the grep regex misses one (e.g. a new naming convention), this
	// catches it as a coverage gap.
	const wantAtLeast = 7
	if len(producerReasons) < wantAtLeast {
		t.Errorf("expected to find ≥%d producer reason literals across the producer files; got %d — the grep convention may have drifted: %v", wantAtLeast, len(producerReasons), producerReasons)
	}
}

// TestChainStallRoutingTable_EveryReasonHasAProducer pins the inverse:
// every reason in chainstall.reasonToRoute must correspond to a token
// some producer writes. A routing-table entry without a producer is
// dead code — either the producer was removed (clean the table) or it
// was renamed (rename the entry).
//
// Special case: ReasonRecoveryExhausted is not a reject_reason string
// literal in a producer file — it's the chain.recovery.exhausted
// predicate that recoverycounter writes as object "true". The test
// allows this reason to be satisfied by predicate-string match instead.
func TestChainStallRoutingTable_EveryReasonHasAProducer(t *testing.T) {
	repoRoot := repoRootFromTestDir(t)
	producerFiles := []string{
		"cmd/semteams/phasevalidator/phase.go",
		"cmd/semteams/phasevalidator/specmode.go",
		"cmd/semteams/phasevalidator/qamode.go",
		"cmd/semteams/chain/needs_review.go",
		"cmd/semteams/recoverycounter/counter.go",
		"cmd/semteams/chain/predicates.go",
	}
	bodies := make(map[string]string, len(producerFiles))
	for _, rel := range producerFiles {
		body, err := os.ReadFile(filepath.Join(repoRoot, rel))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		bodies[rel] = string(body)
	}

	for _, reason := range chainstall.AllReasons() {
		found := false
		// ReasonRecoveryExhausted is named after the predicate, not a
		// reject_reason literal; allow predicate-string match.
		searchToken := reason
		for _, body := range bodies {
			if strings.Contains(body, `"`+searchToken+`"`) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("routing-table entry %q has no producer — either the producer was removed (clean the table) or the spelling drifted (rename the entry to match)", reason)
		}
	}
}

// isStallReasonConstant filters the regex-captured constant names to the
// ones that name an actual structural-gate reject reason chainstall
// must route. Action constants (approvedAction, etc.) are non-rejection
// terminals and don't belong in routing.
func isStallReasonConstant(constName string) bool {
	if strings.HasPrefix(constName, "rejectReason") {
		return true
	}
	if strings.HasPrefix(constName, "qaModeReject") {
		return true
	}
	if strings.HasPrefix(constName, "specModeReject") {
		return true
	}
	if constName == "needsReviewClassificationDefault" {
		return true
	}
	return false
}

// repoRootFromTestDir returns the repo root path. Test runner's CWD is
// the package directory (test/contract), so we walk up to find go.mod.
func repoRootFromTestDir(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for dir := wd; dir != "/" && dir != "."; dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
	}
	t.Fatalf("no go.mod found walking up from %s", wd)
	return ""
}
