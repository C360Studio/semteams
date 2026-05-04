package harness

import (
	"encoding/json"
	"fmt"
)

// PortExpose describes one network port a harness sidecar exposes.
type PortExpose struct {
	Port     int    `json:"port"`
	Protocol string `json:"protocol"`
}

// Exposes enumerates the network surfaces a harness sidecar provides.
// TCP-only in v1; UDP / Unix sockets / etc. land additively when a
// concrete harness needs them.
type Exposes struct {
	TCP []PortExpose `json:"tcp,omitempty"`
}

// Dependency declares one Maven coordinate the harness expects to be
// on the builder's classpath at runtime. Used by the architect to
// scope smoke contracts and by the builder to verify pom.xml lists
// matching coordinates before `mvn verify`.
type Dependency struct {
	GroupID      string `json:"groupId"`
	ArtifactID   string `json:"artifactId"`
	VersionRange string `json:"version_range"`
}

// Harness is one entry in the catalog. Schema mirrors ADR-033 §1.
type Harness struct {
	Name                string       `json:"name"`
	ComposeProfile      string       `json:"compose_profile"`
	Image               string       `json:"image"`
	Exposes             Exposes      `json:"exposes"`
	SmokeContractSchema string       `json:"smoke_contract_schema"`
	RealDependencies    []Dependency `json:"real_dependencies,omitempty"`
	DomainDescription   string       `json:"domain_description"`
}

// Validate checks structural well-formedness. Substantive checks
// (image pullability, compose profile actually exists in the
// deployment's docker-compose file, smoke schema registered as a
// payload type) are deployment-level concerns, not catalog-load
// concerns.
func (h *Harness) Validate() error {
	if h.Name == "" {
		return fmt.Errorf("name required")
	}
	if h.ComposeProfile == "" {
		return fmt.Errorf("compose_profile required")
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
	Harnesses []Harness `json:"harnesses"`
}

// ParseFile decodes catalog JSON bytes and validates each entry.
// Returns the parsed list (possibly empty) and an error iff the
// document is malformed or any entry fails Validate. Duplicate
// names are rejected.
func ParseFile(data []byte) ([]Harness, error) {
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
