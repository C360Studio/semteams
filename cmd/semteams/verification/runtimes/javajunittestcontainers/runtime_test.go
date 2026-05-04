package javajunittestcontainers

import (
	"testing"

	"github.com/c360studio/semteams/cmd/semteams/verification/runtimes"
)

func TestRuntime_InterfaceMetadata(t *testing.T) {
	t.Parallel()
	rt := New()
	if rt.Name() != "java-junit-testcontainers" {
		t.Errorf("Name: got %q", rt.Name())
	}
	if rt.Description() == "" {
		t.Errorf("Description must not be empty")
	}
	if rt.InvokeCommand() == "" {
		t.Errorf("InvokeCommand must not be empty")
	}
}

func TestRuntime_NoTemplatesWiredInD(t *testing.T) {
	t.Parallel()
	// R3.7.2.d posture: no templates. R3.7.2.f wires tcp.binary-
	// protobuf.v1. This test pins the posture so the .f slice has
	// a clear before/after.
	rt := New()
	if got := rt.SupportedFamilies(); len(got) != 0 {
		t.Errorf("SupportedFamilies: got %v, want empty (templates land in R3.7.2.f)", got)
	}
	_, ok := rt.TemplateFor("tcp.binary-protobuf.v1")
	if ok {
		t.Errorf("TemplateFor: returned ok=true for unwired family — templates should land in R3.7.2.f")
	}
}

func TestRuntime_TemplateFor_UnknownFamily(t *testing.T) {
	t.Parallel()
	rt := New()
	got, ok := rt.TemplateFor("ghost.v1")
	if ok {
		t.Errorf("expected ok=false for unknown family")
	}
	if got != "" {
		t.Errorf("expected empty string for unknown family, got %q", got)
	}
}

func TestRegister(t *testing.T) {
	t.Parallel()
	reg := runtimes.NewRegistry()
	if err := Register(reg); err != nil {
		t.Fatalf("Register: %v", err)
	}
	got, err := reg.Get("java-junit-testcontainers")
	if err != nil {
		t.Fatalf("Get after Register: %v", err)
	}
	if got.Name() != "java-junit-testcontainers" {
		t.Errorf("registered runtime Name: got %q", got.Name())
	}
	if err := Register(reg); err == nil {
		t.Errorf("expected duplicate-registration error on second Register")
	}
}
