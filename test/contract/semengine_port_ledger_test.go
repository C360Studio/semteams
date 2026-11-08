package contract

import (
	"encoding/json"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

type ledgerSummaryJSON struct {
	Module                     string `json:"framework_module"`
	CatalogTotalPaths          int    `json:"catalog_total_paths"`
	ActiveRepoImportPaths      int    `json:"active_repo_import_paths"`
	PrunedCatalogPaths         int    `json:"pruned_catalog_paths"`
	DistinctProdFilesImporting int    `json:"distinct_non_test_files_importing"`
	DistinctTestFilesImporting int    `json:"distinct_test_files_importing"`
	DistinctScriptsImporting   int    `json:"distinct_scripts_files_importing"`
	ScriptsLiteralOnlyPaths    []struct {
		Path  string   `json:"path"`
		Files []string `json:"files"`
	} `json:"scripts_literal_only_paths"`
	TotalStringMatchedPaths int `json:"total_string_matched_paths"`
	GroupSummaries          []struct {
		Group          string `json:"group"`
		CatalogPaths   int    `json:"catalog_paths"`
		ActivePaths    int    `json:"active_paths"`
		PrunedPaths    int    `json:"pruned_paths"`
		ProdImports    int    `json:"non_test_import_edges"`
		TestImports    int    `json:"test_import_edges"`
		ScriptsImports int    `json:"scripts_import_edges"`
	} `json:"group_summaries"`
	Rows []struct {
		Subpath            string `json:"subpath"`
		ProdFiles          int    `json:"non_test_files"`
		TestFiles          int    `json:"test_files"`
		RepoFiles          int    `json:"repo_files"`
		ScriptsImportFiles int    `json:"scripts_import_files"`
	} `json:"rows"`
}

func buildLedgerAnalyzer(t *testing.T, repoRoot string, src []byte) string {
	t.Helper()
	tmpDir := t.TempDir()
	srcFile := filepath.Join(tmpDir, "measure-semengine-port-ledger.go")
	if err := os.WriteFile(srcFile, src, 0o644); err != nil {
		t.Fatalf("write temp analyzer source: %v", err)
	}
	binFile := filepath.Join(tmpDir, "measure-semengine-port-ledger")
	cmd := exec.Command("go", "build", "-o", binFile, srcFile)
	cmd.Dir = repoRoot
	if out, buildErr := cmd.CombinedOutput(); buildErr != nil {
		t.Fatalf("compile analyzer: %v\n%s", buildErr, string(out))
	}
	return binFile
}

func runLedgerAnalyzer(bin string, args ...string) (string, int) {
	cmd := exec.Command(bin, args...)
	out, runErr := cmd.CombinedOutput()
	code := 0
	if runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			code = exitErr.ExitCode()
		} else {
			code = -1
		}
	}
	return string(out), code
}

func writeLedgerFixtureFiles(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		full := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", full, err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", full, err)
		}
	}
}

func checkLedgerDocMatchesGenerated(docContent, expectedMarkdown string) error {
	const (
		beginMarker = "<!-- BEGIN GENERATED CROSSWALK -->"
		endMarker   = "<!-- END GENERATED CROSSWALK -->"
	)
	startIdx := strings.Index(docContent, beginMarker)
	if startIdx < 0 {
		return os.ErrNotExist
	}
	afterBegin := docContent[startIdx+len(beginMarker):]
	endIdx := strings.Index(afterBegin, endMarker)
	if endIdx < 0 {
		return os.ErrInvalid
	}
	gotBlock := strings.TrimSpace(afterBegin[:endIdx])
	wantBlock := strings.TrimSpace(expectedMarkdown)
	if gotBlock != wantBlock {
		return &crosswalkDriftError{got: gotBlock, want: wantBlock}
	}
	return nil
}

// spec: openspec/specs/framework-adoption/spec.md#Requirement: Frozen framework adoption preserves qualified product behavior
func TestSemEnginePortLedger(t *testing.T) {
	repoRoot, err := findRepoRoot()
	if err != nil {
		t.Fatalf("findRepoRoot: %v", err)
	}

	scriptPath := filepath.Join(repoRoot, "scripts", "measure-semengine-port-ledger.go")
	scriptBytes, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("read %s: %v", scriptPath, err)
	}

	analyzerBin := buildLedgerAnalyzer(t, repoRoot, scriptBytes)

	t.Run("stdlib_only_and_build_ignore_header", func(t *testing.T) {
		fset := token.NewFileSet()
		fileAST, parseErr := parser.ParseFile(fset, scriptPath, scriptBytes, parser.ParseComments)
		if parseErr != nil {
			t.Fatalf("parse script: %v", parseErr)
		}
		hasBuildIgnore := false
		for _, cg := range fileAST.Comments {
			for _, c := range cg.List {
				if strings.TrimSpace(c.Text) == "//go:build ignore" {
					hasBuildIgnore = true
				}
			}
		}
		if !hasBuildIgnore {
			t.Fatal("scripts/measure-semengine-port-ledger.go must declare //go:build ignore")
		}
		for _, imp := range fileAST.Imports {
			impPath, unquoteErr := strconv.Unquote(imp.Path.Value)
			if unquoteErr != nil {
				t.Fatalf("unquote import %s: %v", imp.Path.Value, unquoteErr)
			}
			if strings.Contains(impPath, ".") {
				t.Fatalf("non-stdlib import %q in scripts/measure-semengine-port-ledger.go; measurement script must stay stdlib-only", impPath)
			}
		}
	})

	t.Run("synthetic_tree_separates_prod_test_scripts_and_skips_ignored_dirs", func(t *testing.T) {
		fixture := t.TempDir()
		writeLedgerFixtureFiles(t, fixture, map[string]string{
			"cmd/app/main.go": `package main
import (
	_ "github.com/c360studio/semstreams/agentic"
	_ "github.com/c360studio/semstreams/component"
)
func main() {}
`,
			"cmd/app/main_test.go": `package main
import (
	_ "github.com/c360studio/semstreams/component"
	_ "github.com/c360studio/semstreams/natsclient"
)
`,
			"cmd/app/comment_only.go": `package main
// Comment mention "github.com/c360studio/semstreams/unclassified/comment-only" must not count.
const nonScriptsLiteral = "github.com/c360studio/semstreams/unclassified/string-in-prod"
`,
			"scripts/probe.go": `//go:build ignore

package main
import _ "github.com/c360studio/semstreams/message"
const rootMod = "github.com/c360studio/semstreams"
const extraMod = "github.com/c360studio/semstreams/custom-probe"
`,
			".worktrees/linked/ignored.go":  "package p\nimport _ \"github.com/c360studio/semstreams/unclassified/worktree\"\n",
			"vendor/mod/ignored.go":         "package p\nimport _ \"github.com/c360studio/semstreams/unclassified/vendor\"\n",
			"node_modules/pkg/ignored.go":   "package p\nimport _ \"github.com/c360studio/semstreams/unclassified/node\"\n",
			".svelte-kit/output/ignored.go": "package p\nimport _ \"github.com/c360studio/semstreams/unclassified/svelte\"\n",
		})

		rawJSON, code := runLedgerAnalyzer(analyzerBin, "-root", fixture, "-format=json")
		if code != 0 {
			t.Fatalf("expected exit 0 on synthetic fixture, got %d:\n%s", code, rawJSON)
		}
		var summary ledgerSummaryJSON
		if err := json.Unmarshal([]byte(rawJSON), &summary); err != nil {
			t.Fatalf("unmarshal json: %v\n%s", err, rawJSON)
		}
		if summary.CatalogTotalPaths != 56 || summary.ActiveRepoImportPaths != 3 || summary.PrunedCatalogPaths != 53 {
			t.Fatalf("unexpected path counts: total=%d active=%d pruned=%d",
				summary.CatalogTotalPaths, summary.ActiveRepoImportPaths, summary.PrunedCatalogPaths)
		}
		if summary.DistinctProdFilesImporting != 1 || summary.DistinctTestFilesImporting != 1 || summary.DistinctScriptsImporting != 1 {
			t.Fatalf("unexpected distinct file counts: prod=%d test=%d scripts=%d",
				summary.DistinctProdFilesImporting, summary.DistinctTestFilesImporting, summary.DistinctScriptsImporting)
		}
		if len(summary.ScriptsLiteralOnlyPaths) != 2 {
			t.Fatalf("expected 2 scripts literal-only paths, got %+v", summary.ScriptsLiteralOnlyPaths)
		}
		if len(summary.Rows) != 56 {
			t.Fatalf("default -active-only=false should emit all 56 baseline rows, got %d", len(summary.Rows))
		}

		activeJSON, activeCode := runLedgerAnalyzer(analyzerBin, "-root", fixture, "-format=json", "-active-only")
		if activeCode != 0 {
			t.Fatalf("expected exit 0 with -active-only, got %d:\n%s", activeCode, activeJSON)
		}
		var activeSummary ledgerSummaryJSON
		if err := json.Unmarshal([]byte(activeJSON), &activeSummary); err != nil {
			t.Fatalf("unmarshal active json: %v", err)
		}
		if len(activeSummary.Rows) != 4 {
			t.Fatalf("expected 4 active rows (agentic, component, natsclient, message), got %d", len(activeSummary.Rows))
		}
		rowBySubpath := map[string]struct{ prod, test, scripts int }{}
		for _, r := range activeSummary.Rows {
			rowBySubpath[r.Subpath] = struct{ prod, test, scripts int }{r.ProdFiles, r.TestFiles, r.ScriptsImportFiles}
		}
		if got := rowBySubpath["component"]; got.prod != 1 || got.test != 1 || got.scripts != 0 {
			t.Fatalf("component counts = %+v, want {prod:1 test:1 scripts:0}", got)
		}
		if got := rowBySubpath["natsclient"]; got.prod != 0 || got.test != 1 || got.scripts != 0 {
			t.Fatalf("natsclient counts = %+v, want {prod:0 test:1 scripts:0}", got)
		}
		if got := rowBySubpath["message"]; got.prod != 0 || got.test != 0 || got.scripts != 1 {
			t.Fatalf("message counts = %+v, want {prod:0 test:0 scripts:1}", got)
		}
	})

	t.Run("fails_closed_on_unclassified_semstreams_import", func(t *testing.T) {
		fixture := t.TempDir()
		writeLedgerFixtureFiles(t, fixture, map[string]string{
			"cmd/app/unclassified.go": `package main
import _ "github.com/c360studio/semstreams/unclassified/newpkg"
`,
		})
		out, code := runLedgerAnalyzer(analyzerBin, "-root", fixture, "-format=json")
		if code == 0 || !strings.Contains(out, "unclassified SemStreams import(s)") || !strings.Contains(out, "cmd/app/unclassified.go") {
			t.Fatalf("expected non-zero fail-closed exit naming unclassified import, got exit %d:\n%s", code, out)
		}
	})

	t.Run("live_repo_all_imports_classified_and_ledger_doc_complete", func(t *testing.T) {
		rawJSON, code := runLedgerAnalyzer(analyzerBin, "-root", repoRoot, "-format=json")
		if code != 0 {
			t.Fatalf("live repository has unclassified SemStreams imports (exit %d):\n%s", code, rawJSON)
		}
		var summary ledgerSummaryJSON
		if err := json.Unmarshal([]byte(rawJSON), &summary); err != nil {
			t.Fatalf("unmarshal live summary: %v", err)
		}
		if summary.CatalogTotalPaths != 56 {
			t.Fatalf("catalog_total_paths = %d, want 56", summary.CatalogTotalPaths)
		}
		if summary.ActiveRepoImportPaths+summary.PrunedCatalogPaths != summary.CatalogTotalPaths {
			t.Fatalf("active (%d) + pruned (%d) != total (%d)",
				summary.ActiveRepoImportPaths, summary.PrunedCatalogPaths, summary.CatalogTotalPaths)
		}
		wantGroupTotals := []int{20, 2, 11, 23}
		if len(summary.GroupSummaries) != len(wantGroupTotals) {
			t.Fatalf("group count = %d, want %d", len(summary.GroupSummaries), len(wantGroupTotals))
		}
		for i, want := range wantGroupTotals {
			if got := summary.GroupSummaries[i].CatalogPaths; got != want {
				t.Fatalf("group %q catalog_paths = %d, want %d", summary.GroupSummaries[i].Group, got, want)
			}
		}

		expectedMarkdown, mdCode := runLedgerAnalyzer(analyzerBin, "-root", repoRoot)
		if mdCode != 0 {
			t.Fatalf("analyzer markdown mode failed on live repo (exit %d):\n%s", mdCode, expectedMarkdown)
		}

		ledgerDocPath := filepath.Join(repoRoot, "docs", "migrations", "semengine-port-ledger.md")
		docBytes, docErr := os.ReadFile(ledgerDocPath)
		if docErr != nil {
			t.Fatalf("read %s: %v", ledgerDocPath, docErr)
		}
		docText := string(docBytes)
		for _, required := range []string{
			"go run ./scripts/measure-semengine-port-ledger.go",
			"ADR-108",
			"ADR-109",
			"ADR-110",
			"ADR-112",
			"ADR-113",
			"<!-- BEGIN GENERATED CROSSWALK -->",
			"<!-- END GENERATED CROSSWALK -->",
		} {
			if !strings.Contains(docText, required) {
				t.Fatalf("docs/migrations/semengine-port-ledger.md missing required section/token %q", required)
			}
		}
		if driftErr := checkLedgerDocMatchesGenerated(docText, expectedMarkdown); driftErr != nil {
			t.Fatalf("docs/migrations/semengine-port-ledger.md generated crosswalk block is stale; regenerate with `go run ./scripts/measure-semengine-port-ledger.go`:\n%v", driftErr)
		}
	})

	t.Run("sensitivity_stale_generated_crosswalk_block_is_detected", func(t *testing.T) {
		expectedMarkdown, mdCode := runLedgerAnalyzer(analyzerBin, "-root", repoRoot)
		if mdCode != 0 {
			t.Fatalf("analyzer markdown mode failed: %s", expectedMarkdown)
		}
		ledgerDocPath := filepath.Join(repoRoot, "docs", "migrations", "semengine-port-ledger.md")
		docBytes, docErr := os.ReadFile(ledgerDocPath)
		if docErr != nil {
			t.Fatalf("read %s: %v", ledgerDocPath, docErr)
		}
		mutatedDoc := strings.Replace(string(docBytes), "| `message` | 22 |", "| `message` | 99 |", 1)
		if mutatedDoc == string(docBytes) {
			t.Fatal("stale-count mutation did not apply to docs/migrations/semengine-port-ledger.md")
		}
		mutatedPath := filepath.Join(t.TempDir(), "semengine-port-ledger.md")
		if err := os.WriteFile(mutatedPath, []byte(mutatedDoc), 0o644); err != nil {
			t.Fatalf("write mutated doc copy: %v", err)
		}
		reRead, err := os.ReadFile(mutatedPath)
		if err != nil {
			t.Fatalf("read mutated doc copy: %v", err)
		}
		if driftErr := checkLedgerDocMatchesGenerated(string(reRead), expectedMarkdown); driftErr == nil {
			t.Fatal("expected checkLedgerDocMatchesGenerated to reject stale generated crosswalk block, proving drift sensitivity")
		}
	})

	t.Run("sensitivity_disabling_unclassified_check_is_detected", func(t *testing.T) {
		mutated := strings.Replace(
			string(scriptBytes),
			"if len(unclassified) > 0 {",
			"if false && len(unclassified) > 0 {",
			1,
		)
		if mutated == string(scriptBytes) {
			t.Fatal("mutation did not apply to unclassified guard")
		}
		mutatedBin := buildLedgerAnalyzer(t, repoRoot, []byte(mutated))
		fixture := t.TempDir()
		writeLedgerFixtureFiles(t, fixture, map[string]string{
			"cmd/app/unclassified.go": "package main\nimport _ \"github.com/c360studio/semstreams/unclassified/newpkg\"\n",
		})
		_, code := runLedgerAnalyzer(mutatedBin, "-root", fixture, "-format=json")
		if code != 0 {
			t.Fatal("expected mutated guard to exit 0, proving fails_closed_on_unclassified_semstreams_import sensitivity")
		}
	})

	t.Run("sensitivity_collapsing_test_into_prod_counts_is_detected", func(t *testing.T) {
		mutated := strings.Replace(
			string(scriptBytes),
			`isTest := strings.HasSuffix(rel, "_test.go")`,
			`isTest := false`,
			1,
		)
		if mutated == string(scriptBytes) {
			t.Fatal("mutation did not apply to _test.go classification")
		}
		mutatedBin := buildLedgerAnalyzer(t, repoRoot, []byte(mutated))
		fixture := t.TempDir()
		writeLedgerFixtureFiles(t, fixture, map[string]string{
			"cmd/app/main_test.go": "package main\nimport _ \"github.com/c360studio/semstreams/natsclient\"\n",
		})
		rawJSON, code := runLedgerAnalyzer(mutatedBin, "-root", fixture, "-format=json", "-active-only")
		if code != 0 {
			t.Fatalf("mutated binary failed unexpectedly: %s", rawJSON)
		}
		var summary ledgerSummaryJSON
		if err := json.Unmarshal([]byte(rawJSON), &summary); err != nil {
			t.Fatalf("unmarshal mutated json: %v", err)
		}
		if summary.DistinctTestFilesImporting != 0 || summary.DistinctProdFilesImporting != 1 {
			t.Fatalf("expected mutated _test.go classifier to miscount test file as prod, got prod=%d test=%d",
				summary.DistinctProdFilesImporting, summary.DistinctTestFilesImporting)
		}
	})
}

type crosswalkDriftError struct {
	got  string
	want string
}

func (e *crosswalkDriftError) Error() string {
	gotLines := strings.Split(e.got, "\n")
	wantLines := strings.Split(e.want, "\n")
	limit := len(gotLines)
	if len(wantLines) > limit {
		limit = len(wantLines)
	}
	for i := 0; i < limit; i++ {
		var g, w string
		if i < len(gotLines) {
			g = gotLines[i]
		}
		if i < len(wantLines) {
			w = wantLines[i]
		}
		if g != w {
			return "first differing line " + strconv.Itoa(i+1) + ":\n  doc: " + g + "\n  gen: " + w
		}
	}
	return "generated crosswalk block differs"
}
