package contract

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type baselineCompositionWarning struct {
	Type      string `json:"type"`
	Severity  string `json:"severity"`
	Component string `json:"component"`
	Port      string `json:"port"`
	Message   string `json:"message"`
}

type baselineTestTally struct {
	Pass int `json:"pass"`
	Skip int `json:"skip"`
	Fail int `json:"fail"`
}

type baselineDocJSON struct {
	SchemaVersion               int    `json:"schema_version"`
	RecordedDate                string `json:"recorded_date"`
	Issue                       string `json:"issue"`
	Epic                        string `json:"epic"`
	ConsumerQualificationIssue  string `json:"consumer_qualification_issue"`
	PreexistingE2EFollowupIssue string `json:"preexisting_e2e_followup_issue"`
	SemTeamsCommit              string `json:"semteams_commit"`
	PriorOctoberSnapshotDir     string `json:"prior_october_snapshot_dir"`
	Framework                   struct {
		Module          string `json:"module"`
		Version         string `json:"version"`
		Commit          string `json:"commit"`
		Sum             string `json:"sum"`
		HasLocalReplace bool   `json:"has_local_replace"`
	} `json:"framework"`
	NATS struct {
		GoNatstest struct {
			PinnedReference string `json:"pinned_reference"`
			RepoDigest      string `json:"repo_digest"`
			ServerVersion   string `json:"server_version"`
		} `json:"go_natstest"`
		UIE2ECompose struct {
			ComposeReference string `json:"compose_reference"`
			RepoDigest       string `json:"repo_digest"`
			ServerVersion    string `json:"server_version"`
		} `json:"ui_e2e_compose"`
	} `json:"nats"`
	Gates struct {
		TaskValidate struct {
			Passed                      bool `json:"passed"`
			GofmtFiles                  int  `json:"gofmt_files"`
			SvelteCheckErrors           int  `json:"svelte_check_errors"`
			SvelteCheckWarnings         int  `json:"svelte_check_warnings"`
			PublishVerifyFixturesPassed int  `json:"publish_verify_fixtures_passed"`
			PublishVerifyFixturesFailed int  `json:"publish_verify_fixtures_failed"`
		} `json:"task_validate"`
		GoUnitRace struct {
			PackagesTotal int               `json:"packages_total"`
			TopLevelTests baselineTestTally `json:"top_level_tests"`
			Subtests      baselineTestTally `json:"subtests"`
		} `json:"go_unit_race"`
		GoIntegration struct {
			PackagesTotal int               `json:"packages_total"`
			TopLevelTests baselineTestTally `json:"top_level_tests"`
			Subtests      baselineTestTally `json:"subtests"`
		} `json:"go_integration"`
		UIVitest struct {
			TestFilesTotal   int `json:"test_files_total"`
			TestFilesPassed  int `json:"test_files_passed"`
			TestFilesSkipped int `json:"test_files_skipped"`
			TestsTotal       int `json:"tests_total"`
			TestsPassed      int `json:"tests_passed"`
			TestsSkipped     int `json:"tests_skipped"`
			TestsFailed      int `json:"tests_failed"`
		} `json:"ui_vitest"`
	} `json:"gates"`
	RuntimeBoundarySuites map[string][]string `json:"runtime_boundary_suites"`
	CompositionLedger     struct {
		CatalogFactoryCount            int                          `json:"catalog_factory_count"`
		BootstrapConfigs               []string                     `json:"bootstrap_configs"`
		Status                         string                       `json:"status"`
		ErrorCount                     int                          `json:"error_count"`
		WarningCount                   int                          `json:"warning_count"`
		LiveBootMatchesOfflineValidate bool                         `json:"live_boot_matches_offline_validate"`
		Warnings                       []baselineCompositionWarning `json:"warnings"`
	} `json:"composition_ledger"`
	ColdStartLoopsReadiness struct {
		Endpoint               string `json:"endpoint"`
		First503ElapsedMS      int    `json:"first_503_elapsed_ms"`
		First200ElapsedMS      int    `json:"first_200_elapsed_ms"`
		BackendLogError        string `json:"backend_log_error"`
		BackendReadyzDuring503 struct {
			Status int    `json:"status"`
			Body   string `json:"body"`
		} `json:"backend_readyz_during_503"`
	} `json:"cold_start_loops_readiness"`
	PlaywrightE2E struct {
		TaskfileChain struct {
			SubtasksTotal                         int `json:"subtasks_total"`
			SubtasksPassedUnderStackUp            int `json:"subtasks_passed_under_stack_up"`
			SubtasksSkipped                       int `json:"subtasks_skipped"`
			SubtasksFailedUnderStackUp            int `json:"subtasks_failed_under_stack_up"`
			SubtasksRecoveredUnderWaitGatedRunner int `json:"subtasks_recovered_under_wait_gated_runner"`
			Subtasks                              []struct {
				Task             string  `json:"task"`
				Spec             string  `json:"spec"`
				StackUpOutcome   string  `json:"stack_up_outcome"`
				WaitGatedOutcome *string `json:"wait_gated_outcome"`
				TestsPassed      int     `json:"tests_passed"`
				TestsSkipped     int     `json:"tests_skipped"`
				TestsFailed      int     `json:"tests_failed"`
				Attribution      string  `json:"attribution"`
			} `json:"subtasks"`
		} `json:"taskfile_chain"`
		OptInAutoresearch struct {
			Total     int `json:"total"`
			Passed    int `json:"passed"`
			Failed    int `json:"failed"`
			Scenarios []struct {
				Task   string `json:"task"`
				Spec   string `json:"spec"`
				Passed bool   `json:"passed"`
			} `json:"scenarios"`
		} `json:"opt_in_autoresearch"`
		BoundaryMatrix struct {
			MatrixFile    string   `json:"matrix_file"`
			SourceChanged bool     `json:"source_changed"`
			Total         int      `json:"total"`
			Passed        int      `json:"passed"`
			Failed        int      `json:"failed"`
			Scenarios     []string `json:"scenarios"`
		} `json:"boundary_matrix"`
		ParkedDonorScaffolds struct {
			Total     int `json:"total"`
			Passed    int `json:"passed"`
			Failed    int `json:"failed"`
			Scenarios []struct {
				Name   string `json:"name"`
				Spec   string `json:"spec"`
				Reason string `json:"reason"`
			} `json:"scenarios"`
		} `json:"parked_donor_scaffolds"`
	} `json:"playwright_e2e"`
}

func parsePinnedNATSImage(content string) string {
	for _, raw := range strings.Split(content, "\n") {
		line := strings.TrimSpace(raw)
		if line != "" && !strings.HasPrefix(line, "#") {
			return line
		}
	}
	return ""
}

func checkBaselineSummaryInvariants(doc baselineDocJSON) error {
	if doc.SchemaVersion != 1 || len(doc.SemTeamsCommit) != 40 {
		return fmt.Errorf("invalid schema_version=%d or semteams_commit=%q", doc.SchemaVersion, doc.SemTeamsCommit)
	}
	if doc.Framework.Module != "github.com/c360studio/semstreams" ||
		doc.Framework.Version != "v1.0.0-beta.162.0.20260930150212-8b99efe9c66a" ||
		doc.Framework.Commit != "8b99efe9c66a4faa4fa509f9f62cc6bad8392128" ||
		doc.Framework.HasLocalReplace {
		return fmt.Errorf("unexpected frozen framework identity: %+v", doc.Framework)
	}
	if !doc.Gates.TaskValidate.Passed || doc.Gates.GoUnitRace.TopLevelTests.Fail != 0 ||
		doc.Gates.GoIntegration.TopLevelTests.Fail != 0 || doc.Gates.UIVitest.TestsFailed != 0 {
		return fmt.Errorf("recorded CI/integration/Vitest floor has failures")
	}
	vitest := doc.Gates.UIVitest
	if vitest.TestFilesTotal != vitest.TestFilesPassed+vitest.TestFilesSkipped ||
		vitest.TestsTotal != vitest.TestsPassed+vitest.TestsSkipped+vitest.TestsFailed {
		return fmt.Errorf("inconsistent ui_vitest counts: %+v", vitest)
	}
	if doc.ColdStartLoopsReadiness.Endpoint != "/teams-dispatch/loops" ||
		doc.ColdStartLoopsReadiness.BackendReadyzDuring503.Status != 200 ||
		doc.ColdStartLoopsReadiness.First503ElapsedMS <= 0 ||
		doc.ColdStartLoopsReadiness.First200ElapsedMS <= doc.ColdStartLoopsReadiness.First503ElapsedMS {
		return fmt.Errorf("invalid cold_start_loops_readiness window: %+v", doc.ColdStartLoopsReadiness)
	}
	return checkBaselinePlaywrightTally(doc)
}

func checkBaselinePlaywrightTally(doc baselineDocJSON) error {
	chain := doc.PlaywrightE2E.TaskfileChain
	if chain.SubtasksTotal != 23 || len(chain.Subtasks) != chain.SubtasksTotal {
		return fmt.Errorf("subtasks len=%d total=%d, want 23", len(chain.Subtasks), chain.SubtasksTotal)
	}
	var passCount, skipCount, failCount, waitRecovered int
	for _, st := range chain.Subtasks {
		switch st.StackUpOutcome {
		case "pass":
			passCount++
		case "skip":
			skipCount++
		case "fail":
			failCount++
			if !strings.Contains(st.Attribution, doc.PreexistingE2EFollowupIssue) {
				return fmt.Errorf("failing subtask %s must cite follow-up %s, got %q", st.Task, doc.PreexistingE2EFollowupIssue, st.Attribution)
			}
		default:
			return fmt.Errorf("subtask %s has invalid stack_up_outcome %q", st.Task, st.StackUpOutcome)
		}
		if st.WaitGatedOutcome != nil && *st.WaitGatedOutcome == "pass" {
			waitRecovered++
		}
	}
	if passCount != chain.SubtasksPassedUnderStackUp || skipCount != chain.SubtasksSkipped ||
		failCount != chain.SubtasksFailedUnderStackUp || waitRecovered != chain.SubtasksRecoveredUnderWaitGatedRunner ||
		passCount+skipCount+failCount != chain.SubtasksTotal {
		return fmt.Errorf("subtask tally mismatch: got %d/%d/%d (recovered %d)", passCount, skipCount, failCount, waitRecovered)
	}
	bm := doc.PlaywrightE2E.BoundaryMatrix
	if bm.SourceChanged || bm.Total != 7 || bm.Passed != 7 || bm.Failed != 0 || len(bm.Scenarios) != 7 {
		return fmt.Errorf("unexpected boundary_matrix summary: %+v", bm)
	}
	optIn := doc.PlaywrightE2E.OptInAutoresearch
	if optIn.Total != 2 || optIn.Passed != 2 || optIn.Failed != 0 || len(optIn.Scenarios) != 2 {
		return fmt.Errorf("unexpected opt_in_autoresearch summary: %+v", optIn)
	}
	donors := doc.PlaywrightE2E.ParkedDonorScaffolds
	if donors.Total != 2 || donors.Passed != 0 || donors.Failed != 2 || len(donors.Scenarios) != 2 {
		return fmt.Errorf("unexpected parked_donor_scaffolds summary: %+v", donors)
	}
	return nil
}

func checkBaselineDocCrossReferences(doc baselineDocJSON, readmeText, ledgerText string) error {
	for _, token := range []string{
		doc.SemTeamsCommit,
		doc.Framework.Version,
		doc.NATS.GoNatstest.PinnedReference,
		doc.NATS.UIE2ECompose.ComposeReference,
		doc.ConsumerQualificationIssue,
		doc.PreexistingE2EFollowupIssue,
		"baseline.json",
		"graph_index_queries",
		"/teams-dispatch/loops",
	} {
		if !strings.Contains(readmeText, token) {
			return fmt.Errorf("README.md missing required baseline token %q", token)
		}
	}
	for _, token := range []string{
		"semengine-port-baseline/README.md",
		"semengine-port-baseline/baseline.json",
		doc.SemTeamsCommit,
		doc.Issue,
		doc.ConsumerQualificationIssue,
		doc.PreexistingE2EFollowupIssue,
	} {
		if !strings.Contains(ledgerText, token) {
			return fmt.Errorf("semengine-port-ledger.md missing required cross-reference token %q", token)
		}
	}
	return nil
}

func collectPackageTestFuncs(t *testing.T, dir string) map[string]bool {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir %s: %v", dir, err)
	}
	fset := token.NewFileSet()
	found := make(map[string]bool)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		fullPath := filepath.Join(dir, entry.Name())
		f, parseErr := parser.ParseFile(fset, fullPath, nil, 0)
		if parseErr != nil {
			t.Fatalf("parse %s: %v", fullPath, parseErr)
		}
		for _, decl := range f.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Recv == nil && strings.HasPrefix(fn.Name.Name, "Test") {
				found[fn.Name.Name] = true
			}
		}
	}
	return found
}

func verifyBaselineRepoOracles(t *testing.T, repoRoot string, doc baselineDocJSON) {
	t.Helper()
	goModBytes, err := os.ReadFile(filepath.Join(repoRoot, "go.mod"))
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}
	// While SemTeams remains on SemStreams ahead of #404, verify live go.mod matches the recorded floor pin.
	if strings.Contains(string(goModBytes), "github.com/c360studio/semstreams ") {
		modCmd := exec.Command("go", "list", "-m", "-json", "github.com/c360studio/semstreams")
		modCmd.Dir = repoRoot
		out, modErr := modCmd.CombinedOutput()
		if modErr != nil {
			t.Fatalf("go list -m semstreams: %v\n%s", modErr, string(out))
		}
		var modInfo struct {
			Version string          `json:"Version"`
			Sum     string          `json:"Sum"`
			Replace json.RawMessage `json:"Replace"`
		}
		if unmarshalErr := json.Unmarshal(out, &modInfo); unmarshalErr != nil {
			t.Fatalf("unmarshal go list -m: %v", unmarshalErr)
		}
		if len(modInfo.Replace) > 0 && string(modInfo.Replace) != "null" {
			t.Fatalf("unexpected semstreams replace directive: %s", string(modInfo.Replace))
		}
		if modInfo.Version != doc.Framework.Version || modInfo.Sum != doc.Framework.Sum {
			t.Fatalf("semstreams module drift: got %s (%s), want %s (%s)", modInfo.Version, modInfo.Sum, doc.Framework.Version, doc.Framework.Sum)
		}
	}

	natsImgBytes, err := os.ReadFile(filepath.Join(repoRoot, "internal", "natstest", "nats-image"))
	if err != nil {
		t.Fatalf("read internal/natstest/nats-image: %v", err)
	}
	if gotPin := parsePinnedNATSImage(string(natsImgBytes)); gotPin != doc.NATS.GoNatstest.PinnedReference {
		t.Fatalf("internal/natstest/nats-image = %q, want %q", gotPin, doc.NATS.GoNatstest.PinnedReference)
	}

	composeBytes, err := os.ReadFile(filepath.Join(repoRoot, "ui", "docker-compose.agentic-e2e.yml"))
	if err != nil {
		t.Fatalf("read ui/docker-compose.agentic-e2e.yml: %v", err)
	}
	if !strings.Contains(string(composeBytes), "image: "+doc.NATS.UIE2ECompose.ComposeReference) {
		t.Fatalf("ui/docker-compose.agentic-e2e.yml missing expected broker image %q", doc.NATS.UIE2ECompose.ComposeReference)
	}

	verifyBaselineSuiteAndSpecFiles(t, repoRoot, doc)
}

func verifyBaselineSuiteAndSpecFiles(t *testing.T, repoRoot string, doc baselineDocJSON) {
	t.Helper()
	funcsByPkg := map[string]map[string]bool{
		"github.com/c360studio/semteams/cmd/semteams":  collectPackageTestFuncs(t, filepath.Join(repoRoot, "cmd", "semteams")),
		"github.com/c360studio/semteams/test/contract": collectPackageTestFuncs(t, filepath.Join(repoRoot, "test", "contract")),
	}
	for suiteKey, entries := range doc.RuntimeBoundarySuites {
		if len(entries) == 0 {
			t.Fatalf("runtime_boundary_suites[%q] must not be empty", suiteKey)
		}
		for _, entry := range entries {
			parts := strings.SplitN(entry, ":", 2)
			if len(parts) != 2 {
				t.Fatalf("malformed suite entry %q", entry)
			}
			topFunc := strings.SplitN(parts[1], "/", 2)[0]
			if !funcsByPkg[parts[0]][topFunc] {
				t.Fatalf("recorded boundary test %s (%s) not found on disk", topFunc, parts[0])
			}
		}
	}

	e2eDir := filepath.Join(repoRoot, "ui", "e2e", "agentic")
	for _, st := range doc.PlaywrightE2E.TaskfileChain.Subtasks {
		if _, err := os.Stat(filepath.Join(e2eDir, st.Spec)); err != nil {
			t.Fatalf("missing E2E spec %s: %v", st.Spec, err)
		}
	}
	for _, sc := range doc.PlaywrightE2E.OptInAutoresearch.Scenarios {
		if _, err := os.Stat(filepath.Join(e2eDir, sc.Spec)); err != nil {
			t.Fatalf("missing opt-in E2E spec %s: %v", sc.Spec, err)
		}
	}
	for _, ds := range doc.PlaywrightE2E.ParkedDonorScaffolds.Scenarios {
		if _, err := os.Stat(filepath.Join(e2eDir, ds.Spec)); err != nil {
			t.Fatalf("missing donor scaffold spec %s: %v", ds.Spec, err)
		}
	}

	matrixBytes, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(doc.PlaywrightE2E.BoundaryMatrix.MatrixFile)))
	if err != nil {
		t.Fatalf("read matrix file: %v", err)
	}
	var matrixScenarios []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(matrixBytes, &matrixScenarios); err != nil {
		t.Fatalf("unmarshal migration matrix: %v", err)
	}
	matrixNames := make(map[string]bool, len(matrixScenarios))
	for _, s := range matrixScenarios {
		matrixNames[s.Name] = true
	}
	for _, name := range doc.PlaywrightE2E.BoundaryMatrix.Scenarios {
		if !matrixNames[name] {
			t.Fatalf("boundary scenario %q missing from %s", name, doc.PlaywrightE2E.BoundaryMatrix.MatrixFile)
		}
	}
}

func checkCompositionMatchesBaseline(repoRoot string, doc baselineDocJSON) error {
	if doc.CompositionLedger.CatalogFactoryCount != 9 || doc.CompositionLedger.WarningCount != 9 ||
		len(doc.CompositionLedger.Warnings) != 9 || doc.CompositionLedger.ErrorCount != 0 ||
		doc.CompositionLedger.Status != "warnings" || !doc.CompositionLedger.LiveBootMatchesOfflineValidate {
		return fmt.Errorf("unexpected composition_ledger header: %+v", doc.CompositionLedger)
	}
	if len(doc.CompositionLedger.BootstrapConfigs) != 2 {
		return fmt.Errorf("expected 2 bootstrap_configs, got %d", len(doc.CompositionLedger.BootstrapConfigs))
	}
	for _, relCfg := range doc.CompositionLedger.BootstrapConfigs {
		status, errCount, liveWarnings, factoryCount, err := inspectBootstrapComposition(filepath.Join(repoRoot, filepath.FromSlash(relCfg)))
		if err != nil {
			return fmt.Errorf("validate %s: %w", relCfg, err)
		}
		if factoryCount != doc.CompositionLedger.CatalogFactoryCount {
			return fmt.Errorf("%s catalog factory count = %d, want %d", relCfg, factoryCount, doc.CompositionLedger.CatalogFactoryCount)
		}
		if status != doc.CompositionLedger.Status || errCount != doc.CompositionLedger.ErrorCount {
			return fmt.Errorf("%s status=%q errors=%d, want %s/0", relCfg, status, errCount, doc.CompositionLedger.Status)
		}
		if !reflect.DeepEqual(liveWarnings, doc.CompositionLedger.Warnings) {
			return fmt.Errorf("%s composition warnings drifted from baseline:\ngot:  %+v\nwant: %+v", relCfg, liveWarnings, doc.CompositionLedger.Warnings)
		}
	}
	return nil
}

// spec: openspec/specs/framework-adoption/spec.md#Requirement: Frozen framework adoption preserves qualified product behavior
func TestSemEnginePortBaseline(t *testing.T) {
	repoRoot, err := findRepoRoot()
	if err != nil {
		t.Fatalf("findRepoRoot: %v", err)
	}

	baselineJSONPath := filepath.Join(repoRoot, "docs", "migrations", "semengine-port-baseline", "baseline.json")
	rawJSON, err := os.ReadFile(baselineJSONPath)
	if err != nil {
		t.Fatalf("read %s: %v", baselineJSONPath, err)
	}
	var doc baselineDocJSON
	if err := json.Unmarshal(rawJSON, &doc); err != nil {
		t.Fatalf("unmarshal baseline.json: %v", err)
	}

	readmeBytes, err := os.ReadFile(filepath.Join(repoRoot, "docs", "migrations", "semengine-port-baseline", "README.md"))
	if err != nil {
		t.Fatalf("read baseline README.md: %v", err)
	}
	ledgerBytes, err := os.ReadFile(filepath.Join(repoRoot, "docs", "migrations", "semengine-port-ledger.md"))
	if err != nil {
		t.Fatalf("read semengine-port-ledger.md: %v", err)
	}

	t.Run("summary_and_repo_oracles_match_live_main_floor", func(t *testing.T) {
		if err := checkBaselineSummaryInvariants(doc); err != nil {
			t.Fatalf("summary invariant failed: %v", err)
		}
		if err := checkBaselineDocCrossReferences(doc, string(readmeBytes), string(ledgerBytes)); err != nil {
			t.Fatalf("cross-reference check failed: %v", err)
		}
		verifyBaselineRepoOracles(t, repoRoot, doc)
		if err := checkCompositionMatchesBaseline(repoRoot, doc); err != nil {
			t.Fatalf("composition parity failed: %v", err)
		}
	})

	t.Run("sensitivity_detects_composition_warning_drift", func(t *testing.T) {
		mutated := doc
		mutated.CompositionLedger.Warnings = append([]baselineCompositionWarning(nil), doc.CompositionLedger.Warnings...)
		mutated.CompositionLedger.Warnings[3].Port = "mutated_port"
		if err := checkCompositionMatchesBaseline(repoRoot, mutated); err == nil {
			t.Fatal("expected checkCompositionMatchesBaseline to reject mutated composition warning")
		}
	})

	t.Run("sensitivity_detects_unattributed_e2e_failure_and_ledger_link_removal", func(t *testing.T) {
		mutated := doc
		if err := json.Unmarshal(rawJSON, &mutated); err != nil {
			t.Fatalf("re-unmarshal: %v", err)
		}
		mutated.PlaywrightE2E.TaskfileChain.Subtasks[0].Attribution = "unexplained_failure"
		if err := checkBaselinePlaywrightTally(mutated); err == nil {
			t.Fatal("expected checkBaselinePlaywrightTally to reject failing E2E subtask missing follow-up issue citation")
		}
		strippedLedger := strings.ReplaceAll(string(ledgerBytes), "semengine-port-baseline/baseline.json", "removed-link")
		if err := checkBaselineDocCrossReferences(doc, string(readmeBytes), strippedLedger); err == nil {
			t.Fatal("expected checkBaselineDocCrossReferences to reject missing baseline.json link in semengine-port-ledger.md")
		}
	})
}
