package evidence

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// writeSurefireReport drops a TEST-<name>.xml file under
// target/surefire-reports/ with the given counts. Mirrors the
// minimal subset surefireReport reads.
func writeSurefireReport(t *testing.T, dir, suiteName string, tests, failures, errs, skipped int) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := `<?xml version="1.0" encoding="UTF-8"?>
<testsuite name="` + suiteName + `" tests="` + strconv.Itoa(tests) + `" failures="` + strconv.Itoa(failures) + `" errors="` + strconv.Itoa(errs) + `" skipped="` + strconv.Itoa(skipped) + `">
</testsuite>
`
	// `TEST-<suiteName>.xml` is surefire's filename convention.
	fn := filepath.Join(dir, "TEST-"+suiteName+".xml")
	if err := os.WriteFile(fn, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSurefirePassingCountChecker_PassUnfiltered(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "target", "surefire-reports")
	writeSurefireReport(t, dir, "com.example.FooIT", 5, 0, 0, 0)
	writeSurefireReport(t, dir, "com.example.BarTest", 3, 0, 0, 0)
	ec, _ := NewContext(root)
	res := SurefirePassingCountChecker{}.Check(context.Background(),
		map[string]any{"min": float64(7)}, ec)
	if res.Status != StatusPass {
		t.Errorf("got %q, want pass; detail=%q", res.Status, res.Detail)
	}
}

func TestSurefirePassingCountChecker_PassClassFiltered(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "target", "surefire-reports")
	writeSurefireReport(t, dir, "com.example.FooIT", 5, 0, 0, 0)
	writeSurefireReport(t, dir, "com.example.BarTest", 3, 0, 0, 0)
	ec, _ := NewContext(root)
	res := SurefirePassingCountChecker{}.Check(context.Background(),
		map[string]any{"min": float64(3), "class_suffix": "FooIT"}, ec)
	if res.Status != StatusPass {
		t.Errorf("got %q, want pass; detail=%q", res.Status, res.Detail)
	}
}

func TestSurefirePassingCountChecker_FailUnderMin(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "target", "surefire-reports")
	writeSurefireReport(t, dir, "com.example.FooIT", 1, 0, 0, 0)
	ec, _ := NewContext(root)
	res := SurefirePassingCountChecker{}.Check(context.Background(),
		map[string]any{"min": float64(3)}, ec)
	if res.Status != StatusFail {
		t.Errorf("got %q, want fail; detail=%q", res.Status, res.Detail)
	}
	if !strings.Contains(res.Detail, "≥ 3") {
		t.Errorf("detail should name expected min: %q", res.Detail)
	}
}

func TestSurefirePassingCountChecker_FailureCountSubtracts(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "target", "surefire-reports")
	// 5 tests, 2 failures, 0 errors, 0 skipped → 3 passing.
	writeSurefireReport(t, dir, "com.example.FooIT", 5, 2, 0, 0)
	ec, _ := NewContext(root)
	res := SurefirePassingCountChecker{}.Check(context.Background(),
		map[string]any{"min": float64(4)}, ec)
	if res.Status != StatusFail {
		t.Errorf("got %q, want fail (only 3 passing); detail=%q", res.Status, res.Detail)
	}
}

func TestSurefirePassingCountChecker_ErrorsAndSkippedSubtract(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "target", "surefire-reports")
	// 10 tests, 1 failure, 1 error, 2 skipped → 6 passing.
	writeSurefireReport(t, dir, "com.example.FooIT", 10, 1, 1, 2)
	ec, _ := NewContext(root)
	res := SurefirePassingCountChecker{}.Check(context.Background(),
		map[string]any{"min": float64(6)}, ec)
	if res.Status != StatusPass {
		t.Errorf("6 passing meets min=6: got %q, detail=%q", res.Status, res.Detail)
	}
	res = SurefirePassingCountChecker{}.Check(context.Background(),
		map[string]any{"min": float64(7)}, ec)
	if res.Status != StatusFail {
		t.Errorf("6 passing under min=7: got %q, want fail", res.Status)
	}
}

func TestSurefirePassingCountChecker_FailWhenDirAbsent(t *testing.T) {
	root := t.TempDir()
	ec, _ := NewContext(root)
	res := SurefirePassingCountChecker{}.Check(context.Background(),
		map[string]any{"min": float64(1)}, ec)
	if res.Status != StatusFail {
		t.Errorf("missing dir: got %q, want fail", res.Status)
	}
	if !strings.Contains(res.Detail, "does not exist") {
		t.Errorf("detail should call out missing dir: %q", res.Detail)
	}
}

func TestSurefirePassingCountChecker_FailWhenSuffixMatchesNothing(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "target", "surefire-reports")
	writeSurefireReport(t, dir, "com.example.FooIT", 5, 0, 0, 0)
	ec, _ := NewContext(root)
	res := SurefirePassingCountChecker{}.Check(context.Background(),
		map[string]any{"min": float64(1), "class_suffix": "GhostIT"}, ec)
	if res.Status != StatusFail {
		t.Errorf("nonexistent suffix: got %q, want fail", res.Status)
	}
	if !strings.Contains(res.Detail, "GhostIT") {
		t.Errorf("detail should name suffix that didn't match: %q", res.Detail)
	}
}

func TestSurefirePassingCountChecker_ErrorOnMissingMinArg(t *testing.T) {
	root := t.TempDir()
	ec, _ := NewContext(root)
	res := SurefirePassingCountChecker{}.Check(context.Background(),
		map[string]any{}, ec)
	if res.Status != StatusError {
		t.Errorf("got %q, want error", res.Status)
	}
}

func TestSurefirePassingCountChecker_ErrorOnNonIntegerMin(t *testing.T) {
	root := t.TempDir()
	ec, _ := NewContext(root)
	res := SurefirePassingCountChecker{}.Check(context.Background(),
		map[string]any{"min": float64(2.5)}, ec)
	if res.Status != StatusError {
		t.Errorf("fractional min: got %q, want error", res.Status)
	}
}

func TestSurefirePassingCountChecker_ErrorOnZeroMin(t *testing.T) {
	root := t.TempDir()
	ec, _ := NewContext(root)
	res := SurefirePassingCountChecker{}.Check(context.Background(),
		map[string]any{"min": float64(0)}, ec)
	if res.Status != StatusError {
		t.Errorf("zero min: got %q, want error", res.Status)
	}
}

func TestSurefirePassingCountChecker_ErrorOnCorruptXML(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "target", "surefire-reports")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "TEST-Bad.xml"), []byte("not xml"), 0o644); err != nil {
		t.Fatal(err)
	}
	ec, _ := NewContext(root)
	res := SurefirePassingCountChecker{}.Check(context.Background(),
		map[string]any{"min": float64(1)}, ec)
	if res.Status != StatusError {
		t.Errorf("corrupt XML: got %q, want error", res.Status)
	}
}
