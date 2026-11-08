package runtimecatalog_test

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/config"
	graphgateway "github.com/c360studio/semstreams/gateway/graph-gateway"
	"github.com/c360studio/semstreams/graph/inference"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/payloadregistry"
	"github.com/c360studio/semstreams/pkg/lifecycle"
	agenticdispatch "github.com/c360studio/semstreams/processor/agentic-dispatch"
	agenticloop "github.com/c360studio/semstreams/processor/agentic-loop"
	agenticmodel "github.com/c360studio/semstreams/processor/agentic-model"
	agentictools "github.com/c360studio/semstreams/processor/agentic-tools"
	graphingest "github.com/c360studio/semstreams/processor/graph-ingest"
	graphquery "github.com/c360studio/semstreams/processor/graph-query"
	"github.com/c360studio/semstreams/processor/rule"
	"github.com/c360studio/semstreams/storage/objectstore"
	"github.com/c360studio/semteams/internal/runtimecatalog"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

var expectedFactoryNames = []string{
	"agentic-dispatch",
	"agentic-loop",
	"agentic-model",
	"agentic-tools",
	"graph-gateway",
	"graph-ingest",
	"graph-query",
	"objectstore",
	"rule-processor",
}

func registerReferenceComponents(t *testing.T) *component.Registry {
	t.Helper()
	reg := component.NewRegistry()
	registrars := []func(*component.Registry) error{
		graphingest.Register,
		graphquery.Register,
		rule.Register,
		objectstore.Register,
		agenticloop.Register,
		func(r *component.Registry) error { return agenticmodel.Register(r) },
		func(r *component.Registry) error { return agentictools.Register(r) },
		agenticdispatch.Register,
		graphgateway.Register,
	}
	for _, fn := range registrars {
		if err := fn(reg); err != nil {
			t.Fatalf("reference component registration failed: %v", err)
		}
	}
	return reg
}

func registerReferencePayloads(t *testing.T) *payloadregistry.Registry {
	t.Helper()
	reg := payloadregistry.New()
	registrars := []func(*payloadregistry.Registry) error{
		message.RegisterPayloads,
		agentic.RegisterPayloads,
		objectstore.RegisterPayloads,
		lifecycle.RegisterPayloads,
		inference.RegisterPayloads,
	}
	for _, fn := range registrars {
		if err := fn(reg); err != nil {
			t.Fatalf("reference payload registration failed: %v", err)
		}
	}
	return reg
}

// Package-owned Register functions are the independent test oracle, avoiding
// the dropped SemStreams aggregator packages.
//
// spec: openspec/specs/runtime-composition/spec.md#Requirement: Product-owned registration boundary
func TestComponentsMatchComposedCatalog(t *testing.T) {
	want := registerReferenceComponents(t)
	got := component.NewRegistry()
	if err := runtimecatalog.RegisterComponents(got); err != nil {
		t.Fatal(err)
	}
	expected, actual := want.ListFactories(), got.ListFactories()
	if len(expected) != len(expectedFactoryNames) {
		t.Fatalf("reference catalog count = %d, want %d", len(expected), len(expectedFactoryNames))
	}
	gotNames := make([]string, 0, len(actual))
	for name := range actual {
		gotNames = append(gotNames, name)
	}
	sort.Strings(gotNames)
	if diff := cmp.Diff(expectedFactoryNames, gotNames); diff != "" {
		t.Fatalf("registered factory set mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(expected, actual, cmpopts.IgnoreFields(component.Registration{}, "Factory", "Ports"), cmp.AllowUnexported(component.PortFieldInfo{})); diff != "" {
		t.Fatalf("component metadata/schema/dependency drift (-reference +product):\n%s", diff)
	}
	for name, registration := range expected {
		t.Run(name, func(t *testing.T) {
			referenceFactory, _ := want.GetFactory(name)
			productFactory, _ := got.GetFactory(name)
			assertSameFunction(t, "factory", referenceFactory, productFactory)
			assertSameFunction(t, "port declaration", registration.Ports, actual[name].Ports)
		})
	}
}

// spec: openspec/specs/runtime-composition/spec.md#Requirement: Product-owned registration boundary
func TestComponentsMatchShippedBootstraps(t *testing.T) {
	registry := component.NewRegistry()
	if err := runtimecatalog.RegisterComponents(registry); err != nil {
		t.Fatal(err)
	}
	registered := registry.ListFactories()

	paths := []string{"../../configs/flow-bootstrap.json", "../../configs/e2e-flow-bootstrap.json"}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			assertBootstrapFactoryParity(t, path, registered)
		})
	}
}

func assertBootstrapFactoryParity(t *testing.T, path string, registered map[string]*component.Registration) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.NewLoader().LoadFromBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	composed := make(map[string]bool)
	for instance, cc := range cfg.Components {
		if cc.Name == "" {
			t.Fatalf("component %q in %s has empty factory name", instance, path)
		}
		if _, ok := registered[cc.Name]; !ok {
			t.Errorf("component %q in %s references unregistered factory %q", instance, path, cc.Name)
		}
		if cc.Enabled {
			composed[cc.Name] = true
		}
	}
	for factory := range registered {
		if !composed[factory] {
			t.Errorf("registered factory %q is not composed by any enabled component in %s", factory, path)
		}
	}
}

// spec: openspec/specs/runtime-composition/spec.md#Requirement: Product-owned registration boundary
func TestPayloadsMatchRetainedOwners(t *testing.T) {
	want := registerReferencePayloads(t)
	got := payloadregistry.New()
	if err := runtimecatalog.RegisterPayloads(got); err != nil {
		t.Fatal(err)
	}
	expected, actual := want.List(), got.List()
	if len(expected) != 24 {
		t.Fatalf("reference payload catalog count = %d, want 24", len(expected))
	}
	if diff := cmp.Diff(expected, actual); diff != "" {
		t.Fatalf("payload metadata/profile/projection-contract drift (-reference +product):\n%s", diff)
	}
	assertDroppedPayloadsAbsent(t, got, actual)
	for name, registration := range expected {
		t.Run(name, func(t *testing.T) {
			reference := want.Create(registration.Domain, registration.Category, registration.Version)
			product := got.Create(registration.Domain, registration.Category, registration.Version)
			if reference == nil || reflect.TypeOf(reference) != reflect.TypeOf(product) {
				t.Fatalf("payload factory type drift for %s", name)
			}
		})
	}
}

func assertDroppedPayloadsAbsent(t *testing.T, got *payloadregistry.Registry, actual map[string]*payloadregistry.Registration) {
	t.Helper()
	droppedDomains := map[string]bool{"gateddag": true, "gated-dag": true, "governance": true}
	for key, reg := range actual {
		if droppedDomains[reg.Domain] {
			t.Errorf("uncomposed payload domain %q still registered under key %q", reg.Domain, key)
		}
	}
	for _, tc := range []struct{ domain, category, version string }{
		{"gateddag", "dispatch", "v1"},
		{"gateddag", "stall", "v1"},
		{"governance", "verdict", "v1"},
	} {
		if v := got.Create(tc.domain, tc.category, tc.version); v != nil {
			t.Errorf("expected dropped payload %s.%s.%s to be unregistered, got %T", tc.domain, tc.category, tc.version, v)
		}
	}
}

func assertSameFunction(t *testing.T, name string, want, got any) {
	t.Helper()
	if reflect.ValueOf(want).Pointer() != reflect.ValueOf(got).Pointer() {
		t.Errorf("%s implementation differs from package-owned registration", name)
	}
}

// spec: openspec/specs/runtime-composition/spec.md#Requirement: Product-owned registration boundary
func TestComponentCollisionNamesFactory(t *testing.T) {
	baseline := registerReferenceComponents(t)
	for name, registration := range baseline.ListFactories() {
		t.Run(name, func(t *testing.T) {
			registry := component.NewRegistry()
			registration.Factory, _ = baseline.GetFactory(name)
			if err := registry.RegisterFactory(name, registration); err != nil {
				t.Fatal(err)
			}
			err := runtimecatalog.RegisterComponents(registry)
			wantPrefix := fmt.Sprintf("register component %q: ", name)
			if err == nil || !strings.HasPrefix(err.Error(), wantPrefix) || errors.Unwrap(err) == nil {
				t.Fatalf("want wrapped collision starting with %q and non-nil Unwrap, got %v", wantPrefix, err)
			}
		})
	}
}

// spec: openspec/specs/runtime-composition/spec.md#Requirement: Product-owned registration boundary
func TestPayloadCollisionsNameEveryOwner(t *testing.T) {
	registry := payloadregistry.New()
	if err := runtimecatalog.RegisterPayloads(registry); err != nil {
		t.Fatal(err)
	}
	err := runtimecatalog.RegisterPayloads(registry)
	if err == nil {
		t.Fatal("duplicate registration succeeded")
	}
	joined, ok := err.(interface{ Unwrap() []error })
	if !ok {
		t.Fatalf("want joined error implementing Unwrap() []error, got %T (%v)", err, err)
	}
	wantOwners := []string{"message", "agentic", "storage/objectstore", "pkg/lifecycle", "graph/inference"}
	branches := joined.Unwrap()
	if len(branches) != len(wantOwners) {
		t.Fatalf("got %d joined owner errors, want %d: %v", len(branches), len(wantOwners), err)
	}
	for i, owner := range wantOwners {
		wantPrefix := fmt.Sprintf("register payload owner %q: ", owner)
		if !strings.HasPrefix(branches[i].Error(), wantPrefix) {
			t.Errorf("branch[%d] = %q, want prefix %q", i, branches[i].Error(), wantPrefix)
		}
		if errors.Unwrap(branches[i]) == nil {
			t.Errorf("branch[%d] (%s) did not wrap underlying error via %%w", i, owner)
		}
	}
}

// spec: openspec/specs/runtime-composition/spec.md#Requirement: Product-owned registration boundary
func TestNilRegistriesReturnErrors(t *testing.T) {
	if err := runtimecatalog.RegisterComponents(nil); err == nil {
		t.Error("nil component registry accepted")
	}
	if err := runtimecatalog.RegisterPayloads(nil); err == nil {
		t.Error("nil payload registry accepted")
	}
}
