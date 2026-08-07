//go:build parked_packs

// The pack this file pins is parked (unwired from flow-bootstrap) pending the
// canonical-predicate migration — see ADR-058. Drop this build tag when the
// pack is re-authored and re-wired.

package contract

import (
	"os"
	"strings"
	"testing"
)

func TestDefinitionOfDoneAuthorityStackContracts(t *testing.T) {
	files := map[string][]string{
		"../../cmd/semteams/tools/projectspecplan/schema.go": {
			"plan.done_authority.*",
			"adapter, not a planner",
		},
		"../../configs/rules/dev-from-task/02-ready-request-to-coordinator.json": {
			"approved change facts on the run entity own what done means",
			"plan.done_authority.*",
			"MUST NOT spawn Lisa",
			"Do not call dev_via_test without subtopics",
		},
		"../../configs/personas/fragments/coordinator/30-plan-walking.md": {
			"Definition of done authority",
			"you sequence work but you do not redefine",
			"CBG owns final done",
		},
		"../../configs/personas/fragments/dev-via-test-execute/10-execute-loop.md": {
			"Definition of done authority",
			"Ralph converges but does not redefine done",
			"Passing the test command is the convergence signal",
			"do not silently rewrite the task",
		},
		"../../configs/personas/fragments/reviewer-dev-via-test/10-review-contract.md": {
			"Definition of done authority",
			"CBG owns final done for implementation",
			"Ralph's per-task test pass is evidence, not final acceptance",
			"coordinator's pre-CBG rollup is context, not a verdict",
		},
	}

	for path, wants := range files {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		text := string(body)
		for _, want := range wants {
			if !containsNormalized(text, want) {
				t.Errorf("%s missing definition-of-done contract text %q", path, want)
			}
		}
	}
}

func containsNormalized(text, want string) bool {
	return strings.Contains(normalizeWhitespace(text), normalizeWhitespace(want))
}

func normalizeWhitespace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
