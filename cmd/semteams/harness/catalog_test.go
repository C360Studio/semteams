package harness

import (
	"strings"
	"testing"
)

func TestHarnessValidate(t *testing.T) {
	tests := []struct {
		name    string
		h       Harness
		wantErr string // substring; "" = expect nil
	}{
		{
			name: "complete valid harness",
			h: Harness{
				Name:                "meshtasticd-3.x",
				ComposeProfile:      "harness-meshtasticd",
				Image:               "meshtastic/meshtasticd:3.5.0",
				SmokeContractSchema: "meshtastic.smoke_contract.v1",
				DomainDescription:   "Real Meshtastic protocol over TCP.",
				Exposes: Exposes{
					TCP: []PortExpose{{Port: 4403, Protocol: "meshtastic-protobuf"}},
				},
				RealDependencies: []Dependency{
					{GroupID: "com.geeksville.mesh", ArtifactID: "meshtastic-protobufs", VersionRange: "[2.x,3.x)"},
				},
			},
		},
		{
			name: "no exposes is valid (UDP / unix sockets land later)",
			h: Harness{
				Name:                "stub",
				ComposeProfile:      "harness-stub",
				Image:               "scratch",
				SmokeContractSchema: "stub.smoke_contract.v1",
				DomainDescription:   "stub for testing",
			},
		},
		{
			name: "empty exposes struct (Exposes{TCP:nil}) is valid",
			h: Harness{
				Name:                "stub",
				ComposeProfile:      "harness-stub",
				Image:               "scratch",
				SmokeContractSchema: "stub.smoke_contract.v1",
				DomainDescription:   "stub for testing",
				Exposes:             Exposes{TCP: nil},
			},
		},
		{
			name: "explicit empty TCP slice (Exposes{TCP:[]}) is valid",
			h: Harness{
				Name:                "stub",
				ComposeProfile:      "harness-stub",
				Image:               "scratch",
				SmokeContractSchema: "stub.smoke_contract.v1",
				DomainDescription:   "stub for testing",
				Exposes:             Exposes{TCP: []PortExpose{}},
			},
		},
		{name: "missing name", h: Harness{ComposeProfile: "p", Image: "i", SmokeContractSchema: "s", DomainDescription: "d"}, wantErr: "name required"},
		{
			// ADR-034 §"What R3.7.2 work is preserved": compose_profile
			// is optional. A Testcontainers-managed harness (process-
			// local-testcontainer Approach) ships with no profile.
			name: "no compose_profile is valid",
			h: Harness{
				Name:                "meshtasticd-3.x",
				Image:               "meshtastic/meshtasticd:3.5.0",
				SmokeContractSchema: "meshtasticd.smoke_contract.v1",
				DomainDescription:   "Real Meshtastic protocol over TCP via Testcontainers.",
				Exposes:             Exposes{TCP: []PortExpose{{Port: 4403, Protocol: "meshtastic-protobuf"}}},
			},
		},
		{name: "missing image", h: Harness{Name: "n", ComposeProfile: "p", SmokeContractSchema: "s", DomainDescription: "d"}, wantErr: "image required"},
		{name: "missing schema", h: Harness{Name: "n", ComposeProfile: "p", Image: "i", DomainDescription: "d"}, wantErr: "smoke_contract_schema required"},
		{name: "missing domain_description", h: Harness{Name: "n", ComposeProfile: "p", Image: "i", SmokeContractSchema: "s"}, wantErr: "domain_description required"},
		{
			name: "port out of range",
			h: Harness{
				Name: "n", ComposeProfile: "p", Image: "i", SmokeContractSchema: "s", DomainDescription: "d",
				Exposes: Exposes{TCP: []PortExpose{{Port: 70000, Protocol: "x"}}},
			},
			wantErr: "out of range",
		},
		{
			name: "port protocol missing",
			h: Harness{
				Name: "n", ComposeProfile: "p", Image: "i", SmokeContractSchema: "s", DomainDescription: "d",
				Exposes: Exposes{TCP: []PortExpose{{Port: 1234}}},
			},
			wantErr: "protocol required",
		},
		{
			name: "dep missing groupId",
			h: Harness{
				Name: "n", ComposeProfile: "p", Image: "i", SmokeContractSchema: "s", DomainDescription: "d",
				RealDependencies: []Dependency{{ArtifactID: "x", VersionRange: "[1,)"}},
			},
			wantErr: "groupId required",
		},
		{
			name: "dep missing artifactId",
			h: Harness{
				Name: "n", ComposeProfile: "p", Image: "i", SmokeContractSchema: "s", DomainDescription: "d",
				RealDependencies: []Dependency{{GroupID: "x", VersionRange: "[1,)"}},
			},
			wantErr: "artifactId required",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.h.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("expected nil error, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestParseFile(t *testing.T) {
	t.Run("empty harnesses array", func(t *testing.T) {
		entries, err := ParseFile([]byte(`{"harnesses": []}`))
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 0 {
			t.Fatalf("expected 0 entries, got %d", len(entries))
		}
	})

	t.Run("missing harnesses key parses to empty slice", func(t *testing.T) {
		entries, err := ParseFile([]byte(`{}`))
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 0 {
			t.Fatalf("expected 0 entries, got %d", len(entries))
		}
	})

	t.Run("$comment field tolerated", func(t *testing.T) {
		entries, err := ParseFile([]byte(`{"$comment":"hi","harnesses":[]}`))
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 0 {
			t.Fatalf("expected 0 entries, got %d", len(entries))
		}
	})

	t.Run("malformed JSON", func(t *testing.T) {
		_, err := ParseFile([]byte(`{not json`))
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("entry fails validate", func(t *testing.T) {
		body := `{"harnesses":[{"name":"","compose_profile":"p","image":"i","smoke_contract_schema":"s","domain_description":"d"}]}`
		_, err := ParseFile([]byte(body))
		if err == nil || !strings.Contains(err.Error(), "name required") {
			t.Fatalf("expected name-required error, got %v", err)
		}
	})

	t.Run("duplicate names rejected", func(t *testing.T) {
		body := `{"harnesses":[
			{"name":"a","compose_profile":"p","image":"i","smoke_contract_schema":"s","domain_description":"d"},
			{"name":"a","compose_profile":"p2","image":"i","smoke_contract_schema":"s","domain_description":"d"}
		]}`
		_, err := ParseFile([]byte(body))
		if err == nil || !strings.Contains(err.Error(), "duplicate name") {
			t.Fatalf("expected duplicate-name error, got %v", err)
		}
	})

	t.Run("valid full entry", func(t *testing.T) {
		body := `{"harnesses":[{
			"name":"meshtasticd-3.x",
			"compose_profile":"harness-meshtasticd",
			"image":"meshtastic/meshtasticd:3.5.0",
			"exposes":{"tcp":[{"port":4403,"protocol":"meshtastic-protobuf"}]},
			"smoke_contract_schema":"meshtastic.smoke_contract.v1",
			"real_dependencies":[{"groupId":"com.geeksville.mesh","artifactId":"meshtastic-protobufs","version_range":"[2.x,3.x)"}],
			"domain_description":"Real Meshtastic protocol over TCP."
		}]}`
		entries, err := ParseFile([]byte(body))
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 1 {
			t.Fatalf("expected 1 entry, got %d", len(entries))
		}
		got := entries[0]
		if got.Name != "meshtasticd-3.x" {
			t.Errorf("name: got %q", got.Name)
		}
		if len(got.Exposes.TCP) != 1 || got.Exposes.TCP[0].Port != 4403 {
			t.Errorf("exposes: got %+v", got.Exposes)
		}
		if len(got.RealDependencies) != 1 || got.RealDependencies[0].GroupID != "com.geeksville.mesh" {
			t.Errorf("real_dependencies: got %+v", got.RealDependencies)
		}
	})
}
