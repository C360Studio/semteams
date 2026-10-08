package runtimecatalog_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/componentregistry"
	"github.com/c360studio/semstreams/payloadbuiltins"
	"github.com/c360studio/semstreams/payloadregistry"
	"github.com/c360studio/semteams/internal/runtimecatalog"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

// The aggregators are an independent test oracle from the frozen dependency,
// never the product's runtime selection. Committed schema contract tests also
// preserve the advertised artifact set independently of this comparison.
func TestComponentsMatchFrozenCatalog(t *testing.T) {
	want := component.NewRegistry()
	if err := componentregistry.Register(want); err != nil {
		t.Fatal(err)
	}
	got := component.NewRegistry()
	if err := runtimecatalog.RegisterComponents(got); err != nil {
		t.Fatal(err)
	}
	expected, actual := want.ListFactories(), got.ListFactories()
	if len(expected) != 27 {
		t.Fatalf("frozen component catalog changed: got %d factories, want 27", len(expected))
	}
	if diff := cmp.Diff(expected, actual, cmpopts.IgnoreFields(component.Registration{}, "Factory", "Ports"), cmp.AllowUnexported(component.PortFieldInfo{})); diff != "" {
		t.Fatalf("component metadata/schema/dependency drift (-frozen +product):\n%s", diff)
	}
	for name, registration := range expected {
		t.Run(name, func(t *testing.T) {
			referenceFactory, _ := want.GetFactory(name)
			productFactory, _ := got.GetFactory(name)
			assertSameFunction(t, "factory", referenceFactory, productFactory)
			// The exact pure declaration function is retained; shipped composition
			// checks separately exercise it against both real bootstrap configs.
			assertSameFunction(t, "port declaration", registration.Ports, actual[name].Ports)
		})
	}
}

func TestPayloadsMatchFrozenCatalog(t *testing.T) {
	want := payloadregistry.New()
	if err := payloadbuiltins.Register(want); err != nil {
		t.Fatal(err)
	}
	got := payloadregistry.New()
	if err := runtimecatalog.RegisterPayloads(got); err != nil {
		t.Fatal(err)
	}
	expected, actual := want.List(), got.List()
	if len(expected) != 27 {
		t.Fatalf("frozen payload catalog changed: got %d types, want 27", len(expected))
	}
	if diff := cmp.Diff(expected, actual); diff != "" {
		t.Fatalf("payload metadata/profile/projection-contract drift (-frozen +product):\n%s", diff)
	}
	for name, registration := range expected {
		t.Run(name, func(t *testing.T) {
			// Payload registry metadata intentionally hides factory functions;
			// exercise Create to compare the concrete decoding types instead.
			reference := want.Create(registration.Domain, registration.Category, registration.Version)
			product := got.Create(registration.Domain, registration.Category, registration.Version)
			if reference == nil || reflect.TypeOf(reference) != reflect.TypeOf(product) {
				t.Fatalf("payload factory type drift for %s", name)
			}
		})
	}
}

func assertSameFunction(t *testing.T, name string, want, got any) {
	t.Helper()
	if reflect.ValueOf(want).Pointer() != reflect.ValueOf(got).Pointer() {
		t.Errorf("%s implementation differs from frozen catalog", name)
	}
}

func TestComponentCollisionNamesFactory(t *testing.T) {
	baseline := component.NewRegistry()
	if err := componentregistry.Register(baseline); err != nil {
		t.Fatal(err)
	}
	for name, registration := range baseline.ListFactories() {
		t.Run(name, func(t *testing.T) {
			registry := component.NewRegistry()
			registration.Factory, _ = baseline.GetFactory(name)
			if err := registry.RegisterFactory(name, registration); err != nil {
				t.Fatal(err)
			}
			err := runtimecatalog.RegisterComponents(registry)
			if err == nil || !strings.Contains(err.Error(), name) || errors.Unwrap(err) == nil {
				t.Fatalf("want wrapped collision identifying %q, got %v", name, err)
			}
		})
	}
}

func TestPayloadCollisionsNameEveryOwnerAndType(t *testing.T) {
	registry := payloadregistry.New()
	if err := runtimecatalog.RegisterPayloads(registry); err != nil {
		t.Fatal(err)
	}
	err := runtimecatalog.RegisterPayloads(registry)
	if err == nil {
		t.Fatal("duplicate registration succeeded")
	}
	for _, owner := range []string{"message", "agentic", "processor/gated-dag", "storage/objectstore", "governance", "pkg/lifecycle", "graph/inference"} {
		if !strings.Contains(err.Error(), "payload owner \""+owner+"\"") {
			t.Errorf("collision does not identify owner %q: %v", owner, err)
		}
	}
	for key := range registry.List() {
		if !strings.Contains(err.Error(), key) {
			t.Errorf("collision does not identify payload %q", key)
		}
	}
}

func TestNilRegistriesReturnErrors(t *testing.T) {
	if err := runtimecatalog.RegisterComponents(nil); err == nil {
		t.Error("nil component registry accepted")
	}
	if err := runtimecatalog.RegisterPayloads(nil); err == nil {
		t.Error("nil payload registry accepted")
	}
}
