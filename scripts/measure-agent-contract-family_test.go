//go:build ignore

package main

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime/debug"
	"testing"
)

// Run explicitly with the analysis script so other build-ignored script mains
// do not become part of this test package.
func TestScanDistinguishesBuildContextsAndExactRoots(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"agentic/root.go":             "package agentic\nimport \"fmt\"\n",
		"agentic/root_test.go":        "package agentic\nfunc TestRoot() {}\n",
		"agentic/child/child.go":      "package child\nimport \"encoding/json\"\n",
		"agentic/child/child_test.go": "package child\nfunc TestChild() {}\n",
		"consumer/import.go":          "package consumer\nimport _ \"github.com/c360studio/semstreams/agentic\"\n",
		"tagged/import_test.go":       "//go:build integration\n\npackage tagged\nimport _ \"github.com/c360studio/semstreams/agentic\"\n",
		"ignored/import.go":           "//go:build ignore\n\npackage ignored\nimport _ \"github.com/c360studio/semstreams/agentic\"\n",
		"prefix/import.go":            "package prefix\nimport _ \"github.com/c360studio/semstreams/agentic-extra\"\n",
	}
	for name, source := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(source), 0644); err != nil {
			t.Fatal(err)
		}
	}
	r, candidates, err := scan(dir, frameworkModule, true)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(candidates["default"], []string{"./consumer"}) {
		t.Fatalf("default candidates %v", candidates["default"])
	}
	if !reflect.DeepEqual(candidates["integration"], []string{"./consumer", "./tagged"}) {
		t.Fatalf("integration candidates %v", candidates["integration"])
	}
	if len(r.TestInventory) != 1 || r.TestInventory[0].Name != "TestRoot" {
		t.Fatalf("exact-root inventory %v", r.TestInventory)
	}
	if len(r.DirectEdges) != 4 {
		t.Fatalf("source import count %d, want root forward + three reverse", len(r.DirectEdges))
	}
	for _, edge := range r.DirectEdges {
		if edge.Line < 1 || edge.Column < 1 {
			t.Fatalf("missing source position: %v", edge)
		}
	}
	if r.SourceSHA256 == "" || len(r.SourceFiles) != len(files) || len(r.MeasuredFileSHA256) != 2 {
		t.Fatalf("incomplete source provenance: %+v", r)
	}
}

func TestConsolidatePreservesDeclarationIdentityAndContexts(t *testing.T) {
	first := symbol{Package: "target", Name: "Value", Declaration: "type target.Value int", DeclaredAt: loc{"first.go", 2, 1}, References: []ref{{nil, "production", "consumer", loc{"use.go", 4, 2}}}}
	second := first
	second.DeclaredAt = loc{"second.go", 2, 1}
	r := repo{Consumers: map[string]typedScope{"default": {Symbols: []symbol{first, second}}, "integration": {Symbols: []symbol{first}}}}
	consolidate(&r)
	if len(r.Symbols) != 2 {
		t.Fatalf("distinct declarations collapsed: %v", r.Symbols)
	}
	if !reflect.DeepEqual(r.Symbols[0].References[0].Contexts, []string{"default", "integration"}) {
		t.Fatalf("lost contexts: %v", r.Symbols[0].References)
	}
	if r.Consumers["default"].ReferenceCount != 2 || r.Consumers["integration"].ReferenceCount != 1 {
		t.Fatal("lost per-context coverage")
	}
}

func TestNominalTypeProbe(t *testing.T) {
	got := nominalProbe()
	for _, key := range []string{"same_wire_json", "alias_retains_frozen_identity", "duplicate_registration_rejected", "shared_error_preserves_identity"} {
		if got[key] != true {
			t.Errorf("%s = %v, want true", key, got[key])
		}
	}
	for _, key := range []string{"relocated_assignable_to_frozen", "relocated_passes_frozen_type_assertion", "same_text_error_preserves_identity"} {
		if got[key] != false {
			t.Errorf("%s = %v, want false", key, got[key])
		}
	}
	if got["alias_definition_package"] != frameworkModule+"/agentic" {
		t.Fatalf("alias owner %v", got["alias_definition_package"])
	}
}

func TestProbeModuleIdentityRejectsMismatchedAuthority(t *testing.T) {
	selected := selectedFramework{Version: "v1.0.0-frozen", Sum: "h1:frozen"}
	valid := func() *debug.BuildInfo {
		return &debug.BuildInfo{Deps: []*debug.Module{{Path: frameworkModule, Version: selected.Version, Sum: selected.Sum}}}
	}
	if err := verifyProbeModule(selected, valid()); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name  string
		alter func(*debug.BuildInfo)
	}{
		{"version", func(b *debug.BuildInfo) { b.Deps[0].Version = "v1.0.0-other" }},
		{"checksum", func(b *debug.BuildInfo) { b.Deps[0].Sum = "h1:other" }},
		{"compiled replacement", func(b *debug.BuildInfo) { b.Deps[0].Replace = &debug.Module{Path: "local"} }},
		{"missing dependency", func(b *debug.BuildInfo) { b.Deps = nil }},
	} {
		t.Run(test.name, func(t *testing.T) {
			b := valid()
			test.alter(b)
			if err := verifyProbeModule(selected, b); err == nil {
				t.Fatal("mismatched compiled probe accepted")
			}
		})
	}
	selected.Replace = &struct{}{}
	if err := verifyProbeModule(selected, valid()); err == nil {
		t.Fatal("consumer replacement accepted")
	}
}

func TestSymbolsIncludePackageVariables(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"go.mod":           "module github.com/c360studio/semstreams\n\ngo 1.26.3\n",
		"agentic/types.go": "package agentic\nvar ErrToolNotFound = 1\ntype Result struct { ExportedField int }\n",
		"consumer/use.go":  "package consumer\nimport \"github.com/c360studio/semstreams/agentic\"\nvar _ = agentic.ErrToolNotFound\nvar _ = agentic.Result{}.ExportedField\n",
	}
	for name, source := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(source), 0644); err != nil {
			t.Fatal(err)
		}
	}
	scope := symbols(dir, dir, []string{"./consumer"}, nil)
	if len(scope.Errors) != 0 {
		t.Fatal(scope.Errors)
	}
	found := false
	for _, s := range scope.Symbols {
		if s.Name == "ExportedField" {
			t.Error("struct field included as package variable")
		}
		if s.Name == "ErrToolNotFound" {
			found = true
			if s.Kind != "variable" || len(s.References) != 1 || s.References[0].File != "consumer/use.go" || s.References[0].Line != 3 {
				t.Fatalf("incorrect variable reference: %+v", s)
			}
		}
	}
	if !found {
		t.Fatal("exported package sentinel reference omitted")
	}
}
