package evidence

import (
	"bufio"
	"context"
	"fmt"
	"go/build/constraint"
	"os"
	"strings"
)

// KindTestUsesBuildTag is the registered kind name for the Go
// integration-tag checker. Narrow on purpose: an architect's
// process-local-testcontainer check naming
// `test_runtime: go-testing-net` typically wants the test gated behind
// a build tag (e.g. `//go:build integration`) so a default
// `go test` run doesn't try to spin up containers. Without this
// rule, a builder writing an in-process unit test against a fake
// satisfies the wire shape but defeats the check.
const KindTestUsesBuildTag = "test_uses_build_tag"

// BuildTagChecker validates that a Go test file declares the cited
// build tag in a `//go:build` constraint within its first 100 lines.
// Args:
//
//	{"path": "cmd/foo/foo_test.go", "tag": "integration"}
//
// Both args are required. The 100-line scan is a defense against a
// pathological all-on-one-line file; real Go files put `//go:build`
// at the top.
//
// Constraint evaluation uses go/build/constraint — the canonical
// stdlib parser handles `&&`, `||`, `!`, and parentheses correctly.
// Critically: `//go:build !integration` evaluates to FALSE for
// `tag=integration`, so a file that explicitly EXCLUDES the
// integration build does not falsely pass (the substring/tokenise
// approach this checker started with had that hole).
//
// First `//go:build` line wins; subsequent directives in the same
// file are extremely rare and would only matter for compound tag
// regimes we don't have today. Document if a smoke surfaces the need.
type BuildTagChecker struct{}

const buildTagScanLines = 100

// Check implements Checker.
func (BuildTagChecker) Check(_ context.Context, args map[string]any, ec *Context) Result {
	rel, err := requireStringArg(args, "path")
	if err != nil {
		return Result{Kind: KindTestUsesBuildTag, Status: StatusError, Detail: err.Error()}
	}
	tag, err := requireStringArg(args, "tag")
	if err != nil {
		return Result{Kind: KindTestUsesBuildTag, Status: StatusError, Detail: err.Error()}
	}

	abs, err := ec.Resolve(rel)
	if err != nil {
		return Result{Kind: KindTestUsesBuildTag, Status: StatusError, Detail: err.Error()}
	}
	f, err := os.Open(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return Result{
				Kind:   KindTestUsesBuildTag,
				Status: StatusFail,
				Detail: fmt.Sprintf("path %q does not exist", rel),
			}
		}
		return Result{
			Kind:   KindTestUsesBuildTag,
			Status: StatusError,
			Detail: fmt.Sprintf("open %q: %v", rel, err),
		}
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 64*1024)
	for i := 0; i < buildTagScanLines && scanner.Scan(); i++ {
		line := strings.TrimSpace(scanner.Text())
		if !constraint.IsGoBuild(line) {
			continue
		}
		expr, parseErr := constraint.Parse(line)
		if parseErr != nil {
			return Result{
				Kind:   KindTestUsesBuildTag,
				Status: StatusError,
				Detail: fmt.Sprintf("parse //go:build in %q: %v", rel, parseErr),
			}
		}
		// "Positively referenced" semantic: the tag appears as an
		// un-negated operand somewhere in the constraint AST. Walks
		// the tree carrying a `positive` flag that flips inside Not.
		// Right shape for the architect's intent ("this test is in
		// the integration set"); strict Eval semantics
		// (`expr.Eval(λt. t == tag)`) over-rejects compound
		// constraints like `integration && linux`. See AST traversal
		// helper below.
		if tagPositivelyReferenced(expr, tag) {
			return Result{Kind: KindTestUsesBuildTag, Status: StatusPass}
		}
		return Result{
			Kind:   KindTestUsesBuildTag,
			Status: StatusFail,
			Detail: fmt.Sprintf("file %q has //go:build but does not positively reference tag %q (constraint: %q)", rel, tag, line),
		}
	}
	if err := scanner.Err(); err != nil {
		return Result{
			Kind:   KindTestUsesBuildTag,
			Status: StatusError,
			Detail: fmt.Sprintf("scan %q: %v", rel, err),
		}
	}
	return Result{
		Kind:   KindTestUsesBuildTag,
		Status: StatusFail,
		Detail: fmt.Sprintf("file %q has no //go:build directive in first %d lines", rel, buildTagScanLines),
	}
}

// tagPositivelyReferenced reports whether `tag` appears as an
// un-negated operand somewhere in expr. The recursion carries a
// `positive` polarity flag that flips on Not; a TagExpr matches
// only when the polarity is positive. Examples (target = "integration"):
//
//   - integration                     → true (TagExpr pos)
//   - integration && linux            → true (positive in And)
//   - integration || e2e              → true (positive in Or)
//   - !integration                    → false (pos flipped under Not)
//   - integration && !cgo             → true (integration is positive)
//   - !(integration && linux)         → false (integration under Not)
//   - integration2                    → false (different tag)
//
// Implementation uses go/build/constraint's three concrete Expr
// types: TagExpr, NotExpr, AndExpr, OrExpr.
func tagPositivelyReferenced(e constraint.Expr, tag string) bool {
	return tagAppears(e, tag, true)
}

func tagAppears(e constraint.Expr, tag string, positive bool) bool {
	switch x := e.(type) {
	case *constraint.TagExpr:
		return positive && x.Tag == tag
	case *constraint.NotExpr:
		return tagAppears(x.X, tag, !positive)
	case *constraint.AndExpr:
		return tagAppears(x.X, tag, positive) || tagAppears(x.Y, tag, positive)
	case *constraint.OrExpr:
		return tagAppears(x.X, tag, positive) || tagAppears(x.Y, tag, positive)
	}
	return false
}
