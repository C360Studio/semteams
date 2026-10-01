//go:build ignore

// Measure compiler-resolved framework references and selected dependency closure.
// Usage from the consumer module: go run /path/to/measure-framework-footprint.go
package main

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/tools/go/packages"
)

const framework = "github.com/c360studio/semstreams"

type location struct {
	File   string `json:"file"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
}
type reference struct {
	location
	SourceKind string `json:"source_kind"`
	Call       bool   `json:"call"`
}
type symbol struct {
	Package     string      `json:"package"`
	Name        string      `json:"name"`
	Declaration string      `json:"declaration"`
	DeclaredAt  location    `json:"declared_at"`
	References  []reference `json:"references"`
}
type importEdge struct {
	From       string `json:"from"`
	To         string `json:"to"`
	SourceKind string `json:"source_kind"`
	location
}
type closure struct {
	Pattern           string       `json:"pattern"`
	Tests             bool         `json:"tests"`
	Tags              []string     `json:"tags"`
	Errors            []string     `json:"errors"`
	Packages          []string     `json:"packages"`
	FrameworkPackages []string     `json:"framework_packages"`
	Modules           []string     `json:"modules"`
	Symbols           []symbol     `json:"symbols"`
	DirectImports     []importEdge `json:"direct_imports"`
}
type moduleEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
}
type report struct {
	MethodVersion    int                `json:"method_version"`
	GoVersion        string             `json:"go_version"`
	GOOS             string             `json:"goos"`
	GOARCH           string             `json:"goarch"`
	ModuleGraph      []moduleEdge       `json:"module_graph"`
	ModuleGraphError string             `json:"module_graph_error,omitempty"`
	Scopes           map[string]closure `json:"scopes"`
}

func position(fset *token.FileSet, pos token.Pos, root string) location {
	p := fset.Position(pos)
	file := p.Filename
	if rel, err := filepath.Rel(root, file); err == nil && !strings.HasPrefix(rel, "..") {
		file = filepath.ToSlash(rel)
	}
	return location{File: file, Line: p.Line, Column: p.Column}
}
func kind(file string) string {
	if strings.HasSuffix(file, "_test.go") {
		return "test"
	}
	return "production"
}
func locationKey(p location) string { return fmt.Sprintf("%s:%d:%d", p.File, p.Line, p.Column) }
func callIdent(expr ast.Expr) *ast.Ident {
	switch e := expr.(type) {
	case *ast.Ident:
		return e
	case *ast.SelectorExpr:
		return e.Sel
	case *ast.IndexExpr:
		return callIdent(e.X)
	case *ast.IndexListExpr:
		return callIdent(e.X)
	case *ast.ParenExpr:
		return callIdent(e.X)
	}
	return nil
}
func measure(pattern string, tests bool, tags ...string) closure {
	wd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	cfg := &packages.Config{Fset: token.NewFileSet(), Tests: tests, Mode: packages.NeedName | packages.NeedFiles | packages.NeedImports | packages.NeedDeps | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedSyntax | packages.NeedModule}
	if len(tags) > 0 {
		cfg.BuildFlags = []string{"-tags=" + strings.Join(tags, ",")}
	}
	roots, err := packages.Load(cfg, pattern)
	r := closure{Pattern: pattern, Tests: tests, Tags: tags, Errors: []string{}, Symbols: []symbol{}, DirectImports: []importEdge{}}
	if err != nil {
		r.Errors = append(r.Errors, err.Error())
		return r
	}
	all := map[string]*packages.Package{}
	ps, fs, ms, errs := map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}
	moduleDirs := map[string]string{}
	var visit func(*packages.Package)
	visit = func(p *packages.Package) {
		if p == nil || ps[p.ID] {
			return
		}
		ps[p.ID] = true
		all[p.ID] = p
		if strings.HasPrefix(p.PkgPath, framework+"/") {
			fs[p.PkgPath] = true
		}
		if p.Module != nil {
			ms[p.Module.Path+"@"+p.Module.Version] = true
			moduleDirs[p.PkgPath] = p.Module.Dir
		}
		for _, e := range p.Errors {
			errs[e.Error()] = true
		}
		for _, i := range p.Imports {
			visit(i)
		}
	}
	for _, p := range roots {
		visit(p)
	}
	refs := map[string]*symbol{}
	seenRefs := map[string]map[string]bool{}
	imports := map[string]importEdge{}
	for _, p := range all {
		if p.Module == nil || !p.Module.Main || p.TypesInfo == nil {
			continue
		}
		calls := map[token.Pos]bool{}
		for _, file := range p.Syntax {
			fileName := cfg.Fset.Position(file.Pos()).Filename
			// Synthetic test mains are build machinery, not consumer source imports.
			if !strings.HasPrefix(fileName, wd+string(filepath.Separator)) {
				continue
			}
			for _, imp := range file.Imports {
				path, e := strconv.Unquote(imp.Path.Value)
				if e != nil {
					continue
				}
				at := position(cfg.Fset, imp.Pos(), wd)
				edge := importEdge{From: p.PkgPath, To: path, SourceKind: kind(at.File), location: at}
				imports[p.PkgPath+"|"+path+"|"+locationKey(at)] = edge
			}
			ast.Inspect(file, func(node ast.Node) bool {
				if call, ok := node.(*ast.CallExpr); ok {
					if id := callIdent(call.Fun); id != nil {
						calls[id.Pos()] = true
					}
				}
				return true
			})
		}
		for id, obj := range p.TypesInfo.Uses {
			if obj.Pkg() == nil || !strings.HasPrefix(obj.Pkg().Path(), framework+"/") || !obj.Exported() {
				continue
			}
			at := position(cfg.Fset, id.Pos(), wd)
			if filepath.IsAbs(at.File) {
				continue
			}
			decl := types.ObjectString(obj, func(p *types.Package) string { return p.Path() })
			declared := position(cfg.Fset, obj.Pos(), moduleDirs[obj.Pkg().Path()])
			key := obj.Pkg().Path() + "|" + locationKey(declared) + "|" + decl
			if refs[key] == nil {
				refs[key] = &symbol{Package: obj.Pkg().Path(), Name: obj.Name(), Declaration: decl, DeclaredAt: declared}
				seenRefs[key] = map[string]bool{}
			}
			refKey := locationKey(at)
			if !seenRefs[key][refKey] {
				refs[key].References = append(refs[key].References, reference{location: at, SourceKind: kind(at.File), Call: calls[id.Pos()]})
				seenRefs[key][refKey] = true
			}
		}
	}
	r.Packages = keys(ps)
	r.FrameworkPackages = keys(fs)
	r.Modules = keys(ms)
	r.Errors = keys(errs)
	for _, s := range refs {
		sort.Slice(s.References, func(i, j int) bool {
			return locationKey(s.References[i].location) < locationKey(s.References[j].location)
		})
		r.Symbols = append(r.Symbols, *s)
	}
	sort.Slice(r.Symbols, func(i, j int) bool {
		a, b := r.Symbols[i], r.Symbols[j]
		return a.Package+"|"+locationKey(a.DeclaredAt)+"|"+a.Declaration < b.Package+"|"+locationKey(b.DeclaredAt)+"|"+b.Declaration
	})
	for _, edge := range imports {
		r.DirectImports = append(r.DirectImports, edge)
	}
	sort.Slice(r.DirectImports, func(i, j int) bool {
		a, b := r.DirectImports[i], r.DirectImports[j]
		return a.From+"|"+a.To+"|"+locationKey(a.location) < b.From+"|"+b.To+"|"+locationKey(b.location)
	})
	return r
}
func keys(m map[string]bool) []string {
	r := make([]string, 0, len(m))
	for k := range m {
		r = append(r, k)
	}
	sort.Strings(r)
	return r
}
func main() {
	r := report{MethodVersion: 2, Scopes: map[string]closure{}}
	if b, err := exec.Command("go", "env", "GOVERSION", "GOOS", "GOARCH").Output(); err == nil {
		v := strings.Fields(string(b))
		if len(v) == 3 {
			r.GoVersion = v[0]
			r.GOOS = v[1]
			r.GOARCH = v[2]
		}
	}
	if b, err := exec.Command("go", "mod", "graph").Output(); err != nil {
		r.ModuleGraphError = err.Error()
	} else {
		for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
			v := strings.Fields(line)
			if len(v) == 2 {
				r.ModuleGraph = append(r.ModuleGraph, moduleEdge{From: v[0], To: v[1]})
			}
		}
	}
	r.Scopes["runtime"] = measure("./cmd/semteams", false)
	r.Scopes["all_with_tests"] = measure("./...", true)
	r.Scopes["all_with_integration_tests"] = measure("./...", true, "integration")
	r.Scopes["all_with_parked_tests"] = measure("./...", true, "parked_packs")
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(r); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
