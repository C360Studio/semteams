package testharness

import (
	"strings"
	"testing"
)

func TestTestHarnessValidate(t *testing.T) {
	tests := []struct {
		name    string
		h       TestHarness
		wantErr string // substring; "" = expect nil
	}{
		{
			name: "complete valid test harness",
			h: TestHarness{
				Name:                "meshtasticd-2.x",
				ComposeProfile:      "harness-meshtasticd",
				Image:               "meshtastic/meshtasticd:2.7.23-alpine",
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
			h: TestHarness{
				Name:                "stub",
				ComposeProfile:      "harness-stub",
				Image:               "scratch",
				SmokeContractSchema: "stub.smoke_contract.v1",
				DomainDescription:   "stub for testing",
			},
		},
		{
			name: "empty exposes struct (Exposes{TCP:nil}) is valid",
			h: TestHarness{
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
			h: TestHarness{
				Name:                "stub",
				ComposeProfile:      "harness-stub",
				Image:               "scratch",
				SmokeContractSchema: "stub.smoke_contract.v1",
				DomainDescription:   "stub for testing",
				Exposes:             Exposes{TCP: []PortExpose{}},
			},
		},
		{name: "missing name", h: TestHarness{ComposeProfile: "p", Image: "i", SmokeContractSchema: "s", DomainDescription: "d"}, wantErr: "name required"},
		{
			// ADR-034 §"What R3.7.2 work is preserved": compose_profile
			// is optional. A Testcontainers-managed test harness (process-
			// local-testcontainer runtime) ships with no profile.
			name: "no compose_profile is valid",
			h: TestHarness{
				Name:                "meshtasticd-2.x",
				Image:               "meshtastic/meshtasticd:2.7.23-alpine",
				SmokeContractSchema: "meshtasticd.smoke_contract.v1",
				DomainDescription:   "Real Meshtastic protocol over TCP via Testcontainers.",
				Exposes:             Exposes{TCP: []PortExpose{{Port: 4403, Protocol: "meshtastic-protobuf"}}},
			},
		},
		{name: "missing image", h: TestHarness{Name: "n", ComposeProfile: "p", SmokeContractSchema: "s", DomainDescription: "d"}, wantErr: "image required"},
		{name: "missing schema", h: TestHarness{Name: "n", ComposeProfile: "p", Image: "i", DomainDescription: "d"}, wantErr: "smoke_contract_schema required"},
		{name: "missing domain_description", h: TestHarness{Name: "n", ComposeProfile: "p", Image: "i", SmokeContractSchema: "s"}, wantErr: "domain_description required"},
		{
			name: "port out of range",
			h: TestHarness{
				Name: "n", ComposeProfile: "p", Image: "i", SmokeContractSchema: "s", DomainDescription: "d",
				Exposes: Exposes{TCP: []PortExpose{{Port: 70000, Protocol: "x"}}},
			},
			wantErr: "out of range",
		},
		{
			name: "port protocol missing",
			h: TestHarness{
				Name: "n", ComposeProfile: "p", Image: "i", SmokeContractSchema: "s", DomainDescription: "d",
				Exposes: Exposes{TCP: []PortExpose{{Port: 1234}}},
			},
			wantErr: "protocol required",
		},
		{
			name: "dep missing groupId",
			h: TestHarness{
				Name: "n", ComposeProfile: "p", Image: "i", SmokeContractSchema: "s", DomainDescription: "d",
				RealDependencies: []Dependency{{ArtifactID: "x", VersionRange: "[1,)"}},
			},
			wantErr: "groupId required",
		},
		{
			name: "dep missing artifactId",
			h: TestHarness{
				Name: "n", ComposeProfile: "p", Image: "i", SmokeContractSchema: "s", DomainDescription: "d",
				RealDependencies: []Dependency{{GroupID: "x", VersionRange: "[1,)"}},
			},
			wantErr: "artifactId required",
		},
		{
			name: "tooling pin missing groupId",
			h: TestHarness{
				Name: "n", Image: "i", SmokeContractSchema: "s", DomainDescription: "d",
				ToolingPins: []ToolingPin{{ArtifactID: "testcontainers", Version: "2.0.5"}},
			},
			wantErr: "tooling_pins[0].groupId required",
		},
		{
			name: "tooling pin missing artifactId",
			h: TestHarness{
				Name: "n", Image: "i", SmokeContractSchema: "s", DomainDescription: "d",
				ToolingPins: []ToolingPin{{GroupID: "org.testcontainers", Version: "2.0.5"}},
			},
			wantErr: "tooling_pins[0].artifactId required",
		},
		{
			name: "tooling pin missing version (range expression NOT accepted — pins exist to prevent LLM drift)",
			h: TestHarness{
				Name: "n", Image: "i", SmokeContractSchema: "s", DomainDescription: "d",
				ToolingPins: []ToolingPin{{GroupID: "org.testcontainers", ArtifactID: "testcontainers"}},
			},
			wantErr: "tooling_pins[0].version required",
		},
		{
			name: "tooling pin happy path (full triple + note)",
			h: TestHarness{
				Name: "n", Image: "i", SmokeContractSchema: "s", DomainDescription: "d",
				ToolingPins: []ToolingPin{{
					GroupID: "org.testcontainers", ArtifactID: "testcontainers", Version: "2.0.5",
					Note: "ships docker-java with Engine 29 support",
				}},
			},
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

func TestResolve_PropagatesToolingPins(t *testing.T) {
	h := TestHarness{
		Name:                "h",
		Image:               "img",
		SmokeContractSchema: "s",
		DomainDescription:   "d",
		Exposes:             Exposes{TCP: []PortExpose{{Port: 4403, Protocol: "p"}}},
		ToolingPins: []ToolingPin{
			{GroupID: "org.testcontainers", ArtifactID: "testcontainers", Version: "2.0.5", Note: "engine29"},
			{GroupID: "org.junit.jupiter", ArtifactID: "junit-jupiter", Version: "5.11.4"},
		},
	}
	m := h.Resolve()
	if got := len(m.ToolingPins); got != 2 {
		t.Fatalf("expected 2 tooling pins propagated to manifest, got %d", got)
	}
	if m.ToolingPins[0].Version != "2.0.5" {
		t.Errorf("first pin version = %q, want 2.0.5", m.ToolingPins[0].Version)
	}
	if m.ToolingPins[0].Note != "engine29" {
		t.Errorf("first pin note dropped: %q", m.ToolingPins[0].Note)
	}
	if m.ToolingPins[1].ArtifactID != "junit-jupiter" {
		t.Errorf("second pin artifactId = %q, want junit-jupiter", m.ToolingPins[1].ArtifactID)
	}
}

func TestResolve_NoToolingPins_FieldOmitted(t *testing.T) {
	h := TestHarness{
		Name:                "h",
		Image:               "img",
		SmokeContractSchema: "s",
		DomainDescription:   "d",
	}
	m := h.Resolve()
	if m.ToolingPins != nil {
		t.Errorf("expected nil ToolingPins when source has none, got %v", m.ToolingPins)
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
			"name":"meshtasticd-2.x",
			"compose_profile":"harness-meshtasticd",
			"image":"meshtastic/meshtasticd:2.7.23-alpine",
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
		if got.Name != "meshtasticd-2.x" {
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
