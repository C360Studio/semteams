//go:build ignore

// Measure three exact frozen SemStreams contracts and two component candidates.
// Run from SemTeams: go run scripts/measure-agent-contract-family.go -framework DIR -consumer DIR
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"go/types"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime/debug"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/payloadregistry"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/tools/go/packages"
)

const frameworkModule = "github.com/c360studio/semstreams"

var roots = []string{"agentic", "vocabulary/agentic", "internal/looptoken"}
var candidateRoots = []string{"processor/agentic-model", "processor/agentic-governance"}
var contexts = []struct {
	Name string
	Tags []string
}{{"default", nil}, {"integration", []string{"integration"}}, {"parked_packs", []string{"parked_packs"}}}

type loc struct {
	File   string `json:"file"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
}
type edge struct {
	From     string   `json:"from"`
	To       string   `json:"to"`
	Kind     string   `json:"source_kind"`
	Contexts []string `json:"contexts"`
	loc
}
type test struct {
	Package  string   `json:"package"`
	Name     string   `json:"name"`
	Kind     string   `json:"kind"`
	Contexts []string `json:"contexts"`
	loc
}
type ref struct {
	Contexts []string `json:"contexts"`
	Kind     string   `json:"source_kind"`
	Package  string   `json:"consumer_package"`
	loc
}
type symbol struct {
	Package     string `json:"package"`
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	Declaration string `json:"declaration"`
	DeclaredAt  loc    `json:"declared_at"`
	References  []ref  `json:"references"`
}
type closure struct {
	Roots               []string      `json:"roots"`
	Tests               bool          `json:"tests"`
	Tags                []string      `json:"tags"`
	PackageIDs          []string      `json:"package_ids"`
	FrameworkPackageIDs []string      `json:"framework_package_ids"`
	Modules             []string      `json:"modules"`
	Errors              []string      `json:"errors"`
	RootPackages        []listPackage `json:"root_packages"`
}
type listPackage struct {
	ImportPath     string
	ForTest        string                                              `json:",omitempty"`
	GoFiles        []string                                            `json:",omitempty"`
	TestGoFiles    []string                                            `json:",omitempty"`
	XTestGoFiles   []string                                            `json:",omitempty"`
	IgnoredGoFiles []string                                            `json:",omitempty"`
	Imports        []string                                            `json:",omitempty"`
	TestImports    []string                                            `json:",omitempty"`
	XTestImports   []string                                            `json:",omitempty"`
	Deps           []string                                            `json:",omitempty"`
	Module         *struct{ Path, Version, Dir, Sum, GoModSum string } `json:",omitempty"`
	Error          *struct{ Err string }                               `json:",omitempty"`
	DepsErrors     []struct{ Err string }                              `json:",omitempty"`
}
type typedScope struct {
	Tags           []string `json:"tags"`
	Patterns       []string `json:"patterns"`
	PackageIDs     []string `json:"package_ids"`
	Errors         []string `json:"errors"`
	Symbols        []symbol `json:"symbols,omitempty"`
	SymbolCount    int      `json:"symbol_count"`
	ReferenceCount int      `json:"reference_count"`
}
type repo struct {
	MeasuredFileSHA256 map[string]string     `json:"measured_file_sha256"`
	Symbols            []symbol              `json:"symbols"`
	Module             string                `json:"module"`
	Revision           string                `json:"revision,omitempty"`
	SourceSHA256       string                `json:"go_sources_sha256"`
	SourceFiles        []string              `json:"hashed_source_files"`
	ModuleFiles        map[string]string     `json:"module_file_sha256"`
	DirectEdges        []edge                `json:"direct_imports"`
	TestInventory      []test                `json:"test_inventory"`
	Consumers          map[string]typedScope `json:"direct_consumer_symbols"`
}
type payloadEntry struct {
	ConsumerPackages map[string][]string `json:"direct_consumer_packages"`
	Key              string              `json:"key"`
	ConcreteType     string              `json:"concrete_type"`
	FactoryAt        loc                 `json:"factory_at"`
	IndexingProfile  string              `json:"indexing_profile"`
	Contracts        json.RawMessage     `json:"projection_contracts"`
}
type selectedFramework struct {
	Dir, Version, Sum, GoModSum string
	Replace                     *struct{}
}

type report struct {
	MeasurementContexts map[string]string `json:"measurement_contexts"`

	Limitations             []string       `json:"limitations"`
	Payloads                []payloadEntry `json:"agentic_payloads"`
	Probe                   map[string]any `json:"nominal_type_probe"`
	RetainedRuntimePackages []string       `json:"semteams_runtime_packages"`

	MethodVersion     int                `json:"method_version"`
	TargetPackages    []string           `json:"target_packages"`
	GoEnv             map[string]string  `json:"go_env"`
	ResolvedFramework json.RawMessage    `json:"resolved_framework"`
	Framework         repo               `json:"framework"`
	Consumer          repo               `json:"consumer"`
	Forward           map[string]closure `json:"forward_closures"`
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func command(dir string, args ...string) ([]byte, error) {
	c := exec.Command(args[0], args[1:]...)
	c.Dir = dir
	var stderr bytes.Buffer
	c.Stderr = &stderr
	b, e := c.Output()
	if e != nil {
		return b, fmt.Errorf("%s: %w: %s", strings.Join(args, " "), e, stderr.String())
	}
	return b, nil
}
func sourceKind(p string) string {
	if strings.HasSuffix(p, "_test.go") {
		return "test"
	}
	return "production"
}
func position(fs *token.FileSet, p token.Pos, root string) loc {
	at := fs.Position(p)
	file := at.Filename
	if rel, e := filepath.Rel(root, file); e == nil && !strings.HasPrefix(rel, "..") {
		file = filepath.ToSlash(rel)
	}
	return loc{file, at.Line, at.Column}
}
func locKey(p loc) string { return fmt.Sprintf("%s:%09d:%09d", p.File, p.Line, p.Column) }
func target(p string) bool {
	for _, r := range append(append([]string{}, roots...), candidateRoots...) {
		if p == frameworkModule+"/"+r {
			return true
		}
	}
	return false
}
func rootDir(p string) bool {
	for _, r := range append(append([]string{}, roots...), candidateRoots...) {
		if p == r {
			return true
		}
	}
	return false
}

// scan includes all source contexts, then MatchFile labels build eligibility.
// It does not silently drop ignored files or claim every candidate compiled.
func scan(dir, module string, inventoryRootsOnly bool) (repo, map[string][]string, error) {
	r := repo{Module: module, ModuleFiles: map[string]string{}, MeasuredFileSHA256: map[string]string{}, Consumers: map[string]typedScope{}}
	candidates := map[string]map[string]bool{}
	for _, ctx := range contexts {
		candidates[ctx.Name] = map[string]bool{}
	}
	fs := token.NewFileSet()
	h := sha256.New()
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", "vendor", ".svelte-kit", ".worktrees":
				return filepath.SkipDir
			}
			return nil
		}
		rel, e := filepath.Rel(dir, path)
		if e != nil {
			return e
		}
		rel = filepath.ToSlash(rel)
		if rel == "go.mod" || rel == "go.sum" {
			b, e := os.ReadFile(path)
			if e != nil {
				return e
			}
			sum := sha256.Sum256(b)
			r.ModuleFiles[rel] = hex.EncodeToString(sum[:])
		}
		if !strings.HasSuffix(rel, ".go") {
			return nil
		}
		b, e := os.ReadFile(path)
		if e != nil {
			return e
		}
		fmt.Fprintf(h, "%s\x00", rel)
		h.Write(b)
		h.Write([]byte{0})
		r.SourceFiles = append(r.SourceFiles, rel)
		if rootDir(filepath.ToSlash(filepath.Dir(rel))) || strings.HasPrefix(rel, "internal/deliverylane/") || strings.HasPrefix(rel, "internal/lifecyclecleanup/") {
			sum := sha256.Sum256(b)
			r.MeasuredFileSHA256[rel] = hex.EncodeToString(sum[:])
		}
		f, e := parser.ParseFile(fs, path, b, parser.ParseComments)
		if e != nil {
			return e
		}
		var active []string
		for _, ctx := range contexts {
			bc := build.Default
			bc.BuildTags = ctx.Tags
			match, e := bc.MatchFile(filepath.Dir(path), filepath.Base(path))
			if e != nil {
				return e
			}
			if match {
				active = append(active, ctx.Name)
			}
		}
		pkgRel := filepath.ToSlash(filepath.Dir(rel))
		pkg := module
		if pkgRel != "." {
			pkg += "/" + pkgRel
		}
		for _, imp := range f.Imports {
			to, e := strconv.Unquote(imp.Path.Value)
			if e != nil {
				return e
			}
			if !target(to) && to != frameworkModule+"/internal/deliverylane" && to != frameworkModule+"/internal/lifecyclecleanup" && !(module == frameworkModule && rootDir(pkgRel)) {
				continue
			}
			r.DirectEdges = append(r.DirectEdges, edge{pkg, to, sourceKind(rel), active, position(fs, imp.Pos(), dir)})
			if target(to) {
				for _, name := range active {
					candidates[name]["./"+pkgRel] = true
				}
			}
		}
		if strings.HasSuffix(rel, "_test.go") && inventoryRootsOnly && rootDir(pkgRel) {
			for _, decl := range f.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Recv != nil {
					continue
				}
				kind := ""
				for _, prefix := range []string{"Test", "Fuzz", "Benchmark", "Example"} {
					if strings.HasPrefix(fn.Name.Name, prefix) {
						kind = prefix
						break
					}
				}
				if kind != "" {
					r.TestInventory = append(r.TestInventory, test{pkg, fn.Name.Name, kind, active, position(fs, fn.Pos(), dir)})
				}
			}
		}
		return nil
	})
	if err != nil {
		return r, nil, err
	}
	r.SourceSHA256 = hex.EncodeToString(h.Sum(nil))
	out := map[string][]string{}
	for name, c := range candidates {
		out[name] = keys(c)
	}
	sort.Slice(r.DirectEdges, func(i, j int) bool {
		return locKey(r.DirectEdges[i].loc)+r.DirectEdges[i].To < locKey(r.DirectEdges[j].loc)+r.DirectEdges[j].To
	})
	return r, out, nil
}

func forward(dir string, selectedRoots []string, tests bool, tags []string) closure {
	args := []string{"go", "list", "-mod=readonly", "-e", "-deps", "-json"}
	if tests {
		args = append(args, "-test")
	}
	if len(tags) > 0 {
		args = append(args, "-tags="+strings.Join(tags, ","))
	}
	patterns := []string{}
	for _, r := range selectedRoots {
		patterns = append(patterns, frameworkModule+"/"+r)
	}
	args = append(args, patterns...)
	r := closure{Roots: patterns, Tests: tests, Tags: tags, Errors: []string{}}
	b, err := command(dir, args...)
	if err != nil {
		r.Errors = append(r.Errors, err.Error())
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	all, fw, modules, errs := map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}
	for {
		var p listPackage
		err := dec.Decode(&p)
		if err == io.EOF {
			break
		}
		if err != nil {
			errs[err.Error()] = true
			break
		}
		all[p.ImportPath] = true
		if strings.HasPrefix(p.ImportPath, frameworkModule+"/") {
			fw[p.ImportPath] = true
		}
		if p.Module != nil {
			modules[p.Module.Path+"@"+p.Module.Version] = true
		}
		if p.Error != nil {
			errs[p.Error.Err] = true
		}
		for _, e := range p.DepsErrors {
			errs[e.Err] = true
		}
		isRoot := false
		for _, root := range selectedRoots {
			if p.ImportPath == frameworkModule+"/"+root {
				isRoot = true
			}
		}
		if isRoot {
			p.Module = nil
			r.RootPackages = append(r.RootPackages, p)
		}
	}
	r.PackageIDs = keys(all)
	r.FrameworkPackageIDs = keys(fw)
	r.Modules = keys(modules)
	r.Errors = append(r.Errors, keys(errs)...)
	sort.Slice(r.RootPackages, func(i, j int) bool { return r.RootPackages[i].ImportPath < r.RootPackages[j].ImportPath })
	return r
}

func symbols(dir, declRoot string, patterns, tags []string) typedScope {
	r := typedScope{Tags: tags, Patterns: patterns, Errors: []string{}, Symbols: []symbol{}}
	fs := token.NewFileSet()
	cfg := &packages.Config{Dir: dir, Fset: fs, Tests: true, Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles | packages.NeedImports | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedSyntax | packages.NeedModule, BuildFlags: []string{"-mod=readonly"}}
	if len(tags) > 0 {
		cfg.BuildFlags = append(cfg.BuildFlags, "-tags="+strings.Join(tags, ","))
	}
	ps, err := packages.Load(cfg, patterns...)
	if err != nil {
		r.Errors = append(r.Errors, err.Error())
	}
	found := map[string]*symbol{}
	seen := map[string]bool{}
	errs := map[string]bool{}
	ids := map[string]bool{}
	for _, p := range ps {
		ids[p.ID] = true
		for _, e := range p.Errors {
			errs[e.Error()] = true
		}
		if p.TypesInfo == nil {
			continue
		}
		for id, obj := range p.TypesInfo.Uses {
			if obj.Pkg() == nil || !target(obj.Pkg().Path()) || !obj.Exported() || p.PkgPath == obj.Pkg().Path() {
				continue
			}
			kind := ""
			switch object := obj.(type) {
			case *types.TypeName:
				kind = "type"
			case *types.Func:
				kind = "function_or_method"
			case *types.Const:
				kind = "constant"
			case *types.Var:
				if object.IsField() || object.Parent() != object.Pkg().Scope() {
					continue
				}
				kind = "variable"
			default:
				continue
			}
			at := position(fs, id.Pos(), dir)
			if filepath.IsAbs(at.File) {
				continue
			}
			declared := position(fs, obj.Pos(), declRoot)
			declaration := types.ObjectString(obj, func(p *types.Package) string { return p.Path() })
			key := obj.Pkg().Path() + "|" + locKey(declared) + "|" + declaration
			if found[key] == nil {
				found[key] = &symbol{Package: obj.Pkg().Path(), Name: obj.Name(), Kind: kind, Declaration: declaration, DeclaredAt: declared}
			}
			refKey := key + "|" + locKey(at)
			if !seen[refKey] {
				seen[refKey] = true
				found[key].References = append(found[key].References, ref{nil, sourceKind(at.File), p.PkgPath, at})
			}
		}
	}
	for _, s := range found {
		sort.Slice(s.References, func(i, j int) bool { return locKey(s.References[i].loc) < locKey(s.References[j].loc) })
		r.Symbols = append(r.Symbols, *s)
	}
	sort.Slice(r.Symbols, func(i, j int) bool {
		return r.Symbols[i].Package+locKey(r.Symbols[i].DeclaredAt) < r.Symbols[j].Package+locKey(r.Symbols[j].DeclaredAt)
	})
	r.Errors = append(r.Errors, keys(errs)...)
	r.PackageIDs = keys(ids)
	return r
}

func main() {
	fw := flag.String("framework", "", "exact frozen SemStreams module directory")
	consumer := flag.String("consumer", ".", "SemTeams source directory")
	flag.Parse()
	if *fw == "" {
		must(fmt.Errorf("-framework is required"))
	}
	var err error
	*fw, err = filepath.Abs(*fw)
	must(err)
	*consumer, err = filepath.Abs(*consumer)
	must(err)
	r := report{MeasurementContexts: map[string]string{"forward_closures": "SemTeams consumer directory and its selected dependency graph; exact fully-qualified package roots", "framework_symbols": "Frozen SemStreams module directory and its module dependency graph", "consumer_symbols": "SemTeams consumer directory and its selected dependency graph", "source_scan": "All Go source files in each input, including build-ignored files; contexts report build eligibility"}, MethodVersion: 1, Forward: map[string]closure{}, GoEnv: map[string]string{}}
	for _, root := range roots {
		r.TargetPackages = append(r.TargetPackages, frameworkModule+"/"+root)
	}
	env, err := command(*consumer, "go", "env", "-json", "GOVERSION", "GOOS", "GOARCH", "CGO_ENABLED", "GOFLAGS")
	must(err)
	must(json.Unmarshal(env, &r.GoEnv))
	r.ResolvedFramework, err = command(*consumer, "go", "list", "-mod=readonly", "-m", "-json", frameworkModule)
	must(err)
	var selected selectedFramework
	must(json.Unmarshal(r.ResolvedFramework, &selected))
	buildInfo, ok := debug.ReadBuildInfo()
	if !ok {
		must(fmt.Errorf("compiled probe build information unavailable"))
	}
	must(verifyProbeModule(selected, buildInfo))
	if filepath.Clean(selected.Dir) != filepath.Clean(*fw) {
		must(fmt.Errorf("framework input %s does not equal consumer-resolved module %s", *fw, selected.Dir))
	}
	r.Limitations = []string{
		"Source scans include ignored and non-admitted files; contexts label Go build eligibility, not execution.",
		"Package-ID closure includes standard library and synthetic test variants; it is not linker reachability or a feature-admission census.",
		"Compiler symbol reports cover packages with direct source imports of the three contracts or two candidate packages; inferred consumers without a direct import may be outside this census.",
		"The full frozen module reverse-import scan includes tools and components not admitted to SemTeams; semteams_runtime_packages separately identifies retained compiled runtime packages.",
		"Type resolution errors are retained as partial diagnostic evidence; zero script exit does not assert every context compiled.",
		"Test inventory records declared top-level test/fuzz/benchmark/example functions in exact target/candidate directories, not executed tests or subtests.",
		"Platform and build-tag coverage is only the recorded Go environment and named contexts; no live models, NATS, or external effects are exercised.",
	}
	r.Payloads = payloadTable(*fw)
	r.Probe = nominalProbe()
	runtime, err := command(*consumer, "go", "list", "-mod=readonly", "-deps", "-f", "{{.ImportPath}}", "./cmd/semteams")
	must(err)
	r.RetainedRuntimePackages = strings.Fields(string(runtime))
	sort.Strings(r.RetainedRuntimePackages)
	var fwCandidates, consumerCandidates map[string][]string
	r.Framework, fwCandidates, err = scan(*fw, frameworkModule, true)
	must(err)
	r.Framework.Revision = selected.Version
	r.Consumer, consumerCandidates, err = scan(*consumer, "github.com/c360studio/semteams", false)
	must(err)
	rev, err := command(*consumer, "git", "rev-parse", "HEAD")
	must(err)
	r.Consumer.Revision = strings.TrimSpace(string(rev))
	r.Forward["production"] = forward(*consumer, roots, false, nil)
	for _, candidate := range candidateRoots {
		r.Forward[candidate+"/production"] = forward(*consumer, []string{candidate}, false, nil)
		r.Forward[candidate+"/default_tests"] = forward(*consumer, []string{candidate}, true, nil)
		r.Forward[candidate+"/integration_tests"] = forward(*consumer, []string{candidate}, true, []string{"integration"})
	}
	for _, ctx := range contexts {
		r.Forward[ctx.Name+"_tests"] = forward(*consumer, roots, true, ctx.Tags)
		fmt.Fprintln(os.Stderr, "resolving", ctx.Name, "framework consumers", len(fwCandidates[ctx.Name]))
		r.Framework.Consumers[ctx.Name] = symbols(*fw, *fw, fwCandidates[ctx.Name], ctx.Tags)
		fmt.Fprintln(os.Stderr, "resolving", ctx.Name, "SemTeams consumers", len(consumerCandidates[ctx.Name]))
		r.Consumer.Consumers[ctx.Name] = symbols(*consumer, *fw, consumerCandidates[ctx.Name], ctx.Tags)
	}
	consolidate(&r.Framework)
	consolidate(&r.Consumer)
	linkPayloadConsumers(&r)
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	must(enc.Encode(r))
}

func payloadTable(dir string) []payloadEntry {
	registry := payloadregistry.New()
	must(agentic.RegisterPayloads(registry))
	fs := token.NewFileSet()
	f, err := parser.ParseFile(fs, filepath.Join(dir, "agentic/payload_registry.go"), nil, 0)
	must(err)
	factories := map[string]loc{}
	ast.Inspect(f, func(n ast.Node) bool {
		kv, ok := n.(*ast.KeyValueExpr)
		if !ok {
			return true
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok || key.Name != "Factory" {
			return true
		}
		ast.Inspect(kv.Value, func(v ast.Node) bool {
			literal, ok := v.(*ast.CompositeLit)
			if !ok {
				return true
			}
			name, ok := literal.Type.(*ast.Ident)
			if ok {
				factories[name.Name] = position(fs, kv.Pos(), dir)
			}
			return true
		})
		return true
	})
	entries := []payloadEntry{}
	for key, r := range registry.List() {
		instance := registry.Create(r.Domain, r.Category, r.Version)
		typ := reflect.TypeOf(instance).Elem()
		at, ok := factories[typ.Name()]
		if !ok {
			must(fmt.Errorf("factory source position missing for %s", typ.Name()))
		}
		contracts, err := json.Marshal(r.Contracts)
		must(err)
		entries = append(entries, payloadEntry{nil, key, typ.PkgPath() + "." + typ.Name(), at, r.IndexingProfile, contracts})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Key < entries[j].Key })
	return entries
}

// relocatedRequest is a deliberately distinct analysis-only named type with
// the same underlying fields and wire encoding. It is not a runtime adapter.
type relocatedRequest agentic.AgentRequest

func (r *relocatedRequest) Schema() message.Type { return (*agentic.AgentRequest)(r).Schema() }
func (r *relocatedRequest) Validate() error      { return (*agentic.AgentRequest)(r).Validate() }
func (r *relocatedRequest) MarshalJSON() ([]byte, error) {
	return (*agentic.AgentRequest)(r).MarshalJSON()
}
func (r *relocatedRequest) UnmarshalJSON(b []byte) error {
	return (*agentic.AgentRequest)(r).UnmarshalJSON(b)
}

type frozenRequestAlias = agentic.AgentRequest

func nominalProbe() map[string]any {
	original := &agentic.AgentRequest{RequestID: "probe", Role: "researcher", Model: "offline", Messages: []agentic.ChatMessage{{Role: "user", Content: "probe"}}}
	must(original.Validate())
	wire, err := json.Marshal(message.NewBaseMessage(original.Schema(), original, "offline-probe"))
	must(err)
	reg := payloadregistry.New()
	mt := original.Schema()
	registration := &payloadregistry.Registration{Domain: mt.Domain, Category: mt.Category, Version: mt.Version, Factory: func() any { return &relocatedRequest{} }}
	must(reg.Register(registration))
	decoded, err := message.NewDecoder(reg).Decode(wire)
	must(err)
	_, accepted := decoded.Payload().(*agentic.AgentRequest)
	before, err := original.MarshalJSON()
	must(err)
	after, err := decoded.Payload().MarshalJSON()
	must(err)
	baseline := payloadregistry.New()
	must(agentic.RegisterPayloads(baseline))
	collision := baseline.Register(registration)
	sameTextSentinel := errors.New(agentic.ErrToolNotFound.Error())
	result := map[string]any{"same_text_error_preserves_identity": errors.Is(sameTextSentinel, agentic.ErrToolNotFound), "shared_error_preserves_identity": errors.Is(agentic.ErrToolNotFound, agentic.ErrToolNotFound), "same_wire_json": bytes.Equal(before, after), "relocated_passes_frozen_type_assertion": accepted, "relocated_assignable_to_frozen": reflect.TypeOf(&relocatedRequest{}).AssignableTo(reflect.TypeOf(original)), "alias_retains_frozen_identity": reflect.TypeOf(&frozenRequestAlias{}) == reflect.TypeOf(original), "alias_definition_package": reflect.TypeOf(frozenRequestAlias{}).PkgPath(), "duplicate_registration_rejected": collision != nil}
	if collision != nil {
		result["duplicate_registration_error"] = collision.Error()
	}
	if accepted || !bytes.Equal(before, after) || collision == nil {
		must(fmt.Errorf("nominal probe did not reproduce expected identity/collision boundary"))
	}
	return result
}

// Consolidate identical source references across build contexts without losing
// which contexts resolved them or the per-context error/coverage counts.
func consolidate(r *repo) {
	found := map[string]*symbol{}
	references := map[string]map[string]int{}
	names := []string{}
	for name := range r.Consumers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		scope := r.Consumers[name]
		scope.SymbolCount = len(scope.Symbols)
		for _, s := range scope.Symbols {
			key := s.Package + "|" + locKey(s.DeclaredAt) + "|" + s.Declaration
			dest := found[key]
			if dest == nil {
				copy := s
				copy.References = nil
				dest = &copy
				found[key] = dest
				references[key] = map[string]int{}
			}
			for _, reference := range s.References {
				scope.ReferenceCount++
				rk := locKey(reference.loc)
				i, seen := references[key][rk]
				if !seen {
					i = len(dest.References)
					references[key][rk] = i
					reference.Contexts = nil
					dest.References = append(dest.References, reference)
				}
				dest.References[i].Contexts = append(dest.References[i].Contexts, name)
			}
		}
		scope.Symbols = nil
		r.Consumers[name] = scope
	}
	for _, s := range found {
		sort.Slice(s.References, func(i, j int) bool { return locKey(s.References[i].loc) < locKey(s.References[j].loc) })
		r.Symbols = append(r.Symbols, *s)
	}
	sort.Slice(r.Symbols, func(i, j int) bool {
		return r.Symbols[i].Package+locKey(r.Symbols[i].DeclaredAt) < r.Symbols[j].Package+locKey(r.Symbols[j].DeclaredAt)
	})
}

func linkPayloadConsumers(r *report) {
	for i := range r.Payloads {
		entry := &r.Payloads[i]
		entry.ConsumerPackages = map[string][]string{}
		for name, repository := range map[string]repo{"framework": r.Framework, "consumer": r.Consumer} {
			consumers := map[string]bool{}
			for _, s := range repository.Symbols {
				if s.Kind != "type" || s.Package+"."+s.Name != entry.ConcreteType {
					continue
				}
				for _, reference := range s.References {
					consumers[reference.Package] = true
				}
			}
			entry.ConsumerPackages[name] = keys(consumers)
		}
	}
}

// The source/consumer directories are runtime inputs, but payload factories and
// the nominal probe are compiled into this analysis executable. Bind both
// authorities before reporting; a matching source path alone is insufficient.
func verifyProbeModule(selected selectedFramework, build *debug.BuildInfo) error {
	if selected.Replace != nil {
		return fmt.Errorf("consumer-selected SemStreams replacement is not supported by frozen measurement")
	}
	if build == nil {
		return fmt.Errorf("compiled probe build information unavailable")
	}
	for _, module := range build.Deps {
		if module.Path != frameworkModule {
			continue
		}
		if module.Replace != nil {
			return fmt.Errorf("compiled SemStreams replacement is not supported by frozen measurement")
		}
		if selected.Version == "" || selected.Sum == "" || module.Version != selected.Version || module.Sum != selected.Sum {
			return fmt.Errorf("compiled SemStreams %s %s does not match consumer-selected %s %s", module.Version, module.Sum, selected.Version, selected.Sum)
		}
		return nil
	}
	return fmt.Errorf("compiled probe has no SemStreams module identity")
}
