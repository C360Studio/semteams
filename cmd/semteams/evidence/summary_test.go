package evidence

import (
	"strings"
	"testing"
)

func TestSummarize_EmptyInput(t *testing.T) {
	out := Summarize(nil)
	if !strings.Contains(out, "(no checks)") {
		t.Errorf("empty input should render placeholder; got:\n%s", out)
	}
}

func TestSummarize_SingleAllPassed(t *testing.T) {
	out := Summarize([]CheckSummary{{
		Index:  1,
		Target: "driver emits CS API observation",
		Results: []Result{
			{Kind: KindTestFileExists, Status: StatusPass},
			{Kind: KindSurefirePassingCount, Status: StatusPass},
		},
	}})

	for _, want := range []string{
		"## Check 1 — driver emits CS API observation",
		"2 pass / 0 fail / 0 unknown / 0 error / total 2 — ALL PASSED",
		"- [pass] test_file_exists",
		"- [pass] surefire_passing_count",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestSummarize_MixedStatusesEchoDetail(t *testing.T) {
	out := Summarize([]CheckSummary{{
		Index:  1,
		Target: "driver emits CS API observation",
		Results: []Result{
			{Kind: KindSurefirePassingCount, Status: StatusPass},
			{Kind: KindTestFileExists, Status: StatusFail, Detail: `path "src/test/java/.../FooIT.java" does not exist in workspace`},
			{Kind: "ghost_kind", Status: StatusUnknownKind, Detail: "no Checker registered for kind \"ghost_kind\" (registered: [a b c])"},
			{Kind: "broken_kind", Status: StatusError, Detail: "I/O error reading workspace"},
		},
	}})

	for _, want := range []string{
		"1 pass / 1 fail / 1 unknown / 1 error / total 4 — NOT all passed",
		"- [pass] surefire_passing_count",
		"- [fail] test_file_exists: path \"src/test/java/.../FooIT.java\" does not exist in workspace",
		"- [unknown_kind] ghost_kind: no Checker registered for kind",
		"- [error] broken_kind: I/O error reading workspace",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestSummarize_EmptyRulesOnCheck(t *testing.T) {
	out := Summarize([]CheckSummary{{
		Index:   1,
		Target:  "x",
		Results: nil,
	}})

	for _, want := range []string{
		"## Check 1 — x",
		"total 0 — (no rules on this check)",
		"Per-rule: (none)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestSummarize_MultiCheckSeparated(t *testing.T) {
	out := Summarize([]CheckSummary{
		{
			Index:   1,
			Target:  "real-stack claim",
			Results: []Result{{Kind: KindSurefirePassingCount, Status: StatusPass}},
		},
		{
			Index:   2,
			Target:  "in-process unit claim",
			Results: []Result{{Kind: KindTestFileExists, Status: StatusPass}},
		},
	})

	idx1 := strings.Index(out, "## Check 1 — real-stack claim")
	if idx1 < 0 {
		t.Fatalf("missing first check heading; got:\n%s", out)
	}
	idx2 := strings.Index(out, "## Check 2 — in-process unit claim")
	if idx2 < 0 {
		t.Fatalf("missing second check heading; got:\n%s", out)
	}
	if idx1 >= idx2 {
		t.Errorf("checks rendered out of order (idx1=%d idx2=%d); rendering must preserve input order", idx1, idx2)
	}
	// Two ALL-PASSED headlines, one per check.
	if got := strings.Count(out, "ALL PASSED"); got != 2 {
		t.Errorf("ALL PASSED count = %d, want 2 (one per check)", got)
	}
}

func TestSummarize_PassWithoutDetail_NoTrailingColon(t *testing.T) {
	// Regression class: a Result with empty Detail must NOT render
	// `- [pass] kind: ` with a dangling colon. The reviewer's
	// example fragment relies on the no-colon shape for "pass-and-
	// nothing-to-report" cases.
	out := Summarize([]CheckSummary{{
		Index:   1,
		Target:  "x",
		Results: []Result{{Kind: "k", Status: StatusPass}},
	}})
	if strings.Contains(out, "- [pass] k: ") {
		t.Errorf("pass without detail must not have trailing colon; got:\n%s", out)
	}
	if !strings.Contains(out, "- [pass] k\n") {
		t.Errorf("pass without detail should render `- [pass] k`; got:\n%s", out)
	}
}
