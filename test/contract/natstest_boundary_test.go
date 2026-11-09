package contract

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const (
	natstestImportPrefix       = "github.com/c360studio/semteams/internal/natstest"
	testcontainersImportPrefix = "github.com/testcontainers/testcontainers-go"
)

var forbiddenNATSClientSymbols = map[string]bool{
	"NewTestClient":       true,
	"NewSharedTestClient": true,
	"TestClient":          true,
}

func matchesImportPrefix(impPath, prefix string) bool {
	return impPath == prefix || strings.HasPrefix(impPath, prefix+"/")
}

func isNATSClientImport(impPath string) bool {
	return impPath == "natsclient" || strings.HasSuffix(impPath, "/natsclient")
}

func scanNATSTestBoundaries(root string) ([]string, error) {
	var violations []string
	fset := token.NewFileSet()

	walkErr := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			relDir, relErr := filepath.Rel(root, path)
			if relErr == nil && filepath.ToSlash(relDir) == "scripts" {
				return filepath.SkipDir
			}
			switch d.Name() {
			case ".git", ".worktrees", "vendor", "node_modules", ".svelte-kit":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".go") {
			return nil
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)

		fileViolations, err := checkFileNATSTestBoundaries(fset, path, rel)
		if err != nil {
			return err
		}
		violations = append(violations, fileViolations...)
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}

	sort.Strings(violations)
	return violations, nil
}

func checkFileNATSTestBoundaries(fset *token.FileSet, absPath, rel string) ([]string, error) {
	src, err := os.ReadFile(absPath)
	if err != nil {
		return nil, err
	}
	fileAST, err := parser.ParseFile(fset, absPath, src, 0)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", rel, err)
	}

	isTest := strings.HasSuffix(rel, "_test.go")
	inNATSTestPkg := rel == "internal/natstest" || strings.HasPrefix(rel, "internal/natstest/")

	var violations []string
	natsClientAliases := make(map[string]bool)
	dotImportedNATSClient := false

	for _, imp := range fileAST.Imports {
		impPath, unquoteErr := strconv.Unquote(imp.Path.Value)
		if unquoteErr != nil {
			return nil, fmt.Errorf("unquote import in %s: %w", rel, unquoteErr)
		}
		line := fset.Position(imp.Pos()).Line

		if !isTest && !inNATSTestPkg {
			if matchesImportPrefix(impPath, natstestImportPrefix) {
				violations = append(violations, fmt.Sprintf("%s:%d: non-test file outside internal/natstest imports %q", rel, line, impPath))
			}
			if matchesImportPrefix(impPath, testcontainersImportPrefix) {
				violations = append(violations, fmt.Sprintf("%s:%d: non-test file outside internal/natstest imports %q", rel, line, impPath))
			}
		}

		if isNATSClientImport(impPath) {
			switch {
			case imp.Name == nil:
				natsClientAliases["natsclient"] = true
			case imp.Name.Name == ".":
				dotImportedNATSClient = true
			case imp.Name.Name != "_":
				natsClientAliases[imp.Name.Name] = true
			}
		}
	}

	if len(natsClientAliases) == 0 && !dotImportedNATSClient {
		return violations, nil
	}

	ast.Inspect(fileAST, func(n ast.Node) bool {
		switch expr := n.(type) {
		case *ast.SelectorExpr:
			pkgIdent, ok := expr.X.(*ast.Ident)
			if ok && natsClientAliases[pkgIdent.Name] && forbiddenNATSClientSymbols[expr.Sel.Name] {
				line := fset.Position(expr.Pos()).Line
				violations = append(violations, fmt.Sprintf("%s:%d: forbidden %s.%s reference (use internal/natstest)", rel, line, pkgIdent.Name, expr.Sel.Name))
			}
		case *ast.Ident:
			if dotImportedNATSClient && forbiddenNATSClientSymbols[expr.Name] {
				line := fset.Position(expr.Pos()).Line
				violations = append(violations, fmt.Sprintf("%s:%d: forbidden dot-imported natsclient.%s reference (use internal/natstest)", rel, line, expr.Name))
			}
		}
		return true
	})

	return violations, nil
}

// spec: openspec/specs/framework-adoption/spec.md#Requirement: Frozen framework adoption preserves qualified product behavior
func TestNATSTestBoundary(t *testing.T) {
	repoRoot, err := findRepoRoot()
	if err != nil {
		t.Fatalf("findRepoRoot: %v", err)
	}

	t.Run("live_repository_enforces_natstest_and_testclient_boundaries", func(t *testing.T) {
		violations, scanErr := scanNATSTestBoundaries(repoRoot)
		if scanErr != nil {
			t.Fatalf("scan live repository: %v", scanErr)
		}
		if len(violations) > 0 {
			t.Fatalf("NATS test-fixture boundary violations (%d):\n  %s", len(violations), strings.Join(violations, "\n  "))
		}
	})

	t.Run("sensitivity_catches_non_test_imports_outside_internal_natstest", func(t *testing.T) {
		fixture := t.TempDir()
		writeLedgerFixtureFiles(t, fixture, map[string]string{
			"cmd/semteams/bad_prod.go": `package main
import (
	_ "github.com/c360studio/semteams/internal/natstest"
	_ "github.com/testcontainers/testcontainers-go"
	_ "github.com/testcontainers/testcontainers-go/wait"
)
`,
		})
		violations, scanErr := scanNATSTestBoundaries(fixture)
		if scanErr != nil {
			t.Fatalf("scan synthetic fixture: %v", scanErr)
		}
		if len(violations) != 3 {
			t.Fatalf("expected 3 non-test import violations, got %d: %v", len(violations), violations)
		}
	})

	t.Run("sensitivity_catches_testclient_references_across_default_aliased_and_dot_imports", func(t *testing.T) {
		fixture := t.TempDir()
		writeLedgerFixtureFiles(t, fixture, map[string]string{
			"cmd/semteams/default_test.go": `package main
import "github.com/c360studio/semstreams/natsclient"
func f() {
	_ = natsclient.NewTestClient
	_ = natsclient.NewSharedTestClient
	var _ *natsclient.TestClient
}
`,
			"cmd/semteams/aliased_test.go": `package main
import nc "github.com/c360studio/semengine/natsclient"
func g() { _ = nc.NewTestClient }
`,
			"cmd/semteams/dot_test.go": `package main
import . "github.com/c360studio/semstreams/natsclient"
func h() { _ = NewTestClient }
`,
		})
		violations, scanErr := scanNATSTestBoundaries(fixture)
		if scanErr != nil {
			t.Fatalf("scan synthetic fixture: %v", scanErr)
		}
		if len(violations) != 5 {
			t.Fatalf("expected 5 forbidden TestClient violations, got %d: %v", len(violations), violations)
		}
	})

	t.Run("sensitivity_permits_internal_natstest_and_clean_test_callers_while_skipping_excluded_dirs", func(t *testing.T) {
		fixture := t.TempDir()
		writeLedgerFixtureFiles(t, fixture, map[string]string{
			"internal/natstest/natstest.go": `package natstest
import (
	_ "github.com/c360studio/semstreams/natsclient"
	_ "github.com/testcontainers/testcontainers-go"
	_ "github.com/testcontainers/testcontainers-go/wait"
)
`,
			"cmd/semteams/clean_integration_test.go": `package main
import (
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semteams/internal/natstest"
)
func use(b *natstest.Broker) *natsclient.Client { return b.Client }
`,
			"scripts/tool.go": `package main
import "github.com/c360studio/semstreams/natsclient"
var _ = natsclient.NewTestClient
`,
			".worktrees/other/bad.go": `package p
import _ "github.com/c360studio/semteams/internal/natstest"
`,
		})
		violations, scanErr := scanNATSTestBoundaries(fixture)
		if scanErr != nil {
			t.Fatalf("scan synthetic fixture: %v", scanErr)
		}
		if len(violations) != 0 {
			t.Fatalf("expected 0 violations on clean fixture, got %v", violations)
		}
	})
}
