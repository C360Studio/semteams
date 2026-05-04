package evidence

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTestFile(t *testing.T, root, rel, content string) {
	t.Helper()
	full := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestBuildTagChecker_PassWhenTagPresent(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "foo_test.go", "//go:build integration\n\npackage foo\n")
	ec, _ := NewContext(root)
	res := BuildTagChecker{}.Check(context.Background(),
		map[string]any{"path": "foo_test.go", "tag": "integration"}, ec)
	if res.Status != StatusPass {
		t.Errorf("got %q, want pass; detail=%q", res.Status, res.Detail)
	}
}

func TestBuildTagChecker_PassWithCompoundConstraint(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "foo_test.go", "//go:build integration && linux\n\npackage foo\n")
	ec, _ := NewContext(root)
	res := BuildTagChecker{}.Check(context.Background(),
		map[string]any{"path": "foo_test.go", "tag": "integration"}, ec)
	if res.Status != StatusPass {
		t.Errorf("compound constraint: got %q, want pass; detail=%q", res.Status, res.Detail)
	}
}

func TestBuildTagChecker_FailWhenWrongTag(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "foo_test.go", "//go:build unit\n\npackage foo\n")
	ec, _ := NewContext(root)
	res := BuildTagChecker{}.Check(context.Background(),
		map[string]any{"path": "foo_test.go", "tag": "integration"}, ec)
	if res.Status != StatusFail {
		t.Errorf("got %q, want fail; detail=%q", res.Status, res.Detail)
	}
	if !strings.Contains(res.Detail, "integration") {
		t.Errorf("detail should name expected tag: %q", res.Detail)
	}
}

func TestBuildTagChecker_FailWhenNoBuildLine(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "foo_test.go", "package foo\n")
	ec, _ := NewContext(root)
	res := BuildTagChecker{}.Check(context.Background(),
		map[string]any{"path": "foo_test.go", "tag": "integration"}, ec)
	if res.Status != StatusFail {
		t.Errorf("got %q, want fail; detail=%q", res.Status, res.Detail)
	}
	if !strings.Contains(res.Detail, "no //go:build") {
		t.Errorf("detail should call out missing directive: %q", res.Detail)
	}
}

func TestBuildTagChecker_NegatedTagDoesNotPass(t *testing.T) {
	// `//go:build !integration` excludes the integration build —
	// must NOT pass for tag=integration. Regression class: a
	// substring/tokenise approach that strips `!` would falsely
	// pass. The constraint-parser path handles this correctly.
	root := t.TempDir()
	writeTestFile(t, root, "foo_test.go", "//go:build !integration\n\npackage foo\n")
	ec, _ := NewContext(root)
	res := BuildTagChecker{}.Check(context.Background(),
		map[string]any{"path": "foo_test.go", "tag": "integration"}, ec)
	if res.Status != StatusFail {
		t.Errorf("negated tag must not pass: got %q, want fail; detail=%q", res.Status, res.Detail)
	}
}

func TestBuildTagChecker_NegatedOtherTagStillPasses(t *testing.T) {
	// `//go:build integration && !cgo` — the file IS an integration
	// build (it just excludes cgo). Should pass for tag=integration.
	root := t.TempDir()
	writeTestFile(t, root, "foo_test.go", "//go:build integration && !cgo\n\npackage foo\n")
	ec, _ := NewContext(root)
	res := BuildTagChecker{}.Check(context.Background(),
		map[string]any{"path": "foo_test.go", "tag": "integration"}, ec)
	if res.Status != StatusPass {
		t.Errorf("integration && !cgo with tag=integration: got %q, want pass; detail=%q", res.Status, res.Detail)
	}
}

func TestBuildTagChecker_OrConstraintPasses(t *testing.T) {
	// `//go:build integration || e2e` — passes for tag=integration
	// because integration ∈ {integration, e2e}.
	root := t.TempDir()
	writeTestFile(t, root, "foo_test.go", "//go:build integration || e2e\n\npackage foo\n")
	ec, _ := NewContext(root)
	res := BuildTagChecker{}.Check(context.Background(),
		map[string]any{"path": "foo_test.go", "tag": "integration"}, ec)
	if res.Status != StatusPass {
		t.Errorf("OR constraint: got %q, want pass; detail=%q", res.Status, res.Detail)
	}
}

func TestBuildTagChecker_DoesNotMatchTagAsSubstring(t *testing.T) {
	// "tag" appearing inside a bigger token ("integration2") must NOT
	// pass — guards a real regression class where a checker would
	// strings.Contains its way to a false positive.
	root := t.TempDir()
	writeTestFile(t, root, "foo_test.go", "//go:build integration2\n\npackage foo\n")
	ec, _ := NewContext(root)
	res := BuildTagChecker{}.Check(context.Background(),
		map[string]any{"path": "foo_test.go", "tag": "integration"}, ec)
	if res.Status != StatusFail {
		t.Errorf("substring match should not pass: got %q, want fail", res.Status)
	}
}

func TestBuildTagChecker_ErrorOnMissingArgs(t *testing.T) {
	root := t.TempDir()
	ec, _ := NewContext(root)
	res := BuildTagChecker{}.Check(context.Background(),
		map[string]any{"path": "foo_test.go"}, ec)
	if res.Status != StatusError {
		t.Errorf("missing tag arg: got %q, want error", res.Status)
	}
}
