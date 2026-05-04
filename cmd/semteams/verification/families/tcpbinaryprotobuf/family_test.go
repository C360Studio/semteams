package tcpbinaryprotobuf

import (
	"strings"
	"testing"

	"github.com/c360studio/semteams/cmd/semteams/harness"
	"github.com/c360studio/semteams/cmd/semteams/verification/families"
)

func TestFamily_InterfaceMetadata(t *testing.T) {
	t.Parallel()
	f := New()
	if f.Name() != "tcp.binary-protobuf" {
		t.Errorf("Name: got %q", f.Name())
	}
	if f.Version() != "v1" {
		t.Errorf("Version: got %q", f.Version())
	}
	if f.Description() == "" {
		t.Errorf("Description must not be empty")
	}
	if got, want := families.FullName(f), "tcp.binary-protobuf.v1"; got != want {
		t.Errorf("FullName: got %q, want %q", got, want)
	}
}

func TestFamily_ValidateContract_NilHarnessRejected(t *testing.T) {
	t.Parallel()
	f := New()
	err := f.ValidateContract(nil, nil)
	if err == nil {
		t.Fatal("expected error for nil harness")
	}
	if !strings.Contains(err.Error(), "harness must not be nil") {
		t.Errorf("expected 'harness must not be nil' in error, got %v", err)
	}
}

func TestFamily_ValidateContract_StubAcceptsAnyContract(t *testing.T) {
	t.Parallel()
	// R3.7.2.d posture: validator is a stub. Any non-nil harness
	// returns nil regardless of contract shape. Concrete validation
	// lands in R3.7.2.f.
	f := New()
	h := &harness.Harness{Name: "test", ComposeProfile: "p", Image: "i", SmokeContractSchema: "s.v1", DomainDescription: "d"}
	cases := []any{
		nil,
		"some prose contract",
		map[string]any{"scenarios": []any{}},
		struct{}{},
	}
	for _, c := range cases {
		if err := f.ValidateContract(c, h); err != nil {
			t.Errorf("expected nil for contract %#v, got %v", c, err)
		}
	}
}

func TestRegister(t *testing.T) {
	t.Parallel()
	reg := families.NewRegistry()
	if err := Register(reg); err != nil {
		t.Fatalf("Register: %v", err)
	}
	got, err := reg.Get("tcp.binary-protobuf.v1")
	if err != nil {
		t.Fatalf("Get after Register: %v", err)
	}
	if got.Name() != "tcp.binary-protobuf" {
		t.Errorf("registered family Name: got %q", got.Name())
	}
	// Second register should error (duplicate).
	if err := Register(reg); err == nil {
		t.Errorf("expected duplicate-registration error on second Register")
	}
}
