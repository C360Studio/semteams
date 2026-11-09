package contract

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"go/parser"
	"go/token"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

type manifestDocJSON struct {
	SchemaVersion         int    `json:"schema_version"`
	SourceModule          string `json:"source_module"`
	SourceVersion         string `json:"source_version"`
	SourceCommit          string `json:"source_commit"`
	SourceSum             string `json:"source_sum"`
	DestinationModule     string `json:"destination_module"`
	TargetSubstrateModule string `json:"target_substrate_module"`
	License               struct {
		SourcePath      string `json:"source_path"`
		DestinationPath string `json:"destination_path"`
		SPDXID          string `json:"spdx_id"`
		CopyrightHolder string `json:"copyright_holder"`
		Bytes           int64  `json:"bytes"`
		SHA256          string `json:"sha256"`
		NoticeVerified  bool   `json:"notice_verified"`
	} `json:"license"`
	TotalAbsorbedUnits      int   `json:"total_absorbed_units"`
	TotalManifestFiles      int   `json:"total_manifest_files"`
	TotalProdGoFiles        int   `json:"total_prod_go_files"`
	TotalTestGoFiles        int   `json:"total_test_go_files"`
	TotalNonGoFiles         int   `json:"total_non_go_files"`
	TotalBytes              int64 `json:"total_bytes"`
	InternalAbsorbedImports int   `json:"internal_absorbed_import_edges"`
	ExcludedDonorPackages   []struct {
		SourceSubpath string `json:"source_subpath"`
		DonorProdGo   int    `json:"donor_prod_go_files"`
		DonorTestGo   int    `json:"donor_test_go_files"`
		Reason        string `json:"reason"`
	} `json:"excluded_donor_packages"`
	NewFiles []struct {
		DestinationPath string `json:"destination_path"`
		TargetIssue     string `json:"target_issue"`
		Reason          string `json:"reason"`
	} `json:"new_files"`
	Units []struct {
		ID                  string `json:"id"`
		SourceSubpath       string `json:"source_subpath"`
		DestinationDir      string `json:"destination_dir"`
		ScanMode            string `json:"scan_mode"`
		TargetIssue         string `json:"target_issue"`
		UnitAdaptationClass string `json:"unit_adaptation_class"`
		ProdGoFiles         int    `json:"prod_go_files"`
		TestGoFiles         int    `json:"test_go_files"`
		OtherFiles          int    `json:"other_files"`
		TotalFiles          int    `json:"total_files"`
		TotalBytes          int64  `json:"total_bytes"`
	} `json:"units"`
	ExternalImports []struct {
		Subpath     string `json:"subpath"`
		Category    string `json:"category"`
		Replacement string `json:"replacement"`
		TargetIssue string `json:"target_issue"`
		ProdFiles   int    `json:"prod_go_files"`
		TestFiles   int    `json:"test_go_files"`
	} `json:"external_semstreams_imports"`
	Files []struct {
		UnitID             string   `json:"unit_id"`
		SourcePath         string   `json:"source_path"`
		DestinationPath    string   `json:"destination_path"`
		Kind               string   `json:"kind"`
		Bytes              int64    `json:"bytes"`
		SHA256             string   `json:"sha256"`
		AdaptationClass    string   `json:"adaptation_class"`
		TargetIssue        string   `json:"target_issue"`
		AdaptationTriggers []string `json:"adaptation_triggers"`
	} `json:"files"`
}

func buildManifestGenerator(t *testing.T, repoRoot string, src []byte) string {
	t.Helper()
	tmpDir := t.TempDir()
	srcFile := filepath.Join(tmpDir, "generate-semengine-port-manifest.go")
	if err := os.WriteFile(srcFile, src, 0o644); err != nil {
		t.Fatalf("write temp generator source: %v", err)
	}
	binFile := filepath.Join(tmpDir, "generate-semengine-port-manifest")
	cmd := exec.Command("go", "build", "-o", binFile, srcFile)
	cmd.Dir = repoRoot
	if out, buildErr := cmd.CombinedOutput(); buildErr != nil {
		t.Fatalf("compile manifest generator: %v\n%s", buildErr, string(out))
	}
	return binFile
}

func checkManifestDocMatchesGenerated(docContent, expectedMarkdown string) error {
	const (
		beginMarker = "<!-- BEGIN GENERATED MANIFEST SUMMARY -->"
		endMarker   = "<!-- END GENERATED MANIFEST SUMMARY -->"
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

func writeSyntheticDonorTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	const validMITLicense = `MIT License

Copyright (c) 2025 C360

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software.

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.
`
	files := map[string]string{
		"LICENSE":                                              validMITLicense,
		"agentic/types.go":                                     "package agentic\n",
		"agentic/identity/did.go":                              "package identity\n",
		"agentic/research/fusion.go":                           "package research\nimport _ \"github.com/c360studio/semstreams/pkg/fusion\"\n",
		"agentic/agentrun/agentrun.go":                         "package agentrun\nimport _ \"github.com/c360studio/semstreams/agentic\"\n",
		"service/milestone_service.go":                         "package service\n",
		"service/milestone_service_test.go":                    "package service\n",
		"service/milestone_health_test.go":                     "package service\n",
		"service/other_service.go":                             "package service\n",
		"vocabulary/agentic/predicates.go":                     "package agentic\n",
		"internal/looptoken/token.go":                          "package looptoken\n",
		"persona/loader.go":                                    "package persona\n",
		"internal/deliverylane/lane.go":                        "package deliverylane\n",
		"internal/agentterminal/terminal.go":                   "package agentterminal\n",
		"model/wire/client.go":                                 "package wire\n",
		"model/wire/testdata/sample.json":                      "{}\n",
		"model/wire/responses/client.go":                       "package responses\n",
		"model/wire/responses/testdata/sample.json":            "{}\n",
		"processor/agentic-loop/component.go":                  "package agenticloop\nimport _ \"github.com/c360studio/semstreams/component\"\n",
		"processor/agentic-loop/internal/loopbucket/bucket.go": "package loopbucket\n",
		"processor/agentic-loop/internal/looprequest/req.go":   "package looprequest\n",
		"processor/agentic-loop/lessonmatch/match.go":          "package lessonmatch\n",
		"processor/agentic-loop/prompt/prompt.go":              "package prompt\n",
		"processor/agentic-dispatch/component.go":              "package agenticdispatch\n",
		"processor/agentic-model/component.go":                 "package agenticmodel\n",
		"processor/agentic-tools/component.go":                 "package agentictools\n",
		"processor/agentic-tools/executors/bash.go":            "package executors\n",
		"processor/agentic-tools/runner/runner.go":             "package runner\n",
		"gateway/graph-gateway/component.go":                   "package graphgateway\n",
	}
	writeLedgerFixtureFiles(t, root, files)
	return root
}

func copyFileAndSHA256(t *testing.T, src, dst string) string {
	t.Helper()
	in, err := os.Open(src)
	if err != nil {
		t.Fatalf("open %s: %v", src, err)
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		t.Fatalf("create %s: %v", dst, err)
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		t.Fatalf("copy %s -> %s: %v", src, dst, err)
	}
	if err := out.Close(); err != nil {
		t.Fatalf("close %s: %v", dst, err)
	}

	copiedBytes, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read copied %s: %v", dst, err)
	}
	sum := sha256.Sum256(copiedBytes)
	return hex.EncodeToString(sum[:])
}

// spec: openspec/changes/semengine-port-adaptation-design/specs/runtime-composition/spec.md#Requirement: Absorbed agent-runtime and gateway port blueprint
func TestSemEnginePortManifest(t *testing.T) {
	repoRoot, err := findRepoRoot()
	if err != nil {
		t.Fatalf("findRepoRoot: %v", err)
	}

	scriptPath := filepath.Join(repoRoot, "scripts", "generate-semengine-port-manifest.go")
	scriptBytes, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("read %s: %v", scriptPath, err)
	}

	genBin := buildManifestGenerator(t, repoRoot, scriptBytes)

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
			t.Fatal("scripts/generate-semengine-port-manifest.go must declare //go:build ignore")
		}
		for _, imp := range fileAST.Imports {
			impPath, unquoteErr := strconv.Unquote(imp.Path.Value)
			if unquoteErr != nil {
				t.Fatalf("unquote import %s: %v", imp.Path.Value, unquoteErr)
			}
			if strings.Contains(impPath, ".") {
				t.Fatalf("non-stdlib import %q in scripts/generate-semengine-port-manifest.go; generator must stay stdlib-only", impPath)
			}
		}
	})

	t.Run("live_frozen_cache_matches_committed_manifest_and_readme_and_oracle", func(t *testing.T) {
		liveJSON, jsonCode := runLedgerAnalyzer(genBin, "-root", repoRoot, "-format=json")
		if jsonCode != 0 {
			t.Fatalf("generator json mode failed (exit %d):\n%s", jsonCode, liveJSON)
		}

		manifestJSONPath := filepath.Join(repoRoot, "docs", "migrations", "semengine-port-manifest", "manifest.json")
		committedJSONBytes, readErr := os.ReadFile(manifestJSONPath)
		if readErr != nil {
			t.Fatalf("read %s: %v", manifestJSONPath, readErr)
		}
		if !bytes.Equal(committedJSONBytes, []byte(liveJSON)) {
			t.Fatalf("committed manifest.json is out of sync with live generator output; refresh with `go run ./scripts/generate-semengine-port-manifest.go -write-json docs/migrations/semengine-port-manifest/manifest.json`")
		}

		var doc manifestDocJSON
		if unmarshalErr := json.Unmarshal(committedJSONBytes, &doc); unmarshalErr != nil {
			t.Fatalf("unmarshal manifest.json: %v", unmarshalErr)
		}

		if doc.SourceVersion != "v1.0.0-beta.162.0.20260930150212-8b99efe9c66a" {
			t.Fatalf("source_version = %q", doc.SourceVersion)
		}
		if doc.SourceCommit != "8b99efe9c66a4faa4fa509f9f62cc6bad8392128" {
			t.Fatalf("source_commit = %q", doc.SourceCommit)
		}
		if doc.SourceSum != "h1:WAlGLMjE/3EnYyeoqounCfYnVyIB3igMlG+FLP1Iq+o=" {
			t.Fatalf("source_sum = %q", doc.SourceSum)
		}
		if doc.TotalAbsorbedUnits != 21 || len(doc.Units) != 21 {
			t.Fatalf("units = %d/%d, want 21", doc.TotalAbsorbedUnits, len(doc.Units))
		}
		if !doc.License.NoticeVerified || doc.License.SPDXID != "MIT" || doc.License.DestinationPath != "internal/agentic/LICENSE" {
			t.Fatalf("unexpected license provenance: %+v", doc.License)
		}

		// Independent oracle: query go list -m directly and verify every manifest file's size + SHA-256 on disk.
		modCmd := exec.Command("go", "list", "-m", "-json", "github.com/c360studio/semstreams")
		modCmd.Dir = repoRoot
		modOut, modErr := modCmd.CombinedOutput()
		if modErr != nil {
			t.Fatalf("go list -m -json semstreams: %v\n%s", modErr, string(modOut))
		}
		var modInfo struct {
			Version string `json:"Version"`
			Sum     string `json:"Sum"`
			Dir     string `json:"Dir"`
			Replace *struct {
				Dir string `json:"Dir"`
			} `json:"Replace"`
		}
		if err := json.Unmarshal(modOut, &modInfo); err != nil {
			t.Fatalf("parse go list -m output: %v", err)
		}
		if modInfo.Replace != nil {
			t.Fatalf("unexpected replace directive on semstreams: %+v", modInfo.Replace)
		}
		if modInfo.Version != doc.SourceVersion || modInfo.Sum != doc.SourceSum {
			t.Fatalf("module metadata mismatch: got %s %s", modInfo.Version, modInfo.Sum)
		}

		seenDestinations := make(map[string]bool, len(doc.Files)+len(doc.NewFiles))
		allowedClasses := map[string]bool{
			"verbatim-copy":                       true,
			"verbatim-plus-import-rewrite":        true,
			"structural-adaptation":               true,
			"replace-with-trimmed-implementation": true,
		}
		for _, f := range doc.Files {
			if !strings.HasPrefix(f.DestinationPath, "internal/") {
				t.Fatalf("destination_path %q escapes internal/", f.DestinationPath)
			}
			if seenDestinations[f.DestinationPath] {
				t.Fatalf("duplicate destination_path %q", f.DestinationPath)
			}
			seenDestinations[f.DestinationPath] = true
			if !allowedClasses[f.AdaptationClass] {
				t.Fatalf("file %s has unknown adaptation_class %q", f.SourcePath, f.AdaptationClass)
			}

			diskPath := filepath.Join(modInfo.Dir, filepath.FromSlash(f.SourcePath))
			diskBytes, diskErr := os.ReadFile(diskPath)
			if diskErr != nil {
				t.Fatalf("oracle read %s: %v", diskPath, diskErr)
			}
			if int64(len(diskBytes)) != f.Bytes {
				t.Fatalf("oracle byte length mismatch for %s: manifest=%d disk=%d", f.SourcePath, f.Bytes, len(diskBytes))
			}
			sum := sha256.Sum256(diskBytes)
			if gotHex := hex.EncodeToString(sum[:]); gotHex != f.SHA256 {
				t.Fatalf("oracle sha256 mismatch for %s: manifest=%s disk=%s", f.SourcePath, f.SHA256, gotHex)
			}
		}

		// Verify verbatim copy round-trip into temp destination matches recorded SHA-256.
		copyTmp := t.TempDir()
		copiedLicSHA := copyFileAndSHA256(t, filepath.Join(modInfo.Dir, "LICENSE"), filepath.Join(copyTmp, "LICENSE"))
		if copiedLicSHA != doc.License.SHA256 {
			t.Fatalf("copied LICENSE sha256 = %s, want %s", copiedLicSHA, doc.License.SHA256)
		}

		wantNewFiles := map[string]bool{
			"internal/agentic/looptoken/token_test.go": false,
			"internal/agentic/graphmutation/client.go": false,
			"internal/agentic/rulefamily/register.go":  false,
		}
		for _, nf := range doc.NewFiles {
			if seenDestinations[nf.DestinationPath] {
				t.Fatalf("new_file %q collides with absorbed destination_path", nf.DestinationPath)
			}
			seenDestinations[nf.DestinationPath] = true
			if _, ok := wantNewFiles[nf.DestinationPath]; ok {
				wantNewFiles[nf.DestinationPath] = true
			}
		}
		for path, found := range wantNewFiles {
			if !found {
				t.Fatalf("manifest new_files missing required obligation %q", path)
			}
		}

		if len(doc.ExcludedDonorPackages) != 2 ||
			doc.ExcludedDonorPackages[0].SourceSubpath != "agentic/identity" ||
			doc.ExcludedDonorPackages[1].SourceSubpath != "agentic/research" {
			t.Fatalf("unexpected excluded_donor_packages: %+v", doc.ExcludedDonorPackages)
		}

		liveMD, mdCode := runLedgerAnalyzer(genBin, "-root", repoRoot, "-format=markdown")
		if mdCode != 0 {
			t.Fatalf("generator markdown mode failed (exit %d):\n%s", mdCode, liveMD)
		}
		readmePath := filepath.Join(repoRoot, "docs", "migrations", "semengine-port-manifest", "README.md")
		readmeBytes, readmeErr := os.ReadFile(readmePath)
		if readmeErr != nil {
			t.Fatalf("read %s: %v", readmePath, readmeErr)
		}
		readmeText := string(readmeBytes)
		if driftErr := checkManifestDocMatchesGenerated(readmeText, liveMD); driftErr != nil {
			t.Fatalf("docs/migrations/semengine-port-manifest/README.md generated summary block is stale:\n%v", driftErr)
		}
		for _, requiredToken := range []string{
			"internal/agentic/LICENSE",
			"internal/agentic/looptoken/token_test.go",
			"RegisterWithExecutorRegistry",
			"RegisterWithToolRegistry",
			"semengine/pkg/lifecyclecleanup",
			"semengine.graph.mutation",
			"TrajectoryPage",
			"kvRevision",
			"graph_index_queries",
			"mcp_path",
		} {
			if !strings.Contains(readmeText, requiredToken) {
				t.Fatalf("README.md missing required blueprint token %q", requiredToken)
			}
		}
	})

	t.Run("synthetic_fixture_enforces_fail_closed_invariants", func(t *testing.T) {
		cleanRepo := t.TempDir()
		baseDonor := writeSyntheticDonorTree(t)
		out, code := runLedgerAnalyzer(genBin, "-root", cleanRepo, "-framework-dir", baseDonor, "-format=json")
		if code != 0 {
			t.Fatalf("baseline synthetic donor tree failed unexpectedly (exit %d):\n%s", code, out)
		}

		t.Run("missing_mit_copyright_notice", func(t *testing.T) {
			donor := writeSyntheticDonorTree(t)
			if err := os.WriteFile(filepath.Join(donor, "LICENSE"), []byte("Apache License 2.0\n"), 0o644); err != nil {
				t.Fatalf("overwrite LICENSE: %v", err)
			}
			errOut, errCode := runLedgerAnalyzer(genBin, "-root", cleanRepo, "-framework-dir", donor, "-format=json")
			if errCode == 0 || !strings.Contains(errOut, "missing required MIT provenance notice") {
				t.Fatalf("expected non-zero exit on missing MIT copyright notice, got %d:\n%s", errCode, errOut)
			}
		})

		t.Run("forbidden_agentic_research_import", func(t *testing.T) {
			donor := writeSyntheticDonorTree(t)
			badGo := "package agenticloop\nimport _ \"github.com/c360studio/semstreams/agentic/research\"\n"
			if err := os.WriteFile(filepath.Join(donor, "processor", "agentic-loop", "component.go"), []byte(badGo), 0o644); err != nil {
				t.Fatalf("write forbidden import: %v", err)
			}
			errOut, errCode := runLedgerAnalyzer(genBin, "-root", cleanRepo, "-framework-dir", donor, "-format=json")
			if errCode == 0 || !strings.Contains(errOut, "forbidden SemStreams import(s)") || !strings.Contains(errOut, "agentic/research") {
				t.Fatalf("expected non-zero exit on forbidden agentic/research import, got %d:\n%s", errCode, errOut)
			}
		})

		t.Run("unclassified_external_semstreams_import", func(t *testing.T) {
			donor := writeSyntheticDonorTree(t)
			badGo := "package agentic\nimport _ \"github.com/c360studio/semstreams/unclassified/mystery\"\n"
			if err := os.WriteFile(filepath.Join(donor, "agentic", "types.go"), []byte(badGo), 0o644); err != nil {
				t.Fatalf("write unclassified import: %v", err)
			}
			errOut, errCode := runLedgerAnalyzer(genBin, "-root", cleanRepo, "-framework-dir", donor, "-format=json")
			if errCode == 0 || !strings.Contains(errOut, "unclassified SemStreams import(s)") || !strings.Contains(errOut, "unclassified/mystery") {
				t.Fatalf("expected non-zero exit on unclassified import, got %d:\n%s", errCode, errOut)
			}
		})

		t.Run("test_only_import_in_prod_file", func(t *testing.T) {
			donor := writeSyntheticDonorTree(t)
			badGo := "package agentic\nimport _ \"github.com/c360studio/semstreams/payloadbuiltins\"\n"
			if err := os.WriteFile(filepath.Join(donor, "agentic", "types.go"), []byte(badGo), 0o644); err != nil {
				t.Fatalf("write test-only import into prod file: %v", err)
			}
			errOut, errCode := runLedgerAnalyzer(genBin, "-root", cleanRepo, "-framework-dir", donor, "-format=json")
			if errCode == 0 || !strings.Contains(errOut, "test_only_rewire imported by non-test file") {
				t.Fatalf("expected non-zero exit on test_only_rewire in prod file, got %d:\n%s", errCode, errOut)
			}
		})

		t.Run("unaccounted_milestone_file_in_service_slice", func(t *testing.T) {
			donor := writeSyntheticDonorTree(t)
			if err := os.WriteFile(filepath.Join(donor, "service", "milestone_extra.go"), []byte("package service\n"), 0o644); err != nil {
				t.Fatalf("write extra milestone file: %v", err)
			}
			errOut, errCode := runLedgerAnalyzer(genBin, "-root", cleanRepo, "-framework-dir", donor, "-format=json")
			if errCode == 0 || !strings.Contains(errOut, "unaccounted milestone* entry") {
				t.Fatalf("expected non-zero exit on extra service/milestone* file, got %d:\n%s", errCode, errOut)
			}
		})
	})

	t.Run("sensitivity_stale_readme_summary_block_is_detected", func(t *testing.T) {
		liveMD, mdCode := runLedgerAnalyzer(genBin, "-root", repoRoot, "-format=markdown")
		if mdCode != 0 {
			t.Fatalf("generator markdown mode failed: %s", liveMD)
		}
		readmePath := filepath.Join(repoRoot, "docs", "migrations", "semengine-port-manifest", "README.md")
		readmeBytes, err := os.ReadFile(readmePath)
		if err != nil {
			t.Fatalf("read README.md: %v", err)
		}
		mutated := strings.Replace(string(readmeBytes), "| `persona` | `persona` | `internal/persona` | 4 |", "| `persona` | `persona` | `internal/persona` | 99 |", 1)
		if mutated == string(readmeBytes) {
			t.Fatal("mutation did not apply to README.md")
		}
		if driftErr := checkManifestDocMatchesGenerated(mutated, liveMD); driftErr == nil {
			t.Fatal("expected checkManifestDocMatchesGenerated to reject stale generated summary block")
		}
	})

	t.Run("sensitivity_disabling_forbidden_import_guard_is_detected", func(t *testing.T) {
		mutatedSrc := strings.Replace(
			string(scriptBytes),
			"if len(forbiddenViolations) > 0 {",
			"if false && len(forbiddenViolations) > 0 {",
			1,
		)
		if mutatedSrc == string(scriptBytes) {
			t.Fatal("mutation did not apply to forbiddenViolations guard")
		}
		mutatedBin := buildManifestGenerator(t, repoRoot, []byte(mutatedSrc))
		cleanRepo := t.TempDir()
		donor := writeSyntheticDonorTree(t)
		badGo := "package agenticloop\nimport _ \"github.com/c360studio/semstreams/agentic/research\"\n"
		if err := os.WriteFile(filepath.Join(donor, "processor", "agentic-loop", "component.go"), []byte(badGo), 0o644); err != nil {
			t.Fatalf("write forbidden import: %v", err)
		}
		_, code := runLedgerAnalyzer(mutatedBin, "-root", cleanRepo, "-framework-dir", donor, "-format=json")
		if code != 0 {
			t.Fatal("expected mutated guard to exit 0, proving forbidden_agentic_research_import sensitivity")
		}
	})
}
