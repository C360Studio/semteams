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

// ToolingPin pins a single build-tooling dependency to a known-good
// version. Operator-curated; see ToolingPins for the rationale and
// the smoke-#8-run-13 incident that motivated the field.
type ToolingPin struct {
	GroupID    string `json:"groupId"`
	ArtifactID string `json:"artifactId"`
	// Version is the EXACT version operators have validated against
	// the harness's image + the deployment's docker daemon. Range
	// expressions (e.g. "[2.0,3.0)") are NOT accepted here — pins
	// exist precisely because LLM-authored builds drift to whatever
	// the LLM remembers, and "remembered ≈ stale" is the failure
	// shape this field exists to prevent.
	Version string `json:"version"`
	// Note is a free-form one-line operator comment explaining WHY
	// this version (e.g. "2.0.5 fixes Docker Engine 29 compat — earlier
	// versions hardcode docker-java API 1.32 and time out on the
	// daemon's 1.54 API"). Surfaces in the rendered manifest so the
	// builder LLM has the rationale, not just the version.
	Note string `json:"note,omitempty"`
}

// TestHarness is one entry in the catalog. Schema mirrors ADR-033 §1
// (revised by ADR-034 to make ComposeProfile optional — see
// Validate; revised again 2026-05-10 to add ToolingPins after smoke
// #8 run-13 wedged on a stale Testcontainers version the LLM picked
// from training-cutoff knowledge).
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
	// ToolingPins names exact build-tooling versions the operator has
	// validated against this harness's image AND the deployment's
	// docker daemon. The LLM-authored pom.xml / build.gradle MUST
	// resolve these from the rendered .test-harness/manifest.json
	// instead of guessing from training-cutoff memory.
	//
	// Smoke #8 run-13 (2026-05-10) wedged when the LLM picked
	// Testcontainers 1.19.7 from memory; that version hardcodes
	// docker-java API 1.32, but the sandbox's docker daemon is at
	// API 1.54. The builder spent 70+ iterations decompiling
	// Testcontainers internals trying to work around the mismatch
	// before timeout. Pinning Testcontainers 2.0.5+ in the catalog
	// (and projecting it into the manifest) prevents the same drift.
	//
	// Optional: a harness whose builds don't need build-tooling
	// guidance can omit this field. When present, the builder
	// persona's contract requires the pom/build to use these exact
	// versions for the named groupId+artifactId pairs.
	ToolingPins []ToolingPin `json:"tooling_pins,omitempty"`
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
	for i, p := range h.ToolingPins {
		if p.GroupID == "" {
			return fmt.Errorf("tooling_pins[%d].groupId required", i)
		}
		if p.ArtifactID == "" {
			return fmt.Errorf("tooling_pins[%d].artifactId required", i)
		}
		if p.Version == "" {
			return fmt.Errorf("tooling_pins[%d].version required (operator-curated exact version, not a range — see ToolingPin doc for the smoke-#8-run-13 incident)", i)
		}
	}
	return nil
}

// ResolvedManifest is the subset of TestHarness fields the builder
// needs to instantiate a Testcontainers client — image, exposed ports,
// environment, and optional healthcheck. It is derived from a catalog
// TestHarness at emit time by emitspecartifact, embedded in the
// <slug>.checks.json sidecar, and projected by bootstrapworkspace
// into .test-harness/manifest.json in the builder's workspace.
//
// Keeping it here (rather than in emitspecartifact or bootstrapworkspace)
// means all package consumers get the same shape without importing
// implementation packages.
type ResolvedManifest struct {
	// ID is the catalog name (TestHarness.Name) — primary key.
	ID string `json:"id"`
	// Image is the docker image reference (e.g. "meshtastic/meshtasticd:2.7.23-alpine").
	Image string `json:"image"`
	// Ports maps a symbolic port label to its container-side port number.
	// Derived from Exposes.TCP: label is Protocol, value is Port.
	Ports map[string]int `json:"ports,omitempty"`
	// Env is optional static environment variables for the container.
	Env map[string]string `json:"env,omitempty"`
	// ToolingPins are operator-curated exact versions for build
	// dependencies the LLM-authored pom/build MUST use verbatim.
	// Projected from TestHarness.ToolingPins so the builder reads
	// from a single source (the manifest in its workspace) instead
	// of guessing from training-cutoff memory.
	ToolingPins []ToolingPin `json:"tooling_pins,omitempty"`
}

// Resolve derives a ResolvedManifest from a catalog TestHarness. The
// result is safe to serialize into the sidecar and workspace manifest.
func (h *TestHarness) Resolve() ResolvedManifest {
	ports := make(map[string]int, len(h.Exposes.TCP))
	for _, p := range h.Exposes.TCP {
		if p.Protocol != "" {
			ports[p.Protocol] = p.Port
		}
	}
	m := ResolvedManifest{
		ID:    h.Name,
		Image: h.Image,
	}
	if len(ports) > 0 {
		m.Ports = ports
	}
	if len(h.ToolingPins) > 0 {
		m.ToolingPins = append(m.ToolingPins, h.ToolingPins...)
	}
	return m
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
