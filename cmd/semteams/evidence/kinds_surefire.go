package evidence

import (
	"context"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// KindSurefirePassingCount is the registered kind name for the
// Maven surefire pass-count checker. Smoke #7 (OSH-Meshtastic) uses
// Maven + Testcontainers; this is the load-bearing post-build
// evidence checker for that path.
const KindSurefirePassingCount = "surefire_passing_count"

// SurefirePassingCountChecker validates that
// target/surefire-reports/ (or ec.SurefireReportDir if overridden)
// contains TEST-*.xml files reporting at least `min` passing tests,
// optionally filtered by a class-name suffix. Args:
//
//	{"min": 1}
//	{"min": 3, "class_suffix": "MeshtasticdIT"}
//
// `min` is required and must be a positive integer (JSON numbers
// arrive as float64 from map[string]any). `class_suffix` is
// optional; when set, only XMLs whose <testsuite name="..."> ends
// with the suffix are counted.
//
// "Passing" here = (tests - failures - errors - skipped). Surefire
// XML reports the totals on the testsuite element; we sum across
// the matched files. Tests skipped (e.g. assumptions failing) do
// NOT count as passing — the gate is grading run-and-passed
// behaviour, not run-and-skipped.
type SurefirePassingCountChecker struct{}

// surefireReport is the minimal subset of surefire's TEST-*.xml
// schema this checker reads. Surefire publishes a richer schema
// (per-test cases, system-out, system-err); we only need the
// suite-level totals + the suite name.
type surefireReport struct {
	XMLName  xml.Name `xml:"testsuite"`
	Name     string   `xml:"name,attr"`
	Tests    int      `xml:"tests,attr"`
	Failures int      `xml:"failures,attr"`
	Errors   int      `xml:"errors,attr"`
	Skipped  int      `xml:"skipped,attr"`
}

// Check implements Checker.
//
// Skipped tests do NOT count as passing — the gate grades run-and-
// passed behaviour, not run-and-skipped. If a suite intentionally
// skips on platform predicates (`Assume.assumeTrue(...)` in JUnit)
// and you need to exclude those suites from the gate, set
// `class_suffix` to the class that runs unconditionally.
func (SurefirePassingCountChecker) Check(_ context.Context, args map[string]any, ec *Context) Result {
	minPassing, err := requirePositiveIntArg(args, "min")
	if err != nil {
		return Result{Kind: KindSurefirePassingCount, Status: StatusError, Detail: err.Error()}
	}
	suffix, err := optionalStringArg(args, "class_suffix")
	if err != nil {
		return Result{Kind: KindSurefirePassingCount, Status: StatusError, Detail: err.Error()}
	}

	dir := ec.SurefireReportDir
	if dir == "" {
		return Result{
			Kind:   KindSurefirePassingCount,
			Status: StatusError,
			Detail: "Context.SurefireReportDir is empty (NewContext sets a default)",
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return Result{
				Kind:   KindSurefirePassingCount,
				Status: StatusFail,
				Detail: fmt.Sprintf("surefire report dir %q does not exist (mvn test never ran?)", dir),
			}
		}
		return Result{
			Kind:   KindSurefirePassingCount,
			Status: StatusError,
			Detail: fmt.Sprintf("read surefire dir %q: %v", dir, err),
		}
	}

	var passing, scanned int
	var matchedSuites []string
	for _, ent := range entries {
		name := ent.Name()
		if ent.IsDir() {
			continue
		}
		// Surefire writes both TEST-*.xml (suite reports) and
		// *.txt (per-test text dumps). We only consume XML.
		if !strings.HasPrefix(name, "TEST-") || !strings.HasSuffix(name, ".xml") {
			continue
		}
		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			return Result{
				Kind:   KindSurefirePassingCount,
				Status: StatusError,
				Detail: fmt.Sprintf("read %s: %v", path, err),
			}
		}
		var rep surefireReport
		if err := xml.Unmarshal(data, &rep); err != nil {
			return Result{
				Kind:   KindSurefirePassingCount,
				Status: StatusError,
				Detail: fmt.Sprintf("parse %s: %v", path, err),
			}
		}
		scanned++
		if suffix != "" && !strings.HasSuffix(rep.Name, suffix) {
			continue
		}
		matchedSuites = append(matchedSuites, rep.Name)
		// Defensive: surefire's invariant is tests ≥ failures + errors + skipped,
		// but a corrupt report could violate it. Clamp at 0 so a
		// malformed suite doesn't subtract from the running total.
		p := rep.Tests - rep.Failures - rep.Errors - rep.Skipped
		if p < 0 {
			p = 0
		}
		passing += p
	}

	if scanned == 0 {
		return Result{
			Kind:   KindSurefirePassingCount,
			Status: StatusFail,
			Detail: fmt.Sprintf("no TEST-*.xml files in %q", dir),
		}
	}
	if suffix != "" && len(matchedSuites) == 0 {
		return Result{
			Kind:   KindSurefirePassingCount,
			Status: StatusFail,
			Detail: fmt.Sprintf("no surefire suite name ends with %q (scanned %d suites)", suffix, scanned),
		}
	}
	if passing < minPassing {
		filterDesc := "across all suites"
		if suffix != "" {
			filterDesc = fmt.Sprintf("across suites matching %q (%d suites: %v)", suffix, len(matchedSuites), matchedSuites)
		}
		return Result{
			Kind:   KindSurefirePassingCount,
			Status: StatusFail,
			Detail: fmt.Sprintf("expected ≥ %d passing tests %s, got %d", minPassing, filterDesc, passing),
		}
	}
	return Result{Kind: KindSurefirePassingCount, Status: StatusPass}
}

// requirePositiveIntArg fetches a positive (≥1) integer arg. JSON
// numbers arrive as float64 from map[string]any — this helper
// accepts either float64 (with no fractional part) or int. The
// positivity floor is folded in here so every kind that reaches
// for it inherits the same "negative or zero is malformed args"
// rule rather than re-checking after type assertion.
func requirePositiveIntArg(args map[string]any, key string) (int, error) {
	v, ok := args[key]
	if !ok {
		return 0, fmt.Errorf("missing required arg %q", key)
	}
	var n int
	switch raw := v.(type) {
	case int:
		n = raw
	case int64:
		n = int(raw)
	case float64:
		if raw != float64(int(raw)) {
			return 0, fmt.Errorf("arg %q must be an integer (got %v)", key, raw)
		}
		n = int(raw)
	default:
		return 0, fmt.Errorf("arg %q must be a number (got %T)", key, v)
	}
	if n < 1 {
		return 0, fmt.Errorf("arg %q must be ≥ 1 (got %d)", key, n)
	}
	return n, nil
}

// optionalStringArg fetches a string-typed arg, returning ("", false)
// if absent and an error if present-but-wrong-type. Empty string
// values are treated the same as absent (operators sometimes pass
// "class_suffix": "" by accident; that's the same intent as
// omitting it).
func optionalStringArg(args map[string]any, key string) (string, error) {
	v, ok := args[key]
	if !ok {
		return "", nil
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("arg %q must be a string (got %T)", key, v)
	}
	return s, nil
}
