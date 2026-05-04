package evidence

import (
	"context"
	"strings"
	"testing"

	"github.com/c360studio/semteams/cmd/semteams/verification"
)

// stubChecker returns a fixed Result. Lets registry/runner tests
// assert dispatch behaviour without any real filesystem or XML.
type stubChecker struct {
	out Result
}

func (s stubChecker) Check(_ context.Context, _ map[string]any, _ *Context) Result {
	return s.out
}

func TestRegistry_RegisterRejectsBadInput(t *testing.T) {
	cases := []struct {
		name    string
		kind    string
		checker Checker
		wantSub string
	}{
		{"nil checker", "valid_kind", nil, "cannot be nil"},
		{"empty kind", "", stubChecker{out: Result{Status: StatusPass}}, "must match"},
		{"PascalCase kind", "TestFileExists", stubChecker{out: Result{Status: StatusPass}}, "must match"},
		{"hyphen kind", "test-file", stubChecker{out: Result{Status: StatusPass}}, "must match"},
		{"leading digit", "1test_file", stubChecker{out: Result{Status: StatusPass}}, "must match"},
		{"leading underscore", "_test_file", stubChecker{out: Result{Status: StatusPass}}, "must match"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := NewRegistry()
			err := r.Register(tc.kind, tc.checker)
			if err == nil || !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("Register(%q): err = %v, want substring %q", tc.kind, err, tc.wantSub)
			}
		})
	}
}

func TestRegistry_RegisterDuplicateKindFails(t *testing.T) {
	r := NewRegistry()
	if err := r.Register("test_kind", stubChecker{out: Result{Status: StatusPass}}); err != nil {
		t.Fatalf("first register: %v", err)
	}
	err := r.Register("test_kind", stubChecker{out: Result{Status: StatusFail}})
	if err == nil || !strings.Contains(err.Error(), "already registered") {
		t.Fatalf("duplicate register: err = %v, want 'already registered'", err)
	}
}

func TestRegistry_GetReturnsRegistered(t *testing.T) {
	r := NewRegistry()
	c := stubChecker{out: Result{Status: StatusPass}}
	if err := r.Register("kind_a", c); err != nil {
		t.Fatal(err)
	}
	if got := r.Get("kind_a"); got == nil {
		t.Errorf("Get(kind_a) = nil, want registered checker")
	}
	if got := r.Get("kind_b"); got != nil {
		t.Errorf("Get(kind_b) = %v, want nil (not registered)", got)
	}
}

func TestRegistry_KindsSorted(t *testing.T) {
	r := NewRegistry()
	for _, k := range []string{"zulu", "alpha", "mike"} {
		if err := r.Register(k, stubChecker{out: Result{Status: StatusPass}}); err != nil {
			t.Fatal(err)
		}
	}
	got := r.Kinds()
	want := []string{"alpha", "mike", "zulu"}
	if len(got) != len(want) {
		t.Fatalf("Kinds len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Kinds[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestRun_DispatchesAndAggregates(t *testing.T) {
	r := NewRegistry()
	must(t, r.Register("a_pass", stubChecker{out: Result{Status: StatusPass}}))
	must(t, r.Register("a_fail", stubChecker{out: Result{Status: StatusFail, Detail: "claim false"}}))
	must(t, r.Register("a_err", stubChecker{out: Result{Status: StatusError, Detail: "io"}}))

	rules := []verification.EvidenceRule{
		{Kind: "a_pass"},
		{Kind: "a_fail"},
		{Kind: "a_err"},
		{Kind: "a_unknown"}, // not registered
	}
	results, agg := r.Run(context.Background(), rules, &Context{})
	if len(results) != 4 {
		t.Fatalf("results len = %d, want 4", len(results))
	}
	wantStatus := []Status{StatusPass, StatusFail, StatusError, StatusUnknownKind}
	for i := range wantStatus {
		if results[i].Status != wantStatus[i] {
			t.Errorf("results[%d].Status = %q, want %q", i, results[i].Status, wantStatus[i])
		}
	}
	if agg.Total != 4 || agg.Pass != 1 || agg.Fail != 1 || agg.Error != 1 || agg.UnknownKind != 1 {
		t.Errorf("aggregate = %+v, want Total=4 Pass=1 Fail=1 Error=1 Unknown=1", agg)
	}
	if agg.AllPassed() {
		t.Errorf("AllPassed() = true, want false (mixed results)")
	}

	// Unknown-kind detail should name the registered kinds so the
	// reviewer can grade ("you cited a kind that doesn't exist; here
	// are the ones that do").
	if !strings.Contains(results[3].Detail, "registered") {
		t.Errorf("unknown-kind Detail should list registered kinds, got %q", results[3].Detail)
	}
}

func TestRun_AllPassed_True(t *testing.T) {
	r := NewRegistry()
	must(t, r.Register("only_pass", stubChecker{out: Result{Status: StatusPass}}))
	rules := []verification.EvidenceRule{{Kind: "only_pass"}, {Kind: "only_pass"}}
	_, agg := r.Run(context.Background(), rules, &Context{})
	if !agg.AllPassed() {
		t.Errorf("AllPassed() = false, want true (%+v)", agg)
	}
}

func TestRun_EmptyRules_AllPassedFalse(t *testing.T) {
	r := NewRegistry()
	_, agg := r.Run(context.Background(), nil, &Context{})
	if agg.AllPassed() {
		t.Errorf("AllPassed() on empty rules should be false (Total=0); got %+v", agg)
	}
}

func TestRun_CheckerReturnsEmptyStatus_PromotedToError(t *testing.T) {
	r := NewRegistry()
	must(t, r.Register("oops", stubChecker{out: Result{}})) // empty Status
	results, agg := r.Run(context.Background(), []verification.EvidenceRule{{Kind: "oops"}}, &Context{})
	if results[0].Status != StatusError {
		t.Errorf("empty-Status return: got %q, want %q", results[0].Status, StatusError)
	}
	if !strings.Contains(results[0].Detail, "programming error") {
		t.Errorf("empty-Status Detail = %q, want contains 'programming error'", results[0].Detail)
	}
	if agg.Error != 1 {
		t.Errorf("aggregate.Error = %d, want 1", agg.Error)
	}
}

func TestRun_CheckerForgetsKind_BackfilledFromRule(t *testing.T) {
	r := NewRegistry()
	// stubChecker returns Result with no Kind set — runOne should
	// echo the rule's Kind so the reviewer sees per-result identity.
	must(t, r.Register("forgotten", stubChecker{out: Result{Status: StatusPass}}))
	results, _ := r.Run(context.Background(), []verification.EvidenceRule{{Kind: "forgotten"}}, &Context{})
	if results[0].Kind != "forgotten" {
		t.Errorf("Result.Kind backfill: got %q, want %q", results[0].Kind, "forgotten")
	}
}

func TestNewWithBuiltins_RegistersExpectedKinds(t *testing.T) {
	r, err := NewWithBuiltins()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{KindSurefirePassingCount, KindTestFileExists, KindTestUsesBuildTag}
	got := r.Kinds()
	if len(got) != len(want) {
		t.Fatalf("Kinds() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Kinds[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
