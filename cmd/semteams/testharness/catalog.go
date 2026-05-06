package testharness

import (
	"encoding/json"
	"fmt"
)

// PortExpose describes one network port a test-harness sidecar exposes.
type PortExpose struct {
	Port     int    `json:"port"`
	Protocol string `json:"protocol"`
}

// Exposes enumerates the network surfaces a test-harness sidecar provides.
// TCP-only in v1; UDP / Unix sockets / etc. land additively when a
// concrete test harness needs them.
type Exposes struct {
	TCP []PortExpose `json:"tcp,omitempty"`
}

// Dependency declares one Maven coordinate the test harness expects to be
// on the builder's classpath at runtime. Used by the architect to
// scope smoke contracts and by the builder to verify pom.xml lists
// matching coordinates before `mvn verify`.
type Dependency struct {
	GroupID      string `json:"groupId"`
	ArtifactID   string `json:"artifactId"`
	VersionRange string `json:"version_range"`
}

// TestHarness is one entry in the catalog. Schema mirrors ADR-033 §1
// (revised by ADR-034 to make ComposeProfile optional — see
// Validate).
type TestHarness struct {
	Name string `json:"name"`
	// ComposeProfile names a docker-compose profile that the operator
	// has wired up to bring the sidecar online out-of-process.
	// OPTIONAL as of ADR-034: chains running under the process-local-
	// testcontainer runtime (sandbox + DooD/DinD + Testcontainers)
	// manage the sidecar lifecycle in-process and don't consult this
	// field. Greenfield browser-flow chains via verification-runner
	// inline `services:` in workflow YAML and also don't consult it.
	// External-sidecar runtime (operator pre-provisions) still uses
	// it. Empty string == "no profile registered for this test_harness;
	// chains must use a Testcontainers-managed lifecycle".
	ComposeProfile      string       `json:"compose_profile,omitempty"`
	Image               string       `json:"image"`
	Exposes             Exposes      `json:"exposes"`
	SmokeContractSchema string       `json:"smoke_contract_schema"`
	RealDependencies    []Dependency `json:"real_dependencies,omitempty"`
	DomainDescription   string       `json:"domain_description"`
}

// Validate checks structural well-formedness. Substantive checks
// (image pullability, compose profile actually exists in the
// deployment's docker-compose file when set, smoke schema registered
// as a payload type) are deployment-level concerns, not catalog-load
// concerns.
//
// ComposeProfile is intentionally NOT required (ADR-034 §"What R3.7.2
// work is preserved"). A test harness consumed exclusively via
// Testcontainers (the dominant path in ADR-034's verification class
// table) ships with no compose_profile field and is still well-
// formed. If the field is set, it must be non-empty; trimming /
// charset rules stay deployment-level.
func (h *TestHarness) Validate() error {
	if h.Name == "" {
		return fmt.Errorf("name required")
	}
	if h.Image == "" {
		return fmt.Errorf("image required")
	}
	if h.SmokeContractSchema == "" {
		return fmt.Errorf("smoke_contract_schema required")
	}
	if h.DomainDescription == "" {
		return fmt.Errorf("domain_description required")
	}
	for i, p := range h.Exposes.TCP {
		if p.Port <= 0 || p.Port > 65535 {
			return fmt.Errorf("exposes.tcp[%d].port out of range: %d", i, p.Port)
		}
		if p.Protocol == "" {
			return fmt.Errorf("exposes.tcp[%d].protocol required", i)
		}
	}
	for i, d := range h.RealDependencies {
		if d.GroupID == "" {
			return fmt.Errorf("real_dependencies[%d].groupId required", i)
		}
		if d.ArtifactID == "" {
			return fmt.Errorf("real_dependencies[%d].artifactId required", i)
		}
	}
	return nil
}

// File is the on-disk format of `configs/harnesses.json`.
type File struct {
	Harnesses []TestHarness `json:"harnesses"`
}

// ParseFile decodes catalog JSON bytes and validates each entry.
// Returns the parsed list (possibly empty) and an error iff the
// document is malformed or any entry fails Validate. Duplicate
// names are rejected.
func ParseFile(data []byte) ([]TestHarness, error) {
	var f File
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("decode catalog: %w", err)
	}
	seen := make(map[string]struct{}, len(f.Harnesses))
	for i := range f.Harnesses {
		h := &f.Harnesses[i]
		if err := h.Validate(); err != nil {
			return nil, fmt.Errorf("harnesses[%d] (%q): %w", i, h.Name, err)
		}
		if _, dup := seen[h.Name]; dup {
			return nil, fmt.Errorf("harnesses[%d]: duplicate name %q", i, h.Name)
		}
		seen[h.Name] = struct{}{}
	}
	return f.Harnesses, nil
}
